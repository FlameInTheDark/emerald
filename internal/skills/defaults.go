package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:generate go run ./cmd/generate-defaults

func EnsureBundledDefaults(skillDir string) error {
	trimmed := strings.TrimSpace(skillDir)
	if trimmed == "" {
		return fmt.Errorf("skills directory is required")
	}

	if err := os.MkdirAll(trimmed, 0o755); err != nil {
		return fmt.Errorf("ensure skills directory: %w", err)
	}

	paths := bundledDefaultPaths()
	for _, relative := range paths {
		destination := filepath.Join(trimmed, filepath.FromSlash(relative))
		if _, err := os.Stat(destination); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat bundled skill destination %s: %w", destination, err)
		}

		content := bundledDefaults[relative]

		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return fmt.Errorf("ensure bundled skill parent %s: %w", destination, err)
		}
		if err := os.WriteFile(destination, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write bundled skill %s: %w", destination, err)
		}
	}

	return nil
}

func bundledDefaultPaths() []string {
	paths := make([]string, 0, len(bundledDefaults))
	for path := range bundledDefaults {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}
