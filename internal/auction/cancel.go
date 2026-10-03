package auction

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ayush1388/auctionEngine/internal/database"
	"github.com/Ayush1388/auctionEngine/internal/outbox"
)

var (
	// ErrForbidden means the caller is authenticated but may not do this.
	ErrForbidden = errors.New("not allowed to modify this auction")

	// ErrAlreadyStarted means the start time has passed, even if the
	// lifecycle worker hasn't marked the auction ACTIVE yet.
	ErrAlreadyStarted = errors.New("auction has already started")
)

// Cancel cancels an auction that hasn't started. Only its owner may do this.
//
// The check and the change happen in one UPDATE ... WHERE, not a SELECT
// followed by an UPDATE. With check-then-act, the lifecycle worker could
// activate the auction between the two statements and we would cancel a
// running auction. Here Postgres evaluates the conditions on the locked
// row, so exactly one of two racing updates wins.
func (s *Service) Cancel(
	ctx context.Context,
	userID uuid.UUID,
	id uuid.UUID,
) (Auction, error) {
	var cancelled Auction

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		repo := s.repository.WithTx(tx)

		ok, err := repo.Transition(ctx, id, StatusCancelled, TransitionGuard{
			OwnerID:      userID,
			StartsAfter:  s.now(),
			RequireOwner: true,
		})
		if err != nil {
			return err
		}

		if !ok {
			return s.cancelFailureReason(ctx, repo, userID, id)
		}

		cancelled, err = repo.GetByID(ctx, id)
		if err != nil {
			return err
		}

		return outbox.Enqueue(ctx, tx, EventTypeCancelled, StatusChangedEvent{
			AuctionID: id,
			Status:    StatusCancelled,
			ChangedAt: s.now().UTC(),
		})
	})
	if err != nil {
		return Auction{}, err
	}

	return cancelled, nil
}

// cancelFailureReason runs only after the guarded UPDATE matched nothing,
// to tell the client why. It decides nothing, so reading without a lock is
// fine.
func (s *Service) cancelFailureReason(
	ctx context.Context,
	repo *Repository,
	userID uuid.UUID,
	id uuid.UUID,
) error {
	current, err := repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	switch {
	case current.OwnerID != userID:
		return ErrForbidden
	case !current.Status.CanTransition(StatusCancelled):
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, StatusCancelled)
	default:
		return ErrAlreadyStarted
	}
}

// TransitionGuard adds conditions to a status change.
type TransitionGuard struct {
	RequireOwner bool
	OwnerID      uuid.UUID

	// StartsAfter, if set, only matches auctions starting after this time.
	StartsAfter time.Time
}

// Transition moves auction id to target if its current status allows it
// (according to the state machine) and the guard matches. It reports
// whether a row changed.
func (r *Repository) Transition(
	ctx context.Context,
	id uuid.UUID,
	target Status,
	guard TransitionGuard,
) (bool, error) {
	var startsAfter *time.Time
	if !guard.StartsAfter.IsZero() {
		startsAfter = &guard.StartsAfter
	}

	var ownerID *uuid.UUID
	if guard.RequireOwner {
		ownerID = &guard.OwnerID
	}

	result, err := r.db.Exec(
		ctx,
		`
		UPDATE auctions
		SET
			status = $2,
			updated_at = now(),
			version = version + 1
		WHERE
			id = $1
			AND status = ANY($3)
			AND ($4::uuid IS NULL OR owner_id = $4)
			AND ($5::timestamptz IS NULL OR starts_at > $5)
		`,
		id,
		target,
		statusStrings(SourcesOf(target)),
		ownerID,
		startsAfter,
	)
	if err != nil {
		return false, fmt.Errorf(
			"failed to move auction to %s: %w",
			target,
			err,
		)
	}

	return result.RowsAffected() == 1, nil
}

func statusStrings(statuses []Status) []string {
	out := make([]string, len(statuses))
	for i, s := range statuses {
		out[i] = string(s)
	}
	return out
}
