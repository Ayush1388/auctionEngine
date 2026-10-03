package wallet

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Ayush1388/auctionEngine/internal/database"
)

// Journal is one atomic money movement: a set of lines that sum to zero.
type Journal struct {
	Kind      Kind
	AuctionID *uuid.UUID
	BidID     *uuid.UUID

	// IdempotencyKey, when set, makes posting the same journal twice a
	// no-op that returns ErrDuplicate (the UNIQUE constraint decides).
	IdempotencyKey string

	Lines []Line
}

// Line moves Amount into (positive) or out of (negative) one account.
// UserID is uuid.Nil for the External account.
type Line struct {
	UserID  uuid.UUID
	Account Account
	Amount  int64
}

// ErrDuplicate is returned when a journal with the same idempotency key was
// already posted. Callers treat it as "already done", not as a failure.
var ErrDuplicate = errors.New("ledger journal already posted")

// Post records j and applies it to wallet balances.
//
// It must run inside the caller's transaction (db is a pgx.Tx) so the money
// movement commits or rolls back together with whatever caused it: the bid,
// the reservation, the auction update.
//
// Deadlock avoidance: two concurrent journals that touch the same two
// wallets in opposite order (A then B, B then A) would each hold one row
// lock and wait for the other forever. PostgreSQL would detect it and abort
// one of them with SQLSTATE 40P01. Sorting lines by user ID means every
// transaction locks wallets in the same global order, so that cycle can't
// form.
func Post(ctx context.Context, db database.DBTX, j Journal) (uuid.UUID, error) {
	if err := validate(j); err != nil {
		return uuid.Nil, err
	}

	txID := uuid.New()

	var key *string
	if j.IdempotencyKey != "" {
		key = &j.IdempotencyKey
	}

	_, err := db.Exec(ctx, `
		INSERT INTO ledger_transactions (id, kind, auction_id, bid_id, idempotency_key)
		VALUES ($1, $2, $3, $4, $5)
	`, txID, j.Kind, j.AuctionID, j.BidID, key)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && key != nil {
			return uuid.Nil, ErrDuplicate
		}
		return uuid.Nil, fmt.Errorf("insert ledger transaction: %w", err)
	}

	lines := sortedLines(j.Lines)

	for _, line := range lines {
		if err := applyLine(ctx, db, txID, line); err != nil {
			return uuid.Nil, err
		}
	}

	return txID, nil
}

func validate(j Journal) error {
	if len(j.Lines) < 2 {
		return fmt.Errorf("%w: a journal needs at least two lines", ErrUnbalanced)
	}

	var sum int64
	for _, l := range j.Lines {
		if l.Amount == 0 {
			return fmt.Errorf("%w: zero-amount line", ErrUnbalanced)
		}
		if (l.Account == External) != (l.UserID == uuid.Nil) {
			return fmt.Errorf("%w: only the external account has no user", ErrUnbalanced)
		}
		sum += l.Amount
	}

	if sum != 0 {
		return fmt.Errorf("%w: lines sum to %d", ErrUnbalanced, sum)
	}
	return nil
}

// sortedLines orders lines by user ID (and account, for determinism) so that
// wallet rows are always locked in the same order. See Post.
func sortedLines(in []Line) []Line {
	out := append([]Line(nil), in...)
	sort.SliceStable(out, func(i, k int) bool {
		if c := bytes.Compare(out[i].UserID[:], out[k].UserID[:]); c != 0 {
			return c < 0
		}
		return out[i].Account < out[k].Account
	})
	return out
}

func applyLine(ctx context.Context, db database.DBTX, txID uuid.UUID, line Line) error {
	var userID *uuid.UUID
	if line.UserID != uuid.Nil {
		userID = &line.UserID
	}

	if _, err := db.Exec(ctx, `
		INSERT INTO ledger_entries (transaction_id, user_id, account, amount)
		VALUES ($1, $2, $3, $4)
	`, txID, userID, line.Account, line.Amount); err != nil {
		return fmt.Errorf("insert ledger entry: %w", err)
	}

	// The external account has no wallet row; it only exists in the ledger.
	if line.Account == External {
		return nil
	}

	// The column name comes from a fixed switch, never from input.
	column := "available_amount"
	if line.Account == Reserved {
		column = "reserved_amount"
	}

	// UPDATE takes a row lock on the wallet. If another transaction is
	// changing the same wallet, this waits for it, then applies the delta to
	// the committed value (no lost update). The CHECK (>= 0) constraint is
	// evaluated on the new value, so an overdraft fails here with 23514.
	result, err := db.Exec(ctx, `
		UPDATE wallets
		SET `+column+` = `+column+` + $2, updated_at = now()
		WHERE user_id = $1
	`, line.UserID, line.Amount)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23514" {
			return ErrInsufficientFunds
		}
		return fmt.Errorf("update wallet: %w", err)
	}

	// No wallet row yet means a zero balance: taking money out is an
	// overdraft; putting money in means the caller forgot EnsureWallet.
	if result.RowsAffected() == 0 {
		if line.Amount < 0 {
			return ErrInsufficientFunds
		}
		return fmt.Errorf("wallet for user %s does not exist", line.UserID)
	}

	return nil
}
