package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

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
