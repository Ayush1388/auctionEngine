DROP TRIGGER IF EXISTS ledger_entries_append_only ON ledger_entries;
DROP TRIGGER IF EXISTS ledger_transactions_append_only ON ledger_transactions;
DROP FUNCTION IF EXISTS ledger_append_only();

ALTER TABLE auctions DROP COLUMN IF EXISTS min_increment;

DROP TABLE IF EXISTS bid_reservations;

CREATE TABLE bid_reservations (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    auction_id UUID NOT NULL REFERENCES auctions(id),
    amount BIGINT NOT NULL
);

CREATE TABLE wallet_transactions (
    id UUID PRIMARY KEY,
    seller_id UUID NOT NULL REFERENCES users(id),
    buyer_id UUID NOT NULL REFERENCES users(id),
    auction_id UUID NOT NULL REFERENCES auctions(id),
    amount BIGINT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL
);

DROP INDEX IF EXISTS bids_auction_created_idx;
DROP INDEX IF EXISTS bids_idempotency_idx;
ALTER TABLE bids
    DROP COLUMN IF EXISTS idempotency_key,
    ALTER COLUMN created_at DROP DEFAULT;

ALTER TABLE auctions
    DROP CONSTRAINT IF EXISTS auctions_bid_consistency,
    DROP COLUMN IF EXISTS settled_at,
    DROP COLUMN IF EXISTS extensions,
    DROP COLUMN IF EXISTS version,
    DROP COLUMN IF EXISTS bid_count,
    DROP COLUMN IF EXISTS current_bidder_id;

DROP TABLE IF EXISTS ledger_entries;
DROP TABLE IF EXISTS ledger_transactions;

ALTER TABLE wallets
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_at,
    DROP COLUMN IF EXISTS reserved_amount;
