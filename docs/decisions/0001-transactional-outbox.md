# 0001. Transactional outbox for emails and side effects

- **Status:** Accepted
- **Date:** 2026-09

## Context
Registering a user writes to Postgres and sends an activation email over SMTP. These are two systems with no shared transaction. Sending the email inside the request handler means an SMTP outage either fails the signup or, if the error is ignored, leaves a user who never gets an activation email. Committing first and then sending means a crash between the two loses the email silently.

## Decision
The user row and an `outbox_events` row are inserted in **one database transaction**. A background worker reads pending events and delivers them, marking each one processed only after it succeeds.

## Alternatives considered
- **Send synchronously in the handler** – couples signup availability to SMTP availability.
- **Fire-and-forget goroutine** – loses the email if the process dies before it runs.
- **Message broker (Kafka, RabbitMQ)** – solves delivery but not the dual write, and adds infrastructure this project doesn't need yet.

## Consequences
- Delivery is **at-least-once**: a worker can send an email and crash before marking it done, so it may be sent twice. Handlers must tolerate duplicates.
- Emails arrive a little later (one polling interval).
- The same table will carry future events such as `auction.completed`.
