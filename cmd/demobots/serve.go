package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ayush1388/auctionEngine/internal/user"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

const (
	maxBidders = 300
	maxRounds  = 20
	_          = 0
)

type server struct {
	api     string
	pool    *pgxpool.Pool
	jwt     *user.JWTService
	origins []string
	client  *http.Client

	mu   sync.Mutex
	bots []bot // provisioned once, reused by every run
	runs map[string]*run
}

type bot struct {
	id    uuid.UUID
	token string
}

// sseEvent is one server-sent event. The frontend (app/live.js, app/trace.js) reads these fields.
type sseEvent struct {
	Type      string  `json:"type"`
	Round     int     `json:"round,omitempty"`
	Rounds    int     `json:"rounds,omitempty"`
	Bidders   int     `json:"bidders,omitempty"`
	Accepted  int     `json:"accepted"`
	Rejected  int     `json:"rejected"`
	Failed    int     `json:"failed,omitempty"`
	Elapsed   float64 `json:"elapsed"`
	P50       float64 `json:"p50,omitempty"`
	P95       float64 `json:"p95,omitempty"`
	PerSecond float64 `json:"perSecond,omitempty"`
	FinalBid  int64   `json:"finalBid,omitempty"`
	BidCount  int     `json:"bidCount,omitempty"`
	Invariant *checks `json:"invariants,omitempty"`
	Error     string  `json:"error,omitempty"`
}

type checks struct {
	OK     bool    `json:"ok"`
	Checks []check `json:"checks"`
}

type check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

type run struct {
	mu     sync.Mutex
	events []sseEvent
	done   bool
	wake   chan struct{}
}

func (r *run) push(e sseEvent) {
	r.mu.Lock()
	r.events = append(r.events, e)
	if e.Type == "done" || e.Type == "error" {
		r.done = true
	}
	old := r.wake
	r.wake = make(chan struct{})
	r.mu.Unlock()
	close(old)
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":4100", "listen address")
	api := fs.String("api", "http://localhost:4000", "the API the bots bid against")
	origins := fs.String("origins", "http://localhost:5173,http://127.0.0.1:5173", "browser origins allowed to call this server")
	_ = fs.Parse(args)

	dbURL, secret := os.Getenv("DATABASE_URL"), os.Getenv("JWT_SECRET")
	if dbURL == "" || secret == "" {
		return errors.New("DATABASE_URL and JWT_SECRET are required (the same values as the API: the bots are real users with real tokens)")
	}
	issuer := os.Getenv("JWT_ISSUER")
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	s := &server{
		api: strings.TrimRight(*api, "/"), pool: pool, jwt: user.NewJWTService(secret, issuer, time.Hour),
		origins: strings.Split(*origins, ","), runs: map[string]*run{},
		client: &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{MaxIdleConns: 400, MaxIdleConnsPerHost: 400}},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok", "api": s.api})
	})
	mux.HandleFunc("POST /stress", s.start)
	mux.HandleFunc("GET /stress/{id}/events", s.events)

	slog.Info("demobots listening", "addr", *addr, "api", s.api, "origins", s.origins)
	return http.ListenAndServe(*addr, s.cors(mux))
}

