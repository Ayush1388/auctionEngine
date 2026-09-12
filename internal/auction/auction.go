package auction

import "time"

type Status string

const (
	StatusNotActive Status = "NOT_ACTIVE"
	StatusActive    Status = "ACTIVE"
	StatusCompleted Status = "COMPLETED"
)

type Wallet struct {
	Available int64
	Reserved  int64
}

type User struct {
	ID     string
	Wallet Wallet
}

type Auction struct {
	ID            string
	ItemID        string
	OwnerID       string
	StartingPrice int64
	CurrentBid    int64
	StartsAt      time.Time
	EndsAt        time.Time
	Status        Status
}
type Item struct {
	ID          string
	Type        string
	Name        string
	Description string
}

type Bid struct {
	ID        string
	AuctionID string
	UserID    string
	Amount    int64
	CreatedAt time.Time
}
