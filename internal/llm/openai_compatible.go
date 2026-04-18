package llm

import (
	"net/url"
	"strings"
)

const openAICompatibleChatPath = "/chat/completions"

func resolveOpenAICompatibleBaseURL(rawBaseURL string, providerType ProviderType) string {
	baseURL := strings.TrimSpace(rawBaseURL)
	if baseURL == "" {
		baseURL = DefaultBaseURL(providerType)
	}
	return strings.TrimRight(baseURL, "/")
}

func resolveOpenAICompatibleChatEndpoint(rawBaseURL string, providerType ProviderType) string {
	baseURL := resolveOpenAICompatibleBaseURL(rawBaseURL, providerType)
	if baseURL == "" {
		return ""
	}

	normalizedPath, ok := normalizeOpenAICompatiblePath(baseURL)
	if ok && strings.HasSuffix(normalizedPath, openAICompatibleChatPath) {
		return strings.TrimRight(baseURL, "/")
	}

	return baseURL + openAICompatibleChatPath
}

func resolveOpenAICompatibleModelsEndpoint(rawBaseURL string, providerType ProviderType) string {
	baseURL := resolveOpenAICompatibleBaseURL(rawBaseURL, providerType)
	if baseURL == "" {
		return ""
	}

	parsedBaseURL, normalizedPath, ok := splitOpenAICompatiblePath(baseURL)
	if ok && strings.HasSuffix(normalizedPath, openAICompatibleChatPath) {
		trimmedPath := strings.TrimSuffix(normalizedPath, openAICompatibleChatPath)
		return rebuildOpenAICompatibleURL(parsedBaseURL, trimmedPath+"/models")
	}

	return baseURL + "/models"
}

func resolveLMStudioModelsEndpoint(rawBaseURL string) string {
	baseURL := resolveOpenAICompatibleBaseURL(rawBaseURL, ProviderLMStudio)
	if baseURL == "" {
		return ""
	}

	parsedBaseURL, _, ok := splitOpenAICompatiblePath(baseURL)
	if !ok {
		return ""
	}

	return rebuildOpenAICompatibleURL(parsedBaseURL, "/api/v1/models")
}

func normalizeOpenAICompatiblePath(rawURL string) (string, bool) {
	_, normalizedPath, ok := splitOpenAICompatiblePath(rawURL)
	return normalizedPath, ok
}

func splitOpenAICompatiblePath(rawURL string) (*url.URL, string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, "", false
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, parsed.Path, true
}

func rebuildOpenAICompatibleURL(parsed *url.URL, path string) string {
	cloned := *parsed
	cloned.Path = path
	return strings.TrimRight(cloned.String(), "/")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
