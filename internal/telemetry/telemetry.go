// Package telemetry sets up distributed tracing with OpenTelemetry (v1.0).
//
// # What a trace is
//
// A trace is the story of one request across every process it touches. It
// is a tree of spans; each span is one timed operation (an HTTP request, a
// gRPC call, a SQL query, a Kafka record being processed) with a parent.
// With an async bid you can see, in one picture (Jaeger), the HTTP request
// that queued it, the outbox relay, the Kafka hop and the worker that
// finally placed it, and which SQL query was slow.
//
// # Propagation
//
// Spans in different processes join the same trace because the trace
// context (trace ID + parent span ID) travels with the work, in the W3C
// "traceparent" format:
//
//	HTTP        traceparent request header          (Middleware)
//	gRPC        traceparent in metadata             (grpcsvc interceptors)
//	outbox      trace_context column on the row     (outbox.Enqueue / Worker)
//	Kafka       traceparent record header           (kafkax.Relay / bidqueue.Worker)
//
// The outbox hop is the interesting one: the request has long finished when
// the worker picks the event up, so the context is saved with the event row
// and restored later.
//
// Without OTEL_EXPORTER_OTLP_ENDPOINT, a no-op provider is used: spans cost
// almost nothing and go nowhere.
package telemetry

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/Ayush1388/auctionEngine/internal/httpx"
	"github.com/Ayush1388/auctionEngine/internal/logctx"
)

const instrumentation = "github.com/Ayush1388/auctionEngine"

// Tracer is what the rest of the code starts spans with.
func Tracer() trace.Tracer { return otel.Tracer(instrumentation) }

// Setup installs the global tracer provider and propagator. It returns a
// shutdown function that flushes buffered spans; call it on exit.
//
// Configuration follows the standard OpenTelemetry environment variables:
// OTEL_EXPORTER_OTLP_ENDPOINT (e.g. http://jaeger:4318) and
// OTEL_TRACES_SAMPLER_ARG (fraction of traces to keep, default 1.0).
func Setup(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracehttp.New(ctx) // reads the OTEL_* env vars
	if err != nil {
		return nil, err
	}

	ratio := 1.0
	if v, err := strconv.ParseFloat(os.Getenv("OTEL_TRACES_SAMPLER_ARG"), 64); err == nil && v >= 0 && v <= 1 {
		ratio = v
	}

	tp := sdktrace.NewTracerProvider(
		// Batch spans in memory and export them in the background, so
		// tracing never adds a network call to the request path.
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", serviceName))),
		// ParentBased: if the caller's trace was sampled, keep ours too, so
		// a trace is never half-recorded across services.
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// Middleware starts a server span for each HTTP request, continuing the
// caller's trace if it sent a traceparent header, and adds trace_id to the
// request's logger so logs and traces can be joined.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := httpx.WithRoute(r.Context())
		ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(r.Header))
		ctx, span := Tracer().Start(ctx, "HTTP "+r.Method, trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.String("http.request.method", r.Method)))
		defer span.End()

		if sc := span.SpanContext(); sc.IsValid() {
			ctx = logctx.With(ctx, logctx.From(ctx).With("trace_id", sc.TraceID().String()))
		}

		rec := &codeRecorder{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rec, r.WithContext(ctx))

		// The route is only known after the router matched (httpx.Tagged).
		if route := httpx.Route(ctx); route != "" {
			span.SetName(route)
			span.SetAttributes(attribute.String("http.route", route))
		}
		span.SetAttributes(attribute.Int("http.response.status_code", rec.code))
		if rec.code >= 500 {
			span.SetStatus(codes.Error, http.StatusText(rec.code))
		}
	})
}

type codeRecorder struct {
	http.ResponseWriter
	code int
}

func (c *codeRecorder) WriteHeader(code int)        { c.code = code; c.ResponseWriter.WriteHeader(code) }
func (c *codeRecorder) Unwrap() http.ResponseWriter { return c.ResponseWriter }

// Hijack and Flush pass through, so WebSocket upgrades keep working when
// this middleware wraps them.
func (c *codeRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := c.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijacking not supported")
	}
	c.code = http.StatusSwitchingProtocols
	return h.Hijack()
}

func (c *codeRecorder) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Inject writes the current trace context into a string map (for the
// outbox row, Kafka headers or gRPC metadata).
func Inject(ctx context.Context) map[string]string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	if len(carrier) == 0 {
		return nil
	}
	return carrier
}

// Extract returns ctx carrying the trace context stored by Inject.
func Extract(ctx context.Context, m map[string]string) context.Context {
	if len(m) == 0 {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(m))
}

// RecordError marks span as failed with err.
func RecordError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}
