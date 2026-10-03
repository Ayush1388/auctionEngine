package bidding

import (
	"time"

	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/auction"
)

// plan is everything a bid will change, worked out before anything is
// written. Keeping the decision a pure function (no database, no clock of
// its own) means every business rule can be unit tested in microseconds,
// and both locking strategies share exactly the same rules.
type plan struct {
	// Reserve is how much moves from the bidder's available balance to
	// reserved. For a new leader it's the full amount; when the leader
	// raises their own bid it's only the difference.
	Reserve int64

	// Release is the previous leader's reservation to hand back, or nil
	// when there was no leader or the leader is raising their own bid.
	Release *release

	NewEndsAt     time.Time
	Extended      bool
	NewExtensions int
}

type release struct {
	UserID uuid.UUID
	Amount int64
}

// MinimumBid is the smallest amount the next bid may be.
func MinimumBid(a auction.Auction) int64 {
	if a.CurrentBid == nil {
		// bids.amount is CHECK (> 0), so even a free auction needs 1.
		return max(a.StartingPrice, 1)
	}
	return *a.CurrentBid + a.MinIncrement
}

// decide applies the bidding rules to a snapshot of the auction. The
// caller must guarantee the snapshot can't change before its writes commit:
// by holding the row lock (pessimistic) or by a version check at write
// time (optimistic).
func decide(a auction.Auction, bidder uuid.UUID, amount int64, now time.Time) (plan, error) {
	if amount <= 0 {
		return plan{}, ErrInvalidAmount
	}

	// The lifecycle worker flips the status on a 1-second tick, so the
	// clock is checked as well: an auction whose end time has passed stops
	// taking bids immediately, even if the worker hasn't caught up.
	switch {
	case a.Status != auction.StatusActive:
		if a.Status == auction.StatusCompleted {
			return plan{}, ErrAuctionEnded
		}
		return plan{}, ErrAuctionNotActive
	case now.Before(a.StartsAt):
		return plan{}, ErrAuctionNotActive
	case !now.Before(a.EndsAt):
		return plan{}, ErrAuctionEnded
	}

	if bidder == a.OwnerID {
		return plan{}, ErrOwnAuction
	}

	if minimum := MinimumBid(a); amount < minimum {
		return plan{}, &BidTooLowError{Minimum: minimum}
	}

	p := plan{
		Reserve:       amount,
		NewEndsAt:     a.EndsAt,
		NewExtensions: a.Extensions,
	}

	if a.CurrentBidderID != nil {
		if *a.CurrentBidderID == bidder {
			// Raising your own bid: the old amount is already reserved,
			// so only the difference moves. Nothing is released.
			p.Reserve = amount - *a.CurrentBid
		} else {
			p.Release = &release{UserID: *a.CurrentBidderID, Amount: *a.CurrentBid}
		}
	}

	if a.EndsAt.Sub(now) < SnipeWindow && a.Extensions < MaxExtensions {
		p.NewEndsAt = now.Add(SnipeWindow)
		p.Extended = true
		p.NewExtensions = a.Extensions + 1
	}

	return p, nil
}
