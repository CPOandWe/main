package controllers

import (
	"backend/middleware"
	"backend/models"
	"log"

	"github.com/gin-gonic/gin"
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
