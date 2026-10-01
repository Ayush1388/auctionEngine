# Decision records

Short notes on choices that shaped the system: what was decided, why, and what it cost.
They are written once and rarely edited. If a decision changes, add a new note that supersedes the old one.

| # | Decision | Status |
|---|---|---|
| [0001](0001-transactional-outbox.md) | Transactional outbox for emails and side effects | Accepted |
| [0002](0002-argon2id-password-hashing.md) | Argon2id for password hashing | Accepted |
| [0003](0003-skip-locked-workers.md) | `FOR UPDATE SKIP LOCKED` for background workers | Accepted |
| [0004](0004-hand-written-migration-runner.md) | Hand-written migration runner | Accepted |
| [0005](0005-transactions-owned-by-services.md) | Services own transaction boundaries | Accepted |
| [0006](0006-bid-concurrency.md) | Serialising bids with row locks and a global lock order | Accepted |
| [0007](0007-double-entry-ledger.md) | Double-entry, append-only ledger | Accepted |
| [0008](0008-access-and-refresh-tokens.md) | Short access tokens, rotating refresh tokens, per-account throttling | Accepted |
| [0009](0009-redis-as-a-disposable-accelerator.md) | Redis as a disposable accelerator | Accepted |

To add one, copy [`template.md`](template.md) and take the next number.
