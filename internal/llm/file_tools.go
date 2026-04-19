package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/filetools"
)

const toolPreviewLines = 20

func (r *ToolRegistry) registerFileTools() {
	if r.fileTools == nil {
		return
	}

	r.Register(listDirectoryToolDefinition(), func(_ context.Context, args json.RawMessage) (any, error) {
		var payload struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(args, &payload); err != nil {
			return nil, fmt.Errorf("parse tool args: %w", err)
		}

		result, err := r.fileTools.ListDirectory(payload.Path)
		if err != nil {
			return nil, err
		}
		return ToolExecutionResult{
			Result:  result,
			Display: buildListDirectoryDisplay(result),
		}, nil
	})

	r.Register(globFilesToolDefinition(), func(_ context.Context, args json.RawMessage) (any, error) {
		var payload struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
		}
		if err := json.Unmarshal(args, &payload); err != nil {
			return nil, fmt.Errorf("parse tool args: %w", err)
		}

		result, err := r.fileTools.GlobFiles(payload.Pattern, payload.Path)
		if err != nil {
			return nil, err
		}
		return ToolExecutionResult{
			Result:  result,
			Display: buildGlobDisplay(result),
		}, nil
	})

	r.Register(grepFilesToolDefinition(), func(_ context.Context, args json.RawMessage) (any, error) {
		var payload struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
			Include string `json:"include"`
		}
		if err := json.Unmarshal(args, &payload); err != nil {
			return nil, fmt.Errorf("parse tool args: %w", err)
		}

		result, err := r.fileTools.GrepFiles(payload.Pattern, payload.Path, payload.Include)
		if err != nil {
			return nil, err
		}
		return ToolExecutionResult{
			Result:  result,
			Display: buildGrepDisplay(result),
		}, nil
	})

	r.Register(readFileToolDefinition(), func(_ context.Context, args json.RawMessage) (any, error) {
		var payload struct {
			Path   string `json:"path"`
			Offset int    `json:"offset"`
			Limit  int    `json:"limit"`
		}
		if err := json.Unmarshal(args, &payload); err != nil {
			return nil, fmt.Errorf("parse tool args: %w", err)
		}

		result, err := r.fileTools.ReadFile(payload.Path, payload.Offset, payload.Limit)
		if err != nil {
			return nil, err
		}
		return ToolExecutionResult{
			Result:  result,
			Display: buildReadDisplay(result),
		}, nil
	})

	r.Register(editFileToolDefinition(), func(_ context.Context, args json.RawMessage) (any, error) {
		var payload struct {
			Path       string `json:"path"`
			OldString  string `json:"old_string"`
			NewString  string `json:"new_string"`
			ReplaceAll bool   `json:"replace_all"`
		}
		if err := json.Unmarshal(args, &payload); err != nil {
			return nil, fmt.Errorf("parse tool args: %w", err)
		}

		result, err := r.fileTools.EditFile(payload.Path, payload.OldString, payload.NewString, payload.ReplaceAll)
		if err != nil {
			return nil, err
		}
		return ToolExecutionResult{
			Result:  result,
			Display: buildFileChangeDisplay(result),
		}, nil
	})

	r.Register(writeFileToolDefinition(), func(_ context.Context, args json.RawMessage) (any, error) {
		var payload struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(args, &payload); err != nil {
			return nil, fmt.Errorf("parse tool args: %w", err)
		}

		result, err := r.fileTools.WriteFile(payload.Path, payload.Content)
		if err != nil {
			return nil, err
		}
		return ToolExecutionResult{
			Result:  result,
			Display: buildFileChangeDisplay(result),
		}, nil
	})
}

func listDirectoryToolDefinition() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "list_directory",
			Description: "List files and folders inside the local workspace. Use this to explore a directory before reading or editing files.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Optional workspace-relative directory path. Omit it to list the workspace root.",
					},
				},
			},
		},
	}
}

func globFilesToolDefinition() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "glob_files",
			Description: "Find workspace files by glob pattern. Use this to discover candidate files before reading or editing them.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{
						"type":        "string",
						"description": "Glob pattern, for example `**/*.go` or `web/src/**/*.tsx`.",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "Optional workspace-relative directory to search inside. Omit it to search from the workspace root.",
					},
				},
				"required": []string{"pattern"},
			},
		},
	}
}

func grepFilesToolDefinition() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "grep_files",
			Description: "Search workspace file contents with a regular expression. Use this to find functions, strings, or code patterns before editing.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{
						"type":        "string",
						"description": "Regular expression to search for.",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "Optional workspace-relative file or directory to search inside. Omit it to search from the workspace root.",
					},
					"include": map[string]any{
						"type":        "string",
						"description": "Optional glob filter such as `*.go` or `**/*.tsx`.",
					},
				},
				"required": []string{"pattern"},
			},
		},
	}
}

func readFileToolDefinition() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "read_file",
			Description: "Read a text file from the local workspace with line numbers. Use this when you need exact code before editing.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Workspace-relative path to the file.",
					},
					"offset": map[string]any{
						"type":        "integer",
						"description": "Optional 1-indexed starting line number.",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Optional maximum number of lines to return.",
					},
				},
				"required": []string{"path"},
			},
		},
	}
}

