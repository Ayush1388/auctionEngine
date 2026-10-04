package ratelimit_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/Ayush1388/auctionEngine/internal/ratelimit"
)

// BenchmarkMemoryAllow: the in-process token bucket, which runs on every
// request when Redis is off. 10k distinct keys to include map growth.
func BenchmarkMemoryAllow(b *testing.B) {
	m := ratelimit.NewMemory()
	rule := ratelimit.Rule{Name: "bench", Rate: 1e6, Burst: 1e6}
	keys := make([]string, 10_000)
	for i := range keys {
		keys[i] = "ip:" + strconv.Itoa(i)
	}
	ctx := context.Background()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_, _ = m.Allow(ctx, rule, keys[i%len(keys)])
			i++
		}
	})
}
