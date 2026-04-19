package llm

import (
	"testing"

	"github.com/FlameInTheDark/emerald/internal/webtools"
)

func TestToolRegistryRegistersWebToolsWhenConfigIsReady(t *testing.T) {
	t.Parallel()

	registry := NewToolRegistryWithOptions(ToolRegistryOptions{
		WebToolsConfig: &webtools.RuntimeConfig{
			Config: webtools.Config{
				SearchProvider:      webtools.SearchProviderSearXNG,
				PageObservationMode: webtools.PageObservationModeHTTP,
				SearchReady:         true,
				PageReadReady:       true,
			},
		},
	})

	toolNames := make([]string, 0, len(registry.GetAllTools()))
	for _, tool := range registry.GetAllTools() {
		toolNames = append(toolNames, tool.Function.Name)
	}

	if !containsString(toolNames, "search_web") {
		t.Fatalf("expected search_web to be registered, got %v", toolNames)
	}
	if !containsString(toolNames, "open_web_page") {
		t.Fatalf("expected open_web_page to be registered, got %v", toolNames)
	}
}
