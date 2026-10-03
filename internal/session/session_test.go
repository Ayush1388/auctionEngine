package session_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ayush1388/auctionEngine/internal/session"
	"github.com/Ayush1388/auctionEngine/internal/testdb"
	"github.com/Ayush1388/auctionEngine/internal/user"
)

func setup(t *testing.T) (*session.Service, *user.JWTService, *pgxpool.Pool, uuid.UUID) {
	t.Helper()
	pool := testdb.New(t)
	jwt := user.NewJWTService("0123456789abcdef0123456789abcdef", "test", time.Minute)
	id := uuid.New()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, activated_at) VALUES ($1, $2, 'x', now())`,
		id, id.String()+"@example.com"); err != nil {
		t.Fatal(err)
	}
	return session.NewService(pool, jwt, time.Hour), jwt, pool, id
}

func TestRefreshRotates(t *testing.T) {
	s, jwt, _, userID := setup(t)
	ctx := context.Background()

	first, err := s.Issue(ctx, userID, user.RoleUser)
	if err != nil {
		t.Fatal(err)
	}

	second, err := s.Refresh(ctx, first.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}
	if claims, err := jwt.ValidateToken(second.AccessToken); err != nil || claims.UserID != userID {
		t.Fatalf("new access token invalid: %v", err)
	}

	// The new token keeps working.
	if _, err := s.Refresh(ctx, second.RefreshToken); err != nil {
		t.Fatalf("refresh with rotated token: %v", err)
	}
}

// A stolen token used after the real client already rotated it must kill
// the whole session, including the real client's current token.
func TestReuseRevokesTheFamily(t *testing.T) {
	s, _, _, userID := setup(t)
	ctx := context.Background()

	stolen, _ := s.Issue(ctx, userID, user.RoleUser)
	current, err := s.Refresh(ctx, stolen.RefreshToken) // the real client rotates
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.Refresh(ctx, stolen.RefreshToken); !errors.Is(err, session.ErrTokenReused) {
		t.Fatalf("replay of a rotated token: %v, want ErrTokenReused", err)
	}
	if _, err := s.Refresh(ctx, current.RefreshToken); !errors.Is(err, session.ErrTokenReused) && !errors.Is(err, session.ErrInvalidToken) {
		t.Fatalf("family survived reuse detection: %v", err)
	}

	// Another login (another family) is unaffected.
	other, _ := s.Issue(ctx, userID, user.RoleUser)
	if _, err := s.Refresh(ctx, other.RefreshToken); err != nil {
		t.Fatalf("unrelated session was revoked: %v", err)
	}
}

func TestLogoutAndExpiry(t *testing.T) {
	s, _, pool, userID := setup(t)
	ctx := context.Background()

	tokens, _ := s.Issue(ctx, userID, user.RoleUser)
	if err := s.Revoke(ctx, tokens.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Refresh(ctx, tokens.RefreshToken); err == nil {
		t.Fatal("refresh worked after logout")
	}

	expired, _ := s.Issue(ctx, userID, user.RoleUser)
	if _, err := pool.Exec(ctx, `UPDATE refresh_tokens SET expires_at = now() - interval '1 second' WHERE revoked_at IS NULL`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Refresh(ctx, expired.RefreshToken); !errors.Is(err, session.ErrInvalidToken) {
		t.Fatalf("expired token: %v", err)
	}

	if _, err := s.Refresh(ctx, "not-a-token"); !errors.Is(err, session.ErrInvalidToken) {
		t.Fatalf("garbage token: %v", err)
	}
}

func TestRefreshPicksUpRoleChanges(t *testing.T) {
	s, jwt, pool, userID := setup(t)
	ctx := context.Background()

	tokens, _ := s.Issue(ctx, userID, user.RoleUser)
	if _, err := pool.Exec(ctx, `UPDATE users SET role = 'admin' WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}

	refreshed, err := s.Refresh(ctx, tokens.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	claims, _ := jwt.ValidateToken(refreshed.AccessToken)
	if claims.Role != user.RoleAdmin {
		t.Fatalf("role = %q, want admin after refresh", claims.Role)
	}
}

func TestStoredTokensAreHashed(t *testing.T) {
	s, _, pool, userID := setup(t)
	ctx := context.Background()

	tokens, _ := s.Issue(ctx, userID, user.RoleUser)
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM refresh_tokens WHERE token_hash = $1`, tokens.RefreshToken).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("raw refresh token stored in the database")
	}
}
