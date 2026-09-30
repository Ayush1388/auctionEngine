package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// requestWithHeader is like request but also sets an extra header.
func (a *api) requestWithHeader(method, path, token, body, header, value string) (*httptest.ResponseRecorder, map[string]any) {
	a.t.Helper()

	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	r.Header.Set(header, value)
	w := httptest.NewRecorder()
	a.handler.ServeHTTP(w, r)
	return w, decode(a.t, w)
}

// activeAuction makes an auction bidding can happen on right now.
func (a *api) activeAuction(token string) string {
	a.t.Helper()
	id := a.createAuction(token, time.Hour, 2*time.Hour)["id"].(string)
	if _, err := a.pool.Exec(context.Background(),
		`UPDATE auctions SET status = 'ACTIVE', starts_at = $2 WHERE id = $1`, id, a.clock.Add(-time.Minute)); err != nil {
		a.t.Fatal(err)
	}
	return id
}

func TestWalletHTTP(t *testing.T) {
	a := newAPI(t)
	_, token := a.newUser()

	if w, _ := a.request("GET", "/v1/wallet", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", w.Code)
	}

	w, _ := a.request("POST", "/v1/wallet/deposits", token, `{"amount": 500}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("deposit without Idempotency-Key: %d", w.Code)
	}

	for i := 0; i < 2; i++ {
		w, body := a.requestWithHeader("POST", "/v1/wallet/deposits", token, `{"amount": 500}`, "Idempotency-Key", "topup-1")
		if w.Code != http.StatusOK || body["available"].(float64) != 500 {
			t.Fatalf("deposit attempt %d: %d %v", i+1, w.Code, body)
		}
	}

	w, body := a.request("GET", "/v1/wallet/ledger", token, "")
	entries, _ := body["entries"].([]any)
	if w.Code != http.StatusOK || len(entries) != 1 {
		t.Fatalf("ledger: %d, %d entries (a retried deposit must appear once)", w.Code, len(entries))
	}
}

func TestPlaceBidHTTP(t *testing.T) {
	a := newAPI(t)
	_, sellerToken := a.newUser()
	_, bidderToken := a.newUser()
	a.requestWithHeader("POST", "/v1/wallet/deposits", bidderToken, `{"amount": 1000000}`, "Idempotency-Key", "seed")

	id := a.activeAuction(sellerToken)
	path := "/v1/auctions/" + id + "/bids"

	if w, _ := a.request("POST", path, "", `{"amount": 60000}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", w.Code)
	}
	if w, _ := a.request("POST", path, sellerToken, `{"amount": 60000}`); w.Code != http.StatusForbidden {
		t.Fatalf("seller bidding: %d", w.Code)
	}

	w, body := a.request("POST", path, bidderToken, `{"amount": 100}`)
	if w.Code != http.StatusUnprocessableEntity || body["minimum_amount"].(float64) != 50000 {
		t.Fatalf("too low: %d %v", w.Code, body)
	}

	w, body = a.requestWithHeader("POST", path, bidderToken, `{"amount": 60000}`, "Idempotency-Key", "k1")
	if w.Code != http.StatusCreated || body["current_bid"].(float64) != 60000 {
		t.Fatalf("bid: %d %v", w.Code, body)
	}

	w, body = a.requestWithHeader("POST", path, bidderToken, `{"amount": 60000}`, "Idempotency-Key", "k1")
	if w.Code != http.StatusOK || body["replayed"] != true {
		t.Fatalf("replay: %d %v", w.Code, body)
	}

	w, body = a.request("GET", path, "", "")
	bids, _ := body["bids"].([]any)
	if w.Code != http.StatusOK || len(bids) != 1 {
		t.Fatalf("history: %d %v", w.Code, body)
	}

	w, body = a.request("GET", "/v1/auctions/"+id, "", "")
	if body["bid_count"].(float64) != 1 || body["current_bidder_id"] == nil {
		t.Fatalf("auction after bid: %v", body)
	}

	if w, _ := a.request("POST", "/v1/auctions/"+uuid.NewString()+"/bids", bidderToken, `{"amount": 1}`); w.Code != http.StatusNotFound {
		t.Fatalf("unknown auction: %d", w.Code)
	}
}
