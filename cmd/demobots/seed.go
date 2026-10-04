package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/bidding"
	"github.com/Ayush1388/auctionEngine/internal/user"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

// demoPassword is shared by every demo account (15 characters or more, as the API requires).
const demoPassword = "marque-demo-password"

// unit is cents per dollar: the catalogue is written in dollars, the API in cents.
const unit = 100

type catalog struct {
	Lots []lot `json:"lots"`
}

type lot struct {
	Key      string   `json:"key"`
	Category string   `json:"category"`
	Lot      string   `json:"lot"`
	Title    string   `json:"title"`
	Alt      string   `json:"alt"`
	Img      string   `json:"img"`
	Pos      string   `json:"pos"`
	Year     int      `json:"year"`
	Make     string   `json:"make"`
	Model    string   `json:"model"`
	Type     string   `json:"type"`
	Where    string   `json:"where"`
	Spec     string   `json:"spec"`
	Event    event    `json:"event"`
	Reserve  string   `json:"reserve"`
	Bid      int64    `json:"bid"`
	Step     int64    `json:"step"`
	Bids     int      `json:"bids"`
	EndsIn   int64    `json:"endsIn"`
	Engine   string   `json:"engine"`
	Trans    string   `json:"trans"`
	Power    string   `json:"power"`
	Colour   string   `json:"colour"`
	Interior string   `json:"interior"`
	Miles    string   `json:"miles"`
	Chassis  string   `json:"chassis"`
	Lead     string   `json:"lead"`
	Text     []string `json:"text"`
	History  string   `json:"history"`
	Quote    string   `json:"quote"`
}

type event struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// meta is the block stored at the end of an item's description. The frontend
// (app/model.js) reads it back; the field names must match metaOf() there.
type meta struct {
	Key      string `json:"key"`
	Category string `json:"category"`
	Lot      string `json:"lot"`
	Alt      string `json:"alt"`
	Img      string `json:"img"`
	Pos      string `json:"pos"`
	Year     int    `json:"year"`
	Make     string `json:"make"`
	Model    string `json:"model"`
	Type     string `json:"type"`
	Where    string `json:"where"`
	Spec     string `json:"spec"`
	Event    event  `json:"event"`
	Reserve  string `json:"reserve"`
	Engine   string `json:"engine"`
	Trans    string `json:"trans"`
	Power    string `json:"power"`
	Colour   string `json:"colour"`
	Interior string `json:"interior"`
	Miles    string `json:"miles"`
	Chassis  string `json:"chassis"`
	History  string `json:"history"`
	Quote    string `json:"quote"`
	Lead     string `json:"lead"`
}

func (l lot) description() string {
	m, _ := json.Marshal(meta{l.Key, l.Category, l.Lot, l.Alt, l.Img, l.Pos, l.Year, l.Make, l.Model, l.Type, l.Where, l.Spec, l.Event, l.Reserve,
		l.Engine, l.Trans, l.Power, l.Colour, l.Interior, l.Miles, l.Chassis, l.History, l.Quote, l.Lead})
	prose := strings.Join(append([]string{l.Lead}, l.Text...), "\n\n")
	return prose + "\n\n[[marque:" + string(m) + "]]"
}

type account struct {
	email string
	role  string
	funds int64 // cents
}

