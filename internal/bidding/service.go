package bidding

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/database"
	"github.com/Ayush1388/auctionEngine/internal/metrics"
	"github.com/Ayush1388/auctionEngine/internal/outbox"
	"github.com/Ayush1388/auctionEngine/internal/telemetry"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

// Strategy selects how concurrent bids on one auction are serialised.
//
// Pessimistic (the default) locks the auction row with SELECT … FOR UPDATE
// before reading it. Competing bids queue on the lock and each one sees the
// result of the one before it. Simple and predictable; throughput per
// auction is one bid per transaction round trip.
//
// Optimistic reads the auction without a lock, decides, then writes with
// UPDATE … WHERE version = <what I read>. If someone else bid in between,
// the version moved, the update matches no row, and the whole transaction
// is retried with fresh data. No waiting on locks while deciding, but under
// heavy contention most attempts are wasted work.
//
// Both give the same results; BenchmarkPlaceBid compares their cost.
type Strategy string

const (
	Pessimistic Strategy = "pessimistic"
	Optimistic  Strategy = "optimistic"

	// maxOptimisticAttempts bounds retries so a hot auction can't make a
	// request loop forever.
	maxOptimisticAttempts = 8
)

// errVersionConflict means an optimistic attempt lost the race.
var errVersionConflict = errors.New("auction changed since it was read")

// errDuplicateKey means another request with the same idempotency key
// committed first (the unique index caught it).
var errDuplicateKey = errors.New("idempotency key already used")

type Service struct {
	db       database.DB
	strategy Strategy
	now      func() time.Time
}

func NewService(db database.DB, strategy Strategy) *Service {
	if strategy == "" {
		strategy = Pessimistic
	}
	return &Service{db: db, strategy: strategy, now: time.Now}
}

// SetClock replaces the clock; tests use it.
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// PlaceBid places a bid, or returns the original result when the same
// idempotency key was used before.
func (s *Service) PlaceBid(ctx context.Context, in PlaceBidInput) (res Result, err error) {
	// Observability (v1.0): a span per bid (its SQL statements become child
	// spans through the pgx tracer) and the bid outcome/latency metrics.
	ctx, span := telemetry.Tracer().Start(ctx, "bidding.PlaceBid", trace.WithAttributes(
		attribute.String("auction.id", in.AuctionID.String()),
		attribute.String("bidding.strategy", string(s.strategy)),
	))
	start := time.Now()
	defer func() {
		result := Outcome(res, err)
		span.SetAttributes(attribute.String("bidding.result", result))
		if result == "error" {
			telemetry.RecordError(span, err)
		}
		span.End()
		metrics.Bids.WithLabelValues(result).Inc()
		metrics.BidDuration.WithLabelValues(string(s.strategy)).Observe(time.Since(start).Seconds())
	}()

	return s.placeBid(ctx, in)
}

// Outcome names a PlaceBid result for metrics and traces. It maps every
// error to one of a FIXED set of strings: label values must be bounded, so
// an error message (which contains amounts) must never become a label.
func Outcome(res Result, err error) string {
	var tooLow *BidTooLowError
	switch {
	case err == nil && res.Replayed:
		return "replayed"
	case err == nil:
		return "accepted"
	case errors.As(err, &tooLow):
		return "too_low"
	case errors.Is(err, wallet.ErrInsufficientFunds):
		return "insufficient_funds"
	case errors.Is(err, ErrAuctionNotActive):
		return "not_active"
	case errors.Is(err, ErrAuctionEnded):
		return "ended"
	case errors.Is(err, ErrOwnAuction):
		return "own_auction"
	case errors.Is(err, ErrContention):
		return "contention"
	case errors.Is(err, auction.ErrNotFound):
		return "not_found"
	case errors.Is(err, ErrInvalidAmount), errors.Is(err, ErrInvalidKey), errors.Is(err, ErrIdempotencyMismatch):
		return "invalid"
	default:
		return "error"
	}
}

