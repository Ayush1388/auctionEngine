package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/auth"
	"github.com/Ayush1388/auctionEngine/internal/bidding"
	"github.com/Ayush1388/auctionEngine/internal/bidqueue"
	"github.com/Ayush1388/auctionEngine/internal/httpx"
	"github.com/Ayush1388/auctionEngine/internal/validation"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

// Bidder is everything the bid handlers need from the bidding domain.
//
// bidding.Service satisfies it in-process (the monolith), and
// grpcsvc.Client satisfies it over the network (v0.9, bidding as a separate
// service). The handlers depend on this interface, not on either concrete
// type: the Dependency Inversion Principle. That is why extracting bidding
// into its own process changed no handler code and no HTTP test.
type Bidder interface {
	PlaceBid(ctx context.Context, in bidding.PlaceBidInput) (bidding.Result, error)
	History(ctx context.Context, auctionID uuid.UUID, limit, cursor string) (bidding.HistoryPage, error)
}

type BidHandler struct {
	service Bidder

	// queue enables asynchronous bids through Kafka (v0.8). nil means every
	// bid is placed synchronously, even if the client asks for async.
	queue *bidqueue.Requests
}

// WithQueue enables "Prefer: respond-async" bids.
func (h *BidHandler) WithQueue(q *bidqueue.Requests) *BidHandler {
	h.queue = q
	return h
}

func NewBidHandler(service Bidder) *BidHandler {
	return &BidHandler{service: service}
}

type placeBidRequest struct {
	Amount int64 `json:"amount"`
}

type bidResponse struct {
	ID        string    `json:"id"`
	AuctionID string    `json:"auction_id"`
	UserID    string    `json:"user_id"`
	Amount    int64     `json:"amount"`
	CreatedAt time.Time `json:"created_at"`
}

type placeBidResponse struct {
	Bid        bidResponse `json:"bid"`
	CurrentBid int64       `json:"current_bid"`
	BidCount   int         `json:"bid_count"`
	EndsAt     time.Time   `json:"ends_at"`
	Extended   bool        `json:"extended"`
	Replayed   bool        `json:"replayed"`
}

func toBidResponse(b bidding.Bid) bidResponse {
	return bidResponse{
		ID:        b.ID.String(),
		AuctionID: b.AuctionID.String(),
		UserID:    b.UserID.String(),
		Amount:    b.Amount,
		CreatedAt: b.CreatedAt.UTC(),
	}
}

// Place handles POST /v1/auctions/{id}/bids.
//
// Clients should send an Idempotency-Key header (any unique string, e.g. a
// UUID generated per click). If the response is lost to a timeout and the
// client retries with the same key, it gets the original bid back instead
// of bidding twice.
//
//	201  bid accepted
//	200  replay of an earlier request with the same key
//	403  bidding on your own auction
//	404  no such auction
//	409  auction not open (not started, ended, cancelled)
//	422  too low (response says the minimum), insufficient funds, or the key
//	     was used for a different bid
//	503  optimistic strategy gave up under contention; retry
func (h *BidHandler) Place(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	auctionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid auction id")
		return
	}

	var req placeBidRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.BadRequest(w, err)
		return
	}

	// RFC 7240: a client that can handle a 202 says so with
	// "Prefer: respond-async". Others keep getting the synchronous answer.
	if h.queue != nil && strings.Contains(strings.ToLower(r.Header.Get("Prefer")), "respond-async") {
		h.placeAsync(w, r, auctionID, userID, req.Amount)
		return
	}

	started := time.Now()
	res, err := h.service.PlaceBid(r.Context(), bidding.PlaceBidInput{
		AuctionID:      auctionID,
		UserID:         userID,
		Amount:         req.Amount,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
	})
	setBidTiming(w, r, time.Since(started), res.Timings)

	var tooLow *bidding.BidTooLowError

	switch {
	case err == nil:
		status := http.StatusCreated
		if res.Replayed {
			status = http.StatusOK
		}
		httpx.WriteJSON(w, status, placeBidResponse{
			Bid:        toBidResponse(res.Bid),
			CurrentBid: res.CurrentBid,
			BidCount:   res.BidCount,
			EndsAt:     res.EndsAt.UTC(),
			Extended:   res.Extended,
			Replayed:   res.Replayed,
		})
	case errors.As(err, &tooLow):
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":          tooLow.Error(),
			"minimum_amount": tooLow.Minimum,
		})
	case errors.Is(err, auction.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "auction not found")
	case errors.Is(err, bidding.ErrOwnAuction):
		httpx.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, bidding.ErrAuctionNotActive), errors.Is(err, bidding.ErrAuctionEnded):
		httpx.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, wallet.ErrInsufficientFunds),
		errors.Is(err, bidding.ErrIdempotencyMismatch),
		errors.Is(err, bidding.ErrInvalidAmount),
		errors.Is(err, bidding.ErrInvalidKey):
		httpx.Error(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, bidding.ErrContention), errors.Is(err, bidding.ErrUnavailable):
		w.Header().Set("Retry-After", "1")
		httpx.Error(w, http.StatusServiceUnavailable, err.Error())
	default:
		httpx.ServerError(w, r, err)
	}
}

