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

// Router sends each event to every handler registered for its type (fan-out),
// so one event can feed several consumers: bid.placed invalidates the cache,
// updates trending scores and (v0.7) is broadcast over WebSockets.
//
// If any handler fails, the whole event is retried, including handlers that
// already succeeded. Every handler must therefore be idempotent: running it
// twice for the same event must have the same effect as running it once.
type Router struct {
	handlers map[string][]Handler
	redact   map[string][]string
}

func NewRouter() *Router {
	return &Router{
		handlers: map[string][]Handler{},
		redact:   map[string][]string{},
	}
}

// Option configures how the router treats one event type.
type Option func(r *Router, eventType string)

// RedactOnSuccess removes the given top-level payload keys once the event
// has been handled successfully.
func RedactOnSuccess(keys ...string) Option {
	return func(r *Router, eventType string) {
		r.redact[eventType] = append(r.redact[eventType], keys...)
	}
}

// Redactor is implemented by handlers that want payload keys removed after
// successful handling. The worker checks for it.
type Redactor interface {
	RedactKeys(eventType string) []string
}

func (r *Router) RedactKeys(eventType string) []string {
	return r.redact[eventType]
}

// Register adds a handler for eventType. Handlers run in registration order.
func (r *Router) Register(eventType string, h Handler, opts ...Option) {
	r.handlers[eventType] = append(r.handlers[eventType], h)

	for _, opt := range opts {
		opt(r, eventType)
	}
}

func (r *Router) Handle(ctx context.Context, event Event) error {
	handlers, ok := r.handlers[event.EventType]
	if !ok {
		return fmt.Errorf("no handler registered for outbox event type %q", event.EventType)
	}
	for _, h := range handlers {
		if err := h.Handle(ctx, event); err != nil {
			return err
		}
	}
	return nil
}