func (s *Service) placeBid(ctx context.Context, in PlaceBidInput) (Result, error) {
	if len(in.IdempotencyKey) > MaxIdempotencyKeyLength {
		return Result{}, ErrInvalidKey
	}

	var (
		res Result
		err error
	)

	switch s.strategy {
	case Optimistic:
		res, err = s.placeOptimistic(ctx, in)
	default:
		res, err = s.placePessimistic(ctx, in)
	}

	// Two identical requests raced and the other one won the unique index.
	// Our transaction rolled back; return what the winner created.
	if errors.Is(err, errDuplicateKey) {
		return s.replay(ctx, s.db, in)
	}

	return res, err
}

// placePessimistic: lock, check, write.
func (s *Service) placePessimistic(ctx context.Context, in PlaceBidInput) (Result, error) {
	var res Result

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		// 1. Lock the auction row. Every other bid on this auction now
		//    waits here until we commit or roll back.
		a, err := getAuction(ctx, tx, in.AuctionID, true)
		if err != nil {
			return err
		}

		// 2. Idempotency check *after* the lock: a concurrent duplicate of
		//    this request that got the lock first has committed by now,
		//    so we see its bid and replay it instead of bidding twice.
		if in.IdempotencyKey != "" {
			replayed, found, err := s.findReplay(ctx, tx, in, a)
			if err != nil || found {
				res = replayed
				return err
			}
		}

		// 3. Decide on data nobody else can change until we commit.
		p, err := decide(a, in.UserID, in.Amount, s.now())
		if err != nil {
			return err
		}

		// 4. Write everything.
		res, err = apply(ctx, tx, a, in, p, s.now(), false)
		return err
	})

	return res, err
}

// placeOptimistic: read, check, compare-and-swap, retry on conflict.
func (s *Service) placeOptimistic(ctx context.Context, in PlaceBidInput) (Result, error) {
	for attempt := 1; attempt <= maxOptimisticAttempts; attempt++ {
		var res Result

		err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
			a, err := getAuction(ctx, tx, in.AuctionID, false)
			if err != nil {
				return err
			}

			if in.IdempotencyKey != "" {
				replayed, found, err := s.findReplay(ctx, tx, in, a)
				if err != nil || found {
					res = replayed
					return err
				}
			}

			p, err := decide(a, in.UserID, in.Amount, s.now())
			if err != nil {
				return err
			}

			res, err = apply(ctx, tx, a, in, p, s.now(), true)
			return err
		})

		if !errors.Is(err, errVersionConflict) {
			return res, err
		}

		// Lost the race. Back off a little, with jitter so the losers
		// don't all retry at the same instant and collide again.
		backoff := time.Duration(attempt*attempt) * time.Millisecond
		jitter := time.Duration(rand.Int64N(int64(backoff) + 1))
		select {
		case <-time.After(backoff + jitter):
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
	}

	return Result{}, ErrContention
}

