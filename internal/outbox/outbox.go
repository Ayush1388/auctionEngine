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
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ayush1388/auctionEngine/internal/database"
	"github.com/Ayush1388/auctionEngine/internal/telemetry"
)

// Event is one row of outbox_events as the worker sees it.
type Event struct {
	ID          uuid.UUID
	EventType   string
	Payload     json.RawMessage
	Attempts    int
	AvailableAt time.Time

	// TraceContext is the W3C trace context of the request that enqueued
	// the event (v1.0), so its processing joins the same trace.
	TraceContext map[string]string
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
			e.available_at,
			e.trace_context
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
			&event.TraceContext,
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

// MarkProcessedMany records several handled events in one statement. Use
// MarkProcessed for events whose payload needs redacting.
func (r *Repository) MarkProcessedMany(ctx context.Context, ids []uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE outbox_events
		SET processed_at = now(), locked_at = NULL, last_error = NULL
		WHERE id = ANY($1)
	`, ids)
	if err != nil {
		return fmt.Errorf("failed to mark outbox events processed: %w", err)
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
//
// The current trace context (if ctx carries a span) is stored with the
// event, so the worker that handles it continues the same trace.
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
			payload,
			trace_context
		)
		VALUES ($1, $2, $3, $4)
		`,
		uuid.New(),
		eventType,
		payloadBytes,
		telemetry.Inject(ctx), // nil map → SQL NULL
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

// ErrNotFailed is returned by RetryFailed for an event that isn't
// dead-lettered (unknown, pending, or already processed).
var ErrNotFailed = errors.New("event is not dead-lettered")

// FailedEvent is a dead-lettered event as shown to operators.
type FailedEvent struct {
	ID        uuid.UUID `json:"id"`
	EventType string    `json:"event_type"`
	Attempts  int       `json:"attempts"`
	LastError *string   `json:"last_error"`
	FailedAt  time.Time `json:"failed_at"`
	CreatedAt time.Time `json:"created_at"`
}

// ListFailed returns the most recently dead-lettered events.
func (r *Repository) ListFailed(ctx context.Context, limit int) ([]FailedEvent, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, event_type, attempts, last_error, failed_at, created_at
		FROM outbox_events
		WHERE failed_at IS NOT NULL AND processed_at IS NULL
		ORDER BY failed_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list failed events: %w", err)
	}
	events, err := pgx.CollectRows(rows, pgx.RowToStructByPos[FailedEvent])
	if err != nil {
		return nil, fmt.Errorf("list failed events: %w", err)
	}
	if events == nil {
		events = []FailedEvent{}
	}
	return events, nil
}

// RetryFailed puts a dead-lettered event back into the queue with a fresh
// set of attempts.
func (r *Repository) RetryFailed(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.Exec(ctx, `
		UPDATE outbox_events
		SET failed_at = NULL, attempts = 0, available_at = now(), locked_at = NULL
		WHERE id = $1 AND failed_at IS NOT NULL AND processed_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("retry failed event: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFailed
	}
	return nil
}

// Backlog counts events still waiting to be delivered and events parked as
// dead letters (v1.0 metrics). A growing pending count means the worker
// can't keep up or a consumer is failing; any dead letter needs a human.
// Both counts use the partial indexes on outbox_events.
func (r *Repository) Backlog(ctx context.Context) (pending, failed int64, err error) {
	err = r.db.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE failed_at IS NULL),
			count(*) FILTER (WHERE failed_at IS NOT NULL)
		FROM outbox_events
		WHERE processed_at IS NULL
	`).Scan(&pending, &failed)
	return pending, failed, err
}
