package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/Ayush1338/auctionEngine/internal/config"
	"github.com/Ayush1338/auctionEngine/internal/database"
	"github.com/Ayush1338/auctionEngine/internal/server"
)

func main() {
	logger := slog.New(
		slog.NewJSONHandler(
			os.Stdout,
			&slog.HandlerOptions{
				Level: slog.LevelInfo,
			},
		),
	)

	slog.SetDefault(logger)

	if err := godotenv.Load(); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logger.Error("failed to load .env", "error", err)
			os.Exit(1)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	logger.Info(
		"starting application",
		"environment", cfg.Environment,
		"port", cfg.Port,
	)

	db, err := database.NewPostgresPool(cfg.DatabaseURL)
	if err != nil {
		logger.Error(
			"failed to connect to PostgreSQL",
			"error", err,
		)
		os.Exit(1)
	}
	defer db.Close()

	logger.Info("connected to PostgreSQL")

	srv := server.New(cfg.Port)

	serverErrors := make(chan error, 1)

	go func() {
		serverErrors <- srv.Start()
	}()

	logger.Info("HTTP server started", "port", cfg.Port)

	shutdownSignals := make(chan os.Signal, 1)

	signal.Notify(
		shutdownSignals,
		os.Interrupt,
		syscall.SIGTERM,
	)

	select {
	case err := <-serverErrors:
		if !errors.Is(err, os.ErrClosed) {
			logger.Error(
				"HTTP server error",
				"error", err,
			)
			os.Exit(1)
		}

	case sig := <-shutdownSignals:
		logger.Info(
			"shutdown signal received",
			"signal", sig.String(),
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	if err := shutdown(ctx, srv); err != nil {
		logger.Error(
			"graceful shutdown failed",
			"error", err,
		)
		os.Exit(1)
	}

	logger.Info("application stopped")
}

func shutdown(ctx context.Context, srv *server.Server) error {
	done := make(chan error, 1)

	go func() {
		done <- srv.Shutdown()
	}()

	select {
	case err := <-done:
		return err

	case <-ctx.Done():
		return ctx.Err()
	}
}
