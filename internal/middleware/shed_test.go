package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Ayush1388/auctionEngine/internal/middleware"
)

func TestLimitInFlightShedsExcessAndExemptsProbes(t *testing.T) {
	release := make(chan struct{})
	var entered sync.WaitGroup
	entered.Add(2)
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/work" {
			entered.Done()
			<-release
		}
		w.WriteHeader(http.StatusOK)
	})
	h := middleware.LimitInFlight(2, "/livez")(slow)

	// Fill both slots.
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/work", nil)) })
	}
	entered.Wait()

	// The third request is rejected at once, with a retry hint.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/work", nil))
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") != "1" {
		t.Fatalf("over limit: %d %v", w.Code, w.Header())
	}

	// Probes still get through while the instance is saturated.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/livez", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("probe shed: %d", w.Code)
	}

	close(release)
	wg.Wait()

	// Slots are released: capacity is back.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/other", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("after release: %d", w.Code)
	}
}
