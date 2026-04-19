package skills

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestEnsureBundledDefaultsWritesMissingSkills(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	skillsDir := filepath.Join(dir, ".agents", "skills")

	if err := EnsureBundledDefaults(skillsDir); err != nil {
		t.Fatalf("EnsureBundledDefaults returned error: %v", err)
	}

	luaPath := filepath.Join(skillsDir, "lua-scripting-guide", "SKILL.md")
	content, err := os.ReadFile(luaPath)
	if err != nil {
		t.Fatalf("read seeded lua skill: %v", err)
	}
	if !strings.Contains(string(content), "The current node input is exposed as a global named `input`.") &&
		!strings.Contains(string(content), "The current node input is exposed as a global named input.") {
		t.Fatalf("seeded lua skill missing input guidance:\n%s", string(content))
	}

	pluginCreatorPath := filepath.Join(skillsDir, "plugin-creator", "SKILL.md")
	pluginCreatorContent, err := os.ReadFile(pluginCreatorPath)
	if err != nil {
		t.Fatalf("read seeded plugin-creator skill: %v", err)
	}
	if !strings.Contains(string(pluginCreatorContent), "pkg/pluginapi") {
		t.Fatalf("seeded plugin-creator skill missing plugin SDK guidance:\n%s", string(pluginCreatorContent))
	}
}

func TestEnsureBundledDefaultsDoesNotOverwriteExistingSkills(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	skillsDir := filepath.Join(dir, ".agents", "skills")
	customPath := filepath.Join(skillsDir, "templating-guide", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(customPath), 0o755); err != nil {
		t.Fatalf("mkdir custom skill dir: %v", err)
	}
	if err := os.WriteFile(customPath, []byte("custom content"), 0o644); err != nil {
		t.Fatalf("write custom skill: %v", err)
	}

	if err := EnsureBundledDefaults(skillsDir); err != nil {
		t.Fatalf("EnsureBundledDefaults returned error: %v", err)
	}

	content, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatalf("read custom skill: %v", err)
	}
	if string(content) != "custom content" {
		t.Fatalf("expected existing skill to be preserved, got %q", string(content))
	}
}

func TestBundledDefaultsMatchRepositorySkills(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	canonicalRoot := filepath.Join(repoRoot, ".agents", "skills")

	canonicalFiles := listSkillFiles(t, canonicalRoot)
	bundledFiles := bundledDefaultPaths()

	if !slices.Equal(canonicalFiles, bundledFiles) {
		t.Fatalf("bundled skill file set does not match repo skills:\ncanonical=%v\nbundled=%v", canonicalFiles, bundledFiles)
	}

	for _, relPath := range canonicalFiles {
		canonicalContent, err := os.ReadFile(filepath.Join(canonicalRoot, relPath))
		if err != nil {
			t.Fatalf("read canonical skill %s: %v", relPath, err)
		}
		bundledContent, ok := bundledDefaults[relPath]
		if !ok {
			t.Fatalf("missing bundled skill %s", relPath)
		}
		if string(canonicalContent) != bundledContent {
			t.Fatalf("bundled skill %s is out of sync with .agents/skills", relPath)
		}
	}
}

func listSkillFiles(t *testing.T, root string) []string {
	t.Helper()

	entries := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.EqualFold(d.Name(), "SKILL.md") {
			return nil
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entries = append(entries, filepath.ToSlash(relPath))
		return nil
	})
	if err != nil {
		t.Fatalf("walk skills under %s: %v", root, err)
	}

	slices.Sort(entries)
	return entries
}
