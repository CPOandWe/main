package middleware

import (
	"backend/models"
	"backend/utils"

	"github.com/gin-gonic/gin"
)

const UserIDKey = "user_id"

// RequireAuth checks the access_token cookie and stores the user id in the context.
func RequireAuth(c *gin.Context) {
	token, err := c.Cookie("access_token")
	if err == nil {
		var claims *utils.Claims
		if claims, err = utils.VerifyToken(token, utils.TokenTypeAccess); err == nil {
			c.Set(UserIDKey, claims.UserID)
			c.Next()
			return
		}
	}

	c.AbortWithStatusJSON(401, models.ErrorResponse{Message: "Unauthorized"})
}
