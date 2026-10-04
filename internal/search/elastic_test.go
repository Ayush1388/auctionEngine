package search_test

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/search"
)

// newElastic returns a client on a fresh, uniquely named alias, or skips the
// test when TEST_ELASTICSEARCH_URL is not set (CI sets it).
func newElastic(t *testing.T) (*search.Elastic, string) {
	t.Helper()
	url := os.Getenv("TEST_ELASTICSEARCH_URL")
	if url == "" {
		t.Skip("TEST_ELASTICSEARCH_URL not set; skipping Elasticsearch test")
	}
	alias := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	es := search.NewElastic(url, alias)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := es.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}
	t.Cleanup(func() {
		req, _ := http.NewRequest(http.MethodDelete, strings.TrimRight(url, "/")+"/"+alias+"_*", nil)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
	})
	return es, alias
}

func doc(title, desc, status string, version int64) search.Document {
	return search.Document{
		ID: uuid.New(), Title: title, Description: desc, Type: "electronics", Status: status,
		OwnerID: uuid.New(), StartingPrice: 100, EndsAt: time.Now().UTC(), CreatedAt: time.Now().UTC(),
		Version: version,
	}
}

func TestElasticSearchIsFuzzyAndRanked(t *testing.T) {
	es, _ := newElastic(t)
	ctx := context.Background()

	if err := es.EnsureIndex(ctx); err != nil { // second call is a no-op
		t.Fatalf("EnsureIndex twice: %v", err)
	}

	camera := doc("Vintage camera", "film body", "ACTIVE", 1)
	strap := doc("Leather strap", "fits any camera", "ACTIVE", 1)
	table := doc("Oak table", "solid wood", "ACTIVE", 1)
	for _, d := range []search.Document{camera, strap, table} {
		if err := es.Upsert(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	if err := es.Refresh(ctx); err != nil {
		t.Fatal(err)
	}

	// "camra" is a typo; fuzziness AUTO should still find both.
	page, err := es.Search(ctx, search.Query{Text: "camra", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	got := ids(page)
	if len(got) != 2 || got[0] != camera.ID {
		t.Fatalf("want title match first, got %v", got)
	}
	if !strings.Contains(page.Hits[0].Highlight, "<em>") {
		t.Fatalf("expected highlight, got %q", page.Hits[0].Highlight)
	}
}

func TestElasticRejectsOlderVersions(t *testing.T) {
	es, _ := newElastic(t)
	ctx := context.Background()

	d := doc("Camera", "", "ACTIVE", 5)
	if err := es.Upsert(ctx, d); err != nil {
		t.Fatal(err)
	}

	// A late, out-of-date event arrives with an older snapshot.
	stale := d
	stale.Status = "NOT_ACTIVE"
	stale.Version = 3
	if err := es.Upsert(ctx, stale); err != nil {
		t.Fatalf("an older version must be ignored, not fail: %v", err)
	}
	if err := es.Refresh(ctx); err != nil {
		t.Fatal(err)
	}

	page, err := es.Search(ctx, search.Query{Text: "camera", Status: "ACTIVE", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 1 {
		t.Fatalf("stale write overwrote the newer document")
	}
}

func TestElasticPaginatesWithSearchAfter(t *testing.T) {
	es, _ := newElastic(t)
	ctx := context.Background()
	for i := 0; i < 7; i++ {
		if err := es.Upsert(ctx, doc("Camera lens", "", "ACTIVE", 1)); err != nil {
			t.Fatal(err)
		}
	}
	if err := es.Refresh(ctx); err != nil {
		t.Fatal(err)
	}

	seen := map[uuid.UUID]bool{}
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		page, err := es.Search(ctx, search.Query{Text: "lens", Limit: 3, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range ids(page) {
			if seen[id] {
				t.Fatalf("%s on two pages", id)
			}
			seen[id] = true
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != 7 {
		t.Fatalf("paged through %d documents, want 7", len(seen))
	}
}

func TestElasticSuggestUsesEdgeNgrams(t *testing.T) {
	es, _ := newElastic(t)
	ctx := context.Background()
	for _, d := range []search.Document{
		doc("Apple iPhone 15", "", "ACTIVE", 1),
		doc("iPhone case", "", "COMPLETED", 1), // not active: not suggested
	} {
		if err := es.Upsert(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	if err := es.Refresh(ctx); err != nil {
		t.Fatal(err)
	}

	// "iph" matches the start of the second word, which a plain prefix on
	// the whole title (the PostgreSQL fallback) would miss.
	got, err := es.Suggest(ctx, "iph", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Apple iPhone 15" {
		t.Fatalf("suggestions = %+v", got)
	}
}

func TestReindexSwapsAliasWithoutLosingDocuments(t *testing.T) {
	es, _ := newElastic(t)
	f := newFixture(t)
	ctx := context.Background()

	a := f.create(t, "Vintage camera", "electronics", "", auction.StatusActive)
	b := f.create(t, "Oak table", "furniture", "", auction.StatusActive)

	batches := [][]auction.Auction{{a}, {b}}
	n, err := es.Reindex(ctx, func(context.Context) ([]auction.Auction, error) {
		if len(batches) == 0 {
			return nil, nil
		}
		next := batches[0]
		batches = batches[1:]
		return next, nil
	})
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if n != 2 {
		t.Fatalf("reindexed %d, want 2", n)
	}
	if err := es.Refresh(ctx); err != nil {
		t.Fatal(err)
	}

	page, err := es.Search(ctx, search.Query{Text: "camera", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(page); len(got) != 1 || got[0] != a.ID {
		t.Fatalf("after reindex, search found %v", got)
	}
}
