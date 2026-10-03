package bidding

import (
	"time"

	"github.com/google/uuid"
)

// Outbox event types published by this package. They are written in the
// same transaction as the change they describe (see outbox package docs).
const (
	// EventTypePlaced: a bid was accepted. Consumers: WebSocket broadcast
	// (v0.7), Kafka (v0.8), trending auctions in Redis (v0.5).
	EventTypePlaced = "bid.placed"

	// EventTypeSettled: the winner's money moved to the seller.
	EventTypeSettled = "auction.settled"
)

type PlacedEvent struct {
	AuctionID        uuid.UUID  `json:"auction_id"`
	BidID            uuid.UUID  `json:"bid_id"`
	BidderID         uuid.UUID  `json:"bidder_id"`
	PreviousBidderID *uuid.UUID `json:"previous_bidder_id"`
	Amount           int64      `json:"amount"`
	BidCount         int        `json:"bid_count"`
	EndsAt           time.Time  `json:"ends_at"`
	Extended         bool       `json:"extended"`
	PlacedAt         time.Time  `json:"placed_at"`
}

type SettledEvent struct {
	AuctionID uuid.UUID  `json:"auction_id"`
	SellerID  uuid.UUID  `json:"seller_id"`
	WinnerID  *uuid.UUID `json:"winner_id"`
	Amount    int64      `json:"amount"`
	SettledAt time.Time  `json:"settled_at"`
}
