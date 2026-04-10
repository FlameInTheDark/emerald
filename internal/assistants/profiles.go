package assistants

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/FlameInTheDark/emerald/internal/db/query"
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
	SkillName   string `json:"skill_name"`
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
		Description: "Validation rules for legal node and edge structures, branch handles, and safe live edits.",
		SkillName:   "pipeline-graph-rules",
	},
	{
		ID:          "node_catalog",
		Name:        "Node Catalog",
		Description: "Compact reference for important node families, common roles, and when to use each category.",
		SkillName:   "node-catalog",
	},
	{
		ID:          "templating_guide",
		Name:        "Templating Guide",
		Description: "How {{template}} interpolation resolves runtime data, pipeline params, and arguments.<name> values.",
		SkillName:   "templating-guide",
	},
	{
		ID:          "lua_scripting_guide",
		Name:        "Lua Scripting Guide",
		Description: "How action:lua uses the global input value, Lua table conversion, and return-value mapping.",
		SkillName:   "lua-scripting-guide",
	},
	{
		ID:          "logic_expression_guide",
		Name:        "Logic Expression Guide",
		Description: "Rules and examples for expr-based condition and switch logic.",
		SkillName:   "logic-expression-guide",
	},
	{
		ID:          "llm_tool_edge_rules",
		Name:        "LLM Tool Edge Rules",
		Description: "Connection rules for llm:agent tool nodes and the dedicated tool handle.",
		SkillName:   "llm-tool-edge-rules",
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
You are the AI assistant embedded inside the Emerald node editor.
Help the user understand and modify the current pipeline safely.
Treat the browser-provided pipeline snapshot as the source of truth because it may include unsaved edits that are not in the database.
Prefer precise, minimal changes and preserve existing node and edge ids whenever possible.
When the user asks for something that requires the other mode, explicitly tell them to switch modes instead of pretending the action succeeded.
`),
			EnabledModules: preferredEnabledModules(scope),
		}
	case ScopeChatWindow:
		return Profile{
			Scope: scope,
			SystemInstructions: strings.TrimSpace(`
You are an automation assistant for infrastructure and Emerald pipelines.
Use the available tools to manage enabled integrations, inspect local skills, run shell commands when appropriate, and create, edit, run, activate, or deactivate pipelines when the user asks.
`),
			EnabledModules: preferredEnabledModules(scope),
		}
	default:
		return Profile{
			Scope:              scope,
			SystemInstructions: "",
			EnabledModules:     preferredEnabledModules(scope),
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

	if skillSection := BuildEnabledSkillSection(profile.EnabledModules); skillSection != "" {
		sections = append(sections, skillSection)
	}

	return strings.TrimSpace(strings.Join(sections, "\n\n"))
}

func BuildEnabledSkillSection(ids []string) string {
	modules := ResolveModules(ids)
	if len(modules) == 0 {
		return ""
	}

	lines := make([]string, 0, len(modules)+1)
	lines = append(lines, "Preferred assistant skills:")
	for _, module := range modules {
		name := strings.TrimSpace(module.SkillName)
		if name == "" {
			name = module.ID
		}
		line := "- " + name
		if description := strings.TrimSpace(module.Description); description != "" {
			line += ": " + description
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func SkillNamesForModules(ids []string) []string {
	modules := ResolveModules(ids)
	names := make([]string, 0, len(modules))
	seen := make(map[string]struct{}, len(modules))
	for _, module := range modules {
		name := strings.TrimSpace(module.SkillName)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
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
	profile.EnabledModules = preferredEnabledModules(scope)
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
		EnabledModules:     preferredEnabledModules(scope),
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

func preferredEnabledModules(scope Scope) []string {
	var modules []string

	switch scope {
	case ScopePipelineEditor:
		modules = []string{
			"pipeline_graph_rules",
			"node_catalog",
			"templating_guide",
			"lua_scripting_guide",
			"logic_expression_guide",
			"llm_tool_edge_rules",
		}
	case ScopeChatWindow:
		modules = []string{}
	default:
		modules = []string{}
	}

	if len(modules) == 0 {
		return nil
	}

	out := make([]string, len(modules))
	copy(out, modules)
	return out
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
