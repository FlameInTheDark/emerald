package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	hplugin "github.com/hashicorp/go-plugin"

	"github.com/FlameInTheDark/emerald/pkg/pluginapi"
	"github.com/FlameInTheDark/emerald/pkg/pluginsdk"
)

const (
	actionTypePrefix = "action:plugin/"
	toolTypePrefix   = "tool:plugin/"
)

type BundleStatus struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path"`
	Healthy     bool   `json:"healthy"`
	Error       string `json:"error,omitempty"`
	NodeCount   int    `json:"node_count"`
}

type NodeBinding struct {
	Type       string             `json:"type"`
	PluginID   string             `json:"plugin_id"`
	PluginName string             `json:"plugin_name"`
	Kind       pluginapi.NodeKind `json:"kind"`
	Spec       pluginapi.NodeSpec `json:"spec"`
	Manifest   Manifest           `json:"-"`
}

type bundleRuntime struct {
	manifest Manifest
	client   *hplugin.Client
	plugin   pluginapi.Plugin
	info     pluginapi.PluginInfo
	error    error
	refs     sync.WaitGroup
}

type Manager struct {
	root    string
	mu      sync.RWMutex
	nodes   map[string]NodeBinding
	bundles map[string]*bundleRuntime
}

func NewManager(root string) *Manager {
	return &Manager{
		root:    strings.TrimSpace(root),
		nodes:   make(map[string]NodeBinding),
		bundles: make(map[string]*bundleRuntime),
	}
}

func (m *Manager) Root() string {
	if m == nil {
		return ""
	}
	return m.root
}

func (m *Manager) Refresh(ctx context.Context) error {
	if m == nil {
		return nil
	}

	manifestPaths, err := discoverManifestPaths(m.root)
	if err != nil {
		return err
	}

	nextBundles := make(map[string]*bundleRuntime, len(manifestPaths))
	nextNodes := make(map[string]NodeBinding)
	loadErrors := make([]string, 0)

	for _, manifestPath := range manifestPaths {
		manifest, err := loadManifest(manifestPath)
		if err != nil {
			loadErrors = append(loadErrors, err.Error())
			continue
		}
		if _, exists := nextBundles[manifest.ID]; exists {
			loadErrors = append(loadErrors, fmt.Sprintf("duplicate plugin id %q in %s", manifest.ID, manifest.Path))
			continue
		}

		runtime, bindings, err := startRuntime(ctx, manifest)
		if err != nil {
			loadErrors = append(loadErrors, err.Error())
			nextBundles[manifest.ID] = &bundleRuntime{
				manifest: manifest,
				error:    err,
			}
			continue
		}

		duplicateType := ""
		for _, binding := range bindings {
			if _, exists := nextNodes[binding.Type]; exists {
				duplicateType = binding.Type
				break
			}
		}
		if duplicateType != "" {
			runtime.close()
			err := fmt.Errorf("plugin %s defines duplicate node type %q", manifest.ID, duplicateType)
			loadErrors = append(loadErrors, err.Error())
			nextBundles[manifest.ID] = &bundleRuntime{
				manifest: manifest,
				error:    err,
			}
			continue
		}

		nextBundles[manifest.ID] = runtime
		for _, binding := range bindings {
			nextNodes[binding.Type] = binding
		}
	}

	m.mu.Lock()
	oldBundles := m.bundles
	m.bundles = nextBundles
	m.nodes = nextNodes
	m.mu.Unlock()

	for _, runtime := range oldBundles {
		if runtime == nil {
			continue
		}
		if next, ok := nextBundles[runtime.manifest.ID]; ok && next == runtime {
			continue
		}
		runtime.close()
	}

	if len(loadErrors) == 0 {
		return nil
	}

	return errors.New(strings.Join(loadErrors, "; "))
}

func (m *Manager) Stop() {
	if m == nil {
		return
	}

	m.mu.Lock()
	bundles := m.bundles
	m.bundles = make(map[string]*bundleRuntime)
	m.nodes = make(map[string]NodeBinding)
	m.mu.Unlock()

	for _, runtime := range bundles {
		if runtime != nil {
			runtime.close()
		}
	}
}

func (m *Manager) Bindings() []NodeBinding {
	if m == nil {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]NodeBinding, 0, len(m.nodes))
	for _, binding := range m.nodes {
		result = append(result, binding)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind == result[j].Kind {
			if result[i].PluginName == result[j].PluginName {
				return result[i].Spec.Label < result[j].Spec.Label
			}
			return result[i].PluginName < result[j].PluginName
		}
		return result[i].Kind < result[j].Kind
	})
	return result
}

