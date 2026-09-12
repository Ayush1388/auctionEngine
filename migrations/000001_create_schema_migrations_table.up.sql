CREATE TABLE schema_migrations (
    version BIGINT PRIMARY KEY,
    applied_at TIMESTAMP(0) WITH TIME ZONE NOT NULL DEFAULT NOW()
);