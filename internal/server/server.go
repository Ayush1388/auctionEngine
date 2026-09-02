package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Ayush1338/auctionEngine/internal/handlers"
)

type Server struct {
	httpServer *http.Server
}

func New(port int) *Server {
	mux := http.NewServeMux()

	mux.HandleFunc(
		"GET /v1/healthcheck",
		handlers.Healthcheck,
	)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
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
