package auction

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ayush1388/auctionEngine/internal/database"
	"github.com/Ayush1388/auctionEngine/internal/validation"
)

const (
	MinDuration = time.Minute
	MaxDuration = 30 * 24 * time.Hour

	// maxStartDelay stops auctions being scheduled years ahead.
	maxStartDelay = 90 * 24 * time.Hour
)

type CreateItemInput struct {
	Name        string `json:"name" validate:"required,max=200"`
	Type        string `json:"type" validate:"required,max=50"`
	Description string `json:"description" validate:"max=5000"`
}

// CreateInput is the body of POST /v1/auctions. Prices are in the smallest
// currency unit (paise, cents), so they are always whole numbers.
type CreateInput struct {
	Item          CreateItemInput `json:"item"`
	StartingPrice int64           `json:"starting_price" validate:"gte=0"`
	StartsAt      time.Time       `json:"starts_at" validate:"required"`
	EndsAt        time.Time       `json:"ends_at" validate:"required"`
}

type Service struct {
	db         database.TxBeginner
	repository *Repository
	validator  *validation.Validator
	now        func() time.Time
}

func NewService(
	db database.TxBeginner,
	repository *Repository,
) *Service {
	return &Service{
		db:         db,
		repository: repository,
		validator:  validation.New(),
		now:        time.Now,
	}
}

// SetClock replaces the service's clock. Tests use it to control "now".
func (s *Service) SetClock(now func() time.Time) {
	s.now = now
}

// Create lists a new item for auction on behalf of ownerID. The item and the
// auction are written in one transaction, so a failure never leaves an item
// without its auction.
func (s *Service) Create(
	ctx context.Context,
	ownerID uuid.UUID,
	input CreateInput,
) (Auction, error) {
	input.Item.Name = strings.TrimSpace(input.Item.Name)
	input.Item.Type = strings.TrimSpace(input.Item.Type)
	input.Item.Description = strings.TrimSpace(input.Item.Description)

	if err := s.validate(input); err != nil {
		return Auction{}, err
	}

	a := Auction{
		ID:            uuid.New(),
		OwnerID:       ownerID,
		StartingPrice: input.StartingPrice,
		StartsAt:      input.StartsAt.UTC(),
		EndsAt:        input.EndsAt.UTC(),
		Status:        StatusNotActive,
		Item: Item{
			ID:          uuid.New(),
			OwnerID:     ownerID,
			Name:        input.Item.Name,
			Type:        input.Item.Type,
			Description: input.Item.Description,
		},
	}

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		repo := s.repository.WithTx(tx)

		if err := repo.CreateItem(ctx, &a.Item); err != nil {
			return err
		}

		return repo.Create(ctx, &a)
	})
	if err != nil {
		return Auction{}, err
	}

	return a, nil
}

func (s *Service) validate(input CreateInput) error {
	// Start with the struct-tag checks, then add the rules that involve
	// the clock or more than one field.
	problems := &validation.Error{}
	if err := s.validator.Struct(input); err != nil && !errors.As(err, &problems) {
		return err
	}

	now := s.now()

	if !input.StartsAt.IsZero() {
		switch {
		case input.StartsAt.Before(now):
			problems.Add("starts_at", "must be in the future")
		case input.StartsAt.After(now.Add(maxStartDelay)):
			problems.Add("starts_at", "must be within 90 days")
		}
	}

	if !input.StartsAt.IsZero() && !input.EndsAt.IsZero() {
		duration := input.EndsAt.Sub(input.StartsAt)

		switch {
		case duration <= 0:
			problems.Add("ends_at", "must be after starts_at")
		case duration < MinDuration:
			problems.Add("ends_at", "must be at least 1 minute after starts_at")
		case duration > MaxDuration:
			problems.Add("ends_at", "must be at most 30 days after starts_at")
		}
	}

	return problems.OrNil()
}

func (s *Service) Get(
	ctx context.Context,
	id uuid.UUID,
) (Auction, error) {
	return s.repository.GetByID(ctx, id)
}
