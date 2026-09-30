package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/Ayush1388/auctionEngine/internal/httpx"
	"github.com/Ayush1388/auctionEngine/internal/user"
	"github.com/google/uuid"
)

type contextKey string

const userIDKey contextKey = "user_id"

type Middleware struct {
	jwt *user.JWTService
}

func NewMiddleware(
	jwtService *user.JWTService,
) *Middleware {
	return &Middleware{
		jwt: jwtService,
	}
}

func (m *Middleware) Authenticate(
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			authorizationHeader := r.Header.Get(
				"Authorization",
			)

			if authorizationHeader == "" {
				httpx.Error(w, http.StatusUnauthorized, "missing authorization header")
				return
			}

			parts := strings.Fields(
				authorizationHeader,
			)

			if len(parts) != 2 ||
				!strings.EqualFold(parts[0], "Bearer") {
				httpx.Error(w, http.StatusUnauthorized, "invalid authorization header")
				return
			}

			claims, err := m.jwt.ValidateToken(
				parts[1],
			)
			if err != nil {
				httpx.Error(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			ctx := context.WithValue(
				r.Context(),
				userIDKey,
				claims.UserID,
			)

			next.ServeHTTP(
				w,
				r.WithContext(ctx),
			)
		},
	)
}

func UserIDFromContext(
	ctx context.Context,
) (uuid.UUID, bool) {
	userID, ok := ctx.Value(userIDKey).(uuid.UUID)

	return userID, ok
}
