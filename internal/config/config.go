package config

import (
	"fmt"
	"os"
	"strconv"
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
}

func Load() (Config, error) {
	port, err := strconv.Atoi(os.Getenv("PORT"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid PORT: %w", err)
	}

	smtpPort, err := strconv.Atoi(os.Getenv("SMTP_PORT"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid SMTP_PORT: %w", err)
	}

	environment := os.Getenv("ENVIRONMENT")
	databaseURL := os.Getenv("DATABASE_URL")

	if environment == "" {
		return Config{}, fmt.Errorf("ENVIRONMENT is required")
	}

	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	smtpHost := os.Getenv("SMTP_HOST")
	smtpUsername := os.Getenv("SMTP_USERNAME")
	smtpPassword := os.Getenv("SMTP_PASSWORD")
	smtpFrom := os.Getenv("SMTP_FROM")
	appBaseURL := os.Getenv("APP_BASE_URL")

	if smtpHost == "" {
		return Config{}, fmt.Errorf("SMTP_HOST is required")
	}

	if smtpFrom == "" {
		return Config{}, fmt.Errorf("SMTP_FROM is required")
	}

	if appBaseURL == "" {
		return Config{}, fmt.Errorf("APP_BASE_URL is required")
	}

	return Config{
		Port:         port,
		Environment:  environment,
		DatabaseURL:  databaseURL,
		SMTPHost:     smtpHost,
		SMTPPort:     smtpPort,
		SMTPUsername: smtpUsername,
		SMTPPassword: smtpPassword,
		SMTPFrom:     smtpFrom,
		AppBaseURL:   appBaseURL,
	}, nil
}
