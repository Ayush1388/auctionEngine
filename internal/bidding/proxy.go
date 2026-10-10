package bidding

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/database"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

// Proxy (maximum) bidding.
//
// A bidder gives a maximum; whenever another bid would take the lead, the
// engine answers on their behalf with the smallest bid that keeps them
// ahead, up to the maximum. Everything happens inside the transaction that
// holds the auction row lock, so concurrent bids and instructions are
// serialised exactly like manual bids: the final price and the winner do not
// depend on timing (see nextAutoBid for the rule).
//
// Money: only the visible bid is held. The maximum is an instruction, not a
// reservation, so a bidder's funds are never locked up by a ceiling they may
// not reach. If their balance has fallen below the maximum when the engine
// needs it, the effective maximum is the balance.

// maxProxies bounds how many instructions one resolution loads.
const maxProxies = 50

// ProxyStatus is a bidder's instruction on one auction and where it stands.
type ProxyStatus struct {
	MaxAmount  int64
	Leading    bool
	CurrentBid int64
	BidCount   int
	EndsAt     time.Time
}

// ErrNoProxy means the user has no instruction on that auction.
var ErrNoProxy = errors.New("no maximum bid set on this auction")

// loadProxies reads the instructions on an auction, highest first.
func loadProxies(ctx context.Context, db database.DBTX, auctionID uuid.UUID) ([]ProxyEntry, error) {
	rows, err := db.Query(ctx, `
		SELECT user_id, max_amount, updated_at
		FROM proxy_bids
		WHERE auction_id = $1
		ORDER BY max_amount DESC, updated_at ASC
		LIMIT $2
	`, auctionID, maxProxies)
	if err != nil {
		return nil, fmt.Errorf("load proxy bids: %w", err)
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (ProxyEntry, error) {
		var p ProxyEntry
		err := r.Scan(&p.UserID, &p.Max, &p.Since)
		return p, err
	})
}

// resolveProxies places the automatic bids that follow the current state of
// the auction, inside tx. The caller holds the auction row lock. It returns
// the number of automatic bids placed.
func (s *Service) resolveProxies(ctx context.Context, tx pgx.Tx, auctionID uuid.UUID) (int, error) {
	placed := 0
	// One step per distinct instruction is enough to settle a contest.
	for step := 0; step <= maxProxies; step++ {
		a, err := getAuction(ctx, tx, auctionID, false)
		if err != nil {
			return placed, err
		}
		proxies, err := loadProxies(ctx, tx, auctionID)
		if err != nil || len(proxies) == 0 {
			return placed, err
		}

		// Lock every involved wallet at once, in user-ID order (LockForUpdate
		// sorts), then read the balances the decision must use.
		ids := make([]uuid.UUID, 0, len(proxies)+1)
		for _, p := range proxies {
			ids = append(ids, p.UserID)
		}
		if a.CurrentBidderID != nil {
			ids = append(ids, *a.CurrentBidderID)
		}
		wallets, err := wallet.LockForUpdate(ctx, tx, ids...)
		if err != nil {
			return placed, err
		}
		funds := func(u uuid.UUID) int64 {
			f := wallets[u].Available
			if a.CurrentBidderID != nil && *a.CurrentBidderID == u && a.CurrentBid != nil {
				f += *a.CurrentBid // what they already hold here
			}
			return f
		}

		next, ok := nextAutoBid(a, proxies, funds)
		if !ok {
			return placed, nil
		}
		now := s.now()
		in := PlaceBidInput{AuctionID: auctionID, UserID: next.UserID, Amount: next.Amount, auto: true}
		p, err := decide(a, in.UserID, in.Amount, now)
		if err != nil {
			// The rules refuse it (for example the auction just ended): stop
			// quietly, the bid that led here stands.
			return placed, nil
		}
		if _, err := apply(ctx, tx, a, in, p, now, false); err != nil {
			return placed, err
		}
		placed++
	}
	return placed, nil
}

