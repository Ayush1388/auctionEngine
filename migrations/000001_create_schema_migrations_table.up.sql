-- The runner creates this table itself before applying anything,
-- so this migration only records that it exists.
CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT PRIMARY KEY,
    applied_at TIMESTAMP(0) WITH TIME ZONE NOT NULL DEFAULT NOW()
);
