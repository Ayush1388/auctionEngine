// Package session issues and rotates refresh tokens.
//
// # Why two kinds of token
//
// An access token (a JWT) is checked on every request without touching the
// database, which is fast but means it can't be revoked before it expires.
// So access tokens are short-lived. A refresh token is a long-lived random
// secret, stored (hashed) in the database, that the client trades for a new
// access token when the old one expires. Because it lives in the database it
// CAN be revoked: logout, a stolen device, a password change.
//
//	login ──► access (short) + refresh (long)
//	          access expired? ──► POST /v1/auth/refresh ──► new access + NEW refresh
//	logout ──► refresh family revoked
//
// # Rotation and reuse detection
//
// Every refresh returns a new refresh token and revokes the one presented.
// Tokens that descend from the same login share a family_id. A legitimate
// client only ever holds the newest token. If an older, already-rotated token
// shows up, someone else has a copy (stolen from logs, a backup, a device),
// so the whole family is revoked and both parties must log in again. The
// attacker's window is at most one rotation.
package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ayush1388/auctionEngine/internal/database"
	"github.com/Ayush1388/auctionEngine/internal/user"
)

var (
	ErrInvalidToken = errors.New("invalid or expired refresh token")

	// ErrTokenReused means a rotated token was presented again. The family
	// has been revoked; the client must log in.
	ErrTokenReused = errors.New("refresh token reuse detected; all sessions from this login were revoked")
)

type Tokens struct {
	AccessToken      string
	RefreshToken     string
	RefreshExpiresAt time.Time
}

type Service struct {
	db         database.DB
	jwt        *user.JWTService
	refreshTTL time.Duration
	now        func() time.Time
}

func NewService(db database.DB, jwt *user.JWTService, refreshTTL time.Duration) *Service {
	return &Service{db: db, jwt: jwt, refreshTTL: refreshTTL, now: time.Now}
}

// Issue starts a new session (a new token family) after a successful login.
func (s *Service) Issue(ctx context.Context, userID uuid.UUID, role string) (Tokens, error) {
	var tokens Tokens

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		var err error
		tokens, _, err = s.newPair(ctx, tx, userID, role, uuid.New())
		return err
	})

	return tokens, err
}

// Refresh trades a valid refresh token for a new access token and a new
// refresh token, revoking the old one.
func (s *Service) Refresh(ctx context.Context, rawToken string) (Tokens, error) {
	if rawToken == "" {
		return Tokens{}, ErrInvalidToken
	}

	var (
		tokens Tokens
		reused bool
	)

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		var (
			id        uuid.UUID
			userID    uuid.UUID
			familyID  uuid.UUID
			expiresAt time.Time
			revokedAt *time.Time
			role      string
			activated *time.Time
		)

		// FOR UPDATE: two refreshes with the same token at the same moment
		// are serialised. The second one then sees revoked_at set, which is
		// exactly the reuse case.
		err := tx.QueryRow(ctx, `
			SELECT t.id, t.user_id, t.family_id, t.expires_at, t.revoked_at, u.role, u.activated_at
			FROM refresh_tokens t
			JOIN users u ON u.id = t.user_id
			WHERE t.token_hash = $1
			FOR UPDATE OF t
		`, user.HashActivationToken(rawToken)).Scan(&id, &userID, &familyID, &expiresAt, &revokedAt, &role, &activated)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidToken
		}
		if err != nil {
			return fmt.Errorf("find refresh token: %w", err)
		}

		if revokedAt != nil {
			// Reuse: revoke every live token in the family. This must be
			// committed, so return nil and report the error afterwards.
			if _, err := tx.Exec(ctx, `
				UPDATE refresh_tokens SET revoked_at = now()
				WHERE family_id = $1 AND revoked_at IS NULL
			`, familyID); err != nil {
				return fmt.Errorf("revoke family: %w", err)
			}
			reused = true
			return nil
		}

		if !s.now().Before(expiresAt) || activated == nil {
			return ErrInvalidToken
		}

		// The role comes from the users table, not the old token, so a
		// promotion or demotion takes effect at the next refresh.
		var newID uuid.UUID
		tokens, newID, err = s.newPair(ctx, tx, userID, role, familyID)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `
			UPDATE refresh_tokens SET revoked_at = now(), replaced_by = $2
			WHERE id = $1
		`, id, newID)
		return err
	})
	if err != nil {
		return Tokens{}, err
	}
	if reused {
		return Tokens{}, ErrTokenReused
	}

	return tokens, nil
}

// Revoke ends the session the token belongs to (logout). Unknown tokens are
// ignored: logging out twice is not an error.
func (s *Service) Revoke(ctx context.Context, rawToken string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE revoked_at IS NULL
		  AND family_id = (SELECT family_id FROM refresh_tokens WHERE token_hash = $1)
	`, user.HashActivationToken(rawToken))
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

// RevokeAll ends every session of a user (e.g. after a password change).
func (s *Service) RevokeAll(ctx context.Context, userID uuid.UUID) error {
	_, err := s.db.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL
	`, userID)
	if err != nil {
		return fmt.Errorf("revoke sessions: %w", err)
	}
	return nil
}

func (s *Service) newPair(ctx context.Context, tx pgx.Tx, userID uuid.UUID, role string, familyID uuid.UUID) (Tokens, uuid.UUID, error) {
	access, err := s.jwt.GenerateToken(userID, role)
	if err != nil {
		return Tokens{}, uuid.Nil, err
	}

	// Same generator as activation tokens: 32 random bytes, only the
	// SHA-256 is stored.
	raw, hash, err := user.GenerateActivationToken()
	if err != nil {
		return Tokens{}, uuid.Nil, err
	}

	id := uuid.New()
	expiresAt := s.now().Add(s.refreshTTL)

	if _, err := tx.Exec(ctx, `
		INSERT INTO refresh_tokens (id, user_id, family_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5)
	`, id, userID, familyID, hash, expiresAt); err != nil {
		return Tokens{}, uuid.Nil, fmt.Errorf("store refresh token: %w", err)
	}

	return Tokens{AccessToken: access, RefreshToken: raw, RefreshExpiresAt: expiresAt}, id, nil
}