func (s *server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && slices.Contains(s.origins, o) {
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *server) start(w http.ResponseWriter, r *http.Request) {
	var req struct {
		API       string `json:"api"`
		AuctionID string `json:"auction_id"`
		Bidders   int    `json:"bidders"`
		Rounds    int    `json:"rounds"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	// The bots only ever bid against the API this server was started with.
	if req.API != "" && strings.TrimRight(req.API, "/") != s.api {
		writeJSON(w, 400, map[string]string{"error": "this server bids against " + s.api})
		return
	}
	id, err := uuid.Parse(req.AuctionID)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid auction_id"})
		return
	}
	bidders, rounds := min(max(req.Bidders, 2), maxBidders), min(max(req.Rounds, 1), maxRounds)

	run := &run{wake: make(chan struct{})}
	runID := uuid.NewString()
	s.mu.Lock()
	s.runs[runID] = run
	s.mu.Unlock()
	go s.execute(run, id, bidders, rounds)
	writeJSON(w, 202, map[string]string{"id": runID})
}

// events streams a run's progress; a late subscriber gets everything from the start.
func (s *server) events(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	run := s.runs[r.PathValue("id")]
	s.mu.Unlock()
	if run == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	sent := 0
	for {
		run.mu.Lock()
		batch, wake, done := run.events[sent:], run.wake, run.done
		run.mu.Unlock()
		for _, e := range batch {
			b, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", b)
			sent++
		}
		if flusher != nil {
			flusher.Flush()
		}
		if done && len(batch) == 0 {
			return
		}
		select {
		case <-wake:
		case <-r.Context().Done():
			return
		}
	}
}

/* ---------------------------------------------------------------- the bots */

// provision makes sure n funded bot users exist and have tokens. Tokens are
// minted directly (the same JWT the API would issue) because logging in 300
// Argon2id accounts would measure password hashing, not bidding.
func (s *server) provision(ctx context.Context, n int) ([]bot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.bots) >= n {
		return s.bots[:n], nil
	}
	hash, err := user.HashPassword(uuid.NewString())
	if err != nil {
		return nil, err
	}
	wallets := wallet.NewService(s.pool)
	for i := len(s.bots); i < n; i++ {
		email := fmt.Sprintf("stress-%03d@bots.marque.test", i+1)
		var id uuid.UUID
		if err := s.pool.QueryRow(ctx, `
			INSERT INTO users (id, email, password_hash, activated_at) VALUES ($1, $2, $3, now())
			ON CONFLICT (email) DO UPDATE SET activated_at = COALESCE(users.activated_at, now()) RETURNING id`,
			uuid.New(), email, hash).Scan(&id); err != nil {
			return nil, err
		}
		// One deposit, idempotent by key: $10M covers any single lot, and a bot leads one bid at a time.
		if _, err := wallets.Deposit(ctx, id, wallet.MaxDeposit, "stress-fund-"+id.String()); err != nil {
			return nil, err
		}
		tok, err := s.jwt.GenerateToken(id, user.RoleUser)
		if err != nil {
			return nil, err
		}
		s.bots = append(s.bots, bot{id: id, token: tok})
	}
	return s.bots[:n], nil
}

type auctionView struct {
	Status          string `json:"status"`
	StartingPrice   int64  `json:"starting_price"`
	MinIncrement    int64  `json:"min_increment"`
	CurrentBid      *int64 `json:"current_bid"`
	CurrentBidderID string `json:"current_bidder_id"`
	BidCount        int    `json:"bid_count"`
}

func (a auctionView) minimum() int64 {
	if a.CurrentBid == nil {
		return max(a.StartingPrice, 1)
	}
	return *a.CurrentBid + a.MinIncrement
}

func (s *server) getAuction(ctx context.Context, id uuid.UUID) (auctionView, error) {
	var a auctionView
	req, _ := http.NewRequestWithContext(ctx, "GET", s.api+"/v1/auctions/"+id.String(), nil)
	resp, err := s.client.Do(req)
	if err != nil {
		return a, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return a, fmt.Errorf("auction: HTTP %d", resp.StatusCode)
	}
	return a, json.NewDecoder(resp.Body).Decode(&a)
}

// execute is one run: every round all bots read the same price and bid at the same instant.
func (s *server) execute(r *run, auctionID uuid.UUID, bidders, rounds int) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	fail := func(err error) { r.push(sseEvent{Type: "error", Error: err.Error()}) }

	bots, err := s.provision(ctx, bidders)
	if err != nil {
		fail(fmt.Errorf("could not set up bidders: %w", err))
		return
	}
	before, err := s.getAuction(ctx, auctionID)
	if err != nil {
		fail(err)
		return
	}
	if before.Status != "ACTIVE" {
		fail(fmt.Errorf("the auction is %s, not open for bids", before.Status))
		return
	}

	var accepted, rejected, failed atomic.Int64
	var latMu sync.Mutex
	var lat []float64
	start := time.Now()
	r.push(sseEvent{Type: "progress", Round: 0, Rounds: rounds, Bidders: bidders})

	for round := 1; round <= rounds; round++ {
		a, err := s.getAuction(ctx, auctionID)
		if err != nil || a.Status != "ACTIVE" {
			fail(errors.New("the auction closed during the run"))
			return
		}
		price := a.minimum() // every bot reads this price...
		gate := make(chan struct{})
		var wg sync.WaitGroup
		for i, b := range bots {
			wg.Add(1)
			go func() {
				defer wg.Done()
				amount := price
				if rand.Float64() < 0.2 {
					amount += (1 + rand.Int64N(3)) * a.MinIncrement // some bid higher: a real bidding war
				}
				<-gate // ...and they all fire together
				t0 := time.Now()
				code, tooLow := s.bid(ctx, auctionID, b, amount, fmt.Sprintf("stress-%s-%d-%d", auctionID, round, i))
				ms := float64(time.Since(t0).Microseconds()) / 1000
				latMu.Lock()
				lat = append(lat, ms)
				latMu.Unlock()
				switch {
				case code == 201 || code == 200:
					accepted.Add(1)
				case code == 422 && tooLow:
					rejected.Add(1)
				default:
					failed.Add(1)
				}
			}()
		}
		close(gate)
		wg.Wait()

		latMu.Lock()
		sorted := slices.Clone(lat)
		latMu.Unlock()
		slices.Sort(sorted)
		q := func(p float64) float64 {
			if len(sorted) == 0 {
				return 0
			}
			return sorted[min(len(sorted)-1, int(p*float64(len(sorted))))]
		}
		el := time.Since(start).Seconds()
		n := accepted.Load() + rejected.Load() + failed.Load()
		r.push(sseEvent{Type: "progress", Round: round, Rounds: rounds, Bidders: bidders, Accepted: int(accepted.Load()), Rejected: int(rejected.Load()),
			Failed: int(failed.Load()), Elapsed: el, P50: q(0.5), P95: q(0.95), PerSecond: float64(n) / max(el, 0.001)})
		time.Sleep(250 * time.Millisecond)
	}

	after, err := s.getAuction(ctx, auctionID)
	if err != nil {
		fail(err)
		return
	}
	inv := s.verify(ctx, auctionID, before, after, int(accepted.Load()), int(failed.Load()))
	fin := int64(0)
	if after.CurrentBid != nil {
		fin = *after.CurrentBid
	}
	r.push(sseEvent{Type: "done", Rounds: rounds, Bidders: bidders, Accepted: int(accepted.Load()), Rejected: int(rejected.Load()), Failed: int(failed.Load()),
		Elapsed: time.Since(start).Seconds(), FinalBid: fin, BidCount: after.BidCount, Invariant: inv})
}

// bid places one bid over HTTP. tooLow is true for a 422 that carries minimum_amount.
func (s *server) bid(ctx context.Context, auctionID uuid.UUID, b bot, amount int64, key string) (code int, tooLow bool) {
	body, _ := json.Marshal(map[string]int64{"amount": amount})
	req, _ := http.NewRequestWithContext(ctx, "POST", s.api+"/v1/auctions/"+auctionID.String()+"/bids", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+b.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
	return resp.StatusCode, bytes.Contains(raw, []byte("minimum_amount"))
}

// verify checks the run from the outside, using only what the API returns.
func (s *server) verify(ctx context.Context, id uuid.UUID, before, after auctionView, accepted, failed int) *checks {
	out := &checks{OK: true}
	add := func(name string, ok bool, detail string) {
		out.Checks = append(out.Checks, check{name, ok, detail})
		out.OK = out.OK && ok
	}

	type bidRow struct {
		UserID string `json:"user_id"`
		Amount int64  `json:"amount"`
	}
	var all []bidRow
	cursor := ""
	for page := 0; page < 40; page++ {
		u := s.api + "/v1/auctions/" + id.String() + "/bids?limit=100"
		if cursor != "" {
			u += "&cursor=" + cursor
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
		resp, err := s.client.Do(req)
		if err != nil {
			add("Bid history could be read back", false, err.Error())
			return out
		}
		var p struct {
			Bids       []bidRow `json:"bids"`
			NextCursor *string  `json:"next_cursor"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&p)
		resp.Body.Close()
		all = append(all, p.Bids...)
		if p.NextCursor == nil {
			break
		}
		cursor = *p.NextCursor
	}

	// newest first: every bid must be higher than the one before it in time
	increasing := true
	for i := 0; i+1 < len(all); i++ {
		if all[i].Amount <= all[i+1].Amount {
			increasing = false
		}
	}
	add("Bid history is strictly increasing", increasing, fmt.Sprintf("%d bids read back from the API", len(all)))
	add("New bids equal accepted requests", after.BidCount-before.BidCount == accepted, fmt.Sprintf("%d accepted, the lot gained %d", accepted, after.BidCount-before.BidCount))
	add("Bid count matches the history", after.BidCount == len(all) || len(all) >= 4000, fmt.Sprintf("%d counted, %d listed", after.BidCount, len(all)))
	lead := len(all) > 0 && after.CurrentBid != nil && all[0].Amount == *after.CurrentBid && all[0].UserID == after.CurrentBidderID
	add("One leader, and it is the last accepted bid", lead, "the top of the history is the current bid and bidder")
	add("No request failed with a server error", failed == 0, fmt.Sprintf("%d failures", failed))
	return out
}
