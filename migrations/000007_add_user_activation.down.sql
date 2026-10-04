DROP INDEX IF EXISTS users_activation_token_hash_idx;

ALTER TABLE users
DROP COLUMN IF EXISTS activation_token_hash,
DROP COLUMN IF EXISTS activation_token_expires_at,
DROP COLUMN IF EXISTS activated_at;