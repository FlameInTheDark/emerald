package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDirectoryFindsNearestWorkspaceAgentsDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	skillsDir := filepath.Join(root, ".agents", "skills", "pipeline-builder")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("mkdir skills dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte("# Pipeline Builder"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	hint := filepath.Join(root, "cmd", "server")
	if err := os.MkdirAll(hint, 0o755); err != nil {
		t.Fatalf("mkdir hint dir: %v", err)
	}

	resolved, err := ResolveDirectory("", hint)
	if err != nil {
		t.Fatalf("ResolveDirectory: %v", err)
	}

	want := filepath.Join(root, ".agents", "skills")
	if resolved != want {
		t.Fatalf("resolved = %q, want %q", resolved, want)
	}
}

func TestResolveDirectoryFallsBackToHintRelativeAgentsDirectory(t *testing.T) {
	t.Parallel()

	hint := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(hint, 0o755); err != nil {
		t.Fatalf("mkdir hint dir: %v", err)
	}

	resolved, err := ResolveDirectory("", hint)
	if err != nil {
		t.Fatalf("ResolveDirectory: %v", err)
	}

	want := filepath.Join(hint, ".agents", "skills")
	if resolved != want {
		t.Fatalf("resolved = %q, want %q", resolved, want)
	}
}

func TestResolveDirectoryUsesExplicitPathWhenProvided(t *testing.T) {
	t.Parallel()

	explicit := filepath.Join(t.TempDir(), "custom-skills")
	resolved, err := ResolveDirectory(explicit)
	if err != nil {
		t.Fatalf("ResolveDirectory: %v", err)
	}

	if resolved != explicit {
		t.Fatalf("resolved = %q, want %q", resolved, explicit)
	}
}
