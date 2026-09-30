package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Ayush1388/auctionEngine/internal/auth"
	"github.com/Ayush1388/auctionEngine/internal/handlers"
	"github.com/Ayush1388/auctionEngine/internal/session"
	"github.com/Ayush1388/auctionEngine/internal/testdb"
	"github.com/Ayush1388/auctionEngine/internal/user"
)

func newUserMux(t *testing.T) http.Handler {
	t.Helper()

	pool := testdb.New(t)
	jwt := user.NewJWTService("0123456789abcdef0123456789abcdef", "test", time.Hour)
	h := handlers.NewUserHandler(user.NewService(pool, user.NewRepository(pool), jwt), session.NewService(pool, jwt, time.Hour), nil)
	mw := auth.NewMiddleware(jwt)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/users/register", h.Register)
	mux.HandleFunc("POST /v1/users/login", h.Login)
	mux.HandleFunc("POST /v1/users/resend-activation", h.ResendActivation)
	mux.Handle("GET /v1/users/me", mw.Authenticate(http.HandlerFunc(h.Me)))
	return mux
}

func do(t *testing.T, h http.Handler, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()

	r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	return w, decode(t, w)
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var decoded map[string]any
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("response is not JSON: %q", w.Body.String())
		}
	}
	return decoded
}

func TestRegisterHTTP(t *testing.T) {
	mux := newUserMux(t)

	w, body := do(t, mux, "POST", "/v1/users/register", `{"email":"dave@example.com","password":"correct-horse-battery-staple"}`)
	if w.Code != http.StatusCreated || body["email"] != "dave@example.com" {
		t.Fatalf("register: %d %v", w.Code, body)
	}

	w, body = do(t, mux, "POST", "/v1/users/register", `{"email":"dave@example.com","password":"correct-horse-battery-staple"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate: %d %v", w.Code, body)
	}

	w, body = do(t, mux, "POST", "/v1/users/register", `{"email":"not-an-email","password":"short"}`)
	fields, _ := body["fields"].(map[string]any)
	if w.Code != http.StatusBadRequest || fields["email"] == nil || fields["password"] == nil {
		t.Fatalf("validation: %d %v", w.Code, body)
	}

	w, body = do(t, mux, "POST", "/v1/users/register", `{"email":"x@example.com","role":"admin"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown field: %d %v", w.Code, body)
	}
}

func TestLoginRequiresActivationAndMeRequiresToken(t *testing.T) {
	mux := newUserMux(t)

	do(t, mux, "POST", "/v1/users/register", `{"email":"erin@example.com","password":"correct-horse-battery-staple"}`)

	w, _ := do(t, mux, "POST", "/v1/users/login", `{"email":"erin@example.com","password":"correct-horse-battery-staple"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("login before activation: %d", w.Code)
	}

	w, _ = do(t, mux, "POST", "/v1/users/login", `{"email":"nobody@example.com","password":"correct-horse-battery-staple"}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unknown user: %d", w.Code)
	}

	w, body := do(t, mux, "GET", "/v1/users/me", "")
	if w.Code != http.StatusUnauthorized || body["error"] == nil {
		t.Fatalf("me without token: %d %v", w.Code, body)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()

	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestResendActivationAlways204(t *testing.T) {
	mux := newUserMux(t)

	do(t, mux, "POST", "/v1/users/register", `{"email":"hank@example.com","password":"correct-horse-battery-staple"}`)

	for _, email := range []string{"hank@example.com", "nobody@example.com"} {
		w, body := do(t, mux, "POST", "/v1/users/resend-activation", `{"email":"`+email+`"}`)
		if w.Code != http.StatusNoContent {
			t.Fatalf("%s: got %d %v, want 204 either way", email, w.Code, body)
		}
	}
}
