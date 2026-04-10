package llm

import (
	"encoding/json"
	"fmt"

	"github.com/FlameInTheDark/emerald/internal/db/models"
)

func ConfigFromModel(provider *models.LLMProvider) (Config, error) {
	if provider == nil {
		return Config{}, fmt.Errorf("provider is required")
	}

	config := Config{
		Name:         provider.Name,
		ProviderType: ProviderType(provider.ProviderType),
		APIKey:       provider.APIKey,
		BaseURL:      stringValue(provider.BaseURL),
		Model:        provider.Model,
	}

	if provider.Config != nil && *provider.Config != "" {
		extraConfig := make(map[string]any)
		if err := json.Unmarshal([]byte(*provider.Config), &extraConfig); err != nil {
			return Config{}, fmt.Errorf("parse provider config: %w", err)
		}
		config.ExtraConfig = extraConfig
	}

	return config, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
