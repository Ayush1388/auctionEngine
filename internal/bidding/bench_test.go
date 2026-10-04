package bidding_test

// Benchmarks (v1.0). Run against a real PostgreSQL:
//
//	TEST_DATABASE_URL=… go test ./internal/bidding -run '^$' -bench . -benchtime 3s
//
// Two shapes of traffic:
//
//   - HotAuction: every goroutine bids on ONE auction, the last minute of
//     a popular lot. All bids serialise on that auction's row, so
//     throughput is bounded by one transaction at a time, whatever the
//     core count. This is where the two locking strategies differ.
//   - Spread: bids over 64 auctions; rows rarely collide, so throughput
//     scales with connections and cores.
//
// Each run also reports what fraction of attempts were accepted and how
// many hit contention (optimistic retries exhausted), next to ns/op.

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ayush1388/auctionEngine/internal/bidding"
	"github.com/Ayush1388/auctionEngine/internal/testdb"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

func benchUser(b *testing.B, pool *pgxpool.Pool, w *wallet.Service) uuid.UUID {
	id := uuid.New()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, activated_at) VALUES ($1, $2, 'x', now())`, id, id.String()+"@b.test"); err != nil {
		b.Fatal(err)
	}
	if _, err := w.Deposit(ctx, id, wallet.MaxDeposit, "seed-"+id.String()); err != nil {
		b.Fatal(err)
	}
	return id
}

func benchAuction(b *testing.B, pool *pgxpool.Pool, owner uuid.UUID) uuid.UUID {
	itemID, id := uuid.New(), uuid.New()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO items (id, owner_id, type, name, description) VALUES ($1, $2, 't', 'n', '')`, itemID, owner); err != nil {
		b.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO auctions (id, item_id, owner_id, starting_price, min_increment, starts_at, ends_at, status)
		VALUES ($1, $2, $3, 1, 1, now() - interval '1 hour', now() + interval '1 day', 'ACTIVE')`,
		id, itemID, owner); err != nil {
		b.Fatal(err)
	}
	return id
}

func BenchmarkPlaceBid(b *testing.B) {
	for _, auctions := range []int{1, 64} {
		for _, strategy := range strategies {
			name := "Spread/" + string(strategy)
			if auctions == 1 {
				name = "HotAuction/" + string(strategy)
			}
			b.Run(name, func(b *testing.B) { benchPlaceBid(b, strategy, auctions) })
		}
	}
}

func benchPlaceBid(b *testing.B, strategy bidding.Strategy, nAuctions int) {
	pool := testdb.New(b)
	wallets := wallet.NewService(pool)
	svc := bidding.NewService(pool, strategy)

	owner := benchUser(b, pool, wallets)
	ids := make([]uuid.UUID, nAuctions)
	for i := range ids {
		ids[i] = benchAuction(b, pool, owner)
	}

	// One bidder per goroutine (a bidder outbidding themselves is fine).
	const bidders = 32
	users := make([]uuid.UUID, bidders)
	for i := range users {
		users[i] = benchUser(b, pool, wallets)
	}

	var next, accepted, contention, worker atomic.Int64
	// RunParallel starts SetParallelism × GOMAXPROCS goroutines; aim for
	// about `bidders` concurrent clients whatever the core count.
	b.SetParallelism(max(1, bidders/runtime.GOMAXPROCS(0)))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		me := users[int(worker.Add(1)-1)%bidders]
		ctx := context.Background()
		for pb.Next() {
			n := next.Add(1)
			// Increasing amounts; under concurrency some arrive "late"
			// and are rejected as too low, which is realistic.
			_, err := svc.PlaceBid(ctx, bidding.PlaceBidInput{
				AuctionID: ids[int(n)%nAuctions], UserID: me, Amount: 1 + n,
			})
			var tooLow *bidding.BidTooLowError
			switch {
			case err == nil:
				accepted.Add(1)
			case errors.Is(err, bidding.ErrContention):
				contention.Add(1)
			case errors.As(err, &tooLow):
			default:
				b.Error(err)
				return
			}
		}
	})
	b.StopTimer()

	total := float64(next.Load())
	b.ReportMetric(float64(accepted.Load())/total, "accepted/op")
	b.ReportMetric(float64(contention.Load())/total, "contention/op")
	b.ReportMetric(total/b.Elapsed().Seconds(), "bids/s")
}
