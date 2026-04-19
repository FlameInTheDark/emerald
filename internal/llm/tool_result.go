package llm

// ToolResultDisplayKind describes how a tool result should be rendered in chat.
type ToolResultDisplayKind string

const (
	ToolResultDisplayGeneric       ToolResultDisplayKind = "generic"
	ToolResultDisplayListDirectory ToolResultDisplayKind = "list_directory"
	ToolResultDisplayGlob          ToolResultDisplayKind = "glob"
	ToolResultDisplayGrep          ToolResultDisplayKind = "grep"
	ToolResultDisplayRead          ToolResultDisplayKind = "read"
	ToolResultDisplayDiff          ToolResultDisplayKind = "diff"
)

// ToolResultDisplay carries optional rendering metadata for tool transcript rows.
type ToolResultDisplay struct {
	Kind      ToolResultDisplayKind `json:"kind"`
	Title     string                `json:"title,omitempty"`
	Path      string                `json:"path,omitempty"`
	Summary   string                `json:"summary,omitempty"`
	Preview   string                `json:"preview,omitempty"`
	Diff      string                `json:"diff,omitempty"`
	Truncated bool                  `json:"truncated,omitempty"`
	Stats     map[string]any        `json:"stats,omitempty"`
}

// ToolExecutionResult wraps a raw tool result with optional chat display metadata.
type ToolExecutionResult struct {
	Result  any                `json:"result,omitempty"`
	Display *ToolResultDisplay `json:"display,omitempty"`
}

func normalizeToolExecutionResult(value any) (any, *ToolResultDisplay) {
	switch typed := value.(type) {
	case ToolExecutionResult:
		return typed.Result, typed.Display
	case *ToolExecutionResult:
		if typed == nil {
			return nil, nil
		}
		return typed.Result, typed.Display
	default:
		return value, nil
	}
}
