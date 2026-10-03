package outbox

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Ayush1388/auctionEngine/internal/testdb"
)

// cancelAfterFirst handles one event, then cancels the worker's context,
// like a shutdown signal arriving mid-batch.
type cancelAfterFirst struct {
	cancel  context.CancelFunc
	handled int
}

func (h *cancelAfterFirst) Handle(ctx context.Context, event Event) error {
	h.handled++
	h.cancel()
	return nil
}

func TestWorkerShutdownMidBatch(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := Enqueue(ctx, pool, "test.event", map[string]int{"n": i}); err != nil {
			t.Fatal(err)
		}
	}

	workerCtx, cancel := context.WithCancel(ctx)
	handler := &cancelAfterFirst{cancel: cancel}
	worker := NewWorker(NewRepository(pool), handler, quietLogger())

	if err := worker.process(workerCtx); err != nil {
		t.Fatalf("process: %v", err)
	}

	if handler.handled != 1 {
		t.Fatalf("handled %d events, want 1", handler.handled)
	}

	var processed, locked, attempted int
	if err := pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE processed_at IS NOT NULL),
			count(*) FILTER (WHERE locked_at IS NOT NULL),
			count(*) FILTER (WHERE processed_at IS NULL AND attempts > 0)
		FROM outbox_events
	`).Scan(&processed, &locked, &attempted); err != nil {
		t.Fatal(err)
	}

	// The handled event is recorded even though ctx was cancelled...
	if processed != 1 {
		t.Errorf("processed = %d, want 1", processed)
	}
	// ...and the other two are handed back untouched, not left locked
	// until the 5-minute lease expires.
	if locked != 0 || attempted != 0 {
		t.Errorf("locked = %d, attempted = %d; want both 0", locked, attempted)
	}
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type recordingHandler struct{ handled chan Event }

func (h *recordingHandler) Handle(_ context.Context, e Event) error {
	h.handled <- e
	return nil
}

// With polling effectively off (1 hour), an event is still picked up at
// once: the INSERT trigger's NOTIFY wakes the worker on commit.
func TestWorkerIsWokenByNotify(t *testing.T) {
	pool := testdb.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := &recordingHandler{handled: make(chan Event, 10)}
	w := NewWorker(NewRepository(pool), h, quietLogger())
	w.interval = time.Hour
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()

	// Let the first (empty) pass and the LISTEN happen.
	time.Sleep(200 * time.Millisecond)

	start := time.Now()
	if err := Enqueue(context.Background(), pool, "test.event", map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-h.handled:
		t.Logf("picked up %s after commit", time.Since(start).Round(time.Millisecond))
	case <-time.After(3 * time.Second):
		t.Fatal("event not handled: NOTIFY did not wake the worker")
	}
	cancel()
	<-done
}

// A backlog larger than one batch is drained back to back, not one batch
// per tick (the v1.0 load-test bug).
func TestWorkerDrainsBacklogWithoutWaitingForTicks(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	const n = 350
	if _, err := pool.Exec(ctx, `
		INSERT INTO outbox_events (id, event_type, payload)
		SELECT gen_random_uuid(), 'test.event', '{}' FROM generate_series(1, $1)`, n); err != nil {
		t.Fatal(err)
	}

	h := &recordingHandler{handled: make(chan Event, n)}
	w := NewWorker(NewRepository(pool), h, quietLogger())
	w.interval = time.Hour
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { w.Run(runCtx); close(done) }()

	deadline := time.After(10 * time.Second)
	for got := 0; got < n; got++ {
		select {
		case <-h.handled:
		case <-deadline:
			t.Fatalf("only %d of %d handled", got, n)
		}
	}
	cancel()
	<-done

	var pending int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE processed_at IS NULL`).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatalf("%d events left unmarked", pending)
	}
}
