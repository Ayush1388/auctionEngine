-- v0.3: bidding, reservations and a double-entry ledger.
--
-- Money model
-- -----------
-- Every user has one wallet with two balances:
--   available_amount  money they can spend on new bids
--   reserved_amount   money held for bids they are currently winning
-- Both are guarded by CHECK (>= 0). The CHECKs are the last line of defence:
-- even if application code had a race, PostgreSQL refuses to commit a
-- negative balance, so double spending is impossible at the storage layer.
--
-- The wallet row is a fast-to-read *projection*. The source of truth is the
-- append-only ledger below: every change to a balance is recorded as ledger
-- entries, and the entries of one ledger transaction always sum to zero
-- (double-entry bookkeeping: money is moved, never created or destroyed).

ALTER TABLE wallets
    ADD COLUMN reserved_amount BIGINT NOT NULL DEFAULT 0
        CHECK (reserved_amount >= 0),
    ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- One row per money movement (a "journal").
CREATE TABLE ledger_transactions (
    id UUID PRIMARY KEY,
    kind TEXT NOT NULL
        CHECK (kind IN ('DEPOSIT', 'RESERVE', 'RELEASE', 'SETTLE')),
    auction_id UUID REFERENCES auctions(id),
    bid_id UUID,
    -- Makes deposits idempotent: retrying the same request can't pay twice.
    idempotency_key TEXT UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The lines of a journal. user_id is NULL for the 'external' account, which
-- stands for money entering the system (deposits from a payment provider).
CREATE TABLE ledger_entries (
    id BIGSERIAL PRIMARY KEY,
    transaction_id UUID NOT NULL REFERENCES ledger_transactions(id),
    user_id UUID REFERENCES users(id),
    account TEXT NOT NULL
        CHECK (account IN ('available', 'reserved', 'external')),
    amount BIGINT NOT NULL CHECK (amount <> 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((account = 'external') = (user_id IS NULL))
);

CREATE INDEX ledger_entries_user_idx
    ON ledger_entries (user_id, id DESC)
    WHERE user_id IS NOT NULL;

CREATE INDEX ledger_entries_transaction_idx
    ON ledger_entries (transaction_id);

-- Auctions track who is winning, how many bids there are, a version for
-- optimistic locking, and when settlement ran.
ALTER TABLE auctions
    ADD COLUMN current_bidder_id UUID REFERENCES users(id),
    ADD COLUMN bid_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN extensions INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN settled_at TIMESTAMPTZ,
    ADD CONSTRAINT auctions_bid_consistency CHECK (
        (current_bid IS NULL) = (current_bidder_id IS NULL)
    );

-- Bids: history of every accepted bid.
ALTER TABLE bids
    ALTER COLUMN created_at SET DEFAULT now(),
    ADD COLUMN idempotency_key TEXT;

-- The same client retry (same user + key) can create at most one bid.
CREATE UNIQUE INDEX bids_idempotency_idx
    ON bids (user_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

-- Bid history for an auction, newest first, keyset paginated.
CREATE INDEX bids_auction_created_idx
    ON bids (auction_id, created_at DESC, id DESC);

-- The old placeholder tables were never used; replace them.
DROP TABLE IF EXISTS wallet_transactions;
DROP TABLE IF EXISTS bid_reservations;

-- A reservation is money held for the current highest bid of an auction.
CREATE TABLE bid_reservations (
    id UUID PRIMARY KEY,
    auction_id UUID NOT NULL REFERENCES auctions(id),
    user_id UUID NOT NULL REFERENCES users(id),
    bid_id UUID NOT NULL REFERENCES bids(id),
    amount BIGINT NOT NULL CHECK (amount > 0),
    status TEXT NOT NULL
        CHECK (status IN ('ACTIVE', 'RELEASED', 'SETTLED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at TIMESTAMPTZ
);

-- At most one ACTIVE reservation per auction: only the winner holds money.
-- If a bug ever tried to leave two, the insert fails.
CREATE UNIQUE INDEX bid_reservations_one_active_idx
    ON bid_reservations (auction_id)
    WHERE status = 'ACTIVE';

CREATE INDEX bid_reservations_user_idx
    ON bid_reservations (user_id, created_at DESC);

-- Minimum amount a new bid must beat the current one by (smallest currency
-- unit). Sellers can set it when creating the auction.
ALTER TABLE auctions
    ADD COLUMN min_increment BIGINT NOT NULL DEFAULT 1
        CHECK (min_increment > 0);

-- Every existing user gets an empty wallet. New users get one lazily
-- (wallet.EnsureWallet), so the users module stays unaware of wallets.
INSERT INTO wallets (user_id)
SELECT id FROM users
ON CONFLICT (user_id) DO NOTHING;

-- The ledger is append-only. Corrections are new journals, never edits,
-- so the history can always be replayed. Enforced here, not just by
-- convention: an UPDATE or DELETE on either table raises an error.
CREATE FUNCTION ledger_append_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER ledger_transactions_append_only
    BEFORE UPDATE OR DELETE ON ledger_transactions
    FOR EACH ROW EXECUTE FUNCTION ledger_append_only();

CREATE TRIGGER ledger_entries_append_only
    BEFORE UPDATE OR DELETE ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION ledger_append_only();
