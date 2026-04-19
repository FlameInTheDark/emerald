package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/FlameInTheDark/emerald/internal/db/models"
)

func TestConfigGetCommandRedactsSensitiveValuesByDefault(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	t.Setenv("EMERALD_DB_PATH", filepath.Join(tempDir, "emerald.db"))
	t.Setenv("EMERALD_SKILLS_DIR", filepath.Join(tempDir, "skills"))
	t.Setenv("EMERALD_PLUGINS_DIR", filepath.Join(tempDir, "plugins"))

	runtime, err := newCLIRuntime(ctx, cliRuntimeOptions{migrate: true})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}

	configJSON := `{"headers":{"Authorization":"secret"},"timeoutSeconds":30}`
	provider := &models.LLMProvider{
		Name:         "Primary Provider",
		ProviderType: "openai",
		APIKey:       "super-secret-key",
		Model:        "gpt-test",
		Config:       &configJSON,
	}
	if err := runtime.LLMProviderStore.Create(ctx, provider); err != nil {
		_ = runtime.Close()
		t.Fatalf("create provider: %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("close runtime: %v", err)
	}

	output := &bytes.Buffer{}
	if err := runConfigGetCommand(ctx, "llm_provider", provider.ID, "", false, output, newCLIRuntime); err != nil {
		t.Fatalf("run config get: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}

	config := payload["config"].(map[string]any)
	if _, exists := config["apiKey"]; exists {
		t.Fatalf("apiKey should be redacted: %#v", config)
	}
	if got := config["apiKeyConfigured"]; got != true {
		t.Fatalf("expected apiKeyConfigured=true, got %#v", got)
	}
	if _, exists := config["config"]; exists {
		t.Fatalf("raw config should not be returned by default: %#v", config)
	}
	if _, exists := config["configSummary"]; !exists {
		t.Fatalf("expected configSummary in output: %#v", config)
	}
}

func TestConfigUpdateCommandPreservesSensitiveValuesWhenOmitted(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	t.Setenv("EMERALD_DB_PATH", filepath.Join(tempDir, "emerald.db"))
	t.Setenv("EMERALD_SKILLS_DIR", filepath.Join(tempDir, "skills"))
	t.Setenv("EMERALD_PLUGINS_DIR", filepath.Join(tempDir, "plugins"))

	runtime, err := newCLIRuntime(ctx, cliRuntimeOptions{migrate: true})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}

	cluster := &models.Cluster{
		Name:           "Primary Cluster",
		Host:           "old.example.com",
		Port:           8006,
		APITokenID:     "token-id",
		APITokenSecret: "keep-me",
	}
	if err := runtime.ClusterStore.Create(ctx, cluster); err != nil {
		_ = runtime.Close()
		t.Fatalf("create cluster: %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("close runtime: %v", err)
	}

	output := &bytes.Buffer{}
	if err := runConfigUpdateCommand(
		ctx,
		"proxmox_cluster",
		cluster.ID,
		"",
		`{"host":"new.example.com","port":9000}`,
		"",
		false,
		output,
		newCLIRuntime,
	); err != nil {
		t.Fatalf("run config update: %v", err)
	}

	verifyRuntime, err := newCLIRuntime(ctx, cliRuntimeOptions{migrate: true})
	if err != nil {
		t.Fatalf("reopen runtime: %v", err)
	}
	defer func() {
		_ = verifyRuntime.Close()
	}()

	updatedCluster, err := verifyRuntime.ClusterStore.GetByID(ctx, cluster.ID)
	if err != nil {
		t.Fatalf("load updated cluster: %v", err)
	}
	if updatedCluster.Host != "new.example.com" || updatedCluster.Port != 9000 {
		t.Fatalf("expected host/port update, got %#v", updatedCluster)
	}
	if updatedCluster.APITokenSecret != "keep-me" {
		t.Fatalf("expected token secret to be preserved, got %q", updatedCluster.APITokenSecret)
	}

	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}

	config := payload["config"].(map[string]any)
	if _, exists := config["apiTokenSecret"]; exists {
		t.Fatalf("apiTokenSecret should be redacted: %#v", config)
	}
	if got := config["apiTokenSecretConfigured"]; got != true {
		t.Fatalf("expected apiTokenSecretConfigured=true, got %#v", got)
	}
}
