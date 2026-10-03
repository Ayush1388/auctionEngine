package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ayush1388/auctionEngine/internal/logctx"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestRequestID(t *testing.T) {
	var seen *slog.Logger
	h := RequestID(quiet)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = logctx.From(r.Context())
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if id := w.Header().Get("X-Request-ID"); len(id) != 24 || seen == nil {
		t.Fatalf("generated id %q", id)
	}

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Request-ID", "lb-123")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Header().Get("X-Request-ID") != "lb-123" {
		t.Fatal("valid incoming id not reused")
	}

	r = httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Request-ID", "bad\nid injected into logs")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if strings.Contains(w.Header().Get("X-Request-ID"), "\n") || w.Header().Get("X-Request-ID") == "" {
		t.Fatal("invalid incoming id was not replaced")
	}
}

func TestRecover(t *testing.T) {
	h := Recover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 500 || !strings.Contains(w.Body.String(), "internal server error") || strings.Contains(w.Body.String(), "boom") {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
}

func TestSecurityHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	SecurityHeaders(true)(http.NotFoundHandler()).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	for _, h := range []string{"X-Content-Type-Options", "X-Frame-Options", "Content-Security-Policy", "Referrer-Policy", "Strict-Transport-Security"} {
		if w.Header().Get(h) == "" {
			t.Errorf("missing %s", h)
		}
	}
}

func TestCORS(t *testing.T) {
	h := CORS([]string{"https://app.example.com"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	preflight := func(origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("OPTIONS", "/v1/auctions", nil)
		r.Header.Set("Origin", origin)
		r.Header.Set("Access-Control-Request-Method", "POST")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	if w := preflight("https://app.example.com"); w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" ||
		!strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "Idempotency-Key") {
		t.Fatalf("allowed preflight: %d %v", w.Code, w.Header())
	}
	if w := preflight("https://evil.example"); w.Code != 403 || w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("disallowed preflight: %d %v", w.Code, w.Header())
	}

	// A normal request without Origin (curl, a mobile app) is untouched.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("non-CORS request: %d %v", w.Code, w.Header())
	}
}

func TestChainOrder(t *testing.T) {
	var order []string
	mk := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	Chain(http.NotFoundHandler(), mk("a"), mk("b"), mk("c")).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if strings.Join(order, "") != "abc" {
		t.Fatalf("order = %v, want first listed outermost", order)
	}
}
