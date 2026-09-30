package auction

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ayush1388/auctionEngine/internal/validation"
)

const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

var errInvalidCursor = errors.New("invalid cursor")

// ListFilter selects which auctions to return. Zero values mean "any".
type ListFilter struct {
	Status  Status
	OwnerID uuid.UUID
}

// Cursor marks the last auction on a page. The next page starts strictly
// after it in (created_at DESC, id DESC) order.
type Cursor struct {
	CreatedAt time.Time `json:"c"`
	ID        uuid.UUID `json:"i"`
}

// Encode turns the cursor into an opaque string for clients. Clients should
// pass it back unchanged and never build one themselves.
func (c Cursor) Encode() string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func DecodeCursor(s string) (Cursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, errInvalidCursor
	}

	var c Cursor
	if err := json.Unmarshal(b, &c); err != nil || c.CreatedAt.IsZero() || c.ID == uuid.Nil {
		return Cursor{}, errInvalidCursor
	}
	return c, nil
}

type Page struct {
	Auctions   []Auction
	NextCursor *Cursor
}

// ListQuery is the raw, unvalidated input from a request's query string.
type ListQuery struct {
	Status  string
	OwnerID uuid.UUID
	Limit   string
	Cursor  string
}

// List returns one page of auctions, newest first.
//
// It uses keyset pagination: instead of OFFSET, each page asks for rows
// "after" the last one the client saw. OFFSET has to scan and discard every
// skipped row, and it skips or repeats rows when new auctions are inserted
// between requests. A keyset query goes straight to the right place in the
// index and is stable under concurrent inserts.
func (s *Service) List(
	ctx context.Context,
	q ListQuery,
) (Page, error) {
	problems := &validation.Error{}

	filter := ListFilter{OwnerID: q.OwnerID}
	if q.Status != "" {
		status, err := ParseStatus(strings.ToUpper(q.Status))
		if err != nil {
			problems.Add("status", "must be one of: NOT_ACTIVE, ACTIVE, COMPLETED, CANCELLED")
		}
		filter.Status = status
	}

	limit := DefaultPageSize
	if q.Limit != "" {
		n, err := strconv.Atoi(q.Limit)
		if err != nil || n < 1 || n > MaxPageSize {
			problems.Add("limit", fmt.Sprintf("must be a whole number between 1 and %d", MaxPageSize))
		}
		limit = n
	}

	var after *Cursor
	if q.Cursor != "" {
		c, err := DecodeCursor(q.Cursor)
		if err != nil {
			problems.Add("cursor", "is invalid")
		}
		after = &c
	}

	if err := problems.OrNil(); err != nil {
		return Page{}, err
	}

	return s.repository.List(ctx, filter, after, limit)
}

func (r *Repository) List(
	ctx context.Context,
	filter ListFilter,
	after *Cursor,
	limit int,
) (Page, error) {
	var (
		conditions []string
		args       []any
	)

	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}

	// Conditions are added only when used, so each combination gets a plan
	// that can use the matching index (see migration 000009).
	if filter.Status != "" {
		conditions = append(conditions, "a.status = "+arg(filter.Status))
	}
	if filter.OwnerID != uuid.Nil {
		conditions = append(conditions, "a.owner_id = "+arg(filter.OwnerID))
	}
	if after != nil {
		// Row comparison: (created_at, id) sorts as one composite key, and
		// id breaks ties between auctions created in the same microsecond.
		conditions = append(conditions, fmt.Sprintf(
			"(a.created_at, a.id) < (%s, %s)", arg(after.CreatedAt), arg(after.ID),
		))
	}

	query := selectAuction
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	// Fetch one extra row to learn whether another page exists.
	query += " ORDER BY a.created_at DESC, a.id DESC LIMIT " + arg(limit+1)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return Page{}, fmt.Errorf("failed to list auctions: %w", err)
	}

	auctions, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Auction, error) {
		return scanAuction(row)
	})
	if err != nil {
		return Page{}, fmt.Errorf("failed to list auctions: %w", err)
	}

	page := Page{Auctions: auctions}

	if len(auctions) > limit {
		page.Auctions = auctions[:limit]
		last := page.Auctions[limit-1]
		page.NextCursor = &Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}

	return page, nil
}
