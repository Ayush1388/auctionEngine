DROP INDEX IF EXISTS items_name_prefix_idx;
DROP INDEX IF EXISTS items_search_vector_idx;
ALTER TABLE items DROP COLUMN IF EXISTS search_vector;
DROP TRIGGER IF EXISTS auctions_bump_version ON auctions;
DROP FUNCTION IF EXISTS auctions_bump_version();
