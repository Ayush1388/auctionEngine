package handlers_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Ayush1388/auctionEngine/internal/auth"
	"github.com/Ayush1388/auctionEngine/internal/handlers"
	"github.com/Ayush1388/auctionEngine/internal/ratelimit"
	"github.com/Ayush1388/auctionEngine/internal/server"
	"github.com/Ayush1388/auctionEngine/internal/session"
	"github.com/Ayush1388/auctionEngine/internal/testdb"
	"github.com/Ayush1388/auctionEngine/internal/user"
)

func TestAdminRoutesNeedTheAdminRole(t *testing.T) {
	a := newAPI(t)
	_, userToken := a.newUser()
	adminID, _ := a.newUser()
	adminToken, err := a.jwt.GenerateToken(adminID, user.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/v1/admin/reconcile", "/v1/admin/outbox/failed"} {
		if w, _ := a.request("GET", path, "", ""); w.Code != http.StatusUnauthorized {
			t.Errorf("%s anonymous: %d", path, w.Code)
		}
		if w, _ := a.request("GET", path, userToken, ""); w.Code != http.StatusForbidden {
			t.Errorf("%s as user: %d", path, w.Code)
		}
		if w, body := a.request("GET", path, adminToken, ""); w.Code != http.StatusOK {
			t.Errorf("%s as admin: %d %v", path, w.Code, body)
		}
	}
}

func TestLoginRefreshLogoutHTTP(t *testing.T) {
	a := newAPI(t)
	ctx := context.Background()

	do(t, a.handler, "POST", "/v1/users/register", `{"email":"ivy@example.com","password":"correct-horse-battery-staple"}`)
	if _, err := a.pool.Exec(ctx, `UPDATE users SET activated_at = now() WHERE email = 'ivy@example.com'`); err != nil {
		t.Fatal(err)
	}

	w, body := do(t, a.handler, "POST", "/v1/users/login", `{"email":"ivy@example.com","password":"correct-horse-battery-staple"}`)
	if w.Code != 200 || body["refresh_token"] == nil {
		t.Fatalf("login: %d %v", w.Code, body)
	}
	refresh := body["refresh_token"].(string)

	w, body = do(t, a.handler, "POST", "/v1/auth/refresh", `{"refresh_token":"`+refresh+`"}`)
	if w.Code != 200 || body["refresh_token"] == refresh {
		t.Fatalf("refresh: %d %v", w.Code, body)
	}
	rotated := body["refresh_token"].(string)

	if w, _ := do(t, a.handler, "POST", "/v1/auth/logout", `{"refresh_token":"`+rotated+`"}`); w.Code != 204 {
		t.Fatalf("logout: %d", w.Code)
	}
	if w, _ := do(t, a.handler, "POST", "/v1/auth/refresh", `{"refresh_token":"`+rotated+`"}`); w.Code != 401 {
		t.Fatalf("refresh after logout: %d", w.Code)
	}
}

// Brute force: after 5 wrong passwords, even the right one is refused for
// a while, and the response says when to retry.
func TestLoginIsThrottledPerEmail(t *testing.T) {
	pool := testdb.New(t)
	jwt := user.NewJWTService("0123456789abcdef0123456789abcdef", "test", time.Hour)
	sessions := session.NewService(pool, jwt, time.Hour)
	limiter := ratelimit.NewMemory()

	h := server.Routes(server.Deps{
		Users:    handlers.NewUserHandler(user.NewService(pool, user.NewRepository(pool), jwt), sessions, limiter),
		Sessions: handlers.NewSessionHandler(sessions),
		Auth:     auth.NewMiddleware(jwt),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	do(t, h, "POST", "/v1/users/register", `{"email":"jay@example.com","password":"correct-horse-battery-staple"}`)
	if _, err := pool.Exec(context.Background(), `UPDATE users SET activated_at = now()`); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		if w, _ := do(t, h, "POST", "/v1/users/login", `{"email":"jay@example.com","password":"wrong-guess-number-x"}`); w.Code != 401 {
			t.Fatalf("attempt %d: %d", i+1, w.Code)
		}
	}

	r := httptest.NewRequest("POST", "/v1/users/login", strings.NewReader(`{"email":"JAY@example.com","password":"correct-horse-battery-staple"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("6th attempt (different case, right password): %d %v", w.Code, w.Header())
	}
}

func TestEveryResponseHasSecurityHeadersAndRequestID(t *testing.T) {
	a := newAPI(t)
	w, _ := a.request("GET", "/v1/healthcheck", "", "")
	for _, h := range []string{"X-Request-ID", "X-Content-Type-Options", "X-Frame-Options"} {
		if w.Header().Get(h) == "" {
			t.Errorf("missing %s", h)
		}
	}
}
