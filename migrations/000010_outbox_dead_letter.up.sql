-- Events that keep failing are parked here instead of retrying forever.
ALTER TABLE outbox_events ADD COLUMN failed_at TIMESTAMPTZ;

DROP INDEX IF EXISTS outbox_events_pending_idx;

CREATE INDEX outbox_events_pending_idx
ON outbox_events (available_at, created_at)
WHERE processed_at IS NULL AND failed_at IS NULL;