// setBidTiming reports where the time of a bid went, so a client can show it.
//
// Server-Timing (W3C) carries the phases of the transaction measured by the
// bidding service, plus "bidding", the whole call as the gateway saw it (with
// a separate bidding service that includes the gRPC hop). X-Trace-ID is the
// OpenTelemetry trace of the request when tracing is on, so the client can
// link to the trace. Both are exposed to browsers by the CORS middleware.
func setBidTiming(w http.ResponseWriter, r *http.Request, total time.Duration, t bidding.Timings) {
	ms := func(d time.Duration) string { return strconv.FormatFloat(float64(d.Microseconds())/1000, 'f', 2, 64) }

	var parts []string
	if !t.IsZero() {
		parts = append(parts,
			"lock;dur="+ms(t.Lock),
			"decide;dur="+ms(t.Decide),
			"write;dur="+ms(t.Write),
			"commit;dur="+ms(t.Commit),
		)
	}
	parts = append(parts, "bidding;dur="+ms(total))
	w.Header().Set("Server-Timing", strings.Join(parts, ", "))

	if sc := trace.SpanContextFromContext(r.Context()); sc.IsValid() {
		w.Header().Set("X-Trace-ID", sc.TraceID().String())
	}
}

type bidRequestResponse struct {
	ID            string     `json:"id"`
	AuctionID     string     `json:"auction_id"`
	Amount        int64      `json:"amount"`
	Status        string     `json:"status"`
	Reason        *string    `json:"reason,omitempty"`
	BidID         *string    `json:"bid_id,omitempty"`
	MinimumAmount *int64     `json:"minimum_amount,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	ProcessedAt   *time.Time `json:"processed_at,omitempty"`
	StatusURL     string     `json:"status_url"`
}

func toBidRequestResponse(q bidqueue.Request) bidRequestResponse {
	resp := bidRequestResponse{
		ID:            q.ID.String(),
		AuctionID:     q.AuctionID.String(),
		Amount:        q.Amount,
		Status:        string(q.Status),
		Reason:        q.Reason,
		MinimumAmount: q.MinimumAmount,
		CreatedAt:     q.CreatedAt.UTC(),
		ProcessedAt:   q.ProcessedAt,
		StatusURL:     "/v1/bid-requests/" + q.ID.String(),
	}
	if q.BidID != nil {
		s := q.BidID.String()
		resp.BidID = &s
	}
	return resp
}

// placeAsync queues the bid and answers 202 immediately. Final validation
// (auction open, amount high enough, funds) happens in the bid worker, so
// a queued bid can still end up REJECTED; poll status_url or watch the
// auction's WebSocket feed.
func (h *BidHandler) placeAsync(w http.ResponseWriter, r *http.Request, auctionID, userID uuid.UUID, amount int64) {
	q, replayed, err := h.queue.Submit(r.Context(), auctionID, userID, amount, r.Header.Get("Idempotency-Key"))
	switch {
	case err == nil:
		w.Header().Set("Location", "/v1/bid-requests/"+q.ID.String())
		status := http.StatusAccepted
		if replayed {
			status = http.StatusOK
		}
		httpx.WriteJSON(w, status, toBidRequestResponse(q))
	case errors.Is(err, bidqueue.ErrInvalidAmount):
		httpx.Error(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, bidqueue.ErrAuctionNotFound):
		httpx.Error(w, http.StatusNotFound, "auction not found")
	default:
		httpx.ServerError(w, r, err)
	}
}

// Request handles GET /v1/bid-requests/{id}: the outcome of an async bid.
// Only its owner can see it.
func (h *BidHandler) Request(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid bid request id")
		return
	}
	if h.queue == nil {
		httpx.Error(w, http.StatusNotFound, "bid request not found")
		return
	}

	q, err := h.queue.Get(r.Context(), id, userID)
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, toBidRequestResponse(q))
	case errors.Is(err, bidqueue.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "bid request not found")
	default:
		httpx.ServerError(w, r, err)
	}
}

type bidHistoryResponse struct {
	Bids       []bidResponse `json:"bids"`
	NextCursor *string       `json:"next_cursor"`
}

// History handles GET /v1/auctions/{id}/bids?limit=&cursor=. Public.
func (h *BidHandler) History(w http.ResponseWriter, r *http.Request) {
	auctionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid auction id")
		return
	}

	q := r.URL.Query()
	page, err := h.service.History(r.Context(), auctionID, q.Get("limit"), q.Get("cursor"))
	switch {
	case err == nil:
	case errors.Is(err, validation.ErrInvalid):
		httpx.BadRequest(w, err)
		return
	case errors.Is(err, auction.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "auction not found")
		return
	default:
		httpx.ServerError(w, r, err)
		return
	}

	resp := bidHistoryResponse{Bids: make([]bidResponse, len(page.Bids))}
	for i, b := range page.Bids {
		resp.Bids[i] = toBidResponse(b)
	}
	if page.NextCursor != nil {
		c := page.NextCursor.Encode()
		resp.NextCursor = &c
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}
