package search_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/breaker"
	"github.com/Ayush1388/auctionEngine/internal/search"
)

// Elasticsearch is "down" (every request is a 503). After the breaker
// opens, searches stop hitting it at all and are answered by PostgreSQL.
func TestBreakerSkipsElasticsearchWhileItIsDown(t *testing.T) {
	f := newFixture(t)
	f.create(t, "Vintage camera", "electronics", "", auction.StatusActive)

	var hits atomic.Int32
	es := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer es.Close()

	b := breaker.New("es-test", breaker.Options{Threshold: 3, Cooldown: time.Hour, IsFailure: search.SearchFailure})
	svc := search.NewService(search.NewGuarded(search.NewElastic(es.URL, "auctions"), b), search.NewPostgres(f.pool), quiet)

	for i := range 10 {
		page, err := svc.Search(context.Background(), search.SearchInput{Text: "camera"})
		if err != nil || page.Backend != "postgres" || len(page.Hits) != 1 {
			t.Fatalf("search %d: backend=%q hits=%d err=%v", i, page.Backend, len(page.Hits), err)
		}
	}
	if got := hits.Load(); got != 3 {
		t.Fatalf("Elasticsearch was called %d times; after 3 failures the breaker should stop calls", got)
	}
	if b.State() != breaker.Open {
		t.Fatalf("state = %s", b.State())
	}
}

// A bad cursor is the caller's mistake, not Elasticsearch failing.
func TestInvalidCursorDoesNotTripBreaker(t *testing.T) {
	if search.SearchFailure(search.ErrInvalidCursor) {
		t.Fatal("ErrInvalidCursor counted as a failure")
	}
	if !search.SearchFailure(context.DeadlineExceeded) {
		t.Fatal("timeout not counted")
	}
}
