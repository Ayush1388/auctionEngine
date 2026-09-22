package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

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
	if err != nil {
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

	if jwtExpirationHours <= 0 {
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
		JWTExpiration: time.Duration(jwtExpirationHours) * time.Hour,
	}, nil
}
