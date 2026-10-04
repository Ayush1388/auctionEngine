CREATE TABLE auctions (
    id UUID PRIMARY KEY,
    item_id UUID NOT NULL REFERENCES items(id),
    owner_id UUID NOT NULL REFERENCES users(id),
    starting_price BIGINT NOT NULL
        CHECK (starting_price >= 0),
    current_bid BIGINT
        CHECK (current_bid >= 0),
    starts_at TIMESTAMP WITH TIME ZONE NOT NULL,
    ends_at TIMESTAMP WITH TIME ZONE NOT NULL,
    status TEXT NOT NULL
        CHECK (status IN ('NOT_ACTIVE', 'ACTIVE', 'COMPLETED')),
    CHECK (ends_at > starts_at)
);