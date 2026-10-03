package middleware

import (
	"net/http"

	"github.com/Ayush1388/auctionEngine/internal/httpx"
	"github.com/Ayush1388/auctionEngine/internal/metrics"
)

// LimitInFlight is load shedding (v1.0): at most max requests are served at
// once; the rest are rejected immediately with 503 and Retry-After.
//
// # Why reject work on purpose?
//
// Past its capacity, a server that accepts everything gets slower for
// EVERYONE: requests queue, latency climbs past client timeouts, clients
// retry (adding load), and goodput (requests answered in time) drops to
// zero even though the CPU is at 100%. This is congestion collapse.
// Rejecting the excess at once, which takes microseconds, keeps latency
// normal for the requests that are accepted. The rejected client retries
// after a second, possibly on a less busy instance.
//
// This is also a bulkhead: one slow dependency can tie up at most max
// goroutines, not an unbounded number.
//
// Exempt: health probes (an overloaded instance is still alive, and a
// probe timing out would get it restarted, making the overload worse) and
// WebSocket upgrades (long-lived; capped separately by WS_MAX_CONNECTIONS).
//
// max <= 0 disables the limit.
func LimitInFlight(max int, exempt ...string) func(http.Handler) http.Handler {
	if max <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	skip := make(map[string]bool, len(exempt))
	for _, p := range exempt {
		skip[p] = true
	}
	slots := make(chan struct{}, max) // a counting semaphore

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skip[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
				next.ServeHTTP(w, r)
			default:
				metrics.Shed.Inc()
				w.Header().Set("Retry-After", "1")
				httpx.Error(w, http.StatusServiceUnavailable, "server is overloaded, please retry")
			}
		})
	}
}
