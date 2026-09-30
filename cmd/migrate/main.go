package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/joho/godotenv"

	"github.com/Ayush1388/auctionEngine/internal/database"
	"github.com/Ayush1388/auctionEngine/internal/migration"
)

// Usage:
//
//	go run ./cmd/migrate            apply all pending migrations
//	go run ./cmd/migrate -down 1    roll back the latest migration
func main() {
	down := flag.Int("down", 0, "roll back this many migrations instead of applying")
	dir := flag.String("dir", "migrations", "directory containing migration files")
	flag.Parse()

	if err := run(*down, *dir); err != nil {
		fmt.Fprintln(os.Stderr, "migration failed:", err)
		os.Exit(1)
	}
}

func run(down int, dir string) error {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to load .env: %w", err)
	}

	// Only the database URL is needed here, so we don't require the
	// SMTP and JWT settings that the API server needs.
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	pool, err := database.NewPostgresPool(databaseURL)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer pool.Close()

	runner := migration.NewRunner(pool, dir)
	ctx := context.Background()

	if down > 0 {
		return runner.Down(ctx, down)
	}

	return runner.Up(ctx)
}
