CREATE TABLE wallet_transactions (
    id UUID PRIMARY KEY,
    seller_id UUID NOT NULL REFERENCES users(id),
    buyer_id UUID NOT NULL REFERENCES users(id),
    auction_id UUID NOT NULL REFERENCES auctions(id),
    amount BIGINT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL
);