# 0007. A double-entry, append-only ledger behind every wallet

- **Status:** Accepted
- **Date:** 2026-10

## Context
Wallets hold real value. A `balance` column that code increments and decrements can't answer "why is this balance what it is?", can't be audited, and silently absorbs bugs.

## Decision
- Every money movement is a **journal** (`ledger_transactions`) of **lines** (`ledger_entries`) whose amounts sum to **zero**: money moves between accounts (`available`, `reserved`, and an `external` account for deposits), never appears or disappears.
- The `wallets` row is a projection updated in the same transaction, so reads stay one row lookup.
- Ledger tables are **append-only**, enforced by triggers. Corrections are new journals.
- `wallet.Reconcile` recomputes every balance from the ledger and checks: wallets = ledger, journals sum to zero, total wallet money = money deposited, reserved money = active reservations. Tests run it after every concurrency scenario.
- Deposits and settlements carry idempotency keys with a unique constraint, so a replayed request or redelivered event can't pay twice.

## Consequences
- More rows per operation (a bid writes up to two journals).
- Any drift between wallets and the ledger is detectable, and every paisa has a history.
