-- schema_migrations is owned by the migration runner and is never dropped,
-- otherwise the runner could not record this rollback.
SELECT 1;
