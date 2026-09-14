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
}

func Load() (Config, error) {
	port, err := strconv.Atoi(os.Getenv("PORT"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid PORT: %w", err)
	}

	if port <= 0 {
		return Config{}, fmt.Errorf("PORT must be greater than 0")
	}

	environment := os.Getenv("ENVIRONMENT")
	databaseURL := os.Getenv("DATABASE_URL")

	if environment == "" {
		return Config{}, fmt.Errorf("ENVIRONMENT is required")
	}

	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	return Config{
		Port:        port,
		Environment: environment,
		DatabaseURL: databaseURL,
	}, nil
}
