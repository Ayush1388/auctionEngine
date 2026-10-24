package demobots

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/bidding"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

// Event is one progress message of a stress run. The frontend (app/live.js,
// app/trace.js) reads these field names.
type Event struct {
	Type      string  `json:"type"`
	Round     int     `json:"round,omitempty"`
	Rounds    int     `json:"rounds,omitempty"`
	Bidders   int     `json:"bidders,omitempty"`
	Accepted  int     `json:"accepted"`
	Rejected  int     `json:"rejected"`
	Failed    int     `json:"failed,omitempty"`
	Elapsed   float64 `json:"elapsed"`
	P50       float64 `json:"p50,omitempty"`
	P95       float64 `json:"p95,omitempty"`
	PerSecond float64 `json:"perSecond,omitempty"`
	FinalBid  int64   `json:"finalBid,omitempty"`
	BidCount  int     `json:"bidCount,omitempty"`
	Invariant *Checks `json:"invariants,omitempty"`
	Error     string  `json:"error,omitempty"`
}

type Checks struct {
	OK     bool    `json:"ok"`
	Checks []Check `json:"checks"`
}

type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

type run struct {
	mu     sync.Mutex
	events []Event
	done   bool
	wake   chan struct{}
}

func (r *run) push(e Event) {
	r.mu.Lock()
	r.events = append(r.events, e)
	if e.Type == "done" || e.Type == "error" {
		r.done = true
	}
	old := r.wake
	r.wake = make(chan struct{})
	r.mu.Unlock()
	close(old)
}

// Snapshot returns the events from index `from`, the channel that closes on
// the next event, and whether the run has finished.
func (r *run) snapshot(from int) ([]Event, <-chan struct{}, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.events[from:]), r.wake, r.done
}

// StartStress begins a run in the background and returns its ID.
func (b *Bots) StartStress(auctionID uuid.UUID, bidders, rounds int) string {
	bidders, rounds = min(max(bidders, 2), MaxBidders), min(max(rounds, 1), MaxRounds)
	r := &run{wake: make(chan struct{})}
	id := uuid.NewString()
	b.mu.Lock()
	b.runs[id] = r
	// keep the last few runs only
	if len(b.runs) > 20 {
		for k := range b.runs {
			if k != id {
				delete(b.runs, k)
				break
			}
		}
	}
	b.mu.Unlock()
	go b.execute(r, auctionID, bidders, rounds)
	return id
}

// Events returns a run's progress reader, or false when it is unknown.
func (b *Bots) Events(id string) (func(from int) ([]Event, <-chan struct{}, bool), bool) {
	b.mu.Lock()
	r := b.runs[id]
	b.mu.Unlock()
	if r == nil {
		return nil, false
	}
	return r.snapshot, true
}

// outcome of one bid, for the counters.
type outcome int

const (
	accepted outcome = iota
	rejected
	failed
)

func classify(err error) outcome {
	var tooLow *bidding.BidTooLowError
	switch {
	case err == nil:
		return accepted
	case errors.As(err, &tooLow), errors.Is(err, wallet.ErrInsufficientFunds), errors.Is(err, bidding.ErrContention):
		return rejected
	default:
		return failed
	}
}

