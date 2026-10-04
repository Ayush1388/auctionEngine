package kafkax

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/Ayush1388/auctionEngine/internal/outbox"
	"github.com/Ayush1388/auctionEngine/internal/telemetry"
)

// HeaderMap returns a record's headers as a map, for telemetry.Extract.
func HeaderMap(rec *kgo.Record) map[string]string {
	m := make(map[string]string, len(rec.Headers))
	for _, h := range rec.Headers {
		m[h.Key] = string(h.Value)
	}
	return m
}

// Relay is an outbox handler that publishes events to Kafka.
//
//	PostgreSQL ── outbox_events ──► outbox.Worker ──► Relay ──► Kafka topic
//
// This is the second half of the transactional outbox: the event was
// committed together with the change that caused it, and the relay keeps
// retrying until Kafka has acknowledged it. Nothing ever writes to Kafka and
// the database as two separate steps (the dual-write problem).
//
// Delivery is still at-least-once: if the relay crashes after Kafka acked
// but before the outbox row is marked processed, the event is sent again.
// Every record carries the outbox event ID in a header so consumers can
// recognise duplicates.
type Relay struct {
	producer *kgo.Client
	topic    func(eventType string) string
}

// NewRelay sends bid.requested to the bids topic and everything else to the
// events topic.
func NewRelay(producer *kgo.Client, topics Topics, bidRequestedType string) *Relay {
	return &Relay{
		producer: producer,
		topic: func(eventType string) string {
			if eventType == bidRequestedType {
				return topics.Bids
			}
			return topics.Events
		},
	}
}

func (r *Relay) Handler() outbox.Handler {
	return outbox.HandlerFunc(func(ctx context.Context, event outbox.Event) error {
		var payload struct {
			AuctionID uuid.UUID `json:"auction_id"`
		}
		if err := outbox.DecodePayload(event, &payload); err != nil {
			return err
		}
		if payload.AuctionID == uuid.Nil {
			return fmt.Errorf("%s event without auction_id; cannot pick a partition", event.EventType)
		}

		record := &kgo.Record{
			Topic: r.topic(event.EventType),
			// The key decides the partition: every record for one auction
			// lands in the same partition and is consumed in order.
			Key:   []byte(payload.AuctionID.String()),
			Value: event.Payload,
			Headers: []kgo.RecordHeader{
				{Key: "event_id", Value: []byte(event.ID.String())},
				{Key: "event_type", Value: []byte(event.EventType)},
			},
		}

		// Trace context travels in record headers (W3C traceparent), so
		// the consumer's span joins the trace of the request that caused
		// this event, across the asynchronous hop.
		for k, v := range telemetry.Inject(ctx) {
			record.Headers = append(record.Headers, kgo.RecordHeader{Key: k, Value: []byte(v)})
		}

		// ProduceSync waits for the broker's acknowledgement. Returning
		// its error makes the outbox retry, so an event is never marked
		// processed before Kafka really has it.
		return r.producer.ProduceSync(ctx, record).FirstErr()
	})
}
