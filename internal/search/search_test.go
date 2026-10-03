package search_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/search"
	"github.com/Ayush1388/auctionEngine/internal/testdb"
	"github.com/Ayush1388/auctionEngine/internal/validation"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

type fixture struct {
	pool     *pgxpool.Pool
	auctions *auction.Service
	owner    uuid.UUID
	now      time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := testdb.New(t)
	f := &fixture{pool: pool, auctions: auction.NewService(pool, auction.NewRepository(pool)), now: time.Now().UTC()}
	f.auctions.SetClock(func() time.Time { return f.now })

	f.owner = uuid.New()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, activated_at) VALUES ($1, $2, 'x', now())`,
		f.owner, f.owner.String()+"@example.com"); err != nil {
		t.Fatal(err)
	}
	return f
}

// create lists an auction and optionally forces its status.
func (f *fixture) create(t *testing.T, name, typ, desc string, status auction.Status) auction.Auction {
	t.Helper()
	a, err := f.auctions.Create(context.Background(), f.owner, auction.CreateInput{
		Item:          auction.CreateItemInput{Name: name, Type: typ, Description: desc},
		StartingPrice: 1000,
		StartsAt:      f.now.Add(time.Hour),
		EndsAt:        f.now.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if status != auction.StatusNotActive {
		if _, err := f.pool.Exec(context.Background(), `UPDATE auctions SET status = $2 WHERE id = $1`, a.ID, status); err != nil {
			t.Fatal(err)
		}
	}
	got, err := f.auctions.Get(context.Background(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func ids(p search.Page) []uuid.UUID {
	out := make([]uuid.UUID, len(p.Hits))
	for i, h := range p.Hits {
		out[i] = h.ID
	}
	return out
}

func TestPostgresSearchRanksTitleMatchesFirst(t *testing.T) {
	f := newFixture(t)
	inDesc := f.create(t, "Leather bag", "fashion", "Comes with a free camera strap", auction.StatusActive)
	inTitle := f.create(t, "Vintage camera", "electronics", "Film body, works", auction.StatusActive)
	f.create(t, "Oak table", "furniture", "Solid wood", auction.StatusActive)

	svc := search.NewService(nil, search.NewPostgres(f.pool), quiet)
	page, err := svc.Search(context.Background(), search.SearchInput{Text: "camera"})
	if err != nil {
		t.Fatal(err)
	}

	got := ids(page)
	if len(got) != 2 || got[0] != inTitle.ID || got[1] != inDesc.ID {
		t.Fatalf("want title match first then description match, got %v", got)
	}
	if page.Backend != "postgres" {
		t.Fatalf("backend = %q", page.Backend)
	}
	if !strings.Contains(page.Hits[0].Highlight, "<em>") {
		t.Fatalf("expected a highlighted snippet, got %q", page.Hits[0].Highlight)
	}
}

func TestPostgresSearchFiltersAndPaginates(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 5; i++ {
		f.create(t, "Camera lens", "electronics", "", auction.StatusActive)
	}
	f.create(t, "Camera tripod", "electronics", "", auction.StatusCompleted)

	svc := search.NewService(nil, search.NewPostgres(f.pool), quiet)

	seen := map[uuid.UUID]bool{}
	cursor := ""
	for pages := 0; ; pages++ {
		page, err := svc.Search(context.Background(), search.SearchInput{Text: "camera", Status: "active", Limit: "2", Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range ids(page) {
			if seen[id] {
				t.Fatalf("%s returned twice", id)
			}
			seen[id] = true
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
		if pages > 5 {
			t.Fatal("pagination did not end")
		}
	}
	if len(seen) != 5 {
		t.Fatalf("found %d active camera auctions, want 5", len(seen))
	}
}

func TestPostgresSearchUnderstandsWebSyntax(t *testing.T) {
	f := newFixture(t)
	working := f.create(t, "Camera", "electronics", "works perfectly", auction.StatusActive)
	f.create(t, "Camera", "electronics", "broken shutter", auction.StatusActive)

	svc := search.NewService(nil, search.NewPostgres(f.pool), quiet)
	page, err := svc.Search(context.Background(), search.SearchInput{Text: "camera -broken"})
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(page); len(got) != 1 || got[0] != working.ID {
		t.Fatalf("exclusion not applied: %v", got)
	}

	// Input that would be a syntax error for to_tsquery must not be a 500.
	if _, err := svc.Search(context.Background(), search.SearchInput{Text: `"unclosed & | !`}); err != nil {
		t.Fatalf("odd input should not error: %v", err)
	}
}

