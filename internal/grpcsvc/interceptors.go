package grpcsvc

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"runtime/debug"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/Ayush1388/auctionEngine/internal/logctx"
)

// Interceptors are gRPC's middleware: functions wrapped around every call.
// The server chain, outermost first:
//
//	recover → request ID + logging → auth → default deadline → handler
//
// Metadata is gRPC's equivalent of HTTP headers (it travels as HTTP/2
// headers). Keys are lower-case.
const (
	mdAuthorization = "authorization"
	mdRequestID     = "x-request-id"
)

// DefaultCallTimeout is applied when a caller sends no deadline, so a
// forgotten deadline can't let a request hang forever.
const DefaultCallTimeout = 5 * time.Second

// ServerOptions returns the interceptor chain for the bidding service.
//
// internalToken authenticates the caller (the gateway). This is
// service-to-service authentication: end users never talk to this service;
// the gateway has already authenticated them and passes their user ID in
// the request. In a larger deployment you'd use mTLS (each service proves
// its identity with a certificate) instead of a shared secret.
func ServerOptions(logger *slog.Logger, internalToken string) []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			unaryRecover(logger),
			unaryLog(logger),
			unaryAuth(internalToken),
			unaryDeadline(DefaultCallTimeout),
		),
		grpc.ChainStreamInterceptor(
			streamRecover(logger),
			streamLog(logger),
			streamAuth(internalToken),
		),
	}
}

func unaryRecover(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if p := recover(); p != nil {
				logger.Error("grpc panic", "method", info.FullMethod, "panic", p, "stack", string(debug.Stack()))
				err = status.Error(codes.Internal, "internal error")
			}
		}()
		return handler(ctx, req)
	}
}

func streamRecover(logger *slog.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if p := recover(); p != nil {
				logger.Error("grpc panic", "method", info.FullMethod, "panic", p, "stack", string(debug.Stack()))
				err = status.Error(codes.Internal, "internal error")
			}
		}()
		return handler(srv, ss)
	}
}

// requestLogger reuses the caller's request ID (sent by the gateway in
// metadata), so one ID follows a request across both services' logs.
func requestLogger(ctx context.Context, logger *slog.Logger, method string) (context.Context, *slog.Logger) {
	id := ""
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if v := md.Get(mdRequestID); len(v) > 0 {
			id = v[0]
		}
	}
	l := logger.With("grpc_method", method)
	if id != "" {
		l = l.With("request_id", id)
		ctx = logctx.WithRequestID(ctx, id)
	}
	return logctx.With(ctx, l), l
}

func unaryLog(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, l := requestLogger(ctx, logger, info.FullMethod)
		start := time.Now()
		resp, err := handler(ctx, req)
		code := status.Code(err)
		level := slog.LevelInfo
		if code == codes.Internal || code == codes.Unknown {
			level = slog.LevelError
		}
		l.Log(ctx, level, "grpc call", "code", code.String(), "duration_ms", time.Since(start).Milliseconds())
		return resp, err
	}
}

func streamLog(logger *slog.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx, l := requestLogger(ss.Context(), logger, info.FullMethod)
		start := time.Now()
		err := handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
		l.Info("grpc stream", "code", status.Code(err).String(), "duration_ms", time.Since(start).Milliseconds())
		return err
	}
}

func checkToken(ctx context.Context, want string) error {
	if want == "" {
		return nil // auth disabled (local development)
	}
	md, _ := metadata.FromIncomingContext(ctx)
	got := ""
	if v := md.Get(mdAuthorization); len(v) > 0 {
		got = strings.TrimPrefix(v[0], "Bearer ")
	}
	// Constant-time comparison: a normal == returns faster the earlier the
	// first wrong byte is, which leaks the secret one byte at a time.
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		return status.Error(codes.Unauthenticated, "missing or invalid service token")
	}
	return nil
}

// public lists methods that skip service auth: load balancers and
// Kubernetes probe health without credentials.
func public(method string) bool {
	return strings.HasPrefix(method, "/grpc.health.v1.Health/")
}

func unaryAuth(token string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if public(info.FullMethod) {
			return handler(ctx, req)
		}
		if err := checkToken(ctx, token); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func streamAuth(token string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if public(info.FullMethod) {
			return handler(srv, ss)
		}
		if err := checkToken(ss.Context(), token); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

// unaryDeadline gives deadline-less calls a default one. Deadlines
// propagate: the ctx passed to PostgreSQL inherits it, so when the caller
// gives up, the database query is cancelled too instead of running on.
func unaryDeadline(d time.Duration) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, d)
			defer cancel()
		}
		return handler(ctx, req)
	}
}

// wrappedStream overrides Context so handlers see the logger and request ID.
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }

// clientUnary adds the service token and request ID to every outgoing call,
// and a default deadline when the caller didn't set one.
func clientUnary(token string, timeout time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = outgoing(ctx, token)
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func clientStream(token string) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		return streamer(outgoing(ctx, token), desc, cc, method, opts...)
	}
}

func outgoing(ctx context.Context, token string) context.Context {
	pairs := []string{}
	if token != "" {
		pairs = append(pairs, mdAuthorization, "Bearer "+token)
	}
	if id := logctx.RequestID(ctx); id != "" {
		pairs = append(pairs, mdRequestID, id)
	}
	if len(pairs) == 0 {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...)
}
