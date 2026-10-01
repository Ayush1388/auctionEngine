// Package redistest gives tests a Redis client and a unique key prefix.
// Tests are skipped unless TEST_REDIS_URL is set.
package redistest

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/Ayush1388/auctionEngine/internal/redisx"
)

// New returns a client and a prefix like "test:1f2e…:". Keys under the
// prefix are deleted when the test ends.
func New(t testing.TB) (*redis.Client, string) {
	t.Helper()

	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set; skipping Redis test")
	}

	client, err := redisx.NewClient(context.Background(), url)
	if err != nil {
		t.Fatalf("redis: %v", err)
	}

	prefix := "test:" + strings.ReplaceAll(uuid.NewString(), "-", "") + ":"

	t.Cleanup(func() {
		ctx := context.Background()
		iter := client.Scan(ctx, 0, prefix+"*", 1000).Iterator()
		for iter.Next(ctx) {
			client.Del(ctx, iter.Val())
		}
		client.Close()
	})

	return client, prefix
}
