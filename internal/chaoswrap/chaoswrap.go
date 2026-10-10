// Package chaoswrap holds the fault-injection decorators that need other
// application packages (outbox handlers, the bidding service). The core
// switchboard is in package chaos, which the database layer imports.
package chaoswrap

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/bidding"
	"github.com/Ayush1388/auctionEngine/internal/breaker"
	"github.com/Ayush1388/auctionEngine/internal/chaos"
	"github.com/Ayush1388/auctionEngine/internal/outbox"
)

// Handler wraps an outbox handler so it fails (and is retried by the outbox)
// while f is broken.
func Handler(f chaos.Fault, next outbox.Handler) outbox.Handler {
	return outbox.HandlerFunc(func(ctx context.Context, e outbox.Event) error {
		if err := chaos.Fail(f); err != nil {
			return err
		}
		return next.Handle(ctx, e)
	})
}

// Bidder is the part of the bidding service the HTTP handlers use.
type Bidder interface {
	PlaceBid(ctx context.Context, in bidding.PlaceBidInput) (bidding.Result, error)
	History(ctx context.Context, auctionID uuid.UUID, limit, cursor string) (bidding.HistoryPage, error)
}

// GuardedBidder fails bids while the Bidding fault is on, through a circuit
// breaker, so the demo shows the real behaviour of a down dependency: a few
// failures, then the breaker opens and bids answer 503 at once.
type GuardedBidder struct {
	Next Bidder
	B    *breaker.Breaker
}

func (g GuardedBidder) PlaceBid(ctx context.Context, in bidding.PlaceBidInput) (bidding.Result, error) {
	var (
		res   bidding.Result
		inner error // the bid's own outcome; a refusal is not a dependency failure
	)
	err := g.B.Do(ctx, func(ctx context.Context) error {
		if chaos.Active(chaos.Bidding) {
			return bidding.ErrUnavailable
		}
		res, inner = g.Next.PlaceBid(ctx, in)
		if errors.Is(inner, bidding.ErrUnavailable) {
			return inner
		}
		return nil
	})
	switch {
	case errors.Is(err, breaker.ErrOpen):
		return bidding.Result{}, bidding.ErrUnavailable
	case err != nil:
		return bidding.Result{}, err
	}
	return res, inner
}

func (g GuardedBidder) History(ctx context.Context, id uuid.UUID, limit, cursor string) (bidding.HistoryPage, error) {
	return g.Next.History(ctx, id, limit, cursor)
}