// SetProxy records (or raises, or lowers) the user's maximum on an auction
// and lets the engine act on it at once: if it takes the lead, the bid is
// placed in the same transaction.
func (s *Service) SetProxy(ctx context.Context, auctionID, userID uuid.UUID, maxAmount int64) (ProxyStatus, error) {
	if maxAmount <= 0 {
		return ProxyStatus{}, ErrInvalidAmount
	}
	var out ProxyStatus
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		a, err := getAuction(ctx, tx, auctionID, true)
		if err != nil {
			return err
		}
		now := s.now()
		switch {
		case a.Status == auction.StatusCompleted, a.Status == auction.StatusActive && !now.Before(a.EndsAt):
			return ErrAuctionEnded
		case a.Status != auction.StatusActive, now.Before(a.StartsAt):
			return ErrAuctionNotActive
		case a.OwnerID == userID:
			return ErrOwnAuction
		}

		leading := a.CurrentBidderID != nil && *a.CurrentBidderID == userID
		floor := MinimumBid(a)
		if leading {
			floor = *a.CurrentBid + 1 // a ceiling at or under what they already hold does nothing
		}
		if maxAmount < floor {
			return &BidTooLowError{Minimum: floor}
		}

		// the new instruction joins the participants: lock them all at once, in order
		if err := lockParticipants(ctx, tx, a, userID); err != nil {
			return err
		}
		w, err := wallet.LockForUpdate(ctx, tx, userID)
		if err != nil {
			return err
		}
		if !leading && w[userID].Available < MinimumBid(a) {
			return wallet.ErrInsufficientFunds
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO proxy_bids (auction_id, user_id, max_amount)
			VALUES ($1, $2, $3)
			ON CONFLICT (auction_id, user_id)
			DO UPDATE SET max_amount = EXCLUDED.max_amount, updated_at = now()
		`, auctionID, userID, maxAmount); err != nil {
			return fmt.Errorf("save proxy bid: %w", err)
		}

		if _, err := s.resolveProxies(ctx, tx, auctionID); err != nil {
			return err
		}
		out, err = proxyStatus(ctx, tx, auctionID, userID)
		return err
	})
	return out, err
}

// Proxy returns the user's instruction on an auction.
func (s *Service) Proxy(ctx context.Context, auctionID, userID uuid.UUID) (ProxyStatus, error) {
	return proxyStatus(ctx, s.db, auctionID, userID)
}

func proxyStatus(ctx context.Context, db database.DBTX, auctionID, userID uuid.UUID) (ProxyStatus, error) {
	a, err := getAuction(ctx, db, auctionID, false)
	if err != nil {
		return ProxyStatus{}, err
	}
	var st ProxyStatus
	err = db.QueryRow(ctx, `SELECT max_amount FROM proxy_bids WHERE auction_id = $1 AND user_id = $2`, auctionID, userID).Scan(&st.MaxAmount)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProxyStatus{}, ErrNoProxy
	}
	if err != nil {
		return ProxyStatus{}, fmt.Errorf("read proxy bid: %w", err)
	}
	st.Leading = a.CurrentBidderID != nil && *a.CurrentBidderID == userID
	if a.CurrentBid != nil {
		st.CurrentBid = *a.CurrentBid
	}
	st.BidCount, st.EndsAt = a.BidCount, a.EndsAt
	return st, nil
}

// CancelProxy removes the user's instruction. Their standing bid, if they
// are leading, is unchanged.
func (s *Service) CancelProxy(ctx context.Context, auctionID, userID uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM proxy_bids WHERE auction_id = $1 AND user_id = $2`, auctionID, userID)
	if err != nil {
		return fmt.Errorf("cancel proxy bid: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoProxy
	}
	return nil
}

// answerWithProxies lets standing instructions answer a bid that was just
// accepted, and updates res to describe the auction afterwards.
func (s *Service) answerWithProxies(ctx context.Context, tx pgx.Tx, res *Result, bidder uuid.UUID) error {
	n, err := s.resolveProxies(ctx, tx, res.Bid.AuctionID)
	if err != nil || n == 0 {
		return err
	}
	a, err := getAuction(ctx, tx, res.Bid.AuctionID, false)
	if err != nil {
		return err
	}
	if a.CurrentBid != nil {
		res.CurrentBid = *a.CurrentBid
	}
	res.BidCount, res.EndsAt = a.BidCount, a.EndsAt
	res.Countered = a.CurrentBidderID == nil || *a.CurrentBidderID != bidder
	return nil
}

// lockParticipants takes, in ONE statement and so in user-ID order, every
// wallet the proxy resolution of this auction can touch: the bidder, the
// current leader and every instruction owner. It must run right after the
// auction row is locked and before any wallet is, because two transactions
// on different auctions that took overlapping wallets in different orders
// could otherwise wait on each other (Postgres would abort one with 40P01).
// With no instructions on the auction nothing extra is locked and the bid
// takes its usual two wallets.
func lockParticipants(ctx context.Context, tx pgx.Tx, a auction.Auction, bidder uuid.UUID) error {
	proxies, err := loadProxies(ctx, tx, a.ID)
	if err != nil || len(proxies) == 0 {
		return err
	}
	ids := make([]uuid.UUID, 0, len(proxies)+2)
	ids = append(ids, bidder)
	if a.CurrentBidderID != nil {
		ids = append(ids, *a.CurrentBidderID)
	}
	for _, p := range proxies {
		ids = append(ids, p.UserID)
	}
	_, err = wallet.LockForUpdate(ctx, tx, ids...)
	return err
}

// hasProxies reports whether anyone has a maximum on the auction.
func hasProxies(ctx context.Context, db database.DBTX, auctionID uuid.UUID) (bool, error) {
	var ok bool
	err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM proxy_bids WHERE auction_id = $1)`, auctionID).Scan(&ok)
	return ok, err
}
