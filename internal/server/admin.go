package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/Ayush1388/auctionEngine/internal/health"
	"github.com/Ayush1388/auctionEngine/internal/metrics"
)

// NewAdmin builds the internal admin listener (v1.0): Prometheus metrics
// and the pprof profiler.
//
// It is a SEPARATE server on its own port (ADMIN_ADDR, e.g. :9090) that is
// never exposed through the load balancer. pprof in particular must not be
// public: a CPU profile reveals code paths, a heap dump can contain user
// data, and a long profile request costs CPU, which is a cheap DoS.
//
// Profiling a live process:
//
//	go tool pprof http://localhost:9090/debug/pprof/profile?seconds=30   # CPU
//	go tool pprof http://localhost:9090/debug/pprof/heap                 # memory
//	curl http://localhost:9090/debug/pprof/goroutine?debug=2             # goroutine dump
//
// pprof's handlers are registered explicitly on a private mux. Importing
// net/http/pprof also registers them on http.DefaultServeMux; this service
// never serves that mux, so that side effect is harmless.
//
// checker, if non-nil, is also served at /livez and /readyz. Processes with
// no public HTTP port (the bid worker) are probed by Kubernetes here.
func NewAdmin(addr string, checker *health.Checker) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metrics.Handler())
	if checker != nil {
		mux.HandleFunc("GET /livez", checker.Live)
		mux.HandleFunc("GET /readyz", checker.Ready)
	}
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		// No WriteTimeout: a 30-second CPU profile must be able to finish.
	}
}

// StartAdmin runs the admin listener in the background if addr is set. The
// returned function shuts it down; call it last, so metrics stay scrapeable
// while the rest of the process drains.
func StartAdmin(addr string, checker *health.Checker, logger *slog.Logger) func(context.Context) {
	if addr == "" {
		return func(context.Context) {}
	}
	srv := NewAdmin(addr, checker)
	go func() {
		if err := ignoreClosed(srv.ListenAndServe()); err != nil {
			logger.Error("admin listener failed", "addr", addr, "error", err)
		}
	}()
	logger.Info("admin listener started (metrics, pprof)", "addr", addr)
	return func(ctx context.Context) { _ = srv.Shutdown(ctx) }
}
