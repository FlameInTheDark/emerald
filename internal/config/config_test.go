package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

var configEnvKeys = []string{
	"EMERALD_PORT",
	"EMERALD_HOST",
	"EMERALD_DB_PATH",
	"EMERALD_AUTH_USERNAME",
	"EMERALD_AUTH_PASSWORD",
	"EMERALD_AUTH_SESSION_TTL_HOURS",
	"EMERALD_AUTH_COOKIE_NAME",
	"EMERALD_ENCRYPTION_KEY",
	"EMERALD_ALLOW_DB_STORED_KEY",
	"EMERALD_ALLOWED_ORIGINS",
	"EMERALD_TRUST_PROXY",
	"EMERALD_TRUSTED_PROXIES",
	"EMERALD_ALLOW_PRIVATE_WEBTOOLS",
	"EMERALD_ALLOW_ABSOLUTE_TOOL_PATHS",
	"EMERALD_ALLOW_PLUGINS",
}

func TestLoad(t *testing.T) {
	t.Run("requires an encryption key by default", func(t *testing.T) {
		unsetConfigEnv(t)

		_, err := Load()
		if err == nil {
			t.Fatal("Load() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "encryptionKey") {
			t.Fatalf("Load() error = %v, want encryption key validation error", err)
		}
	})

	t.Run("uses defaults when DB-stored key compatibility is enabled", func(t *testing.T) {
		unsetConfigEnv(t)
		t.Setenv("EMERALD_ALLOW_DB_STORED_KEY", "true")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.Server.Port != defaultServerPort {
			t.Fatalf("Server.Port = %q, want %q", cfg.Server.Port, defaultServerPort)
		}
		if cfg.Server.Host != defaultServerHost {
			t.Fatalf("Server.Host = %q, want %q", cfg.Server.Host, defaultServerHost)
		}
		if cfg.Database.Path != defaultDatabasePath {
			t.Fatalf("Database.Path = %q, want %q", cfg.Database.Path, defaultDatabasePath)
		}
		if cfg.Auth.Username != defaultAuthUsername {
			t.Fatalf("Auth.Username = %q, want %q", cfg.Auth.Username, defaultAuthUsername)
		}
		if cfg.Auth.Password != defaultAuthPassword {
			t.Fatalf("Auth.Password = %q, want %q", cfg.Auth.Password, defaultAuthPassword)
		}
		if cfg.Auth.SessionTTLHours != defaultAuthSessionTTLHours {
			t.Fatalf("Auth.SessionTTLHours = %d, want %d", cfg.Auth.SessionTTLHours, defaultAuthSessionTTLHours)
		}
		if cfg.Auth.SessionTTL != 24*time.Hour {
			t.Fatalf("Auth.SessionTTL = %s, want %s", cfg.Auth.SessionTTL, 24*time.Hour)
		}
		if cfg.Auth.CookieName != defaultAuthCookieName {
			t.Fatalf("Auth.CookieName = %q, want %q", cfg.Auth.CookieName, defaultAuthCookieName)
		}
		if cfg.Security.EncryptionKey != "" {
			t.Fatalf("Security.EncryptionKey = %q, want empty", cfg.Security.EncryptionKey)
		}
		if !cfg.Security.AllowDBStoredKey {
			t.Fatal("Security.AllowDBStoredKey = false, want true")
		}
	})

	t.Run("applies env overrides", func(t *testing.T) {
		unsetConfigEnv(t)
		t.Setenv("EMERALD_PORT", "9090")
		t.Setenv("EMERALD_HOST", "127.0.0.1")
		t.Setenv("EMERALD_DB_PATH", "/tmp/emerald.db")
		t.Setenv("EMERALD_AUTH_USERNAME", "operator")
		t.Setenv("EMERALD_AUTH_PASSWORD", "secret")
		t.Setenv("EMERALD_AUTH_SESSION_TTL_HOURS", "72")
		t.Setenv("EMERALD_AUTH_COOKIE_NAME", "emerald_auth")
		t.Setenv("EMERALD_ENCRYPTION_KEY", strings.Repeat("k", encryptionKeyLength))
		t.Setenv("EMERALD_ALLOWED_ORIGINS", "https://app.example.com,https://admin.example.com")
		t.Setenv("EMERALD_TRUST_PROXY", "true")
		t.Setenv("EMERALD_TRUSTED_PROXIES", "127.0.0.1,10.0.0.0/8")
		t.Setenv("EMERALD_ALLOW_PRIVATE_WEBTOOLS", "true")
		t.Setenv("EMERALD_ALLOW_ABSOLUTE_TOOL_PATHS", "true")
		t.Setenv("EMERALD_ALLOW_PLUGINS", "false")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.Server.Port != "9090" {
			t.Fatalf("Server.Port = %q, want %q", cfg.Server.Port, "9090")
		}
		if cfg.Server.Host != "127.0.0.1" {
			t.Fatalf("Server.Host = %q, want %q", cfg.Server.Host, "127.0.0.1")
		}
		if cfg.Database.Path != "/tmp/emerald.db" {
			t.Fatalf("Database.Path = %q, want %q", cfg.Database.Path, "/tmp/emerald.db")
		}
		if cfg.Auth.Username != "operator" {
			t.Fatalf("Auth.Username = %q, want %q", cfg.Auth.Username, "operator")
		}
		if cfg.Auth.Password != "secret" {
			t.Fatalf("Auth.Password = %q, want %q", cfg.Auth.Password, "secret")
		}
		if cfg.Auth.SessionTTLHours != 72 {
			t.Fatalf("Auth.SessionTTLHours = %d, want %d", cfg.Auth.SessionTTLHours, 72)
		}
		if cfg.Auth.SessionTTL != 72*time.Hour {
			t.Fatalf("Auth.SessionTTL = %s, want %s", cfg.Auth.SessionTTL, 72*time.Hour)
		}
		if cfg.Auth.CookieName != "emerald_auth" {
			t.Fatalf("Auth.CookieName = %q, want %q", cfg.Auth.CookieName, "emerald_auth")
		}
		if cfg.Security.EncryptionKey != strings.Repeat("k", encryptionKeyLength) {
			t.Fatalf("Security.EncryptionKey = %q, want overridden value", cfg.Security.EncryptionKey)
		}
		if len(cfg.Security.AllowedOrigins) != 2 {
			t.Fatalf("AllowedOrigins length = %d, want 2", len(cfg.Security.AllowedOrigins))
		}
		if !cfg.Security.TrustProxy {
			t.Fatal("Security.TrustProxy = false, want true")
		}
		if len(cfg.Security.TrustedProxies) != 2 {
			t.Fatalf("TrustedProxies length = %d, want 2", len(cfg.Security.TrustedProxies))
		}
		if !cfg.Security.AllowPrivateWebTools {
			t.Fatal("Security.AllowPrivateWebTools = false, want true")
		}
		if !cfg.Security.AllowAbsoluteToolPaths {
			t.Fatal("Security.AllowAbsoluteToolPaths = false, want true")
		}
		if cfg.Security.AllowPlugins {
			t.Fatal("Security.AllowPlugins = true, want false")
		}
	})

	t.Run("rejects invalid port", func(t *testing.T) {
		unsetConfigEnv(t)
		t.Setenv("EMERALD_PORT", "70000")
		t.Setenv("EMERALD_ALLOW_DB_STORED_KEY", "true")

		_, err := Load()
		if err == nil {
			t.Fatal("Load() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "port") {
			t.Fatalf("Load() error = %v, want port validation error", err)
		}
	})

	t.Run("rejects invalid session ttl", func(t *testing.T) {
		unsetConfigEnv(t)
		t.Setenv("EMERALD_AUTH_SESSION_TTL_HOURS", "0")
		t.Setenv("EMERALD_ALLOW_DB_STORED_KEY", "true")

		_, err := Load()
		if err == nil {
			t.Fatal("Load() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "sessionTTLHours") {
			t.Fatalf("Load() error = %v, want session ttl validation error", err)
		}
	})

	t.Run("rejects malformed session ttl value", func(t *testing.T) {
		unsetConfigEnv(t)
		t.Setenv("EMERALD_AUTH_SESSION_TTL_HOURS", "abc")
		t.Setenv("EMERALD_ALLOW_DB_STORED_KEY", "true")

		_, err := Load()
		if err == nil {
			t.Fatal("Load() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "EMERALD_AUTH_SESSION_TTL_HOURS") {
			t.Fatalf("Load() error = %v, want cleanenv parse error", err)
		}
	})

	t.Run("rejects invalid encryption key length", func(t *testing.T) {
		unsetConfigEnv(t)
		t.Setenv("EMERALD_ENCRYPTION_KEY", "short")

		_, err := Load()
		if err == nil {
			t.Fatal("Load() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "encryptionKey") {
			t.Fatalf("Load() error = %v, want encryption key validation error", err)
		}
	})
}

func unsetConfigEnv(t *testing.T) {
	t.Helper()

	snapshot := make(map[string]*string, len(configEnvKeys))
	for _, key := range configEnvKeys {
		if value, ok := os.LookupEnv(key); ok {
			valueCopy := value
			snapshot[key] = &valueCopy
		}
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("Unsetenv(%q): %v", key, err)
		}
	}

	t.Cleanup(func() {
		for _, key := range configEnvKeys {
			value, ok := snapshot[key]
			switch {
			case !ok || value == nil:
				_ = os.Unsetenv(key)
			default:
				_ = os.Setenv(key, *value)
			}
		}
	})
}
