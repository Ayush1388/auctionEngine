package user

import (
	"encoding/base64"
	"fmt"
	"testing"

	"golang.org/x/crypto/argon2"
)

func TestHashAndCheckPassword(t *testing.T) {
	password := "mySecretPassword123"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if hash == "" {
		t.Fatal("expected password hash, got empty string")
	}

	if err := CheckPassword(password, hash); err != nil {
		t.Errorf("expected password to match hash, got %v", err)
	}

	if err := CheckPassword("wrongPassword", hash); err == nil {
		t.Error("expected wrong password to fail")
	}
}

func TestHashPasswordProducesDifferentHashes(t *testing.T) {
	password := "mySecretPassword123"

	hash1, err := HashPassword(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	hash2, err := HashPassword(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	if hash1 == hash2 {
		t.Error("expected different hashes for the same password")
	}
}

func TestCheckPasswordInvalidHash(t *testing.T) {
	err := CheckPassword(
		"mySecretPassword123",
		"invalid-hash",
	)

	if err == nil {
		t.Error("expected invalid hash to return an error")
	}
}

// A hash made with different (older) parameters must still verify.
func TestCheckPasswordUsesParametersFromHash(t *testing.T) {
	salt := []byte("0123456789abcdef")
	key := argon2.IDKey([]byte("old-password-123"), salt, 2, 32*1024, 2, 32)
	old := fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		32*1024, 2, 2,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	)

	if err := CheckPassword("old-password-123", old); err != nil {
		t.Fatalf("hash with older parameters no longer verifies: %v", err)
	}
	if err := CheckPassword("wrong-password", old); err == nil {
		t.Fatal("wrong password accepted")
	}
	if !NeedsRehash(old) {
		t.Fatal("weaker hash should need a rehash")
	}

	current, _ := HashPassword("x")
	if NeedsRehash(current) {
		t.Fatal("a fresh hash should not need a rehash")
	}
}

func TestCheckPasswordRejectsAbsurdParameters(t *testing.T) {
	huge := "$argon2id$v=19$m=4294967295,t=1,p=1$c2FsdHNhbHRzYWx0c2FsdA$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGFzaGhhc2g"
	if err := CheckPassword("x", huge); err == nil {
		t.Fatal("expected out-of-range parameters to be rejected")
	}
}
