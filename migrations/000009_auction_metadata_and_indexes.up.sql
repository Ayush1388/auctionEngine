-- Items belong to the user who listed them.
ALTER TABLE items
    ADD COLUMN owner_id UUID NOT NULL REFERENCES users(id),
    ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE auctions
    ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- An item is sold in at most one auction.
    ADD CONSTRAINT auctions_item_id_key UNIQUE (item_id);

-- Allow sellers to cancel an auction before it starts.
ALTER TABLE auctions DROP CONSTRAINT auctions_status_check;
ALTER TABLE auctions ADD CONSTRAINT auctions_status_check
    CHECK (status IN ('NOT_ACTIVE', 'ACTIVE', 'COMPLETED', 'CANCELLED'));

-- Listing, newest first, with keyset pagination on (created_at, id).
CREATE INDEX auctions_created_idx
    ON auctions (created_at DESC, id DESC);

CREATE INDEX auctions_status_created_idx
    ON auctions (status, created_at DESC, id DESC);

-- "My auctions".
CREATE INDEX auctions_owner_created_idx
    ON auctions (owner_id, created_at DESC, id DESC);

-- The lifecycle worker only ever looks at auctions that are due to move.
-- Partial indexes stay small because finished auctions drop out of them.
CREATE INDEX auctions_pending_start_idx
    ON auctions (starts_at)
    WHERE status = 'NOT_ACTIVE';

CREATE INDEX auctions_pending_end_idx
    ON auctions (ends_at)
    WHERE status = 'ACTIVE';
