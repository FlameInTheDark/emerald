package action

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/FlameInTheDark/emerald/internal/llm"
	"github.com/FlameInTheDark/emerald/internal/node"
	"github.com/FlameInTheDark/emerald/internal/shellcmd"
	"github.com/FlameInTheDark/emerald/internal/templating"
)

type ListNodesToolNode struct {
	Clusters ClusterStore
}

func (e *ListNodesToolNode) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	return (&ListNodesAction{Clusters: e.Clusters}).Execute(ctx, config, input)
}

func (e *ListNodesToolNode) Validate(config json.RawMessage) error {
	var cfg proxmoxConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if strings.TrimSpace(cfg.ClusterID) == "" {
		return fmt.Errorf("clusterId is required")
	}

	return nil
}

func (e *ListNodesToolNode) ToolDefinition(_ context.Context, meta node.ToolNodeMetadata, _ json.RawMessage) (*llm.ToolDefinition, error) {
	return &llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolSpec{
			Name:        sanitizeToolName(meta.Label, "list_nodes"),
			Description: "List all Proxmox nodes in the configured cluster.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}, nil
}

func (e *ListNodesToolNode) ExecuteTool(ctx context.Context, config json.RawMessage, _ json.RawMessage, input map[string]any) (any, error) {
	var cfg proxmoxConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return executeToolNode(ctx, &ListNodesAction{Clusters: e.Clusters}, cfg, input)
}

type ListVMsCTsToolNode struct {
	Clusters ClusterStore
}

func (e *ListVMsCTsToolNode) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	return (&ListVMsCTsAction{Clusters: e.Clusters}).Execute(ctx, config, input)
}

func (e *ListVMsCTsToolNode) Validate(config json.RawMessage) error {
	var cfg listVMsCTsConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if strings.TrimSpace(cfg.ClusterID) == "" {
		return fmt.Errorf("clusterId is required")
	}

	return nil
}

func (e *ListVMsCTsToolNode) ToolDefinition(_ context.Context, meta node.ToolNodeMetadata, _ json.RawMessage) (*llm.ToolDefinition, error) {
	return &llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolSpec{
			Name:        sanitizeToolName(meta.Label, "list_workloads"),
			Description: "List Proxmox virtual machines and containers in the configured cluster. Optionally pass a node name to filter the results.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"node": map[string]any{
						"type":        "string",
						"description": "Optional Proxmox node name to filter workloads.",
					},
				},
			},
		},
	}, nil
}

func (e *ListVMsCTsToolNode) ExecuteTool(ctx context.Context, config json.RawMessage, args json.RawMessage, input map[string]any) (any, error) {
	cfg, err := parseListVMsCTsToolConfig(config)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Node string `json:"node"`
	}
	if err := unmarshalToolArgs(args, &payload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.Node) != "" {
		cfg.Node = strings.TrimSpace(payload.Node)
	}

	return executeToolNode(ctx, &ListVMsCTsAction{Clusters: e.Clusters}, cfg, input)
}

type VMStartToolNode struct {
	Clusters ClusterStore
}

func (e *VMStartToolNode) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	return (&VMStartAction{Clusters: e.Clusters}).Execute(ctx, config, input)
}

func (e *VMStartToolNode) Validate(config json.RawMessage) error {
	var cfg vmStartConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if strings.TrimSpace(cfg.ClusterID) == "" {
		return fmt.Errorf("clusterId is required")
	}

	return nil
}

func (e *VMStartToolNode) ToolDefinition(_ context.Context, meta node.ToolNodeMetadata, config json.RawMessage) (*llm.ToolDefinition, error) {
	cfg, err := parseVMStartToolConfig(config)
	if err != nil {
		return nil, err
	}

	return &llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolSpec{
			Name:        sanitizeToolName(meta.Label, "start_vm"),
			Description: "Start a VM in the configured Proxmox cluster.",
			Parameters:  buildVMControlParameters(cfg.Node, cfg.VMID),
		},
	}, nil
}

