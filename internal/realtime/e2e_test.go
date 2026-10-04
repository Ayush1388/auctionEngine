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

	// The update also names the bid (so a client can match its own request or
	// learn it was outbid) and carries the API's own timestamps, so the time the
	// event spent between commit and push needs no clock sync with the browser.
	b, _ := m["bid"].(map[string]any)
	if b == nil || b["bidder_id"] != bidder.String() || b["amount"] != 1500.0 || b["id"] == "" {
		t.Fatalf("update does not describe the bid: %v", m["bid"])
	}
	tm, _ := m["timing"].(map[string]any)
	placed, err1 := time.Parse(time.RFC3339Nano, tm["placed_at"].(string))
	sent, err2 := time.Parse(time.RFC3339Nano, tm["sent_at"].(string))
	if err1 != nil || err2 != nil || sent.Before(placed) {
		t.Fatalf("timing is wrong: %v", tm)
	}
}
