// Package chaos is a fault-injection switchboard for demos and drills.
//
// It lets an operator break one dependency at a time (Redis, Kafka,
// Elasticsearch, the bidding service, a slow PostgreSQL) and watch the
// system degrade the way it was designed to: bids still land, a breaker
// opens, search falls back to PostgreSQL, readiness reports the failure.
//
// It is OFF unless the API starts with CHAOS_ENABLED=true. When off, every
// hook is a single atomic load and nothing can be switched on, so the
// package is safe to leave wired into production code paths. Faults are
// held in memory only: a restart clears them.
package chaos

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Fault names one thing that can be broken.
type Fault string

const (
	Redis         Fault = "redis"
	Kafka         Fault = "kafka"
	Elasticsearch Fault = "elasticsearch"
	Bidding       Fault = "bidding"
	PostgresSlow  Fault = "postgres_slow"
)

// ErrInjected is the error every failing fault returns.
var ErrInjected = errors.New("chaos: injected failure")

// ErrDisabled means a fault was switched while chaos is not enabled.
var ErrDisabled = errors.New("chaos is disabled: start the API with CHAOS_ENABLED=true")

// Description says what each fault does, for the operator console.
var descriptions = map[Fault]string{
	Redis:         "Every Redis command fails. Cache misses, trending and fan-out fall back; rate limits fail open.",
	Kafka:         "Kafka is unreachable. Synchronous bids are unaffected; queued bids wait in the outbox.",
	Elasticsearch: "Elasticsearch fails. The breaker opens and search is answered from PostgreSQL.",
	Bidding:       "The bidding service is down. Its breaker opens and bids return a fast 503.",
	PostgresSlow:  "Every PostgreSQL statement is delayed. Bids queue on the row lock and latency climbs.",
}

// All lists the faults in display order.
var All = []Fault{Redis, Kafka, Elasticsearch, Bidding, PostgresSlow}

// DefaultSlowQuery is the delay added to each statement by PostgresSlow.
const DefaultSlowQuery = 40 * time.Millisecond

var (
	enabled atomic.Bool
	slowNS  atomic.Int64
	active  = map[Fault]*atomic.Bool{}
	mu      sync.Mutex
)

func init() {
	for _, f := range All {
		active[f] = new(atomic.Bool)
	}
	slowNS.Store(int64(DefaultSlowQuery))
}

// Enable turns the switchboard on (process start-up only).
func Enable() { enabled.Store(true) }

// Enabled reports whether faults can be set.
func Enabled() bool { return enabled.Load() }

// Valid reports whether f is a known fault.
func Valid(f Fault) bool { _, ok := active[f]; return ok }

// Set switches a fault on or off.
func Set(f Fault, on bool) error {
	if !enabled.Load() {
		return ErrDisabled
	}
	a, ok := active[f]
	if !ok {
		return fmt.Errorf("unknown fault %q", f)
	}
	a.Store(on)
	return nil
}

// Reset clears every fault.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	for _, a := range active {
		a.Store(false)
	}
	slowNS.Store(int64(DefaultSlowQuery))
}

// Active reports whether f is currently broken.
func Active(f Fault) bool {
	if !enabled.Load() {
		return false
	}
	a, ok := active[f]
	return ok && a.Load()
}

// Fail returns ErrInjected while f is broken, nil otherwise.
func Fail(f Fault) error {
	if Active(f) {
		return fmt.Errorf("%w (%s)", ErrInjected, f)
	}
	return nil
}

// SetSlowQuery changes the per-statement delay used by PostgresSlow.
func SetSlowQuery(d time.Duration) {
	if d < 0 {
		d = 0
	}
	if d > 2*time.Second {
		d = 2 * time.Second
	}
	slowNS.Store(int64(d))
}

// SlowQuery returns the per-statement delay.
func SlowQuery() time.Duration { return time.Duration(slowNS.Load()) }

// Delay sleeps for the slow-query delay while PostgresSlow is on, or until
// ctx ends.
func Delay(ctx context.Context) {
	if !Active(PostgresSlow) {
		return
	}
	t := time.NewTimer(SlowQuery())
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}

// Status is one fault as reported to the operator console.
type Status struct {
	Name        Fault  `json:"name"`
	Active      bool   `json:"active"`
	Description string `json:"description"`
}

// Snapshot lists every fault and whether it is on.
func Snapshot() []Status {
	out := make([]Status, 0, len(All))
	for _, f := range All {
		out = append(out, Status{Name: f, Active: Active(f), Description: descriptions[f]})
	}
	return out
}
