package migration_test

import (
	"context"
	"testing"

	"github.com/Ayush1388/auctionEngine/internal/migration"
	"github.com/Ayush1388/auctionEngine/internal/testdb"
)

func TestUpIsIdempotent(t *testing.T) {
	pool := testdb.New(t)
	runner := migration.NewRunner(pool, testdb.MigrationsDir(t)).Quiet()

	// testdb.New already applied everything; a second run must be a no-op.
	if err := runner.Up(context.Background()); err != nil {
		t.Fatalf("second Up: %v", err)
	}
}

func TestDownAllThenUpAgain(t *testing.T) {
	pool := testdb.New(t)
	runner := migration.NewRunner(pool, testdb.MigrationsDir(t)).Quiet()
	ctx := context.Background()

	var applied int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&applied); err != nil {
		t.Fatal(err)
	}

	if err := runner.Down(ctx, applied); err != nil {
		t.Fatalf("Down(%d): %v", applied, err)
	}

	var remaining int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("expected 0 applied migrations after full rollback, got %d", remaining)
	}

	var tables int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_name <> 'schema_migrations'
	`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Fatalf("expected no tables after full rollback, found %d", tables)
	}

	if err := runner.Up(ctx); err != nil {
		t.Fatalf("Up after rollback: %v", err)
	}
}
