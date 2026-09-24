package main

import (
	"backend/controllers"
	"backend/db"
	"backend/middleware"
	"context"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	docs "backend/docs"

	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// @title Readness API
// @version 1.0
// @host localhost:8080
// @BasePath /
func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env found")
	}

	dsn := os.Getenv("DATABASE_URL")

	ctx := context.Background()

	pool, err := db.NewPool(ctx, dsn)

	if err != nil {
		log.Fatal(err)
	}

	defer pool.Close()

	if err := db.ApplySchema(ctx, pool); err != nil {
		log.Fatal(err)
	}

	r := gin.Default()

	origin := os.Getenv("CORS_ORIGIN")
	if origin == "" {
		origin = "http://localhost:3000" // credentials require an explicit origin, not "*"
	}
	r.Use(func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Access-Control-Allow-Credentials", "true")
		h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	docs.SwaggerInfo.BasePath = "/"

	r.GET("/test", func(ctx *gin.Context) {
		ctx.JSON(200, gin.H{"message": "Hello, World!"})
	})

	auth := r.Group("/auth")
	authController := controllers.NewAuthController(pool)
	auth.POST("/signup", authController.SignUp)
	auth.POST("/login", authController.Login)
	auth.POST("/refresh", authController.Refresh)
	auth.POST("/logout", authController.Logout)

	users := controllers.NewUserController(pool)
	r.GET("/users/me", middleware.RequireAuth, users.Me)
	r.PATCH("/users/:id/role", middleware.RequireAuth, middleware.RequireRole(pool, "admin"), users.ChangeRole)

	books := controllers.NewBookController(pool)
	r.GET("/books", books.List)
	r.GET("/books/:id", middleware.OptionalAuth, books.Get)
	r.GET("/books/:id/cover", middleware.OptionalAuth, books.GetCover)
	r.PUT("/books/:id/cover", middleware.RequireAuth, books.UploadCover)
	r.GET("/users/me/books", middleware.RequireAuth, books.MyBooks)
	r.POST("/books", middleware.RequireAuth, books.Create)
	r.PATCH("/books/:id", middleware.RequireAuth, books.Update)
	r.DELETE("/books/:id", middleware.RequireAuth, books.Delete)

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))
	r.Run()
}
