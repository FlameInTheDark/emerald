package assistants

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/FlameInTheDark/automator/internal/db/query"
)

type Scope string

const (
	ScopePipelineEditor Scope = "pipeline_editor"
	ScopeChatWindow     Scope = "chat_window"
)

type Module struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

type Profile struct {
	Scope              Scope      `json:"scope"`
	SystemInstructions string     `json:"system_instructions"`
	EnabledModules     []string   `json:"enabled_modules"`
	CreatedAt          *time.Time `json:"created_at,omitempty"`
	UpdatedAt          *time.Time `json:"updated_at,omitempty"`
}

type Store struct {
	configs *query.AppConfigStore
}

type storedProfile struct {
	SystemInstructions string   `json:"system_instructions"`
	EnabledModules     []string `json:"enabled_modules"`
}

var moduleCatalog = []Module{
	{
		ID:          "pipeline_graph_rules",
		Name:        "Pipeline Graph Rules",
		Description: "Validation rules for valid node and edge structures.",
		Content: strings.TrimSpace(`
Pipelines use React Flow style JSON with nodes, edges, and an optional viewport.

Rules:
- Only one logic:return node is allowed in a pipeline.
- Trigger nodes start flows and cannot have incoming normal edges.
- Tool nodes are not part of the main execution chain.
- tool:* nodes can only be connected from an llm:agent node using sourceHandle "tool".
- Never connect a tool node with a normal edge.
- Visual group nodes are layout-only and cannot have incoming or outgoing edges.
- Return nodes cannot have outgoing edges.
- Preserve existing node and edge ids when editing unless there is a strong reason to replace them.
`),
	},
	{
		ID:          "node_catalog",
		Name:        "Node Catalog",
		Description: "Compact reference for the available pipeline node categories.",
		Content: strings.TrimSpace(`
Important node categories:
- Triggers: trigger:manual, trigger:cron, trigger:webhook, trigger:channel_message
- Actions: action:http, action:shell_command, action:lua, action:pipeline_get, action:pipeline_run
- Logic: logic:condition, logic:switch, logic:merge, logic:aggregate, logic:return
- LLM: llm:prompt, llm:agent
- Tools: tool:http, tool:shell_command, tool:pipeline_list, tool:pipeline_get, tool:pipeline_create, tool:pipeline_update, tool:pipeline_delete, tool:pipeline_run
- Infrastructure nodes also exist for Proxmox and Kubernetes
- visual:group is a canvas-only grouping node and does not affect execution

Use labels that clearly describe the node intent.
`),
	},
	{
		ID:          "templating_guide",
		Name:        "Templating Guide",
		Description: "How string interpolation works in node config values.",
		Content: strings.TrimSpace(`
String templates use {{ ... }} expressions.

Examples:
- {{input}}
- {{input.nodes}}
- {{input.nodes[0].status}}

Behavior:
- Templates resolve against the current input object.
- input is always available as the full current payload.
- Top-level input keys are also exposed directly in the template context.
- Templates are rendered recursively in string fields inside node config.
- Missing paths cause template rendering errors.
`),
	},
	{
		ID:          "logic_expression_guide",
		Name:        "Logic Expression Guide",
		Description: "How expressions for condition and switch logic are evaluated.",
		Content: strings.TrimSpace(`
logic:condition and condition-based logic:switch expressions are evaluated with expr-lang.

Behavior:
- The environment includes input as the full payload.
- Top-level input keys are also exposed directly in the expression environment.
- Expressions must evaluate to a boolean value.
- Templating is rendered before expression evaluation.

Examples:
- input.status == "ready"
- retries > 3
- input.cluster == "prod" && input.enabled == true
`),
	},
	{
		ID:          "llm_tool_edge_rules",
		Name:        "LLM Tool Edge Rules",
		Description: "Connection rules specific to llm:agent tool nodes.",
		Content: strings.TrimSpace(`
Tool connection rules:
- Only llm:agent can connect to tool:* nodes.
- The connection must use sourceHandle "tool" on the llm:agent node.
- Tool nodes cannot appear in the main execution chain.
- Normal action or logic nodes must not target tool nodes.
- If you add or move tool nodes, ensure their edges still use the tool handle rule.
`),
	},
}

func NewStore(configs *query.AppConfigStore) *Store {
	return &Store{configs: configs}
}

