// Package database owns the PostgreSQL connection pool and the small
// transaction helpers (tx.go) every repository uses.
package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
