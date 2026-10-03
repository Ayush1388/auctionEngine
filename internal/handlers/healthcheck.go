package handlers

import (
	"net/http"

	"github.com/Ayush1388/auctionEngine/internal/httpx"
)

type HealthResponse struct {
	Status string `json:"status"`
}

func Healthcheck(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, HealthResponse{Status: "available"})
}