func DefaultProfile(scope Scope) Profile {
	switch scope {
	case ScopePipelineEditor:
		return Profile{
			Scope: scope,
			SystemInstructions: strings.TrimSpace(`
You are the AI assistant embedded inside the Automator node editor.
Help the user understand and modify the current pipeline safely.
Treat the browser-provided pipeline snapshot as the source of truth because it may include unsaved edits that are not in the database.
Prefer precise, minimal changes and preserve existing node and edge ids whenever possible.
When the user asks for something that requires the other mode, explicitly tell them to switch modes instead of pretending the action succeeded.
`),
			EnabledModules: []string{
				"pipeline_graph_rules",
				"node_catalog",
				"templating_guide",
				"logic_expression_guide",
				"llm_tool_edge_rules",
			},
		}
	case ScopeChatWindow:
		return Profile{
			Scope: scope,
			SystemInstructions: strings.TrimSpace(`
You are an automation assistant for infrastructure and Automator pipelines.
Use the available tools to manage enabled integrations, inspect local skills, run shell commands when appropriate, and create, edit, run, activate, or deactivate pipelines when the user asks.
`),
			EnabledModules: []string{},
		}
	default:
		return Profile{
			Scope:              scope,
			SystemInstructions: "",
			EnabledModules:     []string{},
		}
	}
}

func ListModules() []Module {
	modules := make([]Module, len(moduleCatalog))
	copy(modules, moduleCatalog)
	return modules
}

func ResolveModules(ids []string) []Module {
	normalized := NormalizeEnabledModules(ids)
	modules := make([]Module, 0, len(normalized))
	for _, id := range normalized {
		if module, ok := moduleByID(id); ok {
			modules = append(modules, module)
		}
	}
	return modules
}

func BuildPromptAppendix(profile Profile) string {
	sections := make([]string, 0, 1+len(profile.EnabledModules))

	if instructions := strings.TrimSpace(profile.SystemInstructions); instructions != "" {
		sections = append(sections, instructions)
	}

	for _, module := range ResolveModules(profile.EnabledModules) {
		sections = append(sections, strings.TrimSpace(module.Name+":\n"+module.Content))
	}

	return strings.TrimSpace(strings.Join(sections, "\n\n"))
}

func (s *Store) Get(ctx context.Context, scope Scope) (Profile, error) {
	if err := ValidateScope(scope); err != nil {
		return Profile{}, err
	}

	profile := DefaultProfile(scope)
	if s == nil || s.configs == nil {
		return profile, nil
	}

	config, err := s.configs.Get(ctx, configKey(scope))
	if err != nil {
		return Profile{}, fmt.Errorf("load assistant profile %s: %w", scope, err)
	}
	if config == nil || strings.TrimSpace(config.Value) == "" {
		return profile, nil
	}

	var stored storedProfile
	if err := json.Unmarshal([]byte(config.Value), &stored); err != nil {
		return Profile{}, fmt.Errorf("decode assistant profile %s: %w", scope, err)
	}

	if instructions := strings.TrimSpace(stored.SystemInstructions); instructions != "" {
		profile.SystemInstructions = instructions
	}
	profile.EnabledModules = NormalizeEnabledModules(stored.EnabledModules)
	profile.CreatedAt = &config.CreatedAt
	profile.UpdatedAt = &config.UpdatedAt

	return profile, nil
}

func (s *Store) Set(ctx context.Context, scope Scope, profile Profile) (Profile, error) {
	if err := ValidateScope(scope); err != nil {
		return Profile{}, err
	}
	if s == nil || s.configs == nil {
		return Profile{}, fmt.Errorf("assistant profile store is not configured")
	}

	payload := storedProfile{
		SystemInstructions: strings.TrimSpace(profile.SystemInstructions),
		EnabledModules:     NormalizeEnabledModules(profile.EnabledModules),
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return Profile{}, fmt.Errorf("encode assistant profile %s: %w", scope, err)
	}
	if err := s.configs.Set(ctx, configKey(scope), string(encoded)); err != nil {
		return Profile{}, fmt.Errorf("store assistant profile %s: %w", scope, err)
	}

	return s.Get(ctx, scope)
}

func (s *Store) RestoreDefaults(ctx context.Context, scope Scope) (Profile, error) {
	return s.Set(ctx, scope, DefaultProfile(scope))
}

func NormalizeEnabledModules(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(ids))
	normalized := make([]string, 0, len(ids))
	for _, id := range ids {
		key := strings.TrimSpace(id)
		if key == "" {
			continue
		}
		if _, ok := moduleByID(key); !ok {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, key)
	}

	if len(normalized) == 0 {
		return nil
	}

	return normalized
}

func ValidateScope(scope Scope) error {
	switch scope {
	case ScopePipelineEditor, ScopeChatWindow:
		return nil
	default:
		return fmt.Errorf("unsupported assistant profile scope %q", scope)
	}
}

func moduleByID(id string) (Module, bool) {
	for _, module := range moduleCatalog {
		if module.ID == id {
			return module, true
		}
	}
	return Module{}, false
}

func configKey(scope Scope) string {
	return "assistant_profile:" + string(scope)
}