func (e *VMStartToolNode) ExecuteTool(ctx context.Context, config json.RawMessage, args json.RawMessage, input map[string]any) (any, error) {
	cfg, err := parseVMStartToolConfig(config)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Node string `json:"node"`
		VMID int    `json:"vmid"`
	}
	if err := unmarshalToolArgs(args, &payload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.Node) != "" {
		cfg.Node = strings.TrimSpace(payload.Node)
	}
	if payload.VMID != 0 {
		cfg.VMID = payload.VMID
	}

	return executeToolNode(ctx, &VMStartAction{Clusters: e.Clusters}, cfg, input)
}

type VMStopToolNode struct {
	Clusters ClusterStore
}

func (e *VMStopToolNode) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	return (&VMStopAction{Clusters: e.Clusters}).Execute(ctx, config, input)
}

func (e *VMStopToolNode) Validate(config json.RawMessage) error {
	var cfg vmStartConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if strings.TrimSpace(cfg.ClusterID) == "" {
		return fmt.Errorf("clusterId is required")
	}

	return nil
}

func (e *VMStopToolNode) ToolDefinition(_ context.Context, meta node.ToolNodeMetadata, config json.RawMessage) (*llm.ToolDefinition, error) {
	cfg, err := parseVMStartToolConfig(config)
	if err != nil {
		return nil, err
	}

	return &llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolSpec{
			Name:        sanitizeToolName(meta.Label, "stop_vm"),
			Description: "Stop a VM in the configured Proxmox cluster immediately.",
			Parameters:  buildVMControlParameters(cfg.Node, cfg.VMID),
		},
	}, nil
}

func (e *VMStopToolNode) ExecuteTool(ctx context.Context, config json.RawMessage, args json.RawMessage, input map[string]any) (any, error) {
	cfg, err := parseVMStartToolConfig(config)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Node string `json:"node"`
		VMID int    `json:"vmid"`
	}
	if err := unmarshalToolArgs(args, &payload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.Node) != "" {
		cfg.Node = strings.TrimSpace(payload.Node)
	}
	if payload.VMID != 0 {
		cfg.VMID = payload.VMID
	}

	return executeToolNode(ctx, &VMStopAction{Clusters: e.Clusters}, cfg, input)
}

type VMCloneToolNode struct {
	Clusters ClusterStore
}

func (e *VMCloneToolNode) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	return (&VMCloneAction{Clusters: e.Clusters}).Execute(ctx, config, input)
}

func (e *VMCloneToolNode) Validate(config json.RawMessage) error {
	var cfg vmCloneConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if strings.TrimSpace(cfg.ClusterID) == "" {
		return fmt.Errorf("clusterId is required")
	}

	return nil
}

func (e *VMCloneToolNode) ToolDefinition(_ context.Context, meta node.ToolNodeMetadata, config json.RawMessage) (*llm.ToolDefinition, error) {
	cfg, err := parseVMCloneToolConfig(config)
	if err != nil {
		return nil, err
	}

	properties := map[string]any{
		"node": map[string]any{
			"type":        "string",
			"description": "Proxmox node name that hosts the source VM.",
		},
		"vmid": map[string]any{
			"type":        "integer",
			"description": "Source VM ID to clone.",
		},
		"newName": map[string]any{
			"type":        "string",
			"description": "Name of the new VM clone.",
		},
		"newId": map[string]any{
			"type":        "integer",
			"description": "New VM ID for the clone.",
		},
	}
	required := make([]string, 0, 4)
	if strings.TrimSpace(cfg.Node) == "" {
		required = append(required, "node")
	}
	if cfg.VMID == 0 {
		required = append(required, "vmid")
	}
	if strings.TrimSpace(cfg.NewName) == "" {
		required = append(required, "newName")
	}
	if cfg.NewID == 0 {
		required = append(required, "newId")
	}

	params := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		params["required"] = required
	}

	return &llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolSpec{
			Name:        sanitizeToolName(meta.Label, "clone_vm"),
			Description: "Clone a VM in the configured Proxmox cluster.",
			Parameters:  params,
		},
	}, nil
}

func (e *VMCloneToolNode) ExecuteTool(ctx context.Context, config json.RawMessage, args json.RawMessage, input map[string]any) (any, error) {
	cfg, err := parseVMCloneToolConfig(config)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Node    string `json:"node"`
		VMID    int    `json:"vmid"`
		NewName string `json:"newName"`
		NewID   int    `json:"newId"`
	}
	if err := unmarshalToolArgs(args, &payload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.Node) != "" {
		cfg.Node = strings.TrimSpace(payload.Node)
	}
	if payload.VMID != 0 {
		cfg.VMID = payload.VMID
	}
	if strings.TrimSpace(payload.NewName) != "" {
		cfg.NewName = strings.TrimSpace(payload.NewName)
	}
	if payload.NewID != 0 {
		cfg.NewID = payload.NewID
	}

	return executeToolNode(ctx, &VMCloneAction{Clusters: e.Clusters}, cfg, input)
}

