DROP INDEX IF EXISTS outbox_events_pending_idx;

CREATE INDEX outbox_events_pending_idx
ON outbox_events (available_at, created_at)
WHERE processed_at IS NULL;

ALTER TABLE outbox_events DROP COLUMN IF EXISTS failed_at;
