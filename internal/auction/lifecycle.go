package auction

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ayush1388/auctionEngine/internal/database"
	"github.com/Ayush1388/auctionEngine/internal/outbox"
)

// LifecycleWorker moves auctions along on schedule:
//
//	NOT_ACTIVE ──(starts_at passed)──► ACTIVE ──(ends_at passed)──► COMPLETED
//
// Several copies can run at once (one per API instance). Each batch is
// claimed with FOR UPDATE SKIP LOCKED, so two workers never move the same
// auction; see docs/decisions/0003.
type LifecycleWorker struct {
	db        database.TxBeginner
	logger    *slog.Logger
	interval  time.Duration
	batchSize int
	now       func() time.Time
}

func NewLifecycleWorker(
	db database.TxBeginner,
	logger *slog.Logger,
) *LifecycleWorker {
	return &LifecycleWorker{
		db:        db,
		logger:    logger,
		interval:  time.Second,
		batchSize: 100,
		now:       time.Now,
	}
}

// SetClock and SetBatchSize exist for tests.
func (w *LifecycleWorker) SetClock(now func() time.Time) { w.now = now }
func (w *LifecycleWorker) SetBatchSize(n int)            { w.batchSize = n }

// Run ticks until ctx is cancelled. When a batch comes back full, it goes
// again immediately instead of waiting for the next tick.
func (w *LifecycleWorker) Run(ctx context.Context) {
	w.logger.Info("auction lifecycle worker started")

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		result, err := w.Tick(ctx)
		if err != nil && ctx.Err() == nil {
			w.logger.Error("auction lifecycle tick failed", "error", err)
		}

		if result.Activated > 0 || result.Completed > 0 {
			w.logger.Info(
				"auctions moved",
				"activated", result.Activated,
				"completed", result.Completed,
			)
		}

		if err == nil && result.Full && ctx.Err() == nil {
			continue
		}

		select {
		case <-ctx.Done():
			w.logger.Info("auction lifecycle worker stopped")
			return
		case <-ticker.C:
		}
	}
}

type TickResult struct {
	Activated int
	Completed int

	// Full is true when either batch hit the batch size, meaning more
	// auctions are probably waiting.
	Full bool
}

// Tick runs one activation batch and one completion batch. Activation goes
// first, so an auction whose whole window passed while the service was
// down is activated and completed in the same tick.
func (w *LifecycleWorker) Tick(ctx context.Context) (TickResult, error) {
	now := w.now()
	var result TickResult

	activated, err := w.activate(ctx, now)
	if err != nil {
		return result, err
	}
	result.Activated = activated

	completed, err := w.complete(ctx, now)
	if err != nil {
		return result, err
	}
	result.Completed = completed

	result.Full = activated == w.batchSize || completed == w.batchSize
	return result, nil
}

func (w *LifecycleWorker) activate(ctx context.Context, now time.Time) (int, error) {
	var n int

	err := database.WithTx(ctx, w.db, func(tx pgx.Tx) error {
		ids, err := claimAndMove(ctx, tx, StatusActive, "starts_at", now, w.batchSize)
		if err != nil {
			return err
		}
		n = len(ids)

		for _, moved := range ids {
			if err := outbox.Enqueue(ctx, tx, EventTypeActivated, StatusChangedEvent{
				AuctionID: moved.ID,
				Status:    StatusActive,
				ChangedAt: now.UTC(),
			}); err != nil {
				return err
			}
		}
		return nil
	})

	return n, err
}

func (w *LifecycleWorker) complete(ctx context.Context, now time.Time) (int, error) {
	var n int

	// The status change and the outbox event commit together: an auction
	// is never COMPLETED without the event that settlement will rely on.
	err := database.WithTx(ctx, w.db, func(tx pgx.Tx) error {
		ids, err := claimAndMove(ctx, tx, StatusCompleted, "ends_at", now, w.batchSize)
		if err != nil {
			return err
		}
		n = len(ids)

		for _, moved := range ids {
			if err := outbox.Enqueue(ctx, tx, EventTypeCompleted, CompletedEvent{
				AuctionID:   moved.ID,
				OwnerID:     moved.OwnerID,
				WinningBid:  moved.CurrentBid,
				CompletedAt: now.UTC(),
			}); err != nil {
				return err
			}
		}

		return nil
	})

	return n, err
}

type movedAuction struct {
	ID         uuid.UUID
	OwnerID    uuid.UUID
	CurrentBid *int64
}

// claimAndMove moves up to limit auctions whose timeColumn has passed into
// target, taking only rows no other worker has locked.
func claimAndMove(
	ctx context.Context,
	tx pgx.Tx,
	target Status,
	timeColumn string,
	now time.Time,
	limit int,
) ([]movedAuction, error) {
	// timeColumn is one of two constants chosen by this package, never
	// user input, so formatting it into the SQL is safe.
	query := fmt.Sprintf(`
		WITH due AS (
			SELECT id
			FROM auctions
			WHERE status = ANY($1)
			  AND %[1]s <= $2
			ORDER BY %[1]s
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		UPDATE auctions a
		SET
			status = $4,
			updated_at = now(),
			-- Bump the version so an optimistic bid that read the auction
			-- while it was ACTIVE fails its compare-and-swap and retries.
			version = a.version + 1
		FROM due
		WHERE a.id = due.id
		RETURNING a.id, a.owner_id, a.current_bid
	`, timeColumn)

	rows, err := tx.Query(ctx, query, statusStrings(SourcesOf(target)), now, limit, target)
	if err != nil {
		return nil, fmt.Errorf("failed to move auctions to %s: %w", target, err)
	}

	moved, err := pgx.CollectRows(rows, pgx.RowToStructByPos[movedAuction])
	if err != nil {
		return nil, fmt.Errorf("failed to move auctions to %s: %w", target, err)
	}

	return moved, nil
}
