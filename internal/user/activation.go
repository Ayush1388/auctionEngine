package user

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

const activationTokenBytes = 32

// GenerateActivationToken returns a random token for the email link and its
// SHA-256 hash for the database.
//
// Only the hash is stored. If the users table leaks, the attacker gets
// hashes, and a hash can't be turned back into a working link. SHA-256 is
// fine here (unlike for passwords) because the token is 32 random bytes,
// far too many possibilities to guess, so a slow hash adds nothing.
func GenerateActivationToken() (string, string, error) {
	tokenBytes := make([]byte, activationTokenBytes)

	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", fmt.Errorf(
			"failed to generate activation token: %w",
			err,
		)
	}

	rawToken := base64.RawURLEncoding.EncodeToString(tokenBytes)

	return rawToken, HashActivationToken(rawToken), nil
}

func HashActivationToken(token string) string {
	hash := sha256.Sum256([]byte(token))

	return base64.RawURLEncoding.EncodeToString(hash[:])
}
