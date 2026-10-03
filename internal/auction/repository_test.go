package auction_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/testdb"
)

func insertUser(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()

	id := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, activated_at) VALUES ($1, $2, 'x', now())`,
		id, id.String()+"@example.com",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func count(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()

	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func newDBService(t *testing.T) (*auction.Service, *pgxpool.Pool) {
	t.Helper()

	pool := testdb.New(t)
	service := auction.NewService(pool, auction.NewRepository(pool))
	service.SetClock(func() time.Time { return now })
	return service, pool
}

func TestCreatePersistsItemAndAuction(t *testing.T) {
	service, pool := newDBService(t)
	owner := insertUser(t, pool)

	created, err := service.Create(context.Background(), owner, validInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if created.Status != auction.StatusNotActive {
		t.Errorf("status = %s, want NOT_ACTIVE", created.Status)
	}
	if created.OwnerID != owner || created.Item.OwnerID != owner {
		t.Errorf("owner not set on auction and item")
	}
	if created.CreatedAt.IsZero() || created.Item.CreatedAt.IsZero() {
		t.Errorf("timestamps not filled in from the database")
	}

	stored, err := auction.NewRepository(pool).GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored.Item.Name != "Vintage camera" || stored.StartingPrice != 50_000 || stored.CurrentBid != nil {
		t.Fatalf("unexpected stored auction: %+v", stored)
	}
	if !stored.StartsAt.Equal(created.StartsAt) || !stored.EndsAt.Equal(created.EndsAt) {
		t.Fatalf("times changed on the way through the database")
	}
}

func TestCreateLeavesNoOrphanItemWhenAuctionInsertFails(t *testing.T) {
	service, pool := newDBService(t)
	owner := insertUser(t, pool)

	// Make only the auction insert fail; the item insert before it succeeds.
	if _, err := pool.Exec(context.Background(),
		`ALTER TABLE auctions ADD CONSTRAINT test_reject_all CHECK (false) NOT VALID`,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Create(context.Background(), owner, validInput()); err == nil {
		t.Fatal("expected Create to fail")
	}

	if n := count(t, pool, "items"); n != 0 {
		t.Fatalf("orphan item left behind: items = %d", n)
	}
}

func TestCreateRejectsUnknownOwner(t *testing.T) {
	service, pool := newDBService(t)

	if _, err := service.Create(context.Background(), uuid.New(), validInput()); err == nil {
		t.Fatal("expected a foreign key error for an owner that doesn't exist")
	}
	if n := count(t, pool, "items"); n != 0 {
		t.Fatalf("items = %d, want 0", n)
	}
}

func TestGetByIDNotFound(t *testing.T) {
	pool := testdb.New(t)

	_, err := auction.NewRepository(pool).GetByID(context.Background(), uuid.New())
	if !errors.Is(err, auction.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}
