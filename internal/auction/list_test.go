package auction_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/validation"
)

// seed inserts n auctions directly. Every third one shares its created_at
// with the previous auction, so the id tie-breaker gets exercised.
func seed(t *testing.T, pool *pgxpool.Pool, owner uuid.UUID, n int, status auction.Status) {
	t.Helper()
	seedAt(t, pool, owner, n, status, now.Add(-time.Duration(n)*time.Minute))
}

func seedAt(t *testing.T, pool *pgxpool.Pool, owner uuid.UUID, n int, status auction.Status, base time.Time) {
	t.Helper()
	ctx := context.Background()

	for i := 0; i < n; i++ {
		created := base.Add(time.Duration(i) * time.Minute)
		if i%3 == 2 {
			created = base.Add(time.Duration(i-1) * time.Minute)
		}

		itemID := uuid.New()
		if _, err := pool.Exec(ctx,
			`INSERT INTO items (id, owner_id, type, name, description) VALUES ($1, $2, 't', 'n', '')`,
			itemID, owner,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO auctions (id, item_id, owner_id, starting_price, starts_at, ends_at, status, created_at)
			VALUES ($1, $2, $3, 100, $4, $5, $6, $7)`,
			uuid.New(), itemID, owner, now, now.Add(time.Hour), status, created,
		); err != nil {
			t.Fatal(err)
		}
	}
}

// collect pages through everything with the given page size.
func collect(t *testing.T, service *auction.Service, q auction.ListQuery) ([]auction.Auction, int) {
	t.Helper()

	var all []auction.Auction
	pages := 0

	for {
		page, err := service.List(context.Background(), q)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		pages++
		all = append(all, page.Auctions...)

		if page.NextCursor == nil {
			return all, pages
		}
		q.Cursor = page.NextCursor.Encode()

		if pages > 100 {
			t.Fatal("pagination did not terminate")
		}
	}
}

func TestListPagesThroughEveryAuctionExactlyOnce(t *testing.T) {
	service, pool := newDBService(t)
	owner := insertUser(t, pool)
	seed(t, pool, owner, 50, auction.StatusActive)

	all, pages := collect(t, service, auction.ListQuery{Limit: "7"})

	if len(all) != 50 {
		t.Fatalf("got %d auctions, want 50", len(all))
	}
	if pages != 8 { // 7 full pages of 7, then 1
		t.Fatalf("got %d pages, want 8", pages)
	}

	seen := map[uuid.UUID]bool{}
	for i, a := range all {
		if seen[a.ID] {
			t.Fatalf("auction %s returned twice", a.ID)
		}
		seen[a.ID] = true

		if i == 0 {
			continue
		}
		prev := all[i-1]
		inOrder := prev.CreatedAt.After(a.CreatedAt) ||
			(prev.CreatedAt.Equal(a.CreatedAt) && prev.ID.String() > a.ID.String())
		if !inOrder {
			t.Fatalf("auctions %d and %d are out of order", i-1, i)
		}
	}
}

func TestListIsStableWhenAuctionsAreInsertedBetweenPages(t *testing.T) {
	service, pool := newDBService(t)
	owner := insertUser(t, pool)
	seed(t, pool, owner, 10, auction.StatusActive)

	first, err := service.List(context.Background(), auction.ListQuery{Limit: "5"})
	if err != nil {
		t.Fatal(err)
	}

	// New auctions arrive at the top of the list while the client pages.
	// With OFFSET, page two would repeat items from page one.
	seedAt(t, pool, owner, 3, auction.StatusActive, now.Add(time.Hour))

	second, err := service.List(context.Background(), auction.ListQuery{Limit: "5", Cursor: first.NextCursor.Encode()})
	if err != nil {
		t.Fatal(err)
	}

	for _, a := range second.Auctions {
		for _, b := range first.Auctions {
			if a.ID == b.ID {
				t.Fatalf("auction %s appeared on both pages", a.ID)
			}
		}
	}
}

func TestListFilters(t *testing.T) {
	service, pool := newDBService(t)
	alice := insertUser(t, pool)
	bob := insertUser(t, pool)
	seed(t, pool, alice, 4, auction.StatusActive)
	seed(t, pool, alice, 2, auction.StatusCompleted)
	seed(t, pool, bob, 3, auction.StatusActive)

	active, _ := collect(t, service, auction.ListQuery{Status: "ACTIVE"})
	if len(active) != 7 {
		t.Fatalf("ACTIVE: got %d, want 7", len(active))
	}

	mine, _ := collect(t, service, auction.ListQuery{OwnerID: alice})
	if len(mine) != 6 {
		t.Fatalf("owner=alice: got %d, want 6", len(mine))
	}

	both, _ := collect(t, service, auction.ListQuery{OwnerID: alice, Status: "completed"})
	if len(both) != 2 {
		t.Fatalf("alice + COMPLETED: got %d, want 2", len(both))
	}
}

func TestListRejectsBadParameters(t *testing.T) {
	service := auction.NewService(nil, nil)

	tests := []struct {
		name  string
		q     auction.ListQuery
		field string
	}{
		{"unknown status", auction.ListQuery{Status: "PAUSED"}, "status"},
		{"limit zero", auction.ListQuery{Limit: "0"}, "limit"},
		{"limit too big", auction.ListQuery{Limit: "101"}, "limit"},
		{"limit not a number", auction.ListQuery{Limit: "ten"}, "limit"},
		{"garbage cursor", auction.ListQuery{Cursor: "!!!"}, "cursor"},
		{"cursor missing fields", auction.ListQuery{Cursor: "e30"}, "cursor"}, // base64 of {}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.List(context.Background(), tt.q)

			var problems *validation.Error
			if !errors.As(err, &problems) || problems.Fields[tt.field] == "" {
				t.Fatalf("got %v, want a validation error on %s", err, tt.field)
			}
		})
	}
}

func TestCursorRoundTrip(t *testing.T) {
	c := auction.Cursor{CreatedAt: now.Add(123456 * time.Microsecond), ID: uuid.New()}

	got, err := auction.DecodeCursor(c.Encode())
	if err != nil {
		t.Fatal(err)
	}
	if !got.CreatedAt.Equal(c.CreatedAt) || got.ID != c.ID {
		t.Fatalf("got %+v, want %+v", got, c)
	}
}
