// Command bidworker runs bid workers without the HTTP API.
//
// The API process can run bid workers itself (BID_WORKERS, default 1), but
// in production you scale them separately: more API instances for more
// users, more bid workers for more bids. Workers in the same consumer group
// split the partitions between them automatically; there is no point
// running more workers than the topic has partitions (kafkax.Partitions).
//
//	BID_WORKERS=4 go run ./cmd/bidworker
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/Ayush1388/auctionEngine/internal/bidding"
	"github.com/Ayush1388/auctionEngine/internal/bidqueue"
	"github.com/Ayush1388/auctionEngine/internal/database"
	"github.com/Ayush1388/auctionEngine/internal/health"
	"github.com/Ayush1388/auctionEngine/internal/kafkax"
	"github.com/Ayush1388/auctionEngine/internal/metrics"
	"github.com/Ayush1388/auctionEngine/internal/server"
	"github.com/Ayush1388/auctionEngine/internal/telemetry"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "bidworker")
	if err := run(logger); err != nil {
		logger.Error("bidworker failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	dbURL := os.Getenv("DATABASE_URL")
	brokers := kafkax.Brokers(os.Getenv("KAFKA_BROKERS"))
	if dbURL == "" || len(brokers) == 0 {
		return errors.New("DATABASE_URL and KAFKA_BROKERS are required")
	}
	count := 1
	if n, err := strconv.Atoi(os.Getenv("BID_WORKERS")); err == nil && n > 0 {
		count = n
	}

	shutdownTracing, err := telemetry.Setup(context.Background(), "auction-bidworker")
	if err != nil {
		return err
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTracing(ctx)
	}()

	db, err := database.NewPostgresPool(dbURL)
	if err != nil {
		return err
	}
	defer db.Close()
	metrics.RegisterPool(db)

	topics := kafkax.TopicsWithPrefix(os.Getenv("KAFKA_TOPIC_PREFIX"))
	producer, err := kafkax.NewProducer(brokers)
	if err != nil {
		return err
	}
	defer producer.Close()

	// The worker has no public port, so its probes and metrics live on the
	// admin listener (ADMIN_ADDR, e.g. :9092). Kafka is critical HERE: a
	// bid worker that can't reach Kafka has nothing to do.
	checker := health.New()
	checker.Add("postgres", true, db.Ping)
	checker.Add("kafka", true, producer.Ping)
	stopAdmin := server.StartAdmin(os.Getenv("ADMIN_ADDR"), checker, logger)
	defer stopAdmin(context.Background())

	strategy := bidding.Pessimistic
	if os.Getenv("BID_STRATEGY") == "optimistic" {
		strategy = bidding.Optimistic
	}
	bids := bidding.NewService(db, strategy)
	requests := bidqueue.NewRequests(db)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		w, err := bidqueue.NewWorker(brokers, "bid-workers", topics.Bids, topics.DLQ, producer, bids.PlaceBid, requests, logger)
		if err != nil {
			return err
		}
		wg.Go(func() { w.Run(ctx) })
	}
	logger.Info("bid workers running", "count", count, "topic", topics.Bids)

	<-ctx.Done()
	logger.Info("shutting down: finishing current batches")
	wg.Wait()
	return nil
}
