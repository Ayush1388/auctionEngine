// Package demobots is the demo companion of the website, built into the API
// (v1.2). It used to be a separate program on another port; now the website
// switches it on and off itself.
//
//   - Ambient rivals: a few bidders that now and then raise a random open lot,
//     so a quiet site stays alive and a lot page has someone to race.
//   - Stress it: hundreds of bidders read one price and bid in the same
//     instant, for several rounds, and the result is checked from the outside.
//   - Seed: the car catalogue, demo accounts and a real bid history.
//
// The bots are real users with real wallets, and every bid goes through the
// same bidding service the HTTP handlers use (so the row lock, the wallet
// holds, the ledger, the outbox and the WebSocket fan-out are all real). They
// skip the HTTP layer and the rate limiter, which is why no RATE_LIMITS=off is
// needed. They exist only when the API starts with DEMO_BOTS_ENABLED=true.
package demobots

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/bidding"
	"github.com/Ayush1388/auctionEngine/internal/user"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

const (
	MaxBidders = 300
	MaxRounds  = 20
)

// Bidder is the part of the bidding service the bots use.
type Bidder interface {
	PlaceBid(ctx context.Context, in bidding.PlaceBidInput) (bidding.Result, error)
	History(ctx context.Context, auctionID uuid.UUID, limit, cursor string) (bidding.HistoryPage, error)
}

// Options wire the bots to the running API.
type Options struct {
	Pool     *pgxpool.Pool
	Bidder   Bidder
	Auctions *auction.Service
	Wallets  *wallet.Service
	Logger   *slog.Logger
}

// Bots is the demo bot controller. The zero value is not usable; call New.
type Bots struct {
	o Options

	mu      sync.Mutex
	rivals  []uuid.UUID // the named rival bidders
	stress  []uuid.UUID // stress bidders, provisioned once and reused
	runs    map[string]*run
	ambient *ambientState
}

func New(o Options) *Bots {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return &Bots{o: o, runs: map[string]*run{}}
}

/* ---------------------------------------------------------------- ambient rivals */

type ambientState struct {
	cancel context.CancelFunc
	since  time.Time
}

// AmbientStats is what the website shows next to the switch.
type AmbientStats struct {
	On       bool       `json:"on"`
	Since    *time.Time `json:"since,omitempty"`
	Bids     int64      `json:"bids"`
	Refused  int64      `json:"refused"`
	LastAt   *time.Time `json:"last_at,omitempty"`
	LastLot  string     `json:"last_lot,omitempty"`
	Interval string     `json:"interval"`
}

var (
	ambientBids, ambientRefused atomic.Int64
	ambientLast                 atomic.Pointer[ambientLastBid]
)

type ambientLastBid struct {
	at  time.Time
	lot string
}

// SetAmbient switches the ambient rivals on or off. Idempotent.
func (b *Bots) SetAmbient(ctx context.Context, on bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if on == (b.ambient != nil) {
		return nil
	}
	if !on {
		b.ambient.cancel()
		b.ambient = nil
		return nil
	}
	b.mu.Unlock()
	_, err := b.ensureRivals(ctx)
	b.mu.Lock()
	if err != nil {
		return fmt.Errorf("could not set up the rival bidders: %w", err)
	}
	if b.ambient != nil { // another caller won while the lock was released
		return nil
	}
	loop, cancel := context.WithCancel(context.Background())
	b.ambient = &ambientState{cancel: cancel, since: time.Now()}
	go b.ambientLoop(loop)
	return nil
}

// Ambient reports the rivals' state.
func (b *Bots) Ambient() AmbientStats {
	b.mu.Lock()
	st := b.ambient
	b.mu.Unlock()
	out := AmbientStats{Bids: ambientBids.Load(), Refused: ambientRefused.Load(), Interval: "every 4 to 9 seconds"}
	if st != nil {
		out.On, out.Since = true, &st.since
	}
	if l := ambientLast.Load(); l != nil {
		out.LastAt, out.LastLot = &l.at, l.lot
	}
	return out
}

func (b *Bots) ambientLoop(ctx context.Context) {
	for {
		wait := 4*time.Second + time.Duration(rand.Int64N(int64(5*time.Second)))
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		b.ambientBid(ctx)
	}
}

// ambientBid raises one random open lot by the minimum, as a rival that is not already leading it.
func (b *Bots) ambientBid(ctx context.Context) {
	var id uuid.UUID
	err := b.o.Pool.QueryRow(ctx, `
		SELECT id FROM auctions
		WHERE status = 'ACTIVE' AND ends_at > now() + interval '30 seconds'
		ORDER BY random() LIMIT 1`).Scan(&id)
	if err != nil {
		return // nothing open right now
	}
	a, err := b.o.Auctions.Get(ctx, id)
	if err != nil {
		return
	}
	b.mu.Lock()
	rivals := append([]uuid.UUID(nil), b.rivals...)
	b.mu.Unlock()
	rand.Shuffle(len(rivals), func(i, j int) { rivals[i], rivals[j] = rivals[j], rivals[i] })
	for _, r := range rivals {
		if a.CurrentBidderID != nil && *a.CurrentBidderID == r {
			continue
		}
		_, err := b.o.Bidder.PlaceBid(ctx, bidding.PlaceBidInput{AuctionID: id, UserID: r, Amount: bidding.MinimumBid(a), IdempotencyKey: "ambient-" + uuid.NewString()})
		if err != nil {
			ambientRefused.Add(1)
			return
		}
		ambientBids.Add(1)
		ambientLast.Store(&ambientLastBid{at: time.Now(), lot: a.Item.Name})
		return
	}
}

/* ---------------------------------------------------------------- stress bidders */

// provision makes sure n funded stress bidders exist.
func (b *Bots) provision(ctx context.Context, n int) ([]uuid.UUID, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.stress) >= n {
		return b.stress[:n], nil
	}
	hash, err := user.HashPassword(uuid.NewString())
	if err != nil {
		return nil, err
	}
	for i := len(b.stress); i < n; i++ {
		email := fmt.Sprintf("stress-%03d@bots.marque.test", i+1)
		var id uuid.UUID
		if err := b.o.Pool.QueryRow(ctx, `
			INSERT INTO users (id, email, password_hash, activated_at) VALUES ($1, $2, $3, now())
			ON CONFLICT (email) DO UPDATE SET activated_at = COALESCE(users.activated_at, now()) RETURNING id`,
			uuid.New(), email, hash).Scan(&id); err != nil {
			return nil, err
		}
		// One deposit, idempotent by key: $10M covers any single lot, and a bot leads one bid at a time.
		if _, err := b.o.Wallets.Deposit(ctx, id, wallet.MaxDeposit, "stress-fund-"+id.String()); err != nil {
			return nil, err
		}
		b.stress = append(b.stress, id)
	}
	return b.stress[:n], nil
}

var errClosed = errors.New("the auction closed during the run")
