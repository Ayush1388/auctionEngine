// Package search lets users find auctions by text ("vintage camera") and get
// type-ahead suggestions ("vin" -> "Vintage camera").
//
// Architecture (v0.6)
//
//	PostgreSQL (source of truth)
//	     │  auction.* / bid.placed events (outbox, same transaction as the change)
//	     ▼
//	Indexer ──► Elasticsearch  ◄── Service.Search ──► fallback: PostgreSQL full-text
//
// Elasticsearch is a *read model*: a copy of the data shaped for search
// (an inverted index, relevance scoring, fuzzy matching, edge n-grams for
// autocomplete). It is never written to directly by request handlers and
// never trusted for anything that matters (bids, money). If it is missing,
// out of date, or down, PostgreSQL still answers, just less cleverly.
//
// The package has two interchangeable backends behind the Backend
// interface: Elastic (elastic.go) and Postgres (postgres.go).
package search

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Document is what we store in the search index for one auction. It is a
// denormalised snapshot: item and auction fields flattened together, because
// a search engine has no joins.
type Document struct {
	ID            uuid.UUID `json:"id"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	Type          string    `json:"type"`
	Status        string    `json:"status"`
	OwnerID       uuid.UUID `json:"owner_id"`
	StartingPrice int64     `json:"starting_price"`
	CurrentBid    *int64    `json:"current_bid"`
	BidCount      int       `json:"bid_count"`
	EndsAt        time.Time `json:"ends_at"`
	CreatedAt     time.Time `json:"created_at"`

	// Version is auctions.version. Elasticsearch stores it as the document
	// version (version_type=external_gte) and rejects writes carrying an
	// older one, so events processed out of order can't roll a document
	// back. It is not part of the JSON body.
	Version int64 `json:"-"`
}

// Query is a full-text search request.
type Query struct {
	Text   string
	Status string // optional exact filter, e.g. "ACTIVE"
	Limit  int
	Cursor string // opaque; from a previous Page.NextCursor
}

// Hit is one search result. Highlight holds a fragment with the matched
// words wrapped in <em>…</em> when the backend supports it.
type Hit struct {
	ID        uuid.UUID
	Highlight string
}

type Page struct {
	Hits       []Hit
	NextCursor string // empty on the last page
	Backend    string // "elasticsearch" or "postgres", useful for clients and debugging
}

type Suggestion struct {
	ID    uuid.UUID `json:"id"`
	Title string    `json:"title"`
}

// Backend is implemented by Elastic and Postgres.
type Backend interface {
	Name() string
	Search(ctx context.Context, q Query) (Page, error)
	Suggest(ctx context.Context, prefix string, limit int) ([]Suggestion, error)
}

// Index is the write side, implemented by Elastic only. (PostgreSQL
// full-text search reads the tables directly; it has nothing to index.)
type Index interface {
	Upsert(ctx context.Context, doc Document) error
	Delete(ctx context.Context, id uuid.UUID) error
}

const (
	DefaultLimit = 20
	MaxLimit     = 50

	// maxResults caps how deep anyone can page. Deep pages of a relevance
	// ranking are useless to humans and expensive for any engine.
	maxResults = 1000
)

var ErrInvalidCursor = errors.New("invalid cursor")

// cursor is the decoded form of Page.NextCursor. Each backend fills the
// field it understands; the backend name guards against using an
// Elasticsearch cursor against the PostgreSQL fallback.
type cursor struct {
	Backend string `json:"b"`
	Offset  int    `json:"o,omitempty"` // PostgreSQL: rows to skip
	After   []any  `json:"a,omitempty"` // Elasticsearch: search_after values
}

func encodeCursor(c cursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s, backend string) (cursor, error) {
	var c cursor
	if s == "" {
		return c, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || json.Unmarshal(b, &c) != nil {
		return cursor{}, ErrInvalidCursor
	}
	if c.Backend != backend {
		// The other backend issued it (we failed over mid-pagination).
		// Restart from the first page rather than return garbage.
		return cursor{}, nil
	}
	return c, nil
}
