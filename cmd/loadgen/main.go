// Command loadgen drives realistic load against a running API and reports
// throughput and latency percentiles (v1.0).
//
//	RATE_LIMITS=off go run ./cmd/api                    # in one terminal
//	go run ./cmd/loadgen -duration 30s -concurrency 64  # in another
//
// It seeds its own data straight into PostgreSQL (DATABASE_URL): activated
// users with funded wallets, and ACTIVE auctions. It then logs every user
// in through the real API and runs a mix of operations for -duration:
//
//	bid     POST /v1/auctions/{id}/bids    (most bids go to one hot auction)
//	read    GET  /v1/auctions/{id}         (cache-aside path with Redis)
//	search  GET  /v1/auctions/search?q=…
//
// Rate limits must be off on the API (RATE_LIMITS=off): every simulated
// user comes from one IP, and the point is to measure capacity, not the
// limiter.
//
// # Why closed-loop, and what that hides
//
// Each of the -concurrency workers sends its next request only after the
// previous one returns (a closed loop). That measures capacity well, but
// when the server slows down the generator also slows down, so latency
// under overload looks better than real users would experience. This is
// "coordinated omission". For an SLO check at a fixed arrival rate, use
// the k6 script in loadtest/k6, which runs an open model.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/user"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

type options struct {
	base        string
	users       int
	auctions    int
	concurrency int
	duration    time.Duration
	hot         float64
	mix         string
	async       bool
	jsonOut     string
	seedOnly    string
}

func main() {
	_ = godotenv.Load()
	var o options
	flag.StringVar(&o.base, "base", "http://localhost:4000", "API base URL")
	flag.IntVar(&o.users, "users", 100, "simulated bidders")
	flag.IntVar(&o.auctions, "auctions", 20, "auctions to bid on")
	flag.IntVar(&o.concurrency, "concurrency", 32, "concurrent workers (closed loop)")
	flag.DurationVar(&o.duration, "duration", 20*time.Second, "how long to run")
	flag.Float64Var(&o.hot, "hot", 0.5, "fraction of bids that go to the single hottest auction")
	flag.StringVar(&o.mix, "mix", "bid=60,read=30,search=10", "operation mix (weights)")
	flag.BoolVar(&o.async, "async", false, "send bids with Prefer: respond-async (Kafka path)")
	flag.StringVar(&o.jsonOut, "json", "", "also write results as JSON to this file")
	flag.StringVar(&o.seedOnly, "seed-only", "", "seed, log in, write {auctions, tokens} to this JSON file for k6, and exit")
	flag.Parse()

	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "loadgen:", err)
		os.Exit(1)
	}
}

