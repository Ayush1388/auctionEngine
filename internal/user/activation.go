package user

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

const activationTokenBytes = 32

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
