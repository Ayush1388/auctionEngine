DROP INDEX IF EXISTS users_password_reset_hash_idx;

ALTER TABLE users
    DROP COLUMN IF EXISTS password_reset_hash,
    DROP COLUMN IF EXISTS password_reset_expires_at;
