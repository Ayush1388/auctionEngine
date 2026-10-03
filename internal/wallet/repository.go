package wallet

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ayush1388/auctionEngine/internal/database"
)

// EnsureWallet creates an empty wallet for userID if it doesn't have one.
// Wallets are created lazily (on first deposit or read) so the frozen
// users module doesn't need to know wallets exist.
func EnsureWallet(ctx context.Context, db database.DBTX, userID uuid.UUID) error {
	_, err := db.Exec(ctx, `
		INSERT INTO wallets (user_id) VALUES ($1)
		ON CONFLICT (user_id) DO NOTHING
	`, userID)
	if err != nil {
		return fmt.Errorf("ensure wallet: %w", err)
	}
	return nil
}

func Get(ctx context.Context, db database.DBTX, userID uuid.UUID) (Wallet, error) {
	w := Wallet{UserID: userID}

	err := db.QueryRow(ctx, `
		SELECT available_amount, reserved_amount, updated_at
		FROM wallets WHERE user_id = $1
	`, userID).Scan(&w.Available, &w.Reserved, &w.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// No wallet yet is simply a zero balance.
		return w, nil
	}
	if err != nil {
		return Wallet{}, fmt.Errorf("get wallet: %w", err)
	}
	return w, nil
}

// Entry is one ledger line as shown in a user's statement.
type Entry struct {
	ID            int64      `json:"id"`
	TransactionID uuid.UUID  `json:"transaction_id"`
	Kind          Kind       `json:"kind"`
	Account       Account    `json:"account"`
	Amount        int64      `json:"amount"`
	AuctionID     *uuid.UUID `json:"auction_id"`
	CreatedAt     time.Time  `json:"created_at"`
}

// ListEntries returns a user's ledger lines, newest first. It pages by the
// entry ID (a BIGSERIAL, so it only grows): pass the last ID you saw as
// before to get the next page.
func ListEntries(ctx context.Context, db database.DBTX, userID uuid.UUID, before int64, limit int) ([]Entry, error) {
	query := `
		SELECT e.id, e.transaction_id, t.kind, e.account, e.amount, t.auction_id, e.created_at
		FROM ledger_entries e
		JOIN ledger_transactions t ON t.id = e.transaction_id
		WHERE e.user_id = $1`
	args := []any{userID}

	if before > 0 {
		args = append(args, before)
		query += ` AND e.id < $` + strconv.Itoa(len(args))
	}

	args = append(args, limit)
	query += ` ORDER BY e.id DESC LIMIT $` + strconv.Itoa(len(args))

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list ledger entries: %w", err)
	}

	entries, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Entry])
	if err != nil {
		return nil, fmt.Errorf("list ledger entries: %w", err)
	}
	return entries, nil
}

// LockForUpdate locks the wallets of userIDs, in ascending user-ID order,
// and returns them keyed by user.
//
// Why the order matters (deadlock prevention): a bid can touch two wallets,
// the new bidder's and the previous leader's. Transaction 1 (Bob outbids
// Alice on auction X) and transaction 2 (Alice outbids Bob on auction Y)
// would otherwise lock "Alice then Bob" and "Bob then Alice". Each holds one
// lock and waits for the other; PostgreSQL detects the cycle and kills one
// with a deadlock error. If every transaction locks wallets in the same
// global order, no cycle can form. ORDER BY user_id inside one statement is
// that order.
//
// Wallets that don't exist yet are created first, so the lock always finds
// a row.
func LockForUpdate(ctx context.Context, db database.DBTX, userIDs ...uuid.UUID) (map[uuid.UUID]Wallet, error) {
	for _, id := range userIDs {
		if err := EnsureWallet(ctx, db, id); err != nil {
			return nil, err
		}
	}

	rows, err := db.Query(ctx, `
		SELECT user_id, available_amount, reserved_amount, updated_at
		FROM wallets
		WHERE user_id = ANY($1)
		ORDER BY user_id
		FOR UPDATE
	`, userIDs)
	if err != nil {
		return nil, fmt.Errorf("lock wallets: %w", err)
	}

	wallets, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Wallet])
	if err != nil {
		return nil, fmt.Errorf("lock wallets: %w", err)
	}

	out := make(map[uuid.UUID]Wallet, len(wallets))
	for _, w := range wallets {
		out[w.UserID] = w
	}
	return out, nil
}
