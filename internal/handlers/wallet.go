package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Ayush1388/auctionEngine/internal/auth"
	"github.com/Ayush1388/auctionEngine/internal/httpx"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

type WalletHandler struct {
	service *wallet.Service
}

func NewWalletHandler(service *wallet.Service) *WalletHandler {
	return &WalletHandler{service: service}
}

type walletResponse struct {
	Available int64 `json:"available"`
	Reserved  int64 `json:"reserved"`
	Total     int64 `json:"total"`
}

func toWalletResponse(w wallet.Wallet) walletResponse {
	return walletResponse{Available: w.Available, Reserved: w.Reserved, Total: w.Total()}
}

// Get handles GET /v1/wallet.
func (h *WalletHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	wal, err := h.service.Get(r.Context(), userID)
	if err != nil {
		httpx.ServerError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toWalletResponse(wal))
}

type depositRequest struct {
	Amount int64 `json:"amount"`
}

// Deposit handles POST /v1/wallet/deposits. The Idempotency-Key header is
// required: a deposit is money, and a retried request must never credit
// twice.
//
// There is no real payment provider; this endpoint stands in for the
// webhook a provider would call after charging a card.
func (h *WalletHandler) Deposit(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	key := r.Header.Get("Idempotency-Key")
	if key == "" || len(key) > 255 {
		httpx.Error(w, http.StatusBadRequest, "Idempotency-Key header is required (max 255 characters)")
		return
	}

	var req depositRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.BadRequest(w, err)
		return
	}

	wal, err := h.service.Deposit(r.Context(), userID, req.Amount, key)
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, toWalletResponse(wal))
	case errors.Is(err, wallet.ErrInvalidAmount):
		httpx.Error(w, http.StatusUnprocessableEntity, "amount must be between 1 and 10000000000")
	default:
		httpx.ServerError(w, r, err)
	}
}

type ledgerResponse struct {
	Entries    []wallet.Entry `json:"entries"`
	NextBefore *int64         `json:"next_before"`
}

// Ledger handles GET /v1/wallet/ledger?limit=&before=: the user's statement,
// every money movement newest first.
func (h *WalletHandler) Ledger(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	limit := 50
	if s := r.URL.Query().Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 200 {
			httpx.Error(w, http.StatusBadRequest, "limit must be between 1 and 200")
			return
		}
		limit = n
	}

	var before int64
	if s := r.URL.Query().Get("before"); s != "" {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || n < 1 {
			httpx.Error(w, http.StatusBadRequest, "before must be a positive entry id")
			return
		}
		before = n
	}

	// Ask for one extra row to know whether another page exists.
	entries, err := h.service.Entries(r.Context(), userID, before, limit+1)
	if err != nil {
		httpx.ServerError(w, r, err)
		return
	}

	resp := ledgerResponse{Entries: entries}
	if len(entries) > limit {
		resp.Entries = entries[:limit]
		next := resp.Entries[limit-1].ID
		resp.NextBefore = &next
	}
	if resp.Entries == nil {
		resp.Entries = []wallet.Entry{}
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}
