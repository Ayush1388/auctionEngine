package bidqueue

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/bidding"
	"github.com/Ayush1388/auctionEngine/internal/kafkax"
	"github.com/Ayush1388/auctionEngine/internal/metrics"
	"github.com/Ayush1388/auctionEngine/internal/telemetry"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

// Placer places one bid. bidding.Service.PlaceBid satisfies it.
type Placer func(ctx context.Context, in bidding.PlaceBidInput) (bidding.Result, error)

// Worker consumes bid commands from Kafka as a member of a consumer group.
//
// # Ordering
//
// Kafka assigns each partition to exactly one member of the group. All
// commands for one auction share a partition (key = auction_id), and the
// worker processes a partition's records strictly one after another, so
// bids for an auction are applied in the order they were queued. Different
// partitions are processed concurrently, one goroutine each.
//
// # Delivery semantics: at-least-once, effectively-once
//
// Offsets are committed only AFTER a batch is fully processed (autocommit is
// off). A crash before the commit means the records are delivered again, to
// this worker after restart or to another after a rebalance. Duplicates are
// harmless because:
//
//   - PlaceBid is called with the idempotency key "req:<request_id>", so a
//     second attempt returns the original bid instead of bidding twice.
//   - Requests.Record only updates a PENDING row, so an outcome is final.
//
// That combination, at-least-once delivery plus idempotent processing, is
// what people usually mean by "exactly-once" in practice.
//
// # Failures
//
//   - Business rejections (too low, ended, no funds) are outcomes, not
//     errors: recorded as REJECTED, offset committed.
//   - Technical errors (database down) are retried in place with backoff.
//     The partition waits, which preserves order. After MaxAttempts the
//     command goes to the dead-letter topic, the request is marked FAILED,
//     and the partition moves on. One poison message must not block an
//     auction forever.
//
// # Rebalancing
//
// BlockRebalanceOnPoll stops the group from taking partitions away while a
// batch is being processed. AllowRebalance is called only after the batch
// is committed, so a partition is never processed by two workers at once
// and no processed-but-uncommitted work is handed to someone else.
type Worker struct {
	client   *kgo.Client
	group    string
	producer *kgo.Client // for the dead-letter topic
	dlqTopic string
	place    Placer
	requests *Requests
	logger   *slog.Logger

	MaxAttempts int
	Backoff     time.Duration

	processed atomic.Int64
}

// NewWorker joins consumer group "group" on topic.
func NewWorker(brokers []string, group, topic, dlqTopic string, producer *kgo.Client, place Placer, requests *Requests, logger *slog.Logger) (*Worker, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
		kgo.BlockRebalanceOnPoll(),
		// A brand-new group starts from the oldest retained record, so
		// bids queued before the first worker started aren't skipped.
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		return nil, err
	}
	return &Worker{
		client: client, group: group, producer: producer, dlqTopic: dlqTopic,
		place: place, requests: requests, logger: logger,
		MaxAttempts: 5, Backoff: 200 * time.Millisecond,
	}, nil
}

// Processed counts handled commands (tests and metrics).
func (w *Worker) Processed() int64 { return w.processed.Load() }

// Run polls until ctx is cancelled, then leaves the group cleanly so its
// partitions are reassigned immediately rather than after a session timeout.
func (w *Worker) Run(ctx context.Context) {
	defer w.client.Close()

	for {
		fetches := w.client.PollRecords(ctx, 500)
		if fetches.IsClientClosed() || ctx.Err() != nil {
			w.client.AllowRebalance()
			return
		}
		fetches.EachError(func(topic string, partition int32, err error) {
			w.logger.Error("kafka fetch failed", "topic", topic, "partition", partition, "error", err)
		})

		// One goroutine per partition: order within a partition,
		// parallelism across partitions.
		var wg sync.WaitGroup
		var stopped atomic.Bool
		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			w.recordLag(p)
			wg.Add(1)
			go func() {
				defer wg.Done()
				for _, rec := range p.Records {
					if !w.handle(ctx, rec) {
						stopped.Store(true)
						return
					}
				}
			}()
		})
		wg.Wait()

		if stopped.Load() {
			// Shutting down mid-batch: don't commit what we didn't finish.
			// Those records are redelivered (and are idempotent).
			w.client.AllowRebalance()
			return
		}

		if err := w.client.CommitUncommittedOffsets(context.WithoutCancel(ctx)); err != nil {
			w.logger.Error("kafka commit failed; batch will be redelivered", "error", err)
		}
		w.client.AllowRebalance()
	}
}

// recordLag publishes consumer lag: how many records sit in the partition
// beyond the ones just fetched. HighWatermark is the offset the NEXT record
// will get, so lag = HighWatermark - (last fetched offset + 1).
//
// Lag is THE health signal for a consumer: if it keeps growing, workers
// can't keep up and async bids wait longer and longer. That is the number
// to alert on and to scale the worker count by.
func (w *Worker) recordLag(p kgo.FetchTopicPartition) {
	if len(p.Records) == 0 {
		return
	}
	last := p.Records[len(p.Records)-1].Offset
	lag := p.HighWatermark - (last + 1)
	if lag < 0 {
		lag = 0
	}
	metrics.KafkaLag.WithLabelValues(w.group, p.Topic, strconv.Itoa(int(p.Partition))).Set(float64(lag))
}

