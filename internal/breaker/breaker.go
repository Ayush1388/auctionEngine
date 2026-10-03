// Package breaker implements the circuit breaker pattern (v1.0).
//
// # The problem
//
// When a dependency (Elasticsearch, the bidding service, the SMTP server)
// goes down, every call to it waits for its timeout before failing. With
// 2-second timeouts and 500 requests/s, a thousand requests are stuck
// waiting at any moment. Goroutines, connections and memory pile up, and a
// failure in one optional feature turns into the whole API being slow or
// down. That is a cascading failure.
//
// Retrying makes it worse: a struggling service gets MORE traffic exactly
// when it needs less, which keeps it from recovering.
//
// # The fix: stop calling it for a while
//
// A breaker sits in front of the dependency and counts failures. It works
// like the fuse in a house: after too many failures in a row it "trips"
// (opens), and calls fail immediately with ErrOpen, without touching the
// dependency. Callers fall back (search uses PostgreSQL) or answer 503 at
// once. After a cool-down it lets a few trial calls through to check
// whether the dependency has recovered.
//
//	         failures ≥ Threshold
//	CLOSED ─────────────────────────► OPEN ◄────────┐
//	  ▲   (calls pass through)        │  (fail fast) │ a trial call fails
//	  │                               │ after Cooldown
//	  │   HalfOpenSuccesses trials    ▼              │
//	  └──────────── succeed ──────── HALF-OPEN ──────┘
//	                                 (at most HalfOpenMax trials at once)
//
// # What counts as a failure
//
// Only errors that say the DEPENDENCY is unhealthy: timeouts, connection
// errors, 5xx. A bid that is too low, or a caller that cancelled its own
// request, says nothing about the dependency's health. Counting those would
// trip the breaker on a healthy service. Options.IsFailure decides.
package breaker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Ayush1388/auctionEngine/internal/metrics"
)

// ErrOpen is returned without calling the dependency while the breaker is
// open (or half-open with all trial slots taken).
var ErrOpen = errors.New("circuit breaker is open")

// State is the breaker's state. The numeric values are what the
// auction_circuit_breaker_state gauge reports.
type State int

const (
	Closed   State = 0
	HalfOpen State = 1
	Open     State = 2
)

func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case HalfOpen:
		return "half-open"
	default:
		return "open"
	}
}

// Options tune a breaker. Zero values get the defaults in brackets.
type Options struct {
	// Threshold is the number of consecutive failures that opens the
	// breaker [5].
	Threshold int
	// Cooldown is how long the breaker stays open before allowing trial
	// calls [10s].
	Cooldown time.Duration
	// HalfOpenMax is the number of trial calls allowed at once while
	// half-open [1]. Only a trickle is let through: if the dependency is
	// still down, only those few calls pay the timeout.
	HalfOpenMax int
	// HalfOpenSuccesses is the number of successful trials that close the
	// breaker again [2].
	HalfOpenSuccesses int
	// IsFailure reports whether err means the dependency is unhealthy
	// [any non-nil error except context.Canceled].
	IsFailure func(error) bool
	// OnStateChange is called after every transition (logging).
	OnStateChange func(name string, from, to State)
	// Now is the clock; tests replace it.
	Now func() time.Time
}

// Breaker guards one dependency. It is safe for concurrent use.
type Breaker struct {
	name string
	opts Options

	mu        sync.Mutex
	state     State
	failures  int       // consecutive failures while closed
	successes int       // successful trials while half-open
	inFlight  int       // trial calls running while half-open
	openedAt  time.Time // when the breaker last opened
}

// New returns a closed breaker. name labels its metric
// (auction_circuit_breaker_state{name=…}), so keep it a fixed string.
func New(name string, opts Options) *Breaker {
	if opts.Threshold <= 0 {
		opts.Threshold = 5
	}
	if opts.Cooldown <= 0 {
		opts.Cooldown = 10 * time.Second
	}
	if opts.HalfOpenMax <= 0 {
		opts.HalfOpenMax = 1
	}
	if opts.HalfOpenSuccesses <= 0 {
		opts.HalfOpenSuccesses = 2
	}
	if opts.IsFailure == nil {
		opts.IsFailure = DefaultIsFailure
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	b := &Breaker{name: name, opts: opts}
	metrics.BreakerState.WithLabelValues(name).Set(float64(Closed))
	return b
}

// DefaultIsFailure counts every error except the caller cancelling.
func DefaultIsFailure(err error) bool {
	return err != nil && !errors.Is(err, context.Canceled)
}

// State returns the current state. An open breaker whose cooldown has
// passed reports HalfOpen, since the next call would be a trial.
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == Open && b.opts.Now().Sub(b.openedAt) >= b.opts.Cooldown {
		return HalfOpen
	}
	return b.state
}

// Do runs fn if the breaker allows it and records the result. While open
// it returns ErrOpen without calling fn.
func (b *Breaker) Do(ctx context.Context, fn func(context.Context) error) error {
	trial, err := b.allow()
	if err != nil {
		return err
	}
	err = fn(ctx)
	b.record(trial, err)
	return err
}

// allow decides whether a call may proceed, and whether it is a half-open
// trial (whose result decides the next state).
func (b *Breaker) allow() (trial bool, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case Closed:
		return false, nil
	case Open:
		if b.opts.Now().Sub(b.openedAt) < b.opts.Cooldown {
			return false, ErrOpen
		}
		b.transition(HalfOpen)
		fallthrough
	default: // HalfOpen
		if b.inFlight >= b.opts.HalfOpenMax {
			return false, ErrOpen
		}
		b.inFlight++
		return true, nil
	}
}

func (b *Breaker) record(trial bool, err error) {
	failed := b.opts.IsFailure(err)

	b.mu.Lock()
	defer b.mu.Unlock()

	if trial {
		b.inFlight--
		// The breaker may have moved on while this trial ran (another
		// trial failed and reopened it). Only count trials of the
		// current half-open period.
		if b.state != HalfOpen {
			return
		}
		if failed {
			b.open()
			return
		}
		b.successes++
		if b.successes >= b.opts.HalfOpenSuccesses {
			b.transition(Closed)
		}
		return
	}

	if b.state != Closed {
		return
	}
	if !failed {
		// Consecutive failures: one success resets the count, so a few
		// scattered errors never trip a healthy dependency.
		b.failures = 0
		return
	}
	b.failures++
	if b.failures >= b.opts.Threshold {
		b.open()
	}
}

func (b *Breaker) open() {
	b.openedAt = b.opts.Now()
	b.transition(Open)
}

// transition must be called with mu held.
func (b *Breaker) transition(to State) {
	from := b.state
	b.state = to
	b.failures, b.successes = 0, 0
	metrics.BreakerState.WithLabelValues(b.name).Set(float64(to))
	if b.opts.OnStateChange != nil && from != to {
		b.opts.OnStateChange(b.name, from, to)
	}
}
