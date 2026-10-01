// Command admin performs operator tasks that must never be reachable over
// the public API, such as granting the admin role.
//
//	go run ./cmd/admin promote alice@example.com
//	go run ./cmd/admin demote  alice@example.com
//	go run ./cmd/admin reindex   rebuild the Elasticsearch index from PostgreSQL
//
// The change reaches the user's access tokens at their next login or token
// refresh (see user.User.Role).
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/joho/godotenv"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/database"
	"github.com/Ayush1388/auctionEngine/internal/search"
	"github.com/Ayush1388/auctionEngine/internal/user"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "admin:", err)
		os.Exit(1)
	}
}

const usage = "usage: admin promote|demote <email> | admin reindex"

func run(args []string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}

	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errors.New("DATABASE_URL is required")
	}

	pool, err := database.NewPostgresPool(url)
	if err != nil {
		return err
	}
	defer pool.Close()

	switch {
	case args[0] == "reindex" && len(args) == 1:
		return reindex(pool)
	case (args[0] == "promote" || args[0] == "demote") && len(args) == 2:
		return setRole(pool, args)
	default:
		return errors.New(usage)
	}
}

// reindex rebuilds the search index with zero downtime: a new index is
// filled from PostgreSQL in batches, then the alias is switched to it in
// one atomic step (see search.Elastic.Reindex).
func reindex(pool *pgxpool.Pool) error {
	esURL := os.Getenv("ELASTICSEARCH_URL")
	if esURL == "" {
		return errors.New("ELASTICSEARCH_URL is required")
	}
	alias := os.Getenv("ELASTICSEARCH_INDEX")
	if alias == "" {
		alias = "auctions"
	}

	repo := auction.NewRepository(pool)
	var after *auction.Cursor
	done := false

	// Page through every auction with the same keyset pagination as the
	// public listing, 500 at a time.
	next := func(ctx context.Context) ([]auction.Auction, error) {
		if done {
			return nil, nil
		}
		page, err := repo.List(ctx, auction.ListFilter{}, after, 500)
		if err != nil {
			return nil, err
		}
		after = page.NextCursor
		done = page.NextCursor == nil
		return page.Auctions, nil
	}

	n, err := search.NewElastic(esURL, alias).Reindex(context.Background(), next)
	if err != nil {
		return err
	}
	fmt.Printf("reindexed %d auctions into %q\n", n, alias)
	return nil
}

func setRole(pool *pgxpool.Pool, args []string) error {
	role := user.RoleAdmin
	if args[0] == "demote" {
		role = user.RoleUser
	}

	result, err := pool.Exec(context.Background(),
		`UPDATE users SET role = $1 WHERE email = $2`, role, args[1])
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("no user with email %q", args[1])
	}

	fmt.Printf("%s is now %s\n", args[1], role)
	return nil
}
