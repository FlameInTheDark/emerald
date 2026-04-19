package webtools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/db/query"
)

const appConfigKey = "web_tools_config"

type SearchProvider string

const (
	SearchProviderDisabled SearchProvider = "disabled"
	SearchProviderSearXNG  SearchProvider = "searxng"
	SearchProviderJina     SearchProvider = "jina"
)

type PageObservationMode string

const (
	PageObservationModeHTTP PageObservationMode = "http"
	PageObservationModeJina PageObservationMode = "jina"
)

type Config struct {
	SearchProvider       SearchProvider      `json:"search_provider"`
	PageObservationMode  PageObservationMode `json:"page_observation_mode"`
	SearXNGBaseURL       string              `json:"searxng_base_url"`
	JinaSearchBaseURL    string              `json:"jina_search_base_url"`
	JinaReaderBaseURL    string              `json:"jina_reader_base_url"`
	JinaAPIKeySecretName string              `json:"jina_api_key_secret_name,omitempty"`
	SearchReady          bool                `json:"search_ready"`
	PageReadReady        bool                `json:"page_read_ready"`
	Warnings             []string            `json:"warnings"`
}

type RuntimeConfig struct {
	Config
	JinaAPIKey string `json:"-"`
}

type Store struct {
	configs *query.AppConfigStore
	secrets *query.SecretStore
}

type storedConfig struct {
	SearchProvider       SearchProvider      `json:"search_provider"`
	PageObservationMode  PageObservationMode `json:"page_observation_mode"`
	SearXNGBaseURL       string              `json:"searxng_base_url"`
	JinaSearchBaseURL    string              `json:"jina_search_base_url"`
	JinaReaderBaseURL    string              `json:"jina_reader_base_url"`
	JinaAPIKeySecretName string              `json:"jina_api_key_secret_name,omitempty"`
}

func NewStore(configs *query.AppConfigStore, secrets *query.SecretStore) *Store {
	return &Store{configs: configs, secrets: secrets}
}

func DefaultConfig() Config {
	return Config{
		SearchProvider:      SearchProviderDisabled,
		PageObservationMode: PageObservationModeHTTP,
		SearXNGBaseURL:      "http://localhost:8080",
		JinaSearchBaseURL:   "https://s.jina.ai",
		JinaReaderBaseURL:   "https://r.jina.ai",
		Warnings:            []string{},
	}
}

func (s *Store) Get(ctx context.Context) (Config, error) {
	cfg := DefaultConfig()
	if s == nil || s.configs == nil {
		return cfg, nil
	}

	record, err := s.configs.Get(ctx, appConfigKey)
	if err != nil {
		return Config{}, fmt.Errorf("load web tools config: %w", err)
	}
	if record == nil || strings.TrimSpace(record.Value) == "" {
		return s.decorate(ctx, cfg)
	}

	var stored storedConfig
	if err := json.Unmarshal([]byte(record.Value), &stored); err != nil {
		return Config{}, fmt.Errorf("decode web tools config: %w", err)
	}

	cfg = normalizeConfig(Config{
		SearchProvider:       stored.SearchProvider,
		PageObservationMode:  stored.PageObservationMode,
		SearXNGBaseURL:       stored.SearXNGBaseURL,
		JinaSearchBaseURL:    stored.JinaSearchBaseURL,
		JinaReaderBaseURL:    stored.JinaReaderBaseURL,
		JinaAPIKeySecretName: stored.JinaAPIKeySecretName,
	})

	return s.decorate(ctx, cfg)
}

func (s *Store) Set(ctx context.Context, cfg Config) (Config, error) {
	if s == nil || s.configs == nil {
		return Config{}, fmt.Errorf("web tools config store is not configured")
	}

	normalized := normalizeConfig(cfg)
	if err := validateConfig(normalized); err != nil {
		return Config{}, err
	}

	encoded, err := json.Marshal(storedConfig{
		SearchProvider:       normalized.SearchProvider,
		PageObservationMode:  normalized.PageObservationMode,
		SearXNGBaseURL:       normalized.SearXNGBaseURL,
		JinaSearchBaseURL:    normalized.JinaSearchBaseURL,
		JinaReaderBaseURL:    normalized.JinaReaderBaseURL,
		JinaAPIKeySecretName: normalized.JinaAPIKeySecretName,
	})
	if err != nil {
		return Config{}, fmt.Errorf("encode web tools config: %w", err)
	}

	if err := s.configs.Set(ctx, appConfigKey, string(encoded)); err != nil {
		return Config{}, fmt.Errorf("store web tools config: %w", err)
	}

	return s.Get(ctx)
}

func (s *Store) Resolve(ctx context.Context) (RuntimeConfig, error) {
	cfg, err := s.Get(ctx)
	if err != nil {
		return RuntimeConfig{}, err
	}

	runtime := RuntimeConfig{Config: cfg}
	if strings.TrimSpace(cfg.JinaAPIKeySecretName) == "" || s == nil || s.secrets == nil {
		return runtime, nil
	}

	value, ok, err := s.secrets.GetValueByName(ctx, cfg.JinaAPIKeySecretName)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("load Jina API key secret: %w", err)
	}
	if ok {
		runtime.JinaAPIKey = value
	}

	return runtime, nil
}

