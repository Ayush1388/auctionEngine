package bidding_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/bidding"
	"github.com/Ayush1388/auctionEngine/internal/outbox"
	"github.com/Ayush1388/auctionEngine/internal/testdb"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

// All bidding behaviour must be identical under both locking strategies,
// so every integration test runs once per strategy.
var strategies = []bidding.Strategy{bidding.Pessimistic, bidding.Optimistic}

var now = time.Now().UTC().Truncate(time.Microsecond)

type env struct {
	t       *testing.T
	ctx     context.Context
	pool    *pgxpool.Pool
	bids    *bidding.Service
	wallets *wallet.Service
}

func newEnv(t *testing.T, strategy bidding.Strategy) *env {
	t.Helper()
	pool := testdb.New(t)
	s := bidding.NewService(pool, strategy)
	s.SetClock(func() time.Time { return now })
	return &env{t: t, ctx: context.Background(), pool: pool, bids: s, wallets: wallet.NewService(pool)}
}

// user creates an activated user holding balance in their wallet.
func (e *env) user(balance int64) uuid.UUID {
	e.t.Helper()
	id := uuid.New()
	if _, err := e.pool.Exec(e.ctx,
		`INSERT INTO users (id, email, password_hash, activated_at) VALUES ($1, $2, 'x', now())`,
		id, id.String()+"@example.com"); err != nil {
		e.t.Fatal(err)
	}
	if balance > 0 {
		if _, err := e.wallets.Deposit(e.ctx, id, balance, "seed"); err != nil {
			e.t.Fatal(err)
		}
	}
	return id
}

// activeAuction inserts an ACTIVE auction that ends endsIn from now.
func (e *env) activeAuction(owner uuid.UUID, startingPrice, increment int64, endsIn time.Duration) uuid.UUID {
	e.t.Helper()
	itemID, id := uuid.New(), uuid.New()
	if _, err := e.pool.Exec(e.ctx,
		`INSERT INTO items (id, owner_id, type, name, description) VALUES ($1, $2, 't', 'n', '')`,
		itemID, owner); err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.pool.Exec(e.ctx, `
		INSERT INTO auctions (id, item_id, owner_id, starting_price, min_increment, starts_at, ends_at, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'ACTIVE')`,
		id, itemID, owner, startingPrice, increment, now.Add(-time.Hour), now.Add(endsIn)); err != nil {
		e.t.Fatal(err)
	}
	return id
}

func (e *env) bid(auctionID, userID uuid.UUID, amount int64) (bidding.Result, error) {
	return e.bids.PlaceBid(e.ctx, bidding.PlaceBidInput{AuctionID: auctionID, UserID: userID, Amount: amount})
}

func (e *env) wallet(userID uuid.UUID) wallet.Wallet {
	e.t.Helper()
	w, err := e.wallets.Get(e.ctx, userID)
	if err != nil {
		e.t.Fatal(err)
	}
	return w
}

func (e *env) auction(id uuid.UUID) auction.Auction {
	e.t.Helper()
	a, err := auction.NewRepository(e.pool).GetByID(e.ctx, id)
	if err != nil {
		e.t.Fatal(err)
	}
	return a
}

