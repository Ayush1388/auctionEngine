// Package ratelimit limits how often a client may call the API, using the
// token bucket algorithm.
//
// # Token bucket
//
// Picture a bucket that holds at most Burst tokens and is refilled at Rate
// tokens per second. Every request takes one token. If the bucket is empty
// the request is rejected with 429 Too Many Requests and a Retry-After
// header saying when the next token arrives.
//
//	Rate = 1/s, Burst = 5
//	t=0s   bucket 5 → five quick requests succeed → bucket 0
//	t=0.1s request → rejected, Retry-After ≈ 0.9s
//	t=3s   bucket refilled to 3
//
// Burst allows short spikes (a page firing several requests at once) while
// Rate caps the long-run average. The state per client is two numbers
// (tokens, last refill time), so it's cheap: no list of timestamps like a
// sliding-window log.
//
// # Why it matters here
//
//   - brute force: a login endpoint without a limit lets an attacker try
//     thousands of passwords per second against one account;
//   - abuse and cost: a script hammering POST /bids or /register;
//   - fairness: one client can't use up the database for everyone else.
//
// The Limiter interface has an in-memory implementation (this file), which
// is per-process: with 3 API instances each one allows the full rate. v0.5
// adds a Redis implementation so all instances share one bucket per client.
package ratelimit

import (
	"context"
	"math"
	"sync"
	"time"
)

// Rule is one limit: Rate tokens per second, up to Burst saved up.
type Rule struct {
	Name  string
	Rate  float64
	Burst int
}

// PerMinute is a readable way to write slow rules: PerMinute("login", 5, 5).
func PerMinute(name string, perMinute float64, burst int) Rule {
	return Rule{Name: name, Rate: perMinute / 60, Burst: burst}
}

// Decision is the outcome of one Allow call.
type Decision struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration
}

// Limiter takes one token for key under rule.
type Limiter interface {
	Allow(ctx context.Context, rule Rule, key string) (Decision, error)
}

type bucket struct {
	tokens float64
	last   time.Time
}

// Memory is an in-process Limiter. Safe for concurrent use.
type Memory struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	now     func() time.Time
}

func NewMemory() *Memory {
	return &Memory{buckets: map[string]*bucket{}, now: time.Now}
}

// SetClock replaces the clock; tests use it.
func (m *Memory) SetClock(now func() time.Time) { m.now = now }

func (m *Memory) Allow(_ context.Context, rule Rule, key string) (Decision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	k := rule.Name + ":" + key

	b, ok := m.buckets[k]
	if !ok {
		// A new client starts with a full bucket.
		b = &bucket{tokens: float64(rule.Burst), last: now}
		m.buckets[k] = b
	}

	// Refill for the time elapsed since the last request, capped at Burst.
	elapsed := now.Sub(b.last).Seconds()
	b.tokens = math.Min(float64(rule.Burst), b.tokens+elapsed*rule.Rate)
	b.last = now

	return take(b, rule), nil
}

func take(b *bucket, rule Rule) Decision {
	if b.tokens >= 1 {
		b.tokens--
		return Decision{Allowed: true, Limit: rule.Burst, Remaining: int(b.tokens)}
	}

	// Time until the bucket holds one whole token again.
	wait := time.Duration((1 - b.tokens) / rule.Rate * float64(time.Second))
	return Decision{Allowed: false, Limit: rule.Burst, Remaining: 0, RetryAfter: wait}
}

// Cleanup forgets buckets that have been idle long enough to be full again.
// Without it the map grows by one entry per distinct IP forever, a memory
// leak an attacker could trigger on purpose by spoofing many addresses.
func (m *Memory) Cleanup(maxIdle time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := m.now().Add(-maxIdle)
	removed := 0
	for k, b := range m.buckets {
		if b.last.Before(cutoff) {
			delete(m.buckets, k)
			removed++
		}
	}
	return removed
}

// Len reports how many buckets are tracked (for tests and metrics).
func (m *Memory) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.buckets)
}

// RunJanitor calls Cleanup every interval until ctx is cancelled.
func (m *Memory) RunJanitor(ctx context.Context, interval, maxIdle time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.Cleanup(maxIdle)
		}
	}
}
