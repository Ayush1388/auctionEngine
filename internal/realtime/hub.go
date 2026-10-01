// Package realtime pushes live auction updates to browsers over WebSockets
// (v0.7): when anyone bids, everyone watching that auction sees the new
// price within milliseconds, without polling.
//
// # Architecture
//
//	bid committed ─(same tx)─► outbox ─► EventHandler (one instance)
//	                                         │ loads a fresh snapshot
//	                                         ▼
//	                                    Publisher ──► Redis PUBLISH "ae:realtime"
//	                                                       │ every instance is SUBSCRIBEd
//	                          ┌────────────────────────────┼──────────────────────┐
//	                          ▼                            ▼                      ▼
//	                     Hub (instance A)            Hub (instance B)        Hub (instance C)
//	                     room "auction X"            room "auction X"           …
//	                      ├─ client 1                 └─ client 3
//	                      └─ client 2
//
// Why the Redis hop: the outbox gives each event to ONE instance, but the
// people watching an auction are connected to ALL instances. Redis pub/sub
// fans the message out so every instance can deliver it to its own
// connections. With no Redis (single instance), LocalPublisher delivers
// directly.
//
// # Concurrency model
//
//   - Each connection has two goroutines: a reader (subscribe/unsubscribe
//     commands) and a writer (the only goroutine allowed to write to the
//     socket, because WebSocket connections are not safe for concurrent
//     writes).
//   - The Hub's room map is protected by one RWMutex. Delivering a message
//     only takes the read lock and never blocks on a network write: it does a
//     non-blocking send into each client's buffered channel.
//   - Backpressure: if a client's buffer is full (a slow phone on bad
//     network), it is disconnected rather than allowed to slow everyone
//     down or grow memory without bound. The client reconnects and reloads.
package realtime

import (
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

// Hub tracks connected clients and which auctions ("rooms") each watches.
type Hub struct {
	mu      sync.RWMutex
	rooms   map[uuid.UUID]map[*client]struct{}
	clients map[*client]struct{}
	closed  bool

	maxClients int

	// Counters for tests and, in v1.0, metrics.
	delivered atomic.Int64
	dropped   atomic.Int64
}

func NewHub(maxClients int) *Hub {
	return &Hub{
		rooms:      map[uuid.UUID]map[*client]struct{}{},
		clients:    map[*client]struct{}{},
		maxClients: maxClients,
	}
}

// register adds a connection. It refuses when the hub is full (load
// shedding: better to turn away the 10,001st viewer than to degrade all
// 10,000) or shutting down.
func (h *Hub) register(c *client) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || len(h.clients) >= h.maxClients {
		return false
	}
	h.clients[c] = struct{}{}
	return true
}

// unregister removes a connection from the hub and every room it was in.
func (h *Hub) unregister(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, c)
	for id := range c.rooms {
		h.leaveLocked(c, id)
	}
}

func (h *Hub) join(c *client, auctionID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.rooms[auctionID]
	if room == nil {
		room = map[*client]struct{}{}
		h.rooms[auctionID] = room
	}
	room[c] = struct{}{}
	c.rooms[auctionID] = struct{}{}
}

func (h *Hub) leave(c *client, auctionID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.leaveLocked(c, auctionID)
}

func (h *Hub) leaveLocked(c *client, auctionID uuid.UUID) {
	delete(c.rooms, auctionID)
	if room := h.rooms[auctionID]; room != nil {
		delete(room, c)
		// Drop empty rooms so the map doesn't grow with every auction
		// anyone ever looked at.
		if len(room) == 0 {
			delete(h.rooms, auctionID)
		}
	}
}

// Deliver sends msg to every local client watching auctionID.
//
// It never blocks: each client gets a non-blocking send into its buffer.
// A client whose buffer is full is too slow to keep up and is disconnected.
func (h *Hub) Deliver(auctionID uuid.UUID, msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.rooms[auctionID] {
		select {
		case c.send <- msg:
			h.delivered.Add(1)
		default:
			h.dropped.Add(1)
			c.kick(closeSlowConsumer)
		}
	}
}

// Close disconnects every client with "going away" so browsers reconnect
// to another instance. http.Server.Shutdown does NOT do this: hijacked
// connections (WebSockets) are invisible to it, so main registers Close
// with Server.RegisterOnShutdown.
func (h *Hub) Close() {
	h.mu.Lock()
	h.closed = true
	clients := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()

	for _, c := range clients {
		c.kick(closeGoingAway)
	}
}

// Stats reports how many clients are connected and how many rooms exist.
func (h *Hub) Stats() (clients, rooms int) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients), len(h.rooms)
}
