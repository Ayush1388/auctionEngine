package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/auth"
	"github.com/Ayush1388/auctionEngine/internal/httpx"
	"github.com/Ayush1388/auctionEngine/internal/validation"
)

type AuctionHandler struct {
	service *auction.Service
}

func NewAuctionHandler(service *auction.Service) *AuctionHandler {
	return &AuctionHandler{
		service: service,
	}
}

type itemResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

type auctionResponse struct {
	ID            string       `json:"id"`
	OwnerID       string       `json:"owner_id"`
	Item          itemResponse `json:"item"`
	StartingPrice int64        `json:"starting_price"`
	CurrentBid    *int64       `json:"current_bid"`
	StartsAt      time.Time    `json:"starts_at"`
	EndsAt        time.Time    `json:"ends_at"`
	Status        string       `json:"status"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
}

func toAuctionResponse(a auction.Auction) auctionResponse {
	return auctionResponse{
		ID:      a.ID.String(),
		OwnerID: a.OwnerID.String(),
		Item: itemResponse{
			ID:          a.Item.ID.String(),
			Name:        a.Item.Name,
			Type:        a.Item.Type,
			Description: a.Item.Description,
		},
		StartingPrice: a.StartingPrice,
		CurrentBid:    a.CurrentBid,
		StartsAt:      a.StartsAt.UTC(),
		EndsAt:        a.EndsAt.UTC(),
		Status:        string(a.Status),
		CreatedAt:     a.CreatedAt.UTC(),
		UpdatedAt:     a.UpdatedAt.UTC(),
	}
}

// Create handles POST /v1/auctions. The owner is always the authenticated
// user; the request body cannot choose it.
func (h *AuctionHandler) Create(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var input auction.CreateInput
	if err := httpx.ReadJSON(w, r, &input); err != nil {
		httpx.BadRequest(w, err)
		return
	}

	created, err := h.service.Create(r.Context(), ownerID, input)
	switch {
	case err == nil:
		w.Header().Set("Location", "/v1/auctions/"+created.ID.String())
		httpx.WriteJSON(w, http.StatusCreated, toAuctionResponse(created))
	case errors.Is(err, validation.ErrInvalid):
		httpx.BadRequest(w, err)
	default:
		httpx.ServerError(w, r, err)
	}
}

// Get handles GET /v1/auctions/{id}. Auctions are public.
func (h *AuctionHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid auction id")
		return
	}

	found, err := h.service.Get(r.Context(), id)
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, toAuctionResponse(found))
	case errors.Is(err, auction.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "auction not found")
	default:
		httpx.ServerError(w, r, err)
	}
}

type listResponse struct {
	Auctions   []auctionResponse `json:"auctions"`
	NextCursor *string           `json:"next_cursor"`
}

// List handles GET /v1/auctions?status=&owner=me&limit=&cursor=
//
// owner=me needs a token; everything else is public.
func (h *AuctionHandler) List(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	q := auction.ListQuery{
		Status: query.Get("status"),
		Limit:  query.Get("limit"),
		Cursor: query.Get("cursor"),
	}

	switch owner := query.Get("owner"); owner {
	case "":
	case "me":
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.Error(w, http.StatusUnauthorized, "owner=me requires authentication")
			return
		}
		q.OwnerID = userID
	default:
		problems := &validation.Error{}
		problems.Add("owner", "must be \"me\"")
		httpx.ValidationError(w, problems)
		return
	}

	page, err := h.service.List(r.Context(), q)
	if err != nil {
		if errors.Is(err, validation.ErrInvalid) {
			httpx.BadRequest(w, err)
			return
		}
		httpx.ServerError(w, r, err)
		return
	}

	response := listResponse{
		Auctions: make([]auctionResponse, len(page.Auctions)),
	}
	for i, a := range page.Auctions {
		response.Auctions[i] = toAuctionResponse(a)
	}
	if page.NextCursor != nil {
		next := page.NextCursor.Encode()
		response.NextCursor = &next
	}

	httpx.WriteJSON(w, http.StatusOK, response)
}

// Cancel handles POST /v1/auctions/{id}/cancel.
//
//	401  not logged in
//	403  logged in, but not the owner
//	404  no such auction
//	409  the auction is no longer in a state that can be cancelled
func (h *AuctionHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid auction id")
		return
	}

	cancelled, err := h.service.Cancel(r.Context(), userID, id)
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, toAuctionResponse(cancelled))
	case errors.Is(err, auction.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "auction not found")
	case errors.Is(err, auction.ErrForbidden):
		httpx.Error(w, http.StatusForbidden, "only the owner can cancel this auction")
	case errors.Is(err, auction.ErrAlreadyStarted):
		httpx.Error(w, http.StatusConflict, "auction has already started")
	case errors.Is(err, auction.ErrInvalidTransition):
		httpx.Error(w, http.StatusConflict, "auction can no longer be cancelled")
	default:
		httpx.ServerError(w, r, err)
	}
}
