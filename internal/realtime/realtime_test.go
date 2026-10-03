package realtime_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/outbox"
	"github.com/Ayush1388/auctionEngine/internal/realtime"
	"github.com/Ayush1388/auctionEngine/internal/redistest"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// server starts a real HTTP server with the WebSocket handler.
func server(t *testing.T, hub *realtime.Hub, opts realtime.Options) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(realtime.NewHandler(hub, opts, quiet))
	t.Cleanup(srv.Close)
	return srv
}

func dial(t *testing.T, srv *httptest.Server, header http.Header) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	return conn
}

func send(t *testing.T, c *websocket.Conn, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func read(t *testing.T, c *websocket.Conn) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, b, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("not JSON: %s", b)
	}
	return m
}

// expectNothing asserts no message arrives within d.
func expectNothing(t *testing.T, c *websocket.Conn, d time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	if _, b, err := c.Read(ctx); err == nil {
		t.Fatalf("unexpected message: %s", b)
	}
}

func subscribe(t *testing.T, c *websocket.Conn, id uuid.UUID) {
	t.Helper()
	send(t, c, map[string]any{"action": "subscribe", "auction_id": id})
	if m := read(t, c); m["type"] != "subscribed" {
		t.Fatalf("subscribe reply: %v", m)
	}
}

// waitFor polls cond until true or times out.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestOnlySubscribersReceiveUpdates(t *testing.T) {
	hub := realtime.NewHub(100)
	srv := server(t, hub, realtime.Options{})

	auctionA, auctionB := uuid.New(), uuid.New()
	watcherA1, watcherA2, watcherB := dial(t, srv, nil), dial(t, srv, nil), dial(t, srv, nil)
	subscribe(t, watcherA1, auctionA)
	subscribe(t, watcherA2, auctionA)
	subscribe(t, watcherB, auctionB)

	realtime.NewLocalPublisher(hub).Publish(context.Background(), auctionA, []byte(`{"type":"auction.updated","n":1}`))

	for _, c := range []*websocket.Conn{watcherA1, watcherA2} {
		if m := read(t, c); m["n"] != 1.0 {
			t.Fatalf("watcher of A got %v", m)
		}
	}
	expectNothing(t, watcherB, 150*time.Millisecond)

	// After unsubscribing, A1 stops receiving.
	send(t, watcherA1, map[string]any{"action": "unsubscribe", "auction_id": auctionA})
	if m := read(t, watcherA1); m["type"] != "unsubscribed" {
		t.Fatalf("unsubscribe reply: %v", m)
	}
	realtime.NewLocalPublisher(hub).Publish(context.Background(), auctionA, []byte(`{"n":2}`))
	if m := read(t, watcherA2); m["n"] != 2.0 {
		t.Fatalf("A2 got %v", m)
	}
	expectNothing(t, watcherA1, 150*time.Millisecond)
}

func TestBadCommandsGetErrorsNotDisconnects(t *testing.T) {
	hub := realtime.NewHub(100)
	srv := server(t, hub, realtime.Options{MaxSubscriptions: 2})
	c := dial(t, srv, nil)

	send(t, c, map[string]any{"action": "subscribe"}) // no auction_id
	if m := read(t, c); m["type"] != "error" {
		t.Fatalf("got %v", m)
	}

	subscribe(t, c, uuid.New())
	subscribe(t, c, uuid.New())
	send(t, c, map[string]any{"action": "subscribe", "auction_id": uuid.New()})
	if m := read(t, c); m["type"] != "error" || m["error"] != "too many subscriptions" {
		t.Fatalf("third subscription: %v", m)
	}
}

func TestOversizedMessageClosesConnection(t *testing.T) {
	hub := realtime.NewHub(100)
	srv := server(t, hub, realtime.Options{MaxMessageBytes: 64})
	c := dial(t, srv, nil)

	send(t, c, map[string]any{"action": strings.Repeat("x", 200)})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _, err := c.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusMessageTooBig {
		t.Fatalf("want close 1009, got %v", err)
	}
}

func TestCrossSiteOriginIsRejected(t *testing.T) {
	hub := realtime.NewHub(100)
	srv := server(t, hub, realtime.Options{OriginPatterns: []string{"app.example.com"}})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(srv.URL, "http")

	_, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"https://evil.example"}}})
	if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("evil origin: err=%v resp=%v", err, resp)
	}

	ok, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"https://app.example.com"}}})
	if err != nil {
		t.Fatalf("allowed origin rejected: %v", err)
	}
	ok.CloseNow()
}

func TestCapacityLimitTurnsAwayExtraClients(t *testing.T) {
	hub := realtime.NewHub(1)
	srv := server(t, hub, realtime.Options{})

	first := dial(t, srv, nil)
	subscribe(t, first, uuid.New()) // proves it's registered

	second := dial(t, srv, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _, err := second.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusTryAgainLater {
		t.Fatalf("want close 1013, got %v", err)
	}
}

func TestSlowConsumerIsDisconnectedWithoutBlockingOthers(t *testing.T) {
	hub := realtime.NewHub(100)
	srv := server(t, hub, realtime.Options{SendBuffer: 2})
	id := uuid.New()

	slow := dial(t, srv, nil)
	fast := dial(t, srv, nil)
	subscribe(t, slow, id)
	subscribe(t, fast, id)

	// "slow" never reads. Deliver must never block on it.
	pub := realtime.NewLocalPublisher(hub)
	start := time.Now()
	for i := 0; i < 200; i++ {
		pub.Publish(context.Background(), id, []byte(`{"type":"tick"}`))
	}
	if time.Since(start) > time.Second {
		t.Fatal("publishing blocked on a slow client")
	}

	// The slow client is cut off with "policy violation"; drain until we
	// see the close.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		_, _, err := slow.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
				t.Fatalf("slow client closed with %v", err)
			}
			break
		}
	}
	waitFor(t, func() bool { n, _ := hub.Stats(); return n == 1 })
}

