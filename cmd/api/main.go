package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/auth"
	"github.com/Ayush1388/auctionEngine/internal/config"
	"github.com/Ayush1388/auctionEngine/internal/database"
	"github.com/Ayush1388/auctionEngine/internal/email"
	"github.com/Ayush1388/auctionEngine/internal/handlers"
	"github.com/Ayush1388/auctionEngine/internal/outbox"
	"github.com/Ayush1388/auctionEngine/internal/server"
	"github.com/Ayush1388/auctionEngine/internal/user"
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
			logger.Error(
				"failed to load .env",
				"error",
				err,
			)
			os.Exit(1)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Error(
			"failed to load configuration",
			"error",
			err,
		)
		os.Exit(1)
	}

	logger.Info(
		"starting application",
		"environment",
		cfg.Environment,
		"port",
		cfg.Port,
	)

	db, err := database.NewPostgresPool(
		cfg.DatabaseURL,
	)
	if err != nil {
		logger.Error(
			"failed to connect to PostgreSQL",
			"error",
			err,
		)
		os.Exit(1)
	}
	defer db.Close()

	logger.Info("connected to PostgreSQL")

	// --------------------------------------------------
	// Email
	// --------------------------------------------------

	emailService := email.NewService(
		cfg.SMTPHost,
		cfg.SMTPPort,
		cfg.SMTPUsername,
		cfg.SMTPPassword,
		cfg.SMTPFrom,
		cfg.AppBaseURL,
	)

	// --------------------------------------------------
	// Outbox
	// --------------------------------------------------

	outboxRepository := outbox.NewRepository(db)

	emailEventHandler := email.NewEventHandler(
		emailService,
	)

	outboxRouter := outbox.NewRouter()

	outboxRouter.Register(
		email.EventTypeActivationEmail,
		emailEventHandler,
		// The raw token is a credential; drop it once the email is sent.
		outbox.RedactOnSuccess("activation_token"),
	)

	// Nothing consumes auction.completed until settlement lands in v0.3.
	// Acknowledge it so it isn't retried forever.
	outboxRouter.Register(
		auction.EventTypeCompleted,
		outbox.HandlerFunc(func(ctx context.Context, event outbox.Event) error {
			logger.Info("auction completed", "event_id", event.ID)
			return nil
		}),
	)

	outboxWorker := outbox.NewWorker(
		outboxRepository,
		outboxRouter,
		logger,
	)

	// --------------------------------------------------
	// Authentication
	// --------------------------------------------------

	jwtService := user.NewJWTService(
		cfg.JWTSecret,
		cfg.JWTIssuer,
		cfg.JWTExpiration,
	)

	authMiddleware := auth.NewMiddleware(
		jwtService,
	)

	// --------------------------------------------------
	// Users
	// --------------------------------------------------

	userRepository := user.NewRepository(db)

	userService := user.NewService(
		db,
		userRepository,
		jwtService,
	)

	userHandler := handlers.NewUserHandler(
		userService,
	)

	// --------------------------------------------------
	// Auctions
	// --------------------------------------------------

	auctionService := auction.NewService(
		db,
		auction.NewRepository(db),
	)

	auctionHandler := handlers.NewAuctionHandler(
		auctionService,
	)

	// --------------------------------------------------
	// HTTP server
	// --------------------------------------------------

	srv := server.New(
		cfg.Port,
		server.Routes(
			userHandler,
			auctionHandler,
			authMiddleware,
		),
	)

	// --------------------------------------------------
	// Background workers
	// --------------------------------------------------

	workersCtx, stopWorkers := context.WithCancel(
		context.Background(),
	)
	defer stopWorkers()

	var workers sync.WaitGroup

	lifecycleWorker := auction.NewLifecycleWorker(
		db,
		logger,
	)

	workers.Go(func() {
		outboxWorker.Run(workersCtx)
	})

	workers.Go(func() {
		lifecycleWorker.Run(workersCtx)
	})

	// --------------------------------------------------
	// Start HTTP server
	// --------------------------------------------------

	serverErrors := make(chan error, 1)

	go func() {
		serverErrors <- srv.Start()
	}()

	logger.Info(
		"HTTP server started",
		"port",
		cfg.Port,
	)

	// --------------------------------------------------
	// Graceful shutdown
	// --------------------------------------------------

	shutdownSignals := make(chan os.Signal, 1)

	signal.Notify(
		shutdownSignals,
		os.Interrupt,
		syscall.SIGTERM,
	)

	defer signal.Stop(shutdownSignals)

	exitCode := 0

	select {
	case err := <-serverErrors:
		// Start only returns early if the server failed, e.g. the port
		// was already in use.
		logger.Error(
			"HTTP server error",
			"error",
			err,
		)
		exitCode = 1

	case sig := <-shutdownSignals:
		logger.Info(
			"shutdown signal received",
			"signal",
			sig.String(),
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		shutdownTimeout,
	)
	defer cancel()

	if err := shutdown(ctx, srv, stopWorkers, &workers); err != nil {
		logger.Error(
			"graceful shutdown failed",
			"error",
			err,
		)
		exitCode = 1
	}

	logger.Info("application stopped")

	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

const shutdownTimeout = 10 * time.Second

// shutdown stops the HTTP server first, so no new work (and no new outbox
// events) arrives, then stops the background workers and waits for them to
// finish their current batch. Everything shares one deadline.
func shutdown(
	ctx context.Context,
	srv *server.Server,
	stopWorkers context.CancelFunc,
	workers *sync.WaitGroup,
) error {
	// Stops accepting connections and waits for in-flight requests.
	httpErr := srv.Shutdown(ctx)

	stopWorkers()

	done := make(chan struct{})
	go func() {
		workers.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		return errors.Join(httpErr, fmt.Errorf("workers did not stop in time: %w", ctx.Err()))
	}

	if httpErr != nil {
		return fmt.Errorf("http server: %w", httpErr)
	}

	return nil
}
