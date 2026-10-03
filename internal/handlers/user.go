package handlers

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Ayush1388/auctionEngine/internal/auth"
	"github.com/Ayush1388/auctionEngine/internal/httpx"
	"github.com/Ayush1388/auctionEngine/internal/ratelimit"
	"github.com/Ayush1388/auctionEngine/internal/session"
	"github.com/Ayush1388/auctionEngine/internal/user"
	"github.com/Ayush1388/auctionEngine/internal/validation"
)

// UserHandler is the HTTP layer for users. Handlers only translate: JSON in,
// call the service, map the result or error to a status code, JSON out.
// Business rules live in user.Service so they can be tested without HTTP.
type UserHandler struct {
	service  *user.Service
	sessions *session.Service

	// loginLimiter throttles login attempts per email address (brute-force
	// protection). The per-IP limit alone isn't enough: an attacker with
	// many IPs (a botnet) can still hammer one account. nil disables it.
	loginLimiter ratelimit.Limiter
}

// LoginPerEmail allows 5 quick attempts, then one per minute, per account.
// Real users almost never notice; guessing a password at 1/minute is
// hopeless against Argon2id-hashed passwords of 15+ characters.
var LoginPerEmail = ratelimit.PerMinute("login_email", 1, 5)

func NewUserHandler(service *user.Service, sessions *session.Service, loginLimiter ratelimit.Limiter) *UserHandler {
	return &UserHandler{
		service:      service,
		sessions:     sessions,
		loginLimiter: loginLimiter,
	}
}

type userResponse struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	ActivatedAt *time.Time `json:"activated_at,omitempty"`
}

type loginResponse struct {
	AccessToken      string       `json:"access_token"`
	TokenType        string       `json:"token_type"`
	RefreshToken     string       `json:"refresh_token"`
	RefreshExpiresAt time.Time    `json:"refresh_expires_at"`
	User             userResponse `json:"user"`
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

	if h.loginLimiter != nil {
		key := strings.ToLower(strings.TrimSpace(input.Email))
		d, err := h.loginLimiter.Allow(r.Context(), LoginPerEmail, key)
		if err == nil && !d.Allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(d.RetryAfter.Seconds()))))
			httpx.Error(w, http.StatusTooManyRequests, "too many login attempts for this account, try again later")
			return
		}
	}

	result, err := h.service.Login(r.Context(), input)
	switch {
	case err == nil:
		tokens, err := h.sessions.Issue(r.Context(), result.User.ID, result.User.Role)
		if err != nil {
			httpx.ServerError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, loginResponse{
			AccessToken:      tokens.AccessToken,
			TokenType:        "Bearer",
			RefreshToken:     tokens.RefreshToken,
			RefreshExpiresAt: tokens.RefreshExpiresAt.UTC(),
			User:             toUserResponse(result.User),
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
