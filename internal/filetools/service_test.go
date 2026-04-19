package filetools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceRejectsEscapingPaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	service, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	testCases := []struct {
		name string
		run  func() error
	}{
		{
			name: "absolute path",
			run: func() error {
				_, err := service.ReadFile(filepath.Join(root, "outside.txt"), 1, 20)
				return err
			},
		},
		{
			name: "traversal path",
			run: func() error {
				_, err := service.ListDirectory("../")
				return err
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.run()
			if err == nil {
				t.Fatal("expected path validation error")
			}
		})
	}
}

func TestServiceReadFileReturnsLineWindow(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("one\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatalf("WriteFile seed: %v", err)
	}

	service, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	result, err := service.ReadFile("notes.txt", 2, 2)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}

	if result.StartLine != 2 || result.EndLine != 3 {
		t.Fatalf("unexpected line window: %+v", result)
	}
	if result.TotalLines != 4 {
		t.Fatalf("TotalLines = %d, want 4", result.TotalLines)
	}
	if !strings.Contains(result.Content, "2: two") || !strings.Contains(result.Content, "3: three") {
		t.Fatalf("unexpected content: %q", result.Content)
	}
	if !result.Truncated {
		t.Fatal("expected truncated result")
	}
}

func TestServiceGlobAndGrepFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	srcDir := filepath.Join(root, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile main.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "helper.txt"), []byte("needle here\n"), 0o644); err != nil {
		t.Fatalf("WriteFile helper.txt: %v", err)
	}

	service, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	globResult, err := service.GlobFiles("**/*.go", "src")
	if err != nil {
		t.Fatalf("GlobFiles returned error: %v", err)
	}
	if len(globResult.Matches) != 1 || globResult.Matches[0] != "src/main.go" {
		t.Fatalf("unexpected glob matches: %+v", globResult.Matches)
	}

	grepResult, err := service.GrepFiles("needle", "src", "*.txt")
	if err != nil {
		t.Fatalf("GrepFiles returned error: %v", err)
	}
	if len(grepResult.Matches) != 1 {
		t.Fatalf("unexpected grep matches: %+v", grepResult.Matches)
	}
	if grepResult.Matches[0].Path != "src/helper.txt" || grepResult.Matches[0].Line != 1 {
		t.Fatalf("unexpected grep match: %+v", grepResult.Matches[0])
	}
}

func TestServiceEditFileProducesDiffAndAmbiguityErrors(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "app.txt")
	if err := os.WriteFile(target, []byte("alpha\nbeta\nbeta\n"), 0o644); err != nil {
		t.Fatalf("WriteFile seed: %v", err)
	}

	service, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if _, err := service.EditFile("app.txt", "beta", "gamma", false); err == nil || !strings.Contains(err.Error(), "matched 2 times") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}

	result, err := service.EditFile("app.txt", "alpha", "omega", false)
	if err != nil {
		t.Fatalf("EditFile returned error: %v", err)
	}
	if result.Operation != "updated" || !result.Changed {
		t.Fatalf("unexpected edit result: %+v", result)
	}
	if result.Additions != 1 || result.Deletions != 1 {
		t.Fatalf("unexpected diff stats: %+v", result)
	}
	if !strings.Contains(result.Diff, "-alpha") || !strings.Contains(result.Diff, "+omega") {
		t.Fatalf("unexpected diff: %q", result.Diff)
	}
}

func TestServiceWriteFileCreatesAndUpdatesFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	service, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	createResult, err := service.WriteFile("nested/new.txt", "hello\nworld\n")
	if err != nil {
		t.Fatalf("WriteFile create returned error: %v", err)
	}
	if createResult.Operation != "created" || !createResult.Changed {
		t.Fatalf("unexpected create result: %+v", createResult)
	}
	if !strings.Contains(createResult.Diff, "+hello") {
		t.Fatalf("expected create diff, got %q", createResult.Diff)
	}

	updateResult, err := service.WriteFile("nested/new.txt", "hello\nemerald\n")
	if err != nil {
		t.Fatalf("WriteFile update returned error: %v", err)
	}
	if updateResult.Operation != "updated" {
		t.Fatalf("unexpected update result: %+v", updateResult)
	}
	if !strings.Contains(updateResult.Diff, "-world") || !strings.Contains(updateResult.Diff, "+emerald") {
		t.Fatalf("expected update diff, got %q", updateResult.Diff)
	}
}
