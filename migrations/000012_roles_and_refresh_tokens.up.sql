-- v0.4: roles for authorization, and refresh tokens for sessions.

-- Role-based access control. Every user is 'user'; admins are promoted
-- explicitly (go run ./cmd/admin promote <email>).
ALTER TABLE users
    ADD COLUMN role TEXT NOT NULL DEFAULT 'user'
        CHECK (role IN ('user', 'admin'));

-- Refresh tokens let clients get new short-lived access tokens without the
-- password. Only a SHA-256 hash of each token is stored (like activation
-- tokens), so a database leak doesn't hand out sessions.
--
-- Rotation: every refresh returns a new refresh token and revokes the old
-- one. All tokens descending from one login share a family_id. If a token
-- that was already rotated is presented again, someone copied it (the real
-- client already moved on), so the whole family is revoked and the user must
-- log in again. This is "refresh token reuse detection".
CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    family_id UUID NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    replaced_by UUID REFERENCES refresh_tokens(id)
);

CREATE INDEX refresh_tokens_family_idx ON refresh_tokens (family_id);
CREATE INDEX refresh_tokens_user_idx ON refresh_tokens (user_id);
