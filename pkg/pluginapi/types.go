package pluginapi

import (
	"context"
	"encoding/json"
)

const APIVersion = "v1"

type NodeKind string

const (
	NodeKindAction NodeKind = "action"
	NodeKindTool   NodeKind = "tool"
)

type FieldType string

const (
	FieldTypeString   FieldType = "string"
	FieldTypeTextarea FieldType = "textarea"
	FieldTypeNumber   FieldType = "number"
	FieldTypeBoolean  FieldType = "boolean"
	FieldTypeSelect   FieldType = "select"
	FieldTypeJSON     FieldType = "json"
)

type FieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type FieldSpec struct {
	Name               string        `json:"name"`
	Label              string        `json:"label"`
	Description        string        `json:"description,omitempty"`
	Type               FieldType     `json:"type"`
	Required           bool          `json:"required,omitempty"`
	Placeholder        string        `json:"placeholder,omitempty"`
	TemplateSupported  bool          `json:"template_supported,omitempty"`
	Options            []FieldOption `json:"options,omitempty"`
	DefaultStringValue string        `json:"default_string_value,omitempty"`
	DefaultBoolValue   *bool         `json:"default_bool_value,omitempty"`
	DefaultNumberValue *float64      `json:"default_number_value,omitempty"`
}

type OutputHandle struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Color string `json:"color,omitempty"`
}

type OutputHint struct {
	Expression  string `json:"expression"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type NodeSpec struct {
	ID            string                 `json:"id"`
	Kind          NodeKind               `json:"kind"`
	Label         string                 `json:"label"`
	Description   string                 `json:"description,omitempty"`
	Icon          string                 `json:"icon,omitempty"`
	Color         string                 `json:"color,omitempty"`
	MenuPath      []string               `json:"menu_path,omitempty"`
	DefaultConfig map[string]any         `json:"default_config,omitempty"`
	Fields        []FieldSpec            `json:"fields,omitempty"`
	Outputs       []OutputHandle         `json:"outputs,omitempty"`
	OutputHints   []OutputHint           `json:"output_hints,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

type PluginInfo struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Version    string     `json:"version"`
	APIVersion string     `json:"api_version"`
	Nodes      []NodeSpec `json:"nodes"`
}

type ToolNodeMetadata struct {
	NodeID string `json:"node_id"`
	Label  string `json:"label"`
}

type ToolDefinition struct {
	Type     string   `json:"type"`
	Function ToolSpec `json:"function"`
}

type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type Plugin interface {
	Describe(ctx context.Context) (PluginInfo, error)
	ValidateConfig(ctx context.Context, nodeID string, config json.RawMessage) error
	ExecuteAction(ctx context.Context, nodeID string, config json.RawMessage, input map[string]any) (any, error)
	ToolDefinition(ctx context.Context, nodeID string, meta ToolNodeMetadata, config json.RawMessage) (*ToolDefinition, error)
	ExecuteTool(ctx context.Context, nodeID string, config json.RawMessage, args json.RawMessage, input map[string]any) (any, error)
}

type ActionNode interface {
	ValidateConfig(ctx context.Context, config json.RawMessage) error
	Execute(ctx context.Context, config json.RawMessage, input map[string]any) (any, error)
}

type ToolNode interface {
	ValidateConfig(ctx context.Context, config json.RawMessage) error
	ToolDefinition(ctx context.Context, meta ToolNodeMetadata, config json.RawMessage) (*ToolDefinition, error)
	ExecuteTool(ctx context.Context, config json.RawMessage, args json.RawMessage, input map[string]any) (any, error)
}

type Bundle struct {
	Info    PluginInfo
	Actions map[string]ActionNode
	Tools   map[string]ToolNode
}

func (b *Bundle) Describe(_ context.Context) (PluginInfo, error) {
	return b.Info, nil
}

func (b *Bundle) ValidateConfig(ctx context.Context, nodeID string, config json.RawMessage) error {
	if action, ok := b.Actions[nodeID]; ok {
		return action.ValidateConfig(ctx, config)
	}
	if tool, ok := b.Tools[nodeID]; ok {
		return tool.ValidateConfig(ctx, config)
	}
	return ErrUnknownNode(nodeID)
}

func (b *Bundle) ExecuteAction(ctx context.Context, nodeID string, config json.RawMessage, input map[string]any) (any, error) {
	action, ok := b.Actions[nodeID]
	if !ok {
		return nil, ErrUnknownNode(nodeID)
	}
	return action.Execute(ctx, config, input)
}

func (b *Bundle) ToolDefinition(ctx context.Context, nodeID string, meta ToolNodeMetadata, config json.RawMessage) (*ToolDefinition, error) {
	tool, ok := b.Tools[nodeID]
	if !ok {
		return nil, ErrUnknownNode(nodeID)
	}
	return tool.ToolDefinition(ctx, meta, config)
}

func (b *Bundle) ExecuteTool(ctx context.Context, nodeID string, config json.RawMessage, args json.RawMessage, input map[string]any) (any, error) {
	tool, ok := b.Tools[nodeID]
	if !ok {
		return nil, ErrUnknownNode(nodeID)
	}
	return tool.ExecuteTool(ctx, config, args, input)
}
