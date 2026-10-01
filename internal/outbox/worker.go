package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/Ayush1388/auctionEngine/internal/metrics"
	"github.com/Ayush1388/auctionEngine/internal/telemetry"
)

// Handler processes one event. Returning an error schedules a retry.
type Handler interface {
	Handle(
		ctx context.Context,
		event Event,
	) error
}

// Worker polls the outbox and hands each claimed event to Handler.
// Run it in a goroutine; it stops when its context is cancelled.
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

// ProcessOnce claims and handles one batch right now, without waiting for
// the ticker. Tests use it to drive the pipeline deterministically.
func (w *Worker) ProcessOnce(ctx context.Context) error {
	return w.process(ctx)
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
	// Continue the trace of the request that enqueued the event. The span
	// covers every handler the router fans the event out to (cache, search,
	// WebSocket, Kafka relay), so a slow consumer shows up in that trace.
	ctx = telemetry.Extract(ctx, event.TraceContext)
	ctx, span := telemetry.Tracer().Start(ctx, "outbox "+event.EventType,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("outbox.event_id", event.ID.String()),
			attribute.Int("outbox.attempt", event.Attempts),
		))
	defer span.End()

	if err := w.processEvent(ctx, event); err != nil {
		telemetry.RecordError(span, err)
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
		result := "retried"
		if deadLettered {
			result = "dead_lettered"
		}
		metrics.OutboxEvents.WithLabelValues(event.EventType, result).Inc()
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

	metrics.OutboxEvents.WithLabelValues(event.EventType, "processed").Inc()
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
