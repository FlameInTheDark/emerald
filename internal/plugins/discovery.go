package plugins

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const manifestFilename = "plugin.json"

// ResolveDirectory returns the plugin root directory. It prefers an explicit
// value and otherwise walks upward from the provided hints until it finds
// a parent that contains a .agents directory.
func ResolveDirectory(explicit string, hints ...string) (string, error) {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		abs, err := filepath.Abs(trimmed)
		if err != nil {
			return "", fmt.Errorf("resolve explicit plugins directory: %w", err)
		}
		return abs, nil
	}

	for _, hint := range hints {
		dir, found, err := resolveDirectoryFromHint(hint)
		if err != nil {
			return "", err
		}
		if found {
			return dir, nil
		}
	}

	baseHint := "."
	for _, hint := range hints {
		if strings.TrimSpace(hint) != "" {
			baseHint = hint
			break
		}
	}

	baseDir, err := normalizeHintDirectory(baseHint)
	if err != nil {
		return "", err
	}

	switch {
	case isPluginsDirectoryPath(baseDir):
		return baseDir, nil
	case strings.EqualFold(filepath.Base(baseDir), ".agents"):
		return filepath.Join(baseDir, "plugins"), nil
	default:
		return filepath.Join(baseDir, ".agents", "plugins"), nil
	}
}

func resolveDirectoryFromHint(hint string) (string, bool, error) {
	if strings.TrimSpace(hint) == "" {
		return "", false, nil
	}

	dir, err := normalizeHintDirectory(hint)
	if err != nil {
		return "", false, err
	}

	switch {
	case isPluginsDirectoryPath(dir):
		return dir, true, nil
	case strings.EqualFold(filepath.Base(dir), ".agents"):
		return filepath.Join(dir, "plugins"), true, nil
	}

	homeDir, _ := userHomeDirectory()
	for current := dir; ; {
		if homeDir != "" && current != dir && pathsEqual(current, homeDir) {
			break
		}

		agentsDir := filepath.Join(current, ".agents")
		if info, err := os.Stat(agentsDir); err == nil && info.IsDir() {
			return filepath.Join(agentsDir, "plugins"), true, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return "", false, nil
}

func normalizeHintDirectory(hint string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(hint))
	if err != nil {
		return "", fmt.Errorf("resolve plugins directory hint %q: %w", hint, err)
	}

	info, err := os.Stat(abs)
	if err == nil && !info.IsDir() {
		return filepath.Dir(abs), nil
	}

	return abs, nil
}

func isPluginsDirectoryPath(path string) bool {
	return strings.EqualFold(filepath.Base(path), "plugins") && strings.EqualFold(filepath.Base(filepath.Dir(path)), ".agents")
}

func userHomeDirectory() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Abs(homeDir)
}

func pathsEqual(left string, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func discoverManifestPaths(root string) ([]string, error) {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return nil, nil
	}

	info, err := os.Stat(trimmed)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat plugins directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("plugins directory %q is not a directory", trimmed)
	}

	manifests := make([]string, 0)
	err = filepath.WalkDir(trimmed, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			base := entry.Name()
			if strings.HasPrefix(base, ".") && !strings.EqualFold(path, trimmed) {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.EqualFold(entry.Name(), manifestFilename) {
			manifests = append(manifests, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk plugins directory: %w", err)
	}

	return manifests, nil
}
