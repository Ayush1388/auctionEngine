-- v1.0: wake the outbox worker the moment an event is committed.
--
-- Before, the worker polled every 2 seconds, so a live bid reached
-- WebSocket watchers, the cache and Kafka up to 2 s late, and the worker
-- spent idle time querying an empty table.
--
-- Now every INSERT into outbox_events also sends a NOTIFY on channel
-- 'outbox_events'. PostgreSQL delivers notifications only when the
-- transaction COMMITS (never for a rollback), and collapses duplicates
-- within one transaction, so the worker gets exactly the signal it needs:
-- "there is committed work". Polling stays as a fallback for a missed
-- notification (for example, while the listening connection reconnects).
--
-- FOR EACH STATEMENT, not FOR EACH ROW: one notification per INSERT
-- statement is enough, and costs no extra round trip for the application.
CREATE FUNCTION outbox_events_notify() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_notify('outbox_events', '');
    RETURN NULL;
END;
$$;

CREATE TRIGGER outbox_events_notify
AFTER INSERT ON outbox_events
FOR EACH STATEMENT
EXECUTE FUNCTION outbox_events_notify();
