package ratelimit

import (
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"github.com/Ayush1388/auctionEngine/internal/httpx"
	"github.com/Ayush1388/auctionEngine/internal/metrics"
)

// KeyFunc picks what a request is limited by: its IP, its user, ...
// Returning "" skips limiting for that request.
type KeyFunc func(r *http.Request) string

// Middleware rejects requests over rule with 429 and sets the standard
// headers so well-behaved clients can slow down on their own:
//
//	RateLimit-Limit: 20
//	RateLimit-Remaining: 0
//	Retry-After: 2
//
// If the limiter itself fails (e.g. Redis is down in v0.5) the request is
// allowed: failing open keeps the API up; the alternative, failing closed,
// would turn a Redis outage into a full outage.
func Middleware(l Limiter, rule Rule, key KeyFunc, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		k := key(r)
		if k == "" {
			next.ServeHTTP(w, r)
			return
		}

		d, err := l.Allow(r.Context(), rule, k)
		if err != nil {
			slog.WarnContext(r.Context(), "rate limiter unavailable, allowing request", "rule", rule.Name, "error", err)
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("RateLimit-Limit", strconv.Itoa(d.Limit))
		w.Header().Set("RateLimit-Remaining", strconv.Itoa(d.Remaining))

		if !d.Allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(d.RetryAfter.Seconds()))))
			metrics.RateLimited.WithLabelValues(rule.Name).Inc()
			httpx.Error(w, http.StatusTooManyRequests, "too many requests, slow down")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ClientIP works out the caller's IP address.
//
// r.RemoteAddr is the TCP peer. Behind a load balancer or reverse proxy that
// peer is the proxy, and the real client is in X-Forwarded-For, which the
// proxy appends to. But anyone can send that header, so it is only trusted
// when the TCP peer is one of our own proxies. Otherwise a client could
// claim a new fake IP on every request and never be rate limited.
//
// With a chain "client, proxy1, proxy2" the rightmost address that isn't a
// trusted proxy is the real client; everything to its left could be forged.
type ClientIP struct {
	trusted []netip.Prefix
}

// NewClientIP takes the trusted proxy ranges as CIDRs, e.g. "10.0.0.0/8".
func NewClientIP(trustedCIDRs []string) (*ClientIP, error) {
	c := &ClientIP{}
	for _, s := range trustedCIDRs {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, err
		}
		c.trusted = append(c.trusted, p)
	}
	return c, nil
}

func (c *ClientIP) isTrusted(a netip.Addr) bool {
	for _, p := range c.trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// Of returns the client IP of r.
func (c *ClientIP) Of(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	peer = peer.Unmap()

	if !c.isTrusted(peer) {
		return peer.String()
	}

	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		a = a.Unmap()
		if !c.isTrusted(a) {
			return a.String()
		}
	}
	return peer.String()
}

// ByIP limits per client IP.
func (c *ClientIP) ByIP() KeyFunc {
	return func(r *http.Request) string { return "ip:" + c.Of(r) }
}
