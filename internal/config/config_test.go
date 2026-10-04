package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("PORT", "4000")
	t.Setenv("ENVIRONMENT", "development")
	t.Setenv(
		"DATABASE_URL",
		"postgres://auction:auction@localhost:5432/auction?sslmode=disable",
	)

	t.Setenv("SMTP_HOST", "smtp.gmail.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USERNAME", "test@gmail.com")
	t.Setenv("SMTP_PASSWORD", "test-app-password")
	t.Setenv("SMTP_FROM", "test@gmail.com")
	t.Setenv("APP_BASE_URL", "http://localhost:4000")

	t.Setenv(
		"JWT_SECRET",
		"test-secret-that-is-at-least-32-characters-long",
	)
	t.Setenv("JWT_ISSUER", "auction-engine-test")
	t.Setenv("JWT_EXPIRATION_HOURS", "24")

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

	if cfg.SMTPHost != "smtp.gmail.com" {
		t.Errorf(
			"expected SMTP host smtp.gmail.com, got %s",
			cfg.SMTPHost,
		)
	}

	if cfg.SMTPPort != 587 {
		t.Errorf(
			"expected SMTP port 587, got %d",
			cfg.SMTPPort,
		)
	}

	if cfg.SMTPUsername != "test@gmail.com" {
		t.Errorf(
			"expected SMTP username test@gmail.com, got %s",
			cfg.SMTPUsername,
		)
	}

	if cfg.SMTPFrom != "test@gmail.com" {
		t.Errorf(
			"expected SMTP from test@gmail.com, got %s",
			cfg.SMTPFrom,
		)
	}

	if cfg.AppBaseURL != "http://localhost:4000" {
		t.Errorf(
			"expected app base URL http://localhost:4000, got %s",
			cfg.AppBaseURL,
		)
	}

	if cfg.JWTSecret != "test-secret-that-is-at-least-32-characters-long" {
		t.Errorf(
			"expected JWT secret to be loaded",
		)
	}

	if cfg.JWTIssuer != "auction-engine-test" {
		t.Errorf(
			"expected JWT issuer auction-engine-test, got %s",
			cfg.JWTIssuer,
		)
	}

	if cfg.JWTExpiration.Hours() != 24 {
		t.Errorf(
			"expected JWT expiration 24 hours, got %v",
			cfg.JWTExpiration,
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

	t.Setenv("SMTP_HOST", "smtp.gmail.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USERNAME", "test@gmail.com")
	t.Setenv("SMTP_PASSWORD", "test-app-password")
	t.Setenv("SMTP_FROM", "test@gmail.com")
	t.Setenv("APP_BASE_URL", "http://localhost:4000")

	t.Setenv(
		"JWT_SECRET",
		"test-secret-that-is-at-least-32-characters-long",
	)
	t.Setenv("JWT_ISSUER", "auction-engine-test")
	t.Setenv("JWT_EXPIRATION_HOURS", "24")

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

	t.Setenv("SMTP_HOST", "smtp.gmail.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USERNAME", "test@gmail.com")
	t.Setenv("SMTP_PASSWORD", "test-app-password")
	t.Setenv("SMTP_FROM", "test@gmail.com")
	t.Setenv("APP_BASE_URL", "http://localhost:4000")

	t.Setenv(
		"JWT_SECRET",
		"test-secret-that-is-at-least-32-characters-long",
	)
	t.Setenv("JWT_ISSUER", "auction-engine-test")
	t.Setenv("JWT_EXPIRATION_HOURS", "24")

	_, err := Load()

	if err == nil {
		t.Fatal("expected error for missing ENVIRONMENT, got nil")
	}
}

func TestLoadMissingDatabaseURL(t *testing.T) {
	t.Setenv("PORT", "4000")
	t.Setenv("ENVIRONMENT", "development")
	t.Setenv("DATABASE_URL", "")

	t.Setenv("SMTP_HOST", "smtp.gmail.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USERNAME", "test@gmail.com")
	t.Setenv("SMTP_PASSWORD", "test-app-password")
	t.Setenv("SMTP_FROM", "test@gmail.com")
	t.Setenv("APP_BASE_URL", "http://localhost:4000")

	t.Setenv(
		"JWT_SECRET",
		"test-secret-that-is-at-least-32-characters-long",
	)
	t.Setenv("JWT_ISSUER", "auction-engine-test")
	t.Setenv("JWT_EXPIRATION_HOURS", "24")

	_, err := Load()

	if err == nil {
		t.Fatal("expected error for missing DATABASE_URL, got nil")
	}
}
