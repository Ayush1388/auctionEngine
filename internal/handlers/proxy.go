package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/auth"
	"github.com/Ayush1388/auctionEngine/internal/bidding"
	"github.com/Ayush1388/auctionEngine/internal/httpx"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

// ProxyBidder is the part of the bidding service that handles maximum bids.
type ProxyBidder interface {
	SetProxy(ctx context.Context, auctionID, userID uuid.UUID, maxAmount int64) (bidding.ProxyStatus, error)
	Proxy(ctx context.Context, auctionID, userID uuid.UUID) (bidding.ProxyStatus, error)
	CancelProxy(ctx context.Context, auctionID, userID uuid.UUID) error
}

// ProxyHandler serves the maximum-bid endpoints of one auction.
type ProxyHandler struct{ service ProxyBidder }

func NewProxyHandler(s ProxyBidder) *ProxyHandler { return &ProxyHandler{service: s} }

type proxyRequest struct {
	MaxAmount int64 `json:"max_amount"`
}

type proxyResponse struct {
	MaxAmount  int64     `json:"max_amount"`
	Leading    bool      `json:"leading"`
	CurrentBid int64     `json:"current_bid"`
	BidCount   int       `json:"bid_count"`
	EndsAt     time.Time `json:"ends_at"`
}

func toProxyResponse(s bidding.ProxyStatus) proxyResponse {
	return proxyResponse{MaxAmount: s.MaxAmount, Leading: s.Leading, CurrentBid: s.CurrentBid, BidCount: s.BidCount, EndsAt: s.EndsAt.UTC()}
}

func (h *ProxyHandler) ids(w http.ResponseWriter, r *http.Request) (auctionID, userID uuid.UUID, ok bool) {
	userID, ok = auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	auctionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid auction id")
		return uuid.Nil, uuid.Nil, false
	}
	return auctionID, userID, true
}

func (h *ProxyHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var tooLow *bidding.BidTooLowError
	switch {
	case errors.As(err, &tooLow):
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": tooLow.Error(), "minimum_amount": tooLow.Minimum})
	case errors.Is(err, auction.ErrNotFound), errors.Is(err, bidding.ErrNoProxy):
		httpx.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, bidding.ErrOwnAuction):
		httpx.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, bidding.ErrAuctionNotActive), errors.Is(err, bidding.ErrAuctionEnded):
		httpx.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, wallet.ErrInsufficientFunds), errors.Is(err, bidding.ErrInvalidAmount):
		httpx.Error(w, http.StatusUnprocessableEntity, err.Error())
	default:
		httpx.ServerError(w, r, err)
	}
}

// Set handles PUT /v1/auctions/{id}/proxy-bid. It is idempotent: the same
// maximum twice is the same state.
//
//	200  saved; the response says whether the engine already took the lead
//	409  auction not open
//	422  maximum below the minimum bid, or not enough funds for the minimum
func (h *ProxyHandler) Set(w http.ResponseWriter, r *http.Request) {
	auctionID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	var req proxyRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.BadRequest(w, err)
		return
	}
	st, err := h.service.SetProxy(r.Context(), auctionID, userID, req.MaxAmount)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toProxyResponse(st))
}

// Get handles GET /v1/auctions/{id}/proxy-bid: the caller's own maximum.
func (h *ProxyHandler) Get(w http.ResponseWriter, r *http.Request) {
	auctionID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	st, err := h.service.Proxy(r.Context(), auctionID, userID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toProxyResponse(st))
}

// Delete handles DELETE /v1/auctions/{id}/proxy-bid.
func (h *ProxyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	auctionID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	if err := h.service.CancelProxy(r.Context(), auctionID, userID); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
