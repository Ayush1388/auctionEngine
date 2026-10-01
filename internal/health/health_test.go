package health_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ayush1388/auctionEngine/internal/health"
)

func ready(t *testing.T, c *health.Checker) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	c.Ready(rec, httptest.NewRequest("GET", "/readyz", nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return rec.Code, body
}

func TestReadiness(t *testing.T) {
	ok := func(context.Context) error { return nil }
	down := func(context.Context) error { return errors.New("connection refused") }

	t.Run("all healthy", func(t *testing.T) {
		c := health.New()
		c.Add("postgres", true, ok)
		if code, _ := ready(t, c); code != 200 {
			t.Fatalf("code = %d", code)
		}
	})

	t.Run("critical dependency down", func(t *testing.T) {
		c := health.New()
		c.Add("postgres", true, down)
		code, body := ready(t, c)
		if code != 503 || body["checks"].(map[string]any)["postgres"] != "fail" {
			t.Fatalf("code = %d body = %v", code, body)
		}
	})

	t.Run("optional dependency down keeps the instance in rotation", func(t *testing.T) {
		c := health.New()
		c.Add("postgres", true, ok)
		c.Add("redis", false, down)
		code, body := ready(t, c)
		if code != 200 || body["checks"].(map[string]any)["redis"] != "fail" {
			t.Fatalf("code = %d body = %v", code, body)
		}
	})

	t.Run("a hung check times out instead of hanging the probe", func(t *testing.T) {
		c := health.New()
		c.Add("postgres", true, func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })
		if code, _ := ready(t, c); code != 503 {
			t.Fatalf("code = %d", code)
		}
	})

	t.Run("draining", func(t *testing.T) {
		c := health.New()
		c.Add("postgres", true, ok)
		c.Drain()
		code, body := ready(t, c)
		if code != 503 || body["status"] != "draining" {
			t.Fatalf("code = %d body = %v", code, body)
		}
		// Liveness is unaffected: a draining process is still alive.
		rec := httptest.NewRecorder()
		c.Live(rec, httptest.NewRequest("GET", "/livez", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("livez = %d", rec.Code)
		}
	})
}
