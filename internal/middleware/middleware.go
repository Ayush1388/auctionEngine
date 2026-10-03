// Package middleware holds the HTTP middleware every request passes
// through. A middleware is a function that wraps an http.Handler with
// another http.Handler, doing something before and/or after calling it.
//
// The chain in server.Routes, outermost first:
//
//	metrics → Recover → RequestID → tracing → AccessLog → SecurityHeaders → CORS → rate limit → router → auth → handler
//
// Order matters. metrics (v1.0) is outermost so it also counts the 500s
// Recover writes. Recover comes next so a panic anywhere, including in
// other middleware, becomes a 500 instead of a dropped connection.
// Tracing comes after RequestID because it adds trace_id to the logger
// RequestID created.
// RequestID comes before AccessLog so log lines carry the ID. CORS answers
// preflight requests before rate limiting and auth, which browsers send
// without credentials.
package middleware

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Ayush1388/auctionEngine/internal/httpx"
	"github.com/Ayush1388/auctionEngine/internal/logctx"
)

// Chain applies middlewares so the first one listed is the outermost.
func Chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// validRequestID accepts IDs a client or load balancer may send. Anything
// else is replaced, so nobody can inject newlines or huge strings into logs.
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// RequestID gives every request an ID (reusing a valid incoming
// X-Request-ID), echoes it in the response, and puts a logger with the ID
// into the context. Quote the ID from a response and you can find every
// log line for that request.
func RequestID(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-ID")
			if !validRequestID.MatchString(id) {
				id = newID()
			}
			w.Header().Set("X-Request-ID", id)

			ctx := logctx.With(r.Context(), base.With("request_id", id))
			ctx = logctx.WithRequestID(ctx, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// AccessLog writes one structured line per request: method, path, status,
// bytes, duration. The query string is left out on purpose; it can contain
// tokens (?token= for activation).
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &StatusRecorder{ResponseWriter: w, Status: http.StatusOK}

		next.ServeHTTP(rec, r)

		level := slog.LevelInfo
		if rec.Status >= 500 {
			level = slog.LevelError
		}
		logctx.From(r.Context()).Log(r.Context(), level, "http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.Status,
			"bytes", rec.Bytes,
			"duration_ms", float64(time.Since(start).Microseconds())/1000,
		)
	})
}

// StatusRecorder remembers the status code and body size a handler wrote.
// It passes Flush and Hijack through to the real ResponseWriter, which
// streaming responses and WebSocket upgrades (v0.7) rely on.
type StatusRecorder struct {
	http.ResponseWriter
	Status      int
	Bytes       int
	wroteHeader bool
}

func (s *StatusRecorder) WriteHeader(code int) {
	if !s.wroteHeader {
		s.Status = code
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *StatusRecorder) Write(b []byte) (int, error) {
	s.wroteHeader = true
	n, err := s.ResponseWriter.Write(b)
	s.Bytes += n
	return n, err
}

func (s *StatusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *StatusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := s.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("hijacking not supported")
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (s *StatusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Recover turns a panic in any handler into a logged 500 with the stack
// trace, instead of the connection being dropped with no response. One bad
// request must never take the process down.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				// http.ErrAbortHandler is the documented way to abort a
				// response on purpose; re-panic so net/http handles it.
				if v == http.ErrAbortHandler {
					panic(v)
				}
				logctx.From(r.Context()).Error("panic in handler",
					"panic", v, "stack", string(debug.Stack()))
				httpx.Error(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// SecurityHeaders sets headers that switch off browser behaviours an API
// never needs:
//
//	X-Content-Type-Options: nosniff  a JSON response is never run as a script
//	X-Frame-Options: DENY            no clickjacking via iframes
//	Content-Security-Policy          the API serves no HTML, so allow nothing
//	Referrer-Policy: no-referrer     URLs (with IDs) don't leak to other sites
//	Strict-Transport-Security        only when served over HTTPS in production:
//	                                 browsers refuse plain HTTP for a year
func SecurityHeaders(hsts bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
			h.Set("Referrer-Policy", "no-referrer")
			if hsts {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORS lets browser apps served from allowedOrigins call the API.
//
// Browsers block a page on site A from reading responses from site B unless
// B says it's fine (the same-origin policy). For anything beyond a simple
// GET, the browser first sends an OPTIONS "preflight" asking which methods
// and headers are allowed. We answer only for listed origins, never "*"
// with credentials. Requests from non-browser clients (curl, mobile apps)
// don't involve CORS at all.
//
// CSRF note: this API authenticates with an Authorization header, not
// cookies. A malicious site can't make the browser attach that header
// automatically, so classic CSRF doesn't apply.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make([]string, 0, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o = strings.TrimSpace(o); o != "" {
			allowed = append(allowed, o)
		}
	}

	const (
		methods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
		headers = "Authorization, Content-Type, Idempotency-Key, X-Request-ID"
		maxAge  = 600 // seconds a browser may cache the preflight answer
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" || !slices.Contains(allowed, origin) {
				// Not a cross-origin browser request, or not allowed.
				if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Expose-Headers", "X-Request-ID, RateLimit-Limit, RateLimit-Remaining, Retry-After")

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				h.Set("Access-Control-Allow-Methods", methods)
				h.Set("Access-Control-Allow-Headers", headers)
				h.Set("Access-Control-Max-Age", strconv.Itoa(maxAge))
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
