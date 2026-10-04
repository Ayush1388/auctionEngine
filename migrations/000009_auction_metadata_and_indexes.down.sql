DROP INDEX IF EXISTS auctions_pending_end_idx;
DROP INDEX IF EXISTS auctions_pending_start_idx;
DROP INDEX IF EXISTS auctions_owner_created_idx;
DROP INDEX IF EXISTS auctions_status_created_idx;
DROP INDEX IF EXISTS auctions_created_idx;

-- Cancelled auctions can't satisfy the old constraint.
DELETE FROM auctions WHERE status = 'CANCELLED';

ALTER TABLE auctions DROP CONSTRAINT auctions_status_check;
ALTER TABLE auctions ADD CONSTRAINT auctions_status_check
    CHECK (status IN ('NOT_ACTIVE', 'ACTIVE', 'COMPLETED'));

ALTER TABLE auctions
    DROP CONSTRAINT IF EXISTS auctions_item_id_key,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_at;

ALTER TABLE items
    DROP COLUMN IF EXISTS created_at,
    DROP COLUMN IF EXISTS owner_id;
