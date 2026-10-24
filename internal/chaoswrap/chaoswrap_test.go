package chaoswrap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/bidding"
	"github.com/Ayush1388/auctionEngine/internal/breaker"
	"github.com/Ayush1388/auctionEngine/internal/chaos"
)

type fake struct{ calls int }

func (f *fake) PlaceBid(context.Context, bidding.PlaceBidInput) (bidding.Result, error) {
	f.calls++
	return bidding.Result{}, &bidding.BidTooLowError{Minimum: 5}
}
func (f *fake) History(context.Context, uuid.UUID, string, string) (bidding.HistoryPage, error) {
	return bidding.HistoryPage{}, nil
}

func TestGuardedBidderOpensTheBreakerAndFailsFast(t *testing.T) {
	chaos.Enable()
	t.Cleanup(chaos.Reset)

	inner := &fake{}
	b := breaker.New("bidding", breaker.Options{Threshold: 3, Cooldown: time.Hour, IsFailure: bidding.BreakerFailure})
	g := GuardedBidder{Next: inner, B: b}
	ctx := context.Background()

	// a refused bid is the service working: it must not count against the breaker
	for i := 0; i < 5; i++ {
		var tooLow *bidding.BidTooLowError
		if _, err := g.PlaceBid(ctx, bidding.PlaceBidInput{}); !errors.As(err, &tooLow) {
			t.Fatalf("refusal was not passed through: %v", err)
		}
	}
	if b.State() != breaker.Closed {
		t.Fatal("refusals tripped the breaker")
	}

	if err := chaos.Set(chaos.Bidding, true); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := g.PlaceBid(ctx, bidding.PlaceBidInput{}); !errors.Is(err, bidding.ErrUnavailable) {
			t.Fatalf("want unavailable, got %v", err)
		}
	}
	if b.State() != breaker.Open {
		t.Fatal("breaker did not open after the threshold")
	}
	calls := inner.calls
	if _, err := g.PlaceBid(ctx, bidding.PlaceBidInput{}); !errors.Is(err, bidding.ErrUnavailable) {
		t.Fatalf("open breaker must answer unavailable, got %v", err)
	}
	if inner.calls != calls {
		t.Fatal("an open breaker still called the service")
	}
}
