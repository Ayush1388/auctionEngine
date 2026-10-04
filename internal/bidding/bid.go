// Package bidding places bids and settles finished auctions.
//
// This is the heart of the project: many users bid on the same auction at
// the same moment, and the result must be exactly what would have happened
// if the bids had arrived one at a time. Concretely:
//
//   - the current bid only ever goes up, by at least the minimum increment;
//   - exactly one bidder is leading at any time;
//   - the leader's money is reserved, everyone else's is free to spend;
//   - no user can spend the same money on two auctions (double spending);
//   - a retried request never places a second bid (idempotency);
//   - a bid can't sneak in after the auction ended.
//
// How: every bid runs in one database transaction that locks the auction
// row first and the affected wallets second (always in that order), makes
// its decision on the locked data, and writes the bid, the reservation, the
// ledger journals, the auction update and a bid.placed outbox event
// together. See service.go for the two locking strategies.
package bidding

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	// Anti-sniping: a bid in the last SnipeWindow of an auction pushes the
	// end back so at least SnipeWindow remains. Without it, "snipers" bid
	// in the final second and nobody can respond, which lowers prices for
	// sellers. MaxExtensions bounds how long an auction can be dragged out.
	SnipeWindow   = 2 * time.Minute
	MaxExtensions = 10

	// MaxIdempotencyKeyLength keeps client-supplied keys to a sane size.
	MaxIdempotencyKeyLength = 255
)

var (
	ErrAuctionNotActive = errors.New("auction is not accepting bids")
	ErrAuctionEnded     = errors.New("auction has ended")
	ErrOwnAuction       = errors.New("sellers cannot bid on their own auction")
	ErrInvalidAmount    = errors.New("bid amount must be positive")
	ErrInvalidKey       = errors.New("idempotency key must be at most 255 characters")

	// ErrIdempotencyMismatch means the Idempotency-Key was already used for
	// a different request (another auction or amount). Reusing a key for a
	// different operation is a client bug, so it is rejected, not replayed.
	ErrIdempotencyMismatch = errors.New("idempotency key was already used for a different bid")

	// ErrContention is returned by the optimistic strategy when it lost
	// the race too many times in a row. The client should retry.
	ErrContention = errors.New("auction is busy, please retry")

	// ErrUnavailable means bidding couldn't be reached at all (v0.9: the
	// bidding service is down, overloaded, or didn't answer in time).
	// Retrying later is appropriate.
	ErrUnavailable = errors.New("bidding is temporarily unavailable, please retry")
)

// BidTooLowError says what the minimum acceptable bid was, so the client
// can show it.
type BidTooLowError struct {
	Minimum int64
}

func (e *BidTooLowError) Error() string {
	return fmt.Sprintf("bid must be at least %d", e.Minimum)
}

// Bid is one accepted bid. Rejected bids are never stored.
type Bid struct {
	ID        uuid.UUID
	AuctionID uuid.UUID
	UserID    uuid.UUID
	Amount    int64
	CreatedAt time.Time
}

// PlaceBidInput is what a client sends.
type PlaceBidInput struct {
	AuctionID uuid.UUID
	UserID    uuid.UUID
	Amount    int64

	// IdempotencyKey comes from the Idempotency-Key header. Optional; when
	// present, retries with the same key return the original bid.
	IdempotencyKey string
}

// Result is what PlaceBid returns.
type Result struct {
	Bid Bid

	// Replayed is true when this request matched an earlier one with the
	// same idempotency key and nothing new happened.
	Replayed bool

	// EndsAt is the auction end after this bid, and Extended says whether
	// the bid triggered anti-sniping.
	EndsAt   time.Time
	Extended bool

	CurrentBid int64
	BidCount   int

	// Timings are the phases of the transaction that handled this request,
	// also set when the bid was refused (then only the phases that ran). They
	// are zero for the optimistic strategy.
	Timings Timings
}

// Timings splits a bid's transaction into the phases a person can reason
// about. The API reports them in a Server-Timing header so a client can show
// where the time of one bid went.
type Timings struct {
	Lock   time.Duration // waiting for the auction row (SELECT ... FOR UPDATE)
	Decide time.Duration // the rules, checked on locked data
	Write  time.Duration // bid, reservation, ledger journals, auction update, outbox event
	Commit time.Duration // the COMMIT round trip
}

// IsZero reports whether no phase was measured.
func (t Timings) IsZero() bool { return t == Timings{} }
