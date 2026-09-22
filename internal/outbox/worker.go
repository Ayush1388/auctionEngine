package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

type Handler interface {
	Handle(
		ctx context.Context,
		event Event,
	) error
}

type Worker struct {
	repository *Repository
	handler    Handler
	logger     *slog.Logger
	interval   time.Duration
	batchSize  int
}

func NewWorker(
	repository *Repository,
	handler Handler,
	logger *slog.Logger,
) *Worker {
	return &Worker{
		repository: repository,
		handler:    handler,
		logger:     logger,
		interval:   2 * time.Second,
		batchSize:  10,
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		if err := w.process(ctx); err != nil {
			w.logger.Error(
				"outbox worker failed",
				"error", err,
			)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) process(ctx context.Context) error {
	events, err := w.repository.Claim(
		ctx,
		w.batchSize,
	)
	if err != nil {
		return err
	}

	for _, event := range events {
		if err := w.handler.Handle(ctx, event); err != nil {
			w.logger.Error(
				"failed to process outbox event",
				"event_id", event.ID,
				"event_type", event.EventType,
				"error", err,
			)

			if retryErr := w.repository.Retry(
				ctx,
				event.ID,
				err,
			); retryErr != nil {
				w.logger.Error(
					"failed to schedule outbox retry",
					"event_id", event.ID,
					"error", retryErr,
				)
			}

			continue
		}

		if err := w.repository.MarkProcessed(
			ctx,
			event.ID,
		); err != nil {
			w.logger.Error(
				"failed to mark outbox event processed",
				"event_id", event.ID,
				"error", err,
			)
		}
	}

	return nil
}

func (r *Repository) Claim(
	ctx context.Context,
	limit int,
) ([]Event, error) {
	rows, err := r.db.Query(
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
		return nil, fmt.Errorf("failed to claim outbox events: %w", err)
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
	delay := 30 * time.Second

	_, err := r.db.Exec(
		ctx,
		`
		UPDATE outbox_events
		SET
			locked_at = NULL,
			available_at = now() + $1::interval,
			last_error = $2
		WHERE id = $3
		`,
		delay.String(),
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
