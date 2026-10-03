package outbox

import (
	"context"
	"testing"
)

func TestRouter(t *testing.T) {
	r := NewRouter()

	var got string
	r.Register("a", HandlerFunc(func(ctx context.Context, e Event) error {
		got = e.EventType
		return nil
	}))

	if err := r.Handle(context.Background(), Event{EventType: "a"}); err != nil || got != "a" {
		t.Fatalf("registered type: err=%v got=%q", err, got)
	}

	if err := r.Handle(context.Background(), Event{EventType: "missing"}); err == nil {
		t.Fatal("expected an error for an unregistered type")
	}

	// A second handler for the same type also runs (fan-out).
	calls := 0
	r.Register("a", HandlerFunc(func(context.Context, Event) error { calls++; return nil }))
	if err := r.Handle(context.Background(), Event{EventType: "a"}); err != nil || calls != 1 {
		t.Fatalf("fan-out: err=%v calls=%d", err, calls)
	}
}
