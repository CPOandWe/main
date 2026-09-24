package middleware

import (
	"backend/models"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

const RoleKey = "role"

var roleRank = map[string]int{"user": 1, "moderator": 2, "admin": 3}

func RequireRole(pool *pgxpool.Pool, minRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var role string
		err := pool.QueryRow(c.Request.Context(),
			`SELECT r.name FROM users u JOIN roles r USING (role_id) WHERE u.user_id = $1`,
			c.GetString(UserIDKey),
		).Scan(&role)
		if err != nil {
			log.Println(err)
			c.AbortWithStatusJSON(401, models.ErrorResponse{Message: "Unauthorized"})
			return
		}

		if roleRank[role] < roleRank[minRole] {
			c.AbortWithStatusJSON(403, models.ErrorResponse{Message: "Forbidden"})
			return
		}

		c.Set(RoleKey, role)
		c.Next()
	}
}
