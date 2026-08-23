package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddress        string
	DatabaseURL        string
	JWTSecret          string
	AccessTTL          time.Duration
	RefreshTTL         time.Duration
	RequestTimeout     time.Duration
	ShutdownTimeout    time.Duration
	FileRoot           string
	MaxUploadBytes     int64
	AllowedOrigins     []string
	OutboxPollInterval time.Duration
	OutboxMaxAttempts  int
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddress:        env("HTTP_ADDRESS", ":8080"),
		DatabaseURL:        env("DATABASE_URL", "postgres://worklog:worklog@localhost:5432/worklog?sslmode=disable"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		AccessTTL:          duration("ACCESS_TTL", 15*time.Minute),
		RefreshTTL:         duration("REFRESH_TTL", 30*24*time.Hour),
		RequestTimeout:     duration("REQUEST_TIMEOUT", 10*time.Second),
		ShutdownTimeout:    duration("SHUTDOWN_TIMEOUT", 15*time.Second),
		FileRoot:           env("FILE_ROOT", "./var/uploads"),
		MaxUploadBytes:     int64Value("MAX_UPLOAD_BYTES", 10<<20),
		AllowedOrigins:     []string{env("WEB_ORIGIN", "http://localhost:5173")},
		OutboxPollInterval: duration("OUTBOX_POLL_INTERVAL", time.Second),
		OutboxMaxAttempts:  intValue("OUTBOX_MAX_ATTEMPTS", 5),
	}
	if len(cfg.JWTSecret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET must contain at least 32 characters")
	}
	if cfg.DatabaseURL == "" || cfg.RequestTimeout <= 0 || cfg.OutboxMaxAttempts < 1 {
		return Config{}, fmt.Errorf("invalid runtime configuration")
	}
	return cfg, nil
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func duration(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func intValue(name string, fallback int) int {
	parsed, err := strconv.Atoi(os.Getenv(name))
	if err != nil {
		return fallback
	}
	return parsed
}

func int64Value(name string, fallback int64) int64 {
	parsed, err := strconv.ParseInt(os.Getenv(name), 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
