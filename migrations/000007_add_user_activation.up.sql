ALTER TABLE users
ADD COLUMN activated_at TIMESTAMPTZ,
ADD COLUMN activation_token_hash TEXT,
ADD COLUMN activation_token_expires_at TIMESTAMPTZ;

CREATE UNIQUE INDEX users_activation_token_hash_idx
ON users (activation_token_hash)
WHERE activation_token_hash IS NOT NULL;