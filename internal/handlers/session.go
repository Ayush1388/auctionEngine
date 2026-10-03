package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/Ayush1388/auctionEngine/internal/httpx"
	"github.com/Ayush1388/auctionEngine/internal/session"
)

type SessionHandler struct {
	sessions *session.Service
}

func NewSessionHandler(sessions *session.Service) *SessionHandler {
	return &SessionHandler{sessions: sessions}
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type tokenResponse struct {
	AccessToken      string    `json:"access_token"`
	TokenType        string    `json:"token_type"`
	RefreshToken     string    `json:"refresh_token"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

func toTokenResponse(t session.Tokens) tokenResponse {
	return tokenResponse{
		AccessToken:      t.AccessToken,
		TokenType:        "Bearer",
		RefreshToken:     t.RefreshToken,
		RefreshExpiresAt: t.RefreshExpiresAt.UTC(),
	}
}

// Refresh handles POST /v1/auth/refresh: trade a refresh token for a new
// access token and a new refresh token (rotation).
func (h *SessionHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.BadRequest(w, err)
		return
	}

	tokens, err := h.sessions.Refresh(r.Context(), req.RefreshToken)
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, toTokenResponse(tokens))
	case errors.Is(err, session.ErrInvalidToken), errors.Is(err, session.ErrTokenReused):
		httpx.Error(w, http.StatusUnauthorized, err.Error())
	default:
		httpx.ServerError(w, r, err)
	}
}

// Logout handles POST /v1/auth/logout: revoke the session the refresh token
// belongs to. Always 204, even for unknown tokens (logging out twice is fine).
func (h *SessionHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.BadRequest(w, err)
		return
	}

	if err := h.sessions.Revoke(r.Context(), req.RefreshToken); err != nil {
		httpx.ServerError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
