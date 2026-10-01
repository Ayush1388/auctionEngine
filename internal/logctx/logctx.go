// Package logctx carries a request-scoped logger in a context.Context.
//
// The request-ID middleware stores a logger that already has request_id
// (and later user_id) attached. Anything handling that request logs through
// From(ctx), so every log line of one request can be found by one ID, even
// across services once IDs are propagated (v0.9 gRPC metadata, v1.0 traces).
package logctx

import (
	"context"
	"log/slog"
)

type key struct{}

type requestIDKey struct{}

// WithRequestID stores the request ID so it can be forwarded to other
// services (gRPC metadata) and attached to traces.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID returns the ID stored by WithRequestID, or "".
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// With returns ctx carrying logger.
func With(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, key{}, logger)
}

// From returns the request's logger, or the default logger.
func From(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(key{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}