type HTTPToolNode struct{}

func (e *HTTPToolNode) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	return (&HTTPAction{}).Execute(ctx, config, input)
}

func (e *HTTPToolNode) Validate(config json.RawMessage) error {
	if len(config) == 0 {
		return nil
	}

	var cfg httpActionConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	return nil
}

func (e *HTTPToolNode) ToolDefinition(_ context.Context, meta node.ToolNodeMetadata, config json.RawMessage) (*llm.ToolDefinition, error) {
	cfg, err := parseHTTPToolConfig(config)
	if err != nil {
		return nil, err
	}

	properties := map[string]any{
		"url": map[string]any{
			"type":        "string",
			"description": "Target URL for the HTTP request.",
		},
		"method": map[string]any{
			"type":        "string",
			"description": "HTTP method such as GET, POST, PUT, PATCH, or DELETE.",
		},
		"headers": map[string]any{
			"type":                 "object",
			"description":          "Optional request headers.",
			"additionalProperties": map[string]any{"type": "string"},
		},
		"body": map[string]any{
			"type":        "string",
			"description": "Optional request body.",
		},
	}

	params := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if strings.TrimSpace(cfg.URL) == "" {
		params["required"] = []string{"url"}
	}

	return &llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolSpec{
			Name:        sanitizeToolName(meta.Label, "http_request"),
			Description: "Execute an HTTP request. Configured values act as defaults and tool arguments can override them.",
			Parameters:  params,
		},
	}, nil
}

func (e *HTTPToolNode) ExecuteTool(ctx context.Context, config json.RawMessage, args json.RawMessage, input map[string]any) (any, error) {
	cfg, err := parseHTTPToolConfig(config)
	if err != nil {
		return nil, err
	}

	var payload struct {
		URL     string            `json:"url"`
		Method  string            `json:"method"`
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}
	if err := unmarshalToolArgs(args, &payload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.URL) != "" {
		cfg.URL = strings.TrimSpace(payload.URL)
	}
	if strings.TrimSpace(payload.Method) != "" {
		cfg.Method = strings.TrimSpace(payload.Method)
	}
	if payload.Headers != nil {
		cfg.Headers = payload.Headers
	}
	if payload.Body != "" {
		cfg.Body = payload.Body
	}

	return executeToolNode(ctx, &HTTPAction{}, cfg, input)
}

type shellCommandToolConfig struct {
	Command          string `json:"command"`
	WorkingDirectory string `json:"workingDirectory"`
	TimeoutSeconds   int    `json:"timeoutSeconds"`
}

type ShellCommandToolNode struct {
	Runner shellcmd.Runner
}

func (e *ShellCommandToolNode) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	result, err := e.execute(ctx, config, nil, input)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal shell result: %w", err)
	}

	return &node.NodeResult{Output: data}, nil
}

func (e *ShellCommandToolNode) Validate(config json.RawMessage) error {
	if len(config) == 0 {
		return nil
	}

	var cfg shellCommandToolConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if cfg.TimeoutSeconds < 0 {
		return fmt.Errorf("timeoutSeconds must be 0 or greater")
	}

	return nil
}

func (e *ShellCommandToolNode) ToolDefinition(_ context.Context, meta node.ToolNodeMetadata, config json.RawMessage) (*llm.ToolDefinition, error) {
	cfg, err := parseShellCommandToolConfig(config)
	if err != nil {
		return nil, err
	}

	properties := map[string]any{
		"command": map[string]any{
			"type":        "string",
			"description": "Shell command to execute.",
		},
		"workingDirectory": map[string]any{
			"type":        "string",
			"description": "Optional working directory. Relative paths are resolved from the app workspace.",
		},
		"timeoutSeconds": map[string]any{
			"type":        "integer",
			"description": "Optional timeout in seconds. Defaults to 60.",
		},
	}
	parameters := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if strings.TrimSpace(cfg.Command) == "" {
		parameters["required"] = []string{"command"}
	}

	return &llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolSpec{
			Name:        sanitizeToolName(meta.Label, "shell_command"),
			Description: "Run a local shell command in the workspace and return stdout, stderr, exit code, and timing information.",
			Parameters:  parameters,
		},
	}, nil
}

