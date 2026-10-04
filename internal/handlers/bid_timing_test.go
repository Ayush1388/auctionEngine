package handlers_test

import (
	"net/http"
	"strings"
	"testing"
)

// The bid endpoint reports where its time went in a Server-Timing header, so the
// frontend can show a bid's journey with measured numbers instead of guesses.
func TestPlaceBidReportsServerTiming(t *testing.T) {
	a := newAPI(t)
	_, sellerToken := a.newUser()
	_, bidderToken := a.newUser()
	a.requestWithHeader("POST", "/v1/wallet/deposits", bidderToken, `{"amount": 1000000}`, "Idempotency-Key", "seed")
	path := "/v1/auctions/" + a.activeAuction(sellerToken) + "/bids"

	has := func(header string, names ...string) bool {
		for _, n := range names {
			if !strings.Contains(header, n+";dur=") {
				return false
			}
		}
		return true
	}

	// An accepted bid shows every phase of its transaction and the whole call.
	w, _ := a.requestWithHeader("POST", path, bidderToken, `{"amount": 60000}`, "Idempotency-Key", "k1")
	if w.Code != http.StatusCreated || !has(w.Header().Get("Server-Timing"), "lock", "decide", "write", "commit", "bidding") {
		t.Fatalf("accepted bid: %d Server-Timing %q", w.Code, w.Header().Get("Server-Timing"))
	}

	// A refused bid still reports how long it waited for the row lock: that wait is
	// the "another bid landed first" evidence the UI shows.
	w, _ = a.request("POST", path, bidderToken, `{"amount": 100}`)
	if w.Code != http.StatusUnprocessableEntity || !has(w.Header().Get("Server-Timing"), "lock", "bidding") {
		t.Fatalf("refused bid: %d Server-Timing %q", w.Code, w.Header().Get("Server-Timing"))
	}

	// A replay did no new work, so there is no transaction to break down.
	w, _ = a.requestWithHeader("POST", path, bidderToken, `{"amount": 60000}`, "Idempotency-Key", "k1")
	if w.Code != http.StatusOK || !has(w.Header().Get("Server-Timing"), "bidding") {
		t.Fatalf("replay: %d Server-Timing %q", w.Code, w.Header().Get("Server-Timing"))
	}
}
