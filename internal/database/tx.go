package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX is satisfied by *pgxpool.Pool, *pgxpool.Conn and pgx.Tx, so the same
// repository code works inside or outside a transaction. The caller decides.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// TxBeginner is anything that can start a transaction, usually the pool.
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// WithTx runs fn inside a transaction. It commits if fn returns nil and rolls
// back otherwise, including when fn panics.
func WithTx(
	ctx context.Context,
	db TxBeginner,
	fn func(tx pgx.Tx) error,
) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Rollback after a successful Commit is a no-op, so this is always safe.
	defer tx.Rollback(context.Background())

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