func (e *ShellCommandToolNode) ExecuteTool(ctx context.Context, config json.RawMessage, args json.RawMessage, input map[string]any) (any, error) {
	return e.execute(ctx, config, args, input)
}

func (e *ShellCommandToolNode) execute(ctx context.Context, config json.RawMessage, args json.RawMessage, input map[string]any) (any, error) {
	if e.Runner == nil {
		return nil, fmt.Errorf("shell runner is not configured")
	}

	cfg, err := parseShellCommandToolConfig(config)
	if err != nil {
		return nil, err
	}
	if err := templating.RenderStrings(&cfg, input); err != nil {
		return nil, fmt.Errorf("render config: %w", err)
	}

	var payload shellCommandToolConfig
	if err := unmarshalToolArgs(args, &payload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.Command) != "" {
		cfg.Command = strings.TrimSpace(payload.Command)
	}
	if strings.TrimSpace(payload.WorkingDirectory) != "" {
		cfg.WorkingDirectory = strings.TrimSpace(payload.WorkingDirectory)
	}
	if payload.TimeoutSeconds > 0 {
		cfg.TimeoutSeconds = payload.TimeoutSeconds
	}
	if strings.TrimSpace(cfg.Command) == "" {
		return nil, fmt.Errorf("command is required")
	}

	timeout := shellcmd.DefaultTimeout
	if cfg.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}

	return e.Runner.Run(ctx, shellcmd.Request{
		Command:          cfg.Command,
		WorkingDirectory: cfg.WorkingDirectory,
		Timeout:          timeout,
	})
}

func executeToolNode(ctx context.Context, executor node.NodeExecutor, config any, input map[string]any) (any, error) {
	configData, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal tool config: %w", err)
	}

	result, err := executor.Execute(ctx, configData, copyParamsMap(input))
	if err != nil {
		return nil, err
	}
	if result == nil || len(result.Output) == 0 {
		return map[string]any{}, nil
	}

	var decoded any
	if err := json.Unmarshal(result.Output, &decoded); err == nil {
		return decoded, nil
	}

	return string(result.Output), nil
}

func parseListVMsCTsToolConfig(config json.RawMessage) (listVMsCTsConfig, error) {
	var cfg listVMsCTsConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return listVMsCTsConfig{}, fmt.Errorf("parse config: %w", err)
	}
	if strings.TrimSpace(cfg.ClusterID) == "" {
		return listVMsCTsConfig{}, fmt.Errorf("clusterId is required")
	}

	return cfg, nil
}

func parseVMStartToolConfig(config json.RawMessage) (vmStartConfig, error) {
	var cfg vmStartConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return vmStartConfig{}, fmt.Errorf("parse config: %w", err)
	}
	if strings.TrimSpace(cfg.ClusterID) == "" {
		return vmStartConfig{}, fmt.Errorf("clusterId is required")
	}

	return cfg, nil
}

func parseVMCloneToolConfig(config json.RawMessage) (vmCloneConfig, error) {
	var cfg vmCloneConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return vmCloneConfig{}, fmt.Errorf("parse config: %w", err)
	}
	if strings.TrimSpace(cfg.ClusterID) == "" {
		return vmCloneConfig{}, fmt.Errorf("clusterId is required")
	}

	return cfg, nil
}

func parseHTTPToolConfig(config json.RawMessage) (httpActionConfig, error) {
	if len(config) == 0 {
		return httpActionConfig{}, nil
	}

	var cfg httpActionConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return httpActionConfig{}, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}

func parseShellCommandToolConfig(config json.RawMessage) (shellCommandToolConfig, error) {
	if len(config) == 0 {
		return shellCommandToolConfig{}, nil
	}

	var cfg shellCommandToolConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return shellCommandToolConfig{}, fmt.Errorf("parse config: %w", err)
	}
	if cfg.TimeoutSeconds < 0 {
		return shellCommandToolConfig{}, fmt.Errorf("timeoutSeconds must be 0 or greater")
	}

	return cfg, nil
}

