# Decision records

Short notes on choices that shaped the system: what was decided, why, and what it cost.
They are written once and rarely edited. If a decision changes, add a new note that supersedes the old one.

| # | Decision | Status |
|---|---|---|
| [0001](0001-transactional-outbox.md) | Transactional outbox for emails and side effects | Accepted |
| [0002](0002-argon2id-password-hashing.md) | Argon2id for password hashing | Accepted |
| [0003](0003-skip-locked-workers.md) | `FOR UPDATE SKIP LOCKED` for background workers | Accepted |
| [0004](0004-hand-written-migration-runner.md) | Hand-written migration runner | Accepted |

To add one, copy [`template.md`](template.md) and take the next number.
