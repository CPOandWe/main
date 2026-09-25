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
		h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS, PUT")
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
	r.GET("/users", middleware.RequireAuth, middleware.RequireRole(pool, "admin"), users.List)
	r.PATCH("/users/:id/role", middleware.RequireAuth, middleware.RequireRole(pool, "admin"), users.ChangeRole)

	moderator := []gin.HandlerFunc{middleware.RequireAuth, middleware.RequireRole(pool, "moderator")}

	dicts := controllers.NewDictionaryController(pool)
	r.GET("/topics", dicts.Topics)
	r.GET("/topics/:id", dicts.Topic)
	r.POST("/topics", append(moderator, dicts.CreateTopic)...)
	r.PUT("/topics/:id", append(moderator, dicts.UpdateTopic)...)
	r.DELETE("/topics/:id", append(moderator, dicts.DeleteTopic)...)

	r.GET("/languages", dicts.Languages)
	r.GET("/languages/:id", dicts.Language)
	r.POST("/languages", append(moderator, dicts.CreateLanguage)...)
	r.PUT("/languages/:id", append(moderator, dicts.UpdateLanguage)...)
	r.DELETE("/languages/:id", append(moderator, dicts.DeleteLanguage)...)

	r.GET("/authors", dicts.Authors)
	r.GET("/authors/:id", dicts.Author)
	r.POST("/authors", middleware.RequireAuth, dicts.CreateAuthor)
	r.PUT("/authors/:id", append(moderator, dicts.UpdateAuthor)...)
	r.DELETE("/authors/:id", append(moderator, dicts.DeleteAuthor)...)

	books := controllers.NewBookController(pool)
	r.GET("/books", middleware.OptionalAuth, books.List)
	r.GET("/books/:id", middleware.OptionalAuth, books.Get)
	r.GET("/books/:id/cover", middleware.OptionalAuth, books.GetCover)
	r.PUT("/books/:id/cover", middleware.RequireAuth, books.UploadCover)
	r.PUT("/books/:id/file", middleware.RequireAuth, books.UploadFile)
	r.GET("/books/:id/download", middleware.RequireAuth, books.Download)
	r.GET("/books/:id/status", middleware.OptionalAuth, books.Status)
	r.POST("/books/:id/publish-request", middleware.RequireAuth, books.RequestPublish)

	r.POST("/books/:id/publish", append(moderator, books.Publish)...)
	r.GET("/moderation/requests", append(moderator, books.ModerationList)...)
	r.PATCH("/moderation/requests/:id", append(moderator, books.ModerationReview)...)
	r.GET("/users/me/books", middleware.RequireAuth, books.MyBooks)
	r.GET("/users/me/uploads", middleware.RequireAuth, books.UploadedBooks)
	r.PUT("/users/me/books/:id/status", middleware.RequireAuth, books.SetReadingStatus)
	r.POST("/users/me/books/:id", middleware.RequireAuth, books.AddToLibrary)
	r.DELETE("/users/me/books/:id", middleware.RequireAuth, books.RemoveFromLibrary)
	r.POST("/books", middleware.RequireAuth, books.Create)
	r.PATCH("/books/:id", middleware.RequireAuth, books.Update)
	r.DELETE("/books/:id", middleware.RequireAuth, books.Delete)

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))
	r.Run()
}