func run(o options) error {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is required (for seeding)")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	mix, err := parseMix(o.mix)
	if err != nil {
		return err
	}

	fmt.Printf("seeding %d users and %d auctions…\n", o.users, o.auctions)
	seed, err := seedData(ctx, pool, o.users, o.auctions)
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			// Keep-alive pool large enough for every worker, otherwise
			// the generator measures TCP handshakes instead of the API.
			MaxIdleConns:        o.concurrency * 2,
			MaxIdleConnsPerHost: o.concurrency * 2,
		},
	}

	fmt.Println("logging users in…")
	tokens, err := login(ctx, client, o.base, seed.emails)
	if err != nil {
		return err
	}

	if o.seedOnly != "" {
		b, _ := json.MarshalIndent(map[string]any{"auctions": seed.auctions, "tokens": tokens, "increment": seed.increment}, "", "  ")
		if err := os.WriteFile(o.seedOnly, b, 0o600); err != nil {
			return err
		}
		fmt.Printf("wrote %d auctions and %d tokens to %s\n", len(seed.auctions), len(tokens), o.seedOnly)
		return nil
	}

	fmt.Printf("running %s with %d workers (mix %s, hot=%.0f%%, async=%v)\n", o.duration, o.concurrency, o.mix, o.hot*100, o.async)
	rec := newRecorder()
	high := make([]atomic.Int64, len(seed.auctions)) // highest amount we've sent per auction
	for i := range high {
		high[i].Store(seed.startingPrice)
	}

	deadline := time.Now().Add(o.duration)
	var wg sync.WaitGroup
	start := time.Now()
	for w := range o.concurrency {
		wg.Go(func() {
			r := rand.New(rand.NewPCG(uint64(w), uint64(time.Now().UnixNano())))
			for time.Now().Before(deadline) {
				op := mix.pick(r)
				i := pickAuction(r, len(seed.auctions), o.hot)
				id := seed.auctions[i]
				switch op {
				case "bid":
					// Bid a little above what we last sent; concurrent
					// workers make some of these too low, which is real.
					amount := high[i].Add(int64(1+r.IntN(5))) + seed.increment
					tok := tokens[r.IntN(len(tokens))]
					body := fmt.Sprintf(`{"amount": %d}`, amount)
					hdr := map[string]string{"Authorization": "Bearer " + tok, "Content-Type": "application/json"}
					if o.async {
						hdr["Prefer"] = "respond-async"
					}
					rec.do(client, "bid", "POST", o.base+"/v1/auctions/"+id.String()+"/bids", body, hdr)
				case "read":
					rec.do(client, "read", "GET", o.base+"/v1/auctions/"+id.String(), "", nil)
				case "search":
					q := searchTerms[r.IntN(len(searchTerms))]
					rec.do(client, "search", "GET", o.base+"/v1/auctions/search?q="+q, "", nil)
				}
			}
		})
	}
	wg.Wait()
	elapsed := time.Since(start)

	report := rec.report(elapsed)
	printReport(report, elapsed)
	if o.jsonOut != "" {
		b, _ := json.MarshalIndent(map[string]any{"options": o.summary(), "elapsed_s": elapsed.Seconds(), "ops": report}, "", "  ")
		if err := os.WriteFile(o.jsonOut, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (o options) summary() map[string]any {
	return map[string]any{"users": o.users, "auctions": o.auctions, "concurrency": o.concurrency,
		"duration": o.duration.String(), "hot": o.hot, "mix": o.mix, "async": o.async}
}

var searchTerms = []string{"camera", "vintage", "watch", "guitar", "lamp", "camra", "bike"}

// pickAuction sends a `hot` fraction of picks to auction 0 and spreads the
// rest uniformly: a few lots get most of the attention.
func pickAuction(r *rand.Rand, n int, hot float64) int {
	if r.Float64() < hot {
		return 0
	}
	return r.IntN(n)
}

// ---------------------------------------------------------------- seeding

type seeded struct {
	emails        []string
	auctions      []uuid.UUID
	startingPrice int64
	increment     int64
}

const password = "load-test-password-123"

func seedData(ctx context.Context, pool *pgxpool.Pool, users, auctions int) (seeded, error) {
	// One Argon2id hash shared by every user: hashing is deliberately slow
	// (~50 ms), and it's the same password anyway.
	hash, err := user.HashPassword(password)
	if err != nil {
		return seeded{}, err
	}
	wallets := wallet.NewService(pool)
	run := uuid.NewString()[:8]

	s := seeded{startingPrice: 100, increment: 1}
	for i := range users {
		id := uuid.New()
		email := fmt.Sprintf("load-%s-%d@loadtest.local", run, i)
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, activated_at) VALUES ($1, $2, $3, now())`, id, email, hash); err != nil {
			return s, err
		}
		if _, err := wallets.Deposit(ctx, id, wallet.MaxDeposit, "load-"+id.String()); err != nil {
			return s, err
		}
		s.emails = append(s.emails, email)
	}

	seller := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, activated_at) VALUES ($1, $2, $3, now())`,
		seller, "load-"+run+"-seller@loadtest.local", hash); err != nil {
		return s, err
	}
	svc := auction.NewService(pool, auction.NewRepository(pool))
	names := []string{"Vintage camera", "Mechanical watch", "Acoustic guitar", "Desk lamp", "Road bike"}
	now := time.Now().UTC()
	for i := range auctions {
		a, err := svc.Create(ctx, seller, auction.CreateInput{
			Item:          auction.CreateItemInput{Name: fmt.Sprintf("%s #%d", names[i%len(names)], i), Type: "load-test"},
			StartingPrice: s.startingPrice, MinIncrement: s.increment,
			StartsAt: now.Add(time.Minute), EndsAt: now.Add(24 * time.Hour),
		})
		if err != nil {
			return s, err
		}
		if _, err := pool.Exec(ctx, `UPDATE auctions SET status = 'ACTIVE', starts_at = now() - interval '1 minute' WHERE id = $1`, a.ID); err != nil {
			return s, err
		}
		s.auctions = append(s.auctions, a.ID)
	}
	return s, nil
}

func login(ctx context.Context, client *http.Client, base string, emails []string) ([]string, error) {
	tokens := make([]string, 0, len(emails))
	for _, email := range emails {
		body, _ := json.Marshal(map[string]string{"email": email, "password": password})
		req, _ := http.NewRequestWithContext(ctx, "POST", base+"/v1/users/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("login: %w (is the API running at %s?)", err, base)
		}
		var out struct {
			AccessToken string `json:"access_token"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, errors.New("login was rate limited: start the API with RATE_LIMITS=off for load tests")
		}
		if resp.StatusCode != http.StatusOK || out.AccessToken == "" {
			return nil, fmt.Errorf("login %s: status %d", email, resp.StatusCode)
		}
		tokens = append(tokens, out.AccessToken)
	}
	return tokens, nil
}

// ---------------------------------------------------------------- mix

type opMix struct {
	names   []string
	weights []int
	total   int
}

func parseMix(s string) (opMix, error) {
	var m opMix
	for _, part := range strings.Split(s, ",") {
		name, w, ok := strings.Cut(strings.TrimSpace(part), "=")
		var n int
		if _, err := fmt.Sscanf(w, "%d", &n); !ok || err != nil || n < 0 {
			return m, fmt.Errorf("bad -mix entry %q (want name=weight)", part)
		}
		if name != "bid" && name != "read" && name != "search" {
			return m, fmt.Errorf("unknown operation %q", name)
		}
		m.names, m.weights, m.total = append(m.names, name), append(m.weights, n), m.total+n
	}
	if m.total == 0 {
		return m, errors.New("-mix weights sum to zero")
	}
	return m, nil
}

func (m opMix) pick(r *rand.Rand) string {
	x := r.IntN(m.total)
	for i, w := range m.weights {
		if x < w {
			return m.names[i]
		}
		x -= w
	}
	return m.names[len(m.names)-1]
}

// ---------------------------------------------------------------- recording

type recorder struct {
	mu      sync.Mutex
	samples map[string][]time.Duration
	codes   map[string]map[int]int
	errors  map[string]int
}

func newRecorder() *recorder {
	return &recorder{samples: map[string][]time.Duration{}, codes: map[string]map[int]int{}, errors: map[string]int{}}
}

func (r *recorder) do(c *http.Client, op, method, url, body string, hdr map[string]string) {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, url, rd)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	start := time.Now()
	resp, err := c.Do(req)
	if err == nil {
		// Read the body fully so the connection is reused (keep-alive).
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	d := time.Since(start)

	// Behave like a well-written client: on 503/429 wait for Retry-After
	// (with jitter, so rejected clients don't all come back in the same
	// instant). A generator that retries instantly turns load shedding
	// into a retry storm and measures something no real client does.
	defer func() {
		if err != nil || (resp.StatusCode != http.StatusServiceUnavailable && resp.StatusCode != http.StatusTooManyRequests) {
			return
		}
		wait := time.Second
		if secs, perr := strconv.Atoi(resp.Header.Get("Retry-After")); perr == nil && secs > 0 {
			wait = time.Duration(secs) * time.Second
		}
		time.Sleep(time.Duration(float64(wait) * (0.5 + rand.Float64()/2)))
	}()

	r.mu.Lock()
	defer r.mu.Unlock()
	if err != nil {
		r.errors[op]++
		return
	}
	r.samples[op] = append(r.samples[op], d)
	if r.codes[op] == nil {
		r.codes[op] = map[int]int{}
	}
	r.codes[op][resp.StatusCode]++
}

type opReport struct {
	Op        string      `json:"op"`
	Requests  int         `json:"requests"`
	PerSecond float64     `json:"per_second"`
	P50ms     float64     `json:"p50_ms"`
	P90ms     float64     `json:"p90_ms"`
	P99ms     float64     `json:"p99_ms"`
	MaxMs     float64     `json:"max_ms"`
	Codes     map[int]int `json:"status_codes"`
	Errors    int         `json:"transport_errors"`
}

func (r *recorder) report(elapsed time.Duration) []opReport {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []opReport
	for _, op := range []string{"bid", "read", "search"} {
		s := r.samples[op]
		if len(s) == 0 && r.errors[op] == 0 {
			continue
		}
		slices.Sort(s)
		q := func(p float64) float64 {
			if len(s) == 0 {
				return 0
			}
			return ms(s[min(len(s)-1, int(p*float64(len(s))))])
		}
		out = append(out, opReport{
			Op: op, Requests: len(s), PerSecond: float64(len(s)) / elapsed.Seconds(),
			P50ms: q(.50), P90ms: q(.90), P99ms: q(.99), MaxMs: q(1),
			Codes: r.codes[op], Errors: r.errors[op],
		})
	}
	return out
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func printReport(ops []opReport, elapsed time.Duration) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "op\trequests\treq/s\tp50 ms\tp90 ms\tp99 ms\tmax ms\tstatus codes\t")
	total := 0
	for _, o := range ops {
		total += o.Requests
		codes := make([]string, 0, len(o.Codes))
		for c, n := range o.Codes {
			codes = append(codes, fmt.Sprintf("%d×%d", c, n))
		}
		slices.Sort(codes)
		if o.Errors > 0 {
			codes = append(codes, fmt.Sprintf("err×%d", o.Errors))
		}
		fmt.Fprintf(tw, "%s\t%d\t%.0f\t%.1f\t%.1f\t%.1f\t%.1f\t%s\t\n",
			o.Op, o.Requests, o.PerSecond, o.P50ms, o.P90ms, o.P99ms, o.MaxMs, strings.Join(codes, " "))
	}
	tw.Flush()
	fmt.Printf("total: %d requests in %s (%.0f req/s)\n", total, elapsed.Round(time.Millisecond), float64(total)/elapsed.Seconds())
}
