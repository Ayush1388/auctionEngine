package demobots

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/bidding"
	"github.com/Ayush1388/auctionEngine/internal/user"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

// DemoPassword is shared by every demo account (15 characters or more, as the API requires).
const DemoPassword = "marque-demo-password"

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

// SeedOptions tune Seed.
type SeedOptions struct {
	// Catalog is the path of frontend/app/catalog.json.
	Catalog string
	// Soon makes the Shelby GT500 close in this long, to watch a sale end.
	Soon time.Duration
	// AdminEmail and AdminPassword (DEMO_ADMIN_EMAIL, DEMO_ADMIN_PASSWORD) give the
	// operator a private login. When both are set the public demo operator
	// (admin@marque.test, a known password) is not created, and is demoted if it exists.
	AdminEmail, AdminPassword string
}

// SeedResult says what Seed did.
type SeedResult struct {
	Created  int           `json:"created"`
	Kept     int           `json:"kept"`
	Accounts []SeedAccount `json:"accounts"`
	Password string        `json:"password"`
}

// SeedAccount is a demo login.
type SeedAccount struct {
	Email string `json:"email"`
	Role  string `json:"role"`
	Funds int64  `json:"funds"` // dollars
}

// Seed writes the car catalogue, the demo accounts and a real bid history.
// Every seeded bid goes through the bidding service, so wallets, holds and the
// ledger are exactly what the API would have produced. It is idempotent: a lot
// that is already open is kept.
func (b *Bots) Seed(ctx context.Context, opt SeedOptions) (SeedResult, error) {
	pool := b.o.Pool
	if opt.Catalog == "" {
		opt.Catalog = "frontend/app/catalog.json"
	}
	raw, err := os.ReadFile(opt.Catalog)
	if err != nil {
		return SeedResult{}, err
	}
	var cat catalog
	if err := json.Unmarshal(raw, &cat); err != nil {
		return SeedResult{}, fmt.Errorf("%s: %w", opt.Catalog, err)
	}

	hash, err := user.HashPassword(DemoPassword)
	if err != nil {
		return SeedResult{}, err
	}
	house, err := b.ensureUser(ctx, "house@marque.test", user.RoleUser, 0, hash)
	if err != nil {
		return SeedResult{}, err
	}
	rivals, err := b.ensureRivals(ctx)
	if err != nil {
		return SeedResult{}, err
	}
	res := SeedResult{Password: DemoPassword}
	accounts := demoAccounts
	if opt.AdminEmail != "" || opt.AdminPassword != "" {
		if opt.AdminEmail == "" || len(opt.AdminPassword) < 15 {
			return res, fmt.Errorf("DEMO_ADMIN_EMAIL is required and DEMO_ADMIN_PASSWORD must be at least 15 characters")
		}
		accounts = nil
		for _, a := range demoAccounts {
			if a.role != user.RoleAdmin {
				accounts = append(accounts, a)
			}
		}
		adminHash, err := user.HashPassword(opt.AdminPassword)
		if err != nil {
			return res, err
		}
		if _, err := b.ensureUser(ctx, opt.AdminEmail, user.RoleAdmin, 2_000_000*unit, adminHash); err != nil {
			return res, err
		}
		// the public demo operator must not stay an operator on a public instance
		if _, err := pool.Exec(ctx, `UPDATE users SET role = 'user' WHERE email = 'admin@marque.test' AND role = 'admin'`); err != nil {
			return res, err
		}
		res.Accounts = append(res.Accounts, SeedAccount{opt.AdminEmail, user.RoleAdmin, 2_000_000})
	}
	for _, a := range accounts {
		if _, err := b.ensureUser(ctx, a.email, a.role, a.funds, hash); err != nil {
			return res, err
		}
		res.Accounts = append(res.Accounts, SeedAccount{a.email, a.role, a.funds / unit})
	}

	svc := b.o.Auctions
	bids := bidding.NewService(pool, bidding.Pessimistic)
	for _, l := range cat.Lots {
		var exists bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM auctions a JOIN items i ON i.id = a.item_id
			WHERE a.status IN ('ACTIVE', 'NOT_ACTIVE') AND a.owner_id = $1 AND i.description LIKE $2)`,
			house, `%"key":"`+l.Key+`"%`).Scan(&exists); err != nil {
			return res, err
		}
		if exists {
			res.Kept++
			continue
		}

		ends := time.Duration(l.EndsIn) * time.Second
		if opt.Soon > 0 && l.Key == "gt500" {
			ends = opt.Soon
		}
		now := time.Now().UTC()
		a, err := svc.Create(ctx, house, auction.CreateInput{
			Item:          auction.CreateItemInput{Name: l.Title, Type: "car", Description: l.description()},
			StartingPrice: (l.Bid - int64(l.Bids)*l.Step) * unit,
			MinIncrement:  l.Step * unit,
			StartsAt:      now.Add(time.Minute), EndsAt: now.Add(24 * time.Hour),
		})
		if err != nil {
			return res, fmt.Errorf("create %s: %w", l.Key, err)
		}
		// Open it now (the API only schedules into the future) and set its real close.
		if _, err := pool.Exec(ctx, `UPDATE auctions SET status = 'ACTIVE', starts_at = now() - interval '6 hours',
			ends_at = now() + $2 * interval '1 second' WHERE id = $1`, a.ID, ends.Seconds()); err != nil {
			return res, err
		}
		for k := 0; k < l.Bids; k++ {
			amount := (l.Bid-int64(l.Bids)*l.Step)*unit + int64(k+1)*l.Step*unit
			if _, err := bids.PlaceBid(ctx, bidding.PlaceBidInput{AuctionID: a.ID, UserID: rivals[k%len(rivals)], Amount: amount}); err != nil {
				return res, fmt.Errorf("bid %d on %s: %w", k+1, l.Key, err)
			}
		}
		// Spread the bids over the last five hours so the history reads like one.
		if _, err := pool.Exec(ctx, `
			WITH r AS (SELECT id, row_number() OVER (ORDER BY amount) AS n FROM bids WHERE auction_id = $1)
			UPDATE bids b SET created_at = now() - interval '5 hours' + (r.n::float8 / $2) * interval '4.9 hours' FROM r WHERE b.id = r.id`,
			a.ID, float64(l.Bids)); err != nil {
			return res, err
		}
		res.Created++
	}
	return res, nil
}

// ensureUser creates (or reactivates) a demo user and tops the wallet up to funds.
func (b *Bots) ensureUser(ctx context.Context, email, role string, funds int64, hash string) (uuid.UUID, error) {
	var id uuid.UUID
	err := b.o.Pool.QueryRow(ctx, `
		INSERT INTO users (id, email, password_hash, activated_at, role) VALUES ($1, $2, $3, now(), $4)
		ON CONFLICT (email) DO UPDATE SET activated_at = COALESCE(users.activated_at, now()), role = EXCLUDED.role, password_hash = EXCLUDED.password_hash
		RETURNING id`, uuid.New(), email, hash, role).Scan(&id)
	if err != nil {
		return id, err
	}
	for i := int64(0); funds > 0; i++ { // one deposit is capped at wallet.MaxDeposit
		part := min(funds, wallet.MaxDeposit)
		if _, err := b.o.Wallets.Deposit(ctx, id, part, fmt.Sprintf("demo-seed-%s-%d", id, i)); err != nil {
			return id, fmt.Errorf("fund %s: %w", email, err)
		}
		funds -= part
	}
	return id, nil
}

// ensureRivals makes the six named rival bidders that bid in the seeded history
// and, when the ambient bots are on, keep a quiet site alive.
func (b *Bots) ensureRivals(ctx context.Context) ([]uuid.UUID, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.rivals) > 0 {
		return b.rivals, nil
	}
	hash, err := user.HashPassword(uuid.NewString())
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for _, n := range []int{184, 291, 407, 52, 318, 96} {
		id, err := b.ensureUser(ctx, fmt.Sprintf("bidder-%d@bots.marque.test", n), user.RoleUser, 60_000_000*unit, hash)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	b.rivals = ids
	return ids, nil
}
