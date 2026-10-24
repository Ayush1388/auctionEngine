package bidding

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/auction"
)

func proxyAuction(current int64, leader *uuid.UUID, inc int64) auction.Auction {
	a := auction.Auction{Status: auction.StatusActive, StartingPrice: 100, MinIncrement: inc, OwnerID: uuid.New()}
	if leader != nil {
		a.CurrentBid = &current
		a.CurrentBidderID = leader
	}
	return a
}

func rich(uuid.UUID) int64 { return 1 << 40 }

func TestNextAutoBid(t *testing.T) {
	ann, bob, cy := uuid.New(), uuid.New(), uuid.New()
	t0 := time.Now()
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }

	t.Run("a lone challenger bids the minimum", func(t *testing.T) {
		a := proxyAuction(1000, &ann, 50)
		got, ok := nextAutoBid(a, []ProxyEntry{{bob, 5000, at(0)}}, rich)
		if !ok || got.UserID != bob || got.Amount != 1050 {
			t.Fatalf("got %+v %v", got, ok)
		}
	})
	t.Run("higher maximum wins at one increment over the lower", func(t *testing.T) {
		a := proxyAuction(1000, &ann, 50)
		got, ok := nextAutoBid(a, []ProxyEntry{{ann, 2000, at(0)}, {bob, 5000, at(1)}}, rich)
		if !ok || got.UserID != bob || got.Amount != 2050 {
			t.Fatalf("got %+v %v", got, ok)
		}
	})
	t.Run("a lower challenger loses and the leader's price rises to just over it", func(t *testing.T) {
		a := proxyAuction(1000, &ann, 50)
		got, ok := nextAutoBid(a, []ProxyEntry{{ann, 5000, at(0)}, {bob, 2000, at(1)}}, rich)
		if !ok || got.UserID != ann || got.Amount != 2050 {
			t.Fatalf("got %+v %v", got, ok)
		}
	})
	t.Run("a tie goes to the earlier instruction, at the tied amount", func(t *testing.T) {
		a := proxyAuction(1000, &ann, 50)
		got, ok := nextAutoBid(a, []ProxyEntry{{ann, 3000, at(0)}, {bob, 3000, at(1)}}, rich)
		if !ok || got.UserID != ann || got.Amount != 3000 {
			t.Fatalf("got %+v %v", got, ok)
		}
	})
	t.Run("the price never passes the winner's own maximum", func(t *testing.T) {
		a := proxyAuction(1000, &ann, 500)
		got, _ := nextAutoBid(a, []ProxyEntry{{ann, 1800, at(0)}, {bob, 1600, at(1)}}, rich)
		if got.UserID != ann || got.Amount != 1800 {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("a maximum below the minimum does nothing", func(t *testing.T) {
		a := proxyAuction(1000, &ann, 50)
		if _, ok := nextAutoBid(a, []ProxyEntry{{bob, 1040, at(0)}}, rich); ok {
			t.Fatal("bid below minimum")
		}
	})
	t.Run("funds cap the maximum", func(t *testing.T) {
		a := proxyAuction(1000, &ann, 50)
		poor := func(u uuid.UUID) int64 {
			if u == bob {
				return 1200
			}
			return 1 << 40
		}
		got, ok := nextAutoBid(a, []ProxyEntry{{ann, 1500, at(0)}, {bob, 9000, at(1)}}, poor)
		if !ok || got.UserID != ann || got.Amount != 1250 {
			t.Fatalf("got %+v %v", got, ok)
		}
	})
	t.Run("the seller never auto-bids", func(t *testing.T) {
		a := proxyAuction(1000, &ann, 50)
		if _, ok := nextAutoBid(a, []ProxyEntry{{a.OwnerID, 9000, at(0)}}, rich); ok {
			t.Fatal("owner bid")
		}
	})
	t.Run("an auction with no bids opens at the starting price", func(t *testing.T) {
		a := proxyAuction(0, nil, 50)
		got, ok := nextAutoBid(a, []ProxyEntry{{bob, 900, at(0)}}, rich)
		if !ok || got.UserID != bob || got.Amount != 100 {
			t.Fatalf("got %+v %v", got, ok)
		}
	})
	t.Run("the contest ends in at most one step per proxy", func(t *testing.T) {
		a := proxyAuction(1000, &ann, 50)
		ps := []ProxyEntry{{ann, 2000, at(0)}, {bob, 3000, at(1)}, {cy, 4000, at(2)}}
		for i := 0; i < 10; i++ {
			next, ok := nextAutoBid(a, ps, rich)
			if !ok {
				if a.CurrentBidderID == nil || *a.CurrentBidderID != cy || *a.CurrentBid != 3050 {
					t.Fatalf("settled at %v by %v after %d steps", *a.CurrentBid, a.CurrentBidderID, i)
				}
				return
			}
			amt := next.Amount
			a.CurrentBid, a.CurrentBidderID = &amt, &next.UserID
		}
		t.Fatal("did not settle")
	})
}
