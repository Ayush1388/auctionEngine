package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/bidding"
	"github.com/Ayush1388/auctionEngine/internal/outbox"
)

// Publisher sends a message to every instance's hub.
type Publisher interface {
	Publish(ctx context.Context, auctionID uuid.UUID, msg []byte) error
}

// LocalPublisher delivers straight to this instance's hub. Correct only
// when there is a single API instance (no Redis configured).
type LocalPublisher struct{ hub *Hub }

func NewLocalPublisher(hub *Hub) *LocalPublisher { return &LocalPublisher{hub: hub} }

func (p *LocalPublisher) Publish(_ context.Context, auctionID uuid.UUID, msg []byte) error {
	p.hub.Deliver(auctionID, msg)
	return nil
}

// RedisFanout publishes through a Redis pub/sub channel that every instance
// subscribes to.
//
// Pub/sub is fire-and-forget: a message published while an instance is
// disconnected from Redis is simply not seen by it. That is acceptable for
// live updates: they are a convenience on top of the REST API, every message
// carries the full current snapshot (not a delta), and clients reload the
// auction on reconnect. Nothing is lost that matters.
type RedisFanout struct {
	rdb     *redis.Client
	channel string
	hub     *Hub
	logger  *slog.Logger
}

func NewRedisFanout(rdb *redis.Client, prefix string, hub *Hub, logger *slog.Logger) *RedisFanout {
	return &RedisFanout{rdb: rdb, channel: prefix + "realtime", hub: hub, logger: logger}
}

type envelope struct {
	AuctionID uuid.UUID       `json:"a"`
	Message   json.RawMessage `json:"m"`
}

func (f *RedisFanout) Publish(ctx context.Context, auctionID uuid.UUID, msg []byte) error {
	b, err := json.Marshal(envelope{AuctionID: auctionID, Message: msg})
	if err != nil {
		return err
	}
	return f.rdb.Publish(ctx, f.channel, b).Err()
}

// Run subscribes and delivers every message to the local hub until ctx ends.
// go-redis reconnects and resubscribes automatically after network errors.
func (f *RedisFanout) Run(ctx context.Context) {
	sub := f.rdb.Subscribe(ctx, f.channel)
	defer sub.Close()

	ch := sub.Channel(redis.WithChannelHealthCheckInterval(30 * time.Second))
	for {
		select {
		case <-ctx.Done():
			return
		case m, ok := <-ch:
			if !ok {
				return
			}
			var env envelope
			if err := json.Unmarshal([]byte(m.Payload), &env); err != nil {
				f.logger.Warn("realtime: bad fan-out message", "error", err)
				continue
			}
			f.hub.Deliver(env.AuctionID, env.Message)
		}
	}
}

// Ready blocks until the subscription is active (tests use it to avoid
// publishing before anyone listens).
func (f *RedisFanout) Ready(ctx context.Context) error {
	for {
		n, err := f.rdb.PubSubNumSub(ctx, f.channel).Result()
		if err != nil {
			return err
		}
		if n[f.channel] > 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// Snapshot is the public, live part of an auction sent to watchers.
//
// Version lets clients ignore out-of-order messages: events can be handled
// out of order (several outbox workers, retries), so a client should only
// apply a snapshot whose version is higher than the one it already shows.
type Snapshot struct {
	ID              uuid.UUID  `json:"id"`
	Status          string     `json:"status"`
	CurrentBid      *int64     `json:"current_bid"`
	CurrentBidderID *uuid.UUID `json:"current_bidder_id"`
	BidCount        int        `json:"bid_count"`
	MinNextBid      int64      `json:"min_next_bid"`
	EndsAt          time.Time  `json:"ends_at"`
	Extensions      int        `json:"extensions"`
	Version         int64      `json:"version"`
}

type update struct {
	Type    string   `json:"type"`
	Cause   string   `json:"cause"`
	Auction Snapshot `json:"auction"`
}

// EventHandler turns outbox events into live updates. Like the search
// indexer, it reloads the auction instead of trusting the event payload, so
// every message is the latest state, and a duplicate event just sends the
// same snapshot again (harmless).
func EventHandler(load func(context.Context, uuid.UUID) (auction.Auction, error), pub Publisher) outbox.Handler {
	return outbox.HandlerFunc(func(ctx context.Context, event outbox.Event) error {
		var payload struct {
			AuctionID uuid.UUID `json:"auction_id"`
		}
		if err := outbox.DecodePayload(event, &payload); err != nil {
			return err
		}
		if payload.AuctionID == uuid.Nil {
			return fmt.Errorf("%s event without auction_id", event.EventType)
		}

		a, err := load(ctx, payload.AuctionID)
		if errors.Is(err, auction.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}

		msg, err := json.Marshal(update{Type: "auction.updated", Cause: event.EventType, Auction: SnapshotOf(a)})
		if err != nil {
			return err
		}
		return pub.Publish(ctx, a.ID, msg)
	})
}

// SnapshotOf extracts the live fields of an auction.
func SnapshotOf(a auction.Auction) Snapshot {
	return Snapshot{
		ID:              a.ID,
		Status:          string(a.Status),
		CurrentBid:      a.CurrentBid,
		CurrentBidderID: a.CurrentBidderID,
		BidCount:        a.BidCount,
		MinNextBid:      bidding.MinimumBid(a), // same rule the bidding engine enforces
		EndsAt:          a.EndsAt.UTC(),
		Extensions:      a.Extensions,
		Version:         a.Version,
	}
}
