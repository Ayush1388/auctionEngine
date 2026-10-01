package ratelimit_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Ayush1388/auctionEngine/internal/ratelimit"
	"github.com/Ayush1388/auctionEngine/internal/redistest"
)

func TestRedisLimiterBurstAndRefill(t *testing.T) {
	rdb, prefix := redistest.New(t)
	l := ratelimit.NewRedis(rdb, prefix)
	rule := ratelimit.Rule{Name: "t", Rate: 0.5, Burst: 3}
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		d, err := l.Allow(ctx, rule, "alice")
		if err != nil || !d.Allowed || d.Remaining != 2-i {
			t.Fatalf("request %d: %+v %v", i+1, d, err)
		}
	}
	d, _ := l.Allow(ctx, rule, "alice")
	if d.Allowed || d.RetryAfter <= 0 || d.RetryAfter > 2_100_000_000 {
		t.Fatalf("over the burst: %+v", d)
	}
}

// Three API instances share one Redis. 60 requests arrive at once across
// them for the same client with a burst of 10: exactly 10 may pass. If the
// read-refill-write weren't atomic, several instances would read the same
// token count and let more through.
func TestRedisLimiterIsSharedAndAtomic(t *testing.T) {
	rdb, prefix := redistest.New(t)
	instances := []ratelimit.Limiter{
		ratelimit.NewRedis(rdb, prefix),
		ratelimit.NewRedis(rdb, prefix),
		ratelimit.NewRedis(rdb, prefix),
	}
	rule := ratelimit.Rule{Name: "shared", Rate: 0.001, Burst: 10}

	var allowed atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 60; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			d, err := instances[i%3].Allow(context.Background(), rule, "bot")
			if err != nil {
				t.Error(err)
			}
			if d.Allowed {
				allowed.Add(1)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	if n := allowed.Load(); n != 10 {
		t.Fatalf("%d requests allowed across instances, want exactly 10", n)
	}
}
