package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/Ayush1388/auctionEngine/internal/httpx"
	"github.com/Ayush1388/auctionEngine/internal/user"
	"github.com/google/uuid"
)

// contextKey is unexported so no other package can collide with, or forge,
// the user ID stored in the request context.
type contextKey string

const (
	userIDKey contextKey = "user_id"
	roleKey   contextKey = "role"
)

// Middleware checks the Authorization: Bearer <jwt> header and, when valid,
// stores the user ID in the request context for handlers to read with
// UserIDFromContext. This is authentication (who are you?). Authorization
// (may you do this?) happens in the services, e.g. auction.Service.Cancel.
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
			ctx = context.WithValue(ctx, roleKey, claims.Role)

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

// RoleFromContext returns the role from the caller's access token.
func RoleFromContext(ctx context.Context) string {
	role, _ := ctx.Value(roleKey).(string)
	return role
}

// RequireRole allows the request only if the authenticated user has role.
// Use it behind Authenticate:
//
//	mw.Authenticate(mw.RequireRole("admin", handler))
//
// 401 means "log in"; 403 means "logged in, but not allowed". Checking the
// role from the token (not the database) keeps this free, at the cost that a
// demoted admin keeps access until their access token expires. That is why
// access tokens are short-lived.
func (m *Middleware) RequireRole(role string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := UserIDFromContext(r.Context()); !ok {
			httpx.Error(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if RoleFromContext(r.Context()) != role {
			httpx.Error(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r)
	})
}