func buildVMControlParameters(defaultNode string, defaultVMID int) map[string]any {
	properties := map[string]any{
		"node": map[string]any{
			"type":        "string",
			"description": "Proxmox node name that hosts the VM.",
		},
		"vmid": map[string]any{
			"type":        "integer",
			"description": "VM ID to operate on.",
		},
	}
	required := make([]string, 0, 2)
	if strings.TrimSpace(defaultNode) == "" {
		required = append(required, "node")
	}
	if defaultVMID == 0 {
		required = append(required, "vmid")
	}

	params := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		params["required"] = required
	}

	return params
}

func unmarshalToolArgs(args json.RawMessage, target any) error {
	if len(args) == 0 {
		return nil
	}
	if err := json.Unmarshal(args, target); err != nil {
		return fmt.Errorf("parse tool args: %w", err)
	}

	return nil
}

type ChannelSendAndWaitToolNode struct {
	Channels ChannelStore
	Contacts ChannelContactStore
	Sender   ChannelMessageSender
	Waiter   ChannelReplyWaiter
}

func (e *ChannelSendAndWaitToolNode) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	return (&ChannelSendAndWaitAction{
		Channels: e.Channels,
		Contacts: e.Contacts,
		Sender:   e.Sender,
		Waiter:   e.Waiter,
	}).Execute(ctx, config, input)
}

func (e *ChannelSendAndWaitToolNode) Validate(config json.RawMessage) error {
	var cfg channelSendAndWaitConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if strings.TrimSpace(cfg.ChannelID) == "" {
		return fmt.Errorf("channelId is required")
	}
	if cfg.TimeoutSeconds < 0 {
		return fmt.Errorf("timeoutSeconds must be 0 or greater")
	}

	return nil
}

func (e *ChannelSendAndWaitToolNode) ToolDefinition(_ context.Context, meta node.ToolNodeMetadata, config json.RawMessage) (*llm.ToolDefinition, error) {
	var cfg channelSendAndWaitConfig
	if len(config) > 0 {
		if err := json.Unmarshal(config, &cfg); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
	}

	properties := map[string]any{
		"message": map[string]any{
			"type":        "string",
			"description": "Message to send to the user before waiting for their next reply.",
		},
		"recipient": map[string]any{
			"type":        "string",
			"description": "Optional contact ID or chat ID. Omit to reply to the current triggering user.",
		},
		"timeoutSeconds": map[string]any{
			"type":        "integer",
			"description": "Optional wait timeout in seconds before the tool fails. Defaults to 300 seconds.",
		},
	}

	required := make([]string, 0, 1)
	if strings.TrimSpace(cfg.Message) == "" {
		required = append(required, "message")
	}

	parameters := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		parameters["required"] = required
	}

	return &llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolSpec{
			Name:        sanitizeToolName(meta.Label, "message_user_and_wait"),
			Description: "Send a message to a connected channel user and wait for their next reply. The matched reply is returned to the agent.",
			Parameters:  parameters,
		},
	}, nil
}

func (e *ChannelSendAndWaitToolNode) ExecuteTool(ctx context.Context, config json.RawMessage, args json.RawMessage, input map[string]any) (any, error) {
	var cfg channelSendAndWaitConfig
	if len(config) > 0 {
		if err := json.Unmarshal(config, &cfg); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
	}

	var payload struct {
		Message        string `json:"message"`
		Recipient      string `json:"recipient"`
		TimeoutSeconds int    `json:"timeoutSeconds"`
	}
	if err := unmarshalToolArgs(args, &payload); err != nil {
		return nil, err
	}

	if strings.TrimSpace(payload.Message) != "" {
		cfg.Message = strings.TrimSpace(payload.Message)
	}
	if strings.TrimSpace(payload.Recipient) != "" {
		cfg.Recipient = strings.TrimSpace(payload.Recipient)
	}
	if payload.TimeoutSeconds != 0 {
		cfg.TimeoutSeconds = payload.TimeoutSeconds
	}

	return executeToolNode(
		ctx,
		&ChannelSendAndWaitAction{
			Channels: e.Channels,
			Contacts: e.Contacts,
			Sender:   e.Sender,
			Waiter:   e.Waiter,
		},
		cfg,
		input,
	)
}
