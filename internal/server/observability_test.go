package server_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/Ayush1388/auctionEngine/internal/health"
	"github.com/Ayush1388/auctionEngine/internal/metrics"
	"github.com/Ayush1388/auctionEngine/internal/realtime"
	"github.com/Ayush1388/auctionEngine/internal/server"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	return w
}

// Requests are counted by route PATTERN; unknown paths share one label, so
// a scanner trying a million URLs can't create a million time series.
func TestMetricsAreLabelledByRoutePattern(t *testing.T) {
	h := server.Routes(server.Deps{Logger: quiet})

	live := metrics.HTTPRequests.WithLabelValues("GET", "GET /livez", "200")
	unmatched := metrics.HTTPRequests.WithLabelValues("GET", "unmatched", "404")
	beforeLive, beforeUnmatched := testutil.ToFloat64(live), testutil.ToFloat64(unmatched)

	get(h, "/livez")
	get(h, "/no/such/path/"+uuid.NewString())
	get(h, "/another/"+uuid.NewString())

	if d := testutil.ToFloat64(live) - beforeLive; d != 1 {
		t.Errorf("livez counted %v times, want 1", d)
	}
	if d := testutil.ToFloat64(unmatched) - beforeUnmatched; d != 2 {
		t.Errorf("unmatched counted %v times, want 2", d)
	}
}

func TestProbes(t *testing.T) {
	checker := health.New()
	checker.Add("postgres", true, func(context.Context) error { return nil })
	h := server.Routes(server.Deps{Logger: quiet, Health: checker})

	if w := get(h, "/livez"); w.Code != 200 {
		t.Fatalf("livez = %d", w.Code)
	}
	if w := get(h, "/readyz"); w.Code != 200 {
		t.Fatalf("readyz = %d %s", w.Code, w.Body)
	}
	checker.Drain()
	if w := get(h, "/readyz"); w.Code != 503 {
		t.Fatalf("readyz while draining = %d", w.Code)
	}
	if w := get(h, "/livez"); w.Code != 200 {
		t.Fatalf("livez while draining = %d", w.Code)
	}
}

// Every middleware in the chain wraps the ResponseWriter. If any wrapper
// hid http.Hijacker, WebSocket upgrades would fail. Dial through the full
// production chain to prove they don't.
func TestWebSocketUpgradeThroughFullMiddlewareChain(t *testing.T) {
	hub := realtime.NewHub(10)
	srv := httptest.NewServer(server.Routes(server.Deps{
		Logger:   quiet,
		Realtime: realtime.NewHandler(hub, realtime.Options{}, quiet),
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/ws", nil)
	if err != nil {
		t.Fatalf("upgrade failed through middleware: %v", err)
	}
	defer conn.CloseNow()

	id := uuid.New()
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"action":"subscribe","auction_id":"`+id.String()+`"}`)); err != nil {
		t.Fatal(err)
	}
	_, msg, err := conn.Read(ctx)
	if err != nil || !strings.Contains(string(msg), "subscribed") {
		t.Fatalf("reply = %s, %v", msg, err)
	}
}

func TestAdminServesMetricsAndPprof(t *testing.T) {
	checker := health.New()
	admin := httptest.NewServer(server.NewAdmin("", checker).Handler)
	defer admin.Close()

	for path, want := range map[string]string{
		"/metrics":      "auction_http_requests_total",
		"/debug/pprof/": "goroutine",
		"/readyz":       `"status":"ok"`,
		"/metrics?x=go": "go_goroutines",
	} {
		resp, err := http.Get(admin.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(string(body), want) {
			t.Errorf("%s: %d, missing %q", path, resp.StatusCode, want)
		}
	}
}
