// Package database owns the PostgreSQL connection pool and the small
// transaction helpers (tx.go) every repository uses.
package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ayush1388/auctionEngine/internal/telemetry"
)

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

	// Every query becomes a span in the request's trace (v1.0). The tracer
	// records only SQL text, never arguments, and only inside an existing
	// trace, so background polling doesn't flood the tracing backend.
	config.ConnConfig.Tracer = telemetry.PGXTracer{}

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
