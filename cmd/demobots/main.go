// Command demobots is the demo companion of the frontend (v1.1).
//
//	go run ./cmd/demobots seed     # the car catalogue, demo accounts and a real bid history
//	go run ./cmd/demobots serve    # the "Stress it" server on :4100
//
// seed writes straight into PostgreSQL (DATABASE_URL), the way cmd/loadgen does,
// but through the real services: every seeded bid goes through bidding.PlaceBid,
// so wallets, reservations and the ledger are exactly what the API would have
// produced. The catalogue is frontend/app/catalog.json, the same file the
// frontend's built-in demo engine reads.
//
// serve starts a small HTTP server for the lot page's "Stress it" button. It
// fires many bidders at one auction through the real HTTP API (no shortcuts),
// streams progress as server-sent events, and checks the result when it is done.
// Start the API with RATE_LIMITS=off first: every bot comes from one IP.
package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "seed":
		err = runSeed(os.Args[2:])
	case "serve":
		err = runServe(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "demobots:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  demobots seed  [-catalog frontend/app/catalog.json] [-soon 4m]
  demobots serve [-addr :4100] [-api http://localhost:4000]`)
}
