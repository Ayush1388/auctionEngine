package handlers

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/httpx"
	"github.com/Ayush1388/auctionEngine/internal/outbox"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

// AdminHandler serves operator endpoints. Every route is behind
// RequireRole("admin") in server.routes; the handlers themselves don't check.
type AdminHandler struct {
	wallets *wallet.Service
	outbox  *outbox.Repository
}

func NewAdminHandler(wallets *wallet.Service, outboxRepo *outbox.Repository) *AdminHandler {
	return &AdminHandler{wallets: wallets, outbox: outboxRepo}
}

// Reconcile handles GET /v1/admin/reconcile: recompute every balance from
// the ledger and report any disagreement. 200 with "ok": true when the books
// balance, 409 with the details when they don't.
func (h *AdminHandler) Reconcile(w http.ResponseWriter, r *http.Request) {
	report, err := h.wallets.Reconcile(r.Context())
	if err != nil {
		httpx.ServerError(w, r, err)
		return
	}

	status := http.StatusOK
	if !report.OK() {
		status = http.StatusConflict
	}
	httpx.WriteJSON(w, status, map[string]any{
		"ok":                        report.OK(),
		"mismatches":                len(report.Mismatches),
		"unbalanced_journals":       len(report.UnbalancedJournals),
		"money_deposited":           -report.ExternalTotal,
		"money_in_wallets":          report.WalletTotal,
		"reserved_in_wallets":       report.ReservedTotal,
		"active_reservations_total": report.ActiveReservationsTotal,
	})
}

// FailedEvents handles GET /v1/admin/outbox/failed: the dead-letter queue,
// events that failed MaxAttempts times and are no longer retried.
func (h *AdminHandler) FailedEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.outbox.ListFailed(r.Context(), 100)
	if err != nil {
		httpx.ServerError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"events": events})
}

// RetryEvent handles POST /v1/admin/outbox/{id}/retry: put a dead-lettered
// event back in the queue, typically after fixing whatever made it fail.
func (h *AdminHandler) RetryEvent(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid event id")
		return
	}

	err = h.outbox.RetryFailed(r.Context(), id)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, outbox.ErrNotFailed):
		httpx.Error(w, http.StatusNotFound, "no dead-lettered event with that id")
	default:
		httpx.ServerError(w, r, err)
	}
}
