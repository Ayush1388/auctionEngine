package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Ayush1388/auctionEngine/internal/testdb"
)

func TestRetryBacksOffThenDeadLetters(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	repo := NewRepository(pool)

	if err := Enqueue(ctx, pool, "test.event", map[string]string{}); err != nil {
		t.Fatal(err)
	}

	var lastDelay time.Duration

	for attempt := 1; attempt <= MaxAttempts; attempt++ {
		// Make the event due and claim it, as the worker would.
		if _, err := pool.Exec(ctx, `UPDATE outbox_events SET available_at = now()`); err != nil {
			t.Fatal(err)
		}
		events, err := repo.Claim(ctx, 10)
		if err != nil || len(events) != 1 {
			t.Fatalf("attempt %d: claimed %d events, err %v", attempt, len(events), err)
		}

		deadLettered, err := repo.Retry(ctx, events[0].ID, errors.New("smtp down"))
		if err != nil {
			t.Fatal(err)
		}

		if want := attempt == MaxAttempts; deadLettered != want {
			t.Fatalf("attempt %d: deadLettered = %v, want %v", attempt, deadLettered, want)
		}

		var delay time.Duration
		if err := pool.QueryRow(ctx,
			`SELECT available_at - now() FROM outbox_events`,
		).Scan(&delay); err != nil {
			t.Fatal(err)
		}

		if attempt > 1 && attempt < 7 && delay < lastDelay*2-time.Second {
			t.Fatalf("attempt %d: delay %s did not double from %s", attempt, delay, lastDelay)
		}
		if delay > time.Hour+time.Second {
			t.Fatalf("attempt %d: delay %s exceeds the 1h cap", attempt, delay)
		}
		lastDelay = delay
	}

	// A dead-lettered event is never claimed again.
	if _, err := pool.Exec(ctx, `UPDATE outbox_events SET available_at = now()`); err != nil {
		t.Fatal(err)
	}
	events, err := repo.Claim(ctx, 10)
	if err != nil || len(events) != 0 {
		t.Fatalf("dead-lettered event was claimed again: %d, %v", len(events), err)
	}
}

type okHandler struct{}

func (okHandler) Handle(context.Context, Event) error { return nil }

func TestProcessedEventsHaveSecretsRedacted(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	if err := Enqueue(ctx, pool, "email.activation", map[string]string{
		"to":               "a@example.com",
		"activation_token": "secret-token",
	}); err != nil {
		t.Fatal(err)
	}

	router := NewRouter()
	router.Register("email.activation", okHandler{}, RedactOnSuccess("activation_token"))

	worker := NewWorker(NewRepository(pool), router, quietLogger())
	if err := worker.process(ctx); err != nil {
		t.Fatal(err)
	}

	var hasToken bool
	var to string
	if err := pool.QueryRow(ctx,
		`SELECT payload ? 'activation_token', payload->>'to' FROM outbox_events`,
	).Scan(&hasToken, &to); err != nil {
		t.Fatal(err)
	}
	if hasToken {
		t.Fatal("activation token still stored after the email was sent")
	}
	if to != "a@example.com" {
		t.Fatalf("non-secret fields should be kept, got to=%q", to)
	}
}

func TestDeadLetterCanBeRequeued(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	repo := NewRepository(pool)

	if err := Enqueue(ctx, pool, "test.event", map[string]string{}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE outbox_events SET failed_at = now(), attempts = 8, last_error = 'smtp down'`); err != nil {
		t.Fatal(err)
	}

	failed, err := repo.ListFailed(ctx, 10)
	if err != nil || len(failed) != 1 || *failed[0].LastError != "smtp down" {
		t.Fatalf("ListFailed: %v %+v", err, failed)
	}

	if err := repo.RetryFailed(ctx, failed[0].ID); err != nil {
		t.Fatal(err)
	}
	if events, _ := repo.Claim(ctx, 10); len(events) != 1 {
		t.Fatalf("requeued event not claimable: %d", len(events))
	}
	if err := repo.RetryFailed(ctx, failed[0].ID); !errors.Is(err, ErrNotFailed) {
		t.Fatalf("retrying a non-failed event: %v", err)
	}
}
