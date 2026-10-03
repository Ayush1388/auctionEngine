// Package telemetrytest records spans in memory so tests can assert on
// traces: which spans exist, their parents, and that they share a trace ID.
package telemetrytest

import (
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// Record installs a global tracer provider that keeps every finished span,
// and restores the previous provider when the test ends. Tests using it
// must not run in parallel (the provider is global).
func Record(t testing.TB) *tracetest.SpanRecorder {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))

	prevTP, prevProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})
	return rec
}

// Find returns the finished spans with the given name.
func Find(rec *tracetest.SpanRecorder, name string) []sdktrace.ReadOnlySpan {
	var out []sdktrace.ReadOnlySpan
	for _, s := range rec.Ended() {
		if s.Name() == name {
			out = append(out, s)
		}
	}
	return out
}

// TraceIDs returns the distinct trace IDs among finished spans.
func TraceIDs(rec *tracetest.SpanRecorder) map[trace.TraceID]int {
	ids := map[trace.TraceID]int{}
	for _, s := range rec.Ended() {
		ids[s.SpanContext().TraceID()]++
	}
	return ids
}
