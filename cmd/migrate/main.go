package main

import (
	"fmt"
	"os"

	"github.com/Ayush1338/auctionEngine/internal/database"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		fmt.Println("failed to load .env:", err)
		return
	}
	databaseURL := os.Getenv("DATABASE_URL")

	pool, err := database.NewPostgresPool(databaseURL)
	if err != nil {
		fmt.Println("failed to connect", err)
		return
	}
	defer pool.Close()
	fmt.Println("database connection successful")
}
