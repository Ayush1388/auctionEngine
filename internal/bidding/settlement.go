package bidding

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/database"
	"github.com/Ayush1388/auctionEngine/internal/outbox"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

// Settle pays the seller from the winner's reservation once an auction has
// completed. It is called by the outbox worker for every auction.completed
// event (registered in cmd/api/main.go).
//
// Idempotency is essential here, because the outbox is at-least-once: a
// crash after committing but before the event is marked processed delivers
// the same event again. Settling twice would pay the seller twice. The
// settled_at column, checked under the auction row lock, makes the second
// run a no-op.
func (s *Service) Settle(ctx context.Context, auctionID uuid.UUID) error {
	return database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		// Same lock order as bidding: auction first.
		a, err := getAuction(ctx, tx, auctionID, true)
		if err != nil {
			return err
		}

		if a.SettledAt != nil {
			return nil // already done: a redelivered event
		}

		if a.Status != auction.StatusCompleted {
			return fmt.Errorf("cannot settle auction %s in status %s", a.ID, a.Status)
		}

		var winner *uuid.UUID
		var amount int64

		if a.CurrentBidderID != nil {
			winner = a.CurrentBidderID
			amount = *a.CurrentBid

			// Wallets second, sorted by LockForUpdate.
			if _, err := wallet.LockForUpdate(ctx, tx, *winner, a.OwnerID); err != nil {
				return err
			}

			result, err := tx.Exec(ctx, `
				UPDATE bid_reservations
				SET status = 'SETTLED', closed_at = now()
				WHERE auction_id = $1 AND status = 'ACTIVE' AND user_id = $2 AND amount = $3
			`, a.ID, *winner, amount)
			if err != nil {
				return fmt.Errorf("settle reservation: %w", err)
			}
			if result.RowsAffected() != 1 {
				// The winner's money must be reserved. If it isn't, the books
				// are wrong; refuse rather than move money we can't account for.
				return fmt.Errorf("auction %s: no matching active reservation for winner", a.ID)
			}

			auctionID := a.ID
			if _, err := wallet.Post(ctx, tx, wallet.Journal{
				Kind:      wallet.KindSettle,
				AuctionID: &auctionID,
				// One settlement per auction, ever, even if this code had a bug.
				IdempotencyKey: "settle:" + a.ID.String(),
				Lines: []wallet.Line{
					{UserID: *winner, Account: wallet.Reserved, Amount: -amount},
					{UserID: a.OwnerID, Account: wallet.Available, Amount: amount},
				},
			}); err != nil {
				return err
			}
		}

		var settledAt = s.now().UTC()
		if _, err := tx.Exec(ctx,
			`UPDATE auctions SET settled_at = $2, updated_at = now() WHERE id = $1`,
			a.ID, settledAt,
		); err != nil {
			return fmt.Errorf("mark settled: %w", err)
		}

		return outbox.Enqueue(ctx, tx, EventTypeSettled, SettledEvent{
			AuctionID: a.ID,
			SellerID:  a.OwnerID,
			WinnerID:  winner,
			Amount:    amount,
			SettledAt: settledAt,
		})
	})
}

// SettlementHandler adapts Settle to the outbox.Handler interface.
func (s *Service) SettlementHandler() outbox.Handler {
	return outbox.HandlerFunc(func(ctx context.Context, event outbox.Event) error {
		var payload auction.CompletedEvent
		if err := outbox.DecodePayload(event, &payload); err != nil {
			return err
		}
		if payload.AuctionID == uuid.Nil {
			return errors.New("auction.completed event without auction_id")
		}
		return s.Settle(ctx, payload.AuctionID)
	})
}