// apply performs every write of an accepted bid inside tx. With cas set,
// the auction update only succeeds if the version is still the one read.
//
// Lock order is always: auction row first (above, or in the UPDATE below),
// then wallet rows in user-ID order. Settlement uses the same order, and the
// lifecycle worker only locks auctions, so no two code paths can wait on
// each other in a cycle.
func apply(
	ctx context.Context,
	tx pgx.Tx,
	a auction.Auction,
	in PlaceBidInput,
	p plan,
	now time.Time,
	cas bool,
) (Result, error) {
	bidID := uuid.New()

	// Auction first. In pessimistic mode we already hold its lock and the
	// version can't have moved; the predicate is only decisive for cas.
	var (
		endsAt   time.Time
		bidCount int
	)
	err := tx.QueryRow(ctx, `
		UPDATE auctions
		SET current_bid = $2,
		    current_bidder_id = $3,
		    bid_count = bid_count + 1,
		    ends_at = $4,
		    extensions = $5,
		    version = version + 1,
		    updated_at = now()
		WHERE id = $1 AND version = $6 AND status = 'ACTIVE'
		RETURNING ends_at, bid_count
	`, a.ID, in.Amount, in.UserID, p.NewEndsAt, p.NewExtensions, a.Version).Scan(&endsAt, &bidCount)
	if errors.Is(err, pgx.ErrNoRows) {
		if cas {
			return Result{}, errVersionConflict
		}
		// Impossible while holding the row lock; treat as a bug.
		return Result{}, fmt.Errorf("auction %s changed while locked", a.ID)
	}
	if err != nil {
		return Result{}, fmt.Errorf("update auction: %w", err)
	}

	// Wallets second, in a fixed order.
	lockIDs := []uuid.UUID{in.UserID}
	if p.Release != nil {
		lockIDs = append(lockIDs, p.Release.UserID)
	}
	wallets, err := wallet.LockForUpdate(ctx, tx, lockIDs...)
	if err != nil {
		return Result{}, err
	}

	// Check funds on the locked row, so nobody can spend the same money
	// in parallel. (The CHECK >= 0 on wallets would also catch it, but a
	// constraint violation aborts the transaction with a less useful error.)
	if wallets[in.UserID].Available < p.Reserve {
		return Result{}, wallet.ErrInsufficientFunds
	}

	// The bid itself.
	var key *string
	if in.IdempotencyKey != "" {
		key = &in.IdempotencyKey
	}
	// created_at is clock_timestamp(), not the column default now(): now() is the
	// time the TRANSACTION started, so a bid that queued behind others for the
	// auction row would keep a timestamp from before it won the lock, and the
	// history (ordered by created_at) could list a lower bid after a higher one.
	// By the time this runs the row lock is held, so these timestamps follow the
	// order bids were accepted.
	var createdAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO bids (id, auction_id, user_id, amount, idempotency_key, created_at)
		VALUES ($1, $2, $3, $4, $5, clock_timestamp())
		RETURNING created_at
	`, bidID, a.ID, in.UserID, in.Amount, key).Scan(&createdAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "bids_idempotency_idx" {
			return Result{}, errDuplicateKey
		}
		return Result{}, fmt.Errorf("insert bid: %w", err)
	}

	auctionID := a.ID

	// Hand the previous leader's money back.
	if p.Release != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE bid_reservations
			SET status = 'RELEASED', closed_at = now()
			WHERE auction_id = $1 AND status = 'ACTIVE'
		`, a.ID); err != nil {
			return Result{}, fmt.Errorf("release reservation: %w", err)
		}

		if _, err := wallet.Post(ctx, tx, wallet.Journal{
			Kind:      wallet.KindRelease,
			AuctionID: &auctionID,
			BidID:     &bidID,
			Lines: []wallet.Line{
				{UserID: p.Release.UserID, Account: wallet.Reserved, Amount: -p.Release.Amount},
				{UserID: p.Release.UserID, Account: wallet.Available, Amount: p.Release.Amount},
			},
		}); err != nil {
			return Result{}, err
		}
	}

	// Hold the new leader's money. When the leader raises their own bid the
	// existing reservation grows; otherwise a new one starts. The unique
	// index "one ACTIVE reservation per auction" backs this up.
	if p.Release == nil && a.CurrentBidderID != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE bid_reservations
			SET amount = $2, bid_id = $3
			WHERE auction_id = $1 AND status = 'ACTIVE'
		`, a.ID, in.Amount, bidID); err != nil {
			return Result{}, fmt.Errorf("grow reservation: %w", err)
		}
	} else {
		if _, err := tx.Exec(ctx, `
			INSERT INTO bid_reservations (id, auction_id, user_id, bid_id, amount, status)
			VALUES ($1, $2, $3, $4, $5, 'ACTIVE')
		`, uuid.New(), a.ID, in.UserID, bidID, in.Amount); err != nil {
			return Result{}, fmt.Errorf("create reservation: %w", err)
		}
	}

	if _, err := wallet.Post(ctx, tx, wallet.Journal{
		Kind:      wallet.KindReserve,
		AuctionID: &auctionID,
		BidID:     &bidID,
		Lines: []wallet.Line{
			{UserID: in.UserID, Account: wallet.Available, Amount: -p.Reserve},
			{UserID: in.UserID, Account: wallet.Reserved, Amount: p.Reserve},
		},
	}); err != nil {
		return Result{}, err
	}

	// Tell the rest of the system (WebSockets in v0.7, Kafka in v0.8), in
	// the same transaction, so the event exists if and only if the bid does.
	event := PlacedEvent{
		AuctionID: a.ID,
		BidID:     bidID,
		BidderID:  in.UserID,
		Amount:    in.Amount,
		BidCount:  bidCount,
		EndsAt:    endsAt,
		Extended:  p.Extended,
		PlacedAt:  createdAt,
	}
	if p.Release != nil {
		event.PreviousBidderID = &p.Release.UserID
	}
	if err := outbox.Enqueue(ctx, tx, EventTypePlaced, event); err != nil {
		return Result{}, err
	}

	return Result{
		Bid: Bid{
			ID:        bidID,
			AuctionID: a.ID,
			UserID:    in.UserID,
			Amount:    in.Amount,
			CreatedAt: createdAt,
		},
		EndsAt:     endsAt,
		Extended:   p.Extended,
		CurrentBid: in.Amount,
		BidCount:   bidCount,
	}, nil
}

// findReplay looks for an earlier bid by this user with the same key.
func (s *Service) findReplay(ctx context.Context, db database.DBTX, in PlaceBidInput, a auction.Auction) (Result, bool, error) {
	var b Bid
	err := db.QueryRow(ctx, `
		SELECT id, auction_id, user_id, amount, created_at
		FROM bids
		WHERE user_id = $1 AND idempotency_key = $2
	`, in.UserID, in.IdempotencyKey).Scan(&b.ID, &b.AuctionID, &b.UserID, &b.Amount, &b.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, false, nil
	}
	if err != nil {
		return Result{}, false, fmt.Errorf("find idempotent bid: %w", err)
	}

	if b.AuctionID != in.AuctionID || b.Amount != in.Amount {
		return Result{}, true, ErrIdempotencyMismatch
	}

	current := int64(0)
	if a.CurrentBid != nil {
		current = *a.CurrentBid
	}

	return Result{
		Bid:        b,
		Replayed:   true,
		EndsAt:     a.EndsAt,
		CurrentBid: current,
		BidCount:   a.BidCount,
	}, true, nil
}

func (s *Service) replay(ctx context.Context, db database.DBTX, in PlaceBidInput) (Result, error) {
	a, err := getAuction(ctx, db, in.AuctionID, false)
	if err != nil {
		return Result{}, err
	}
	res, found, err := s.findReplay(ctx, db, in, a)
	if err != nil {
		return Result{}, err
	}
	if !found {
		return Result{}, fmt.Errorf("idempotent bid for key vanished")
	}
	return res, nil
}

// getAuction reads the auction, optionally locking its row. FOR UPDATE OF a
// locks only the auctions row, not the joined items row.
func getAuction(ctx context.Context, db database.DBTX, id uuid.UUID, lock bool) (auction.Auction, error) {
	query := auction.SelectAuction + ` WHERE a.id = $1`
	if lock {
		query += ` FOR UPDATE OF a`
	}

	a, err := auction.ScanAuction(db.QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return auction.Auction{}, auction.ErrNotFound
	}
	if err != nil {
		return auction.Auction{}, fmt.Errorf("get auction: %w", err)
	}
	return a, nil
}
