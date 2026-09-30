package auction

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ayush1388/auctionEngine/internal/database"
)

var ErrNotFound = errors.New("auction not found")

// Repository reads and writes items and auctions. Like every repository in
// this project it never begins transactions; see docs/decisions/0005.
type Repository struct {
	db database.DBTX
}

func NewRepository(db database.DBTX) *Repository {
	return &Repository{
		db: db,
	}
}

func (r *Repository) WithTx(tx pgx.Tx) *Repository {
	return &Repository{
		db: tx,
	}
}

// CreateItem inserts item and fills in its CreatedAt.
func (r *Repository) CreateItem(
	ctx context.Context,
	item *Item,
) error {
	err := r.db.QueryRow(
		ctx,
		`
		INSERT INTO items (
			id,
			owner_id,
			type,
			name,
			description
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at
		`,
		item.ID,
		item.OwnerID,
		item.Type,
		item.Name,
		item.Description,
	).Scan(&item.CreatedAt)
	if err != nil {
		return fmt.Errorf(
			"failed to create item: %w",
			err,
		)
	}

	return nil
}

// Create inserts a and fills in its CreatedAt and UpdatedAt.
// The item must already exist.
func (r *Repository) Create(
	ctx context.Context,
	a *Auction,
) error {
	err := r.db.QueryRow(
		ctx,
		`
		INSERT INTO auctions (
			id,
			item_id,
			owner_id,
			starting_price,
			starts_at,
			ends_at,
			status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, updated_at
		`,
		a.ID,
		a.Item.ID,
		a.OwnerID,
		a.StartingPrice,
		a.StartsAt,
		a.EndsAt,
		a.Status,
	).Scan(&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return fmt.Errorf(
			"failed to create auction: %w",
			err,
		)
	}

	return nil
}

// selectAuction is shared by every query that returns full auctions.
const selectAuction = `
	SELECT
		a.id,
		a.owner_id,
		a.starting_price,
		a.current_bid,
		a.starts_at,
		a.ends_at,
		a.status,
		a.created_at,
		a.updated_at,
		i.id,
		i.owner_id,
		i.type,
		i.name,
		i.description,
		i.created_at
	FROM auctions a
	JOIN items i ON i.id = a.item_id
`

func scanAuction(row pgx.Row) (Auction, error) {
	var a Auction

	err := row.Scan(
		&a.ID,
		&a.OwnerID,
		&a.StartingPrice,
		&a.CurrentBid,
		&a.StartsAt,
		&a.EndsAt,
		&a.Status,
		&a.CreatedAt,
		&a.UpdatedAt,
		&a.Item.ID,
		&a.Item.OwnerID,
		&a.Item.Type,
		&a.Item.Name,
		&a.Item.Description,
		&a.Item.CreatedAt,
	)

	return a, err
}

func (r *Repository) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (Auction, error) {
	a, err := scanAuction(r.db.QueryRow(
		ctx,
		selectAuction+` WHERE a.id = $1`,
		id,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Auction{}, ErrNotFound
		}

		return Auction{}, fmt.Errorf(
			"failed to get auction: %w",
			err,
		)
	}

	return a, nil
}
