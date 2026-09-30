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

	for i, event := range events {
		// On shutdown, hand unstarted events back instead of leaving them
		// locked until the lease expires.
		if ctx.Err() != nil {
			w.release(events[i:])
			return nil
		}

		// Recording the outcome must survive shutdown: an email that was
		// sent should be marked sent even if ctx was cancelled meanwhile.
		finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		w.processOne(ctx, finishCtx, event)
		cancel()
	}

	return nil
}

func (w *Worker) processOne(
	ctx context.Context,
	finishCtx context.Context,
	event Event,
) {
	if err := w.processEvent(ctx, event); err != nil {
		w.logger.Error(
			"failed to process outbox event",
			"event_id", event.ID,
			"event_type", event.EventType,
			"attempts", event.Attempts,
			"error", err,
		)

		deadLettered, retryErr := w.repository.Retry(
			finishCtx,
			event.ID,
			err,
		)
		if retryErr != nil {
			w.logger.Error(
				"failed to schedule outbox retry",
				"event_id", event.ID,
				"error", retryErr,
			)
		}
		if deadLettered {
			w.logger.Error(
				"outbox event gave up after max attempts",
				"event_id", event.ID,
				"event_type", event.EventType,
				"attempts", event.Attempts,
			)
		}

		return
	}

	var redactKeys []string
	if r, ok := w.handler.(Redactor); ok {
		redactKeys = r.RedactKeys(event.EventType)
	}

	if err := w.repository.MarkProcessed(
		finishCtx,
		event.ID,
		redactKeys,
	); err != nil {
		w.logger.Error(
			"failed to mark outbox event processed",
			"event_id", event.ID,
			"error", err,
		)

		return
	}

	w.logger.Info(
		"outbox event processed",
		"event_id", event.ID,
		"event_type", event.EventType,
	)
}

func (w *Worker) release(events []Event) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, event := range events {
		if err := w.repository.Release(ctx, event.ID); err != nil {
			w.logger.Error(
				"failed to release outbox event",
				"event_id", event.ID,
				"error", err,
			)
		}
	}
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
