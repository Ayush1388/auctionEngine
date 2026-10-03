package main

import (
	"compress/gzip"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// CORS implements the browser's cross-origin contract (video: "CORS flow").
//
//	simple request  -> add Access-Control-Allow-Origin to the response
//	pre-flight      -> browser sends OPTIONS first; we answer 204 + capability headers
//
// Only origins in the allow-list are echoed back. We never combine "*" with
// credentials, and we add Vary: Origin so shared caches do not mix origins up.
func CORS(allowed []string) func(http.Handler) http.Handler {
	set := make(map[string]bool, len(allowed))
	for _, o := range allowed {
		set[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			w.Header().Add("Vary", "Origin")

			// Not a cross-origin browser request, or origin not allowed:
			// do nothing. The browser will block the response for us.
			if origin == "" || !set[origin] {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)

			isPreflight := r.Method == http.MethodOptions &&
				r.Header.Get("Access-Control-Request-Method") != ""
			if isPreflight {
				h := w.Header()
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
				h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				h.Set("Access-Control-Max-Age", "86400") // cache the answer for 24h
				w.WriteHeader(http.StatusNoContent)      // 204: nothing to say, only headers
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Logging records method, path, status and latency of every request.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request",
			"method", r.Method, "path", r.URL.Path,
			"status", rec.status, "dur", time.Since(start))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Flush keeps Server-Sent Events working through this wrapper.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Gzip compresses the response when the client advertises support through
// Accept-Encoding (content negotiation for encoding).
func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length") // length changes after compression
		gz := gzip.NewWriter(w)
		defer gz.Close()
		next.ServeHTTP(&gzipWriter{ResponseWriter: w, zw: gz}, r)
	})
}

type gzipWriter struct {
	http.ResponseWriter
	zw *gzip.Writer
}

func (g *gzipWriter) Write(b []byte) (int, error) { return g.zw.Write(b) }
