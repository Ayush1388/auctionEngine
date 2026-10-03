package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ayush1388/auctionEngine/internal/httpx"
	"github.com/Ayush1388/auctionEngine/internal/metrics"
)

// BenchmarkMiddleware: what RED metrics add to every request.
func BenchmarkMiddleware(b *testing.B) {
	mux := http.NewServeMux()
	mux.Handle("GET /v1/auctions/{id}", httpx.Tagged(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	h := metrics.Middleware(mux)
	req := httptest.NewRequest("GET", "/v1/auctions/123", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
}

// BenchmarkBaseline: the same request without metrics, for comparison.
func BenchmarkBaseline(b *testing.B) {
	mux := http.NewServeMux()
	mux.Handle("GET /v1/auctions/{id}", httpx.Tagged(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	req := httptest.NewRequest("GET", "/v1/auctions/123", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		mux.ServeHTTP(httptest.NewRecorder(), req)
	}
}
