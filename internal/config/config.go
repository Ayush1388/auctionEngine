// Package config loads runtime settings from environment variables.
//
// Why environment variables (the "12-factor app" rule): the same compiled
// binary runs in development, CI and production; only its environment
// changes. Secrets such as JWT_SECRET and SMTP_PASSWORD never live in the
// code or in git (.env is gitignored).
//
// Load fails fast: a missing or invalid setting stops the process at start-up
// with a clear message, instead of surfacing as a confusing error on the
// first request that needs it.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is every setting the API needs, already parsed and validated.
// It is built once in main and passed to the constructors that need it
// (dependency injection), so no package reads os.Getenv on its own.
type Config struct {
	Port        int
	Environment string
	DatabaseURL string

	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string

	AppBaseURL string

	JWTSecret     string
	JWTIssuer     string
	JWTExpiration time.Duration

	// BidLocking is "pessimistic" (default) or "optimistic"; see
	// internal/bidding/service.go for the trade-off.
	BidLocking string

	// RefreshTokenTTL is how long a login lasts without activity.
	RefreshTokenTTL time.Duration

	// CORSAllowedOrigins are the browser origins allowed to call the API,
	// e.g. https://app.example.com. Empty means no cross-origin browser
	// access (non-browser clients are unaffected).
	CORSAllowedOrigins []string

	// TrustedProxies are CIDRs of load balancers whose X-Forwarded-For
	// header may be believed when working out a client's IP.
	TrustedProxies []string

	// RateLimitsEnabled can be switched off for load testing.
	RateLimitsEnabled bool
}

func Load() (Config, error) {
	port, err := strconv.Atoi(
		os.Getenv("PORT"),
	)
	if err != nil {
		return Config{}, fmt.Errorf(
			"invalid PORT: %w",
			err,
		)
	}

	smtpPort, err := strconv.Atoi(
		os.Getenv("SMTP_PORT"),
	)
	if err != nil {
		return Config{}, fmt.Errorf(
			"invalid SMTP_PORT: %w",
			err,
		)
	}

	jwtExpirationHours, err := strconv.Atoi(
		os.Getenv("JWT_EXPIRATION_HOURS"),
	)
	if err != nil && os.Getenv("ACCESS_TOKEN_TTL") == "" {
		return Config{}, fmt.Errorf(
			"invalid JWT_EXPIRATION_HOURS: %w",
			err,
		)
	}

	environment := os.Getenv("ENVIRONMENT")
	databaseURL := os.Getenv("DATABASE_URL")

	smtpHost := os.Getenv("SMTP_HOST")
	smtpUsername := os.Getenv("SMTP_USERNAME")
	smtpPassword := os.Getenv("SMTP_PASSWORD")
	smtpFrom := os.Getenv("SMTP_FROM")

	appBaseURL := os.Getenv("APP_BASE_URL")

	jwtSecret := os.Getenv("JWT_SECRET")
	jwtIssuer := os.Getenv("JWT_ISSUER")

	if environment == "" {
		return Config{}, fmt.Errorf(
			"ENVIRONMENT is required",
		)
	}

	if databaseURL == "" {
		return Config{}, fmt.Errorf(
			"DATABASE_URL is required",
		)
	}

	if smtpHost == "" {
		return Config{}, fmt.Errorf(
			"SMTP_HOST is required",
		)
	}

	if smtpFrom == "" {
		return Config{}, fmt.Errorf(
			"SMTP_FROM is required",
		)
	}

	if appBaseURL == "" {
		return Config{}, fmt.Errorf(
			"APP_BASE_URL is required",
		)
	}

	if jwtSecret == "" {
		return Config{}, fmt.Errorf(
			"JWT_SECRET is required",
		)
	}

	// HS256 signs tokens with this secret. A short secret can be brute
	// forced offline from any token an attacker sees, letting them mint
	// tokens for any user. 32 bytes = 256 bits, matching the hash size.
	if len(jwtSecret) < 32 {
		return Config{}, fmt.Errorf(
			"JWT_SECRET must be at least 32 characters",
		)
	}

	if jwtIssuer == "" {
		return Config{}, fmt.Errorf(
			"JWT_ISSUER is required",
		)
	}

	bidLocking := os.Getenv("BID_LOCKING")
	if bidLocking == "" {
		bidLocking = "pessimistic"
	}
	if bidLocking != "pessimistic" && bidLocking != "optimistic" {
		return Config{}, fmt.Errorf(
			"BID_LOCKING must be pessimistic or optimistic",
		)
	}

	// ACCESS_TOKEN_TTL (a Go duration such as "15m") overrides
	// JWT_EXPIRATION_HOURS. Short access tokens limit how long a stolen or
	// no-longer-valid token works; refresh tokens keep users logged in.
	jwtExpiration := time.Duration(jwtExpirationHours) * time.Hour
	if v := os.Getenv("ACCESS_TOKEN_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return Config{}, fmt.Errorf("invalid ACCESS_TOKEN_TTL: %q", v)
		}
		jwtExpiration = d
	}

	refreshTTL := 30 * 24 * time.Hour
	if v := os.Getenv("REFRESH_TOKEN_TTL_HOURS"); v != "" {
		h, err := strconv.Atoi(v)
		if err != nil || h <= 0 {
			return Config{}, fmt.Errorf("invalid REFRESH_TOKEN_TTL_HOURS: %q", v)
		}
		refreshTTL = time.Duration(h) * time.Hour
	}

	rateLimits := os.Getenv("RATE_LIMITS") != "off"

	if jwtExpirationHours <= 0 && os.Getenv("ACCESS_TOKEN_TTL") == "" {
		return Config{}, fmt.Errorf(
			"JWT_EXPIRATION_HOURS must be greater than zero",
		)
	}

	return Config{
		Port:        port,
		Environment: environment,
		DatabaseURL: databaseURL,

		SMTPHost:     smtpHost,
		SMTPPort:     smtpPort,
		SMTPUsername: smtpUsername,
		SMTPPassword: smtpPassword,
		SMTPFrom:     smtpFrom,

		AppBaseURL: appBaseURL,

		JWTSecret:     jwtSecret,
		JWTIssuer:     jwtIssuer,
		JWTExpiration: jwtExpiration,

		BidLocking: bidLocking,

		RefreshTokenTTL:    refreshTTL,
		CORSAllowedOrigins: splitList(os.Getenv("CORS_ALLOWED_ORIGINS")),
		TrustedProxies:     splitList(os.Getenv("TRUSTED_PROXIES")),
		RateLimitsEnabled:  rateLimits,
	}, nil
}

// splitList parses a comma-separated env var, ignoring blanks.
func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
