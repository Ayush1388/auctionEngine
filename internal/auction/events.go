package auction

import (
	"time"

	"github.com/google/uuid"
)

// EventTypeCompleted is written to the outbox, in the same transaction,
// whenever an auction ends. Settlement (v0.3) will consume it.
const EventTypeCompleted = "auction.completed"

type CompletedEvent struct {
	AuctionID   uuid.UUID `json:"auction_id"`
	OwnerID     uuid.UUID `json:"owner_id"`
	WinningBid  *int64    `json:"winning_bid"`
	CompletedAt time.Time `json:"completed_at"`
}
