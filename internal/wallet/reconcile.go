package wallet

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ayush1388/auctionEngine/internal/database"
)

// Mismatch is a wallet whose stored balances differ from the ledger.
type Mismatch struct {
	UserID          uuid.UUID
	StoredAvailable int64
	LedgerAvailable int64
	StoredReserved  int64
	LedgerReserved  int64
}

// Report summarises a reconciliation run.
type Report struct {
	Mismatches []Mismatch

	// UnbalancedJournals lists ledger transactions whose lines don't sum
	// to zero. Post makes that impossible, so any entry here is corruption.
	UnbalancedJournals []uuid.UUID

	// ExternalTotal is the net money that entered the system (negative:
	// the external account is debited on deposits). -ExternalTotal must
	// equal the sum of every wallet.
	ExternalTotal int64
	WalletTotal   int64

	// Every paisa in wallets' reserved balances must be backed by an ACTIVE
	// bid reservation, and vice versa.
	ReservedTotal           int64
	ActiveReservationsTotal int64
}

// OK reports whether the books balance.
func (r Report) OK() bool {
	return len(r.Mismatches) == 0 &&
		len(r.UnbalancedJournals) == 0 &&
		-r.ExternalTotal == r.WalletTotal &&
		r.ReservedTotal == r.ActiveReservationsTotal
}

// Reconcile recomputes every balance from the ledger and compares it with
// the wallets table. Real payment systems run this nightly; here tests run it
// after concurrency storms to prove no money was created or lost.
func Reconcile(ctx context.Context, db database.DBTX) (Report, error) {
	var report Report

	rows, err := db.Query(ctx, `
		WITH ledger AS (
			SELECT user_id,
			       COALESCE(SUM(amount) FILTER (WHERE account = 'available'), 0) AS available,
			       COALESCE(SUM(amount) FILTER (WHERE account = 'reserved'), 0) AS reserved
			FROM ledger_entries
			WHERE user_id IS NOT NULL
			GROUP BY user_id
		)
		SELECT COALESCE(w.user_id, l.user_id),
		       COALESCE(w.available_amount, 0), COALESCE(l.available, 0),
		       COALESCE(w.reserved_amount, 0), COALESCE(l.reserved, 0)
		FROM wallets w
		FULL OUTER JOIN ledger l ON l.user_id = w.user_id
		WHERE COALESCE(w.available_amount, 0) <> COALESCE(l.available, 0)
		   OR COALESCE(w.reserved_amount, 0) <> COALESCE(l.reserved, 0)
	`)
	if err != nil {
		return report, fmt.Errorf("reconcile balances: %w", err)
	}
	report.Mismatches, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Mismatch])
	if err != nil {
		return report, fmt.Errorf("reconcile balances: %w", err)
	}

	rows, err = db.Query(ctx, `
		SELECT transaction_id FROM ledger_entries
		GROUP BY transaction_id HAVING SUM(amount) <> 0
	`)
	if err != nil {
		return report, fmt.Errorf("reconcile journals: %w", err)
	}
	report.UnbalancedJournals, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return report, fmt.Errorf("reconcile journals: %w", err)
	}

	if err := db.QueryRow(ctx, `
		SELECT
			(SELECT COALESCE(SUM(amount), 0) FROM ledger_entries WHERE account = 'external'),
			(SELECT COALESCE(SUM(available_amount + reserved_amount), 0) FROM wallets),
			(SELECT COALESCE(SUM(reserved_amount), 0) FROM wallets),
			(SELECT COALESCE(SUM(amount), 0) FROM bid_reservations WHERE status = 'ACTIVE')
	`).Scan(&report.ExternalTotal, &report.WalletTotal, &report.ReservedTotal, &report.ActiveReservationsTotal); err != nil {
		return report, fmt.Errorf("reconcile totals: %w", err)
	}

	return report, nil
}
