package user

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// JWTService issues and verifies access tokens.
//
// A JWT is header.payload.signature. The payload (claims) is only base64,
// so anyone can read it; the HMAC-SHA256 signature is what stops anyone
// from changing it. Because the server can verify a token with just the
// secret, it needs no session lookup per request (stateless auth). The cost
// is that a token can't be revoked before it expires, which is why expiry
// matters (see JWT_EXPIRATION_HOURS).
type JWTService struct {
	secret     []byte
	issuer     string
	expiration time.Duration
}

func NewJWTService(
	secret string,
	issuer string,
	expiration time.Duration,
) *JWTService {
	return &JWTService{
		secret:     []byte(secret),
		issuer:     issuer,
		expiration: expiration,
	}
}

type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	Role   string    `json:"role"`

	jwt.RegisteredClaims
}

// GenerateToken issues an access token for userID with the given role.
func (s *JWTService) GenerateToken(userID uuid.UUID, role string) (string, error) {
	now := time.Now()

	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Issuer:    s.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.expiration)),
		},
	}

	token := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		claims,
	)

	signedToken, err := token.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	return signedToken, nil
}

func (s *JWTService) ValidateToken(
	tokenString string,
) (Claims, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&Claims{},
		func(token *jwt.Token) (any, error) {
			// Pin the algorithm. Without this check, a forged token
			// with "alg": "none" or an RS256/HS256 confusion attack
			// could pass verification.
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, fmt.Errorf(
					"unexpected signing method: %s",
					token.Method.Alg(),
				)
			}

			return s.secret, nil
		},
		jwt.WithIssuer(s.issuer),
		jwt.WithValidMethods(
			[]string{jwt.SigningMethodHS256.Alg()},
		),
	)

	if err != nil {
		return Claims{}, fmt.Errorf("invalid JWT: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return Claims{}, fmt.Errorf("invalid JWT claims")
	}

	if !token.Valid {
		return Claims{}, fmt.Errorf("invalid JWT")
	}

	return *claims, nil
}
