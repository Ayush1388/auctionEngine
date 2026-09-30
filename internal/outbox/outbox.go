// Package outbox implements the transactional outbox pattern.
//
// Problem: a single action often has to change the database AND tell
// another system (send an email, publish to Kafka). Those are two systems
// with no shared transaction, so either one can succeed while the other
// fails: a user without their activation email, or an email for a user
// whose insert rolled back.
//
// Fix: write the message into the outbox_events table in the SAME
// transaction as the change (Enqueue). Either both commit or neither does.
// A background Worker then reads pending events and delivers them,
// retrying until they succeed. Delivery is therefore at-least-once, and
// handlers must tolerate seeing an event twice.
package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ayush1388/auctionEngine/internal/database"
)

// Event is one row of outbox_events as the worker sees it.
type Event struct {
	ID          uuid.UUID
	EventType   string
	Payload     json.RawMessage
	Attempts    int
	AvailableAt time.Time
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db: db,
	}
}

// Claim locks up to limit pending events for this worker and returns them.
//
// FOR UPDATE SKIP LOCKED is what makes many workers safe: each one locks
// different rows, and rows another worker already locked are skipped
// instead of waited on. locked_at is a lease: if a worker dies holding
// events, another worker takes them over once the lease is 5 minutes old.
// attempts is incremented here, at claim time, so a crash still counts.
func (r *Repository) Claim(
	ctx context.Context,
	limit int,
) ([]Event, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to begin outbox claim transaction: %w",
			err,
		)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(
		ctx,
		`
		WITH candidates AS (
			SELECT id
			FROM outbox_events
			WHERE processed_at IS NULL
			  AND failed_at IS NULL
			  AND available_at <= now()
			  AND (
				locked_at IS NULL
				OR locked_at < now() - INTERVAL '5 minutes'
			  )
			ORDER BY created_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE outbox_events e
		SET
			locked_at = now(),
			attempts = e.attempts + 1
		FROM candidates
		WHERE e.id = candidates.id
		RETURNING
			e.id,
			e.event_type,
			e.payload,
			e.attempts,
			e.available_at
		`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to claim outbox events: %w",
			err,
		)
	}
	defer rows.Close()

	var events []Event

	for rows.Next() {
		var event Event

		if err := rows.Scan(
			&event.ID,
			&event.EventType,
			&event.Payload,
			&event.Attempts,
			&event.AvailableAt,
		); err != nil {
			return nil, fmt.Errorf(
				"failed to scan outbox event: %w",
				err,
			)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"failed to iterate outbox events: %w",
			err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf(
			"failed to commit outbox claim transaction: %w",
			err,
		)
	}

	return events, nil
}

// MarkProcessed records that an event was handled. Any redactKeys are
// removed from the stored payload, so secrets such as activation tokens
// don't sit in the table after they've been delivered.
func (r *Repository) MarkProcessed(
	ctx context.Context,
	id uuid.UUID,
	redactKeys []string,
) error {
	if redactKeys == nil {
		redactKeys = []string{}
	}

	_, err := r.db.Exec(
		ctx,
		`
		UPDATE outbox_events
		SET
			processed_at = now(),
			locked_at = NULL,
			last_error = NULL,
			payload = payload - $2::text[]
		WHERE id = $1
		`,
		id,
		redactKeys,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to mark outbox event processed: %w",
			err,
		)
	}

	return nil
}

// Release unlocks a claimed event that was never attempted, and undoes the
// attempt counted when it was claimed.
func (r *Repository) Release(
	ctx context.Context,
	id uuid.UUID,
) error {
	_, err := r.db.Exec(
		ctx,
		`
		UPDATE outbox_events
		SET
			locked_at = NULL,
			attempts = GREATEST(attempts - 1, 0)
		WHERE id = $1
		  AND processed_at IS NULL
		`,
		id,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to release outbox event: %w",
			err,
		)
	}

	return nil
}

// MaxAttempts is how many times an event is tried before it is parked
// with failed_at set. With exponential backoff from 30 seconds, eight
// attempts span roughly an hour.
const MaxAttempts = 8

// Retry schedules another attempt with exponential backoff (30s, 1m, 2m,
// ... capped at 1h), or dead-letters the event once it has used all its
// attempts. It reports whether the event was dead-lettered.
func (r *Repository) Retry(
	ctx context.Context,
	id uuid.UUID,
	processingErr error,
) (bool, error) {
	var deadLettered bool

	// attempts was already incremented when the event was claimed.
	err := r.db.QueryRow(
		ctx,
		`
		UPDATE outbox_events
		SET
			locked_at = NULL,
			last_error = $1,
			failed_at = CASE WHEN attempts >= $3 THEN now() END,
			available_at = now() + LEAST(
				INTERVAL '30 seconds' * power(2, GREATEST(attempts - 1, 0)),
				INTERVAL '1 hour'
			)
		WHERE id = $2
		RETURNING failed_at IS NOT NULL
		`,
		processingErr.Error(),
		id,
		MaxAttempts,
	).Scan(&deadLettered)
	if err != nil {
		return false, fmt.Errorf(
			"failed to schedule outbox retry: %w",
			err,
		)
	}

	return deadLettered, nil
}

// Enqueue adds an event to the outbox. Pass the transaction that makes the
// change the event describes, so both are committed or neither is.
func Enqueue(
	ctx context.Context,
	db database.DBTX,
	eventType string,
	payload any,
) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf(
			"failed to marshal outbox payload: %w",
			err,
		)
	}

	_, err = db.Exec(
		ctx,
		`
		INSERT INTO outbox_events (
			id,
			event_type,
			payload
		)
		VALUES ($1, $2, $3)
		`,
		uuid.New(),
		eventType,
		payloadBytes,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to enqueue %s event: %w",
			eventType,
			err,
		)
	}

	return nil
}

func DecodePayload(
	event Event,
	target any,
) error {
	if err := json.Unmarshal(event.Payload, target); err != nil {
		return fmt.Errorf(
			"failed to decode outbox event payload: %w",
			err,
		)
	}

	return nil
}