// execute is one run: every round all bots read the same price and bid at the same instant.
func (b *Bots) execute(r *run, auctionID uuid.UUID, bidders, rounds int) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	fail := func(err error) { r.push(Event{Type: "error", Error: err.Error()}) }

	bots, err := b.provision(ctx, bidders)
	if err != nil {
		fail(fmt.Errorf("could not set up bidders: %w", err))
		return
	}
	before, err := b.o.Auctions.Get(ctx, auctionID)
	if err != nil {
		fail(err)
		return
	}
	if before.Status != auction.StatusActive {
		fail(fmt.Errorf("the auction is %s, not open for bids", before.Status))
		return
	}

	var nAcc, nRej, nFail atomic.Int64
	var latMu sync.Mutex
	var lat []float64
	start := time.Now()
	r.push(Event{Type: "progress", Round: 0, Rounds: rounds, Bidders: bidders})

	for round := 1; round <= rounds; round++ {
		a, err := b.o.Auctions.Get(ctx, auctionID)
		if err != nil || a.Status != auction.StatusActive {
			fail(errClosed)
			return
		}
		price := bidding.MinimumBid(a) // every bot reads this price...
		gate := make(chan struct{})
		var wg sync.WaitGroup
		for i, bot := range bots {
			wg.Add(1)
			go func() {
				defer wg.Done()
				amount := price
				if rand.Float64() < 0.2 {
					amount += (1 + rand.Int64N(3)) * a.MinIncrement // some bid higher: a real bidding war
				}
				<-gate // ...and they all fire together
				t0 := time.Now()
				_, err := b.o.Bidder.PlaceBid(ctx, bidding.PlaceBidInput{
					AuctionID: auctionID, UserID: bot, Amount: amount,
					IdempotencyKey: fmt.Sprintf("stress-%s-%d-%d", auctionID, round, i),
				})
				ms := float64(time.Since(t0).Microseconds()) / 1000
				latMu.Lock()
				lat = append(lat, ms)
				latMu.Unlock()
				switch classify(err) {
				case accepted:
					nAcc.Add(1)
				case rejected:
					nRej.Add(1)
				default:
					nFail.Add(1)
				}
			}()
		}
		close(gate)
		wg.Wait()

		latMu.Lock()
		sorted := slices.Clone(lat)
		latMu.Unlock()
		slices.Sort(sorted)
		q := func(p float64) float64 {
			if len(sorted) == 0 {
				return 0
			}
			return sorted[min(len(sorted)-1, int(p*float64(len(sorted))))]
		}
		el := time.Since(start).Seconds()
		n := nAcc.Load() + nRej.Load() + nFail.Load()
		r.push(Event{Type: "progress", Round: round, Rounds: rounds, Bidders: bidders, Accepted: int(nAcc.Load()), Rejected: int(nRej.Load()),
			Failed: int(nFail.Load()), Elapsed: el, P50: q(0.5), P95: q(0.95), PerSecond: float64(n) / max(el, 0.001)})
		time.Sleep(250 * time.Millisecond)
	}

	after, err := b.o.Auctions.Get(ctx, auctionID)
	if err != nil {
		fail(err)
		return
	}
	inv := b.verify(ctx, auctionID, before, after, int(nAcc.Load()), int(nFail.Load()))
	fin := int64(0)
	if after.CurrentBid != nil {
		fin = *after.CurrentBid
	}
	r.push(Event{Type: "done", Rounds: rounds, Bidders: bidders, Accepted: int(nAcc.Load()), Rejected: int(nRej.Load()), Failed: int(nFail.Load()),
		Elapsed: time.Since(start).Seconds(), FinalBid: fin, BidCount: after.BidCount, Invariant: inv})
}

// verify checks the run from the outside, using only what the bid history returns.
func (b *Bots) verify(ctx context.Context, id uuid.UUID, before, after auction.Auction, accepted, failed int) *Checks {
	out := &Checks{OK: true}
	add := func(name string, ok bool, detail string) {
		out.Checks = append(out.Checks, Check{name, ok, detail})
		out.OK = out.OK && ok
	}

	var all []bidding.Bid
	cursor := ""
	for page := 0; page < 40; page++ {
		p, err := b.o.Bidder.History(ctx, id, "100", cursor)
		if err != nil {
			add("Bid history could be read back", false, err.Error())
			return out
		}
		all = append(all, p.Bids...)
		if p.NextCursor == nil {
			break
		}
		cursor = p.NextCursor.Encode()
	}

	// newest first: every bid must be higher than the one before it in time
	increasing := true
	for i := 0; i+1 < len(all); i++ {
		if all[i].Amount <= all[i+1].Amount {
			increasing = false
		}
	}
	add("Bid history is strictly increasing", increasing, fmt.Sprintf("%d bids read back", len(all)))
	add("New bids equal accepted requests", after.BidCount-before.BidCount == accepted, fmt.Sprintf("%d accepted, the lot gained %d", accepted, after.BidCount-before.BidCount))
	add("Bid count matches the history", after.BidCount == len(all) || len(all) >= 4000, fmt.Sprintf("%d counted, %d listed", after.BidCount, len(all)))
	lead := len(all) > 0 && after.CurrentBid != nil && all[0].Amount == *after.CurrentBid && after.CurrentBidderID != nil && all[0].UserID == *after.CurrentBidderID
	add("One leader, and it is the last accepted bid", lead, "the top of the history is the current bid and bidder")
	add("No request failed with a server error", failed == 0, fmt.Sprintf("%d failures", failed))
	return out
}
