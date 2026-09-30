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
	CurrentBid    *int64
	StartsAt      time.Time
	EndsAt        time.Time
	Status        Status
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
