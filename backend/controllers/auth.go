package controllers

import (
	"backend/models"
	"backend/utils"
	"errors"
	"log"
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
// @Description Вход пользователя по почте и паролю, выдача токенов
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body models.LoginBody true "Данные для входа"
// @Success 200 {object} models.TokenResponse
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

	c.JSON(200, models.TokenResponse{AccessToken: accessToken, RefreshToken: refreshToken})
}

// @Summary Обновление токена
// @Description Выдаёт новый access-токен по refresh-токену
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body models.RefreshBody true "Refresh-токен"
// @Success 200 {object} models.AccessTokenResponse
// @Failure 401 {object} models.ErrorResponse "Invalid or expired refresh token"
// @Router /auth/refresh [post]
func (ctrl *AuthController) Refresh(c *gin.Context) {
	ctx := c.Request.Context()

	var body models.RefreshBody

	if err := c.BindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}

	if err := validate.Struct(body); err != nil {
		c.JSON(400, gin.H{"errors": utils.FormatValidationError(err)})
		return
	}

	claims, err := utils.VerifyToken(body.RefreshToken, utils.TokenTypeRefresh)
	if err != nil {
		c.JSON(401, models.ErrorResponse{Message: "Invalid or expired refresh token"})
		return
	}

	var exists bool
	err = ctrl.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM refresh_tokens WHERE token_hash = $1 AND expires_at > now())`,
		utils.HashToken(body.RefreshToken),
	).Scan(&exists)
	if err != nil {
		log.Println(err)
		c.JSON(500, models.ErrorResponse{Message: "Failed to refresh token, try again~"})
		return
	}

	// ponytail: DB row (not just JWT signature) is the source of truth, so a
	// logged-out/revoked token is rejected even before its JWT expiry hits.
	// Not rotated: the response contract only returns a new access_token.
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

	c.JSON(200, models.AccessTokenResponse{AccessToken: accessToken})
}
