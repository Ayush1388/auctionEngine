package outbox

import (
	"context"
	"fmt"
)

// HandlerFunc adapts a function to the Handler interface.
type HandlerFunc func(ctx context.Context, event Event) error

func (f HandlerFunc) Handle(ctx context.Context, event Event) error {
	return f(ctx, event)
}

// Router sends each event to the handler registered for its type, so the
// worker can serve several packages (email, auctions, ...) at once.
type Router struct {
	handlers map[string]Handler
}

func NewRouter() *Router {
	return &Router{handlers: map[string]Handler{}}
}

// Register sets the handler for eventType. Registering a type twice is a
// wiring bug, so it panics at startup rather than failing later.
func (r *Router) Register(eventType string, h Handler) {
	if _, exists := r.handlers[eventType]; exists {
		panic(fmt.Sprintf("outbox: handler for %q registered twice", eventType))
	}
	r.handlers[eventType] = h
}

func (r *Router) Handle(ctx context.Context, event Event) error {
	h, ok := r.handlers[event.EventType]
	if !ok {
		return fmt.Errorf("no handler registered for outbox event type %q", event.EventType)
	}
	return h.Handle(ctx, event)
}
