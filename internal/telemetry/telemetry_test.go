package telemetry_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/Ayush1388/auctionEngine/internal/httpx"
	"github.com/Ayush1388/auctionEngine/internal/logctx"
	"github.com/Ayush1388/auctionEngine/internal/telemetry"
	"github.com/Ayush1388/auctionEngine/internal/telemetry/telemetrytest"
)

const incoming = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

func router(h http.HandlerFunc) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /v1/auctions/{id}", httpx.Tagged(h))
	return telemetry.Middleware(mux)
}

// A caller's traceparent is continued, not replaced: our span is a child of
// the caller's span, in the caller's trace.
func TestMiddlewareContinuesIncomingTrace(t *testing.T) {
	rec := telemetrytest.Record(t)

	var inHandler trace.SpanContext
	h := router(func(w http.ResponseWriter, r *http.Request) {
		inHandler = trace.SpanContextFromContext(r.Context())
		w.WriteHeader(http.StatusTeapot)
	})
	req := httptest.NewRequest("GET", "/v1/auctions/123", nil)
	req.Header.Set("traceparent", incoming)
	h.ServeHTTP(httptest.NewRecorder(), req)

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d spans", len(spans))
	}
	s := spans[0]
	if s.SpanContext().TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace not continued: %s", s.SpanContext().TraceID())
	}
	if s.Parent().SpanID().String() != "00f067aa0ba902b7" {
		t.Fatalf("parent = %s", s.Parent().SpanID())
	}
	// Named by route pattern, not raw path: bounded cardinality.
	if s.Name() != "GET /v1/auctions/{id}" {
		t.Fatalf("span name = %q", s.Name())
	}
	if inHandler.SpanID() != s.SpanContext().SpanID() {
		t.Fatal("handler context does not carry the server span")
	}
}

func TestMiddlewareAddsTraceIDToLogs(t *testing.T) {
	telemetrytest.Record(t)
	var buf strings.Builder
	h := router(func(w http.ResponseWriter, r *http.Request) {
		logctx.From(r.Context()).Info("hello")
	})
	req := httptest.NewRequest("GET", "/v1/auctions/1", nil)
	req = req.WithContext(logctx.With(req.Context(), newLogger(&buf)))
	req.Header.Set("traceparent", incoming)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !strings.Contains(buf.String(), "trace_id=4bf92f3577b34da6a3ce929d0e0e4736") {
		t.Fatalf("log line lacks trace_id: %s", buf.String())
	}
}

// Inject/Extract is how the trace crosses the outbox and Kafka.
func TestInjectExtractRoundTrip(t *testing.T) {
	telemetrytest.Record(t)
	ctx, span := telemetry.Tracer().Start(context.Background(), "producer")
	defer span.End()

	carrier := telemetry.Inject(ctx)
	if !strings.HasPrefix(carrier["traceparent"], "00-"+span.SpanContext().TraceID().String()) {
		t.Fatalf("carrier = %v", carrier)
	}

	restored := trace.SpanContextFromContext(telemetry.Extract(context.Background(), carrier))
	if restored.TraceID() != span.SpanContext().TraceID() || !restored.IsRemote() {
		t.Fatalf("restored = %+v", restored)
	}

	// Without a span there is nothing to carry (stored as SQL NULL).
	if m := telemetry.Inject(context.Background()); m != nil {
		t.Fatalf("expected nil carrier, got %v", m)
	}
}
