package bidding_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/bidding"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

// Maximum bids are answered inside the bid's own transaction, so they must
// keep every money invariant of manual bidding.
func TestProxyBidding(t *testing.T) {
	forEachStrategy(t, func(t *testing.T, e *env) {
		seller, ann, bob := e.user(0), e.user(1_000_000), e.user(1_000_000)
		a := e.activeAuction(seller, 1000, 100, time.Hour)

		// Ann sets a maximum of 5000: she takes the lead at the opening price.
		st, err := e.bids.SetProxy(e.ctx, a, ann, 5000)
		if err != nil || !st.Leading || st.CurrentBid != 1000 {
			t.Fatalf("set proxy: %+v %v", st, err)
		}

		// Bob bids 2000 by hand: Ann answers at once with 2100 and keeps the lead.
		res, err := e.bid(a, bob, 2000)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Countered || res.CurrentBid != 2100 {
			t.Fatalf("expected a countered bid at 2100, got %+v", res)
		}
		if au := e.auction(a); *au.CurrentBidderID != ann || *au.CurrentBid != 2100 {
			t.Fatalf("auction: %+v", au)
		}
		// Only the visible bid is held, not the maximum.
		if w := e.wallet(ann); w.Reserved != 2100 {
			t.Fatalf("held %d, want 2100", w.Reserved)
		}
		if w := e.wallet(bob); w.Reserved != 0 {
			t.Fatalf("bob holds %d, want 0", w.Reserved)
		}

		// Bob sets a higher maximum: he wins at one increment over Ann's maximum.
		st, err = e.bids.SetProxy(e.ctx, a, bob, 9000)
		if err != nil || !st.Leading || st.CurrentBid != 5100 {
			t.Fatalf("bob proxy: %+v %v", st, err)
		}
		if w := e.wallet(ann); w.Reserved != 0 || w.Available != 1_000_000 {
			t.Fatalf("ann should be fully released: %+v", w)
		}
		e.reconcile()

		if n := e.count(`SELECT count(*) FROM bids WHERE auction_id = $1 AND auto`, a); n < 2 {
			t.Fatalf("expected automatic bids to be recorded, got %d", n)
		}
	})
}

func TestProxyBidsAreRaceFree(t *testing.T) {
	forEachStrategy(t, func(t *testing.T, e *env) {
		seller := e.user(0)
		a := e.activeAuction(seller, 1000, 100, time.Hour)
		users := make([]uuid.UUID, 6)
		for i := range users {
			users[i] = e.user(10_000_000)
		}
		var wg sync.WaitGroup
		for i, u := range users {
			wg.Add(1)
			go func() {
				defer wg.Done()
				max := int64(2000 + 1000*i)
				_, _ = e.bids.SetProxy(e.ctx, a, u, max)
				_, _ = e.bids.PlaceBid(e.ctx, bidding.PlaceBidInput{AuctionID: a, UserID: u, Amount: 1000 + int64(i)*100})
			}()
		}
		wg.Wait()
		// The highest maximum (7000) wins at one increment over the runner-up (6000).
		au := e.auction(a)
		if *au.CurrentBidderID != users[5] || *au.CurrentBid != 6100 {
			t.Fatalf("winner %v at %d, want users[5] at 6100", *au.CurrentBidderID, *au.CurrentBid)
		}
		e.reconcile()
	})
}

// Bidders with maximums on two auctions, bidding on both at once in opposite
// orders: the shape that deadlocks if wallets are locked one auction at a time
// in different orders. Postgres aborts a deadlock victim with 40P01, which
// would surface here as an error.
func TestProxyCrossAuctionDoesNotDeadlock(t *testing.T) {
	forEachStrategy(t, func(t *testing.T, e *env) {
		seller := e.user(0)
		a1 := e.activeAuction(seller, 1000, 100, time.Hour)
		a2 := e.activeAuction(seller, 1000, 100, time.Hour)
		const n = 8
		users := make([]uuid.UUID, n)
		for i := range users {
			users[i] = e.user(100_000_000)
		}
		var wg sync.WaitGroup
		errs := make(chan error, 20000)
		for round := 0; round < 40; round++ {
			for i, u := range users {
				for k, au := range []uuid.UUID{a1, a2} {
					if (i+k)%2 == 1 {
						au = []uuid.UUID{a2, a1}[k] // half the users go to the auctions in the other order
					}
					wg.Add(1)
					go func() {
						defer wg.Done()
						max := int64(3000 + 700*(i+1) + 90*round)
						if _, err := e.bids.SetProxy(e.ctx, au, u, max); err != nil && !isBusinessError(err) {
							errs <- err
						}
						amount := int64(1000 + 100*(round*n+i))
						if _, err := e.bids.PlaceBid(e.ctx, bidding.PlaceBidInput{AuctionID: au, UserID: u, Amount: amount}); err != nil && !isBusinessError(err) {
							errs <- err
						}
					}()
				}
			}
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Errorf("unexpected error: %v", err)
		}
		e.reconcile()
	})
}

// isBusinessError is a refusal the rules explain (too low, no funds, busy),
// as opposed to a database failure such as a deadlock.
func isBusinessError(err error) bool {
	var tooLow *bidding.BidTooLowError
	return errors.As(err, &tooLow) || errors.Is(err, wallet.ErrInsufficientFunds) || errors.Is(err, bidding.ErrContention) ||
		errors.Is(err, bidding.ErrAuctionEnded) || errors.Is(err, bidding.ErrAuctionNotActive)
}
