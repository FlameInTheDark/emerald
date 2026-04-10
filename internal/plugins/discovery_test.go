package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestResolveDirectoryFindsNearestAgentsPluginsDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pluginsDir := filepath.Join(root, ".agents", "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll plugins dir: %v", err)
	}

	workspace := filepath.Join(root, "workspaces", "team-a", "project")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll workspace: %v", err)
	}

	resolved, err := ResolveDirectory("", workspace)
	if err != nil {
		t.Fatalf("ResolveDirectory: %v", err)
	}
	if resolved != pluginsDir {
		t.Fatalf("resolved plugins dir = %q, want %q", resolved, pluginsDir)
	}
}

func TestDiscoverManifestPathsSkipsHiddenDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	visibleManifest := filepath.Join(root, "visible-plugin", manifestFilename)
	nestedManifest := filepath.Join(root, "nested", "plugin", manifestFilename)
	hiddenManifest := filepath.Join(root, ".hidden-plugin", manifestFilename)

	for _, manifestPath := range []string{visibleManifest, nestedManifest, hiddenManifest} {
		if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %v", manifestPath, err)
		}
		if err := os.WriteFile(manifestPath, []byte(`{}`), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", manifestPath, err)
		}
	}

	manifests, err := discoverManifestPaths(root)
	if err != nil {
		t.Fatalf("discoverManifestPaths: %v", err)
	}

	sort.Strings(manifests)
	expected := []string{nestedManifest, visibleManifest}
	sort.Strings(expected)

	if len(manifests) != len(expected) {
		t.Fatalf("manifest count = %d, want %d (%#v)", len(manifests), len(expected), manifests)
	}
	for idx := range expected {
		if manifests[idx] != expected[idx] {
			t.Fatalf("manifest[%d] = %q, want %q", idx, manifests[idx], expected[idx])
		}
	}
}

func TestLoadManifestResolvesRelativeExecutableAndNormalizesEnv(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifestPath := filepath.Join(root, manifestFilename)

	payload := map[string]any{
		"id":         "acme-plugin",
		"executable": filepath.Join("bin", "plugin.exe"),
		"env": map[string]string{
			" TOKEN ": "value",
			"":        "drop-me",
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	manifest, err := loadManifest(manifestPath)
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}

	if manifest.Name != "acme-plugin" {
		t.Fatalf("manifest name = %q, want acme-plugin", manifest.Name)
	}
	if !filepath.IsAbs(manifest.Executable) {
		t.Fatalf("expected executable to be absolute, got %q", manifest.Executable)
	}
	if want := filepath.Join(root, "bin", "plugin.exe"); manifest.Executable != want {
		t.Fatalf("manifest executable = %q, want %q", manifest.Executable, want)
	}
	if _, exists := manifest.Env[""]; exists {
		t.Fatalf("expected blank env key to be removed: %#v", manifest.Env)
	}
	if got := manifest.Env["TOKEN"]; got != "value" {
		t.Fatalf("normalized env value = %q, want value", got)
	}
}
