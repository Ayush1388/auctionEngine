package grpcsvc_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/bidding"
	"github.com/Ayush1388/auctionEngine/internal/breaker"
	pb "github.com/Ayush1388/auctionEngine/internal/gen/biddingv1"
	"github.com/Ayush1388/auctionEngine/internal/grpcsvc"
	"github.com/Ayush1388/auctionEngine/internal/realtime"
	"github.com/Ayush1388/auctionEngine/internal/testdb"
	"github.com/Ayush1388/auctionEngine/internal/validation"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

const token = "test-internal-token-0123456789"

type env struct {
	pool   *pgxpool.Pool
	client *grpcsvc.Client
	conn   *grpc.ClientConn
	hub    *realtime.Hub
	srv    *grpc.Server
	seller uuid.UUID
}

// start runs a real gRPC server on an in-memory listener (bufconn): the
// full gRPC stack, HTTP/2 framing included, without opening a port.
func start(t *testing.T, clientToken string) *env {
	t.Helper()
	pool := testdb.New(t)
	hub := realtime.NewHub(100)

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpcsvc.ServerOptions(quiet, token)...)
	pb.RegisterBiddingServiceServer(srv, grpcsvc.NewServer(bidding.NewService(pool, bidding.Pessimistic), hub))
	hs := health.NewServer()
	healthpb.RegisterHealthServer(srv, hs)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)

	opts := append(grpcsvc.ClientOptions(clientToken),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }))
	conn, err := grpc.NewClient("passthrough:///bufnet", opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	e := &env{pool: pool, client: grpcsvc.NewClientFromConn(conn), conn: conn, hub: hub, srv: srv}
	e.seller = e.user(0)
	return e
}

