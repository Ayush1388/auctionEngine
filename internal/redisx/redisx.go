// Package redisx creates the Redis client and holds conventions shared by
// every Redis user in the project.
//
// # What Redis is used for, and what it is NOT used for
//
// Redis is an in-memory data store: very fast (sub-millisecond), but data
// can be lost on a restart unless persistence is configured, and it isn't
// transactional with PostgreSQL. So it only holds things that are cheap to
// lose or rebuild:
//
//   - a cache of auction reads (package auctioncache)
//   - trending-auction scores (package trending)
//   - rate-limit buckets shared by every API instance (ratelimit.Redis)
//
// PostgreSQL stays the source of truth. Nothing that decides money or bids
// ever reads from Redis, and if Redis goes down every feature above degrades
// (cache misses, Postgres-based trending, rate limits fail open) instead of
// the API going down.
//
// Every key starts with a prefix ("ae:" in production) so several apps or
// test runs can share one Redis without colliding.
package redisx

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewClient connects to url (redis://[:password@]host:port/db) and pings.
//
// The timeouts are deliberately short: a cache that takes 3 seconds to
// answer is worse than no cache. When Redis is slow, calls fail fast and
// callers fall back to PostgreSQL.
func NewClient(ctx context.Context, url string) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse REDIS_URL: %w", err)
	}

	opts.DialTimeout = 2 * time.Second
	opts.ReadTimeout = 250 * time.Millisecond
	opts.WriteTimeout = 250 * time.Millisecond
	opts.PoolSize = 20

	client := redis.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return client, nil
}
