package handlers_test

import (
	"context"
	"net/http"
	"testing"
)

func TestAsyncBidHTTP(t *testing.T) {
	a := newAPI(t)
	_, seller := a.newUser()
	bidderID, bidder := a.newUser()
	_, stranger := a.newUser()
	auctionID := a.activeAuction(seller)

	path := "/v1/auctions/" + auctionID + "/bids"

	// Asking for async gets 202 and a request to poll, not a bid.
	w, body := a.requestWithHeader("POST", path, bidder, `{"amount": 60000}`, "Prefer", "respond-async")
	if w.Code != http.StatusAccepted || body["status"] != "PENDING" {
		t.Fatalf("async: %d %v", w.Code, body)
	}
	statusURL := body["status_url"].(string)
	if w.Header().Get("Location") != statusURL {
		t.Fatalf("Location %q vs status_url %q", w.Header().Get("Location"), statusURL)
	}

	// Nothing has been placed yet: that's the bid worker's job.
	var bids int
	if err := a.pool.QueryRow(context.Background(), `SELECT count(*) FROM bids`).Scan(&bids); err != nil {
		t.Fatal(err)
	}
	if bids != 0 {
		t.Fatalf("async request placed a bid synchronously")
	}

	if w, body := a.request("GET", statusURL, bidder, ""); w.Code != http.StatusOK || body["amount"] != 60000.0 {
		t.Fatalf("owner reads status: %d %v", w.Code, body)
	}
	if w, _ := a.request("GET", statusURL, stranger, ""); w.Code != http.StatusNotFound {
		t.Fatalf("stranger reads status: %d (must be 404, not 403)", w.Code)
	}
	if w, _ := a.request("GET", statusURL, "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", w.Code)
	}

	// Without the Prefer header the same endpoint stays synchronous.
	a.requestWithHeader("POST", "/v1/wallet/deposits", bidder, `{"amount": 100000}`, "Idempotency-Key", "seed-"+bidderID.String())
	if w, body := a.request("POST", path, bidder, `{"amount": 60000}`); w.Code != http.StatusCreated {
		t.Fatalf("sync bid: %d %v", w.Code, body)
	}
}
