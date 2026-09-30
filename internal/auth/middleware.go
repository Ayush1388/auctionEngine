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

// Authenticate rejects requests without a valid bearer token.
func (m *Middleware) Authenticate(
	next http.Handler,
) http.Handler {
	return m.authenticate(next, true)
}

// Optional lets requests without an Authorization header through
// anonymously, for public endpoints that behave differently when the caller
// is logged in. A header that is present but invalid is still rejected, so
// a broken token never silently turns into an anonymous request.
func (m *Middleware) Optional(
	next http.Handler,
) http.Handler {
	return m.authenticate(next, false)
}

func (m *Middleware) authenticate(
	next http.Handler,
	required bool,
) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			authorizationHeader := r.Header.Get(
				"Authorization",
			)

			if authorizationHeader == "" {
				if !required {
					next.ServeHTTP(w, r)
					return
				}

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
