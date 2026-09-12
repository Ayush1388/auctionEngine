CREATE TABLE bids (
    id UUID PRIMARY KEY,
    auction_id UUID NOT NULL REFERENCES auctions(id),
    user_id UUID NOT NULL REFERENCES users(id),
    amount BIGINT NOT NULL
        CHECK (amount > 0),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL
);