// reconcile fails the test unless the books balance: every wallet equals
// its ledger, every journal sums to zero, and reserved money equals the
// ACTIVE reservations.
func (e *env) reconcile() {
	e.t.Helper()
	r, err := e.wallets.Reconcile(e.ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	if !r.OK() {
		e.t.Fatalf("books don't balance: %+v", r)
	}
}

func (e *env) count(query string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(e.ctx, query, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func forEachStrategy(t *testing.T, fn func(t *testing.T, e *env)) {
	for _, s := range strategies {
		t.Run(string(s), func(t *testing.T) { fn(t, newEnv(t, s)) })
	}
}

func TestBidsMoveMoneyCorrectly(t *testing.T) {
	forEachStrategy(t, func(t *testing.T, e *env) {
		seller, alice, bob := e.user(0), e.user(10_000), e.user(10_000)
		a := e.activeAuction(seller, 1_000, 100, time.Hour)

		// Alice leads with 1000: 1000 of her money is held.
		if _, err := e.bid(a, alice, 1_000); err != nil {
			t.Fatal(err)
		}
		if w := e.wallet(alice); w.Available != 9_000 || w.Reserved != 1_000 {
			t.Fatalf("alice after first bid: %+v", w)
		}

		// Bob outbids: Alice gets everything back, Bob's 1500 is held.
		if _, err := e.bid(a, bob, 1_500); err != nil {
			t.Fatal(err)
		}
		if w := e.wallet(alice); w.Available != 10_000 || w.Reserved != 0 {
			t.Fatalf("alice after being outbid: %+v", w)
		}
		if w := e.wallet(bob); w.Available != 8_500 || w.Reserved != 1_500 {
			t.Fatalf("bob after bidding: %+v", w)
		}

		// Bob raises his own bid: only the difference moves.
		res, err := e.bid(a, bob, 2_000)
		if err != nil {
			t.Fatal(err)
		}
		if w := e.wallet(bob); w.Available != 8_000 || w.Reserved != 2_000 {
			t.Fatalf("bob after raising: %+v", w)
		}
		if res.BidCount != 3 || res.CurrentBid != 2_000 {
			t.Fatalf("result: %+v", res)
		}

		got := e.auction(a)
		if *got.CurrentBid != 2_000 || *got.CurrentBidderID != bob || got.BidCount != 3 {
			t.Fatalf("auction: bid=%v bidder=%v count=%d", *got.CurrentBid, *got.CurrentBidderID, got.BidCount)
		}

		if n := e.count(`SELECT count(*) FROM bid_reservations WHERE auction_id = $1 AND status = 'ACTIVE'`, a); n != 1 {
			t.Fatalf("active reservations = %d, want 1", n)
		}
		e.reconcile()
	})
}

func TestBidRejections(t *testing.T) {
	forEachStrategy(t, func(t *testing.T, e *env) {
		seller, alice, poor := e.user(0), e.user(10_000), e.user(500)
		a := e.activeAuction(seller, 1_000, 100, time.Hour)

		var tooLow *bidding.BidTooLowError
		if _, err := e.bid(a, alice, 999); !errors.As(err, &tooLow) || tooLow.Minimum != 1_000 {
			t.Fatalf("below start: %v", err)
		}
		if _, err := e.bid(a, seller, 5_000); !errors.Is(err, bidding.ErrOwnAuction) {
			t.Fatalf("seller: %v", err)
		}
		if _, err := e.bid(a, poor, 1_000); !errors.Is(err, wallet.ErrInsufficientFunds) {
			t.Fatalf("poor bidder: %v", err)
		}
		if _, err := e.bid(uuid.New(), alice, 1_000); !errors.Is(err, auction.ErrNotFound) {
			t.Fatalf("unknown auction: %v", err)
		}

		ended := e.activeAuction(seller, 1_000, 100, -time.Second)
		if _, err := e.bid(ended, alice, 1_000); !errors.Is(err, bidding.ErrAuctionEnded) {
			t.Fatalf("ended auction: %v", err)
		}

		// Rejected bids leave no trace at all.
		if n := e.count(`SELECT count(*) FROM bids`); n != 0 {
			t.Fatalf("bids = %d, want 0", n)
		}
		if w := e.wallet(poor); w.Available != 500 || w.Reserved != 0 {
			t.Fatalf("poor bidder's wallet changed: %+v", w)
		}
		e.reconcile()
	})
}

// Many users bid on one auction at the same moment. The outcome must be
// exactly as if the bids had arrived one at a time.
func TestConcurrentBidsOnOneAuction(t *testing.T) {
	forEachStrategy(t, func(t *testing.T, e *env) {
		const (
			bidders      = 40
			bidsEach     = 5
			startBalance = 100_000
		)
		seller := e.user(0)
		a := e.activeAuction(seller, 100, 10, time.Hour)

		users := make([]uuid.UUID, bidders)
		for i := range users {
			users[i] = e.user(startBalance)
		}

		type accepted struct {
			user   uuid.UUID
			amount int64
		}
		var (
			mu   sync.Mutex
			wins []accepted
			wg   sync.WaitGroup
			go_  = make(chan struct{})
		)

		for _, u := range users {
			wg.Add(1)
			go func(u uuid.UUID) {
				defer wg.Done()
				<-go_
				for i := 0; i < bidsEach; i++ {
					amount := 100 + rand.Int64N(50_000)
					_, err := e.bid(a, u, amount)
					var tooLow *bidding.BidTooLowError
					switch {
					case err == nil:
						mu.Lock()
						wins = append(wins, accepted{u, amount})
						mu.Unlock()
					case errors.As(err, &tooLow), errors.Is(err, bidding.ErrContention):
						// Expected: someone else bid higher first, or the
						// optimistic strategy gave up. Nothing must change.
					default:
						t.Errorf("unexpected error: %v", err)
					}
				}
			}(u)
		}
		close(go_)
		wg.Wait()

		if len(wins) == 0 {
			t.Fatal("no bid was accepted")
		}

		// Accepted amounts are strictly increasing in acceptance order, so
		// they are all distinct and the highest is the current bid.
		sort.Slice(wins, func(i, j int) bool { return wins[i].amount < wins[j].amount })
		for i := 1; i < len(wins); i++ {
			if wins[i].amount-wins[i-1].amount < 10 {
				t.Fatalf("accepted bids %d and %d are closer than the increment", wins[i-1].amount, wins[i].amount)
			}
		}
		top := wins[len(wins)-1]

		got := e.auction(a)
		if *got.CurrentBid != top.amount || *got.CurrentBidderID != top.user {
			t.Fatalf("auction shows %d by %s, highest accepted was %d by %s",
				*got.CurrentBid, *got.CurrentBidderID, top.amount, top.user)
		}
		if got.BidCount != len(wins) || e.count(`SELECT count(*) FROM bids WHERE auction_id = $1`, a) != len(wins) {
			t.Fatalf("bid_count %d, rows %d, accepted %d", got.BidCount,
				e.count(`SELECT count(*) FROM bids WHERE auction_id = $1`, a), len(wins))
		}

		// Only the leader has money held, and exactly the winning amount.
		for _, u := range users {
			w := e.wallet(u)
			wantReserved := int64(0)
			if u == top.user {
				wantReserved = top.amount
			}
			if w.Reserved != wantReserved || w.Total() != startBalance {
				t.Fatalf("user %s: %+v, want reserved %d and total %d", u, w, wantReserved, startBalance)
			}
		}
		e.reconcile()
	})
}

// One user, enough money for one of two bids, bidding on both at once.
func TestNoDoubleSpending(t *testing.T) {
	forEachStrategy(t, func(t *testing.T, e *env) {
		seller := e.user(0)

		for round := 0; round < 15; round++ {
			spender := e.user(1_000)
			x := e.activeAuction(seller, 800, 1, time.Hour)
			y := e.activeAuction(seller, 800, 1, time.Hour)

			var wg sync.WaitGroup
			errs := make([]error, 2)
			start := make(chan struct{})
			for i, a := range []uuid.UUID{x, y} {
				wg.Add(1)
				go func(i int, a uuid.UUID) {
					defer wg.Done()
					<-start
					_, errs[i] = e.bid(a, spender, 800)
				}(i, a)
			}
			close(start)
			wg.Wait()

			ok := 0
			for _, err := range errs {
				switch {
				case err == nil:
					ok++
				case errors.Is(err, wallet.ErrInsufficientFunds):
				default:
					t.Fatalf("round %d: unexpected error %v", round, err)
				}
			}
			if ok != 1 {
				t.Fatalf("round %d: %d bids succeeded with money for one", round, ok)
			}
			if w := e.wallet(spender); w.Available != 200 || w.Reserved != 800 {
				t.Fatalf("round %d: wallet %+v", round, w)
			}
		}
		e.reconcile()
	})
}

// Alice leads auction X, Bob leads Y. At the same instant Bob outbids Alice
// on X and Alice outbids Bob on Y. Each transaction touches both wallets,
// in opposite roles. Without a global lock order this deadlocks.
func TestCrossAuctionBidsDoNotDeadlock(t *testing.T) {
	forEachStrategy(t, func(t *testing.T, e *env) {
		seller, alice, bob := e.user(0), e.user(10_000_000), e.user(10_000_000)

		for round := 0; round < 20; round++ {
			x := e.activeAuction(seller, 100, 1, time.Hour)
			y := e.activeAuction(seller, 100, 1, time.Hour)
			if _, err := e.bid(x, alice, 100); err != nil {
				t.Fatal(err)
			}
			if _, err := e.bid(y, bob, 100); err != nil {
				t.Fatal(err)
			}

			var wg sync.WaitGroup
			errs := make([]error, 2)
			start := make(chan struct{})
			wg.Add(2)
			go func() { defer wg.Done(); <-start; _, errs[0] = e.bid(x, bob, 200) }()
			go func() { defer wg.Done(); <-start; _, errs[1] = e.bid(y, alice, 200) }()
			close(start)
			wg.Wait()

			for _, err := range errs {
				if err != nil {
					t.Fatalf("round %d: %v", round, err)
				}
			}
		}
		e.reconcile()
	})
}

func TestIdempotentBids(t *testing.T) {
	forEachStrategy(t, func(t *testing.T, e *env) {
		seller, alice := e.user(0), e.user(10_000)
		a := e.activeAuction(seller, 1_000, 100, time.Hour)
		in := bidding.PlaceBidInput{AuctionID: a, UserID: alice, Amount: 1_500, IdempotencyKey: "click-42"}

		// The same request, retried 10 times at once (a flaky network).
		var wg sync.WaitGroup
		results := make([]bidding.Result, 10)
		errs := make([]error, 10)
		start := make(chan struct{})
		for i := range results {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				results[i], errs[i] = e.bids.PlaceBid(e.ctx, in)
			}(i)
		}
		close(start)
		wg.Wait()

		fresh := 0
		for i, err := range errs {
			if err != nil {
				t.Fatalf("attempt %d: %v", i, err)
			}
			if results[i].Bid.ID != results[0].Bid.ID {
				t.Fatalf("attempt %d returned a different bid", i)
			}
			if !results[i].Replayed {
				fresh++
			}
		}
		if fresh != 1 {
			t.Fatalf("%d attempts placed a bid, want exactly 1", fresh)
		}
		if n := e.count(`SELECT count(*) FROM bids`); n != 1 {
			t.Fatalf("bids = %d, want 1", n)
		}
		if w := e.wallet(alice); w.Reserved != 1_500 {
			t.Fatalf("money reserved %d times over: %+v", w.Reserved/1_500, w)
		}

		// Reusing the key for a different bid is a client bug.
		in.Amount = 2_000
		if _, err := e.bids.PlaceBid(e.ctx, in); !errors.Is(err, bidding.ErrIdempotencyMismatch) {
			t.Fatalf("reused key: %v", err)
		}
		e.reconcile()
	})
}

func TestAntiSniping(t *testing.T) {
	forEachStrategy(t, func(t *testing.T, e *env) {
		seller, alice := e.user(0), e.user(10_000)
		a := e.activeAuction(seller, 100, 1, 30*time.Second)

		res, err := e.bid(a, alice, 100)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Extended || !res.EndsAt.Equal(now.Add(bidding.SnipeWindow)) {
			t.Fatalf("last-second bid should extend to now+%v: %+v", bidding.SnipeWindow, res)
		}
		if got := e.auction(a); got.Extensions != 1 {
			t.Fatalf("extensions = %d", got.Extensions)
		}
	})
}

// A bid racing the lifecycle worker's completion: either the bid lands
// first (and the auction completes with it as the winner) or completion
// lands first (and the bid is rejected). Never both, never neither.
func TestBidRacingCompletion(t *testing.T) {
	forEachStrategy(t, func(t *testing.T, e *env) {
		seller := e.user(0)
		worker := auction.NewLifecycleWorker(e.pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
		worker.SetClock(func() time.Time { return now.Add(time.Hour) }) // long past the end

		for round := 0; round < 20; round++ {
			bidder := e.user(10_000)
			a := e.activeAuction(seller, 100, 1, 10*time.Minute)

			var bidErr error
			var wg sync.WaitGroup
			start := make(chan struct{})
			wg.Add(2)
			go func() { defer wg.Done(); <-start; _, bidErr = e.bid(a, bidder, 500) }()
			go func() {
				defer wg.Done()
				<-start
				if _, err := worker.Tick(e.ctx); err != nil {
					t.Error(err)
				}
			}()
			close(start)
			wg.Wait()

			// Make sure the auction is completed either way.
			if _, err := worker.Tick(e.ctx); err != nil {
				t.Fatal(err)
			}

			got := e.auction(a)
			if got.Status != auction.StatusCompleted {
				t.Fatalf("round %d: status %s", round, got.Status)
			}

			switch {
			case bidErr == nil:
				if got.CurrentBidderID == nil || *got.CurrentBidderID != bidder {
					t.Fatalf("round %d: bid accepted but not recorded as leading", round)
				}
			case errors.Is(bidErr, bidding.ErrAuctionEnded), errors.Is(bidErr, bidding.ErrContention):
				if got.CurrentBidderID != nil {
					t.Fatalf("round %d: bid rejected but auction has a leader", round)
				}
				if w := e.wallet(bidder); w.Reserved != 0 {
					t.Fatalf("round %d: rejected bidder has money held: %+v", round, w)
				}
			default:
				t.Fatalf("round %d: unexpected error %v", round, bidErr)
			}
		}
		e.reconcile()
	})
}

func TestSettlement(t *testing.T) {
	forEachStrategy(t, func(t *testing.T, e *env) {
		seller, alice, bob := e.user(0), e.user(10_000), e.user(10_000)
		a := e.activeAuction(seller, 1_000, 100, time.Hour)
		for _, b := range []struct {
			u uuid.UUID
			v int64
		}{{alice, 1_000}, {bob, 2_000}, {alice, 3_000}} {
			if _, err := e.bid(a, b.u, b.v); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := e.pool.Exec(e.ctx, `UPDATE auctions SET status = 'COMPLETED' WHERE id = $1`, a); err != nil {
			t.Fatal(err)
		}

		// The outbox may deliver auction.completed more than once.
		for i := 0; i < 3; i++ {
			if err := e.bids.Settle(e.ctx, a); err != nil {
				t.Fatalf("settle #%d: %v", i+1, err)
			}
		}

		if w := e.wallet(seller); w.Available != 3_000 {
			t.Fatalf("seller paid %d, want exactly 3000 once", w.Available)
		}
		if w := e.wallet(alice); w.Available != 7_000 || w.Reserved != 0 {
			t.Fatalf("winner: %+v", w)
		}
		if w := e.wallet(bob); w.Available != 10_000 || w.Reserved != 0 {
			t.Fatalf("loser: %+v", w)
		}
		if got := e.auction(a); got.SettledAt == nil {
			t.Fatal("settled_at not set")
		}
		if n := e.count(`SELECT count(*) FROM bid_reservations WHERE auction_id = $1 AND status = 'SETTLED'`, a); n != 1 {
			t.Fatalf("settled reservations = %d", n)
		}

		// An auction nobody bid on settles without moving money.
		empty := e.activeAuction(seller, 1_000, 100, time.Hour)
		if _, err := e.pool.Exec(e.ctx, `UPDATE auctions SET status = 'COMPLETED' WHERE id = $1`, empty); err != nil {
			t.Fatal(err)
		}
		if err := e.bids.Settle(e.ctx, empty); err != nil {
			t.Fatal(err)
		}
		e.reconcile()
	})
}

func TestLedgerIsAppendOnly(t *testing.T) {
	e := newEnv(t, bidding.Pessimistic)
	alice := e.user(1_000)

	for _, q := range []string{
		`UPDATE ledger_entries SET amount = amount * 2`,
		`DELETE FROM ledger_entries`,
		`DELETE FROM ledger_transactions`,
	} {
		if _, err := e.pool.Exec(e.ctx, q); err == nil {
			t.Fatalf("%q succeeded on an append-only table", q)
		}
	}
	if w := e.wallet(alice); w.Available != 1_000 {
		t.Fatalf("wallet changed: %+v", w)
	}
}

func TestDepositIsIdempotent(t *testing.T) {
	e := newEnv(t, bidding.Pessimistic)
	alice := e.user(0)

	for i := 0; i < 3; i++ {
		if _, err := e.wallets.Deposit(e.ctx, alice, 500, "topup-1"); err != nil {
			t.Fatal(err)
		}
	}
	if w := e.wallet(alice); w.Available != 500 {
		t.Fatalf("deposit applied %d times", w.Available/500)
	}
	e.reconcile()
}

func TestHistory(t *testing.T) {
	e := newEnv(t, bidding.Pessimistic)
	seller, alice, bob := e.user(0), e.user(1_000_000), e.user(1_000_000)
	a := e.activeAuction(seller, 100, 1, time.Hour)

	for i := 0; i < 25; i++ {
		u := alice
		if i%2 == 1 {
			u = bob
		}
		if _, err := e.bid(a, u, int64(100+i*10)); err != nil {
			t.Fatal(err)
		}
	}

	var all []bidding.Bid
	cursor := ""
	for pages := 0; ; pages++ {
		page, err := e.bids.History(e.ctx, a, "10", cursor)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, page.Bids...)
		if page.NextCursor == nil {
			break
		}
		cursor = page.NextCursor.Encode()
		if pages > 5 {
			t.Fatal("pagination did not end")
		}
	}

	if len(all) != 25 {
		t.Fatalf("got %d bids, want 25", len(all))
	}
	seen := map[uuid.UUID]bool{}
	for _, b := range all {
		if seen[b.ID] {
			t.Fatalf("bid %s listed twice", b.ID)
		}
		seen[b.ID] = true
	}
}

// The whole chain as it runs in production: the lifecycle worker completes
// the auction and enqueues auction.completed; the outbox worker routes it
// to settlement; the seller is paid.
func TestAuctionCompletionSettlesThroughTheOutbox(t *testing.T) {
	e := newEnv(t, bidding.Pessimistic)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	seller, alice := e.user(0), e.user(10_000)
	a := e.activeAuction(seller, 1_000, 100, time.Hour)
	if _, err := e.bid(a, alice, 4_000); err != nil {
		t.Fatal(err)
	}

	lifecycle := auction.NewLifecycleWorker(e.pool, quiet)
	lifecycle.SetClock(func() time.Time { return now.Add(2 * time.Hour) })
	if _, err := lifecycle.Tick(e.ctx); err != nil {
		t.Fatal(err)
	}

	router := outbox.NewRouter()
	router.Register(auction.EventTypeCompleted, e.bids.SettlementHandler())
	for _, typ := range []string{bidding.EventTypePlaced, bidding.EventTypeSettled} {
		router.Register(typ, outbox.HandlerFunc(func(context.Context, outbox.Event) error { return nil }))
	}

	ctx, cancel := context.WithCancel(e.ctx)
	done := make(chan struct{})
	go func() { outbox.NewWorker(outbox.NewRepository(e.pool), router, quiet).Run(ctx); close(done) }()

	deadline := time.Now().Add(10 * time.Second)
	for e.wallet(seller).Available != 4_000 {
		if time.Now().After(deadline) {
			t.Fatalf("seller not paid: %+v", e.wallet(seller))
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	<-done

	if w := e.wallet(alice); w.Available != 6_000 || w.Reserved != 0 {
		t.Fatalf("winner: %+v", w)
	}
	e.reconcile()
}
