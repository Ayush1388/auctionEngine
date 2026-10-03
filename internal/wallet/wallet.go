// Package wallet owns user balances and the ledger that explains them.
//
// # Model
//
// Each user has one wallet with two balances:
//
//	available  money that can be spent on new bids
//	reserved   money held for bids the user is currently winning
//
// Balances never change directly. Every change is a ledger transaction
// ("journal") made of lines whose amounts sum to zero. Money is only ever
// moved between accounts, never created or destroyed, which is the core rule
// of double-entry bookkeeping:
//
//	deposit ₹500      external  -500   →  alice.available +500
//	bid ₹300          alice.available -300  →  alice.reserved +300
//	outbid            alice.reserved  -300  →  alice.available +300
//	auction won       bob.reserved    -300  →  seller.available +300
//
// The wallets table is a projection of the ledger kept in the same
// transaction, so reads are one row lookup. Reconcile recomputes balances
// from the ledger to prove the two never disagree.
//
// # Invariants and who enforces them
//
//   - Balances are never negative: CHECK constraints on wallets. Even if
//     application code raced, PostgreSQL refuses to commit a negative balance.
//   - Every journal sums to zero: validated in Post before anything is written.
//   - Ledger rows are never updated or deleted: nothing in the code does so;
//     corrections are new journals (an audit trail you can replay).
package wallet

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Account is one of the balances a ledger line moves money in or out of.
type Account string

const (
	Available Account = "available"
	Reserved  Account = "reserved"

	// External represents the outside world (a payment provider). A deposit
	// moves money from External into a user's available balance, so the
	// journal still sums to zero.
	External Account = "external"
)

// Kind says why money moved. It is stored on every ledger transaction.
type Kind string

const (
	KindDeposit Kind = "DEPOSIT" // money enters from outside
	KindReserve Kind = "RESERVE" // held for a winning bid
	KindRelease Kind = "RELEASE" // returned when outbid
	KindSettle  Kind = "SETTLE"  // winner pays the seller when the auction ends
)

var (
	// ErrInsufficientFunds means a line would make a balance negative.
	ErrInsufficientFunds = errors.New("insufficient funds")

	// ErrUnbalanced means a journal's lines don't sum to zero: a bug in the
	// caller, never a user error.
	ErrUnbalanced = errors.New("ledger journal does not balance")

	ErrInvalidAmount = errors.New("amount must be positive")
)

type Wallet struct {
	UserID    uuid.UUID `json:"user_id"`
	Available int64     `json:"available"`
	Reserved  int64     `json:"reserved"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Total is everything the user owns: spendable plus held.
func (w Wallet) Total() int64 {
	return w.Available + w.Reserved
}
