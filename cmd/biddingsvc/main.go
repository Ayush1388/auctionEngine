// Command biddingsvc is the bidding service (v0.9): bidding.Service exposed
// over gRPC, deployable and scalable on its own.
//
//	GRPC_PORT=50051 INTERNAL_TOKEN=… DATABASE_URL=… go run ./cmd/biddingsvc
//
// The API gateway (cmd/api) calls it when BIDDING_GRPC_ADDR is set. It also
// serves the standard gRPC health-checking protocol (for load balancers and
// Kubernetes probes) and server reflection (so grpcurl can list and call
// methods without the .proto file).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/Ayush1388/auctionEngine/internal/bidding"
	"github.com/Ayush1388/auctionEngine/internal/database"
	pb "github.com/Ayush1388/auctionEngine/internal/gen/biddingv1"
	"github.com/Ayush1388/auctionEngine/internal/grpcsvc"
	apphealth "github.com/Ayush1388/auctionEngine/internal/health"
	"github.com/Ayush1388/auctionEngine/internal/metrics"
	"github.com/Ayush1388/auctionEngine/internal/realtime"
	"github.com/Ayush1388/auctionEngine/internal/redisx"
	"github.com/Ayush1388/auctionEngine/internal/server"
	"github.com/Ayush1388/auctionEngine/internal/telemetry"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "biddingsvc")
	if err := run(logger); err != nil {
		logger.Error("biddingsvc failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	port := 50051
	if p, err := strconv.Atoi(os.Getenv("GRPC_PORT")); err == nil {
		port = p
	}
	token := os.Getenv("INTERNAL_TOKEN")
	if token == "" {
		logger.Warn("INTERNAL_TOKEN is empty: any caller can use this service (development only)")
	}

	shutdownTracing, err := telemetry.Setup(context.Background(), "auction-biddingsvc")
	if err != nil {
		return err
	}
	defer flush(shutdownTracing)

	db, err := database.NewPostgresPool(dbURL)
	if err != nil {
		return err
	}
	defer db.Close()
	metrics.RegisterPool(db)

	// ADMIN_ADDR (e.g. :9091) serves /metrics and pprof.
	checker := apphealth.New()
	checker.Add("postgres", true, db.Ping)
	stopAdmin := server.StartAdmin(os.Getenv("ADMIN_ADDR"), checker, logger)
	defer stopAdmin(context.Background())

	strategy := bidding.Pessimistic
	if os.Getenv("BID_STRATEGY") == "optimistic" {
		strategy = bidding.Optimistic
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// WatchAuction needs the Redis fan-out that the API's outbox worker
	// publishes to. Without Redis the stream is disabled.
	var watcher grpcsvc.Watcher
	if url := os.Getenv("REDIS_URL"); url != "" {
		rdb, err := redisx.NewClient(ctx, url)
		if err != nil {
			logger.Warn("redis unavailable; WatchAuction disabled", "error", err)
		} else {
			defer rdb.Close()
			hub := realtime.NewHub(10000)
			prefix := os.Getenv("REDIS_PREFIX")
			if prefix == "" {
				prefix = "ae:"
			}
			go realtime.NewRedisFanout(rdb, prefix, hub, logger).Run(ctx)
			watcher = hub
		}
	}

	srv := grpc.NewServer(grpcsvc.ServerOptions(logger, token)...)
	pb.RegisterBiddingServiceServer(srv, grpcsvc.NewServer(bidding.NewService(db, strategy), watcher))

	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(srv, healthSrv)
	healthSrv.SetServingStatus("auctionengine.bidding.v1.BiddingService", healthpb.HealthCheckResponse_SERVING)
	reflection.Register(srv)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}

	errs := make(chan error, 1)
	go func() { errs <- srv.Serve(lis) }()
	logger.Info("bidding service listening", "port", port, "strategy", strategy)

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}

	// Tell load balancers to stop sending traffic, then drain: GracefulStop
	// waits for in-flight RPCs (streams included) to finish. If that takes
	// too long, force it.
	healthSrv.Shutdown()
	done := make(chan struct{})
	go func() { srv.GracefulStop(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		logger.Warn("graceful stop timed out; forcing")
		srv.Stop()
	}
	logger.Info("bidding service stopped")
	return nil
}

// flush exports buffered spans before the process exits.
func flush(shutdown func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = shutdown(ctx)
}
