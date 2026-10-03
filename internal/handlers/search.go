package handlers

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/httpx"
	"github.com/Ayush1388/auctionEngine/internal/search"
	"github.com/Ayush1388/auctionEngine/internal/validation"
)

// SearchHandler serves full-text search and type-ahead suggestions (v0.6).
type SearchHandler struct {
	search   *search.Service
	auctions *auction.Service
}

func NewSearchHandler(s *search.Service, auctions *auction.Service) *SearchHandler {
	return &SearchHandler{search: s, auctions: auctions}
}

type searchResult struct {
	auctionResponse
	// Highlight is a snippet with matched words in <em>…</em>.
	Highlight string `json:"highlight,omitempty"`
}

type searchResponse struct {
	Auctions   []searchResult `json:"auctions"`
	NextCursor *string        `json:"next_cursor"`
	// Backend says who answered: "elasticsearch", or "postgres" when
	// Elasticsearch is not configured or is down.
	Backend string `json:"backend"`
}

// Search handles GET /v1/auctions/search?q=&status=&limit=&cursor=.
//
// The search engine returns only IDs in relevance order; the auctions are
// then loaded from PostgreSQL in ONE query (GetMany). That keeps prices and
// statuses exact, because the index can lag the database by a second or two.
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	page, err := h.search.Search(r.Context(), search.SearchInput{
		Text:   q.Get("q"),
		Status: q.Get("status"),
		Limit:  q.Get("limit"),
		Cursor: q.Get("cursor"),
	})
	if err != nil {
		if errors.Is(err, validation.ErrInvalid) {
			httpx.BadRequest(w, err)
			return
		}
		httpx.ServerError(w, r, err)
		return
	}

	ids := make([]uuid.UUID, len(page.Hits))
	highlights := make(map[uuid.UUID]string, len(page.Hits))
	for i, hit := range page.Hits {
		ids[i] = hit.ID
		highlights[hit.ID] = hit.Highlight
	}

	found, err := h.auctions.GetMany(r.Context(), ids)
	if err != nil {
		httpx.ServerError(w, r, err)
		return
	}

	resp := searchResponse{Auctions: make([]searchResult, 0, len(found)), Backend: page.Backend}
	for _, a := range found {
		resp.Auctions = append(resp.Auctions, searchResult{
			auctionResponse: toAuctionResponse(a),
			Highlight:       highlights[a.ID],
		})
	}
	if page.NextCursor != "" {
		resp.NextCursor = &page.NextCursor
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

// Suggest handles GET /v1/auctions/suggest?q=&limit= for type-ahead.
func (h *SearchHandler) Suggest(w http.ResponseWriter, r *http.Request) {
	out, err := h.search.Suggest(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("limit"))
	if err != nil {
		if errors.Is(err, validation.ErrInvalid) {
			httpx.BadRequest(w, err)
			return
		}
		httpx.ServerError(w, r, err)
		return
	}
	if out == nil {
		out = []search.Suggestion{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"suggestions": out})
}
