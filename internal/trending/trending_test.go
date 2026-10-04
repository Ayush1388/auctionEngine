package trending_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/Ayush1388/auctionEngine/internal/redistest"
	"github.com/Ayush1388/auctionEngine/internal/testdb"
	"github.com/Ayush1388/auctionEngine/internal/trending"
)

func TestRecordIsIdempotentAndTopIsOrdered(t *testing.T) {
	rdb, prefix := redistest.New(t)
	now := time.Date(2026, 10, 1, 12, 30, 0, 0, time.UTC)
	r := trending.NewRedis(rdb, prefix, nil)
	r.SetClock(func() time.Time { return now })
	ctx := context.Background()

	hot, warm, cold := uuid.New(), uuid.New(), uuid.New()
	counts := map[uuid.UUID]int{hot: 5, warm: 3, cold: 1}
	for a, n := range counts {
		for i := 0; i < n; i++ {
			bid := uuid.New()
			first, err := r.Record(ctx, bid, a, now)
			if err != nil || !first {
				t.Fatalf("record: %v %v", first, err)
			}
			// The outbox delivers the same event again: must not count twice.
			again, err := r.Record(ctx, bid, a, now)
			if err != nil || again {
				t.Fatalf("redelivered bid was counted: %v %v", again, err)
			}
		}
	}

	top, err := r.Top(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 2 || top[0] != hot || top[1] != warm {
		t.Fatalf("top = %v, want [hot warm]", top)
	}
}

func TestOlderBucketCountsLessAsTheHourPasses(t *testing.T) {
	rdb, prefix := redistest.New(t)
	ctx := context.Background()
	r := trending.NewRedis(rdb, prefix, nil)

	earlier, recent := uuid.New(), uuid.New()
	for i := 0; i < 4; i++ {
		r.Record(ctx, uuid.New(), earlier, time.Date(2026, 10, 1, 11, 50, 0, 0, time.UTC))
	}
	for i := 0; i < 2; i++ {
		r.Record(ctx, uuid.New(), recent, time.Date(2026, 10, 1, 12, 5, 0, 0, time.UTC))
	}

	// At 12:10 the 11:xx bucket still counts 5/6: 4 × 0.83 = 3.3 > 2.
	r.SetClock(func() time.Time { return time.Date(2026, 10, 1, 12, 10, 0, 0, time.UTC) })
	if top, _ := r.Top(ctx, 1); top[0] != earlier {
		t.Fatalf("at 12:10 want earlier first, got %v", top)
	}

	// At 12:50 it counts 1/6: 4 × 0.17 = 0.7 < 2.
	r.SetClock(func() time.Time { return time.Date(2026, 10, 1, 12, 50, 0, 0, time.UTC) })
	if top, _ := r.Top(ctx, 1); top[0] != recent {
		t.Fatalf("at 12:50 want recent first, got %v", top)
	}
}

func TestPostgresRankingAndFallback(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	owner := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, password_hash) VALUES ($1, 'o@x', 'x')`, owner); err != nil {
		t.Fatal(err)
	}
	mk := func(bids int) uuid.UUID {
		item, id := uuid.New(), uuid.New()
		pool.Exec(ctx, `INSERT INTO items (id, owner_id, type, name, description) VALUES ($1, $2, 't', 'n', '')`, item, owner)
		pool.Exec(ctx, `INSERT INTO auctions (id, item_id, owner_id, starting_price, starts_at, ends_at, status)
			VALUES ($1, $2, $3, 1, now() - interval '1 hour', now() + interval '1 hour', 'ACTIVE')`, id, item, owner)
		for i := 0; i < bids; i++ {
			if _, err := pool.Exec(ctx, `INSERT INTO bids (id, auction_id, user_id, amount) VALUES ($1, $2, $3, $4)`,
				uuid.New(), id, owner, i+1); err != nil {
				t.Fatal(err)
			}
		}
		return id
	}
	busy, quiet := mk(4), mk(1)

	pg := trending.NewPostgres(pool)
	top, err := pg.Top(ctx, 5)
	if err != nil || len(top) != 2 || top[0] != busy || top[1] != quiet {
		t.Fatalf("postgres top = %v, %v", top, err)
	}

	// With Redis unreachable, the Redis ranker answers from Postgres.
	dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 50 * time.Millisecond, MaxRetries: -1})
	defer dead.Close()
	top, err = trending.NewRedis(dead, "x:", pg).Top(ctx, 5)
	if err != nil || len(top) != 2 || top[0] != busy {
		t.Fatalf("fallback top = %v, %v", top, err)
	}
}
