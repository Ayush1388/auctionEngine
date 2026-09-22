package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Ayush1338/auctionEngine/internal/auth"
	"github.com/Ayush1338/auctionEngine/internal/handlers"
)

type Server struct {
	httpServer *http.Server
}

func New(
	port int,
	userHandler *handlers.UserHandler,
	authMiddleware *auth.Middleware,
) *Server {
	mux := http.NewServeMux()

	mux.HandleFunc(
		"GET /v1/healthcheck",
		handlers.Healthcheck,
	)

	// Public user routes.

	mux.HandleFunc(
		"POST /v1/users/register",
		userHandler.Register,
	)

	mux.HandleFunc(
		"GET /v1/users/activate",
		userHandler.Activate,
	)

	mux.HandleFunc(
		"POST /v1/users/resend-activation",
		userHandler.ResendActivation,
	)

	mux.HandleFunc(
		"POST /v1/users/login",
		userHandler.Login,
	)

	// Protected user routes.

	mux.Handle(
		"GET /v1/users/me",
		authMiddleware.Authenticate(
			http.HandlerFunc(userHandler.Me),
		),
	)

	httpServer := &http.Server{
		Addr: fmt.Sprintf(":%d", port),

		Handler: mux,

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
