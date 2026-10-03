package auction_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/testdb"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func newWorker(pool *pgxpool.Pool, at time.Time) *auction.LifecycleWorker {
	w := auction.NewLifecycleWorker(pool, quiet)
	w.SetClock(func() time.Time { return at })
	return w
}

// insertAuction creates an auction with explicit times and status.
func insertAuction(t *testing.T, pool *pgxpool.Pool, owner uuid.UUID, startsAt, endsAt time.Time, status auction.Status) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	itemID, id := uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO items (id, owner_id, type, name, description) VALUES ($1, $2, 't', 'n', '')`,
		itemID, owner,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO auctions (id, item_id, owner_id, starting_price, starts_at, ends_at, status)
		VALUES ($1, $2, $3, 100, $4, $5, $6)`,
		id, itemID, owner, startsAt, endsAt, status,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

func statusOf(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) auction.Status {
	t.Helper()
	var s auction.Status
	if err := pool.QueryRow(context.Background(), `SELECT status FROM auctions WHERE id = $1`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTickMovesDueAuctions(t *testing.T) {
	pool := testdb.New(t)
	owner := insertUser(t, pool)
	h := time.Hour

	dueToStart := insertAuction(t, pool, owner, now.Add(-h), now.Add(h), auction.StatusNotActive)
	dueToEnd := insertAuction(t, pool, owner, now.Add(-2*h), now.Add(-time.Second), auction.StatusActive)
	future := insertAuction(t, pool, owner, now.Add(h), now.Add(2*h), auction.StatusNotActive)
	running := insertAuction(t, pool, owner, now.Add(-h), now.Add(h), auction.StatusActive)
	cancelled := insertAuction(t, pool, owner, now.Add(-2*h), now.Add(-h), auction.StatusCancelled)
	// The whole window passed while the service was down.
	missed := insertAuction(t, pool, owner, now.Add(-3*h), now.Add(-2*h), auction.StatusNotActive)

	result, err := newWorker(pool, now).Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}

	want := map[uuid.UUID]auction.Status{
		dueToStart: auction.StatusActive,
		dueToEnd:   auction.StatusCompleted,
		future:     auction.StatusNotActive,
		running:    auction.StatusActive,
		cancelled:  auction.StatusCancelled, // never revived
		missed:     auction.StatusCompleted, // activated and completed in one tick
	}
	for id, status := range want {
		if got := statusOf(t, pool, id); got != status {
			t.Errorf("auction %s: status %s, want %s", id, got, status)
		}
	}

	if result.Activated != 2 || result.Completed != 2 {
		t.Errorf("result = %+v, want 2 activated, 2 completed", result)
	}
}

func TestCompletionEnqueuesEventInSameTransaction(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	owner := insertUser(t, pool)
	id := insertAuction(t, pool, owner, now.Add(-2*time.Hour), now.Add(-time.Minute), auction.StatusActive)

	if _, err := newWorker(pool, now).Tick(ctx); err != nil {
		t.Fatal(err)
	}

	var payload []byte
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM outbox_events WHERE event_type = $1`, auction.EventTypeCompleted,
	).Scan(&payload); err != nil {
		t.Fatalf("expected one auction.completed event: %v", err)
	}

	var event auction.CompletedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatal(err)
	}
	if event.AuctionID != id || event.OwnerID != owner || event.WinningBid != nil {
		t.Fatalf("unexpected event: %+v", event)
	}

	// A second tick finds nothing to do and adds no duplicate event.
	result, err := newWorker(pool, now).Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Activated+result.Completed != 0 {
		t.Fatalf("second tick moved auctions: %+v", result)
	}

	var events int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("outbox has %d events, want 1", events)
	}
}

func TestCompletionRollsBackWhenOutboxFails(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	owner := insertUser(t, pool)
	id := insertAuction(t, pool, owner, now.Add(-2*time.Hour), now.Add(-time.Minute), auction.StatusActive)

	if _, err := pool.Exec(ctx, `ALTER TABLE outbox_events RENAME TO outbox_events_gone`); err != nil {
		t.Fatal(err)
	}

	if _, err := newWorker(pool, now).Tick(ctx); err == nil {
		t.Fatal("expected Tick to fail when the outbox is unavailable")
	}

	if got := statusOf(t, pool, id); got != auction.StatusActive {
		t.Fatalf("auction completed without its event: status %s", got)
	}
}

// Several API instances each run a worker. Every auction must be moved by
// exactly one of them.
func TestConcurrentWorkersNeverMoveTheSameAuctionTwice(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	owner := insertUser(t, pool)

	const auctions = 200
	for i := 0; i < auctions; i++ {
		insertAuction(t, pool, owner, now.Add(-2*time.Hour), now.Add(-time.Minute), auction.StatusActive)
	}

	const workers = 4
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		completed int
		start     = make(chan struct{})
	)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := newWorker(pool, now)
			w.SetBatchSize(10)
			<-start

			for {
				result, err := w.Tick(ctx)
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				completed += result.Completed
				mu.Unlock()

				if result.Completed == 0 {
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()

	if completed != auctions {
		t.Fatalf("workers reported %d completions, want exactly %d", completed, auctions)
	}

	var events, distinct int
	if err := pool.QueryRow(ctx,
		`SELECT count(*), count(DISTINCT payload->>'auction_id') FROM outbox_events`,
	).Scan(&events, &distinct); err != nil {
		t.Fatal(err)
	}
	if events != auctions || distinct != auctions {
		t.Fatalf("events = %d (distinct %d), want %d of each", events, distinct, auctions)
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	pool := testdb.New(t)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		newWorker(pool, now).Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}
