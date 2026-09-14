package migration

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Runner struct {
	db *pgxpool.Pool
}

type Migration struct {
	Version int
	Name    string
	Path    string
}

func NewRunner(db *pgxpool.Pool) *Runner {
	return &Runner{
		db: db,
	}
}

func (r *Runner) Up(ctx context.Context) error {
	lockAcquired, err := r.acquireLock(ctx)
	if err != nil {
		return err
	}

	if !lockAcquired {
		return fmt.Errorf("another migration is currently running")
	}

	defer r.releaseLock(context.Background())

	migrations, err := r.discoverMigrations()
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		applied, err := r.isApplied(ctx, migration.Version)
		if err != nil {
			return err
		}

		if applied {
			fmt.Printf(
				"migration %03d already applied, skipping\n",
				migration.Version,
			)
			continue
		}

		if err := r.applyMigration(ctx, migration); err != nil {
			return err
		}
	}

	return nil
}

func (r *Runner) discoverMigrations() ([]Migration, error) {
	entries, err := os.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf(
			"failed to read migrations directory: %w",
			err,
		)
	}

	var migrations []Migration

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		if !strings.HasSuffix(name, ".up.sql") {
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
			Version: version,
			Name:    name,
			Path:    "migrations/" + name,
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

func (r *Runner) isApplied(
	ctx context.Context,
	version int,
) (bool, error) {
	var applied bool

	err := r.db.QueryRow(
		ctx,
		`
		SELECT EXISTS (
			SELECT 1
			FROM schema_migrations
			WHERE version = $1
		)
		`,
		version,
	).Scan(&applied)

	if err != nil {
		return false, fmt.Errorf(
			"failed to check migration %03d: %w",
			version,
			err,
		)
	}

	return applied, nil
}

func (r *Runner) applyMigration(
	ctx context.Context,
	migration Migration,
) error {
	sqlBytes, err := os.ReadFile(migration.Path)
	if err != nil {
		return fmt.Errorf(
			"failed to read migration %03d: %w",
			migration.Version,
			err,
		)
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf(
			"failed to begin transaction for migration %03d: %w",
			migration.Version,
			err,
		)
	}

	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, string(sqlBytes))
	if err != nil {
		return fmt.Errorf(
			"failed to execute migration %03d: %w",
			migration.Version,
			err,
		)
	}

	_, err = tx.Exec(
		ctx,
		`
		INSERT INTO schema_migrations (version)
		VALUES ($1)
		`,
		migration.Version,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to record migration %03d: %w",
			migration.Version,
			err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf(
			"failed to commit migration %03d: %w",
			migration.Version,
			err,
		)
	}

	fmt.Printf(
		"migration %03d applied successfully\n",
		migration.Version,
	)

	return nil
}

func (r *Runner) acquireLock(ctx context.Context) (bool, error) {
	var acquired bool

	err := r.db.QueryRow(
		ctx,
		"SELECT pg_try_advisory_lock(123456789)",
	).Scan(&acquired)

	if err != nil {
		return false, fmt.Errorf(
			"failed to acquire migration lock: %w",
			err,
		)
	}

	return acquired, nil
}

func (r *Runner) releaseLock(ctx context.Context) {
	_, _ = r.db.Exec(
		ctx,
		"SELECT pg_advisory_unlock(123456789)",
	)
}
