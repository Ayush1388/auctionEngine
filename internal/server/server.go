package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/Ayush1388/auctionEngine/api"
	"github.com/Ayush1388/auctionEngine/internal/auth"
	"github.com/Ayush1388/auctionEngine/internal/handlers"
	"github.com/Ayush1388/auctionEngine/internal/middleware"
	"github.com/Ayush1388/auctionEngine/internal/ratelimit"
	"github.com/Ayush1388/auctionEngine/internal/user"
)

type Server struct {
	httpServer *http.Server
}

// Deps is everything the router needs. main builds it; tests build their own.
type Deps struct {
	Users    *handlers.UserHandler
	Sessions *handlers.SessionHandler
	Auctions *handlers.AuctionHandler
	Bids     *handlers.BidHandler
	Wallets  *handlers.WalletHandler
	Admin    *handlers.AdminHandler
	Search   *handlers.SearchHandler

	// Realtime serves WebSocket upgrades (v0.7). nil in tests that don't
	// need it.
	Realtime http.Handler

	Auth *auth.Middleware

	// Limiter enforces Limits. nil disables rate limiting (tests).
	Limiter  ratelimit.Limiter
	ClientIP *ratelimit.ClientIP
	Limits   Limits

	Logger      *slog.Logger
	CORSOrigins []string
	HSTS        bool
}

// Limits are the rate-limit rules, applied per client IP unless noted.
type Limits struct {
	Global ratelimit.Rule // every request
	Auth   ratelimit.Rule // register, login, resend, refresh
	Bids   ratelimit.Rule // placing bids, per user
}

// DefaultLimits are generous for real users and tight for scripts.
func DefaultLimits() Limits {
	return Limits{
		Global: ratelimit.Rule{Name: "global", Rate: 20, Burst: 40},
		Auth:   ratelimit.PerMinute("auth", 10, 10),
		Bids:   ratelimit.Rule{Name: "bids", Rate: 5, Burst: 10},
	}
}

// Route is one entry of the API. The router is built from this table, and
// a test checks the OpenAPI document lists exactly these routes.
type Route struct {
	Method  string
	Pattern string
	Handler http.Handler
}

func routes(d Deps) []Route {
	var (
		protected = func(h http.HandlerFunc) http.Handler { return d.Auth.Authenticate(h) }
		optional  = func(h http.HandlerFunc) http.Handler { return d.Auth.Optional(h) }
		admin     = func(h http.HandlerFunc) http.Handler {
			return d.Auth.Authenticate(d.Auth.RequireRole(user.RoleAdmin, h))
		}
		limited = func(rule ratelimit.Rule, key ratelimit.KeyFunc, h http.Handler) http.Handler {
			if d.Limiter == nil {
				return h
			}
			return ratelimit.Middleware(d.Limiter, rule, key, h)
		}
		byIP   = func(r *http.Request) string { return "ip:" + d.ClientIP.Of(r) }
		byUser = func(r *http.Request) string {
			id, _ := auth.UserIDFromContext(r.Context())
			return "user:" + id.String()
		}
		authLimited = func(h http.HandlerFunc) http.Handler { return limited(d.Limits.Auth, byIP, h) }
	)

	return []Route{
		{"GET", "/v1/healthcheck", http.HandlerFunc(handlers.Healthcheck)},
		{"GET", "/v1/openapi.json", http.HandlerFunc(api.ServeOpenAPI)},

		// Users and sessions
		{"POST", "/v1/users/register", authLimited(d.Users.Register)},
		{"GET", "/v1/users/activate", authLimited(d.Users.Activate)},
		{"POST", "/v1/users/resend-activation", authLimited(d.Users.ResendActivation)},
		{"POST", "/v1/users/login", authLimited(d.Users.Login)},
		{"GET", "/v1/users/me", protected(d.Users.Me)},
		{"POST", "/v1/auth/refresh", authLimited(d.Sessions.Refresh)},
		{"POST", "/v1/auth/logout", http.HandlerFunc(d.Sessions.Logout)},

		// Auctions
		{"POST", "/v1/auctions", protected(d.Auctions.Create)},
		{"GET", "/v1/auctions", optional(d.Auctions.List)},
		{"GET", "/v1/auctions/trending", http.HandlerFunc(d.Auctions.Trending)},
		{"GET", "/v1/auctions/search", http.HandlerFunc(d.Search.Search)},
		{"GET", "/v1/auctions/suggest", http.HandlerFunc(d.Search.Suggest)},
		{"GET", "/v1/auctions/{id}", http.HandlerFunc(d.Auctions.Get)},
		{"POST", "/v1/auctions/{id}/cancel", protected(d.Auctions.Cancel)},

		// Live updates over WebSocket (public, read-only stream).
		{"GET", "/v1/ws", realtimeOr(d.Realtime)},

		// Bidding: rate limited per user, after authentication.
		{"POST", "/v1/auctions/{id}/bids", d.Auth.Authenticate(limited(d.Limits.Bids, byUser, http.HandlerFunc(d.Bids.Place)))},
		{"GET", "/v1/auctions/{id}/bids", http.HandlerFunc(d.Bids.History)},

		// Wallet
		{"GET", "/v1/wallet", protected(d.Wallets.Get)},
		{"POST", "/v1/wallet/deposits", protected(d.Wallets.Deposit)},
		{"GET", "/v1/wallet/ledger", protected(d.Wallets.Ledger)},

		// Admin (role "admin" only)
		{"GET", "/v1/admin/reconcile", admin(d.Admin.Reconcile)},
		{"GET", "/v1/admin/outbox/failed", admin(d.Admin.FailedEvents)},
		{"POST", "/v1/admin/outbox/{id}/retry", admin(d.Admin.RetryEvent)},
	}
}

