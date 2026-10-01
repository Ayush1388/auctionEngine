// Package metrics defines the service's Prometheus metrics (v1.0).
//
// # Logs, metrics and traces: the three pillars of observability
//
//   - Logs (slog, JSON) answer "what happened in THIS request?"
//   - Metrics (this package) answer "how is the system doing overall?":
//     cheap counters and histograms, aggregated over time, that dashboards
//     and alerts are built on. Prometheus scrapes GET /metrics every few
//     seconds and stores time series.
//   - Traces (package telemetry) answer "where did the time go in this one
//     request, across services?"
//
// # The RED method
//
// For every endpoint we track Rate (requests/s), Errors (5xx/s) and
// Duration (latency histogram). Histograms, not averages: an average hides
// the slow tail, while p99 latency (computed from histogram buckets with
// histogram_quantile) shows what the unluckiest 1% of users experience.
//
// # Label cardinality
//
// Every distinct label combination is a separate time series held in
// memory. Labels must therefore be bounded: the route PATTERN
// ("/v1/auctions/{id}"), never the raw path ("/v1/auctions/9f1c…"), which
// would create one series per auction and eventually take Prometheus down.
package metrics

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/Ayush1388/auctionEngine/internal/httpx"
)

// Registry holds every metric. A private registry (rather than the global
// default) keeps tests independent and makes the exposed set explicit.
var Registry = prometheus.NewRegistry()

var factory = prometheusFactory{Registry}

func init() {
	Registry.MustRegister(
		collectors.NewGoCollector(),                                       // goroutines, GC, memory
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}), // CPU, RSS, open fds
	)
}

var (
	HTTPRequests = factory.counterVec("http_requests_total",
		"HTTP requests by method, route pattern and status code.", "method", "route", "code")

	HTTPDuration = factory.histogramVec("http_request_duration_seconds",
		"HTTP request latency by method and route pattern.",
		[]float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}, "method", "route")

	HTTPInFlight = factory.gauge("http_requests_in_flight",
		"Requests currently being served.")

	Bids = factory.counterVec("bids_total",
		"Bid attempts by outcome (accepted, replayed, too_low, insufficient_funds, not_active, ended, own_auction, contention, not_found, invalid, error).", "result")

	BidDuration = factory.histogramVec("bid_duration_seconds",
		"Time to place a bid, including waiting for the auction's row lock.",
		[]float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5}, "strategy")

	OutboxEvents = factory.counterVec("outbox_events_total",
		"Outbox events handled by type and result (processed, retried, dead_lettered).", "type", "result")

	BidQueue = factory.counterVec("bid_queue_commands_total",
		"Async bid commands handled by the Kafka bid worker, by outcome.", "status")

	KafkaLag = factory.gaugeVec("kafka_consumer_lag",
		"Records waiting to be consumed, per consumer group and partition.", "group", "topic", "partition")

	SearchRequests = factory.counterVec("search_requests_total",
		"Search requests by the backend that answered (elasticsearch, postgres) and whether the primary failed over.", "backend", "fallback")

	RateLimited = factory.counterVec("rate_limited_total",
		"Requests rejected with 429, by rule.", "rule")

	GRPCRequests = factory.counterVec("grpc_server_requests_total",
		"gRPC calls handled, by method and status code.", "method", "code")

	GRPCDuration = factory.histogramVec("grpc_server_duration_seconds",
		"gRPC call latency by method.",
		[]float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5}, "method")

	BreakerState = factory.gaugeVec("circuit_breaker_state",
		"Circuit breaker state per dependency: 0 closed, 1 half-open, 2 open.", "name")
)

// Handler serves the registry in the Prometheus text format.
func Handler() http.Handler {
	return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{Registry: Registry})
}

// Middleware records RED metrics for every request.
//
// The route label is the pattern the mux matched (see httpx.Tagged). It is
// read once the inner handler has returned. Unmatched requests (404s for
// random paths a scanner tries) are labelled "unmatched", so they can't
// create new series.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		HTTPInFlight.Inc()
		defer HTTPInFlight.Dec()

		r = r.WithContext(httpx.WithRoute(r.Context()))
		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rec, r)

		route := httpx.Route(r.Context())
		if route == "" {
			route = "unmatched"
		}
		HTTPRequests.WithLabelValues(r.Method, route, strconv.Itoa(rec.code)).Inc()
		HTTPDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.code = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach Flush/Hijack on the real writer
