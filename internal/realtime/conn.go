package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
)

// Protocol (JSON text frames)
//
//	client → server  {"action": "subscribe",   "auction_id": "…"}
//	                 {"action": "unsubscribe", "auction_id": "…"}
//	server → client  {"type": "subscribed",    "auction_id": "…"}
//	                 {"type": "unsubscribed",  "auction_id": "…"}
//	                 {"type": "auction.updated", "cause": "bid.placed", "auction": {…, "version": 42}}
//	                 {"type": "error", "error": "…"}
//
// The stream is read-only and carries only public data (the same as
// GET /v1/auctions/{id}), so connecting needs no token. Bids are still
// placed over the authenticated REST API.

type command struct {
	Action    string    `json:"action"`
	AuctionID uuid.UUID `json:"auction_id"`
}

type reply struct {
	Type      string     `json:"type"`
	AuctionID *uuid.UUID `json:"auction_id,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// Options tune the server. Zero values get sensible defaults.
type Options struct {
	// OriginPatterns lists allowed browser origins (host patterns such as
	// "app.example.com"). Without this check, any website a user visits
	// could open a socket to us from their browser: Cross-Site WebSocket
	// Hijacking.
	OriginPatterns []string

	PingInterval     time.Duration // how often to check the client is alive
	PongTimeout      time.Duration // how long to wait for the answer
	SendBuffer       int           // messages queued per client before it is "slow"
	MaxSubscriptions int           // rooms per connection
	MaxMessageBytes  int64         // largest frame a client may send
}

func (o Options) withDefaults() Options {
	if o.PingInterval == 0 {
		o.PingInterval = 25 * time.Second
	}
	if o.PongTimeout == 0 {
		o.PongTimeout = 10 * time.Second
	}
	if o.SendBuffer == 0 {
		o.SendBuffer = 32
	}
	if o.MaxSubscriptions == 0 {
		o.MaxSubscriptions = 20
	}
	if o.MaxMessageBytes == 0 {
		o.MaxMessageBytes = 4 << 10
	}
	return o
}

// Close reasons sent to clients.
var (
	closeSlowConsumer = closeReason{websocket.StatusPolicyViolation, "too slow to keep up"}
	closeGoingAway    = closeReason{websocket.StatusGoingAway, "server shutting down"}
)

type closeReason struct {
	code websocket.StatusCode
	text string
}

// client is one WebSocket connection.
type client struct {
	send  chan []byte
	rooms map[uuid.UUID]struct{} // guarded by Hub.mu

	once   sync.Once
	reason closeReason
	gone   chan struct{} // closed by kick: tells the writer to stop
}

// kick asks the connection to close. Safe to call many times, from any
// goroutine (the hub calls it while holding its lock, so it must not block).
func (c *client) kick(r closeReason) {
	c.once.Do(func() {
		c.reason = r
		close(c.gone)
	})
}

// Handler upgrades GET /v1/ws to a WebSocket and serves it.
type Handler struct {
	hub    *Hub
	opts   Options
	logger *slog.Logger
}

func NewHandler(hub *Hub, opts Options, logger *slog.Logger) *Handler {
	return &Handler{hub: hub, opts: opts.withDefaults(), logger: logger}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: h.opts.OriginPatterns,
	})
	if err != nil {
		// Accept has already written the HTTP error (403 bad origin, 426…).
		return
	}

	c := &client{
		send:  make(chan []byte, h.opts.SendBuffer),
		rooms: map[uuid.UUID]struct{}{},
		gone:  make(chan struct{}),
	}

	if !h.hub.register(c) {
		// 1013 "try again later": the client should back off and retry.
		conn.Close(websocket.StatusTryAgainLater, "server at capacity")
		return
	}
	defer h.hub.unregister(c)

	// The request context ends when the client disconnects; ctx also ends
	// when we decide to close, so both goroutines stop together.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn.SetReadLimit(h.opts.MaxMessageBytes)

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		h.writeLoop(ctx, conn, c)
		cancel()
	}()

	h.readLoop(ctx, conn, c)
	c.kick(closeReason{websocket.StatusNormalClosure, ""})
	<-writerDone
}

// readLoop handles commands until the client goes away or sends garbage.
func (h *Handler) readLoop(ctx context.Context, conn *websocket.Conn, c *client) {
	for {
		var cmd command
		typ, data, err := conn.Read(ctx)
		if err != nil {
			return // closed, read limit exceeded, or ctx cancelled
		}
		if typ != websocket.MessageText || json.Unmarshal(data, &cmd) != nil || cmd.AuctionID == uuid.Nil {
			h.reply(c, reply{Type: "error", Error: `expected {"action": "subscribe"|"unsubscribe", "auction_id": "<uuid>"}`})
			continue
		}

		id := cmd.AuctionID
		switch cmd.Action {
		case "subscribe":
			if len(c.roomsSnapshot(h.hub)) >= h.opts.MaxSubscriptions {
				h.reply(c, reply{Type: "error", AuctionID: &id, Error: "too many subscriptions"})
				continue
			}
			h.hub.join(c, id)
			h.reply(c, reply{Type: "subscribed", AuctionID: &id})
		case "unsubscribe":
			h.hub.leave(c, id)
			h.reply(c, reply{Type: "unsubscribed", AuctionID: &id})
		default:
			h.reply(c, reply{Type: "error", Error: "unknown action"})
		}
	}
}

// writeLoop is the only goroutine that writes to the socket. It sends
// queued messages and pings the client on a timer; a client that doesn't
// answer a ping in time is considered dead (heartbeat), which is how we
// notice half-open TCP connections that would otherwise linger forever.
func (h *Handler) writeLoop(ctx context.Context, conn *websocket.Conn, c *client) {
	ticker := time.NewTicker(h.opts.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.gone:
			conn.Close(c.reason.code, c.reason.text)
			return

		case msg := <-c.send:
			writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Write(writeCtx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				return
			}

		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, h.opts.PongTimeout)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				// The peer is gone (or hopelessly slow). A polite close
				// handshake would wait for a reply that will never come,
				// so drop the TCP connection immediately.
				conn.CloseNow()
				return
			}

		case <-ctx.Done():
			return
		}
	}
}

// reply queues a control message for this client only.
func (h *Handler) reply(c *client, r reply) {
	b, _ := json.Marshal(r)
	select {
	case c.send <- b:
	default:
		c.kick(closeSlowConsumer)
	}
}

func (c *client) roomsSnapshot(h *Hub) []uuid.UUID {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]uuid.UUID, 0, len(c.rooms))
	for id := range c.rooms {
		out = append(out, id)
	}
	return out
}

// OriginPatterns converts CORS origins ("https://app.example.com") into the
// host patterns the WebSocket origin check expects ("app.example.com").
// With no patterns, only same-origin browser connections are accepted;
// non-browser clients (no Origin header) are always allowed.
func OriginPatterns(corsOrigins []string) []string {
	out := make([]string, 0, len(corsOrigins))
	for _, o := range corsOrigins {
		if u, err := url.Parse(o); err == nil && u.Host != "" {
			out = append(out, u.Host)
		}
	}
	return out
}
