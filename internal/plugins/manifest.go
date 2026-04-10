package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Manifest struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Version     string            `json:"version,omitempty"`
	Description string            `json:"description,omitempty"`
	Executable  string            `json:"executable"`
	Args        []string          `json:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Path        string            `json:"-"`
	Dir         string            `json:"-"`
}

func loadManifest(path string) (Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest %s: %w", path, err)
	}

	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest %s: %w", path, err)
	}

	manifest.ID = strings.TrimSpace(manifest.ID)
	manifest.Name = strings.TrimSpace(manifest.Name)
	manifest.Executable = strings.TrimSpace(manifest.Executable)
	manifest.Path = path
	manifest.Dir = filepath.Dir(path)

	if manifest.ID == "" {
		return Manifest{}, fmt.Errorf("manifest %s: id is required", path)
	}
	if manifest.Name == "" {
		manifest.Name = manifest.ID
	}
	if manifest.Executable == "" {
		return Manifest{}, fmt.Errorf("manifest %s: executable is required", path)
	}

	if !filepath.IsAbs(manifest.Executable) {
		manifest.Executable = filepath.Join(manifest.Dir, manifest.Executable)
	}

	resolvedExecutable, err := filepath.Abs(manifest.Executable)
	if err != nil {
		return Manifest{}, fmt.Errorf("resolve executable for %s: %w", manifest.ID, err)
	}
	manifest.Executable = resolvedExecutable

	for key, value := range manifest.Env {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			delete(manifest.Env, key)
			continue
		}
		if trimmedKey != key {
			delete(manifest.Env, key)
			manifest.Env[trimmedKey] = value
		}
	}

	return manifest, nil
}
