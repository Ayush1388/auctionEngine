package user

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argon2Time    uint32 = 1
	argon2Memory  uint32 = 64 * 1024
	argon2Threads uint8  = 4
	argon2KeyLen  uint32 = 32
	argon2SaltLen        = 16
)

func HashPassword(password string) (string, error) {
	salt := make([]byte, argon2SaltLen)

	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		argon2Time,
		argon2Memory,
		argon2Threads,
		argon2KeyLen,
	)

	encodedSalt := base64.RawStdEncoding.EncodeToString(salt)
	encodedHash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argon2Memory,
		argon2Time,
		argon2Threads,
		encodedSalt,
		encodedHash,
	), nil
}

func CheckPassword(password, encodedHash string) error {
	salt, expectedHash, err := parseHash(encodedHash)
	if err != nil {
		return err
	}

	actualHash := argon2.IDKey(
		[]byte(password),
		salt,
		argon2Time,
		argon2Memory,
		argon2Threads,
		argon2KeyLen,
	)

	if subtle.ConstantTimeCompare(actualHash, expectedHash) != 1 {
		return fmt.Errorf("invalid password")
	}

	return nil
}

func parseHash(encodedHash string) ([]byte, []byte, error) {
	parts := strings.Split(encodedHash, "$")

	if len(parts) != 6 {
		return nil, nil, fmt.Errorf("invalid password hash format")
	}

	if parts[1] != "argon2id" || parts[2] != "v=19" {
		return nil, nil, fmt.Errorf("unsupported password hash")
	}

	params := strings.Split(parts[3], ",")

	if len(params) != 3 {
		return nil, nil, fmt.Errorf("invalid Argon2 parameters")
	}

	var memory uint32
	var time uint32
	var threads uint8

	for _, param := range params {
		parts := strings.SplitN(param, "=", 2)

		if len(parts) != 2 {
			return nil, nil, fmt.Errorf("invalid Argon2 parameter")
		}

		value, err := strconv.ParseUint(parts[1], 10, 32)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid Argon2 parameter value")
		}

		switch parts[0] {
		case "m":
			memory = uint32(value)
		case "t":
			time = uint32(value)
		case "p":
			threads = uint8(value)
		}
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid password salt")
	}

	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid password hash")
	}

	if memory != argon2Memory ||
		time != argon2Time ||
		threads != argon2Threads {
		return nil, nil, fmt.Errorf("unsupported Argon2 parameters")
	}

	return salt, hash, nil
}
