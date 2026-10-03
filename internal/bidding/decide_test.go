package bidding

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/auction"
)

// decide is pure, so every rule is tested here without a database.
func TestDecide(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	owner, alice, bob := uuid.New(), uuid.New(), uuid.New()
	i64 := func(v int64) *int64 { return &v }

	base := func() auction.Auction {
		return auction.Auction{
			ID:            uuid.New(),
			OwnerID:       owner,
			StartingPrice: 1000,
			MinIncrement:  100,
			StartsAt:      now.Add(-time.Hour),
			EndsAt:        now.Add(time.Hour),
			Status:        auction.StatusActive,
		}
	}
	leading := func(who uuid.UUID, amount int64) auction.Auction {
		a := base()
		a.CurrentBid = i64(amount)
		a.CurrentBidderID = &who
		a.BidCount = 1
		return a
	}

	tests := []struct {
		name        string
		auction     auction.Auction
		bidder      uuid.UUID
		amount      int64
		wantErr     error
		wantMin     int64 // for BidTooLowError
		wantReserve int64
		wantRelease *release
		wantExtend  bool
	}{
		{name: "first bid at starting price", auction: base(), bidder: alice, amount: 1000, wantReserve: 1000},
		{name: "first bid below starting price", auction: base(), bidder: alice, amount: 999, wantMin: 1000},
		{name: "outbid needs the increment", auction: leading(alice, 1000), bidder: bob, amount: 1099, wantMin: 1100},
		{name: "outbid releases previous leader", auction: leading(alice, 1000), bidder: bob, amount: 1100,
			wantReserve: 1100, wantRelease: &release{UserID: alice, Amount: 1000}},
		{name: "leader raising own bid reserves only the difference", auction: leading(alice, 1000), bidder: alice, amount: 1500,
			wantReserve: 500},
		{name: "seller cannot bid", auction: base(), bidder: owner, amount: 5000, wantErr: ErrOwnAuction},
		{name: "zero amount", auction: base(), bidder: alice, amount: 0, wantErr: ErrInvalidAmount},
		{name: "not started yet", auction: func() auction.Auction { a := base(); a.Status = auction.StatusNotActive; return a }(),
			bidder: alice, amount: 1000, wantErr: ErrAuctionNotActive},
		{name: "status active but start in the future", auction: func() auction.Auction { a := base(); a.StartsAt = now.Add(time.Minute); return a }(),
			bidder: alice, amount: 1000, wantErr: ErrAuctionNotActive},
		{name: "cancelled", auction: func() auction.Auction { a := base(); a.Status = auction.StatusCancelled; return a }(),
			bidder: alice, amount: 1000, wantErr: ErrAuctionNotActive},
		{name: "completed", auction: func() auction.Auction { a := base(); a.Status = auction.StatusCompleted; return a }(),
			bidder: alice, amount: 1000, wantErr: ErrAuctionEnded},
		{name: "end time passed before the worker noticed", auction: func() auction.Auction { a := base(); a.EndsAt = now; return a }(),
			bidder: alice, amount: 1000, wantErr: ErrAuctionEnded},
		{name: "bid in the last minute extends the auction", auction: func() auction.Auction { a := base(); a.EndsAt = now.Add(30 * time.Second); return a }(),
			bidder: alice, amount: 1000, wantReserve: 1000, wantExtend: true},
		{name: "no extension after the maximum", auction: func() auction.Auction {
			a := base()
			a.EndsAt = now.Add(30 * time.Second)
			a.Extensions = MaxExtensions
			return a
		}(), bidder: alice, amount: 1000, wantReserve: 1000},
		{name: "free auction still needs a positive bid", auction: func() auction.Auction { a := base(); a.StartingPrice = 0; return a }(),
			bidder: alice, amount: 1, wantReserve: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := decide(tt.auction, tt.bidder, tt.amount, now)

			if tt.wantMin != 0 {
				var tooLow *BidTooLowError
				if !errors.As(err, &tooLow) || tooLow.Minimum != tt.wantMin {
					t.Fatalf("got %v, want BidTooLowError{%d}", err, tt.wantMin)
				}
				return
			}
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if p.Reserve != tt.wantReserve {
				t.Errorf("Reserve = %d, want %d", p.Reserve, tt.wantReserve)
			}
			if (p.Release == nil) != (tt.wantRelease == nil) ||
				(p.Release != nil && *p.Release != *tt.wantRelease) {
				t.Errorf("Release = %+v, want %+v", p.Release, tt.wantRelease)
			}
			if p.Extended != tt.wantExtend {
				t.Errorf("Extended = %v, want %v", p.Extended, tt.wantExtend)
			}
			if p.Extended && !p.NewEndsAt.Equal(now.Add(SnipeWindow)) {
				t.Errorf("NewEndsAt = %v, want now + %v", p.NewEndsAt, SnipeWindow)
			}
		})
	}
}