func (e *env) user(funds int64) uuid.UUID {
	id := uuid.New()
	ctx := context.Background()
	if _, err := e.pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, activated_at) VALUES ($1, $2, 'x', now())`, id, id.String()+"@example.com"); err != nil {
		panic(err)
	}
	if funds > 0 {
		if _, err := wallet.NewService(e.pool).Deposit(ctx, id, funds, "seed-"+id.String()); err != nil {
			panic(err)
		}
	}
	return id
}

func (e *env) activeAuction(t *testing.T) uuid.UUID {
	t.Helper()
	now := time.Now().UTC()
	svc := auction.NewService(e.pool, auction.NewRepository(e.pool))
	a, err := svc.Create(context.Background(), e.seller, auction.CreateInput{
		Item: auction.CreateItemInput{Name: "Lot", Type: "misc"}, StartingPrice: 1000,
		StartsAt: now.Add(time.Minute), EndsAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(context.Background(), `UPDATE auctions SET status = 'ACTIVE', starts_at = now() - interval '1 minute' WHERE id = $1`, a.ID); err != nil {
		t.Fatal(err)
	}
	return a.ID
}

func TestPlaceBidOverGRPC(t *testing.T) {
	e := start(t, token)
	a := e.activeAuction(t)
	bidder := e.user(100_000)
	ctx := context.Background()

	res, err := e.client.PlaceBid(ctx, bidding.PlaceBidInput{AuctionID: a, UserID: bidder, Amount: 1500, IdempotencyKey: "k1"})
	if err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	if res.CurrentBid != 1500 || res.BidCount != 1 || res.Replayed || res.Bid.UserID != bidder {
		t.Fatalf("result = %+v", res)
	}

	again, err := e.client.PlaceBid(ctx, bidding.PlaceBidInput{AuctionID: a, UserID: bidder, Amount: 1500, IdempotencyKey: "k1"})
	if err != nil || !again.Replayed || again.Bid.ID != res.Bid.ID {
		t.Fatalf("idempotent replay over gRPC: %+v %v", again, err)
	}

	page, err := e.client.History(ctx, a, "", "")
	if err != nil || len(page.Bids) != 1 || page.Bids[0].Amount != 1500 {
		t.Fatalf("History: %+v %v", page, err)
	}
}

// The client must hand back exactly the errors the in-process service
// returns, so the HTTP handlers' errors.Is/As switch keeps working.
func TestErrorsSurviveTheNetwork(t *testing.T) {
	e := start(t, token)
	a := e.activeAuction(t)
	bidder := e.user(100_000)
	poor := e.user(10)
	ctx := context.Background()

	_, err := e.client.PlaceBid(ctx, bidding.PlaceBidInput{AuctionID: a, UserID: bidder, Amount: 5})
	var tooLow *bidding.BidTooLowError
	if !errors.As(err, &tooLow) || tooLow.Minimum != 1000 {
		t.Fatalf("too low: %v", err)
	}

	cases := []struct {
		name string
		in   bidding.PlaceBidInput
		want error
	}{
		{"unknown auction", bidding.PlaceBidInput{AuctionID: uuid.New(), UserID: bidder, Amount: 2000}, auction.ErrNotFound},
		{"own auction", bidding.PlaceBidInput{AuctionID: a, UserID: e.seller, Amount: 2000}, bidding.ErrOwnAuction},
		{"no funds", bidding.PlaceBidInput{AuctionID: a, UserID: poor, Amount: 2000}, wallet.ErrInsufficientFunds},
		{"bad amount", bidding.PlaceBidInput{AuctionID: a, UserID: bidder, Amount: -1}, bidding.ErrInvalidAmount},
	}
	for _, c := range cases {
		if _, err := e.client.PlaceBid(ctx, c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}

	// Validation errors keep their per-field messages (BadRequest detail).
	_, err = e.client.History(ctx, a, "0", "")
	var problems *validation.Error
	if !errors.As(err, &problems) || problems.Fields["limit"] == "" {
		t.Fatalf("validation error lost its fields: %v", err)
	}
}

func TestWrongServiceTokenIsRejected(t *testing.T) {
	e := start(t, "not-the-token")
	_, err := e.client.PlaceBid(context.Background(), bidding.PlaceBidInput{AuctionID: uuid.New(), UserID: uuid.New(), Amount: 1})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("got %v, want UNAUTHENTICATED", err)
	}
}

// When the caller's deadline passes, the call fails fast AND the database
// work is cancelled with it: the deadline travels inside the context from
// the gateway, through gRPC, into pgx.
func TestDeadlinePropagatesToTheDatabase(t *testing.T) {
	e := start(t, token)
	a := e.activeAuction(t)
	bidder := e.user(100_000)

	// Another transaction holds the auction's row lock, so PlaceBid will
	// wait on SELECT … FOR UPDATE.
	lockTx, err := e.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer lockTx.Rollback(context.Background())
	if _, err := lockTx.Exec(context.Background(), `SELECT 1 FROM auctions WHERE id = $1 FOR UPDATE`, a); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = e.client.PlaceBid(ctx, bidding.PlaceBidInput{AuctionID: a, UserID: bidder, Amount: 1500})
	if !errors.Is(err, bidding.ErrUnavailable) {
		t.Fatalf("got %v, want ErrUnavailable (deadline exceeded)", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("call took %s; the deadline wasn't honoured", time.Since(start))
	}

	// The server-side query must have been cancelled, not left waiting on
	// the lock.
	deadline := time.Now().Add(3 * time.Second)
	for {
		var waiting int
		if err := e.pool.QueryRow(context.Background(), `
			SELECT count(*) FROM pg_stat_activity
			WHERE wait_event_type = 'Lock' AND query ILIKE '%FOR UPDATE%' AND pid <> pg_backend_pid()
		`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the database query kept waiting after the caller gave up")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestWatchAuctionStreamsSnapshots(t *testing.T) {
	e := start(t, token)
	id := uuid.New()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := e.client.Watch(ctx, id)
	if err != nil {
		t.Fatal(err)
	}

	// Wait until the server has registered the watcher, then publish the
	// same JSON the realtime event handler fans out.
	deadline := time.Now().Add(3 * time.Second)
	for n, _ := e.hub.Stats(); n == 0; n, _ = e.hub.Stats() {
		if time.Now().After(deadline) {
			t.Fatal("watcher never registered")
		}
		time.Sleep(5 * time.Millisecond)
	}
	for v := 1; v <= 3; v++ {
		e.hub.Deliver(id, []byte(`{"type":"auction.updated","cause":"bid.placed","auction":{"id":"`+id.String()+`","status":"ACTIVE","bid_count":`+string(rune('0'+v))+`,"version":`+string(rune('0'+v))+`}}`))
	}

	for v := int64(1); v <= 3; v++ {
		snap, err := stream.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if snap.GetVersion() != v || snap.GetCause() != "bid.placed" || snap.GetId() != id.String() {
			t.Fatalf("snapshot %d = %v", v, snap)
		}
	}

	// Cancelling the stream on the client releases the server-side watcher.
	cancel()
	deadline = time.Now().Add(3 * time.Second)
	for n, _ := e.hub.Stats(); n != 0; n, _ = e.hub.Stats() {
		if time.Now().After(deadline) {
			t.Fatal("server kept the watcher after the client cancelled")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Health checks need no service token (probes don't have one).
func TestHealthCheck(t *testing.T) {
	e := start(t, "")
	resp, err := healthpb.NewHealthClient(e.conn).Check(context.Background(), &healthpb.HealthCheckRequest{})
	if err != nil || resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("health: %v %v", resp, err)
	}
}

// With the bidding service down, the breaker opens after a few failures
// and later calls fail at once with ErrUnavailable (HTTP 503), without
// waiting on the network. Business errors never open it.
func TestBreakerFailsFastWhenServiceIsDown(t *testing.T) {
	e := start(t, token)
	a := e.activeAuction(t)
	bidder := e.user(100_000)
	b := breaker.New("bidding-test", breaker.Options{Threshold: 2, Cooldown: time.Hour, IsFailure: grpcsvc.BreakerFailure})
	client := grpcsvc.NewClientFromConn(e.conn).WithBreaker(b)
	ctx := context.Background()

	// Rejections are the service working correctly: breaker stays closed.
	for range 5 {
		if _, err := client.PlaceBid(ctx, bidding.PlaceBidInput{AuctionID: a, UserID: bidder, Amount: 1}); errors.Is(err, bidding.ErrUnavailable) {
			t.Fatalf("too-low bid reported as unavailable: %v", err)
		}
	}
	if b.State() != breaker.Closed {
		t.Fatalf("business errors opened the breaker")
	}

	e.srv.Stop()
	for range 2 {
		short, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		_, err := client.PlaceBid(short, bidding.PlaceBidInput{AuctionID: a, UserID: bidder, Amount: 2000})
		cancel()
		if !errors.Is(err, bidding.ErrUnavailable) {
			t.Fatalf("service down: %v", err)
		}
	}
	if b.State() != breaker.Open {
		t.Fatalf("state = %s, want open", b.State())
	}

	start := time.Now()
	_, err := client.PlaceBid(ctx, bidding.PlaceBidInput{AuctionID: a, UserID: bidder, Amount: 2000})
	if !errors.Is(err, bidding.ErrUnavailable) || !errors.Is(err, breaker.ErrOpen) {
		t.Fatalf("open breaker: %v", err)
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Fatalf("open breaker still waited %s", time.Since(start))
	}
}
