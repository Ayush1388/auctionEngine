// Command admin performs operator tasks that must never be reachable over
// the public API, such as granting the admin role.
//
//	go run ./cmd/admin promote alice@example.com
//	go run ./cmd/admin demote  alice@example.com
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

	"github.com/Ayush1388/auctionEngine/internal/database"
	"github.com/Ayush1388/auctionEngine/internal/user"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "admin:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 2 || (args[0] != "promote" && args[0] != "demote") {
		return errors.New("usage: admin promote|demote <email>")
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
