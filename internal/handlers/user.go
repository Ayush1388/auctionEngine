package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/Ayush1388/auctionEngine/internal/auth"
	"github.com/Ayush1388/auctionEngine/internal/httpx"
	"github.com/Ayush1388/auctionEngine/internal/user"
	"github.com/Ayush1388/auctionEngine/internal/validation"
)

// UserHandler is the HTTP layer for users. Handlers only translate: JSON in,
// call the service, map the result or error to a status code, JSON out.
// Business rules live in user.Service so they can be tested without HTTP.
type UserHandler struct {
	service *user.Service
}

func NewUserHandler(service *user.Service) *UserHandler {
	return &UserHandler{
		service: service,
	}
}

type userResponse struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	ActivatedAt *time.Time `json:"activated_at,omitempty"`
}

type loginResponse struct {
	AccessToken string       `json:"access_token"`
	TokenType   string       `json:"token_type"`
	User        userResponse `json:"user"`
}

func toUserResponse(u user.User) userResponse {
	response := userResponse{
		ID:    u.ID.String(),
		Email: u.Email,
	}

	if u.ActivatedAt != nil {
		activatedAt := u.ActivatedAt.UTC()
		response.ActivatedAt = &activatedAt
	}

	return response
}

func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	var input user.RegisterInput
	if err := httpx.ReadJSON(w, r, &input); err != nil {
		httpx.BadRequest(w, err)
		return
	}

	newUser, err := h.service.Register(r.Context(), input)
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusCreated, toUserResponse(newUser))
	case errors.Is(err, validation.ErrInvalid):
		httpx.BadRequest(w, err)
	case errors.Is(err, user.ErrEmailAlreadyExists):
		httpx.Error(w, http.StatusConflict, "email already exists")
	default:
		httpx.ServerError(w, r, err)
	}
}

func (h *UserHandler) Activate(w http.ResponseWriter, r *http.Request) {
	err := h.service.Activate(r.Context(), r.URL.Query().Get("token"))
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "account activated"})
	case errors.Is(err, user.ErrInvalidActivationToken):
		httpx.Error(w, http.StatusBadRequest, "invalid or expired activation token")
	default:
		httpx.ServerError(w, r, err)
	}
}

func (h *UserHandler) ResendActivation(w http.ResponseWriter, r *http.Request) {
	var input user.ResendActivationInput
	if err := httpx.ReadJSON(w, r, &input); err != nil {
		httpx.BadRequest(w, err)
		return
	}

	// Always 204 for a valid email, whether or not it has an account or is
	// already activated, so this endpoint can't be used to probe for users.
	err := h.service.ResendActivation(r.Context(), input)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, validation.ErrInvalid):
		httpx.BadRequest(w, err)
	default:
		httpx.ServerError(w, r, err)
	}
}

func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
	var input user.LoginInput
	if err := httpx.ReadJSON(w, r, &input); err != nil {
		httpx.BadRequest(w, err)
		return
	}

	result, err := h.service.Login(r.Context(), input)
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, loginResponse{
			AccessToken: result.AccessToken,
			TokenType:   "Bearer",
			User:        toUserResponse(result.User),
		})
	case errors.Is(err, validation.ErrInvalid):
		httpx.BadRequest(w, err)
	case errors.Is(err, user.ErrInvalidCredentials):
		httpx.Error(w, http.StatusUnauthorized, "invalid email or password")
	case errors.Is(err, user.ErrAccountNotActivated):
		httpx.Error(w, http.StatusForbidden, "account is not activated")
	default:
		httpx.ServerError(w, r, err)
	}
}

func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	currentUser, err := h.service.GetByID(r.Context(), userID)
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, toUserResponse(currentUser))
	case errors.Is(err, user.ErrUserNotFound):
		httpx.Error(w, http.StatusNotFound, "user not found")
	default:
		httpx.ServerError(w, r, err)
	}
}
