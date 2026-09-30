package outbox

import (
	"context"
	"io"
	"log/slog"
	"testing"

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
	worker := NewWorker(NewRepository(pool), handler, slog.New(slog.NewTextHandler(io.Discard, nil)))

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
