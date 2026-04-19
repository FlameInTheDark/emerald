package skills

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestStoreLoadsSkillsFromDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	skillDir := filepath.Join(dir, "frontend-design")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}

	content := `---
name: frontend-design
description: Create polished frontend experiences.
---

# Frontend Design
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	store := NewStore(dir, 25*time.Millisecond)
	if err := store.Start(context.Background()); err != nil {
		t.Fatalf("start store: %v", err)
	}
	defer store.Stop()

	summaries := store.List()
	if len(summaries) != 1 {
		t.Fatalf("expected 1 skill summary, got %d", len(summaries))
	}
	if summaries[0].Name != "frontend-design" {
		t.Fatalf("unexpected skill name %q", summaries[0].Name)
	}
	if summaries[0].Description != "Create polished frontend experiences." {
		t.Fatalf("unexpected description %q", summaries[0].Description)
	}

	skill, ok := store.GetByName("frontend-design")
	if !ok {
		t.Fatal("expected to get skill by name")
	}
	if skill.Content == "" {
		t.Fatal("expected skill content to be loaded")
	}
}

func TestParseSkillFileFallsBackToFolderName(t *testing.T) {
	t.Parallel()

	path := filepath.Join("workspace", "skills", "demo", "SKILL.md")
	skill := parseSkillFile(path, []byte("# No front matter"))
	if skill.Name != "demo" {
		t.Fatalf("expected fallback name demo, got %q", skill.Name)
	}
}

func TestResolvingStoreReloadsWhenDirectoryChanges(t *testing.T) {
	t.Parallel()

	dirOne := t.TempDir()
	dirTwo := t.TempDir()
	writeSkillFixture(t, dirOne, "alpha-skill", "First skill")
	writeSkillFixture(t, dirTwo, "beta-skill", "Second skill")

	var dirMu sync.RWMutex
	currentDir := dirOne
	store := NewResolvingStore(func() (string, error) {
		dirMu.RLock()
		defer dirMu.RUnlock()
		return currentDir, nil
	}, 10*time.Millisecond)
	if err := store.Start(context.Background()); err != nil {
		t.Fatalf("start store: %v", err)
	}
	defer store.Stop()

	assertSkillLoaded(t, store, "alpha-skill")

	dirMu.Lock()
	currentDir = dirTwo
	dirMu.Unlock()
	waitForSkill(t, store, "beta-skill")

	if _, ok := store.GetByName("alpha-skill"); ok {
		t.Fatal("expected old skill directory contents to be replaced after resolver switch")
	}
}

func writeSkillFixture(t *testing.T, root string, name string, description string) {
	t.Helper()

	skillDir := filepath.Join(root, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}

	content := `---
name: ` + name + `
description: ` + description + `
---

# ` + name + `
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}

func assertSkillLoaded(t *testing.T, reader Reader, name string) {
	t.Helper()

	if _, ok := reader.GetByName(name); !ok {
		t.Fatalf("expected skill %q to be available", name)
	}
}

func waitForSkill(t *testing.T, reader Reader, name string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := reader.GetByName(name); ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for skill %q to load", name)
}