// handle processes one record. It returns false only if ctx was cancelled
// before the record was finished.
func (w *Worker) handle(ctx context.Context, rec *kgo.Record) bool {
	// Continue the trace from the record headers (set by the relay).
	ctx = telemetry.Extract(ctx, kafkax.HeaderMap(rec))
	ctx, span := telemetry.Tracer().Start(ctx, "kafka process bid command",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "kafka"),
			attribute.String("messaging.destination.name", rec.Topic),
			attribute.Int("messaging.kafka.partition", int(rec.Partition)),
			attribute.Int64("messaging.kafka.offset", rec.Offset),
		))
	defer span.End()

	var cmd Command
	if err := json.Unmarshal(rec.Value, &cmd); err != nil || cmd.RequestID == uuid.Nil {
		// Unparseable: retrying can never help. Dead-letter it.
		w.deadLetter(ctx, rec, "malformed command")
		metrics.BidQueue.WithLabelValues("malformed").Inc()
		return true
	}

	for attempt := 1; ; attempt++ {
		outcome, err := w.process(ctx, cmd)
		if err == nil {
			if err := w.requests.Record(ctx, cmd.RequestID, outcome); err != nil {
				// The bid itself is safe (idempotent); retry recording.
				w.logger.Warn("record outcome failed, retrying", "request_id", cmd.RequestID, "error", err)
			} else {
				w.processed.Add(1)
				metrics.BidQueue.WithLabelValues(string(outcome.Status)).Inc()
				return true
			}
		}
		if ctx.Err() != nil {
			return false
		}
		if attempt >= w.MaxAttempts {
			w.logger.Error("bid command failed permanently", "request_id", cmd.RequestID, "error", err)
			w.deadLetter(ctx, rec, errString(err))
			_ = w.requests.Record(context.WithoutCancel(ctx), cmd.RequestID, Outcome{Status: StatusFailed, Reason: "could not be processed, please retry"})
			telemetry.RecordError(span, err)
			metrics.BidQueue.WithLabelValues(string(StatusFailed)).Inc()
			w.processed.Add(1)
			return true
		}

		select {
		case <-ctx.Done():
			return false
		case <-time.After(w.Backoff * time.Duration(1<<(attempt-1))):
		}
	}
}

// process places the bid and turns business rejections into outcomes. Only
// technical failures come back as errors (and are retried).
func (w *Worker) process(ctx context.Context, cmd Command) (Outcome, error) {
	res, err := w.place(ctx, bidding.PlaceBidInput{
		AuctionID: cmd.AuctionID,
		UserID:    cmd.UserID,
		Amount:    cmd.Amount,
		// Same key on every redelivery: a duplicate can't bid twice.
		IdempotencyKey: "req:" + cmd.RequestID.String(),
	})

	var tooLow *bidding.BidTooLowError
	switch {
	case err == nil:
		return Outcome{Status: StatusAccepted, BidID: &res.Bid.ID}, nil
	case errors.As(err, &tooLow):
		min := tooLow.Minimum
		return Outcome{Status: StatusRejected, Reason: tooLow.Error(), MinimumAmount: &min}, nil
	case errors.Is(err, auction.ErrNotFound),
		errors.Is(err, bidding.ErrOwnAuction),
		errors.Is(err, bidding.ErrAuctionNotActive),
		errors.Is(err, bidding.ErrAuctionEnded),
		errors.Is(err, bidding.ErrInvalidAmount),
		errors.Is(err, bidding.ErrIdempotencyMismatch),
		errors.Is(err, wallet.ErrInsufficientFunds):
		return Outcome{Status: StatusRejected, Reason: err.Error()}, nil
	default:
		// Includes ErrContention: worth retrying.
		return Outcome{}, err
	}
}

func (w *Worker) deadLetter(ctx context.Context, rec *kgo.Record, reason string) {
	dlq := &kgo.Record{
		Topic: w.dlqTopic,
		Key:   rec.Key,
		Value: rec.Value,
		Headers: append(append([]kgo.RecordHeader(nil), rec.Headers...),
			kgo.RecordHeader{Key: "dlq_reason", Value: []byte(reason)},
			kgo.RecordHeader{Key: "dlq_source", Value: []byte(rec.Topic)},
		),
	}
	if err := w.producer.ProduceSync(context.WithoutCancel(ctx), dlq).FirstErr(); err != nil {
		w.logger.Error("dead-letter produce failed", "error", err)
	}
}

func errString(err error) string {
	if err == nil {
		return "unknown error"
	}
	return err.Error()
}
