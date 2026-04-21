package handlers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/auth"
	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
	"github.com/FlameInTheDark/emerald/internal/llm"
)

func (h *LLMChatHandler) toolAuditLogger(session auth.Session) llm.ToolAuditLogger {
	if h == nil || h.auditLogStore == nil {
		return nil
	}

	return func(ctx context.Context, entry llm.ToolAuditEntry) error {
		actorID := session.UserID
		resourceType := "llm_tool"
		resourceID := strings.TrimSpace(entry.Name)

		auditEntry := models.AuditLog{
			ActorType:    "user",
			ActorID:      &actorID,
			Action:       "llm.tool.execute",
			ResourceType: &resourceType,
			ResourceID:   &resourceID,
			Details:      query.MarshalAuditDetails(sanitizeToolAuditEntry(entry)),
		}

		return h.auditLogStore.Create(ctx, auditEntry)
	}
}

func sanitizeToolAuditEntry(entry llm.ToolAuditEntry) map[string]any {
	details := map[string]any{
		"tool":        entry.Name,
		"success":     entry.Success,
		"duration_ms": entry.Duration.Milliseconds(),
	}
	if entry.Error != "" {
		details["error"] = entry.Error
	}

	switch strings.TrimSpace(entry.Name) {
	case "run_shell_command":
		var payload struct {
			Command          string `json:"command"`
			WorkingDirectory string `json:"working_directory"`
			TimeoutSeconds   int    `json:"timeout_seconds"`
		}
		if json.Unmarshal(entry.Args, &payload) == nil {
			details["command"] = payload.Command
			details["working_directory"] = payload.WorkingDirectory
			details["timeout_seconds"] = payload.TimeoutSeconds
		}
	case "list_directory":
		var payload struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(entry.Args, &payload) == nil {
			details["path"] = payload.Path
		}
	case "glob_files":
		var payload struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
		}
		if json.Unmarshal(entry.Args, &payload) == nil {
			details["pattern"] = payload.Pattern
			details["path"] = payload.Path
		}
	case "grep_files":
		var payload struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
			Include string `json:"include"`
		}
		if json.Unmarshal(entry.Args, &payload) == nil {
			details["pattern"] = payload.Pattern
			details["path"] = payload.Path
			details["include"] = payload.Include
		}
	case "read_file":
		var payload struct {
			Path   string `json:"path"`
			Offset int    `json:"offset"`
			Limit  int    `json:"limit"`
		}
		if json.Unmarshal(entry.Args, &payload) == nil {
			details["path"] = payload.Path
			details["offset"] = payload.Offset
			details["limit"] = payload.Limit
		}
	case "edit_file":
		var payload struct {
			Path       string `json:"path"`
			OldString  string `json:"old_string"`
			NewString  string `json:"new_string"`
			ReplaceAll bool   `json:"replace_all"`
		}
		if json.Unmarshal(entry.Args, &payload) == nil {
			details["path"] = payload.Path
			details["replace_all"] = payload.ReplaceAll
			details["old_length"] = len(payload.OldString)
			details["new_length"] = len(payload.NewString)
		}
	case "write_file":
		var payload struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if json.Unmarshal(entry.Args, &payload) == nil {
			details["path"] = payload.Path
			details["content_length"] = len(payload.Content)
		}
	case "search_web":
		var payload struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		if json.Unmarshal(entry.Args, &payload) == nil {
			details["query"] = payload.Query
			details["limit"] = payload.Limit
		}
	case "open_web_page":
		var payload struct {
			URL           string `json:"url"`
			Mode          string `json:"mode"`
			MaxCharacters int    `json:"max_characters"`
		}
		if json.Unmarshal(entry.Args, &payload) == nil {
			details["url"] = payload.URL
			details["mode"] = payload.Mode
			details["max_characters"] = payload.MaxCharacters
		}
	}

	return details
}