func (m *Manager) Statuses() []BundleStatus {
	if m == nil {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]BundleStatus, 0, len(m.bundles))
	for _, runtime := range m.bundles {
		if runtime == nil {
			continue
		}
		status := BundleStatus{
			ID:          runtime.manifest.ID,
			Name:        runtime.manifest.Name,
			Version:     runtime.manifest.Version,
			Description: runtime.manifest.Description,
			Path:        runtime.manifest.Path,
			Healthy:     runtime.error == nil,
		}
		if runtime.error != nil {
			status.Error = runtime.error.Error()
		} else {
			status.NodeCount = len(runtime.info.Nodes)
			if strings.TrimSpace(runtime.info.Version) != "" {
				status.Version = runtime.info.Version
			}
		}
		result = append(result, status)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func (m *Manager) Binding(nodeType string) (NodeBinding, bool) {
	if m == nil {
		return NodeBinding{}, false
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	binding, ok := m.nodes[strings.TrimSpace(nodeType)]
	return binding, ok
}

func (m *Manager) ValidateConfig(ctx context.Context, nodeType string, config json.RawMessage) error {
	runtime, _, nodeID, release, err := m.resolve(nodeType)
	if err != nil {
		return err
	}
	defer release()
	return runtime.plugin.ValidateConfig(ctx, nodeID, normalizeConfigPayload(config))
}

func (m *Manager) ExecuteAction(ctx context.Context, nodeType string, config json.RawMessage, input map[string]any) (any, error) {
	runtime, _, nodeID, release, err := m.resolve(nodeType)
	if err != nil {
		return nil, err
	}
	defer release()
	return runtime.plugin.ExecuteAction(ctx, nodeID, normalizeConfigPayload(config), copyInput(input))
}

func (m *Manager) ToolDefinition(ctx context.Context, nodeType string, meta pluginapi.ToolNodeMetadata, config json.RawMessage) (*pluginapi.ToolDefinition, error) {
	runtime, _, nodeID, release, err := m.resolve(nodeType)
	if err != nil {
		return nil, err
	}
	defer release()
	return runtime.plugin.ToolDefinition(ctx, nodeID, meta, normalizeConfigPayload(config))
}

func (m *Manager) ExecuteTool(ctx context.Context, nodeType string, config json.RawMessage, args json.RawMessage, input map[string]any) (any, error) {
	runtime, _, nodeID, release, err := m.resolve(nodeType)
	if err != nil {
		return nil, err
	}
	defer release()
	return runtime.plugin.ExecuteTool(ctx, nodeID, normalizeConfigPayload(config), normalizeConfigPayload(args), copyInput(input))
}

func (m *Manager) resolve(nodeType string) (*bundleRuntime, NodeBinding, string, func(), error) {
	kind, pluginID, nodeID, ok := ParseNodeType(nodeType)
	if !ok {
		return nil, NodeBinding{}, "", nil, fmt.Errorf("node type %q is not a plugin node", nodeType)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	binding, ok := m.nodes[strings.TrimSpace(nodeType)]
	if !ok {
		return nil, NodeBinding{}, "", nil, fmt.Errorf("plugin node type %q is unavailable", nodeType)
	}
	if binding.Kind != kind || binding.PluginID != pluginID {
		return nil, NodeBinding{}, "", nil, fmt.Errorf("plugin node type %q resolved to an unexpected plugin binding", nodeType)
	}

	runtime := m.bundles[pluginID]
	if runtime == nil || runtime.plugin == nil {
		return nil, NodeBinding{}, "", nil, fmt.Errorf("plugin %q is unavailable", pluginID)
	}
	if runtime.error != nil {
		return nil, NodeBinding{}, "", nil, runtime.error
	}

	runtime.refs.Add(1)
	return runtime, binding, nodeID, runtime.refs.Done, nil
}

func startRuntime(ctx context.Context, manifest Manifest) (*bundleRuntime, []NodeBinding, error) {
	if _, err := os.Stat(manifest.Executable); err != nil {
		return nil, nil, fmt.Errorf("plugin %s executable %s: %w", manifest.ID, manifest.Executable, err)
	}

	cmd := exec.Command(manifest.Executable, manifest.Args...)
	cmd.Dir = manifest.Dir
	cmd.Env = append([]string{}, os.Environ()...)
	for key, value := range manifest.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
	}

	client := hplugin.NewClient(pluginsdk.NewClientConfig(cmd))
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, nil, fmt.Errorf("start plugin %s: %w", manifest.ID, err)
	}

	raw, err := rpcClient.Dispense(pluginsdk.PluginName)
	if err != nil {
		client.Kill()
		return nil, nil, fmt.Errorf("dispense plugin %s: %w", manifest.ID, err)
	}

	remote, ok := raw.(pluginapi.Plugin)
	if !ok || remote == nil {
		client.Kill()
		return nil, nil, fmt.Errorf("plugin %s returned an unexpected implementation", manifest.ID)
	}

	info, err := remote.Describe(ctx)
	if err != nil {
		client.Kill()
		return nil, nil, fmt.Errorf("describe plugin %s: %w", manifest.ID, err)
	}

	if strings.TrimSpace(info.APIVersion) != pluginapi.APIVersion {
		client.Kill()
		return nil, nil, fmt.Errorf("plugin %s uses unsupported api version %q", manifest.ID, strings.TrimSpace(info.APIVersion))
	}

	if info.ID != "" && !strings.EqualFold(strings.TrimSpace(info.ID), manifest.ID) {
		client.Kill()
		return nil, nil, fmt.Errorf("plugin manifest id %q does not match runtime id %q", manifest.ID, strings.TrimSpace(info.ID))
	}
	info.ID = manifest.ID
	if strings.TrimSpace(info.Name) == "" {
		info.Name = manifest.Name
	}
	if strings.TrimSpace(info.Version) == "" {
		info.Version = manifest.Version
	}

	bindings := make([]NodeBinding, 0, len(info.Nodes))
	seenNodeIDs := make(map[string]struct{}, len(info.Nodes))
	for _, spec := range info.Nodes {
		spec.ID = strings.TrimSpace(spec.ID)
		spec.Label = strings.TrimSpace(spec.Label)
		spec.Icon = strings.TrimSpace(spec.Icon)
		spec.Color = strings.TrimSpace(spec.Color)

		if spec.ID == "" {
			client.Kill()
			return nil, nil, fmt.Errorf("plugin %s contains a node without an id", manifest.ID)
		}
		if _, exists := seenNodeIDs[spec.ID]; exists {
			client.Kill()
			return nil, nil, fmt.Errorf("plugin %s defines duplicate node id %q", manifest.ID, spec.ID)
		}
		seenNodeIDs[spec.ID] = struct{}{}

		if spec.Kind != pluginapi.NodeKindAction && spec.Kind != pluginapi.NodeKindTool {
			client.Kill()
			return nil, nil, fmt.Errorf("plugin %s node %q uses unsupported kind %q", manifest.ID, spec.ID, spec.Kind)
		}
		if spec.Label == "" {
			spec.Label = spec.ID
		}
		if spec.DefaultConfig == nil {
			spec.DefaultConfig = map[string]any{}
		}

		bindings = append(bindings, NodeBinding{
			Type:       BuildNodeType(spec.Kind, manifest.ID, spec.ID),
			PluginID:   manifest.ID,
			PluginName: info.Name,
			Kind:       spec.Kind,
			Spec:       spec,
			Manifest:   manifest,
		})
	}

	return &bundleRuntime{
		manifest: manifest,
		client:   client,
		plugin:   remote,
		info:     info,
	}, bindings, nil
}

func (r *bundleRuntime) close() {
	if r == nil || r.client == nil {
		return
	}

	done := make(chan struct{})
	go func() {
		r.refs.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}

	r.client.Kill()
}

func BuildNodeType(kind pluginapi.NodeKind, pluginID string, nodeID string) string {
	pluginID = strings.TrimSpace(pluginID)
	nodeID = strings.TrimSpace(nodeID)

	switch kind {
	case pluginapi.NodeKindTool:
		return toolTypePrefix + pluginID + "/" + nodeID
	default:
		return actionTypePrefix + pluginID + "/" + nodeID
	}
}

func ParseNodeType(nodeType string) (pluginapi.NodeKind, string, string, bool) {
	trimmed := strings.TrimSpace(nodeType)

	if strings.HasPrefix(trimmed, actionTypePrefix) {
		pluginID, nodeID, ok := splitPluginPath(strings.TrimPrefix(trimmed, actionTypePrefix))
		return pluginapi.NodeKindAction, pluginID, nodeID, ok
	}
	if strings.HasPrefix(trimmed, toolTypePrefix) {
		pluginID, nodeID, ok := splitPluginPath(strings.TrimPrefix(trimmed, toolTypePrefix))
		return pluginapi.NodeKindTool, pluginID, nodeID, ok
	}

	return "", "", "", false
}

func IsPluginNodeType(nodeType string) bool {
	_, _, _, ok := ParseNodeType(nodeType)
	return ok
}

func splitPluginPath(path string) (string, string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 {
		return "", "", false
	}

	pluginID := strings.TrimSpace(parts[0])
	nodeID := strings.TrimSpace(parts[1])
	if pluginID == "" || nodeID == "" {
		return "", "", false
	}

	return pluginID, nodeID, true
}

func copyInput(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}

	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func normalizeConfigPayload(payload json.RawMessage) json.RawMessage {
	if len(payload) == 0 {
		return json.RawMessage("{}")
	}
	return payload
}