func TestHeartbeatKeepsHealthyConnectionsOpen(t *testing.T) {
	hub := realtime.NewHub(100)
	srv := server(t, hub, realtime.Options{PingInterval: 20 * time.Millisecond, PongTimeout: 200 * time.Millisecond})
	c := dial(t, srv, nil)
	id := uuid.New()
	subscribe(t, c, id)

	// The client library answers pings while Read is running. Keep reading
	// through several ping intervals; the connection must stay up.
	go func() {
		time.Sleep(150 * time.Millisecond)
		realtime.NewLocalPublisher(hub).Publish(context.Background(), id, []byte(`{"type":"late"}`))
	}()
	if m := read(t, c); m["type"] != "late" {
		t.Fatalf("got %v", m)
	}
}

func TestHubCloseSendsGoingAway(t *testing.T) {
	hub := realtime.NewHub(100)
	srv := server(t, hub, realtime.Options{})
	c := dial(t, srv, nil)
	subscribe(t, c, uuid.New())

	hub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _, err := c.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusGoingAway {
		t.Fatalf("want 1001 going away, got %v", err)
	}
}

// Two hubs = two API instances. An event processed by one must reach the
// watchers connected to the other.
func TestRedisFanoutReachesEveryInstance(t *testing.T) {
	rdb, prefix := redistest.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hubA, hubB := realtime.NewHub(100), realtime.NewHub(100)
	fanA := realtime.NewRedisFanout(rdb, prefix, hubA, quiet)
	fanB := realtime.NewRedisFanout(rdb, prefix, hubB, quiet)
	go fanA.Run(ctx)
	go fanB.Run(ctx)

	waitCtx, cancelWait := context.WithTimeout(ctx, 3*time.Second)
	defer cancelWait()
	waitFor(t, func() bool {
		n, err := rdb.PubSubNumSub(waitCtx, prefix+"realtime").Result()
		return err == nil && n[prefix+"realtime"] == 2
	})

	id := uuid.New()
	onA := dial(t, server(t, hubA, realtime.Options{}), nil)
	onB := dial(t, server(t, hubB, realtime.Options{}), nil)
	subscribe(t, onA, id)
	subscribe(t, onB, id)

	// Instance A's outbox worker handled the event; it publishes once.
	if err := fanA.Publish(ctx, id, []byte(`{"type":"auction.updated","from":"A"}`)); err != nil {
		t.Fatal(err)
	}

	for name, c := range map[string]*websocket.Conn{"A": onA, "B": onB} {
		if m := read(t, c); m["from"] != "A" {
			t.Fatalf("watcher on %s got %v", name, m)
		}
	}
}

// captured records what EventHandler publishes.
type captured struct{ msgs [][]byte }

func (c *captured) Publish(_ context.Context, _ uuid.UUID, msg []byte) error {
	c.msgs = append(c.msgs, msg)
	return nil
}

func TestEventHandlerSendsFreshSnapshot(t *testing.T) {
	bid := int64(5000)
	a := auction.Auction{
		ID: uuid.New(), Status: auction.StatusActive, StartingPrice: 1000, MinIncrement: 100,
		CurrentBid: &bid, BidCount: 3, Version: 42, EndsAt: time.Now(),
	}
	load := func(_ context.Context, id uuid.UUID) (auction.Auction, error) {
		if id != a.ID {
			return auction.Auction{}, auction.ErrNotFound
		}
		return a, nil
	}

	pub := &captured{}
	h := realtime.EventHandler(load, pub)

	payload, _ := json.Marshal(map[string]any{"auction_id": a.ID, "amount": 1}) // stale amount in payload
	if err := h.Handle(context.Background(), outbox.Event{EventType: "bid.placed", Payload: payload}); err != nil {
		t.Fatal(err)
	}

	var got struct {
		Type    string            `json:"type"`
		Cause   string            `json:"cause"`
		Auction realtime.Snapshot `json:"auction"`
	}
	if len(pub.msgs) != 1 || json.Unmarshal(pub.msgs[0], &got) != nil {
		t.Fatalf("published %d messages", len(pub.msgs))
	}
	if got.Type != "auction.updated" || got.Cause != "bid.placed" || *got.Auction.CurrentBid != 5000 ||
		got.Auction.Version != 42 || got.Auction.MinNextBid != 5100 {
		t.Fatalf("snapshot = %+v", got)
	}

	// A vanished auction is skipped, not retried forever.
	gone, _ := json.Marshal(map[string]any{"auction_id": uuid.New()})
	if err := h.Handle(context.Background(), outbox.Event{EventType: "bid.placed", Payload: gone}); err != nil {
		t.Fatalf("missing auction should be ignored: %v", err)
	}
}

func TestDeadClientIsDetectedByHeartbeat(t *testing.T) {
	hub := realtime.NewHub(100)
	srv := server(t, hub, realtime.Options{PingInterval: 20 * time.Millisecond, PongTimeout: 50 * time.Millisecond})

	// A client that never reads never answers pings: like a phone that
	// lost signal without closing the TCP connection.
	c := dial(t, srv, nil)
	_ = c
	waitFor(t, func() bool { n, _ := hub.Stats(); return n == 1 })
	waitFor(t, func() bool { n, _ := hub.Stats(); return n == 0 })
}
