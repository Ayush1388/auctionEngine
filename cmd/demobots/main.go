// Command demobots loads the demo data from the command line.
//
//	go run ./cmd/demobots seed     # the car catalogue, demo accounts and a real bid history
//
// The bots themselves (the rival bidders and the "Stress it" button) are no
// longer a separate program: they are built into the API and switched on and
// off from the website. Start the API with DEMO_BOTS_ENABLED=true, and add
// DEMO_SEED=true to load this data at start-up as well.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/demobots"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

func main() {
	_ = godotenv.Load()
	if len(os.Args) < 2 || os.Args[1] != "seed" {
		fmt.Fprintln(os.Stderr, `usage:
  demobots seed [-catalog frontend/app/catalog.json] [-soon 4m]

The rival bidders and Stress it are built into the API: start it with DEMO_BOTS_ENABLED=true.`)
		os.Exit(2)
	}
	if err := runSeed(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "demobots:", err)
		os.Exit(1)
	}
}

func runSeed(args []string) error {
	fs := flag.NewFlagSet("seed", flag.ExitOnError)
	path := fs.String("catalog", "frontend/app/catalog.json", "the car catalogue")
	soon := fs.Duration("soon", 0, "make the Shelby GT500 close in this long, to watch a sale end (e.g. 4m)")
	_ = fs.Parse(args)

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	bots := demobots.New(demobots.Options{Pool: pool, Auctions: auction.NewService(pool, auction.NewRepository(pool)), Wallets: wallet.NewService(pool)})
	res, err := bots.Seed(ctx, demobots.SeedOptions{Catalog: *path, Soon: *soon, AdminEmail: os.Getenv("DEMO_ADMIN_EMAIL"), AdminPassword: os.Getenv("DEMO_ADMIN_PASSWORD")})
	if err != nil {
		return err
	}
	fmt.Printf("catalogue: %d lots created, %d already open\n\n", res.Created, res.Kept)
	fmt.Println("demo accounts (password: " + res.Password + ")")
	for _, a := range res.Accounts {
		fmt.Printf("  %-26s %-6s $%d\n", a.Email, a.Role, a.Funds)
	}
	fmt.Println("\nnext: start the API with DEMO_BOTS_ENABLED=true and switch the bots on from the website")
	return nil
}
