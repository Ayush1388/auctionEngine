package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// Shutdown must let a request that is already running finish.
func TestShutdownDrainsInFlightRequests(t *testing.T) {
	started := make(chan struct{})

	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		time.Sleep(300 * time.Millisecond)
		_, _ = io.WriteString(w, "done")
	})

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	srv := New(0, slow)
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(l) }()

	type result struct {
		status int
		body   string
		err    error
	}
	res := make(chan result, 1)

	go func() {
		resp, err := http.Get("http://" + l.Addr().String())
		if err != nil {
			res <- result{err: err}
			return
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		res <- result{status: resp.StatusCode, body: string(b)}
	}()

	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	r := <-res
	if r.err != nil || r.status != http.StatusOK || r.body != "done" {
		t.Fatalf("in-flight request was not completed: %+v", r)
	}

	if err := <-serveErr; err != nil {
		t.Fatalf("Serve returned %v after a graceful shutdown, want nil", err)
	}
}

func TestShutdownRespectsDeadline(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)

	stuck := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
	})

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	srv := New(0, stuck)
	go func() { _ = srv.Serve(l) }()
	go func() { _, _ = http.Get("http://" + l.Addr().String()) }()
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := srv.Shutdown(ctx); err == nil {
		t.Fatal("expected Shutdown to report the missed deadline")
	}
}
