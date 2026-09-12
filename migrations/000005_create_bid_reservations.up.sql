CREATE TABLE bid_reservations (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    auction_id UUID NOT NULL REFERENCES auctions(id),
    amount BIGINT NOT NULL
);