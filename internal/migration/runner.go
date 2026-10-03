// Package migration applies versioned SQL files from migrations/ in order
// and records each one in schema_migrations, so every environment ends up
// with the same schema. Each migration runs in its own transaction: a
// failing migration leaves no half-applied state behind. (PostgreSQL, unlike
// MySQL, can roll back DDL such as CREATE TABLE.)
package migration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// lockKey identifies the advisory lock that serialises migration runs.
const lockKey = 123456789

var ErrMissingDownMigration = errors.New("missing down migration")

type Runner struct {
	db  *pgxpool.Pool
	dir string
	out io.Writer // progress messages; os.Stdout by default
}

type Migration struct {
	Version  int
	Name     string
	UpPath   string
	DownPath string
}

func NewRunner(db *pgxpool.Pool, dir string) *Runner {
	return &Runner{
		db:  db,
		dir: dir,
		out: os.Stdout,
	}
}

// Quiet discards progress messages (tests use it).
func (r *Runner) Quiet() *Runner {
	r.out = io.Discard
	return r
}

// Up applies every migration that has not been applied yet, in version order.
func (r *Runner) Up(ctx context.Context) error {
	return r.withLock(ctx, func(conn *pgxpool.Conn) error {
		migrations, err := r.discoverMigrations()
		if err != nil {
			return err
		}

		applied, err := appliedVersions(ctx, conn)
		if err != nil {
			return err
		}

		for _, migration := range migrations {
			if applied[migration.Version] {
				continue
			}

			if err := apply(ctx, conn, migration, r.out); err != nil {
				return err
			}
		}

		return nil
	})
}

// Down rolls back the most recently applied migrations, newest first.
func (r *Runner) Down(ctx context.Context, steps int) error {
	if steps < 1 {
		return fmt.Errorf("steps must be at least 1, got %d", steps)
	}

	return r.withLock(ctx, func(conn *pgxpool.Conn) error {
		migrations, err := r.discoverMigrations()
		if err != nil {
			return err
		}

		applied, err := appliedVersions(ctx, conn)
		if err != nil {
			return err
		}

		rolledBack := 0

		for i := len(migrations) - 1; i >= 0 && rolledBack < steps; i-- {
			migration := migrations[i]

			if !applied[migration.Version] {
				continue
			}

			if err := rollback(ctx, conn, migration, r.out); err != nil {
				return err
			}

			rolledBack++
		}

		return nil
	})
}

// withLock runs fn on a single dedicated connection while holding the
// migration advisory lock. Advisory locks belong to a session, so the lock
// and unlock must happen on the same connection, not on whichever pooled
// connection happens to be free.
func (r *Runner) withLock(
	ctx context.Context,
	fn func(conn *pgxpool.Conn) error,
) error {
	conn, err := r.db.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	// Blocks until any other migration run has finished.
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		return fmt.Errorf("failed to acquire migration lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", lockKey)
	}()

	// The tracking table must exist before we can ask what has been applied.
	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			applied_at TIMESTAMP(0) WITH TIME ZONE NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("failed to create schema_migrations: %w", err)
	}

	return fn(conn)
}

func (r *Runner) discoverMigrations() ([]Migration, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to read migrations directory: %w",
			err,
		)
	}

	var migrations []Migration

	for _, entry := range entries {
		name := entry.Name()

		if entry.IsDir() || !strings.HasSuffix(name, ".up.sql") {
			continue
		}

		parts := strings.SplitN(name, "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf(
				"invalid migration filename: %s",
				name,
			)
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf(
				"invalid migration version in %s: %w",
				name,
				err,
			)
		}

		migrations = append(migrations, Migration{
			Version:  version,
			Name:     name,
			UpPath:   filepath.Join(r.dir, name),
			DownPath: filepath.Join(r.dir, strings.TrimSuffix(name, ".up.sql")+".down.sql"),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	for i := 1; i < len(migrations); i++ {
		if migrations[i].Version == migrations[i-1].Version {
			return nil, fmt.Errorf(
				"duplicate migration version %03d",
				migrations[i].Version,
			)
		}
	}

	return migrations, nil
}

func appliedVersions(
	ctx context.Context,
	conn *pgxpool.Conn,
) (map[int]bool, error) {
	rows, err := conn.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to read applied migrations: %w", err)
	}

	versions, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, fmt.Errorf("failed to read applied migrations: %w", err)
	}

	applied := make(map[int]bool, len(versions))
	for _, v := range versions {
		applied[int(v)] = true
	}

	return applied, nil
}

func apply(
	ctx context.Context,
	conn *pgxpool.Conn,
	migration Migration,
	out io.Writer,
) error {
	return runInTx(ctx, conn, out, migration.UpPath, migration.Version, "applied", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", migration.Version)
		return err
	})
}

func rollback(
	ctx context.Context,
	conn *pgxpool.Conn,
	migration Migration,
	out io.Writer,
) error {
	if _, err := os.Stat(migration.DownPath); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w for %03d", ErrMissingDownMigration, migration.Version)
	}

	return runInTx(ctx, conn, out, migration.DownPath, migration.Version, "rolled back", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "DELETE FROM schema_migrations WHERE version = $1", migration.Version)
		return err
	})
}

// runInTx executes one migration file and updates schema_migrations in the
// same transaction, so a failed migration leaves no partial state behind.
func runInTx(
	ctx context.Context,
	conn *pgxpool.Conn,
	out io.Writer,
	path string,
	version int,
	verb string,
	record func(tx pgx.Tx) error,
) error {
	sqlBytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read migration %03d: %w", version, err)
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction for migration %03d: %w", version, err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
		return fmt.Errorf("failed to execute migration %03d: %w", version, err)
	}

	if err := record(tx); err != nil {
		return fmt.Errorf("failed to record migration %03d: %w", version, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit migration %03d: %w", version, err)
	}

	fmt.Fprintf(out, "migration %03d %s\n", version, verb)

	return nil
}
