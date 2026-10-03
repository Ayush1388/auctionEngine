package auction_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ayush1388/auctionEngine/internal/auction"
)

func setStatus(t *testing.T, pool *pgxpool.Pool, id uuid.UUID, status auction.Status) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `UPDATE auctions SET status = $2 WHERE id = $1`, id, status); err != nil {
		t.Fatal(err)
	}
}

func TestCancel(t *testing.T) {
	service, pool := newDBService(t)
	ctx := context.Background()
	owner := insertUser(t, pool)
	other := insertUser(t, pool)

	create := func() uuid.UUID {
		a, err := service.Create(ctx, owner, validInput())
		if err != nil {
			t.Fatal(err)
		}
		return a.ID
	}

	t.Run("owner can cancel before start", func(t *testing.T) {
		got, err := service.Cancel(ctx, owner, create())
		if err != nil {
			t.Fatalf("Cancel: %v", err)
		}
		if got.Status != auction.StatusCancelled {
			t.Fatalf("status = %s", got.Status)
		}
		if !got.UpdatedAt.After(got.CreatedAt) {
			t.Fatalf("updated_at not bumped")
		}
	})

	t.Run("someone else gets ErrForbidden and nothing changes", func(t *testing.T) {
		id := create()

		if _, err := service.Cancel(ctx, other, id); !errors.Is(err, auction.ErrForbidden) {
			t.Fatalf("got %v, want ErrForbidden", err)
		}

		a, _ := service.Get(ctx, id)
		if a.Status != auction.StatusNotActive {
			t.Fatalf("status changed to %s", a.Status)
		}
	})

	t.Run("unknown auction", func(t *testing.T) {
		if _, err := service.Cancel(ctx, owner, uuid.New()); !errors.Is(err, auction.ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}
	})

	for _, status := range []auction.Status{auction.StatusActive, auction.StatusCompleted, auction.StatusCancelled} {
		t.Run("cannot cancel "+string(status), func(t *testing.T) {
			id := create()
			setStatus(t, pool, id, status)

			if _, err := service.Cancel(ctx, owner, id); !errors.Is(err, auction.ErrInvalidTransition) {
				t.Fatalf("got %v, want ErrInvalidTransition", err)
			}
		})
	}

	t.Run("cannot cancel once the start time has passed", func(t *testing.T) {
		id := create()

		// The worker hasn't activated it yet, but the clock says it started.
		later := auction.NewService(pool, auction.NewRepository(pool))
		later.SetClock(func() time.Time { return now.Add(2 * time.Hour) })

		if _, err := later.Cancel(ctx, owner, id); !errors.Is(err, auction.ErrAlreadyStarted) {
			t.Fatalf("got %v, want ErrAlreadyStarted", err)
		}
	})
}

// A cancel racing with activation must produce exactly one winner, never
// an auction that is both "cancelled" for the seller and "active" for
// bidders.
func TestCancelRacingActivation(t *testing.T) {
	service, pool := newDBService(t)
	ctx := context.Background()
	owner := insertUser(t, pool)
	repo := auction.NewRepository(pool)

	for i := 0; i < 25; i++ {
		a, err := service.Create(ctx, owner, validInput())
		if err != nil {
			t.Fatal(err)
		}

		var (
			wg          sync.WaitGroup
			cancelErr   error
			activated   bool
			activateErr error
			start       = make(chan struct{})
		)

		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, cancelErr = service.Cancel(ctx, owner, a.ID)
		}()
		go func() {
			defer wg.Done()
			<-start
			activated, activateErr = repo.Transition(ctx, a.ID, auction.StatusActive, auction.TransitionGuard{})
		}()
		close(start)
		wg.Wait()

		if activateErr != nil {
			t.Fatal(activateErr)
		}

		cancelled := cancelErr == nil
		if cancelled == activated {
			t.Fatalf("round %d: cancelled=%v activated=%v, want exactly one (cancel err: %v)", i, cancelled, activated, cancelErr)
		}
		if !cancelled && !errors.Is(cancelErr, auction.ErrInvalidTransition) {
			t.Fatalf("round %d: losing cancel returned %v, want ErrInvalidTransition", i, cancelErr)
		}

		final, _ := service.Get(ctx, a.ID)
		want := auction.StatusActive
		if cancelled {
			want = auction.StatusCancelled
		}
		if final.Status != want {
			t.Fatalf("round %d: final status %s, want %s", i, final.Status, want)
		}
	}
}
