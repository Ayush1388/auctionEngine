package realtime_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/bidding"
	"github.com/Ayush1388/auctionEngine/internal/outbox"
	"github.com/Ayush1388/auctionEngine/internal/realtime"
	"github.com/Ayush1388/auctionEngine/internal/testdb"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

// The whole path: a bid commits in PostgreSQL → its bid.placed outbox event
// is handled → the watcher's WebSocket receives the new price.
func TestBidReachesWatcherEndToEnd(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	now := time.Now().UTC()

	newUser := func() uuid.UUID {
		id := uuid.New()
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, activated_at) VALUES ($1, $2, 'x', now())`, id, id.String()+"@example.com"); err != nil {
			t.Fatal(err)
		}
		return id
	}
	seller, bidder := newUser(), newUser()

	auctions := auction.NewService(pool, auction.NewRepository(pool))
	auctions.SetClock(func() time.Time { return now })
	a, err := auctions.Create(ctx, seller, auction.CreateInput{
		Item:          auction.CreateItemInput{Name: "Vintage camera", Type: "electronics"},
		StartingPrice: 1000, StartsAt: now.Add(time.Minute), EndsAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE auctions SET status = 'ACTIVE', starts_at = now() - interval '1 minute' WHERE id = $1`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := wallet.NewService(pool).Deposit(ctx, bidder, 10_000, "seed"); err != nil {
		t.Fatal(err)
	}

	hub := realtime.NewHub(10)
	watcher := dial(t, server(t, hub, realtime.Options{}), nil)
	subscribe(t, watcher, a.ID)

	router := outbox.NewRouter()
	for _, typ := range []string{auction.EventTypeCreated, bidding.EventTypePlaced} {
		router.Register(typ, realtime.EventHandler(auctions.Get, realtime.NewLocalPublisher(hub)))
	}
	worker := outbox.NewWorker(outbox.NewRepository(pool), router, quiet)
	// Drain the auction.created event first so only the bid matters below.
	if err := worker.ProcessOnce(ctx); err != nil {
		t.Fatal(err)
	}
	read(t, watcher) // snapshot from auction.created

	if _, err := bidding.NewService(pool, bidding.Pessimistic).PlaceBid(ctx, bidding.PlaceBidInput{
		AuctionID: a.ID, UserID: bidder, Amount: 1500,
	}); err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	if err := worker.ProcessOnce(ctx); err != nil {
		t.Fatal(err)
	}

	m := read(t, watcher)
	snap := m["auction"].(map[string]any)
	if m["cause"] != "bid.placed" || snap["current_bid"] != 1500.0 || snap["bid_count"] != 1.0 {
		t.Fatalf("watcher got %v", m)
	}
}
