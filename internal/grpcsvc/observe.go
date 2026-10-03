package grpcsvc

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/Ayush1388/auctionEngine/internal/logctx"
	"github.com/Ayush1388/auctionEngine/internal/metrics"
	"github.com/Ayush1388/auctionEngine/internal/telemetry"
)

// Tracing and metrics for gRPC (v1.0).
//
// The trace context crosses the network in gRPC metadata (HTTP/2 headers),
// using the same W3C "traceparent" key as HTTP. The gateway's span for
// POST /v1/auctions/{id}/bids therefore becomes the parent of the bidding
// service's span, and the SQL spans below that: one trace, two processes.

// mdCarrier adapts gRPC metadata to OpenTelemetry's TextMapCarrier.
type mdCarrier metadata.MD

func (c mdCarrier) Get(key string) string {
	if v := metadata.MD(c).Get(key); len(v) > 0 {
		return v[0]
	}
	return ""
}
func (c mdCarrier) Set(key, value string) { metadata.MD(c).Set(key, value) }
func (c mdCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// serverSpan continues the caller's trace and records RED metrics for one
// call. It returns the context to use and a function to call when the call
// ends.
func serverSpan(ctx context.Context, method string) (context.Context, func(error)) {
	start := time.Now()
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		ctx = otel.GetTextMapPropagator().Extract(ctx, mdCarrier(md))
	}
	ctx, span := telemetry.Tracer().Start(ctx, method,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attribute.String("rpc.system", "grpc"), attribute.String("rpc.method", method)))
	if sc := span.SpanContext(); sc.IsValid() {
		ctx = logctx.With(ctx, logctx.From(ctx).With("trace_id", sc.TraceID().String()))
	}

	return ctx, func(err error) {
		code := status.Code(err)
		span.SetAttributes(attribute.String("rpc.grpc.status_code", code.String()))
		if err != nil {
			span.SetStatus(codes.Error, code.String())
		}
		span.End()
		// method is a fixed set (the service's RPCs), so it is a safe label.
		metrics.GRPCRequests.WithLabelValues(method, code.String()).Inc()
		metrics.GRPCDuration.WithLabelValues(method).Observe(time.Since(start).Seconds())
	}
}

func unaryObserve() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, done := serverSpan(ctx, info.FullMethod)
		resp, err := handler(ctx, req)
		done(err)
		return resp, err
	}
}

func streamObserve() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx, done := serverSpan(ss.Context(), info.FullMethod)
		err := handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
		done(err)
		return err
	}
}

// injectTrace adds the current trace context to outgoing metadata.
func injectTrace(ctx context.Context) context.Context {
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	otel.GetTextMapPropagator().Inject(ctx, mdCarrier(md))
	return metadata.NewOutgoingContext(ctx, md)
}

// clientSpan wraps an outgoing call in a client span.
func clientSpan(ctx context.Context, method string) (context.Context, trace.Span) {
	return telemetry.Tracer().Start(ctx, method,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("rpc.system", "grpc"), attribute.String("rpc.method", method)))
}
