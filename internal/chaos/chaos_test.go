package chaos

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSwitchboard(t *testing.T) {
	t.Cleanup(func() { Reset(); enabled.Store(false) })

	if err := Set(Redis, true); !errors.Is(err, ErrDisabled) {
		t.Fatalf("a disabled switchboard must refuse: %v", err)
	}
	if Fail(Redis) != nil || Active(Redis) {
		t.Fatal("nothing may fail while disabled")
	}

	Enable()
	if err := Set("nope", true); err == nil {
		t.Fatal("unknown fault accepted")
	}
	if err := Set(Elasticsearch, true); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(Fail(Elasticsearch), ErrInjected) || Fail(Redis) != nil {
		t.Fatal("only the chosen fault fails")
	}
	Reset()
	if Fail(Elasticsearch) != nil {
		t.Fatal("reset must clear faults")
	}
}

func TestDelayOnlyWhenSlow(t *testing.T) {
	t.Cleanup(func() { Reset(); enabled.Store(false) })
	Enable()
	SetSlowQuery(30 * time.Millisecond)

	start := time.Now()
	Delay(context.Background())
	if time.Since(start) > 15*time.Millisecond {
		t.Fatal("delayed while the fault is off")
	}
	_ = Set(PostgresSlow, true)
	start = time.Now()
	Delay(context.Background())
	if time.Since(start) < 25*time.Millisecond {
		t.Fatal("did not delay while the fault is on")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start = time.Now()
	Delay(ctx)
	if time.Since(start) > 15*time.Millisecond {
		t.Fatal("a cancelled context must end the delay")
	}
}
