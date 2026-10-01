package bidqueue_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/bidding"
	"github.com/Ayush1388/auctionEngine/internal/bidqueue"
	"github.com/Ayush1388/auctionEngine/internal/kafkax"
	"github.com/Ayush1388/auctionEngine/internal/outbox"
	"github.com/Ayush1388/auctionEngine/internal/testdb"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// kafka returns broker addresses and a unique topic set. It uses a real
// broker when TEST_KAFKA_BROKERS is set (CI) and an in-process kfake
// cluster otherwise, so these tests always run.
func kafka(t *testing.T) ([]string, kafkax.Topics, *kgo.Client) {
	t.Helper()

	var brokers []string
	if env := os.Getenv("TEST_KAFKA_BROKERS"); env != "" {
		brokers = kafkax.Brokers(env)
	} else {
		cluster, err := kfake.NewCluster(kfake.NumBrokers(1))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(cluster.Close)
		brokers = cluster.ListenAddrs()
	}

	prefix := "t" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12] + "-"
	topics := kafkax.TopicsWithPrefix(prefix)

	producer, err := kafkax.NewProducer(brokers)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(producer.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := kafkax.EnsureTopics(ctx, producer, topics, 1); err != nil {
		t.Fatalf("EnsureTopics: %v", err)
	}
	return brokers, topics, producer
}

type world struct {
	t        *testing.T
	pool     *pgxpool.Pool
	auctions *auction.Service
	bids     *bidding.Service
	requests *bidqueue.Requests
	wallets  *wallet.Service
	brokers  []string
	topics   kafkax.Topics
	producer *kgo.Client
	relay    *outbox.Worker
	seller   uuid.UUID
}

func newWorld(t *testing.T) *world {
	t.Helper()
	pool := testdb.New(t)
	brokers, topics, producer := kafka(t)

	w := &world{
		t: t, pool: pool,
		auctions: auction.NewService(pool, auction.NewRepository(pool)),
		bids:     bidding.NewService(pool, bidding.Pessimistic),
		requests: bidqueue.NewRequests(pool),
		wallets:  wallet.NewService(pool),
		brokers:  brokers, topics: topics, producer: producer,
	}

	// The outbox worker with only the Kafka relay registered: it moves
	// bid.requested events from PostgreSQL into Kafka.
	router := outbox.NewRouter()
	router.Register(bidqueue.EventTypeRequested, kafkax.NewRelay(producer, topics, bidqueue.EventTypeRequested).Handler())
	router.Register(auction.EventTypeCreated, outbox.HandlerFunc(func(context.Context, outbox.Event) error { return nil }))
	router.Register(bidding.EventTypePlaced, outbox.HandlerFunc(func(context.Context, outbox.Event) error { return nil }))
	w.relay = outbox.NewWorker(outbox.NewRepository(pool), router, quiet)

	w.seller = w.user(0)
	return w
}

