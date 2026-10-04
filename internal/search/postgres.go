package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ayush1388/auctionEngine/internal/database"
)

// Postgres is the fallback backend: PostgreSQL's built-in full-text search.
//
// items.search_vector (migration 000013) is a generated tsvector column:
// PostgreSQL keeps it up to date on every insert/update, so there is no
// indexing step and no way for it to be stale. The GIN index on it is an
// inverted index, the same core data structure as Elasticsearch.
//
// What it lacks compared with Elasticsearch: typo tolerance (no fuzziness),
// edge n-gram autocomplete, highlighting quality, and it competes with
// bids for the primary database's CPU. Good enough to keep search working
// when Elasticsearch is down.
type Postgres struct {
	db database.DBTX
}

func NewPostgres(db database.DBTX) *Postgres {
	return &Postgres{db: db}
}

func (p *Postgres) Name() string { return "postgres" }

// Search ranks matches with ts_rank_cd (cover density: rewards query words
// that appear close together) and paginates by offset, capped at
// maxResults. Relevance order can't use keyset pagination cheaply here,
// because the rank is computed per query, not stored in an index.
func (p *Postgres) Search(ctx context.Context, q Query) (Page, error) {
	c, err := decodeCursor(q.Cursor, p.Name())
	if err != nil {
		return Page{}, err
	}
	if c.Offset >= maxResults {
		return Page{Backend: p.Name()}, nil
	}

	// websearch_to_tsquery understands what users type: quoted phrases,
	// "or", and -exclusions, and never raises a syntax error on odd input.
	args := []any{q.Text, q.Limit + 1, c.Offset}
	where := "i.search_vector @@ websearch_to_tsquery('english', $1)"
	if q.Status != "" {
		args = append(args, q.Status)
		where += fmt.Sprintf(" AND a.status = $%d", len(args))
	}

	rows, err := p.db.Query(ctx, `
		SELECT
			a.id,
			ts_headline('english', i.name || ' — ' || i.description,
				websearch_to_tsquery('english', $1),
				'StartSel=<em>, StopSel=</em>, MaxFragments=1, MaxWords=18, MinWords=5')
		FROM auctions a
		JOIN items i ON i.id = a.item_id
		WHERE `+where+`
		ORDER BY ts_rank_cd(i.search_vector, websearch_to_tsquery('english', $1)) DESC,
		         a.created_at DESC, a.id
		LIMIT $2 OFFSET $3
	`, args...)
	if err != nil {
		return Page{}, fmt.Errorf("postgres search: %w", err)
	}

	hits, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Hit, error) {
		var h Hit
		err := row.Scan(&h.ID, &h.Highlight)
		return h, err
	})
	if err != nil {
		return Page{}, fmt.Errorf("postgres search: %w", err)
	}

	page := Page{Backend: p.Name(), Hits: hits}
	if len(hits) > q.Limit {
		page.Hits = hits[:q.Limit]
		page.NextCursor = encodeCursor(cursor{Backend: p.Name(), Offset: c.Offset + q.Limit})
	}
	return page, nil
}

// Suggest matches titles starting with prefix (case-insensitive), using the
// lower(name) text_pattern_ops index. Unlike Elasticsearch it only matches
// the start of the title, not the start of any word.
func (p *Postgres) Suggest(ctx context.Context, prefix string, limit int) ([]Suggestion, error) {
	rows, err := p.db.Query(ctx, `
		SELECT a.id, i.name
		FROM items i
		JOIN auctions a ON a.item_id = i.id
		WHERE lower(i.name) LIKE $1 AND a.status = 'ACTIVE'
		ORDER BY a.bid_count DESC, a.created_at DESC
		LIMIT $2
	`, escapeLike(strings.ToLower(prefix))+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("postgres suggest: %w", err)
	}

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Suggestion, error) {
		var s Suggestion
		var id uuid.UUID
		err := row.Scan(&id, &s.Title)
		s.ID = id
		return s, err
	})
}

// escapeLike stops user input such as "50%" from acting as a wildcard.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
