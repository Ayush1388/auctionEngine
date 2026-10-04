// Package auction holds the auction domain: items, auctions and the rules
// for how an auction moves through its lifecycle.
package auction

import (
	"time"

	"github.com/google/uuid"
)

type Item struct {
	ID          uuid.UUID
	OwnerID     uuid.UUID
	Type        string
	Name        string
	Description string
	CreatedAt   time.Time
}

type Auction struct {
	ID            uuid.UUID
	Item          Item
	OwnerID       uuid.UUID
	StartingPrice int64

	// MinIncrement is how much a new bid must beat the current one by.
	MinIncrement int64

	StartsAt  time.Time
	EndsAt    time.Time
	Status    Status
	CreatedAt time.Time
	UpdatedAt time.Time

	// Bidding state, maintained by the bidding package (v0.3).
	// CurrentBid and CurrentBidderID are both nil until the first bid.
	CurrentBid      *int64
	CurrentBidderID *uuid.UUID
	BidCount        int

	// Version increases on every change to the row. Optimistic bidders
	// read it, then update with "WHERE version = <what I read>", so a
	// concurrent change makes their update match nothing and they retry.
	Version int64

	// Extensions counts anti-sniping extensions of EndsAt.
	Extensions int

	// SettledAt is set once the winner's money has moved to the seller.
	SettledAt *time.Time
}
