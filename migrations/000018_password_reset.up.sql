-- Forgot password: a single-use, expiring link. Only the SHA-256 hash of the token is stored,
-- exactly like activation tokens.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS password_reset_hash TEXT,
    ADD COLUMN IF NOT EXISTS password_reset_expires_at TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS users_password_reset_hash_idx
    ON users (password_reset_hash)
    WHERE password_reset_hash IS NOT NULL;
