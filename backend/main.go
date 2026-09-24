package main

import (
	"backend/controllers"
	"backend/db"
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

	docs.SwaggerInfo.BasePath = "/"

	r.GET("/test", func(ctx *gin.Context) {
		ctx.JSON(200, gin.H{"message": "Hello, World!"})
	})

	auth := r.Group("/auth")
	authController := controllers.NewAuthController(pool)
	auth.POST("/signup", authController.SignUp)
	auth.POST("/login", authController.Login)
	auth.POST("/refresh", authController.Refresh)

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))
	r.Run()
}
