package telemetry

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// PGXTracer creates a span for every SQL statement, so a trace shows which
// query was slow. pgx calls TraceQueryStart/End around each query.
//
// Only the statement text is recorded, never the arguments: arguments
// contain user data (emails, password hashes) that must not leak into a
// tracing backend.
type PGXTracer struct{}

type spanKey struct{}

func (PGXTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	// Don't start traces from background polling (outbox, lifecycle): only
	// record SQL that is part of an existing trace.
	if !trace.SpanContextFromContext(ctx).IsValid() {
		return ctx
	}
	ctx, span := Tracer().Start(ctx, "db "+operation(data.SQL), trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.query.text", truncate(data.SQL, 2000)),
		))
	return context.WithValue(ctx, spanKey{}, span)
}

func (PGXTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	span, ok := ctx.Value(spanKey{}).(trace.Span)
	if !ok {
		return
	}
	RecordError(span, data.Err)
	span.End()
}

// operation returns the first SQL keyword ("SELECT", "UPDATE", "WITH"…).
func operation(sql string) string {
	f := strings.Fields(sql)
	if len(f) == 0 {
		return "query"
	}
	return strings.ToUpper(f[0])
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
