package breaker_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/Ayush1388/auctionEngine/internal/breaker"
	"github.com/Ayush1388/auctionEngine/internal/metrics"
)

var errDown = errors.New("connection refused")

type clock struct{ now time.Time }

func (c *clock) Now() time.Time          { return c.now }
func (c *clock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func newBreaker(t *testing.T, name string) (*breaker.Breaker, *clock, *[]string) {
	t.Helper()
	c := &clock{now: time.Unix(1_700_000_000, 0)}
	var transitions []string
	b := breaker.New(name, breaker.Options{
		Threshold: 3, Cooldown: 10 * time.Second, HalfOpenMax: 1, HalfOpenSuccesses: 2,
		Now: c.Now,
		OnStateChange: func(_ string, from, to breaker.State) {
			transitions = append(transitions, from.String()+"→"+to.String())
		},
	})
	return b, c, &transitions
}

func call(b *breaker.Breaker, err error) (called bool, got error) {
	got = b.Do(context.Background(), func(context.Context) error { called = true; return err })
	return called, got
}

func TestFullCycle(t *testing.T) {
	b, clk, transitions := newBreaker(t, "test-cycle")

	// Two failures, then a success: the count resets, still closed.
	call(b, errDown)
	call(b, errDown)
	call(b, nil)
	call(b, errDown)
	call(b, errDown)
	if b.State() != breaker.Closed {
		t.Fatalf("state = %s, want closed (failures weren't consecutive)", b.State())
	}

	// Third consecutive failure trips it.
	call(b, errDown)
	if b.State() != breaker.Open {
		t.Fatalf("state = %s, want open", b.State())
	}
	if got := testutil.ToFloat64(metrics.BreakerState.WithLabelValues("test-cycle")); got != 2 {
		t.Fatalf("gauge = %v, want 2 (open)", got)
	}

	// While open: fail fast, the dependency is NOT called.
	if called, err := call(b, nil); called || !errors.Is(err, breaker.ErrOpen) {
		t.Fatalf("open breaker: called=%v err=%v", called, err)
	}

	// After the cooldown a trial is allowed; a failed trial reopens.
	clk.Advance(10 * time.Second)
	if called, _ := call(b, errDown); !called {
		t.Fatal("trial call not let through after cooldown")
	}
	if b.State() != breaker.Open {
		t.Fatalf("failed trial: state = %s, want open", b.State())
	}

	// Next cooldown: two successful trials close it.
	clk.Advance(10 * time.Second)
	call(b, nil)
	if b.State() != breaker.HalfOpen {
		t.Fatalf("after one good trial: %s, want half-open", b.State())
	}
	call(b, nil)
	if b.State() != breaker.Closed {
		t.Fatalf("after two good trials: %s, want closed", b.State())
	}

	want := []string{"closed→open", "open→half-open", "half-open→open", "open→half-open", "half-open→closed"}
	if len(*transitions) != len(want) {
		t.Fatalf("transitions = %v, want %v", *transitions, want)
	}
	for i := range want {
		if (*transitions)[i] != want[i] {
			t.Fatalf("transitions = %v, want %v", *transitions, want)
		}
	}
}

// Half-open lets only a trickle through: while one trial is in flight, the
// rest fail fast instead of all hitting a dependency that may still be down.
func TestHalfOpenLimitsConcurrentTrials(t *testing.T) {
	b, clk, _ := newBreaker(t, "test-halfopen")
	for range 3 {
		call(b, errDown)
	}
	clk.Advance(10 * time.Second)

	release := make(chan struct{})
	started := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		_ = b.Do(context.Background(), func(context.Context) error {
			close(started)
			<-release
			return nil
		})
	})
	<-started

	var rejected atomic.Int32
	for range 10 {
		if _, err := call(b, nil); errors.Is(err, breaker.ErrOpen) {
			rejected.Add(1)
		}
	}
	close(release)
	wg.Wait()
	if rejected.Load() != 10 {
		t.Fatalf("%d of 10 concurrent calls rejected during the trial, want all 10", rejected.Load())
	}
}

// Errors that don't mean "the dependency is down" never trip the breaker.
func TestOnlyDependencyFailuresCount(t *testing.T) {
	errTooLow := errors.New("bid too low")
	b := breaker.New("test-classify", breaker.Options{
		Threshold: 2,
		IsFailure: func(err error) bool { return err != nil && !errors.Is(err, errTooLow) },
	})
	for range 10 {
		call(b, errTooLow)
	}
	if b.State() != breaker.Closed {
		t.Fatalf("business errors tripped the breaker: %s", b.State())
	}

	// The default classifier also ignores the caller's own cancellation.
	d := breaker.New("test-default", breaker.Options{Threshold: 1})
	_ = d.Do(context.Background(), func(context.Context) error { return context.Canceled })
	if d.State() != breaker.Closed {
		t.Fatal("context.Canceled tripped the default breaker")
	}
	_ = d.Do(context.Background(), func(context.Context) error { return context.DeadlineExceeded })
	if d.State() != breaker.Open {
		t.Fatal("a timeout should count as a failure")
	}
}

func TestConcurrentUseIsRaceFree(t *testing.T) {
	b := breaker.New("test-race", breaker.Options{Threshold: 5, Cooldown: time.Millisecond})
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			for j := range 200 {
				var err error
				if (i+j)%3 == 0 {
					err = errDown
				}
				_ = b.Do(context.Background(), func(context.Context) error { return err })
				_ = b.State()
			}
		})
	}
	wg.Wait()
}

// The breaker sits on hot paths (every search, every gRPC bid), so its own
// cost must be negligible next to a network call (~0.1–1 ms).
func BenchmarkDoClosed(b *testing.B) {
	br := breaker.New("bench", breaker.Options{})
	ctx := context.Background()
	ok := func(context.Context) error { return nil }
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = br.Do(ctx, ok)
		}
	})
}
