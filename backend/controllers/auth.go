package controllers

import (
	"backend/models"
	"backend/utils"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthController struct {
	pool *pgxpool.Pool
}

func NewAuthController(pool *pgxpool.Pool) *AuthController {
	return &AuthController{pool: pool}
}

const (
	accessCookie  = "access_token"
	refreshCookie = "refresh_token"
)

func setCookie(c *gin.Context, name, value, path string, ttl time.Duration) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, value, int(ttl.Seconds()), path, "", os.Getenv("COOKIE_SECURE") != "false", true)
}

var validate = validator.New(validator.WithRequiredStructEnabled())

// @Summary Регистрация
// @Description Регистрация пользователя по имени, почте и паролю
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body models.SignUpBody true "Данные для регистрации"
// @Success 200 {object} models.SignUpResponse
// @Failure 400 {object} models.ErrorResponse "Invalid request or validation error"
// @Router /auth/signup [post]
func (ctrl *AuthController) SignUp(c *gin.Context) {
	ctx := c.Request.Context()

	var body models.SignUpBody

	if err := c.BindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}

	if err := validate.Struct(body); err != nil {
		c.JSON(400, gin.H{"errors": utils.FormatValidationError(err)})
		return
	}

	passwordHash, err := utils.HashPassword(body.Password)
	if err != nil {
		log.Println(err)
		c.JSON(500, models.ErrorResponse{Message: "Failed to create user, try again~"})
		return
	}

	var userId string
	err = ctrl.pool.QueryRow(ctx,
		`INSERT INTO users (username, email, password_hash, role_id)
		 VALUES ($1, $2, $3, (SELECT role_id FROM roles WHERE name = 'user'))
		 RETURNING user_id`,
		body.Username, body.Email, passwordHash,
	).Scan(&userId)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			c.JSON(409, models.ErrorResponse{Message: "Email is already in use"})
			return
		}

		log.Println(err)
		c.JSON(500, models.ErrorResponse{Message: "Failed to create user, try again~"})
		return
	}

	c.JSON(200, models.SignUpResponse{UserID: userId})
}

// @Summary Вход
// @Description Вход пользователя по почте и паролю
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body models.LoginBody true "Данные для входа"
// @Success 204 "Токены установлены в cookie"
// @Failure 400 {object} models.ErrorResponse "Invalid request or validation error"
// @Failure 401 {object} models.ErrorResponse "Invalid credentials"
// @Router /auth/login [post]
func (ctrl *AuthController) Login(c *gin.Context) {
	ctx := c.Request.Context()

	var body models.LoginBody

	if err := c.BindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}

	if err := validate.Struct(body); err != nil {
		c.JSON(400, gin.H{"errors": utils.FormatValidationError(err)})
		return
	}

	var userId, passwordHash string
	err := ctrl.pool.QueryRow(ctx,
		`SELECT user_id, password_hash FROM users WHERE email = $1`, body.Email,
	).Scan(&userId, &passwordHash)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(401, models.ErrorResponse{Message: "Invalid email or password"})
			return
		}

		log.Println(err)
		c.JSON(500, models.ErrorResponse{Message: "Failed to sign in, try again~"})
		return
	}

	if !utils.CheckPassword(passwordHash, body.Password) {
		c.JSON(401, models.ErrorResponse{Message: "Invalid email or password"})
		return
	}

	accessToken, refreshToken, err := utils.GenerateTokenPair(userId)
	if err != nil {
		log.Println(err)
		c.JSON(500, models.ErrorResponse{Message: "Failed to sign in, try again~"})
		return
	}

	_, err = ctrl.pool.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userId, utils.HashToken(refreshToken), time.Now().Add(utils.RefreshTokenTTL),
	)
	if err != nil {
		log.Println(err)
		c.JSON(500, models.ErrorResponse{Message: "Failed to sign in, try again~"})
		return
	}

	setCookie(c, accessCookie, accessToken, "/", utils.AccessTokenTTL)
	setCookie(c, refreshCookie, refreshToken, "/auth", utils.RefreshTokenTTL)
	c.Status(204)
}

// @Summary Обновление токена
// @Description Выдаёт новый access-токен (в cookie) по refresh-токену из cookie
// @Tags Auth
// @Produce json
// @Success 204 "Новый access-токен установлен в cookie"
// @Failure 401 {object} models.ErrorResponse "Invalid or expired refresh token"
// @Router /auth/refresh [post]
func (ctrl *AuthController) Refresh(c *gin.Context) {
	ctx := c.Request.Context()

	refreshToken, err := c.Cookie(refreshCookie)
	if err != nil {
		c.JSON(401, models.ErrorResponse{Message: "Invalid or expired refresh token"})
		return
	}

	claims, err := utils.VerifyToken(refreshToken, utils.TokenTypeRefresh)
	if err != nil {
		c.JSON(401, models.ErrorResponse{Message: "Invalid or expired refresh token"})
		return
	}

	var exists bool
	err = ctrl.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM refresh_tokens WHERE token_hash = $1 AND expires_at > now())`,
		utils.HashToken(refreshToken),
	).Scan(&exists)
	if err != nil {
		log.Println(err)
		c.JSON(500, models.ErrorResponse{Message: "Failed to refresh token, try again~"})
		return
	}

	if !exists {
		c.JSON(401, models.ErrorResponse{Message: "Invalid or expired refresh token"})
		return
	}

	accessToken, err := utils.GenerateAccessToken(claims.UserID)
	if err != nil {
		log.Println(err)
		c.JSON(500, models.ErrorResponse{Message: "Failed to refresh token, try again~"})
		return
	}

	setCookie(c, accessCookie, accessToken, "/", utils.AccessTokenTTL)
	c.Status(204)
}

// @Summary Выход
// @Description Удаляет refresh-токен из БД и очищает cookie
// @Tags Auth
// @Success 204 "Cookie очищены"
// @Router /auth/logout [post]
func (ctrl *AuthController) Logout(c *gin.Context) {
	if refreshToken, err := c.Cookie(refreshCookie); err == nil {
		if _, err := ctrl.pool.Exec(c.Request.Context(),
			`DELETE FROM refresh_tokens WHERE token_hash = $1`, utils.HashToken(refreshToken),
		); err != nil {
			log.Println(err)
			c.JSON(500, models.ErrorResponse{Message: "Failed to sign out, try again~"})
			return
		}
	}

	setCookie(c, accessCookie, "", "/", -time.Second)
	setCookie(c, refreshCookie, "", "/auth", -time.Second)
	c.Status(204)
}
