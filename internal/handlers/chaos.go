package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/Ayush1388/auctionEngine/internal/chaos"
	"github.com/Ayush1388/auctionEngine/internal/httpx"
)

// ChaosHandler serves the fault switchboard to operators (role "admin").
// GET always answers, so a client can tell whether chaos is available;
// changing a fault needs the API to have started with CHAOS_ENABLED=true.
type ChaosHandler struct{}

func NewChaosHandler() *ChaosHandler { return &ChaosHandler{} }

type chaosState struct {
	Enabled     bool           `json:"enabled"`
	Faults      []chaos.Status `json:"faults"`
	SlowQueryMS int64          `json:"slow_query_ms"`
}

func chaosSnapshot() chaosState {
	return chaosState{Enabled: chaos.Enabled(), Faults: chaos.Snapshot(), SlowQueryMS: chaos.SlowQuery().Milliseconds()}
}

// Get handles GET /v1/admin/chaos.
func (h *ChaosHandler) Get(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, chaosSnapshot())
}

type chaosRequest struct {
	Fault       string `json:"fault"`
	Active      bool   `json:"active"`
	SlowQueryMS *int64 `json:"slow_query_ms"`
}

// Set handles PUT /v1/admin/chaos: switch one fault on or off, and
// optionally set the per-statement delay of the slow-Postgres fault.
func (h *ChaosHandler) Set(w http.ResponseWriter, r *http.Request) {
	var req chaosRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.BadRequest(w, err)
		return
	}
	if !chaos.Enabled() {
		httpx.Error(w, http.StatusForbidden, chaos.ErrDisabled.Error())
		return
	}
	if req.SlowQueryMS != nil {
		chaos.SetSlowQuery(time.Duration(*req.SlowQueryMS) * time.Millisecond)
	}
	if req.Fault != "" {
		f := chaos.Fault(req.Fault)
		if !chaos.Valid(f) {
			httpx.Error(w, http.StatusUnprocessableEntity, "unknown fault")
			return
		}
		if err := chaos.Set(f, req.Active); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, chaos.ErrDisabled) {
				status = http.StatusForbidden
			}
			httpx.Error(w, status, err.Error())
			return
		}
	}
	httpx.WriteJSON(w, http.StatusOK, chaosSnapshot())
}

// Reset handles POST /v1/admin/chaos/reset: every dependency back to normal.
func (h *ChaosHandler) Reset(w http.ResponseWriter, r *http.Request) {
	chaos.Reset()
	httpx.WriteJSON(w, http.StatusOK, chaosSnapshot())
}