var demoAccounts = []account{
	{"demo@marque.test", user.RoleUser, 2_000_000 * unit},
	{"collector@marque.test", user.RoleUser, 2_000_000 * unit},
	{"admin@marque.test", user.RoleAdmin, 2_000_000 * unit},
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
	raw, err := os.ReadFile(*path)
	if err != nil {
		return err
	}
	var cat catalog
	if err := json.Unmarshal(raw, &cat); err != nil {
		return fmt.Errorf("%s: %w", *path, err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	hash, err := user.HashPassword(demoPassword)
	if err != nil {
		return err
	}
	wallets := wallet.NewService(pool)

	ensure := func(email, role string, funds int64) (uuid.UUID, error) {
		var id uuid.UUID
		err := pool.QueryRow(ctx, `
			INSERT INTO users (id, email, password_hash, activated_at, role) VALUES ($1, $2, $3, now(), $4)
			ON CONFLICT (email) DO UPDATE SET activated_at = COALESCE(users.activated_at, now()), role = EXCLUDED.role
			RETURNING id`, uuid.New(), email, hash, role).Scan(&id)
		if err != nil {
			return id, err
		}
		for i := int64(0); funds > 0; i++ { // one deposit is capped at wallet.MaxDeposit
			part := min(funds, wallet.MaxDeposit)
			if _, err := wallets.Deposit(ctx, id, part, fmt.Sprintf("demo-seed-%s-%d", id, i)); err != nil {
				return id, fmt.Errorf("fund %s: %w", email, err)
			}
			funds -= part
		}
		return id, nil
	}

	house, err := ensure("house@marque.test", user.RoleUser, 0)
	if err != nil {
		return err
	}
	var rivals []uuid.UUID
	for _, n := range []int{184, 291, 407, 52, 318, 96} {
		id, err := ensure(fmt.Sprintf("bidder-%d@bots.marque.test", n), user.RoleUser, 60_000_000*unit)
		if err != nil {
			return err
		}
		rivals = append(rivals, id)
	}
	for _, a := range demoAccounts {
		if _, err := ensure(a.email, a.role, a.funds); err != nil {
			return err
		}
	}

	svc := auction.NewService(pool, auction.NewRepository(pool))
	bids := bidding.NewService(pool, bidding.Pessimistic)
	created, kept := 0, 0
	for _, l := range cat.Lots {
		var exists bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM auctions a JOIN items i ON i.id = a.item_id
			WHERE a.status IN ('ACTIVE', 'NOT_ACTIVE') AND a.owner_id = $1 AND i.description LIKE $2)`,
			house, `%"key":"`+l.Key+`"%`).Scan(&exists); err != nil {
			return err
		}
		if exists {
			kept++
			continue
		}

		ends := time.Duration(l.EndsIn) * time.Second
		if *soon > 0 && l.Key == "gt500" {
			ends = *soon
		}
		now := time.Now().UTC()
		a, err := svc.Create(ctx, house, auction.CreateInput{
			Item:          auction.CreateItemInput{Name: l.Title, Type: "car", Description: l.description()},
			StartingPrice: (l.Bid - int64(l.Bids)*l.Step) * unit,
			MinIncrement:  l.Step * unit,
			StartsAt:      now.Add(time.Minute), EndsAt: now.Add(24 * time.Hour),
		})
		if err != nil {
			return fmt.Errorf("create %s: %w", l.Key, err)
		}
		// Open it now (the API only schedules into the future) and set its real close.
		if _, err := pool.Exec(ctx, `UPDATE auctions SET status = 'ACTIVE', starts_at = now() - interval '6 hours',
			ends_at = now() + $2 * interval '1 second' WHERE id = $1`, a.ID, ends.Seconds()); err != nil {
			return err
		}
		// A real history: each bid goes through the real bidding service, so holds and the ledger are right.
		for k := 0; k < l.Bids; k++ {
			amount := (l.Bid-int64(l.Bids)*l.Step)*unit + int64(k+1)*l.Step*unit
			if _, err := bids.PlaceBid(ctx, bidding.PlaceBidInput{AuctionID: a.ID, UserID: rivals[k%len(rivals)], Amount: amount}); err != nil {
				return fmt.Errorf("bid %d on %s: %w", k+1, l.Key, err)
			}
		}
		// Spread the bids over the last five hours so the history reads like one.
		if _, err := pool.Exec(ctx, `
			WITH r AS (SELECT id, row_number() OVER (ORDER BY amount) AS n FROM bids WHERE auction_id = $1)
			UPDATE bids b SET created_at = now() - interval '5 hours' + (r.n::float8 / $2) * interval '4.9 hours' FROM r WHERE b.id = r.id`,
			a.ID, float64(l.Bids)); err != nil {
			return err
		}
		created++
	}

	fmt.Printf("catalogue: %d lots created, %d already open\n\n", created, kept)
	fmt.Println("demo accounts (password: " + demoPassword + ")")
	for _, a := range demoAccounts {
		role := a.role
		fmt.Printf("  %-26s %-6s $%s\n", a.email, role, fmt.Sprint(a.funds/unit))
	}
	fmt.Println("\nnext: go run ./cmd/demobots serve   (the Stress it button)")
	return nil
}