func editFileToolDefinition() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "edit_file",
			Description: "Replace an exact text snippet inside a workspace file and return the applied diff. Prefer this over shell edits for normal code changes.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Workspace-relative path to the file.",
					},
					"old_string": map[string]any{
						"type":        "string",
						"description": "Existing exact text to replace. It must match the file contents.",
					},
					"new_string": map[string]any{
						"type":        "string",
						"description": "Replacement text. It must be different from old_string.",
					},
					"replace_all": map[string]any{
						"type":        "boolean",
						"description": "Optional flag to replace every match. Omit it or set false to require a single unambiguous match.",
					},
				},
				"required": []string{"path", "old_string", "new_string"},
			},
		},
	}
}

func writeFileToolDefinition() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "write_file",
			Description: "Create or fully replace a workspace file and return the resulting diff. Use this for new files or full rewrites.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Workspace-relative file path to create or replace.",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Complete file contents to write.",
					},
				},
				"required": []string{"path", "content"},
			},
		},
	}
}

func buildListDirectoryDisplay(result *filetools.ListDirectoryResult) *ToolResultDisplay {
	if result == nil {
		return nil
	}

	preview := make([]string, 0, minInt(len(result.Entries), toolPreviewLines))
	for _, entry := range result.Entries[:minInt(len(result.Entries), toolPreviewLines)] {
		line := entry.Name
		if entry.Type == "directory" {
			line += "/"
		}
		preview = append(preview, line)
	}

	return &ToolResultDisplay{
		Kind:      ToolResultDisplayListDirectory,
		Title:     result.Path,
		Path:      result.Path,
		Summary:   summarizeCount("entry", result.Count, result.Truncated),
		Preview:   strings.Join(preview, "\n"),
		Truncated: result.Truncated,
		Stats: map[string]any{
			"count": result.Count,
		},
	}
}

func buildGlobDisplay(result *filetools.GlobFilesResult) *ToolResultDisplay {
	if result == nil {
		return nil
	}

	return &ToolResultDisplay{
		Kind:      ToolResultDisplayGlob,
		Title:     result.BasePath,
		Path:      result.BasePath,
		Summary:   fmt.Sprintf("%s for %q", summarizeCount("match", result.Count, result.Truncated), result.Pattern),
		Preview:   strings.Join(result.Matches[:minInt(len(result.Matches), toolPreviewLines)], "\n"),
		Truncated: result.Truncated,
		Stats: map[string]any{
			"count":   result.Count,
			"pattern": result.Pattern,
		},
	}
}

func buildGrepDisplay(result *filetools.GrepFilesResult) *ToolResultDisplay {
	if result == nil {
		return nil
	}

	lines := make([]string, 0, minInt(len(result.Matches), toolPreviewLines))
	for _, match := range result.Matches[:minInt(len(result.Matches), toolPreviewLines)] {
		lines = append(lines, fmt.Sprintf("%s:%d: %s", match.Path, match.Line, match.Text))
	}

	stats := map[string]any{
		"count":   result.Count,
		"pattern": result.Pattern,
	}
	if strings.TrimSpace(result.Include) != "" {
		stats["include"] = result.Include
	}

	return &ToolResultDisplay{
		Kind:      ToolResultDisplayGrep,
		Title:     result.BasePath,
		Path:      result.BasePath,
		Summary:   fmt.Sprintf("%s for /%s/", summarizeCount("match", result.Count, result.Truncated), result.Pattern),
		Preview:   strings.Join(lines, "\n"),
		Truncated: result.Truncated,
		Stats:     stats,
	}
}

func buildReadDisplay(result *filetools.ReadFileResult) *ToolResultDisplay {
	if result == nil {
		return nil
	}

	return &ToolResultDisplay{
		Kind:      ToolResultDisplayRead,
		Title:     result.Path,
		Path:      result.Path,
		Summary:   fmt.Sprintf("Lines %d-%d of %d", result.StartLine, result.EndLine, result.TotalLines),
		Preview:   result.Content,
		Truncated: result.Truncated,
		Stats: map[string]any{
			"start_line":  result.StartLine,
			"end_line":    result.EndLine,
			"total_lines": result.TotalLines,
		},
	}
}

func buildFileChangeDisplay(result *filetools.FileChangeResult) *ToolResultDisplay {
	if result == nil {
		return nil
	}

	summary := titleWord(result.Operation)
	if !result.Changed {
		summary = "Already up to date"
	}

	return &ToolResultDisplay{
		Kind:    ToolResultDisplayDiff,
		Title:   result.Path,
		Path:    result.Path,
		Summary: fmt.Sprintf("%s (+%d -%d)", summary, result.Additions, result.Deletions),
		Diff:    result.Diff,
		Stats: map[string]any{
			"operation": result.Operation,
			"additions": result.Additions,
			"deletions": result.Deletions,
			"changed":   result.Changed,
		},
	}
}

func summarizeCount(label string, count int, truncated bool) string {
	summary := fmt.Sprintf("%d %s", count, label)
	if count != 1 {
		summary += "es"
	}
	if label == "entry" && count != 1 {
		summary = fmt.Sprintf("%d entries", count)
	}
	if label == "match" && count != 1 {
		summary = fmt.Sprintf("%d matches", count)
	}
	if truncated {
		summary += " shown"
	}
	return summary
}

func titleWord(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func previewFirstLines(value string, limit int) string {
	if limit <= 0 {
		return value
	}

	lines := strings.Split(value, "\n")
	if len(lines) <= limit {
		return value
	}
	return strings.Join(lines[:limit], "\n")
}
