package outbox

import (
	"context"
	"log/slog"
	"time"
)

// NotifyChannel is the PostgreSQL channel the outbox_events trigger
// notifies on (migration 000016).
const NotifyChannel = "outbox_events"

// Listen holds one dedicated connection in LISTEN mode and signals on the
// returned channel whenever events are committed. The channel has a buffer
// of one and signals are coalesced: a burst of 500 commits wakes the worker
// once, and the worker then drains everything that is pending.
//
// The connection is taken out of the pool for good (Hijack): a connection
// in LISTEN mode must not be handed to an unrelated query later. On any
// error the listener reconnects after a short pause; meanwhile the
// worker's polling ticker keeps things moving.
func (r *Repository) Listen(ctx context.Context, logger *slog.Logger) <-chan struct{} {
	wake := make(chan struct{}, 1)
	signal := func() {
		select {
		case wake <- struct{}{}:
		default: // a wake-up is already pending
		}
	}

	go func() {
		for ctx.Err() == nil {
			if err := r.listenOnce(ctx, signal); err != nil && ctx.Err() == nil {
				logger.Warn("outbox listener disconnected, retrying", "error", err)
				select {
				case <-ctx.Done():
				case <-time.After(time.Second):
				}
			}
		}
	}()
	return wake
}

func (r *Repository) listenOnce(ctx context.Context, signal func()) error {
	pc, err := r.db.Acquire(ctx)
	if err != nil {
		return err
	}
	conn := pc.Hijack() // ours alone now; closed below, never returned to the pool
	defer conn.Close(context.WithoutCancel(ctx))

	if _, err := conn.Exec(ctx, "LISTEN "+NotifyChannel); err != nil {
		return err
	}
	// Anything committed before LISTEN took effect would otherwise wait for
	// the next poll.
	signal()

	for {
		if _, err := conn.WaitForNotification(ctx); err != nil {
			return err
		}
		signal()
	}
}
