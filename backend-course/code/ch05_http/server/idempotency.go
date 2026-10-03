package main

import (
	"bytes"
	"net/http"
	"sync"
)

// Idempotent makes a POST safe to retry. The client sends a unique
// Idempotency-Key (e.g. a UUID made once per "user intent", reused on retries).
// The first request runs normally and its response is stored; any retry with
// the same key gets the stored response back instead of running again.
//
// Production notes: store keys in Redis/DB with a TTL, bind the key to the
// user, compare a hash of the request body (same key + different body = 422),
// and guard against two identical requests running at the same moment.
func Idempotent(next http.Handler) http.Handler {
	type saved struct {
		status int
		header http.Header
		body   []byte
	}
	var (
		mu   sync.Mutex
		seen = map[string]*saved{}
	)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			next.ServeHTTP(w, r)
			return
		}
		mu.Lock()
		defer mu.Unlock() // simple version: serialises keyed requests (also blocks the race)
		if s, ok := seen[key]; ok {
			for k, v := range s.header {
				w.Header()[k] = v
			}
			w.Header().Set("Idempotent-Replayed", "true")
			w.WriteHeader(s.status)
			w.Write(s.body)
			return
		}
		rec := &capture{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if rec.status < 500 { // never cache server errors: let the client retry
			seen[key] = &saved{rec.status, rec.Header().Clone(), rec.buf.Bytes()}
		}
	})
}

// capture writes through to the client while keeping a copy for replay.
type capture struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (c *capture) WriteHeader(code int) { c.status = code; c.ResponseWriter.WriteHeader(code) }
func (c *capture) Write(b []byte) (int, error) {
	c.buf.Write(b)
	return c.ResponseWriter.Write(b)
}
