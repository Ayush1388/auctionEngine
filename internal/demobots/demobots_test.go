package demobots_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/bidding"
	"github.com/Ayush1388/auctionEngine/internal/demobots"
	"github.com/Ayush1388/auctionEngine/internal/testdb"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

func TestDisabledHandlerAnswers404ExceptState(t *testing.T) {
	h := demobots.NewHandler(nil, "")
	rec := httptest.NewRecorder()
	h.State(rec, httptest.NewRequest("GET", "/v1/demo/bots", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Fatalf("state: %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	h.StartStress(rec, httptest.NewRequest("POST", "/v1/demo/stress", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("a disabled bot server must answer 404, got %d", rec.Code)
	}
}

// A stress run against a real database must keep every invariant it checks.
func TestStressRunKeepsTheInvariants(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	svc := auction.NewService(pool, auction.NewRepository(pool))
	bids := bidding.NewService(pool, bidding.Pessimistic)
	bots := demobots.New(demobots.Options{Pool: pool, Bidder: bids, Auctions: svc, Wallets: wallet.NewService(pool)})

	owner := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, activated_at) VALUES ($1, 'o@x.test', 'x', now())`, owner); err != nil {
		t.Fatal(err)
	}
	itemID, id := uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO items (id, owner_id, type, name, description) VALUES ($1, $2, 'car', 'Test', '')`, itemID, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO auctions (id, item_id, owner_id, starting_price, min_increment, starts_at, ends_at, status)
		VALUES ($1, $2, $3, 1000, 100, now() - interval '1 hour', now() + interval '1 hour', 'ACTIVE')`, id, itemID, owner); err != nil {
		t.Fatal(err)
	}

	runID := bots.StartStress(id, 25, 3)
	read, ok := bots.Events(runID)
	if !ok {
		t.Fatal("run not found")
	}
	deadline := time.Now().Add(60 * time.Second)
	sent := 0
	for time.Now().Before(deadline) {
		batch, wake, done := read(sent)
		for _, e := range batch {
			sent++
			if e.Type == "error" {
				t.Fatalf("run failed: %s", e.Error)
			}
			if e.Type == "done" {
				if e.Invariant == nil || !e.Invariant.OK {
					t.Fatalf("invariants failed: %+v", e.Invariant)
				}
				if e.Accepted < 3 {
					t.Fatalf("expected at least one accepted bid per round, got %d", e.Accepted)
				}
				return
			}
		}
		if done && len(batch) == 0 {
			break
		}
		select {
		case <-wake:
		case <-time.After(time.Second):
		}
	}
	t.Fatal("the run did not finish")
}

func TestAmbientRivalsSwitchOnAndOff(t *testing.T) {
	pool := testdb.New(t)
	bots := demobots.New(demobots.Options{
		Pool: pool, Bidder: bidding.NewService(pool, bidding.Pessimistic),
		Auctions: auction.NewService(pool, auction.NewRepository(pool)), Wallets: wallet.NewService(pool),
	})
	ctx := context.Background()
	if bots.Ambient().On {
		t.Fatal("the rivals must start off")
	}
	if err := bots.SetAmbient(ctx, true); err != nil {
		t.Fatal(err)
	}
	if !bots.Ambient().On {
		t.Fatal("not on after SetAmbient(true)")
	}
	if err := bots.SetAmbient(ctx, true); err != nil { // idempotent
		t.Fatal(err)
	}
	if err := bots.SetAmbient(ctx, false); err != nil {
		t.Fatal(err)
	}
	if bots.Ambient().On {
		t.Fatal("still on after SetAmbient(false)")
	}
}
