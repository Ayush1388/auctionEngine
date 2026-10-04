package bidding

import (
	"context"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/validation"
)

type HistoryPage struct {
	Bids       []Bid
	NextCursor *auction.Cursor
}

// History returns an auction's bids, newest first, using the same keyset
// pagination as auction listing: (created_at, id) < cursor, limit+1 rows,
// served by the bids_auction_created_idx index.
func (s *Service) History(ctx context.Context, auctionID uuid.UUID, limitParam, cursorParam string) (HistoryPage, error) {
	problems := &validation.Error{}

	limit := auction.DefaultPageSize
	if limitParam != "" {
		n, err := strconv.Atoi(limitParam)
		if err != nil || n < 1 || n > auction.MaxPageSize {
			problems.Add("limit", fmt.Sprintf("must be a whole number between 1 and %d", auction.MaxPageSize))
		}
		limit = n
	}

	var after *auction.Cursor
	if cursorParam != "" {
		c, err := auction.DecodeCursor(cursorParam)
		if err != nil {
			problems.Add("cursor", "is invalid")
		}
		after = &c
	}

	if err := problems.OrNil(); err != nil {
		return HistoryPage{}, err
	}

	// 404 for an auction that doesn't exist, rather than an empty list.
	if _, err := getAuction(ctx, s.db, auctionID, false); err != nil {
		return HistoryPage{}, err
	}

	query := `
		SELECT id, auction_id, user_id, amount, created_at
		FROM bids
		WHERE auction_id = $1`
	args := []any{auctionID}

	if after != nil {
		query += ` AND (created_at, id) < ($2, $3)`
		args = append(args, after.CreatedAt, after.ID)
	}

	args = append(args, limit+1)
	query += ` ORDER BY created_at DESC, id DESC LIMIT $` + strconv.Itoa(len(args))

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return HistoryPage{}, fmt.Errorf("list bids: %w", err)
	}

	bids, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Bid])
	if err != nil {
		return HistoryPage{}, fmt.Errorf("list bids: %w", err)
	}

	page := HistoryPage{Bids: bids}
	if len(bids) > limit {
		page.Bids = bids[:limit]
		last := page.Bids[limit-1]
		page.NextCursor = &auction.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}

	return page, nil
}
