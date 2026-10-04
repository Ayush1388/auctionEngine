package auctioncache_test

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/auctioncache"
	"github.com/Ayush1388/auctionEngine/internal/outbox"
	"github.com/Ayush1388/auctionEngine/internal/redistest"
)

// fakeDB counts loads and can be slowed down.
type fakeDB struct {
	loads atomic.Int64
	delay time.Duration
	price atomic.Int64
}

func (f *fakeDB) load(ctx context.Context, id uuid.UUID) (auction.Auction, error) {
	f.loads.Add(1)
	time.Sleep(f.delay)
	return auction.Auction{ID: id, StartingPrice: f.price.Load(), Status: auction.StatusActive}, nil
}

func TestCacheAside(t *testing.T) {
	rdb, prefix := redistest.New(t)
	db := &fakeDB{}
	db.price.Store(100)
	c := auctioncache.New(rdb, prefix, db.load)
	ctx := context.Background()
	id := uuid.New()

	a, err := c.Get(ctx, id)
	if err != nil || a.StartingPrice != 100 {
		t.Fatalf("first read: %+v %v", a, err)
	}
	if _, err := c.Get(ctx, id); err != nil {
		t.Fatal(err)
	}
	if db.loads.Load() != 1 {
		t.Fatalf("second read hit the database (loads = %d)", db.loads.Load())
	}

	// The auction changes; until invalidated, readers see the cached copy.
	db.price.Store(200)
	if a, _ := c.Get(ctx, id); a.StartingPrice != 100 {
		t.Fatal("expected the cached value before invalidation")
	}

	// An auction event arrives through the outbox and drops the entry.
	payload, _ := json.Marshal(map[string]any{"auction_id": id})
	if err := c.InvalidationHandler().Handle(ctx, outbox.Event{EventType: "bid.placed", Payload: payload}); err != nil {
		t.Fatal(err)
	}
	if a, _ := c.Get(ctx, id); a.StartingPrice != 200 {
		t.Fatalf("after invalidation got %d, want 200", a.StartingPrice)
	}

	if ttl := rdb.TTL(ctx, prefix+"auction:"+id.String()).Val(); ttl <= 0 || ttl > auctioncache.TTL {
		t.Fatalf("entry TTL = %v, want (0, %v]", ttl, auctioncache.TTL)
	}

	if s := c.Stats(); s.Hits != 2 || s.Misses != 2 || s.Loads != 2 {
		t.Fatalf("stats = %+v", s)
	}
}

// A cold, popular key: 100 requests miss at the same moment. Without
// singleflight that's 100 identical database queries.
func TestStampedeCollapsesToOneLoad(t *testing.T) {
	rdb, prefix := redistest.New(t)
	db := &fakeDB{delay: 50 * time.Millisecond}
	c := auctioncache.New(rdb, prefix, db.load)
	id := uuid.New()

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := c.Get(context.Background(), id); err != nil {
				t.Error(err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if n := db.loads.Load(); n != 1 {
		t.Fatalf("database loaded %d times, want 1", n)
	}
}

// Redis being down must cost speed, never availability.
func TestRedisDownFallsBackToDatabase(t *testing.T) {
	dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 50 * time.Millisecond, MaxRetries: -1})
	defer dead.Close()

	db := &fakeDB{}
	db.price.Store(42)
	c := auctioncache.New(dead, "x:", db.load)

	a, err := c.Get(context.Background(), uuid.New())
	if err != nil || a.StartingPrice != 42 {
		t.Fatalf("got %+v, %v; want the database value", a, err)
	}
	if c.Stats().Errors != 1 {
		t.Fatalf("stats = %+v", c.Stats())
	}
}
