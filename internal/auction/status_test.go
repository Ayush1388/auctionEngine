package auction

import (
	"errors"
	"testing"
)

var allStatuses = []Status{StatusNotActive, StatusActive, StatusCompleted, StatusCancelled}

func TestTransition(t *testing.T) {
	allowed := map[[2]Status]bool{
		{StatusNotActive, StatusActive}:    true,
		{StatusNotActive, StatusCancelled}: true,
		{StatusActive, StatusCompleted}:    true,
	}

	// Check every pair, so a new transition can't be added by accident.
	for _, from := range allStatuses {
		for _, to := range allStatuses {
			name := string(from) + "->" + string(to)

			t.Run(name, func(t *testing.T) {
				got, err := from.Transition(to)
				want := allowed[[2]Status{from, to}]

				if want {
					if err != nil {
						t.Fatalf("expected transition to be allowed, got %v", err)
					}
					if got != to {
						t.Fatalf("got status %s, want %s", got, to)
					}
					return
				}

				if !errors.Is(err, ErrInvalidTransition) {
					t.Fatalf("got err %v, want ErrInvalidTransition", err)
				}
				if got != from {
					t.Fatalf("status changed to %s on a rejected transition", got)
				}
			})
		}
	}
}

func TestTransitionUnknownStatus(t *testing.T) {
	tests := []struct {
		name     string
		from, to Status
	}{
		{"unknown source", Status("PAUSED"), StatusActive},
		{"unknown target", StatusNotActive, Status("PAUSED")},
		{"empty", Status(""), StatusActive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.from.Transition(tt.to); !errors.Is(err, ErrUnknownStatus) {
				t.Fatalf("got %v, want ErrUnknownStatus", err)
			}
		})
	}
}

func TestTerminal(t *testing.T) {
	tests := map[Status]bool{
		StatusNotActive: false,
		StatusActive:    false,
		StatusCompleted: true,
		StatusCancelled: true,
		Status("NOPE"):  false,
	}

	for status, want := range tests {
		if got := status.Terminal(); got != want {
			t.Errorf("%s.Terminal() = %v, want %v", status, got, want)
		}
	}
}

func TestParseStatus(t *testing.T) {
	for _, s := range allStatuses {
		got, err := ParseStatus(string(s))
		if err != nil || got != s {
			t.Errorf("ParseStatus(%q) = %q, %v", s, got, err)
		}
	}

	for _, bad := range []string{"", "active", "DONE"} {
		if _, err := ParseStatus(bad); !errors.Is(err, ErrUnknownStatus) {
			t.Errorf("ParseStatus(%q): got %v, want ErrUnknownStatus", bad, err)
		}
	}
}

func TestSourcesOf(t *testing.T) {
	tests := map[Status][]Status{
		StatusActive:    {StatusNotActive},
		StatusCompleted: {StatusActive},
		StatusCancelled: {StatusNotActive},
		StatusNotActive: nil,
	}

	for target, want := range tests {
		got := SourcesOf(target)
		if len(got) != len(want) {
			t.Fatalf("SourcesOf(%s) = %v, want %v", target, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("SourcesOf(%s) = %v, want %v", target, got, want)
			}
		}
	}
}
