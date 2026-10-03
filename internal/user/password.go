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

// HashPassword hashes password with Argon2id and a fresh random salt.
//
// Why a slow, memory-hard hash: passwords are low-entropy, so an attacker
// who steals the table will try billions of guesses. Argon2id makes each
// guess cost 64 MiB of memory, which is what makes GPU cracking expensive.
// The random salt means two users with the same password get different
// hashes, so precomputed tables are useless.
//
// Output is the standard PHC string:
//
//	$argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>
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

type argon2Params struct {
	memory  uint32
	time    uint32
	threads uint8
}

// CheckPassword verifies password against a stored PHC-format hash. It uses
// the parameters recorded in the hash, not the current constants, so
// raising the cost for new hashes doesn't lock out existing users.
func CheckPassword(password, encodedHash string) error {
	params, salt, expectedHash, err := parseHash(encodedHash)
	if err != nil {
		return err
	}

	actualHash := argon2.IDKey(
		[]byte(password),
		salt,
		params.time,
		params.memory,
		params.threads,
		uint32(len(expectedHash)),
	)

	if subtle.ConstantTimeCompare(actualHash, expectedHash) != 1 {
		return fmt.Errorf("invalid password")
	}

	return nil
}

// NeedsRehash reports whether a stored hash uses weaker parameters than
// the current ones, so it can be upgraded after a successful login.
func NeedsRehash(encodedHash string) bool {
	params, _, _, err := parseHash(encodedHash)
	if err != nil {
		return true
	}

	return params.memory < argon2Memory ||
		params.time < argon2Time ||
		params.threads < argon2Threads
}

// dummyHash is verified when a login names an unknown email, so a login
// takes about as long whether the account exists or not. Otherwise response
// times would reveal which emails are registered.
var dummyHash = func() string {
	h, err := HashPassword("dummy-password-for-timing-only")
	if err != nil {
		panic(err)
	}
	return h
}()

func parseHash(encodedHash string) (argon2Params, []byte, []byte, error) {
	var params argon2Params

	parts := strings.Split(encodedHash, "$")

	if len(parts) != 6 {
		return params, nil, nil, fmt.Errorf("invalid password hash format")
	}

	if parts[1] != "argon2id" || parts[2] != "v=19" {
		return params, nil, nil, fmt.Errorf("unsupported password hash")
	}

	fields := strings.Split(parts[3], ",")

	if len(fields) != 3 {
		return params, nil, nil, fmt.Errorf("invalid Argon2 parameters")
	}

	for _, field := range fields {
		kv := strings.SplitN(field, "=", 2)

		if len(kv) != 2 {
			return params, nil, nil, fmt.Errorf("invalid Argon2 parameter")
		}

		value, err := strconv.ParseUint(kv[1], 10, 32)
		if err != nil {
			return params, nil, nil, fmt.Errorf("invalid Argon2 parameter value")
		}

		switch kv[0] {
		case "m":
			params.memory = uint32(value)
		case "t":
			params.time = uint32(value)
		case "p":
			params.threads = uint8(value)
		default:
			return params, nil, nil, fmt.Errorf("unknown Argon2 parameter %q", kv[0])
		}
	}

	// Refuse absurd values so a corrupted row can't make one login burn
	// gigabytes of memory.
	if params.memory < 8*1024 || params.memory > 1024*1024 ||
		params.time < 1 || params.time > 10 ||
		params.threads < 1 || params.threads > 16 {
		return params, nil, nil, fmt.Errorf("argon2 parameters out of range")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return params, nil, nil, fmt.Errorf("invalid password salt")
	}

	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(hash) < 16 {
		return params, nil, nil, fmt.Errorf("invalid password hash")
	}

	return params, salt, hash, nil
}
