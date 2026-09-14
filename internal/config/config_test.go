package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("PORT", "4000")
	t.Setenv("ENVIRONMENT", "development")
	t.Setenv(
		"DATABASE_URL",
		"postgres://auction:auction@localhost:5432/auction?sslmode=disable",
	)

	cfg, err := Load()

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Port != 4000 {
		t.Errorf("expected port 4000, got %d", cfg.Port)
	}

	if cfg.Environment != "development" {
		t.Errorf(
			"expected environment development, got %s",
			cfg.Environment,
		)
	}

	expectedURL := "postgres://auction:auction@localhost:5432/auction?sslmode=disable"

	if cfg.DatabaseURL != expectedURL {
		t.Errorf(
			"expected database URL %s, got %s",
			expectedURL,
			cfg.DatabaseURL,
		)
	}
}

func TestLoadInvalidPort(t *testing.T) {
	t.Setenv("PORT", "hello")
	t.Setenv("ENVIRONMENT", "development")
	t.Setenv(
		"DATABASE_URL",
		"postgres://auction:auction@localhost:5432/auction?sslmode=disable",
	)

	_, err := Load()

	if err == nil {
		t.Fatal("expected error for invalid PORT, got nil")
	}
}

func TestLoadMissingEnvironment(t *testing.T) {
	t.Setenv("PORT", "4000")
	t.Setenv("ENVIRONMENT", "")
	t.Setenv(
		"DATABASE_URL",
		"postgres://auction:auction@localhost:5432/auction?sslmode=disable",
	)

	_, err := Load()

	if err == nil {
		t.Fatal("expected error for missing ENVIRONMENT, got nil")
	}
}

func TestLoadMissingDatabaseURL(t *testing.T) {
	t.Setenv("PORT", "4000")
	t.Setenv("ENVIRONMENT", "development")
	t.Setenv("DATABASE_URL", "")

	_, err := Load()

	if err == nil {
		t.Fatal("expected error for missing DATABASE_URL, got nil")
	}
}
