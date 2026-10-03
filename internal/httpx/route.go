package httpx

import (
	"context"
	"net/http"
)

// Route patterns for metrics and tracing (v1.0).
//
// Metrics and traces are labelled by the route PATTERN ("/v1/auctions/{id}"),
// never the raw path, to keep label cardinality bounded. http.ServeMux sets
// r.Pattern only on the request it hands to the matched handler. Every
// middleware calls r.WithContext, which copies the request, so an outer
// middleware never sees that field on its own copy.
//
// The fix is a small mutable holder in the context. The outermost middleware
// creates it with WithRoute. Each route handler is wrapped with Tagged,
// which writes r.Pattern into the holder. Because the context (and so the
// pointer) is shared by every copy of the request, outer middleware can read
// the pattern with Route after the inner handler returns.

type routeKey struct{}

type routeHolder struct{ pattern string }

// WithRoute returns ctx with an empty route holder. If ctx already has one,
// it is returned unchanged, so several middleware can call it safely.
func WithRoute(ctx context.Context) context.Context {
	if _, ok := ctx.Value(routeKey{}).(*routeHolder); ok {
		return ctx
	}
	return context.WithValue(ctx, routeKey{}, &routeHolder{})
}

// Route returns the matched route pattern, or "" if no route matched yet.
func Route(ctx context.Context) string {
	if h, ok := ctx.Value(routeKey{}).(*routeHolder); ok {
		return h.pattern
	}
	return ""
}

// Tagged wraps a route handler so it records the pattern the mux matched.
func Tagged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h, ok := r.Context().Value(routeKey{}).(*routeHolder); ok {
			h.pattern = r.Pattern
		}
		next.ServeHTTP(w, r)
	})
}
