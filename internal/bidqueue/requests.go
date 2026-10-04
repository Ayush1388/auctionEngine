// Package bidqueue implements asynchronous bidding through Kafka (v0.8).
//
//	POST /bids (Prefer: respond-async)
//	   │ one transaction: INSERT bid_requests (PENDING) + outbox bid.requested
//	   ▼
//	202 Accepted {request_id}            outbox relay ─► Kafka "auction-bids" (key = auction_id)
//	                                                         │ consumer group "bid-workers"
//	                                                         ▼
//	GET /v1/bid-requests/{id} ◄── UPDATE bid_requests ◄── Worker: bidding.PlaceBid
//
// Why go asynchronous at all? The synchronous path holds an HTTP request
// open while it waits for the auction's row lock. On a hot auction, many
// requests queue on that one lock inside PostgreSQL, each holding a
// connection. With Kafka, the queue moves out of the database: all bids for
// one auction sit in one partition and one worker applies them one after
// another, so they never contend for the lock, and the API answers in
// milliseconds whatever the load.
//
// Correctness doesn't change: the worker calls the same PlaceBid
// transaction as the synchronous path, with its row locks and checks.
// Kafka changes *who waits and where*, not the rules.
package bidqueue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Ayush1388/auctionEngine/internal/database"
	"github.com/Ayush1388/auctionEngine/internal/outbox"
)

// EventTypeRequested is the outbox event (and Kafka record) for a queued bid.
const EventTypeRequested = "bid.requested"

type Status string

const (
	StatusPending  Status = "PENDING"
	StatusAccepted Status = "ACCEPTED"
	StatusRejected Status = "REJECTED" // a business rule said no: too low, ended, no funds
	StatusFailed   Status = "FAILED"   // a technical failure that retries couldn't fix
)

var (
	ErrNotFound      = errors.New("bid request not found")
	ErrInvalidAmount = errors.New("bid amount must be positive")
)

// Request is one queued bid and, once processed, its outcome.
type Request struct {
	ID             uuid.UUID
	AuctionID      uuid.UUID
	UserID         uuid.UUID
	Amount         int64
	IdempotencyKey *string
	Status         Status
	Reason         *string
	BidID          *uuid.UUID
	MinimumAmount  *int64
	CreatedAt      time.Time
	ProcessedAt    *time.Time
}

// Command is the Kafka record value: everything a worker needs.
type Command struct {
	RequestID uuid.UUID `json:"request_id"`
	AuctionID uuid.UUID `json:"auction_id"`
	UserID    uuid.UUID `json:"user_id"`
	Amount    int64     `json:"amount"`
}

type Requests struct {
	db database.DB
}

func NewRequests(db database.DB) *Requests {
	return &Requests{db: db}
}

// Submit records a bid request and queues it, in one transaction.
//
// With an idempotency key, a retry returns the existing request (replayed =
// true) instead of queueing the bid twice.
func (r *Requests) Submit(ctx context.Context, auctionID, userID uuid.UUID, amount int64, idempotencyKey string) (req Request, replayed bool, err error) {
	if amount <= 0 {
		return Request{}, false, ErrInvalidAmount
	}

	var key *string
	if idempotencyKey != "" {
		key = &idempotencyKey
	}

	req = Request{ID: uuid.New(), AuctionID: auctionID, UserID: userID, Amount: amount, IdempotencyKey: key, Status: StatusPending}

	err = database.WithTx(ctx, r.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO bid_requests (id, auction_id, user_id, amount, idempotency_key)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING created_at
		`, req.ID, auctionID, userID, amount, key).Scan(&req.CreatedAt)
		if err != nil {
			return err
		}

		return outbox.Enqueue(ctx, tx, EventTypeRequested, Command{
			RequestID: req.ID, AuctionID: auctionID, UserID: userID, Amount: amount,
		})
	})

	var pgErr *pgconn.PgError
	switch {
	case err == nil:
		return req, false, nil
	case errors.As(err, &pgErr) && pgErr.Code == "23505" && key != nil:
		// Same user, same key: return the original request.
		existing, getErr := r.byKey(ctx, userID, *key)
		if getErr != nil {
			return Request{}, false, getErr
		}
		return existing, true, nil
	case errors.As(err, &pgErr) && pgErr.Code == "23503":
		// Foreign key violation: the auction doesn't exist.
		return Request{}, false, ErrAuctionNotFound
	default:
		return Request{}, false, fmt.Errorf("submit bid request: %w", err)
	}
}

// ErrAuctionNotFound is returned when the auction ID doesn't exist.
var ErrAuctionNotFound = errors.New("auction not found")

const selectRequest = `
	SELECT id, auction_id, user_id, amount, idempotency_key, status, reason,
	       bid_id, minimum_amount, created_at, processed_at
	FROM bid_requests
`

func scanRequest(row pgx.Row) (Request, error) {
	var r Request
	err := row.Scan(&r.ID, &r.AuctionID, &r.UserID, &r.Amount, &r.IdempotencyKey, &r.Status,
		&r.Reason, &r.BidID, &r.MinimumAmount, &r.CreatedAt, &r.ProcessedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrNotFound
	}
	return r, err
}

// Get returns a request, but only to the user who made it. Anyone else gets
// ErrNotFound (not "forbidden"), so request IDs can't be probed.
func (r *Requests) Get(ctx context.Context, id, userID uuid.UUID) (Request, error) {
	return scanRequest(r.db.QueryRow(ctx, selectRequest+` WHERE id = $1 AND user_id = $2`, id, userID))
}

func (r *Requests) byKey(ctx context.Context, userID uuid.UUID, key string) (Request, error) {
	return scanRequest(r.db.QueryRow(ctx, selectRequest+` WHERE user_id = $1 AND idempotency_key = $2`, userID, key))
}

// Outcome is what a worker records after processing.
type Outcome struct {
	Status        Status
	Reason        string
	BidID         *uuid.UUID
	MinimumAmount *int64
}

// Record stores the outcome. "WHERE status = 'PENDING'" makes it write-once:
// a redelivered command that's processed again can't change a final answer.
func (r *Requests) Record(ctx context.Context, id uuid.UUID, o Outcome) error {
	var reason *string
	if o.Reason != "" {
		reason = &o.Reason
	}
	_, err := r.db.Exec(ctx, `
		UPDATE bid_requests
		SET status = $2, reason = $3, bid_id = $4, minimum_amount = $5, processed_at = now()
		WHERE id = $1 AND status = 'PENDING'
	`, id, o.Status, reason, o.BidID, o.MinimumAmount)
	if err != nil {
		return fmt.Errorf("record bid request outcome: %w", err)
	}
	return nil
}
