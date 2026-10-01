// Package health serves the liveness and readiness probes (v1.0).
//
// Kubernetes (and most load balancers) ask two different questions:
//
//	GET /livez   "Is this process alive?" If not, RESTART it.
//	             Answers 200 as long as the process can serve HTTP at all.
//	             It deliberately does NOT check the database: if Postgres is
//	             down, restarting every API pod won't fix Postgres. It would
//	             only add a restart storm on top of the outage.
//
//	GET /readyz  "Should traffic be sent here right now?" If not, take the
//	             pod OUT of the load balancer, but leave it running.
//	             Checks critical dependencies (PostgreSQL) and returns 503
//	             while the instance is draining during shutdown.
//
// # Draining
//
// On SIGTERM the instance first calls Drain: /readyz turns 503 and the load
// balancer stops sending NEW requests. main then waits a few seconds (the
// time the balancer needs to notice) before http.Server.Shutdown. Without
// that wait, requests routed in the gap between "pod is terminating" and
// "balancer noticed" hit a closed port, and users see connection errors on
// every deploy.
package health

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Ayush1388/auctionEngine/internal/httpx"
	"github.com/Ayush1388/auctionEngine/internal/logctx"
)

// CheckTimeout bounds every readiness check, so a hung dependency makes the
// probe fail instead of making it hang (probes have their own timeout, and
// a hung probe looks like a dead process).
const CheckTimeout = 2 * time.Second

type check struct {
	name     string
	critical bool
	fn       func(context.Context) error
}

// Checker holds the readiness checks and the draining flag.
type Checker struct {
	draining atomic.Bool
	checks   []check
}

func New() *Checker { return &Checker{} }

// Add registers a check. A failing critical check makes /readyz return 503.
// A non-critical one (Redis, Elasticsearch, Kafka: features that degrade
// gracefully) is reported in the body but keeps the instance in rotation:
// taking every instance out because the cache is down would turn a
// degraded service into a full outage.
//
// Add must be called before the server starts.
func (c *Checker) Add(name string, critical bool, fn func(context.Context) error) {
	c.checks = append(c.checks, check{name: name, critical: critical, fn: fn})
}

// Drain marks the instance as shutting down; /readyz answers 503 from now on.
func (c *Checker) Drain() { c.draining.Store(true) }

// Draining reports whether Drain was called.
func (c *Checker) Draining() bool { return c.draining.Load() }

// Live answers the liveness probe.
func (c *Checker) Live(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ready runs every check concurrently and answers the readiness probe.
//
// The body only says "ok" or "fail" per dependency; the error itself is
// logged, not returned, because this endpoint is reachable without
// authentication and error messages can reveal hostnames.
func (c *Checker) Ready(w http.ResponseWriter, r *http.Request) {
	if c.Draining() {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "draining"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), CheckTimeout)
	defer cancel()

	results := make([]error, len(c.checks))
	var wg sync.WaitGroup
	for i, ch := range c.checks {
		wg.Go(func() { results[i] = ch.fn(ctx) })
	}
	wg.Wait()

	status := http.StatusOK
	report := make(map[string]string, len(c.checks))
	for i, ch := range c.checks {
		if results[i] == nil {
			report[ch.name] = "ok"
			continue
		}
		report[ch.name] = "fail"
		logctx.From(r.Context()).Warn("readiness check failed",
			"check", ch.name, "critical", ch.critical, "error", results[i])
		if ch.critical {
			status = http.StatusServiceUnavailable
		}
	}

	overall := "ok"
	if status != http.StatusOK {
		overall = "unavailable"
	}
	httpx.WriteJSON(w, status, map[string]any{"status": overall, "checks": report})
}
