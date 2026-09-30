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
	"github.com/Ayush1388/auctionEngine/internal/bidding"
	"github.com/Ayush1388/auctionEngine/internal/config"
	"github.com/Ayush1388/auctionEngine/internal/database"
	"github.com/Ayush1388/auctionEngine/internal/email"
	"github.com/Ayush1388/auctionEngine/internal/handlers"
	"github.com/Ayush1388/auctionEngine/internal/outbox"
	"github.com/Ayush1388/auctionEngine/internal/ratelimit"
	"github.com/Ayush1388/auctionEngine/internal/server"
	"github.com/Ayush1388/auctionEngine/internal/session"
	"github.com/Ayush1388/auctionEngine/internal/user"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

// main is the composition root: the one place that reads configuration,
// builds every dependency and wires them together (dependency injection by
// hand, no framework). Packages below never construct their own
// dependencies, which is what lets tests swap in a test database or a fake
// clock.
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

	// --------------------------------------------------
	// Bidding and wallets
	// --------------------------------------------------

	biddingService := bidding.NewService(
		db,
		bidding.Strategy(cfg.BidLocking),
	)

	walletService := wallet.NewService(db)

	// Completing an auction triggers settlement: the winner's reserved
	// money moves to the seller. Idempotent, because the outbox may
	// deliver an event more than once.
	outboxRouter.Register(
		auction.EventTypeCompleted,
		biddingService.SettlementHandler(),
	)

	// bid.placed and auction.settled have no consumer yet (WebSockets and
	// Kafka arrive later). Acknowledge them so they aren't retried.
	for _, eventType := range []string{bidding.EventTypePlaced, bidding.EventTypeSettled} {
		outboxRouter.Register(
			eventType,
			outbox.HandlerFunc(func(ctx context.Context, event outbox.Event) error {
				return nil
			}),
		)
	}

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

	// --------------------------------------------------
	// Sessions and rate limiting
	// --------------------------------------------------

	sessionService := session.NewService(
		db,
		jwtService,
		cfg.RefreshTokenTTL,
	)

	// In-memory token buckets: per process. v0.5 swaps in Redis so every
	// instance shares the same buckets.
	memoryLimiter := ratelimit.NewMemory()

	var limiter ratelimit.Limiter
	if cfg.RateLimitsEnabled {
		limiter = memoryLimiter
	}

	clientIP, err := ratelimit.NewClientIP(cfg.TrustedProxies)
	if err != nil {
		logger.Error("invalid TRUSTED_PROXIES", "error", err)
		os.Exit(1)
	}

	userHandler := handlers.NewUserHandler(
		userService,
		sessionService,
		limiter,
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
		server.Routes(server.Deps{
			Users:    userHandler,
			Sessions: handlers.NewSessionHandler(sessionService),
			Auctions: auctionHandler,
			Bids:     handlers.NewBidHandler(biddingService),
			Wallets:  handlers.NewWalletHandler(walletService),
			Admin:    handlers.NewAdminHandler(walletService, outboxRepository),

			Auth:     authMiddleware,
			Limiter:  limiter,
			ClientIP: clientIP,
			Limits:   server.DefaultLimits(),

			Logger:      logger,
			CORSOrigins: cfg.CORSAllowedOrigins,
			HSTS:        cfg.Environment == "production",
		}),
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

	// Forget idle rate-limit buckets so the map can't grow forever.
	workers.Go(func() {
		memoryLimiter.RunJanitor(workersCtx, time.Minute, 10*time.Minute)
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
