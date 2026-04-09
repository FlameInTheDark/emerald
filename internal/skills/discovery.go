package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveDirectory returns the workspace skill directory.
// It prefers an explicit directory when provided, otherwise it walks upward
// from each hint until it finds a parent that contains a .agents directory.
func ResolveDirectory(explicit string, hints ...string) (string, error) {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		abs, err := filepath.Abs(trimmed)
		if err != nil {
			return "", fmt.Errorf("resolve explicit skills directory: %w", err)
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
	case isSkillsDirectoryPath(baseDir):
		return baseDir, nil
	case strings.EqualFold(filepath.Base(baseDir), ".agents"):
		return filepath.Join(baseDir, "skills"), nil
	default:
		return filepath.Join(baseDir, ".agents", "skills"), nil
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
	homeDir, _ := userHomeDirectory()

	switch {
	case isSkillsDirectoryPath(dir):
		return dir, true, nil
	case strings.EqualFold(filepath.Base(dir), ".agents"):
		return filepath.Join(dir, "skills"), true, nil
	}

	for current := dir; ; {
		if homeDir != "" && current != dir && pathsEqual(current, homeDir) {
			break
		}

		agentsDir := filepath.Join(current, ".agents")
		if info, err := os.Stat(agentsDir); err == nil && info.IsDir() {
			return filepath.Join(agentsDir, "skills"), true, nil
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
		return "", fmt.Errorf("resolve skills directory hint %q: %w", hint, err)
	}

	info, err := os.Stat(abs)
	if err == nil && !info.IsDir() {
		return filepath.Dir(abs), nil
	}

	return abs, nil
}

func isSkillsDirectoryPath(path string) bool {
	return strings.EqualFold(filepath.Base(path), "skills") && strings.EqualFold(filepath.Base(filepath.Dir(path)), ".agents")
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
