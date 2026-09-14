package main

import (
	"context"
	"fmt"

	"github.com/Ayush1338/auctionEngine/internal/config"
	"github.com/Ayush1338/auctionEngine/internal/database"
	"github.com/Ayush1338/auctionEngine/internal/migration"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		fmt.Println("failed to load .env:", err)
		return
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Println("failed to load config:", err)
	}
	pool, err := database.NewPostgresPool(cfg.DatabaseURL)
	if err != nil {
		fmt.Println("failed to connect", err)
		return
	}
	defer pool.Close()
	runner := migration.NewRunner(pool)

	ctx := context.Background()

	if err := runner.Up(ctx); err != nil {
		fmt.Println("migration failed:", err)
		return
	}
}
