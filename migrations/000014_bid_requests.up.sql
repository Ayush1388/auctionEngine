-- v0.8: asynchronous bids through Kafka.
--
-- POST /v1/auctions/{id}/bids with "Prefer: respond-async" doesn't place the
-- bid in the request. It records the *request* here and enqueues a
-- bid.requested outbox event in the same transaction, then returns 202.
-- The outbox relay publishes the event to Kafka (key = auction_id), a bid
-- worker places the bid, and writes the outcome back to this row, which the
-- client reads with GET /v1/bid-requests/{id}.
CREATE TABLE bid_requests (
    id UUID PRIMARY KEY,
    auction_id UUID NOT NULL REFERENCES auctions(id),
    user_id UUID NOT NULL REFERENCES users(id),
    amount BIGINT NOT NULL CHECK (amount > 0),

    -- The client's Idempotency-Key, if sent: retrying the same request
    -- returns the same bid request instead of queueing a second bid.
    idempotency_key TEXT,

    status TEXT NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING', 'ACCEPTED', 'REJECTED', 'FAILED')),
    reason TEXT,
    bid_id UUID,
    -- What the worker saw, for the response.
    minimum_amount BIGINT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX bid_requests_idempotency_idx
    ON bid_requests (user_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX bid_requests_user_idx ON bid_requests (user_id, created_at DESC);
