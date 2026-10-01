// Package testdb gives integration tests an isolated, fully migrated
// PostgreSQL schema.
//
// Tests are skipped unless TEST_DATABASE_URL is set, so `go test ./...`
// still works on a machine without Postgres.
package testdb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ayush1388/auctionEngine/internal/telemetry"

	"github.com/Ayush1388/auctionEngine/internal/migration"
)

// New creates a fresh schema, runs every migration into it and returns a
// pool whose search_path points at that schema. The schema is dropped when
// the test finishes.
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")

	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer admin.Close()

	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	// Same SQL tracing as production (database.NewPostgresPool).
	config.ConnConfig.Tracer = telemetry.PGXTracer{}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("connect to schema: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()

		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		admin, err := pgxpool.New(cleanupCtx, databaseURL)
		if err != nil {
			t.Logf("cleanup connect: %v", err)
			return
		}
		defer admin.Close()

		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Logf("drop schema: %v", err)
		}
	})

	if err := migration.NewRunner(pool, MigrationsDir(t)).Quiet().Up(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return pool
}

// MigrationsDir finds the repository's migrations folder by walking up
// from the test's working directory to the directory holding go.mod.
func MigrationsDir(t testing.TB) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "migrations")
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal(fmt.Errorf("go.mod not found above %s", dir))
		}
		dir = parent
	}
}
