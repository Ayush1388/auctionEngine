// Package database owns the PostgreSQL connection pool and the small
// transaction helpers (tx.go) every repository uses.
package database

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ayush1388/auctionEngine/internal/chaos"
)

// DefaultMaxConns is the pool size used unless the URL sets pool_max_conns.
const DefaultMaxConns = 20

// NewPostgresPool creates a pgx connection pool and checks it can reach the
// database.
//
// Why a pool: opening a PostgreSQL connection costs a TCP handshake, TLS,
// authentication and a new backend process on the server. A pool keeps a
// set of open connections and lends them to requests, so each query skips
// that cost. The pool also caps how many connections the app can open, which
// protects Postgres (every connection uses server memory).
//
// Ping fails start-up immediately if the database is unreachable, instead
// of on the first request.
func NewPostgresPool(databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}

	// Pool size (v1.0). pgxpool's default is max(4, number of CPUs), which
	// the v1.0 load test showed is far too small: with 32 concurrent
	// clients, nearly every request waited for a free connection
	// (auction_db_pool_empty_acquire_total ≈ request count) and latency was
	// mostly queueing in our own process, not database work.
	//
	// Bigger is not always better, though. Every PostgreSQL connection is
	// a server process with its own memory, and past a few connections per
	// database CPU core extra connections just contend on locks and CPU.
	// Total connections = instances × pool size, which must stay below the
	// server's max_connections (100 by default). Put a pooler (PgBouncer)
	// in front of it when that budget gets tight.
	//
	// The URL can still override this (?pool_max_conns=50).
	if !strings.Contains(databaseURL, "pool_max_conns") {
		config.MaxConns = DefaultMaxConns
	}
	if !strings.Contains(databaseURL, "pool_min_conns") {
		// Keep a few connections open so the first requests after an idle
		// period don't pay for the handshake.
		config.MinConns = 2
	}
	// Recycle connections now and then, so a database failover or a
	// rebalanced proxy doesn't leave the pool pinned to old backends.
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute

	// Every query becomes a span in the request's trace (v1.0). The tracer
	// records only SQL text, never arguments, and only inside an existing
	// trace, so background polling doesn't flood the tracing backend.
	config.ConnConfig.Tracer = chaos.SlowTracer{}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil

}