// realtimeOr returns h, or a 503 handler when WebSockets aren't wired.
func realtimeOr(h http.Handler) http.Handler {
	if h != nil {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "live updates are not available", http.StatusServiceUnavailable)
	})
}

// Patterns lists "METHOD /path" for every route (used by the OpenAPI test).
func Patterns() []string {
	var out []string
	for _, r := range routes(Deps{}) {
		out = append(out, r.Method+" "+r.Pattern)
	}
	return out
}

// Routes builds the HTTP handler: the router wrapped in the middleware
// chain. It is separate from New so tests exercise the real stack without
// starting a server.
func Routes(d Deps) http.Handler {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.ClientIP == nil {
		d.ClientIP, _ = ratelimit.NewClientIP(nil)
	}

	mux := http.NewServeMux()
	for _, r := range routes(d) {
		mux.Handle(r.Method+" "+r.Pattern, r.Handler)
	}

	global := http.Handler(mux)
	if d.Limiter != nil {
		global = ratelimit.Middleware(d.Limiter, d.Limits.Global, d.ClientIP.ByIP(), mux)
	}

	return middleware.Chain(global,
		middleware.Recover,
		middleware.RequestID(d.Logger),
		middleware.AccessLog,
		middleware.SecurityHeaders(d.HSTS),
		middleware.CORS(d.CORSOrigins),
	)
}

func New(
	port int,
	handler http.Handler,
) *Server {
	httpServer := &http.Server{
		Addr: fmt.Sprintf(":%d", port),

		Handler: handler,

		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return &Server{
		httpServer: httpServer,
	}
}

// Start blocks until the server stops. It returns nil after a graceful
// Shutdown and an error if the server failed, such as the port being taken.
func (s *Server) Start() error {
	return ignoreClosed(s.httpServer.ListenAndServe())
}

// Serve is like Start but uses an existing listener. Tests use it to pick a
// free port.
func (s *Server) Serve(l net.Listener) error {
	return ignoreClosed(s.httpServer.Serve(l))
}

// RegisterOnShutdown runs f when Shutdown starts. WebSocket connections are
// "hijacked" from net/http, so Shutdown neither waits for nor closes them;
// the realtime hub registers here to close them itself.
func (s *Server) RegisterOnShutdown(f func()) {
	s.httpServer.RegisterOnShutdown(f)
}

// Shutdown stops accepting new connections and waits for in-flight
// requests to finish, or for ctx to expire. (http.Server.Close, which this
// used to call, drops in-flight requests immediately.)
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func ignoreClosed(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