// (WebSocket upgrades go through this middleware too).
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Hijack passes WebSocket upgrades through to the real connection. The
// WebSocket library type-asserts http.Hijacker, so without this method the
// upgrade would fail with 501 once metrics wrap the handler.
func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijacking not supported")
	}
	s.code = http.StatusSwitchingProtocols
	return h.Hijack()
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// prometheusFactory registers metrics with the "auction_" namespace.
type prometheusFactory struct{ r *prometheus.Registry }

const namespace = "auction"

func (f prometheusFactory) counterVec(name, help string, labels ...string) *prometheus.CounterVec {
	c := prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: namespace, Name: name, Help: help}, labels)
	f.r.MustRegister(c)
	return c
}

func (f prometheusFactory) gauge(name, help string) prometheus.Gauge {
	g := prometheus.NewGauge(prometheus.GaugeOpts{Namespace: namespace, Name: name, Help: help})
	f.r.MustRegister(g)
	return g
}

func (f prometheusFactory) gaugeVec(name, help string, labels ...string) *prometheus.GaugeVec {
	g := prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: namespace, Name: name, Help: help}, labels)
	f.r.MustRegister(g)
	return g
}

func (f prometheusFactory) histogramVec(name, help string, buckets []float64, labels ...string) *prometheus.HistogramVec {
	h := prometheus.NewHistogramVec(prometheus.HistogramOpts{Namespace: namespace, Name: name, Help: help, Buckets: buckets}, labels)
	f.r.MustRegister(h)
	return h
}

// GaugeFunc registers a gauge whose value is computed at scrape time,
// e.g. the number of open WebSocket connections or the outbox backlog.
// Registering the same name twice keeps the first one (tests build several
// servers in one process).
func GaugeFunc(name, help string, fn func() float64) {
	register(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Namespace: namespace, Name: name, Help: help}, fn))
}

// CounterFunc registers a counter read from an existing atomic counter.
func CounterFunc(name, help string, fn func() float64) {
	register(prometheus.NewCounterFunc(prometheus.CounterOpts{Namespace: namespace, Name: name, Help: help}, fn))
}

func register(c prometheus.Collector) {
	if err := Registry.Register(c); err != nil {
		var already prometheus.AlreadyRegisteredError
		if !errors.As(err, &already) {
			panic(err)
		}
	}
}

// RegisterPool exports the PostgreSQL connection pool's state. The two
// numbers to watch:
//
//   - acquired close to max: every connection is busy; requests start
//     queueing for a connection (and latency climbs) before Postgres
//     itself is even slow.
//   - empty_acquire / acquire_duration rising: requests are WAITING for a
//     connection. The fix is fewer/faster queries or a bigger pool, not
//     more API instances (which would only add more waiters).
func RegisterPool(pool *pgxpool.Pool) {
	stat := func(f func(*pgxpool.Stat) float64) func() float64 {
		return func() float64 { return f(pool.Stat()) }
	}
	GaugeFunc("db_pool_max_connections", "Maximum size of the connection pool.",
		stat(func(s *pgxpool.Stat) float64 { return float64(s.MaxConns()) }))
	GaugeFunc("db_pool_acquired_connections", "Connections currently in use.",
		stat(func(s *pgxpool.Stat) float64 { return float64(s.AcquiredConns()) }))
	GaugeFunc("db_pool_idle_connections", "Open connections waiting to be used.",
		stat(func(s *pgxpool.Stat) float64 { return float64(s.IdleConns()) }))
	CounterFunc("db_pool_empty_acquire_total", "Acquires that had to wait because no connection was free.",
		stat(func(s *pgxpool.Stat) float64 { return float64(s.EmptyAcquireCount()) }))
	CounterFunc("db_pool_acquire_duration_seconds_total", "Total time spent waiting to acquire a connection.",
		stat(func(s *pgxpool.Stat) float64 { return s.AcquireDuration().Seconds() }))
}
