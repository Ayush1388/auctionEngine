package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Ayush1388/auctionEngine/internal/auth"
	"github.com/Ayush1388/auctionEngine/internal/handlers"
)

type Server struct {
	httpServer *http.Server
}

// Routes builds the HTTP handler. It is separate from New so tests can
// exercise the real routing without starting a server.
func Routes(
	userHandler *handlers.UserHandler,
	auctionHandler *handlers.AuctionHandler,
	authMiddleware *auth.Middleware,
) http.Handler {
	mux := http.NewServeMux()

	protected := func(h http.HandlerFunc) http.Handler {
		return authMiddleware.Authenticate(h)
	}

	mux.HandleFunc("GET /v1/healthcheck", handlers.Healthcheck)

	// Users
	mux.HandleFunc("POST /v1/users/register", userHandler.Register)
	mux.HandleFunc("GET /v1/users/activate", userHandler.Activate)
	mux.HandleFunc("POST /v1/users/resend-activation", userHandler.ResendActivation)
	mux.HandleFunc("POST /v1/users/login", userHandler.Login)
	mux.Handle("GET /v1/users/me", protected(userHandler.Me))

	// Auctions
	mux.Handle("POST /v1/auctions", protected(auctionHandler.Create))
	mux.HandleFunc("GET /v1/auctions/{id}", auctionHandler.Get)

	return mux
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

func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown() error {
	return s.httpServer.Close()
}
