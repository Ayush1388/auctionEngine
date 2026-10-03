// Package auctioncache caches auction reads in Redis.
//
// # Cache-aside (lazy loading)
//
//	GET /v1/auctions/{id}
//	  └─ Redis GET ae:auction:{id}
//	       ├─ hit  → return it                      (~0.3 ms)
//	       └─ miss → load from PostgreSQL           (~2 ms)
//	                 → Redis SET … EX 30s → return it
//
// The application owns the logic: Redis never talks to PostgreSQL. Only
// data that was actually requested gets cached. Other patterns, for
// comparison: write-through (update the cache on every write, even for data
// nobody reads) and write-back (write to the cache and flush to the database
// later, which risks losing writes).
//
// # Invalidation
//
// "There are only two hard things in computer science: cache invalidation
// and naming things." When an auction changes (a bid, cancellation,
// start/end, settlement) its entry is deleted, not updated. The next read
// reloads it. Deleting is safe under concurrency; racing updates could leave
// an older value last.
//
// Deletion is driven by outbox events (bid.placed, auction.cancelled, …),
// the same events every other consumer uses, so no write path has to
// remember to touch the cache. The price is a short stale window (outbox
// delivery latency), bounded by the TTL anyway. That's fine because nothing
// that decides money or bids ever reads this cache: bidding locks the row in
// PostgreSQL.
//
// # Stampede protection
//
// When a popular key expires, hundreds of concurrent requests miss at once
// and all hit the database: a cache stampede. singleflight collapses the
// concurrent misses in this process into one load; the others wait for its
// result. (Across instances you could add a short Redis lock or refresh
// entries early before they expire; one load per instance is fine at this
// scale.)
package auctioncache

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/outbox"
)

// TTL bounds how stale an entry can be if an invalidation is ever missed.
const TTL = 30 * time.Second

// Loader reads an auction from the source of truth.
type Loader func(ctx context.Context, id uuid.UUID) (auction.Auction, error)

type Cache struct {
	rdb    *redis.Client
	prefix string
	load   Loader
	group  singleflight.Group

	// Counters for observability (exported as metrics in v1.0).
	hits, misses, loads, errs atomic.Int64
}

func New(rdb *redis.Client, prefix string, load Loader) *Cache {
	return &Cache{rdb: rdb, prefix: prefix, load: load}
}

func (c *Cache) key(id uuid.UUID) string { return c.prefix + "auction:" + id.String() }

// Get returns the auction, from Redis if possible.
func (c *Cache) Get(ctx context.Context, id uuid.UUID) (auction.Auction, error) {
	raw, err := c.rdb.Get(ctx, c.key(id)).Bytes()
	switch {
	case err == nil:
		var a auction.Auction
		if json.Unmarshal(raw, &a) == nil {
			c.hits.Add(1)
			return a, nil
		}
		// A corrupt entry is just a miss.
	case errors.Is(err, redis.Nil):
		// Plain miss.
	default:
		// Redis is down or slow: skip it and read the database. The cache
		// is an optimisation, never a dependency.
		c.errs.Add(1)
		slog.WarnContext(ctx, "auction cache unavailable", "error", err)
		return c.load(ctx, id)
	}

	c.misses.Add(1)

	// All concurrent misses for this id share one load.
	v, err, _ := c.group.Do(id.String(), func() (any, error) {
		// Detach from the first caller's context: if that client gives up,
		// the others waiting on this load shouldn't fail with it.
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()

		// Double-check. A request can miss the cache, then reach Do just
		// after another flight for the same key finished and filled the
		// cache. singleflight only merges calls that overlap in time, so
		// without this second look that late request would start a fresh
		// database load. (Found by the stampede test under -race load.)
		if raw, err := c.rdb.Get(loadCtx, c.key(id)).Bytes(); err == nil {
			var a auction.Auction
			if json.Unmarshal(raw, &a) == nil {
				return a, nil
			}
		}

		c.loads.Add(1)
		a, err := c.load(loadCtx, id)
		if err != nil {
			return auction.Auction{}, err
		}
		if b, err := json.Marshal(a); err == nil {
			// Best effort: a failed SET only costs a future miss.
			c.rdb.Set(loadCtx, c.key(id), b, TTL)
		}
		return a, nil
	})
	if err != nil {
		return auction.Auction{}, err
	}
	return v.(auction.Auction), nil
}

// Invalidate removes the cached entry for id.
func (c *Cache) Invalidate(ctx context.Context, id uuid.UUID) error {
	return c.rdb.Del(ctx, c.key(id)).Err()
}

// Stats is a snapshot of the counters.
type Stats struct {
	Hits, Misses, Loads, Errors int64
}

func (c *Cache) Stats() Stats {
	return Stats{Hits: c.hits.Load(), Misses: c.misses.Load(), Loads: c.loads.Load(), Errors: c.errs.Load()}
}

// InvalidationHandler deletes the cache entry of the auction an event is
// about. Every auction-changing event carries "auction_id", so one handler
// serves them all. Deleting twice is harmless, so it's idempotent.
func (c *Cache) InvalidationHandler() outbox.Handler {
	return outbox.HandlerFunc(func(ctx context.Context, event outbox.Event) error {
		var payload struct {
			AuctionID uuid.UUID `json:"auction_id"`
		}
		if err := outbox.DecodePayload(event, &payload); err != nil {
			return err
		}
		if payload.AuctionID == uuid.Nil {
			return nil
		}
		return c.Invalidate(ctx, payload.AuctionID)
	})
}
