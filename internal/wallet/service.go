package wallet

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ayush1388/auctionEngine/internal/database"
)

// MaxDeposit caps one deposit (₹1 crore in paise) as a sanity limit.
const MaxDeposit = 1_00_00_000_00

type Service struct {
	db database.DB
}

func NewService(db database.DB) *Service {
	return &Service{db: db}
}

// Deposit adds money to a user's available balance.
//
// In a real product this would be triggered by a payment provider's webhook
// after the card was charged; here it is a direct endpoint so the rest of
// the system can be exercised. The idempotency key is required: a client
// that retries after a timeout must not be credited twice.
func (s *Service) Deposit(
	ctx context.Context,
	userID uuid.UUID,
	amount int64,
	idempotencyKey string,
) (Wallet, error) {
	if amount <= 0 || amount > MaxDeposit {
		return Wallet{}, ErrInvalidAmount
	}
	if idempotencyKey == "" {
		return Wallet{}, errors.New("idempotency key is required")
	}

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		if err := EnsureWallet(ctx, tx, userID); err != nil {
			return err
		}

		_, err := Post(ctx, tx, Journal{
			Kind: KindDeposit,
			// Namespaced by user so two users can reuse the same key.
			IdempotencyKey: "deposit:" + userID.String() + ":" + idempotencyKey,
			Lines: []Line{
				{Account: External, Amount: -amount},
				{UserID: userID, Account: Available, Amount: amount},
			},
		})
		return err
	})

	// A replay of a deposit that already succeeded is not an error: the
	// client gets the current balance, exactly as for the first request.
	if err != nil && !errors.Is(err, ErrDuplicate) {
		return Wallet{}, fmt.Errorf("deposit: %w", err)
	}

	return Get(ctx, s.db, userID)
}

func (s *Service) Get(ctx context.Context, userID uuid.UUID) (Wallet, error) {
	return Get(ctx, s.db, userID)
}

func (s *Service) Entries(ctx context.Context, userID uuid.UUID, before int64, limit int) ([]Entry, error) {
	return ListEntries(ctx, s.db, userID, before, limit)
}

func (s *Service) Reconcile(ctx context.Context) (Report, error) {
	return Reconcile(ctx, s.db)
}
