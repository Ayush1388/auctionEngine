// Package trending ranks auctions by how many bids they received recently.
//
// # Redis sorted sets
//
// A sorted set holds unique members, each with a numeric score, kept ordered
// by score. Increment a score (ZINCRBY) and read the top N (ZREVRANGE) in
// O(log n). That is exactly a leaderboard, which is what "trending" is.
//
// # A sliding window from hourly buckets
//
// "Most bids in the last hour" is kept as one sorted set per clock hour:
//
//	ae:trending:2026100112   (bids between 12:00 and 12:59)
//	ae:trending:2026100113
//
// Each bid increments its auction in the current hour's set, which expires
// after two hours. Reading combines the current and previous hour with
// ZUNION, weighting the previous hour by how much of it is still inside the
// 60-minute window (at 13:15 the 12:xx bucket counts 75%). That approximates
// a true sliding window without storing every bid's timestamp.
//
// # Idempotent consumer
//
// Scores are fed by the bid.placed outbox event, which can be delivered more
// than once. ZINCRBY isn't idempotent: a redelivery would count a bid twice.
// So a Lua script does "SET seen:<bid_id> NX; if it was new, ZINCRBY"
// atomically inside Redis. A redelivered event finds seen:<bid_id> already
// set and changes nothing. This is the inbox/dedup pattern in miniature.
//
// # Fallback
//
// If Redis is unavailable, Top computes the same ranking from the bids table
// in PostgreSQL. Slower, but correct and always available.
package trending

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/Ayush1388/auctionEngine/internal/database"
	"github.com/Ayush1388/auctionEngine/internal/outbox"
)

// Ranker returns the IDs of the most active auctions, best first.
type Ranker interface {
	Top(ctx context.Context, n int) ([]uuid.UUID, error)
}

// recordScript: count a bid once, however often it's delivered.
//
//	KEYS[1] = hour bucket, KEYS[2] = dedup key for this bid
//	ARGV[1] = auction id, ARGV[2] = dedup TTL seconds, ARGV[3] = bucket TTL seconds
var recordScript = redis.NewScript(`
if redis.call("SET", KEYS[2], "1", "NX", "EX", ARGV[2]) then
	redis.call("ZINCRBY", KEYS[1], 1, ARGV[1])
	redis.call("EXPIRE", KEYS[1], ARGV[3])
	return 1
end
return 0
`)

// Redis is the fast ranker, with PostgreSQL as its fallback.
type Redis struct {
	rdb      *redis.Client
	prefix   string
	fallback *Postgres
	now      func() time.Time
}

func NewRedis(rdb *redis.Client, prefix string, fallback *Postgres) *Redis {
	return &Redis{rdb: rdb, prefix: prefix, fallback: fallback, now: time.Now}
}

// SetClock replaces the clock; tests use it.
func (r *Redis) SetClock(now func() time.Time) { r.now = now }

func (r *Redis) bucket(t time.Time) string {
	return r.prefix + "trending:" + t.UTC().Format("2006010215")
}

// Record counts one bid for auctionID at time at. It returns false if the
// bid was already counted.
func (r *Redis) Record(ctx context.Context, bidID, auctionID uuid.UUID, at time.Time) (bool, error) {
	n, err := recordScript.Run(ctx, r.rdb,
		[]string{r.bucket(at), r.prefix + "trending:seen:" + bidID.String()},
		auctionID.String(), int((3 * time.Hour).Seconds()), int((2 * time.Hour).Seconds()),
	).Int()
	if err != nil {
		return false, fmt.Errorf("record trending bid: %w", err)
	}
	return n == 1, nil
}

func (r *Redis) Top(ctx context.Context, n int) ([]uuid.UUID, error) {
	now := r.now().UTC()
	current := now.Truncate(time.Hour)
	previousWeight := 1 - now.Sub(current).Seconds()/3600

	results, err := r.rdb.ZUnionWithScores(ctx, redis.ZStore{
		Keys:    []string{r.bucket(current), r.bucket(current.Add(-time.Hour))},
		Weights: []float64{1, previousWeight},
	}).Result()
	if err != nil {
		slog.WarnContext(ctx, "trending: redis unavailable, using postgres", "error", err)
		return r.fallback.Top(ctx, n)
	}

	// ZUNION returns ascending by score; walk from the end.
	ids := make([]uuid.UUID, 0, n)
	for i := len(results) - 1; i >= 0 && len(ids) < n; i-- {
		member, _ := results[i].Member.(string)
		if id, err := uuid.Parse(member); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// Handler feeds bid.placed events into the ranking.
func (r *Redis) Handler() outbox.Handler {
	return outbox.HandlerFunc(func(ctx context.Context, event outbox.Event) error {
		var p struct {
			AuctionID uuid.UUID `json:"auction_id"`
			BidID     uuid.UUID `json:"bid_id"`
			PlacedAt  time.Time `json:"placed_at"`
		}
		if err := outbox.DecodePayload(event, &p); err != nil {
			return err
		}
		_, err := r.Record(ctx, p.BidID, p.AuctionID, p.PlacedAt)
		return err
	})
}

// Postgres ranks straight from the bids table: always correct, costs a
// query. Served by bids_auction_created_idx only partly (it groups across
// auctions), which is fine for a fallback.
type Postgres struct {
	db  database.DBTX
	now func() time.Time
}

func NewPostgres(db database.DBTX) *Postgres {
	return &Postgres{db: db, now: time.Now}
}

func (p *Postgres) SetClock(now func() time.Time) { p.now = now }

func (p *Postgres) Top(ctx context.Context, n int) ([]uuid.UUID, error) {
	rows, err := p.db.Query(ctx, `
		SELECT b.auction_id
		FROM bids b
		JOIN auctions a ON a.id = b.auction_id
		WHERE b.created_at > $1 AND a.status = 'ACTIVE'
		GROUP BY b.auction_id
		ORDER BY count(*) DESC, max(b.created_at) DESC
		LIMIT $2
	`, p.now().Add(-time.Hour), n)
	if err != nil {
		return nil, fmt.Errorf("trending from postgres: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ParseLimit reads ?limit= for the trending endpoint (1–50, default 10).
func ParseLimit(s string) (int, bool) {
	if s == "" {
		return 10, true
	}
	n, err := strconv.Atoi(s)
	return n, err == nil && n >= 1 && n <= 50
}
