//go:build e2e

package e2e

// The whole product, end to end, against the real deployment:
//
//	register → activation email (Mailpit) → activate → login
//	→ deposit → create auction → auction starts (lifecycle worker)
//	→ watch it over WebSocket (through Caddy, any API instance)
//	→ bid synchronously (API → gRPC → bidding service)
//	→ retry with the same Idempotency-Key (no double bid)
//	→ bid asynchronously (API → outbox → Kafka → bid worker)
//	→ search (outbox → Elasticsearch) and trending (Redis)
//	→ auction ends → settlement moves the money → books balance
//	→ metrics in Prometheus, traces in Jaeger
//
// Each step is something a unit test can't prove: that the services,
// brokers and databases are wired together correctly in the deployed
// configuration.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
)

var (
	baseURL       = env("E2E_BASE_URL", "http://localhost")
	mailpitURL    = env("E2E_MAILPIT_URL", "http://127.0.0.1:8025")
	prometheusURL = env("E2E_PROMETHEUS_URL", "http://127.0.0.1:9090")
	jaegerURL     = env("E2E_JAEGER_URL", "http://127.0.0.1:16686")
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

const password = "correct-horse-battery-staple"

func TestFullAuctionLifecycle(t *testing.T) {
	run := strings.ReplaceAll(uuid.NewString()[:8], "-", "")

	// ---------------------------------------------------------------
	// 1. Three users sign up through the real email flow.
	// ---------------------------------------------------------------
	seller := signUp(t, "seller-"+run+"@example.com")
	alice := signUp(t, "alice-"+run+"@example.com")
	bob := signUp(t, "bob-"+run+"@example.com")

	// ---------------------------------------------------------------
	// 2. Bidders add money. Deposits are idempotent: replaying the same
	//    key must not credit twice.
	// ---------------------------------------------------------------
	const funds = 1_000_000
	for _, u := range []*client{alice, bob} {
		key := "deposit-" + uuid.NewString()
		u.mustDo(t, "POST", "/v1/wallet/deposits", map[string]any{"amount": funds}, 200, header("Idempotency-Key", key))
		u.mustDo(t, "POST", "/v1/wallet/deposits", map[string]any{"amount": funds}, 200, header("Idempotency-Key", key))
		if w := u.wallet(t); w.Available != funds {
			t.Fatalf("%s: available %d after a replayed deposit, want %d", u.email, w.Available, funds)
		}
	}

	// ---------------------------------------------------------------
	// 3. The seller lists an item. It opens in a few seconds.
	// ---------------------------------------------------------------
	startsAt := time.Now().Add(4 * time.Second).UTC()
	itemName := "Vintage Leica camera " + run
	created := seller.mustDo(t, "POST", "/v1/auctions", map[string]any{
		"item": map[string]any{
			"name":        itemName,
			"type":        "electronics",
			"description": "Rangefinder from 1954, working shutter",
		},
		"starting_price": 1000,
		"min_increment":  100,
		"starts_at":      startsAt.Format(time.RFC3339Nano),
		"ends_at":        startsAt.Add(61 * time.Second).Format(time.RFC3339Nano),
	}, 201)
	auctionID := created["id"].(string)
	t.Logf("auction %s created", auctionID)

	// ---------------------------------------------------------------
	// 4. A viewer watches it live over a WebSocket (through Caddy, so it
	//    lands on either API instance; the Redis fan-out must deliver
	//    updates produced on the other one).
	// ---------------------------------------------------------------
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	updates := watch(t, ctx, auctionID)

	// ---------------------------------------------------------------
	// 5. The lifecycle worker opens the auction on time.
	// ---------------------------------------------------------------
	waitFor(t, 30*time.Second, "auction to become ACTIVE", func() bool {
		return getAuction(t, auctionID)["status"] == "ACTIVE"
	})

	// ---------------------------------------------------------------
	// 6. Synchronous bids: API gateway → gRPC → bidding service.
	// ---------------------------------------------------------------
	key := "bid-" + uuid.NewString()
	first := alice.mustDo(t, "POST", "/v1/auctions/"+auctionID+"/bids", map[string]any{"amount": 1500}, 201, header("Idempotency-Key", key))

	// The client "timed out" and retries with the same key: same bid back,
	// 200 instead of 201, nothing new placed.
	again := alice.mustDo(t, "POST", "/v1/auctions/"+auctionID+"/bids", map[string]any{"amount": 1500}, 200, header("Idempotency-Key", key))
	if bidID(first) != bidID(again) || again["replayed"] != true {
		t.Fatalf("retry with the same Idempotency-Key placed a second bid: %v vs %v", first, again)
	}

	// Too low: the answer says what the minimum is (1500 + 100).
	tooLow := bob.mustDo(t, "POST", "/v1/auctions/"+auctionID+"/bids", map[string]any{"amount": 1550}, 422)
	if num(tooLow["minimum_amount"]) != 1600 {
		t.Fatalf("minimum_amount = %v, want 1600", tooLow["minimum_amount"])
	}
	bob.mustDo(t, "POST", "/v1/auctions/"+auctionID+"/bids", map[string]any{"amount": 2000}, 201, header("Idempotency-Key", uuid.NewString()))

	// The seller can't bid on their own auction.
	seller.mustDo(t, "POST", "/v1/auctions/"+auctionID+"/bids", map[string]any{"amount": 5000}, 403)

	// Alice was outbid, so her reservation was released.
	if w := alice.wallet(t); w.Reserved != 0 || w.Available != funds {
		t.Fatalf("alice after being outbid: %+v, want everything available", w)
	}

	// ---------------------------------------------------------------
	// 7. Asynchronous bid: 202 now; outbox → Kafka (partitioned by
	//    auction) → bid worker places it.
	// ---------------------------------------------------------------
	const winning = 4200
	queued := alice.mustDo(t, "POST", "/v1/auctions/"+auctionID+"/bids", map[string]any{"amount": winning}, 202,
		header("Idempotency-Key", uuid.NewString()), header("Prefer", "respond-async"))
	statusURL := queued["status_url"].(string)

	waitFor(t, 60*time.Second, "async bid to be processed by a bid worker", func() bool {
		r := alice.mustDo(t, "GET", statusURL, nil, 200)
		switch r["status"] {
		case "ACCEPTED":
			return true
		case "PENDING":
			return false
		default:
			t.Fatalf("async bid ended as %v (%v)", r["status"], r["reason"])
			return false
		}
	})

	// The live feed showed the winning bid.
	waitForUpdate(t, updates, 30*time.Second, func(a map[string]any) bool { return num(a["current_bid"]) == winning })

	if w := alice.wallet(t); w.Reserved != winning || w.Available != funds-winning {
		t.Fatalf("alice while leading: %+v, want %d reserved", w, winning)
	}
	if w := bob.wallet(t); w.Reserved != 0 {
		t.Fatalf("bob after being outbid: %+v, want nothing reserved", w)
	}

	// ---------------------------------------------------------------
	// 8. Read models catch up through the outbox: search (Elasticsearch)
	//    and trending (Redis).
	// ---------------------------------------------------------------
	waitFor(t, 60*time.Second, "auction to be searchable", func() bool {
		r := do(t, nil, "GET", "/v1/auctions/search?q="+url.QueryEscape("leica "+run), nil)
		return r.code == 200 && containsID(r.body["auctions"], auctionID)
	})
	search := do(t, nil, "GET", "/v1/auctions/search?q="+url.QueryEscape("leica "+run), nil)
	if search.body["backend"] != "elasticsearch" {
		t.Errorf("search answered by %v, want elasticsearch", search.body["backend"])
	}

	waitFor(t, 30*time.Second, "auction to be trending", func() bool {
		r := do(t, nil, "GET", "/v1/auctions/trending", nil)
		return r.code == 200 && containsID(r.body["auctions"], auctionID)
	})

	// Bid history is public and newest first.
	history := do(t, nil, "GET", "/v1/auctions/"+auctionID+"/bids", nil)
	if bids, _ := history.body["bids"].([]any); len(bids) != 3 {
		t.Fatalf("bid history has %d bids, want 3 (1500, 2000, %d)", len(bids), winning)
	}

	// RBAC: a normal user can't reach operator endpoints.
	alice.mustDo(t, "GET", "/v1/admin/reconcile", nil, 403)

	// ---------------------------------------------------------------
	// 9. The auction ends (the last bid pushed the end out by the
	//    anti-sniping window) and settlement pays the seller.
	// ---------------------------------------------------------------
	waitFor(t, 4*time.Minute, "auction to complete and settle", func() bool {
		a := getAuction(t, auctionID)
		return a["status"] == "COMPLETED" && a["settled_at"] != nil
	})

	if w := seller.wallet(t); w.Available != winning {
		t.Fatalf("seller after settlement: %+v, want %d available", w, winning)
	}
	if w := alice.wallet(t); w.Reserved != 0 || w.Total != funds-winning {
		t.Fatalf("winner after settlement: %+v, want total %d", w, funds-winning)
	}
	if w := bob.wallet(t); w.Total != funds || w.Reserved != 0 {
		t.Fatalf("loser after settlement: %+v, want all %d back", w, funds)
	}

	ledger := seller.mustDo(t, "GET", "/v1/wallet/ledger", nil, 200)
	if entries, _ := ledger["entries"].([]any); len(entries) == 0 || entries[0].(map[string]any)["kind"] != "SETTLE" {
		t.Fatalf("seller's latest ledger entry should be SETTLE: %v", ledger["entries"])
	}

	waitForUpdate(t, updates, 30*time.Second, func(a map[string]any) bool { return a["status"] == "COMPLETED" })

	// ---------------------------------------------------------------
	// 10. Observability: every service is scraped, the bids were
	//     counted, and traces from all three services reached Jaeger.
	// ---------------------------------------------------------------
	checkPrometheus(t)
	checkJaeger(t)
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

type client struct {
	email string
	token string
}

// signUp registers, reads the activation email from Mailpit, activates the
// account and logs in.
func signUp(t *testing.T, email string) *client {
	t.Helper()

	r := do(t, nil, "POST", "/v1/users/register", map[string]any{"email": email, "password": password})
	if r.code != 201 {
		t.Fatalf("register %s: %d %v", email, r.code, r.body)
	}

	token := activationToken(t, email)
	if r := do(t, nil, "GET", "/v1/users/activate?token="+url.QueryEscape(token), nil); r.code != 200 {
		t.Fatalf("activate %s: %d %v", email, r.code, r.body)
	}

	r = do(t, nil, "POST", "/v1/users/login", map[string]any{"email": email, "password": password})
	if r.code != 200 || r.body["access_token"] == "" {
		t.Fatalf("login %s: %d %v", email, r.code, r.body)
	}
	return &client{email: email, token: r.body["access_token"].(string)}
}

var tokenRe = regexp.MustCompile(`token=([A-Za-z0-9_-]+)`)

// activationToken waits for the email the outbox worker sends and pulls
// the token out of its link.
func activationToken(t *testing.T, email string) string {
	t.Helper()

	var token string
	waitFor(t, 60*time.Second, "activation email for "+email, func() bool {
		var list struct {
			Messages []struct {
				ID string
				To []struct{ Address string }
			}
		}
		if err := getJSON(mailpitURL+"/api/v1/messages?limit=200", &list); err != nil {
			return false
		}
		for _, m := range list.Messages {
			for _, to := range m.To {
				if strings.EqualFold(to.Address, email) {
					var msg struct{ Text string }
					if err := getJSON(mailpitURL+"/api/v1/message/"+m.ID, &msg); err != nil {
						return false
					}
					if match := tokenRe.FindStringSubmatch(msg.Text); match != nil {
						token = match[1]
						return true
					}
				}
			}
		}
		return false
	})
	return token
}

type wallet struct{ Available, Reserved, Total int64 }

func (c *client) wallet(t *testing.T) wallet {
	t.Helper()
	b := c.mustDo(t, "GET", "/v1/wallet", nil, 200)
	return wallet{Available: int64(num(b["available"])), Reserved: int64(num(b["reserved"])), Total: int64(num(b["total"]))}
}

func getAuction(t *testing.T, id string) map[string]any {
	t.Helper()
	r := do(t, nil, "GET", "/v1/auctions/"+id, nil)
	if r.code != 200 {
		t.Fatalf("get auction: %d %v", r.code, r.body)
	}
	return r.body
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

type option func(*http.Request)

func header(k, v string) option { return func(r *http.Request) { r.Header.Set(k, v) } }

type response struct {
	code int
	body map[string]any
}

func (c *client) mustDo(t *testing.T, method, path string, body any, want int, opts ...option) map[string]any {
	t.Helper()
	r := do(t, c, method, path, body, opts...)
	if r.code != want {
		t.Fatalf("%s %s as %s: got %d %v, want %d", method, path, c.email, r.code, r.body, want)
	}
	return r.body
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// do sends one request. A 429 is honoured like a well-behaved client would:
// wait for Retry-After, then try again.
func do(t *testing.T, c *client, method, path string, body any, opts ...option) response {
	t.Helper()

	for attempt := 0; ; attempt++ {
		var reader io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			reader = bytes.NewReader(b)
		}
		req, err := http.NewRequest(method, baseURL+path, reader)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if c != nil {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
		for _, o := range opts {
			o(req)
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests && attempt < 10 {
			wait, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
			time.Sleep(time.Duration(max(wait, 1)) * time.Second)
			continue
		}

		out := response{code: resp.StatusCode}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &out.body)
		}
		return out
	}
}

func getJSON(u string, dst any) error {
	resp, err := httpClient.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("GET %s: %d", u, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

func waitFor(t *testing.T, timeout time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if ok() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s", timeout, what)
		}
		time.Sleep(time.Second)
	}
}

func num(v any) float64 {
	f, _ := v.(float64)
	return f
}

func bidID(r map[string]any) any {
	b, _ := r["bid"].(map[string]any)
	return b["id"]
}

func containsID(list any, id string) bool {
	items, _ := list.([]any)
	for _, it := range items {
		if m, ok := it.(map[string]any); ok && m["id"] == id {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// WebSocket
// ---------------------------------------------------------------------------

// watch subscribes to an auction and returns the stream of snapshots.
func watch(t *testing.T, ctx context.Context, auctionID string) <-chan map[string]any {
	t.Helper()

	wsURL := strings.Replace(baseURL, "http", "ws", 1) + "/v1/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	t.Cleanup(func() { conn.Close(websocket.StatusNormalClosure, "") })

	if err := wsjson.Write(ctx, conn, map[string]any{"action": "subscribe", "auction_id": auctionID}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	out := make(chan map[string]any, 64)
	go func() {
		defer close(out)
		for {
			var msg map[string]any
			if err := wsjson.Read(ctx, conn, &msg); err != nil {
				return
			}
			if msg["type"] == "auction.updated" {
				if a, ok := msg["auction"].(map[string]any); ok {
					out <- a
				}
			}
		}
	}()
	return out
}

func waitForUpdate(t *testing.T, updates <-chan map[string]any, timeout time.Duration, match func(map[string]any) bool) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case a, ok := <-updates:
			if !ok {
				t.Fatal("websocket closed before the expected update")
			}
			if match(a) {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for a live update")
		}
	}
}

// ---------------------------------------------------------------------------
// Observability
// ---------------------------------------------------------------------------

func checkPrometheus(t *testing.T) {
	t.Helper()

	// All scrape targets up.
	waitFor(t, 60*time.Second, "all Prometheus targets to be up", func() bool {
		var res struct {
			Data struct {
				ActiveTargets []struct {
					Labels map[string]string
					Health string
				}
			}
		}
		if err := getJSON(prometheusURL+"/api/v1/targets", &res); err != nil {
			return false
		}
		jobs := map[string]bool{}
		for _, tg := range res.Data.ActiveTargets {
			if tg.Health != "up" {
				return false
			}
			jobs[tg.Labels["job"]] = true
		}
		return jobs["api"] && jobs["biddingsvc"] && jobs["bidworker"]
	})

	// The bids above were counted (by whichever process placed them).
	waitFor(t, 60*time.Second, "accepted bids to appear in Prometheus", func() bool {
		var res struct {
			Data struct {
				Result []struct {
					Value []any
				}
			}
		}
		q := url.QueryEscape(`sum(auction_bids_total{result="accepted"})`)
		if err := getJSON(prometheusURL+"/api/v1/query?query="+q, &res); err != nil || len(res.Data.Result) == 0 {
			return false
		}
		v, _ := strconv.ParseFloat(fmt.Sprint(res.Data.Result[0].Value[1]), 64)
		return v >= 3
	})
}

func checkJaeger(t *testing.T) {
	t.Helper()
	waitFor(t, 60*time.Second, "traces from every service in Jaeger", func() bool {
		var res struct{ Data []string }
		if err := getJSON(jaegerURL+"/api/services", &res); err != nil {
			return false
		}
		got := strings.Join(res.Data, ",")
		return strings.Contains(got, "auction-api") &&
			strings.Contains(got, "auction-biddingsvc") &&
			strings.Contains(got, "auction-bidworker")
	})
}