func (w *world) user(funds int64) uuid.UUID {
	w.t.Helper()
	id := uuid.New()
	if _, err := w.pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, activated_at) VALUES ($1, $2, 'x', now())`, id, id.String()+"@example.com"); err != nil {
		w.t.Fatal(err)
	}
	if funds > 0 {
		if _, err := w.wallets.Deposit(context.Background(), id, funds, "seed-"+id.String()); err != nil {
			w.t.Fatal(err)
		}
	}
	return id
}

func (w *world) activeAuction() uuid.UUID {
	w.t.Helper()
	now := time.Now().UTC()
	a, err := w.auctions.Create(context.Background(), w.seller, auction.CreateInput{
		Item:          auction.CreateItemInput{Name: "Lot", Type: "misc"},
		StartingPrice: 100, StartsAt: now.Add(time.Minute), EndsAt: now.Add(time.Hour),
	})
	if err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.pool.Exec(context.Background(),
		`UPDATE auctions SET status = 'ACTIVE', starts_at = now() - interval '1 minute' WHERE id = $1`, a.ID); err != nil {
		w.t.Fatal(err)
	}
	return a.ID
}

// drainOutbox relays every pending outbox event to Kafka.
func (w *world) drainOutbox() {
	w.t.Helper()
	for i := 0; i < 50; i++ {
		var pending int
		if err := w.pool.QueryRow(context.Background(), `SELECT count(*) FROM outbox_events WHERE processed_at IS NULL AND failed_at IS NULL`).Scan(&pending); err != nil {
			w.t.Fatal(err)
		}
		if pending == 0 {
			return
		}
		if err := w.relay.ProcessOnce(context.Background()); err != nil {
			w.t.Fatal(err)
		}
	}
	w.t.Fatal("outbox did not drain")
}

// startWorkers runs n bid workers in one consumer group until the test ends.
func (w *world) startWorkers(n int, place bidqueue.Placer) []*bidqueue.Worker {
	w.t.Helper()
	if place == nil {
		place = w.bids.PlaceBid
	}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	w.t.Cleanup(func() { cancel(); wg.Wait() })

	var out []*bidqueue.Worker
	for i := 0; i < n; i++ {
		worker, err := bidqueue.NewWorker(w.brokers, "bid-workers-"+w.topics.Bids, w.topics.Bids, w.topics.DLQ, w.producer, place, w.requests, quiet)
		if err != nil {
			w.t.Fatal(err)
		}
		worker.Backoff = 10 * time.Millisecond
		out = append(out, worker)
		wg.Add(1)
		go func() { defer wg.Done(); worker.Run(ctx) }()
	}
	return out
}

func (w *world) waitProcessed(ids []uuid.UUID) map[uuid.UUID]bidqueue.Request {
	w.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		out := map[uuid.UUID]bidqueue.Request{}
		rows, err := w.pool.Query(context.Background(), `SELECT id, user_id FROM bid_requests WHERE id = ANY($1) AND status <> 'PENDING'`, ids)
		if err != nil {
			w.t.Fatal(err)
		}
		type pair struct{ id, user uuid.UUID }
		var done []pair
		for rows.Next() {
			var p pair
			if err := rows.Scan(&p.id, &p.user); err != nil {
				w.t.Fatal(err)
			}
			done = append(done, p)
		}
		rows.Close()
		if len(done) == len(ids) {
			for _, p := range done {
				r, err := w.requests.Get(context.Background(), p.id, p.user)
				if err != nil {
					w.t.Fatal(err)
				}
				out[p.id] = r
			}
			return out
		}
		if time.Now().After(deadline) {
			w.t.Fatalf("only %d of %d requests processed", len(done), len(ids))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (w *world) submit(auctionID, userID uuid.UUID, amount int64, key string) bidqueue.Request {
	w.t.Helper()
	r, _, err := w.requests.Submit(context.Background(), auctionID, userID, amount, key)
	if err != nil {
		w.t.Fatal(err)
	}
	return r
}

func TestAsyncBidEndToEnd(t *testing.T) {
	w := newWorld(t)
	a := w.activeAuction()
	bidder := w.user(10_000)

	req := w.submit(a, bidder, 500, "")
	if req.Status != bidqueue.StatusPending {
		t.Fatalf("new request status %s", req.Status)
	}

	w.drainOutbox()
	w.startWorkers(1, nil)
	got := w.waitProcessed([]uuid.UUID{req.ID})[req.ID]

	if got.Status != bidqueue.StatusAccepted || got.BidID == nil {
		t.Fatalf("request = %+v", got)
	}
	auc, _ := w.auctions.Get(context.Background(), a)
	if auc.CurrentBid == nil || *auc.CurrentBid != 500 {
		t.Fatalf("auction current bid = %v", auc.CurrentBid)
	}
}

// Every bid is higher than the one before. If the worker ever processed
// them out of order, a later (higher) bid would win first and the earlier
// one would be rejected as too low. All 30 accepted proves strict order.
// Four auctions are interleaved to prove other partitions run independently.
func TestBidsForOneAuctionAreProcessedInOrder(t *testing.T) {
	w := newWorld(t)
	auctions := []uuid.UUID{w.activeAuction(), w.activeAuction(), w.activeAuction(), w.activeAuction()}
	bidders := make([]uuid.UUID, 30)
	for i := range bidders {
		bidders[i] = w.user(1_000_000)
	}

	var ids []uuid.UUID
	for i := 0; i < 30; i++ {
		for _, a := range auctions {
			// Alternate bidders so each bid is a new leader.
			ids = append(ids, w.submit(a, bidders[i], int64(1000+i*100), "").ID)
		}
	}

	w.drainOutbox()
	w.startWorkers(2, nil) // two members of the group share the partitions
	results := w.waitProcessed(ids)

	for id, r := range results {
		if r.Status != bidqueue.StatusAccepted {
			t.Fatalf("request %s was %s (%v): bids were processed out of order", id, r.Status, deref(r.Reason))
		}
	}
	for _, a := range auctions {
		auc, _ := w.auctions.Get(context.Background(), a)
		if *auc.CurrentBid != 1000+29*100 || auc.BidCount != 30 {
			t.Fatalf("auction %s: current %d, count %d", a, *auc.CurrentBid, auc.BidCount)
		}
	}
}

// Kafka (and the outbox before it) deliver at least once. The same command
// arriving twice must still place one bid.
func TestDuplicateDeliveryPlacesOneBid(t *testing.T) {
	w := newWorld(t)
	a := w.activeAuction()
	bidder := w.user(10_000)
	req := w.submit(a, bidder, 700, "")

	w.drainOutbox()
	// Simulate a relay crash after Kafka acked but before the outbox row was
	// marked processed: the same record is produced a second time.
	cmd, _ := json.Marshal(bidqueue.Command{RequestID: req.ID, AuctionID: a, UserID: bidder, Amount: 700})
	if err := w.producer.ProduceSync(context.Background(), &kgo.Record{Topic: w.topics.Bids, Key: []byte(a.String()), Value: cmd}).FirstErr(); err != nil {
		t.Fatal(err)
	}

	workers := w.startWorkers(1, nil)
	w.waitProcessed([]uuid.UUID{req.ID})
	deadline := time.Now().Add(10 * time.Second)
	for workers[0].Processed() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	var bids int
	if err := w.pool.QueryRow(context.Background(), `SELECT count(*) FROM bids WHERE auction_id = $1`, a).Scan(&bids); err != nil {
		t.Fatal(err)
	}
	if bids != 1 {
		t.Fatalf("duplicate delivery placed %d bids", bids)
	}
}

func TestRejectionsAreOutcomesNotRetries(t *testing.T) {
	w := newWorld(t)
	a := w.activeAuction()
	poor := w.user(50) // can't afford 500

	tooLow := w.submit(a, w.user(10_000), 10, "")
	noFunds := w.submit(a, poor, 500, "")
	own := w.submit(a, w.seller, 600, "")

	w.drainOutbox()
	w.startWorkers(1, nil)
	res := w.waitProcessed([]uuid.UUID{tooLow.ID, noFunds.ID, own.ID})

	for _, id := range []uuid.UUID{tooLow.ID, noFunds.ID, own.ID} {
		if res[id].Status != bidqueue.StatusRejected || res[id].Reason == nil {
			t.Fatalf("%s: %+v", id, res[id])
		}
	}
	if m := res[tooLow.ID].MinimumAmount; m == nil || *m != 100 {
		t.Fatalf("too-low response should carry the minimum, got %v", m)
	}
}

// A database blip is retried in place (the partition waits, preserving
// order); a permanent failure goes to the dead-letter topic and the
// partition moves on, so one bad message can't block an auction forever.
func TestTransientFailuresRetryAndPermanentOnesDeadLetter(t *testing.T) {
	w := newWorld(t)
	a := w.activeAuction()
	bidder := w.user(100_000)

	flaky := w.submit(a, bidder, 500, "")
	doomed := w.submit(a, bidder, 600, "")
	after := w.submit(a, bidder, 700, "")

	var calls sync.Map
	place := func(ctx context.Context, in bidding.PlaceBidInput) (bidding.Result, error) {
		n, _ := calls.LoadOrStore(in.IdempotencyKey, new(atomic.Int32))
		attempt := n.(*atomic.Int32).Add(1)
		switch in.IdempotencyKey {
		case "req:" + flaky.ID.String():
			if attempt <= 2 {
				return bidding.Result{}, errors.New("connection reset")
			}
		case "req:" + doomed.ID.String():
			return bidding.Result{}, errors.New("disk full")
		}
		return w.bids.PlaceBid(ctx, in)
	}

	w.drainOutbox()
	w.startWorkers(1, place)
	res := w.waitProcessed([]uuid.UUID{flaky.ID, doomed.ID, after.ID})

	if res[flaky.ID].Status != bidqueue.StatusAccepted {
		t.Fatalf("flaky: %+v", res[flaky.ID])
	}
	if res[doomed.ID].Status != bidqueue.StatusFailed {
		t.Fatalf("doomed: %+v", res[doomed.ID])
	}
	if res[after.ID].Status != bidqueue.StatusAccepted {
		t.Fatalf("the partition stayed blocked behind a poison message: %+v", res[after.ID])
	}

	// The doomed command is in the DLQ with the reason.
	dlq, err := kgo.NewClient(kgo.SeedBrokers(w.brokers...), kgo.ConsumeTopics(w.topics.DLQ), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer dlq.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	fetches := dlq.PollRecords(ctx, 10)
	recs := fetches.Records()
	if len(recs) != 1 {
		t.Fatalf("DLQ has %d records", len(recs))
	}
	var reason string
	for _, h := range recs[0].Headers {
		if h.Key == "dlq_reason" {
			reason = string(h.Value)
		}
	}
	if !strings.Contains(reason, "disk full") {
		t.Fatalf("dlq_reason = %q", reason)
	}
}

func TestSubmitIsIdempotentWithKey(t *testing.T) {
	w := newWorld(t)
	a := w.activeAuction()
	bidder := w.user(10_000)

	first, replayed, err := w.requests.Submit(context.Background(), a, bidder, 500, "click-1")
	if err != nil || replayed {
		t.Fatalf("first submit: %v %v", replayed, err)
	}
	again, replayed, err := w.requests.Submit(context.Background(), a, bidder, 500, "click-1")
	if err != nil || !replayed || again.ID != first.ID {
		t.Fatalf("retry should return the same request: %+v %v %v", again, replayed, err)
	}

	var queued int
	if err := w.pool.QueryRow(context.Background(), `SELECT count(*) FROM outbox_events WHERE event_type = $1`, bidqueue.EventTypeRequested).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("queued %d commands for one click", queued)
	}

	if _, _, err := w.requests.Submit(context.Background(), uuid.New(), bidder, 500, ""); !errors.Is(err, bidqueue.ErrAuctionNotFound) {
		t.Fatalf("unknown auction: %v", err)
	}
	// Owner-only reads.
	if _, err := w.requests.Get(context.Background(), first.ID, w.user(0)); !errors.Is(err, bidqueue.ErrNotFound) {
		t.Fatalf("someone else read the request: %v", err)
	}
}

// The relay uses auction_id as the record key, so all of one auction's
// events land in one partition.
func TestRelayKeysByAuction(t *testing.T) {
	w := newWorld(t)
	a := w.activeAuction()
	bidder := w.user(10_000)
	for i := 0; i < 5; i++ {
		w.submit(a, bidder, int64(100+i), "")
	}
	w.drainOutbox()

	cl, err := kgo.NewClient(kgo.SeedBrokers(w.brokers...), kgo.ConsumeTopics(w.topics.Bids), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	partitions := map[int32]bool{}
	got := 0
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for got < 5 {
		fetches := cl.PollRecords(ctx, 10)
		if ctx.Err() != nil {
			t.Fatalf("only %d records", got)
		}
		for _, r := range fetches.Records() {
			if string(r.Key) != a.String() {
				t.Fatalf("key = %q", r.Key)
			}
			partitions[r.Partition] = true
			got++
		}
	}
	if len(partitions) != 1 {
		t.Fatalf("one auction's records spread over partitions %v", partitions)
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return fmt.Sprint(*s)
}
