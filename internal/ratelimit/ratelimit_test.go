package ratelimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTokenBucket(t *testing.T) {
	clock := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	m := NewMemory()
	m.SetClock(func() time.Time { return clock })
	rule := Rule{Name: "test", Rate: 1, Burst: 3} // 1 token/s, bucket of 3
	ctx := context.Background()

	// A new client can burst up to the bucket size.
	for i := 0; i < 3; i++ {
		d, _ := m.Allow(ctx, rule, "alice")
		if !d.Allowed || d.Remaining != 2-i {
			t.Fatalf("request %d: %+v", i+1, d)
		}
	}

	// Then it's empty: rejected, told to wait a full second.
	d, _ := m.Allow(ctx, rule, "alice")
	if d.Allowed || d.RetryAfter != time.Second {
		t.Fatalf("over the burst: %+v", d)
	}

	// Other clients have their own bucket.
	if d, _ := m.Allow(ctx, rule, "bob"); !d.Allowed {
		t.Fatal("bob was limited by alice's usage")
	}

	// Half a second later: half a token, still rejected, wait 0.5s.
	clock = clock.Add(500 * time.Millisecond)
	if d, _ := m.Allow(ctx, rule, "alice"); d.Allowed || d.RetryAfter != 500*time.Millisecond {
		t.Fatalf("after 0.5s: %+v", d)
	}

	// Refill never exceeds the burst, however long the client was idle.
	clock = clock.Add(time.Hour)
	for i := 0; i < 3; i++ {
		if d, _ := m.Allow(ctx, rule, "alice"); !d.Allowed {
			t.Fatalf("after idle, request %d rejected", i+1)
		}
	}
	if d, _ := m.Allow(ctx, rule, "alice"); d.Allowed {
		t.Fatal("bucket refilled past its burst size")
	}
}

func TestCleanupForgetsIdleClients(t *testing.T) {
	clock := time.Now()
	m := NewMemory()
	m.SetClock(func() time.Time { return clock })
	rule := Rule{Name: "t", Rate: 1, Burst: 1}

	for _, ip := range []string{"1", "2", "3"} {
		m.Allow(context.Background(), rule, ip)
	}
	clock = clock.Add(11 * time.Minute)
	m.Allow(context.Background(), rule, "4")

	if removed := m.Cleanup(10 * time.Minute); removed != 3 || m.Len() != 1 {
		t.Fatalf("removed %d, left %d", removed, m.Len())
	}
}

func TestMiddleware(t *testing.T) {
	m := NewMemory()
	rule := Rule{Name: "mw", Rate: 0.001, Burst: 2}
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := Middleware(m, rule, func(r *http.Request) string { return "k" }, ok)

	codes := []int{}
	var last *httptest.ResponseRecorder
	for i := 0; i < 3; i++ {
		last = httptest.NewRecorder()
		h.ServeHTTP(last, httptest.NewRequest("GET", "/", nil))
		codes = append(codes, last.Code)
	}
	if codes[0] != 200 || codes[1] != 200 || codes[2] != 429 {
		t.Fatalf("codes = %v, want [200 200 429]", codes)
	}
	if last.Header().Get("Retry-After") == "" || last.Header().Get("RateLimit-Remaining") != "0" {
		t.Fatalf("missing rate-limit headers: %v", last.Header())
	}
}

func TestClientIP(t *testing.T) {
	c, err := NewClientIP([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		remote string
		xff    string
		want   string
	}{
		{"direct client", "203.0.113.7:5000", "", "203.0.113.7"},
		{"direct client can't spoof X-Forwarded-For", "203.0.113.7:5000", "1.2.3.4", "203.0.113.7"},
		{"behind our proxy", "10.0.0.5:5000", "198.51.100.9", "198.51.100.9"},
		{"forged hop left of the real client is ignored", "10.0.0.5:5000", "6.6.6.6, 198.51.100.9", "198.51.100.9"},
		{"two of our proxies", "10.0.0.5:5000", "198.51.100.9, 10.0.0.9", "198.51.100.9"},
		{"IPv6", "[2001:db8::1]:443", "", "2001:db8::1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tt.remote
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			if got := c.Of(r); got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}
