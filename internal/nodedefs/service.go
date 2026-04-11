package nodedefs

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/pipeline"
	"github.com/FlameInTheDark/emerald/internal/plugins"
	"github.com/FlameInTheDark/emerald/pkg/pluginapi"
)

const (
	SourceBuiltin = "builtin"
	SourcePlugin  = "plugin"
)

type Definition struct {
	Type          string                   `json:"type"`
	Category      string                   `json:"category"`
	Source        string                   `json:"source"`
	PluginID      string                   `json:"plugin_id,omitempty"`
	PluginName    string                   `json:"plugin_name,omitempty"`
	Label         string                   `json:"label"`
	Description   string                   `json:"description,omitempty"`
	Icon          string                   `json:"icon,omitempty"`
	Color         string                   `json:"color,omitempty"`
	MenuPath      []string                 `json:"menu_path,omitempty"`
	DefaultConfig map[string]any           `json:"default_config,omitempty"`
	Fields        []pluginapi.FieldSpec    `json:"fields,omitempty"`
	Outputs       []pluginapi.OutputHandle `json:"outputs,omitempty"`
	OutputHints   []pluginapi.OutputHint   `json:"output_hints,omitempty"`
}

type anyBinding struct {
	Type       string
	PluginID   string
	PluginName string
	Spec       pluginapi.NodeSpec
}

type Service struct {
	pluginManager *plugins.Manager
	builtins      map[string]Definition
}

func NewService(pluginManager *plugins.Manager) *Service {
	builtins := make(map[string]Definition)
	for _, definition := range BuiltinDefinitions() {
		builtins[definition.Type] = definition
	}

	return &Service{
		pluginManager: pluginManager,
		builtins:      builtins,
	}
}

func (s *Service) List() []Definition {
	if s == nil {
		return nil
	}

	definitions := make([]Definition, 0, len(s.builtins))
	for _, definition := range s.builtins {
		definitions = append(definitions, definition)
	}

	if s.pluginManager != nil {
		for _, binding := range s.pluginManager.Bindings() {
			definitions = append(definitions, pluginDefinitionFromBinding(anyBinding{
				Type:       binding.Type,
				PluginID:   binding.PluginID,
				PluginName: binding.PluginName,
				Spec:       binding.Spec,
			}))
		}
	}

	sort.Slice(definitions, func(i, j int) bool {
		if definitions[i].Category == definitions[j].Category {
			return definitions[i].Label < definitions[j].Label
		}
		return definitions[i].Category < definitions[j].Category
	})

	return definitions
}

func (s *Service) PluginStatuses() []plugins.BundleStatus {
	if s == nil || s.pluginManager == nil {
		return nil
	}
	return s.pluginManager.Statuses()
}

func (s *Service) RefreshPlugins(ctx context.Context) error {
	if s == nil || s.pluginManager == nil {
		return nil
	}
	return s.pluginManager.Refresh(ctx)
}

func (s *Service) Get(nodeType string) (Definition, bool) {
	if s == nil {
		return Definition{}, false
	}

	if definition, ok := s.builtins[strings.TrimSpace(nodeType)]; ok {
		return definition, true
	}
	if s.pluginManager == nil {
		return Definition{}, false
	}

	binding, ok := s.pluginManager.Binding(nodeType)
	if !ok {
		return Definition{}, false
	}

	return pluginDefinitionFromBinding(anyBinding{
		Type:       binding.Type,
		PluginID:   binding.PluginID,
		PluginName: binding.PluginName,
		Spec:       binding.Spec,
	}), true
}

func (s *Service) ValidateDefinition(ctx context.Context, nodesJSON string, edgesJSON string, allowUnavailablePlugins bool) error {
	flowData, err := pipeline.ParseFlowData(nodesJSON, edgesJSON)
	if err != nil {
		return err
	}
	return s.ValidateFlowData(ctx, *flowData, allowUnavailablePlugins)
}

func (s *Service) ValidateFlowData(ctx context.Context, flowData pipeline.FlowData, allowUnavailablePlugins bool) error {
	if err := pipeline.ValidateFlowData(flowData); err != nil {
		return err
	}

	definitions := make(map[string]Definition)
	for _, definition := range s.List() {
		definitions[definition.Type] = definition
	}

	nodeDefinitions := make(map[string]Definition, len(flowData.Nodes))
	for _, flowNode := range flowData.Nodes {
		nodeType, config := resolveNodeTypeAndConfig(flowNode)
		if definition, ok := definitions[nodeType]; ok {
			nodeDefinitions[flowNode.ID] = definition
			if definition.Source == SourcePlugin && s.pluginManager != nil {
				if err := s.pluginManager.ValidateConfig(ctx, nodeType, config); err != nil {
					return fmt.Errorf("node %q: %w", flowNode.ID, err)
				}
			}
			continue
		}

		if plugins.IsPluginNodeType(nodeType) && allowUnavailablePlugins {
			continue
		}

		if plugins.IsPluginNodeType(nodeType) {
			return fmt.Errorf("node %q uses unavailable plugin node type %q", flowNode.ID, nodeType)
		}

		return fmt.Errorf("node %q uses unknown node type %q", flowNode.ID, nodeType)
	}

	for _, edge := range flowData.Edges {
		sourceDefinition, ok := nodeDefinitions[edge.Source]
		if !ok || len(sourceDefinition.Outputs) == 0 {
			continue
		}

		handleID := strings.TrimSpace(edge.SourceHandle)
		if handleID == "" {
			return fmt.Errorf("edge %q must use one of the declared output handles for node %q", edge.ID, edge.Source)
		}
		if !definitionHasHandle(sourceDefinition, handleID) {
			return fmt.Errorf("edge %q references unknown output handle %q on node %q", edge.ID, handleID, edge.Source)
		}
	}

	return nil
}

func resolveNodeTypeAndConfig(flowNode pipeline.FlowNode) (string, json.RawMessage) {
	nodeType := strings.TrimSpace(flowNode.Type)
	configData := flowNode.Data

	if len(flowNode.Data) == 0 {
		return nodeType, configData
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(flowNode.Data, &payload); err != nil {
		return nodeType, configData
	}

	if rawType, ok := payload["type"]; ok {
		var decoded string
		if err := json.Unmarshal(rawType, &decoded); err == nil && strings.TrimSpace(decoded) != "" {
			nodeType = strings.TrimSpace(decoded)
		}
	}
	if rawConfig, ok := payload["config"]; ok {
		configData = rawConfig
	}

	return nodeType, configData
}

func definitionHasHandle(definition Definition, handleID string) bool {
	for _, handle := range definition.Outputs {
		if strings.TrimSpace(handle.ID) == handleID {
			return true
		}
	}
	return false
}
