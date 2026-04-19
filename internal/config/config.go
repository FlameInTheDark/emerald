package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server        ServerConfig
	Database      DatabaseConfig
	Auth          AuthConfig
	EncryptionKey string
}

type ServerConfig struct {
	Port string
	Host string
}

type DatabaseConfig struct {
	Path string
}

type AuthConfig struct {
	Username   string
	Password   string
	SessionTTL time.Duration
	CookieName string
}

func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port: getEnv("EMERALD_PORT", "8080"),
			Host: getEnv("EMERALD_HOST", "0.0.0.0"),
		},
		Database: DatabaseConfig{
			Path: getEnv("EMERALD_DB_PATH", "./emerald.db"),
		},
		Auth: AuthConfig{
			Username:   getEnv("EMERALD_AUTH_USERNAME", "admin"),
			Password:   getEnv("EMERALD_AUTH_PASSWORD", "admin"),
			SessionTTL: time.Duration(getEnvInt("EMERALD_AUTH_SESSION_TTL_HOURS", 24)) * time.Hour,
			CookieName: getEnv("EMERALD_AUTH_COOKIE_NAME", "emerald_session"),
		},
		EncryptionKey: getEnv("EMERALD_ENCRYPTION_KEY", ""),
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.Server.Port == "" {
		return fmt.Errorf("server port is required")
	}
	if c.Database.Path == "" {
		return fmt.Errorf("database path is required")
	}
	if c.Auth.Username == "" {
		return fmt.Errorf("auth username is required")
	}
	if c.Auth.Password == "" {
		return fmt.Errorf("auth password is required")
	}
	if c.Auth.SessionTTL <= 0 {
		return fmt.Errorf("auth session ttl must be greater than zero")
	}
	if c.Auth.CookieName == "" {
		return fmt.Errorf("auth cookie name is required")
	}
	if c.EncryptionKey != "" && len(c.EncryptionKey) != 32 {
		return fmt.Errorf("encryption key seed must be 32 bytes when provided")
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}

	return value
}
