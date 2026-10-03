package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Five sequential requests must share ONE TCP connection (HTTP/1.1 keep-alive).
func TestKeepAliveReusesConnection(t *testing.T) {
	var conns atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":"yes"}`))
	}))
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			conns.Add(1)
		}
	}
	srv.Start()
	defer srv.Close()

	c := newClient()
	for i := 0; i < 5; i++ {
		var out map[string]string
		if err := getJSON(context.Background(), c, srv.URL, &out); err != nil {
			t.Fatal(err)
		}
	}
	if got := conns.Load(); got != 1 {
		t.Fatalf("opened %d TCP connections for 5 requests, want 1", got)
	}
}

func TestBadStatusIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	var out map[string]string
	if err := getJSON(context.Background(), newClient(), srv.URL, &out); err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestContextCancelsSlowServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	var out map[string]string
	if err := getJSON(ctx, newClient(), srv.URL, &out); err == nil {
		t.Fatal("expected timeout error")
	}
	if time.Since(start) > time.Second {
		t.Fatal("context did not cancel the request promptly")
	}
}
