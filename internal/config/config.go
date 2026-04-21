package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/ilyakaznacheev/cleanenv"
)

const (
	defaultServerPort          = "8080"
	defaultServerHost          = "0.0.0.0"
	defaultDatabasePath        = "./emerald.db"
	defaultAuthUsername        = "admin"
	defaultAuthPassword        = "admin"
	defaultAuthSessionTTLHours = 24
	defaultAuthCookieName      = "emerald_session"
	encryptionKeyLength        = 32
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Auth     AuthConfig
	Security SecurityConfig
}

type ServerConfig struct {
	Port string `env:"EMERALD_PORT"`
	Host string `env:"EMERALD_HOST"`
}

type DatabaseConfig struct {
	Path string `env:"EMERALD_DB_PATH"`
}

type AuthConfig struct {
	Username        string `env:"EMERALD_AUTH_USERNAME"`
	Password        string `env:"EMERALD_AUTH_PASSWORD"`
	SessionTTLHours int    `env:"EMERALD_AUTH_SESSION_TTL_HOURS"`
	SessionTTL      time.Duration
	CookieName      string `env:"EMERALD_AUTH_COOKIE_NAME"`
}

type SecurityConfig struct {
	EncryptionKey          string   `env:"EMERALD_ENCRYPTION_KEY"`
	AllowDBStoredKey       bool     `env:"EMERALD_ALLOW_DB_STORED_KEY"`
	AllowedOrigins         []string `env:"EMERALD_ALLOWED_ORIGINS" env-separator:","`
	TrustProxy             bool     `env:"EMERALD_TRUST_PROXY"`
	TrustedProxies         []string `env:"EMERALD_TRUSTED_PROXIES" env-separator:","`
	AllowPrivateWebTools   bool     `env:"EMERALD_ALLOW_PRIVATE_WEBTOOLS"`
	AllowAbsoluteToolPaths bool     `env:"EMERALD_ALLOW_ABSOLUTE_TOOL_PATHS"`
	AllowPlugins           bool     `env:"EMERALD_ALLOW_PLUGINS"`
}

func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port: defaultServerPort,
			Host: defaultServerHost,
		},
		Database: DatabaseConfig{
			Path: defaultDatabasePath,
		},
		Auth: AuthConfig{
			Username:        defaultAuthUsername,
			Password:        defaultAuthPassword,
			SessionTTLHours: defaultAuthSessionTTLHours,
			CookieName:      defaultAuthCookieName,
		},
		Security: SecurityConfig{
			AllowPlugins: true,
		},
	}
	if err := cleanenv.ReadEnv(cfg); err != nil {
		return nil, fmt.Errorf("read environment config: %w", err)
	}

	cfg.normalize()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("config is required")
	}

	c.normalize()

	return validation.Errors{
		"server":   c.Server.Validate(),
		"database": c.Database.Validate(),
		"auth":     c.Auth.Validate(),
		"security": c.Security.Validate(),
	}.Filter()
}

func (c *Config) normalize() {
	if c == nil {
		return
	}
	c.Auth.normalize()
	c.Security.normalize()
}

func (c ServerConfig) Validate() error {
	return validation.Errors{
		"port": validation.Validate(
			c.Port,
			validation.Required,
			validation.By(validatePort),
		),
		"host": validation.Validate(c.Host, validation.Required),
	}.Filter()
}

func (c DatabaseConfig) Validate() error {
	return validation.Errors{
		"path": validation.Validate(c.Path, validation.Required),
	}.Filter()
}

func (c AuthConfig) Validate() error {
	return validation.Errors{
		"username":        validation.Validate(c.Username, validation.Required),
		"password":        validation.Validate(c.Password, validation.Required),
		"sessionTTLHours": validation.Validate(c.SessionTTLHours, validation.By(validatePositiveSessionTTLHours)),
		"cookieName":      validation.Validate(c.CookieName, validation.Required),
	}.Filter()
}

func (c *AuthConfig) normalize() {
	if c == nil {
		return
	}
	if c.SessionTTLHours > 0 {
		c.SessionTTL = time.Duration(c.SessionTTLHours) * time.Hour
	}
}

func (c *SecurityConfig) normalize() {
	if c == nil {
		return
	}
	c.AllowedOrigins = normalizeCSVValues(c.AllowedOrigins)
	c.TrustedProxies = normalizeCSVValues(c.TrustedProxies)
}

func (c SecurityConfig) Validate() error {
	return validation.Errors{
		"encryptionKey":          validateEncryptionKey(c.EncryptionKey, c.AllowDBStoredKey),
		"allowedOrigins":         validation.Each(validation.By(validateOriginValue)).Validate(c.AllowedOrigins),
		"trustedProxies":         validation.Each(validation.By(validateNonEmptyValue)).Validate(c.TrustedProxies),
		"allowAbsoluteToolPaths": validation.Validate(c.AllowAbsoluteToolPaths),
	}.Filter()
}

func validatePort(value any) error {
	raw, _ := value.(string)

	port, err := strconv.Atoi(raw)
	if err != nil {
		return fmt.Errorf("must be a valid integer")
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("must be between 1 and 65535")
	}

	return nil
}

func validateEncryptionKey(value string, allowDBStoredKey bool) error {
	if value == "" && allowDBStoredKey {
		return nil
	}
	if value == "" {
		return fmt.Errorf("is required unless EMERALD_ALLOW_DB_STORED_KEY is enabled")
	}
	if len(value) != encryptionKeyLength {
		return fmt.Errorf("must be %d bytes when provided", encryptionKeyLength)
	}
	return nil
}

func validatePositiveSessionTTLHours(value any) error {
	hours, _ := value.(int)
	if hours <= 0 {
		return fmt.Errorf("must be greater than zero")
	}
	return nil
}

func validateOriginValue(value any) error {
	raw, _ := value.(string)
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	if trimmed == "*" {
		return fmt.Errorf("wildcard origin is not allowed")
	}
	if !strings.Contains(trimmed, "://") {
		return fmt.Errorf("must include scheme and host")
	}
	return nil
}

func validateNonEmptyValue(value any) error {
	raw, _ := value.(string)
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("must not be empty")
	}
	return nil
}

func normalizeCSVValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized
}
