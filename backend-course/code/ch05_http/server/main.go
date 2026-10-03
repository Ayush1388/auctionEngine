// Chapter 5: a production-shaped net/http server that demonstrates every idea
// from "Understanding HTTP for backend engineers".
//
//	go run ./ch05_http/server
//	curl -i localhost:8082/hello -H 'Accept-Language: es'
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func newHandler() http.Handler {
	a := &api{s: newStore()}
	var h http.Handler = a.routes()
	h = CORS([]string{"http://localhost:5173"})(h) // browser frontend origin
	h = Logging(h)                                 // outermost: logs everything
	return h
}

func main() {
	srv := &http.Server{
		Addr:              ":8082",
		Handler:           newHandler(),
		ReadHeaderTimeout: 5 * time.Second,  // slowloris protection
		ReadTimeout:       30 * time.Second, // whole request incl. body
		IdleTimeout:       60 * time.Second, // keep-alive connections
		// WriteTimeout is deliberately unset: it would kill long SSE streams
		// and big downloads. Use http.ResponseController per-handler instead.
	}

	go func() {
		slog.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown: stop accepting, let in-flight requests finish.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("forced shutdown", "err", err)
	}
	slog.Info("bye")
}
