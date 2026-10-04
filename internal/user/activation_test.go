package user

import "testing"

func TestGenerateActivationToken(t *testing.T) {
	token1, hash1, err := GenerateActivationToken()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if token1 == "" {
		t.Fatal("expected token, got empty string")
	}

	if hash1 == "" {
		t.Fatal("expected token hash, got empty string")
	}

	token2, hash2, err := GenerateActivationToken()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if token1 == token2 {
		t.Error("expected different tokens")
	}

	if hash1 == hash2 {
		t.Error("expected different token hashes")
	}
}
