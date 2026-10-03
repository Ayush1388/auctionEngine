// Package kafkax connects the service to Kafka (v0.8): topic layout, the
// producer, and the relay that moves outbox events into Kafka.
//
// # Why Kafka on top of the outbox
//
// The outbox (PostgreSQL) already gives us durable, at-least-once events.
// Kafka adds what a single table can't:
//
//   - Partitioned, ordered processing at scale. Records with the same key
//     (auction_id) always go to the same partition, and one consumer in a
//     group owns a partition at a time, so all bids for one auction are
//     processed in order by one worker, while different auctions are
//     processed in parallel by different workers. Add partitions and
//     workers to scale.
//   - Many independent consumers. Each consumer group (bid workers,
//     analytics, notifications) reads the same stream at its own pace,
//     tracking its own offsets, without the producer knowing they exist.
//   - Retention and replay. Records stay for days; a new consumer can start
//     from the beginning, and a buggy one can be rewound.
//
// # Topics
//
//	auction-bids      bid.requested commands, key = auction_id   (12 partitions)
//	auction-events    every auction.* and bid.* event, key = auction_id
//	auction-bids-dlq  commands that failed permanently (dead letters)
package kafkax

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Partitions per topic. It caps consumer parallelism: a group can't use
// more workers than partitions. It can be increased later, but that changes
// which partition a key maps to, so it is chosen generously up front.
const Partitions = 12

type Topics struct {
	Bids   string
	Events string
	DLQ    string
}

func TopicsWithPrefix(prefix string) Topics {
	return Topics{
		Bids:   prefix + "auction-bids",
		Events: prefix + "auction-events",
		DLQ:    prefix + "auction-bids-dlq",
	}
}

// Brokers parses "host1:9092,host2:9092".
func Brokers(s string) []string {
	var out []string
	for _, b := range strings.Split(s, ",") {
		if b = strings.TrimSpace(b); b != "" {
			out = append(out, b)
		}
	}
	return out
}

// NewProducer creates a producer client.
//
// franz-go producers are idempotent by default: each record carries a
// producer ID and sequence number, so a network retry inside the client
// can't write a record twice. With acks=all, a record is only acknowledged
// once every in-sync replica has it, so a broker crash can't lose an
// acknowledged record.
func NewProducer(brokers []string) (*kgo.Client, error) {
	return kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.ProducerLinger(5*time.Millisecond), // batch tiny bursts
		kgo.RecordPartitioner(kgo.StickyKeyPartitioner(nil)),
	)
}

// EnsureTopics creates the topics if they don't exist. Safe to call on every
// start-up and from several instances at once.
func EnsureTopics(ctx context.Context, cl *kgo.Client, t Topics, replicationFactor int16) error {
	adm := kadm.NewClient(cl)
	for topic, partitions := range map[string]int32{t.Bids: Partitions, t.Events: Partitions, t.DLQ: 1} {
		resp, err := adm.CreateTopic(ctx, partitions, replicationFactor, nil, topic)
		if err != nil && !strings.Contains(err.Error(), "TOPIC_ALREADY_EXISTS") {
			return fmt.Errorf("create topic %s: %w", topic, err)
		}
		if resp.Err != nil && !strings.Contains(resp.Err.Error(), "TOPIC_ALREADY_EXISTS") {
			return fmt.Errorf("create topic %s: %w", topic, resp.Err)
		}
	}
	return nil
}
