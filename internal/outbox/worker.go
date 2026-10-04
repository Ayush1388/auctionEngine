package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
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

// Worker drains the outbox and hands each claimed event to Handler.
// Run it in a goroutine; it stops when its context is cancelled.
//
// # How it finds work (v1.0)
//
// It is woken by PostgreSQL LISTEN/NOTIFY the moment an event commits
// (Repository.Listen), and polls every interval as a fallback. When woken,
// it claims batches back to back until a batch comes back less than full,
// then sleeps again.
//
// The v1.0 load test showed why draining matters: the earlier version took
// one batch of 10 per 2-second tick, at most 5 events/s, while the API
// produced about 350 events/s (one per bid). After 20 seconds, 18,000
// events were waiting: live updates, cache invalidations and Kafka relays
// were minutes behind. Throughput is now bounded by how fast handlers run,
// not by a timer.
//
// One worker per process, deliberately: events for one auction must be
// relayed in order (the Kafka bid stream is ordered per auction), and two
// workers claiming concurrently could swap them. Several processes are
// still safe for correctness thanks to SKIP LOCKED; consumers tolerate
// reordering through versions and idempotency.
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
		batchSize:  100,
	}
}

func (w *Worker) Run(ctx context.Context) {
	w.logger.Info("outbox worker started")

	wake := w.repository.Listen(ctx, w.logger)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		n, err := w.processBatch(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}

			w.logger.Error(
				"outbox worker processing failed",
				"error", err,
			)
		}

		// A full batch means more is probably waiting: go again at once.
		if err == nil && n == w.batchSize && ctx.Err() == nil {
			continue
		}

		select {
		case <-ctx.Done():
			w.logger.Info("outbox worker stopped")
			return

		case <-wake:
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
	_, err := w.processBatch(ctx)
	return err
}

// processBatch claims and handles one batch and reports how many events
// it claimed.
func (w *Worker) processBatch(ctx context.Context) (int, error) {
	events, err := w.repository.Claim(
		ctx,
		w.batchSize,
	)
	if err != nil {
		return 0, err
	}

	if len(events) == 0 {
		return 0, nil
	}

	// Successful events with nothing to redact are marked processed
	// together, in one UPDATE at the end of the batch, instead of one round
	// trip per event. If the process dies before that UPDATE, those events
	// are delivered again, which at-least-once delivery already allows.
	var done []Event
	defer func() { w.markDone(ctx, done) }()

	for i, event := range events {
		// On shutdown, hand unstarted events back instead of leaving them
		// locked until the lease expires.
		if ctx.Err() != nil {
			w.release(events[i:])
			return len(events), nil
		}

		// Recording the outcome must survive shutdown: an email that was
		// sent should be marked sent even if ctx was cancelled meanwhile.
		finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		if w.processOne(ctx, finishCtx, event) {
			done = append(done, event)
		}
		cancel()
	}

	return len(events), nil
}

// markDone records a batch of successful events in one statement.
func (w *Worker) markDone(ctx context.Context, events []Event) {
	if len(events) == 0 {
		return
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	ids := make([]uuid.UUID, len(events))
	for i, e := range events {
		ids[i] = e.ID
	}
	if err := w.repository.MarkProcessedMany(finishCtx, ids); err != nil {
		w.logger.Error("failed to mark outbox events processed", "count", len(ids), "error", err)
		return
	}
	for _, e := range events {
		metrics.OutboxEvents.WithLabelValues(e.EventType, "processed").Inc()
	}
}

// processOne handles one event and records failures and redacted
// successes itself. It returns true for a success that still needs
// marking processed (the caller batches those).
func (w *Worker) processOne(
	ctx context.Context,
	finishCtx context.Context,
	event Event,
) bool {
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

		return false
	}

	var redactKeys []string
	if r, ok := w.handler.(Redactor); ok {
		redactKeys = r.RedactKeys(event.EventType)
	}
	if len(redactKeys) == 0 {
		return true
	}

	// Events carrying secrets are marked (and redacted) right away.
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

		return false
	}

	metrics.OutboxEvents.WithLabelValues(event.EventType, "processed").Inc()
	// Debug, not Info: at hundreds of events per second an Info line per
	// event is mostly noise and real cost. The metric above counts them.
	w.logger.Debug(
		"outbox event processed",
		"event_id", event.ID,
		"event_type", event.EventType,
	)
	return false
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
