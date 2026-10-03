-- v1.0: carry the distributed-tracing context across the outbox.
--
-- The request that enqueued an event has finished long before the worker
-- picks the event up, so the W3C trace context ({"traceparent": "00-…"})
-- is saved with the row and restored by the worker. The worker's span (and
-- the Kafka record, and the bid worker after it) then joins the original
-- request's trace instead of starting a disconnected one.
--
-- NULL when tracing is off or the event was enqueued outside a request.
ALTER TABLE outbox_events ADD COLUMN trace_context JSONB;
