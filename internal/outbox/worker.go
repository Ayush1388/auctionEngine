package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"
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

	interval  time.Duration
	batchSize int
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
	w.logger.Info("outbox worker started")

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		if err := w.process(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}

			w.logger.Error(
				"outbox worker processing failed",
				"error", err,
			)
		}

		select {
		case <-ctx.Done():
			w.logger.Info("outbox worker stopped")
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

	if len(events) == 0 {
		return nil
	}

	for _, event := range events {
		if err := w.processEvent(ctx, event); err != nil {
			w.logger.Error(
				"failed to process outbox event",
				"event_id", event.ID,
				"event_type", event.EventType,
				"attempts", event.Attempts,
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

			continue
		}

		w.logger.Info(
			"outbox event processed",
			"event_id", event.ID,
			"event_type", event.EventType,
		)
	}

	return nil
}

func (w *Worker) processEvent(
	ctx context.Context,
	event Event,
) error {
	if err := w.handler.Handle(ctx, event); err != nil {
		return fmt.Errorf(
			"event handler failed: %w",
			err,
		)
	}

	return nil
}
