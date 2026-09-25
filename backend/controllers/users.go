package controllers

import (
	"backend/middleware"
	"backend/models"
	"backend/utils"
	"errors"
	"log"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserController struct {
	pool *pgxpool.Pool
}

func NewUserController(pool *pgxpool.Pool) *UserController {
	return &UserController{pool: pool}
}

// @Summary Текущий пользователь
// @Description Данные авторизованного пользователя (по access-токену из cookie)
// @Tags Users
// @Produce json
// @Success 200 {object} models.MeResponse
// @Failure 401 {object} models.ErrorResponse "Unauthorized"
// @Router /users/me [get]
func (ctrl *UserController) Me(c *gin.Context) {
	var me models.MeResponse
	err := ctrl.pool.QueryRow(c.Request.Context(),
		`SELECT u.user_id, u.username, u.email, r.name
		 FROM users u JOIN roles r USING (role_id)
		 WHERE u.user_id = $1`, c.GetString(middleware.UserIDKey),
	).Scan(&me.UserID, &me.Username, &me.Email, &me.Role)
	if err != nil {
		// no row = user deleted after token was issued
		log.Println(err)
		c.JSON(401, models.ErrorResponse{Message: "Unauthorized"})
		return
	}

	c.JSON(200, me)
}

// @Summary Смена роли
// @Description Меняет роль пользователя (только Admin). Снятие модератора — выдача роли user
// @Tags Users
// @Accept json
// @Param id path string true "ID пользователя"
// @Param request body models.ChangeRoleBody true "Новая роль"
// @Success 204 "Роль изменена"
// @Failure 400 {object} models.ErrorResponse "Invalid request, unknown role or own role"
// @Failure 401 {object} models.ErrorResponse "Unauthorized"
// @Failure 403 {object} models.ErrorResponse "Forbidden"
// @Failure 404 {object} models.ErrorResponse "User not found"
// @Router /users/{id}/role [patch]
func (ctrl *UserController) ChangeRole(c *gin.Context) {
	id := c.Param("id")
	if validate.Var(id, "uuid") != nil {
		c.JSON(404, models.ErrorResponse{Message: "User not found"})
		return
	}

	var body models.ChangeRoleBody
	if err := c.BindJSON(&body); err != nil {
		c.JSON(400, models.ErrorResponse{Message: "invalid request"})
		return
	}
	if err := validate.Struct(body); err != nil {
		c.JSON(400, models.ErrorResponse{Message: "invalid request", Errors: utils.FormatValidationError(err)})
		return
	}

	// ponytail: blocks self-change so the last admin can't lock everyone out
	if id == c.GetString(middleware.UserIDKey) {
		c.JSON(400, models.ErrorResponse{Message: "Can't change your own role"})
		return
	}

	tag, err := ctrl.pool.Exec(c.Request.Context(),
		`UPDATE users SET role_id = $1 WHERE user_id = $2`, body.RoleID, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			c.JSON(400, models.ErrorResponse{Message: "Unknown role"})
			return
		}
		log.Println(err)
		c.JSON(500, models.ErrorResponse{Message: "Failed to change role, try again~"})
		return
	}
	if tag.RowsAffected() == 0 {
		c.JSON(404, models.ErrorResponse{Message: "User not found"})
		return
	}

	c.Status(204)
}

// @Summary Список пользователей
// @Description Только Admin. Фильтры: role, поиск q по имени и почте; по 20 на страницу
// @Tags Users
// @Produce json
// @Param role query string false "user, moderator или admin"
// @Param q query string false "Поиск по username и email"
// @Param page query int false "Страница, с 1"
// @Success 200 {array} models.UserListItem
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Router /users [get]
func (ctrl *UserController) List(c *gin.Context) {
	role := c.Query("role")
	if validate.Var(role, "omitempty,oneof=user moderator admin") != nil {
		c.JSON(400, models.ErrorResponse{Message: "invalid role"})
		return
	}
	page := 1
	if p := c.Query("page"); p != "" {
		var err error
		if page, err = strconv.Atoi(p); err != nil || page < 1 {
			c.JSON(400, models.ErrorResponse{Message: "invalid page"})
			return
		}
	}

	rows, err := ctrl.pool.Query(c.Request.Context(),
		`SELECT u.user_id, u.username, u.email, r.name AS role
		 FROM users u JOIN roles r USING (role_id)
		 WHERE ($1::text = '' OR r.name = $1)
		   AND ($2::text = '' OR u.username ILIKE '%' || $2 || '%' OR u.email ILIKE '%' || $2 || '%')
		 ORDER BY u.username, u.user_id LIMIT $3 OFFSET $4`,
		role, likeEscaper.Replace(c.Query("q")), booksPageSize, (page-1)*booksPageSize)
	if err != nil {
		internalErr(c, err)
		return
	}
	users, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.UserListItem])
	if err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(200, users)
}
