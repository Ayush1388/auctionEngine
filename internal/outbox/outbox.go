package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

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

func (r *Repository) MarkProcessed(
	ctx context.Context,
	id uuid.UUID,
) error {
	_, err := r.db.Exec(
		ctx,
		`
		UPDATE outbox_events
		SET
			processed_at = now(),
			locked_at = NULL,
			last_error = NULL
		WHERE id = $1
		`,
		id,
	)

	if err != nil {
		return fmt.Errorf(
			"failed to mark outbox event processed: %w",
			err,
		)
	}

	return nil
}

func (r *Repository) Retry(
	ctx context.Context,
	id uuid.UUID,
	processingErr error,
) error {
	_, err := r.db.Exec(
		ctx,
		`
		UPDATE outbox_events
		SET
			locked_at = NULL,
			available_at = now() + INTERVAL '30 seconds',
			last_error = $1
		WHERE id = $2
		`,
		processingErr.Error(),
		id,
	)

	if err != nil {
		return fmt.Errorf(
			"failed to schedule outbox retry: %w",
			err,
		)
	}

	return nil
}

func (r *Repository) Create(
	ctx context.Context,
	tx pgx.Tx,
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

	_, err = tx.Exec(
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
			"failed to create outbox event: %w",
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