func normalizeConfig(cfg Config) Config {
	defaults := DefaultConfig()

	cfg.SearchProvider = normalizeSearchProvider(cfg.SearchProvider)
	cfg.PageObservationMode = normalizePageObservationMode(cfg.PageObservationMode)
	cfg.SearXNGBaseURL = normalizeBaseURL(defaults.SearXNGBaseURL, cfg.SearXNGBaseURL)
	cfg.JinaSearchBaseURL = normalizeBaseURL(defaults.JinaSearchBaseURL, cfg.JinaSearchBaseURL)
	cfg.JinaReaderBaseURL = normalizeBaseURL(defaults.JinaReaderBaseURL, cfg.JinaReaderBaseURL)
	cfg.JinaAPIKeySecretName = strings.TrimSpace(cfg.JinaAPIKeySecretName)
	cfg.SearchReady = false
	cfg.PageReadReady = false
	cfg.Warnings = []string{}

	return cfg
}

func normalizeSearchProvider(value SearchProvider) SearchProvider {
	switch SearchProvider(strings.ToLower(strings.TrimSpace(string(value)))) {
	case SearchProviderDisabled, SearchProviderSearXNG, SearchProviderJina:
		return SearchProvider(strings.ToLower(strings.TrimSpace(string(value))))
	default:
		return SearchProviderDisabled
	}
}

func normalizePageObservationMode(value PageObservationMode) PageObservationMode {
	switch PageObservationMode(strings.ToLower(strings.TrimSpace(string(value)))) {
	case PageObservationModeHTTP, PageObservationModeJina:
		return PageObservationMode(strings.ToLower(strings.TrimSpace(string(value))))
	default:
		return PageObservationModeHTTP
	}
}

func normalizeBaseURL(defaultValue string, value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return defaultValue
	}
	return strings.TrimRight(trimmed, "/")
}

func validateConfig(cfg Config) error {
	if err := validateHTTPURL("SearXNG base URL", cfg.SearXNGBaseURL); err != nil {
		return err
	}
	if err := validateHTTPURL("Jina search base URL", cfg.JinaSearchBaseURL); err != nil {
		return err
	}
	if err := validateHTTPURL("Jina reader base URL", cfg.JinaReaderBaseURL); err != nil {
		return err
	}
	return nil
}

func validateHTTPURL(label string, value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", label, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s must use http or https", label)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return fmt.Errorf("%s must include a host", label)
	}
	return nil
}

func (s *Store) decorate(ctx context.Context, cfg Config) (Config, error) {
	cfg = normalizeConfig(cfg)
	searchReady, pageReady, warnings, err := s.assess(ctx, cfg)
	if err != nil {
		return Config{}, err
	}
	cfg.SearchReady = searchReady
	cfg.PageReadReady = pageReady
	cfg.Warnings = warnings
	return cfg, nil
}

func (s *Store) assess(ctx context.Context, cfg Config) (bool, bool, []string, error) {
	warnings := make([]string, 0, 2)

	searchReady := false
	switch cfg.SearchProvider {
	case SearchProviderDisabled:
	case SearchProviderSearXNG:
		searchReady = strings.TrimSpace(cfg.SearXNGBaseURL) != ""
		if !searchReady {
			warnings = append(warnings, "SearXNG search is selected, but the base URL is empty.")
		}
	case SearchProviderJina:
		if strings.TrimSpace(cfg.JinaSearchBaseURL) == "" {
			warnings = append(warnings, "Jina search is selected, but the search base URL is empty.")
			break
		}
		if strings.TrimSpace(cfg.JinaAPIKeySecretName) == "" {
			warnings = append(warnings, "Jina search requires an API key secret. Create one in Secrets and select it here.")
			break
		}
		if s == nil || s.secrets == nil {
			warnings = append(warnings, "Jina search requires secret storage, but the secret store is not configured.")
			break
		}
		_, ok, err := s.secrets.GetValueByName(ctx, cfg.JinaAPIKeySecretName)
		if err != nil {
			return false, false, nil, fmt.Errorf("load Jina API key secret: %w", err)
		}
		if !ok {
			warnings = append(warnings, fmt.Sprintf("The selected Jina API key secret %q was not found.", cfg.JinaAPIKeySecretName))
			break
		}
		searchReady = true
	}

	pageReady := false
	switch cfg.PageObservationMode {
	case PageObservationModeHTTP:
		pageReady = true
	case PageObservationModeJina:
		pageReady = strings.TrimSpace(cfg.JinaReaderBaseURL) != ""
		if !pageReady {
			warnings = append(warnings, "Jina page reading is selected, but the reader base URL is empty.")
		}
		if strings.TrimSpace(cfg.JinaAPIKeySecretName) != "" && s != nil && s.secrets != nil {
			if _, ok, err := s.secrets.GetValueByName(ctx, cfg.JinaAPIKeySecretName); err != nil {
				return false, false, nil, fmt.Errorf("load Jina API key secret: %w", err)
			} else if !ok {
				warnings = append(warnings, fmt.Sprintf("The selected Jina API key secret %q was not found.", cfg.JinaAPIKeySecretName))
			}
		}
	}

	return searchReady, pageReady, dedupeWarnings(warnings), nil
}

func dedupeWarnings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}

	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return []string{}
	}
	return out
}
