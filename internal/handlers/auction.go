package handlers

import (
	"errors"
	"net/http"
	"time"

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
