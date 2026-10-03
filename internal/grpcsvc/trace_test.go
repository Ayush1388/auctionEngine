package grpcsvc_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel/trace"

	"github.com/Ayush1388/auctionEngine/internal/auth"
	"github.com/Ayush1388/auctionEngine/internal/handlers"
	"github.com/Ayush1388/auctionEngine/internal/metrics"
	"github.com/Ayush1388/auctionEngine/internal/server"
	"github.com/Ayush1388/auctionEngine/internal/telemetry/telemetrytest"
	"github.com/Ayush1388/auctionEngine/internal/user"
)

// One bid, one trace, two processes' worth of spans:
//
//	POST /v1/auctions/{id}/bids          (gateway, HTTP server span)
//	└─ /…BiddingService/PlaceBid         (gateway, gRPC client span)
//	   └─ /…BiddingService/PlaceBid      (bidding service, gRPC server span)
//	      └─ bidding.PlaceBid
//	         └─ db SELECT / db INSERT …  (pgx tracer)
//
// The trace context crossed the network inside gRPC metadata.
func TestOneTraceFromHTTPThroughGRPCToSQL(t *testing.T) {
	rec := telemetrytest.Record(t)
	e := start(t, token)
	a := e.activeAuction(t)
	bidder := e.user(100_000)
	rec.Reset() // ignore the set-up queries

	jwt := user.NewJWTService("0123456789abcdef0123456789abcdef", "test", time.Hour)
	api := server.Routes(server.Deps{Bids: handlers.NewBidHandler(e.client), Auth: auth.NewMiddleware(jwt), Logger: quiet})

	before := testutil.ToFloat64(metrics.Bids.WithLabelValues("accepted"))

	tok, _ := jwt.GenerateToken(bidder, "user")
	r := httptest.NewRequest("POST", "/v1/auctions/"+a.String()+"/bids", strings.NewReader(`{"amount": 1500}`))
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	api.ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("bid: %d %s", w.Code, w.Body)
	}

	httpSpans := telemetrytest.Find(rec, "POST /v1/auctions/{id}/bids")
	if len(httpSpans) != 1 {
		t.Fatalf("http spans: %d", len(httpSpans))
	}
	traceID := httpSpans[0].SpanContext().TraceID()

	const method = "/auctionengine.bidding.v1.BiddingService/PlaceBid"
	var client, srv trace.SpanContext
	for _, s := range telemetrytest.Find(rec, method) {
		switch s.SpanKind() {
		case trace.SpanKindClient:
			client = s.SpanContext()
		case trace.SpanKindServer:
			srv = s.SpanContext()
			if s.Parent().SpanID() != client.SpanID() && client.IsValid() {
				t.Errorf("server span's parent is not the client span")
			}
		}
	}
	if !client.IsValid() || !srv.IsValid() {
		t.Fatalf("missing gRPC spans: client=%v server=%v", client.IsValid(), srv.IsValid())
	}

	if len(telemetrytest.Find(rec, "bidding.PlaceBid")) != 1 {
		t.Fatal("no bidding.PlaceBid span")
	}
	sql := 0
	for _, s := range rec.Ended() {
		if strings.HasPrefix(s.Name(), "db ") {
			sql++
			for _, kv := range s.Attributes() {
				if kv.Key == "db.query.text" && strings.Contains(kv.Value.AsString(), "1500") {
					t.Error("SQL span leaked a bind argument")
				}
			}
		}
	}
	if sql == 0 {
		t.Fatal("no SQL spans")
	}

	// Everything belongs to the one trace started by the HTTP request.
	for _, s := range rec.Ended() {
		if s.SpanContext().TraceID() != traceID {
			t.Errorf("span %q is in a different trace", s.Name())
		}
	}

	if got := testutil.ToFloat64(metrics.Bids.WithLabelValues("accepted")) - before; got != 1 {
		t.Errorf("bids_total{result=accepted} grew by %v, want 1", got)
	}
}
