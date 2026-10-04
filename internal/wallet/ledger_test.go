package wallet

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestJournalMustBalance(t *testing.T) {
	alice := uuid.New()

	tests := []struct {
		name  string
		lines []Line
		ok    bool
	}{
		{"deposit", []Line{{Account: External, Amount: -500}, {UserID: alice, Account: Available, Amount: 500}}, true},
		{"reserve", []Line{{UserID: alice, Account: Available, Amount: -300}, {UserID: alice, Account: Reserved, Amount: 300}}, true},
		{"creates money", []Line{{Account: External, Amount: -500}, {UserID: alice, Account: Available, Amount: 600}}, false},
		{"one line", []Line{{UserID: alice, Account: Available, Amount: 500}}, false},
		{"zero line", []Line{{UserID: alice, Account: Available, Amount: 0}, {UserID: alice, Account: Reserved, Amount: 0}}, false},
		{"external with a user", []Line{{UserID: alice, Account: External, Amount: -5}, {UserID: alice, Account: Available, Amount: 5}}, false},
		{"user account without a user", []Line{{Account: External, Amount: -5}, {Account: Available, Amount: 5}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate(Journal{Kind: KindDeposit, Lines: tt.lines})
			if tt.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.ok && !errors.Is(err, ErrUnbalanced) {
				t.Fatalf("got %v, want ErrUnbalanced", err)
			}
		})
	}
}

func TestLinesAreLockedInUserOrder(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	if a.String() > b.String() {
		a, b = b, a
	}
	got := sortedLines([]Line{
		{UserID: b, Account: Available, Amount: 1},
		{UserID: a, Account: Reserved, Amount: -1},
	})
	if got[0].UserID != a || got[1].UserID != b {
		t.Fatalf("lines not sorted by user id: %+v", got)
	}
}
