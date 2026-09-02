package config

import (
	"errors"
	"os"
	"strconv"
)

type Config struct {
	Port        int
	Environment string
	DatabaseURL string
}

func Load() (Config, error) {
	port, err := strconv.Atoi(getEnv("PORT", "4000"))
	if err != nil {
		return Config{}, errors.New("invalid PORT")
	}

	cfg := Config{
		Port:        port,
		Environment: getEnv("ENVIRONMENT", "development"),
		DatabaseURL: getEnv("DATABASE_URL", ""),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	return cfg, nil
}

func getEnv(key string, fallback string) string {
	value, exists := os.LookupEnv(key)

	if !exists {
		return fallback
	}

	return value
}
