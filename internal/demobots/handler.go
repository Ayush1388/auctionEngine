package demobots

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/httpx"
)

// Handler serves the website's controls for the bots.
//
//	GET  /v1/demo/bots                 state (public: the site draws the switch from it)
//	PUT  /v1/demo/bots                 {"ambient": true|false}   (signed in)
//	POST /v1/demo/stress               start a run               (signed in)
//	GET  /v1/demo/stress/{id}/events   server-sent progress      (signed in)
//	POST /v1/demo/seed                 load the catalogue        (operator)
//
// With the bots disabled (the default) every route but GET answers 404.
type Handler struct {
	bots    *Bots // nil when disabled
	catalog string
}

func NewHandler(b *Bots, catalog string) *Handler { return &Handler{bots: b, catalog: catalog} }

type stateResponse struct {
	Enabled bool         `json:"enabled"`
	Ambient AmbientStats `json:"ambient"`
	Message string       `json:"message,omitempty"`
}

func (h *Handler) State(w http.ResponseWriter, r *http.Request) {
	if h.bots == nil {
		httpx.WriteJSON(w, http.StatusOK, stateResponse{Message: "Start the API with DEMO_BOTS_ENABLED=true to switch the demo bots on from the website."})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, stateResponse{Enabled: true, Ambient: h.bots.Ambient()})
}

func (h *Handler) off(w http.ResponseWriter) bool {
	if h.bots != nil {
		return false
	}
	httpx.Error(w, http.StatusNotFound, "demo bots are not enabled on this API")
	return true
}

type setRequest struct {
	Ambient *bool `json:"ambient"`
}

func (h *Handler) Set(w http.ResponseWriter, r *http.Request) {
	if h.off(w) {
		return
	}
	var req setRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.BadRequest(w, err)
		return
	}
	if req.Ambient != nil {
		if err := h.bots.SetAmbient(r.Context(), *req.Ambient); err != nil {
			httpx.ServerError(w, r, err)
			return
		}
	}
	httpx.WriteJSON(w, http.StatusOK, stateResponse{Enabled: true, Ambient: h.bots.Ambient()})
}

type stressRequest struct {
	AuctionID string `json:"auction_id"`
	Bidders   int    `json:"bidders"`
	Rounds    int    `json:"rounds"`
}

func (h *Handler) StartStress(w http.ResponseWriter, r *http.Request) {
	if h.off(w) {
		return
	}
	var req stressRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.BadRequest(w, err)
		return
	}
	id, err := uuid.Parse(req.AuctionID)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid auction_id")
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"id": h.bots.StartStress(id, req.Bidders, req.Rounds)})
}

// StressEvents streams a run's progress; a late subscriber gets everything from the start.
func (h *Handler) StressEvents(w http.ResponseWriter, r *http.Request) {
	if h.off(w) {
		return
	}
	snap, ok := h.bots.Events(r.PathValue("id"))
	if !ok {
		httpx.Error(w, http.StatusNotFound, "unknown run")
		return
	}
	// the server's write timeout would cut a long run's stream
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := w.(http.Flusher)
	sent := 0
	for {
		batch, wake, done := snap(sent)
		for _, e := range batch {
			b, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", b)
			sent++
		}
		if flusher != nil {
			flusher.Flush()
		}
		if done && len(batch) == 0 {
			return
		}
		select {
		case <-wake:
		case <-r.Context().Done():
			return
		}
	}
}

// Seed handles POST /v1/demo/seed (operators only, enforced by the route).
func (h *Handler) Seed(w http.ResponseWriter, r *http.Request) {
	if h.off(w) {
		return
	}
	res, err := h.bots.Seed(r.Context(), SeedOptions{Catalog: h.catalog, AdminEmail: os.Getenv("DEMO_ADMIN_EMAIL"), AdminPassword: os.Getenv("DEMO_ADMIN_PASSWORD")})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not load the demo cars: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}
