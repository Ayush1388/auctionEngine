-- Proxy (maximum) bidding: a bidder states the most they will pay and the
-- engine raises their bid by the minimum increment, up to that maximum, when
-- someone outbids them. The maximum is a private instruction, not a
-- reservation: only the visible bid is ever held.
CREATE TABLE proxy_bids (
    auction_id UUID NOT NULL REFERENCES auctions(id),
    user_id    UUID NOT NULL REFERENCES users(id),
    max_amount BIGINT NOT NULL CHECK (max_amount > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (auction_id, user_id)
);

-- True for a bid the engine placed on a bidder's behalf.
ALTER TABLE bids ADD COLUMN auto BOOLEAN NOT NULL DEFAULT false;
