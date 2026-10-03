package grpcsvc_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/auth"
	"github.com/Ayush1388/auctionEngine/internal/handlers"
	"github.com/Ayush1388/auctionEngine/internal/server"
	"github.com/Ayush1388/auctionEngine/internal/user"
)

// The same HTTP API, with bidding behind gRPC instead of in-process. The
// handlers are unchanged; only the Bidder implementation differs.
func TestHTTPGatewayOverGRPC(t *testing.T) {
	e := start(t, token)
	a := e.activeAuction(t)
	bidder := e.user(100_000)

	jwt := user.NewJWTService("0123456789abcdef0123456789abcdef", "test", time.Hour)
	api := server.Routes(server.Deps{
		Bids:   handlers.NewBidHandler(e.client),
		Auth:   auth.NewMiddleware(jwt),
		Logger: quiet,
	})

	call := func(userID uuid.UUID, amount string) (int, map[string]any) {
		tok, _ := jwt.GenerateToken(userID, "user")
		r := httptest.NewRequest("POST", "/v1/auctions/"+a.String()+"/bids", strings.NewReader(`{"amount": `+amount+`}`))
		r.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		var body map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return w.Code, body
	}

	if code, body := call(bidder, "1500"); code != http.StatusCreated || body["current_bid"] != 1500.0 {
		t.Fatalf("bid: %d %v", code, body)
	}
	if code, body := call(bidder, "10"); code != http.StatusUnprocessableEntity || body["minimum_amount"] == nil {
		t.Fatalf("too low: %d %v", code, body)
	}
	if code, _ := call(e.seller, "5000"); code != http.StatusForbidden {
		t.Fatalf("own auction: %d", code)
	}

	// The bidding service goes away: users get a retryable 503, not a 500.
	e.srv.Stop()
	if code, _ := call(bidder, "2000"); code != http.StatusServiceUnavailable {
		t.Fatalf("service down: %d, want 503", code)
	}
}
