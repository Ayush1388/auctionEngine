-- v0.6: search.
--
-- 1. auctions.version must increase on EVERY update, not only the ones that
--    remember to write "version = version + 1". The search indexer sends it
--    to Elasticsearch as an external version, so a slow, out-of-date update
--    can never overwrite a newer document. A trigger is the only way to
--    guarantee it for every current and future UPDATE.
CREATE FUNCTION auctions_bump_version() RETURNS trigger AS $$
BEGIN
    NEW.version := OLD.version + 1;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER auctions_bump_version
    BEFORE UPDATE ON auctions
    FOR EACH ROW EXECUTE FUNCTION auctions_bump_version();

-- 2. PostgreSQL full-text search, used when Elasticsearch is not configured
--    or is down. A generated column keeps the tsvector in sync with the row
--    automatically; weights rank title matches above type and description.
ALTER TABLE items ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(name, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(type, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(description, '')), 'C')
    ) STORED;

-- GIN is an inverted index: term -> rows containing it. That is the same
-- idea Elasticsearch is built on.
CREATE INDEX items_search_vector_idx ON items USING GIN (search_vector);

-- Prefix suggestions ("ipho" -> "iPhone 15"): text_pattern_ops makes
-- LIKE 'prefix%' use a B-tree index.
CREATE INDEX items_name_prefix_idx ON items (lower(name) text_pattern_ops);
