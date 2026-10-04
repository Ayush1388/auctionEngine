# 0005. Services own transaction boundaries

- **Status:** Accepted
- **Date:** 2026-09

## Context
Registration originally opened its transaction inside `user.Repository.Create`, which also wrote the outbox row with its own copy of the outbox SQL. That only works while one repository method is the whole unit of work. Creating an auction writes an item **and** an auction. The lifecycle worker updates an auction **and** enqueues an event. Neither fits inside one repository.

## Decision
- Repositories take a `database.DBTX`, an interface satisfied by the pool, a connection and a transaction, and never begin transactions themselves.
- `repo.WithTx(tx)` returns a copy bound to a transaction.
- The **service** decides what belongs in one transaction, using `database.WithTx(ctx, pool, fn)`.
- `outbox.Enqueue(ctx, db, type, payload)` is the only code that writes to `outbox_events`.

```go
err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
    if err := s.repository.WithTx(tx).Create(ctx, user, ...); err != nil {
        return err
    }
    return outbox.Enqueue(ctx, tx, email.EventTypeActivationEmail, event)
})
```

## Alternatives considered
- **Unit-of-work object holding every repository** – more ceremony than a project this size needs.
- **Transaction in `context.Context`** – hidden control flow; easy to forget which queries are inside it.

## Consequences
- Business rules that span tables live in one readable place, the service.
- Repositories stay simple and testable with either a pool or a transaction.
- Services now depend on something that can begin a transaction, so their constructors take the pool.