func TestPostgresSuggest(t *testing.T) {
	f := newFixture(t)
	f.create(t, "iPhone 15", "phones", "", auction.StatusActive)
	f.create(t, "iPad mini", "tablets", "", auction.StatusActive)
	f.create(t, "iPhone 14", "phones", "", auction.StatusCancelled) // not active
	f.create(t, "50% off sofa", "furniture", "", auction.StatusActive)

	svc := search.NewService(nil, search.NewPostgres(f.pool), quiet)

	got, err := svc.Suggest(context.Background(), "IPH", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "iPhone 15" {
		t.Fatalf("suggestions = %+v", got)
	}

	// "%" must be literal, not a wildcard that matches everything.
	got, err = svc.Suggest(context.Background(), "5%", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("LIKE wildcard leaked: %+v", got)
	}
}

func TestSearchValidation(t *testing.T) {
	svc := search.NewService(nil, search.NewPostgres(nil), quiet)

	for _, in := range []search.SearchInput{
		{Text: ""},
		{Text: "x", Limit: "0"},
		{Text: "x", Limit: "51"},
		{Text: "x", Status: "PAUSED"},
	} {
		if _, err := svc.Search(context.Background(), in); !errors.Is(err, validation.ErrInvalid) {
			t.Errorf("%+v: got %v, want validation error", in, err)
		}
	}
	if _, err := svc.Suggest(context.Background(), "a", ""); !errors.Is(err, validation.ErrInvalid) {
		t.Errorf("1-char prefix should be rejected, got %v", err)
	}
}

// failing is a primary backend that is always down.
type failing struct{}

func (failing) Name() string { return "elasticsearch" }
func (failing) Search(context.Context, search.Query) (search.Page, error) {
	return search.Page{}, errors.New("connection refused")
}
func (failing) Suggest(context.Context, string, int) ([]search.Suggestion, error) {
	return nil, errors.New("connection refused")
}

func TestFallsBackToPostgresWhenPrimaryFails(t *testing.T) {
	f := newFixture(t)
	f.create(t, "Vintage camera", "electronics", "", auction.StatusActive)

	svc := search.NewService(failing{}, search.NewPostgres(f.pool), quiet)

	page, err := svc.Search(context.Background(), search.SearchInput{Text: "camera"})
	if err != nil {
		t.Fatalf("search should degrade, not fail: %v", err)
	}
	if page.Backend != "postgres" || len(page.Hits) != 1 {
		t.Fatalf("got backend %q with %d hits", page.Backend, len(page.Hits))
	}

	if s, err := svc.Suggest(context.Background(), "vin", ""); err != nil || len(s) != 1 {
		t.Fatalf("suggest fallback: %+v %v", s, err)
	}
}

// memIndex records what the indexer writes.
type memIndex struct {
	docs    map[uuid.UUID]search.Document
	deleted []uuid.UUID
}

func (m *memIndex) Upsert(_ context.Context, d search.Document) error {
	m.docs[d.ID] = d
	return nil
}
func (m *memIndex) Delete(_ context.Context, id uuid.UUID) error {
	m.deleted = append(m.deleted, id)
	return nil
}

func TestIndexerIndexesCurrentStateAndVersion(t *testing.T) {
	f := newFixture(t)
	a := f.create(t, "Vintage camera", "electronics", "Film", auction.StatusNotActive)

	idx := &memIndex{docs: map[uuid.UUID]search.Document{}}
	ix := search.NewIndexer(idx, f.auctions.Get, quiet)

	if err := ix.Sync(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	first := idx.docs[a.ID]

	// Any update, including ones that don't mention version, must bump it
	// (trigger from migration 000013), or external versioning can't order
	// writes.
	if _, err := f.pool.Exec(context.Background(), `UPDATE auctions SET status = 'ACTIVE' WHERE id = $1`, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := ix.Sync(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	second := idx.docs[a.ID]

	if second.Status != "ACTIVE" || second.Version <= first.Version {
		t.Fatalf("expected newer ACTIVE doc, got %+v after %+v", second, first)
	}
	if second.Title != "Vintage camera" || second.Type != "electronics" {
		t.Fatalf("document not flattened from item: %+v", second)
	}

	// An auction that no longer exists is removed from the index.
	missing := uuid.New()
	if err := ix.Sync(context.Background(), missing); err != nil {
		t.Fatal(err)
	}
	if len(idx.deleted) != 1 || idx.deleted[0] != missing {
		t.Fatalf("deleted = %v", idx.deleted)
	}
}
