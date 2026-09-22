package user

import "testing"

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
