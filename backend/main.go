package main

import (
	"backend/db"
	"context"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

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

	r.GET("/test", func(ctx *gin.Context) {
		ctx.JSON(200, gin.H{"message": "Hello, World!"})
	})

	r.Run()
}
