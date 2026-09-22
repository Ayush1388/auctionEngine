package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Ayush1338/auctionEngine/internal/auth"
	"github.com/Ayush1338/auctionEngine/internal/user"
)

type UserHandler struct {
	service *user.Service
}

func NewUserHandler(service *user.Service) *UserHandler {
	return &UserHandler{
		service: service,
	}
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type registerResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type resendActivationRequest struct {
	Email string `json:"email"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken string       `json:"access_token"`
	TokenType   string       `json:"token_type"`
	User        userResponse `json:"user"`
}

type userResponse struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	ActivatedAt string `json:"activated_at,omitempty"`
}

func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(
			w,
			"invalid JSON",
			http.StatusBadRequest,
		)
		return
	}

	newUser, err := h.service.Register(
		r.Context(),
		user.RegisterInput{
			Email:    req.Email,
			Password: req.Password,
		},
	)

	if err != nil {
		if errors.Is(err, user.ErrEmailAlreadyExists) {
			http.Error(
				w,
				"email already exists",
				http.StatusConflict,
			)
			return
		}

		http.Error(
			w,
			err.Error(),
			http.StatusBadRequest,
		)
		return
	}

	response := registerResponse{
		ID:    newUser.ID.String(),
		Email: newUser.Email,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		return
	}
}

func (h *UserHandler) Activate(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")

	if err := h.service.Activate(r.Context(), token); err != nil {
		if errors.Is(err, user.ErrInvalidActivationToken) {
			http.Error(
				w,
				"invalid or expired activation token",
				http.StatusBadRequest,
			)
			return
		}

		http.Error(
			w,
			"failed to activate account",
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)

	_, _ = w.Write([]byte("account activated successfully"))
}

func (h *UserHandler) ResendActivation(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req resendActivationRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(
			w,
			"invalid JSON",
			http.StatusBadRequest,
		)
		return
	}

	err := h.service.ResendActivation(
		r.Context(),
		user.ResendActivationInput{
			Email: req.Email,
		},
	)

	if err != nil {
		if errors.Is(err, user.ErrUserAlreadyActivated) {
			http.Error(
				w,
				"account is already activated",
				http.StatusConflict,
			)
			return
		}

		http.Error(
			w,
			err.Error(),
			http.StatusBadRequest,
		)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *UserHandler) Login(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req loginRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(
			w,
			"invalid JSON",
			http.StatusBadRequest,
		)
		return
	}

	result, err := h.service.Login(
		r.Context(),
		user.LoginInput{
			Email:    req.Email,
			Password: req.Password,
		},
	)

	if err != nil {
		if errors.Is(err, user.ErrInvalidCredentials) {
			http.Error(
				w,
				"invalid email or password",
				http.StatusUnauthorized,
			)
			return
		}

		if errors.Is(err, user.ErrAccountNotActivated) {
			http.Error(
				w,
				"account is not activated",
				http.StatusForbidden,
			)
			return
		}

		http.Error(
			w,
			err.Error(),
			http.StatusBadRequest,
		)
		return
	}

	response := loginResponse{
		AccessToken: result.AccessToken,
		TokenType:   "Bearer",
		User: userResponse{
			ID:    result.User.ID.String(),
			Email: result.User.Email,
		},
	}

	if result.User.ActivatedAt != nil {
		response.User.ActivatedAt = result.User.ActivatedAt.UTC().Format("2006-01-02T15:04:05Z")
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		return
	}
}

func (h *UserHandler) Me(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := auth.UserIDFromContext(r.Context())

	if !ok {
		http.Error(
			w,
			"unauthorized",
			http.StatusUnauthorized,
		)
		return
	}

	currentUser, err := h.service.GetByID(
		r.Context(),
		userID,
	)

	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			http.Error(
				w,
				"user not found",
				http.StatusNotFound,
			)
			return
		}

		http.Error(
			w,
			"failed to get user",
			http.StatusInternalServerError,
		)
		return
	}

	response := userResponse{
		ID:    currentUser.ID.String(),
		Email: currentUser.Email,
	}

	if currentUser.ActivatedAt != nil {
		response.ActivatedAt = currentUser.ActivatedAt.UTC().Format("2006-01-02T15:04:05Z")
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		return
	}
}
