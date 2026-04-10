package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	ik8s "github.com/FlameInTheDark/emerald/internal/kubernetes"
	"github.com/FlameInTheDark/emerald/internal/proxmox"
	"github.com/FlameInTheDark/emerald/internal/shellcmd"
	"github.com/FlameInTheDark/emerald/internal/skills"
)

type ToolHandler func(ctx context.Context, args json.RawMessage) (any, error)

type ToolClusterStore interface {
	List(ctx context.Context) ([]models.Cluster, error)
	GetByID(ctx context.Context, id string) (*models.Cluster, error)
}

type ToolKubernetesClusterStore interface {
	List(ctx context.Context) ([]models.KubernetesCluster, error)
	GetByID(ctx context.Context, id string) (*models.KubernetesCluster, error)
}

type ToolPipelineStore interface {
	List(ctx context.Context) ([]models.Pipeline, error)
	GetByID(ctx context.Context, id string) (*models.Pipeline, error)
	Create(ctx context.Context, pipeline *models.Pipeline) error
	Update(ctx context.Context, pipeline *models.Pipeline) error
	Delete(ctx context.Context, id string) error
}

type ToolPipelineReloader interface {
	Reload(ctx context.Context) error
}

type ToolPipelineRunner interface {
	Run(ctx context.Context, pipelineID string, input map[string]any) (*ToolPipelineRunResult, error)
}

type ToolPipelineRunResult struct {
	ExecutionID  string `json:"execution_id"`
	PipelineID   string `json:"pipeline_id"`
	PipelineName string `json:"pipeline_name"`
	Status       string `json:"status"`
	NodesRun     int    `json:"nodes_run"`
	Returned     bool   `json:"returned"`
	ReturnValue  any    `json:"return_value,omitempty"`
}

type ToolRegistryOptions struct {
	ProxmoxStore               ToolClusterStore
	KubernetesStore            ToolKubernetesClusterStore
	PipelineStore              ToolPipelineStore
	PipelineReloader           ToolPipelineReloader
	PipelineRunner             ToolPipelineRunner
	DefaultProxmoxClusterID    string
	DefaultKubernetesClusterID string
	EnableProxmox              bool
	EnableKubernetes           bool
	SkillStore                 skills.Reader
	ShellRunner                shellcmd.Runner
}

type ToolRegistry struct {
	tools                      map[string]ToolDefinition
	handlers                   map[string]ToolHandler
	clusterStore               ToolClusterStore
	kubernetesStore            ToolKubernetesClusterStore
	pipelineStore              ToolPipelineStore
	pipelineReloader           ToolPipelineReloader
	pipelineRunner             ToolPipelineRunner
	defaultClusterID           string
	defaultKubernetesClusterID string
	enableProxmox              bool
	enableKubernetes           bool
	skillStore                 skills.Reader
	shellRunner                shellcmd.Runner
}

type proxmoxToolArgs struct {
	ClusterID   string `json:"cluster_id"`
	ClusterName string `json:"cluster_name"`
}

func NewToolRegistry(
	clusterStore ToolClusterStore,
	pipelineStore ToolPipelineStore,
	pipelineReloader ToolPipelineReloader,
	pipelineRunner ToolPipelineRunner,
	defaultClusterID string,
	skillStore skills.Reader,
	shellRunner shellcmd.Runner,
) *ToolRegistry {
	return NewToolRegistryWithOptions(ToolRegistryOptions{
		ProxmoxStore:            clusterStore,
		PipelineStore:           pipelineStore,
		PipelineReloader:        pipelineReloader,
		PipelineRunner:          pipelineRunner,
		DefaultProxmoxClusterID: defaultClusterID,
		EnableProxmox:           true,
		SkillStore:              skillStore,
		ShellRunner:             shellRunner,
	})
}

func NewToolRegistryWithOptions(opts ToolRegistryOptions) *ToolRegistry {
	r := &ToolRegistry{
		tools:                      make(map[string]ToolDefinition),
		handlers:                   make(map[string]ToolHandler),
		clusterStore:               opts.ProxmoxStore,
		kubernetesStore:            opts.KubernetesStore,
		pipelineStore:              opts.PipelineStore,
		pipelineReloader:           opts.PipelineReloader,
		pipelineRunner:             opts.PipelineRunner,
		defaultClusterID:           strings.TrimSpace(opts.DefaultProxmoxClusterID),
		defaultKubernetesClusterID: strings.TrimSpace(opts.DefaultKubernetesClusterID),
		enableProxmox:              opts.EnableProxmox,
		enableKubernetes:           opts.EnableKubernetes,
		skillStore:                 opts.SkillStore,
		shellRunner:                opts.ShellRunner,
	}

	if r.enableProxmox {
		r.registerProxmoxTools()
	}
	if r.enableKubernetes {
		r.registerKubernetesTools()
	}
	r.registerPipelineTools()
	r.registerSkillTools()
	r.registerShellTools()
	return r
}

func (r *ToolRegistry) registerProxmoxTools() {
	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "list_nodes",
			Description: "List all nodes in a Proxmox cluster. When cluster_id or cluster_name is omitted, the selected chat cluster or the only configured cluster is used.",
			Parameters:  r.proxmoxToolParameters(nil),
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var params proxmoxToolArgs
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}

		client, cluster, err := r.resolveClusterClient(ctx, params.proxmoxToolArgs())
		if err != nil {
			return nil, fmt.Errorf("resolve cluster: %w", err)
		}

		nodes, err := client.ListNodes(ctx)
		if err != nil {
			return nil, fmt.Errorf("list nodes for cluster %s: %w", cluster.Name, err)
		}

		return map[string]any{
			"cluster_id":   cluster.ID,
			"cluster_name": cluster.Name,
			"nodes":        nodes,
		}, nil
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "list_vms",
			Description: "List all VMs on a specific Proxmox node.",
			Parameters: r.proxmoxToolParameters(map[string]any{
				"node": map[string]any{
					"type":        "string",
					"description": "Proxmox node name",
				},
			}, "node"),
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var params struct {
			proxmoxToolArgs
			Node string `json:"node"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}

		client, cluster, err := r.resolveClusterClient(ctx, params.proxmoxToolArgs)
		if err != nil {
			return nil, fmt.Errorf("resolve cluster: %w", err)
		}

		vms, err := client.ListVMs(ctx, params.Node)
		if err != nil {
			return nil, fmt.Errorf("list vms on cluster %s: %w", cluster.Name, err)
		}

		return map[string]any{
			"cluster_id":   cluster.ID,
			"cluster_name": cluster.Name,
			"node":         params.Node,
			"vms":          vms,
		}, nil
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "start_vm",
			Description: "Start a VM on a Proxmox node.",
			Parameters: r.proxmoxToolParameters(map[string]any{
				"node": map[string]any{
					"type":        "string",
					"description": "Proxmox node name",
				},
				"vmid": map[string]any{
					"type":        "integer",
					"description": "VM ID",
				},
			}, "node", "vmid"),
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var params struct {
			proxmoxToolArgs
			Node string `json:"node"`
			VMID int    `json:"vmid"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}

		client, cluster, err := r.resolveClusterClient(ctx, params.proxmoxToolArgs)
		if err != nil {
			return nil, fmt.Errorf("resolve cluster: %w", err)
		}

		if err := client.StartVM(ctx, params.Node, params.VMID); err != nil {
			return nil, fmt.Errorf("start vm on cluster %s: %w", cluster.Name, err)
		}

		return map[string]any{
			"status":       "started",
			"cluster_id":   cluster.ID,
			"cluster_name": cluster.Name,
			"node":         params.Node,
			"vmid":         params.VMID,
		}, nil
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "stop_vm",
			Description: "Stop a VM on a Proxmox node immediately.",
			Parameters: r.proxmoxToolParameters(map[string]any{
				"node": map[string]any{
					"type":        "string",
					"description": "Proxmox node name",
				},
				"vmid": map[string]any{
					"type":        "integer",
					"description": "VM ID",
				},
			}, "node", "vmid"),
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var params struct {
			proxmoxToolArgs
			Node string `json:"node"`
			VMID int    `json:"vmid"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}

		client, cluster, err := r.resolveClusterClient(ctx, params.proxmoxToolArgs)
		if err != nil {
			return nil, fmt.Errorf("resolve cluster: %w", err)
		}

		if err := client.StopVM(ctx, params.Node, params.VMID); err != nil {
			return nil, fmt.Errorf("stop vm on cluster %s: %w", cluster.Name, err)
		}

		return map[string]any{
			"status":       "stopped",
			"cluster_id":   cluster.ID,
			"cluster_name": cluster.Name,
			"node":         params.Node,
			"vmid":         params.VMID,
		}, nil
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "shutdown_vm",
			Description: "Gracefully shutdown a VM on a Proxmox node.",
			Parameters: r.proxmoxToolParameters(map[string]any{
				"node": map[string]any{
					"type":        "string",
					"description": "Proxmox node name",
				},
				"vmid": map[string]any{
					"type":        "integer",
					"description": "VM ID",
				},
			}, "node", "vmid"),
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var params struct {
			proxmoxToolArgs
			Node string `json:"node"`
			VMID int    `json:"vmid"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}

		client, cluster, err := r.resolveClusterClient(ctx, params.proxmoxToolArgs)
		if err != nil {
			return nil, fmt.Errorf("resolve cluster: %w", err)
		}

		if err := client.ShutdownVM(ctx, params.Node, params.VMID); err != nil {
			return nil, fmt.Errorf("shutdown vm on cluster %s: %w", cluster.Name, err)
		}

		return map[string]any{
			"status":       "shutting_down",
			"cluster_id":   cluster.ID,
			"cluster_name": cluster.Name,
			"node":         params.Node,
			"vmid":         params.VMID,
		}, nil
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "list_storages",
			Description: "List all storages on a Proxmox node.",
			Parameters: r.proxmoxToolParameters(map[string]any{
				"node": map[string]any{
					"type":        "string",
					"description": "Proxmox node name",
				},
			}, "node"),
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var params struct {
			proxmoxToolArgs
			Node string `json:"node"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}

		client, cluster, err := r.resolveClusterClient(ctx, params.proxmoxToolArgs)
		if err != nil {
			return nil, fmt.Errorf("resolve cluster: %w", err)
		}

		storages, err := client.ListStorages(ctx, params.Node)
		if err != nil {
			return nil, fmt.Errorf("list storages on cluster %s: %w", cluster.Name, err)
		}

		return map[string]any{
			"cluster_id":   cluster.ID,
			"cluster_name": cluster.Name,
			"node":         params.Node,
			"storages":     storages,
		}, nil
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "list_cluster_resources",
			Description: "List all resources in the Proxmox cluster, including VMs, containers, nodes, and storages.",
			Parameters:  r.proxmoxToolParameters(nil),
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var params proxmoxToolArgs
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}

		client, cluster, err := r.resolveClusterClient(ctx, params.proxmoxToolArgs())
		if err != nil {
			return nil, fmt.Errorf("resolve cluster: %w", err)
		}

		resources, err := client.ListClusterResources(ctx)
		if err != nil {
			return nil, fmt.Errorf("list cluster resources for %s: %w", cluster.Name, err)
		}

		return map[string]any{
			"cluster_id":   cluster.ID,
			"cluster_name": cluster.Name,
			"resources":    resources,
		}, nil
	})
}

func (r *ToolRegistry) registerSkillTools() {
	if r.skillStore == nil {
		return
	}

	r.Register(SkillToolDefinition(), func(ctx context.Context, args json.RawMessage) (any, error) {
		return ExecuteSkillTool(ctx, r.skillStore, args)
	})
}

func (r *ToolRegistry) registerShellTools() {
	if r.shellRunner == nil {
		return
	}

	r.Register(ShellToolDefinition(), func(ctx context.Context, args json.RawMessage) (any, error) {
		return ExecuteShellTool(ctx, r.shellRunner, args)
	})
}

func SkillToolDefinition() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "get_skill",
			Description: "Read a local skill by name to get its full instructions and reference material.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "Skill name, for example frontend-design.",
					},
				},
				"required": []string{"name"},
			},
		},
	}
}

func ExecuteSkillTool(_ context.Context, reader skills.Reader, args json.RawMessage) (any, error) {
	if reader == nil {
		return nil, fmt.Errorf("skill store is not configured")
	}

	var payload struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, fmt.Errorf("parse tool args: %w", err)
	}
	if strings.TrimSpace(payload.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}

	skill, ok := reader.GetByName(payload.Name)
	if !ok {
		return nil, fmt.Errorf("skill %q not found", payload.Name)
	}

	return map[string]any{
		"name":        skill.Name,
		"description": skill.Description,
		"path":        skill.Path,
		"content":     skill.Content,
	}, nil
}

func ShellToolDefinition() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "run_shell_command",
			Description: "Run a local shell command in the workspace and return stdout, stderr, exit code, and timing information.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "Shell command to execute.",
					},
					"working_directory": map[string]any{
						"type":        "string",
						"description": "Optional working directory. Relative paths are resolved from the app workspace.",
					},
					"timeout_seconds": map[string]any{
						"type":        "integer",
						"description": "Optional timeout in seconds. Defaults to 60 seconds.",
					},
				},
				"required": []string{"command"},
			},
		},
	}
}

func ExecuteShellTool(ctx context.Context, runner shellcmd.Runner, args json.RawMessage) (any, error) {
	if runner == nil {
		return nil, fmt.Errorf("shell runner is not configured")
	}

	var payload struct {
		Command          string `json:"command"`
		WorkingDirectory string `json:"working_directory"`
		TimeoutSeconds   int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, fmt.Errorf("parse tool args: %w", err)
	}
	if strings.TrimSpace(payload.Command) == "" {
		return nil, fmt.Errorf("command is required")
	}

	timeout := shellcmd.DefaultTimeout
	if payload.TimeoutSeconds > 0 {
		timeout = time.Duration(payload.TimeoutSeconds) * time.Second
	}

	return runner.Run(ctx, shellcmd.Request{
		Command:          payload.Command,
		WorkingDirectory: payload.WorkingDirectory,
		Timeout:          timeout,
	})
}

func (r *ToolRegistry) Register(def ToolDefinition, handler ToolHandler) {
	r.tools[def.Function.Name] = def
	r.handlers[def.Function.Name] = handler
}

func (r *ToolRegistry) kubernetesToolParameters(extraProperties map[string]any, required ...string) map[string]any {
	properties := map[string]any{
		"cluster_id": map[string]any{
			"type":        "string",
			"description": "Configured Kubernetes cluster ID. Optional when a cluster is already selected for the chat or only one cluster exists.",
		},
		"cluster_name": map[string]any{
			"type":        "string",
			"description": "Configured Kubernetes cluster name. Optional alternative to cluster_id.",
		},
	}

	for key, value := range extraProperties {
		properties[key] = value
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

func (r *ToolRegistry) GetAllTools() []ToolDefinition {
	tools := make([]ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

func (r *ToolRegistry) Execute(ctx context.Context, toolName string, args json.RawMessage) (any, error) {
	handler, ok := r.handlers[toolName]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
	return handler(ctx, args)
}

func (r *ToolRegistry) proxmoxToolParameters(extraProperties map[string]any, required ...string) map[string]any {
	properties := map[string]any{
		"cluster_id": map[string]any{
			"type":        "string",
			"description": "Configured Proxmox cluster ID. Optional when a cluster is already selected for the chat or only one cluster exists.",
		},
		"cluster_name": map[string]any{
			"type":        "string",
			"description": "Configured Proxmox cluster name. Optional alternative to cluster_id.",
		},
	}

	for key, value := range extraProperties {
		properties[key] = value
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

func (r *ToolRegistry) resolveClusterClient(ctx context.Context, params proxmoxToolArgs) (*proxmox.Client, *models.Cluster, error) {
	cluster, err := r.resolveCluster(ctx, params)
	if err != nil {
		return nil, nil, err
	}

	client := proxmox.NewClient(proxmox.ClientConfig{
		Host:          cluster.Host,
		Port:          cluster.Port,
		TokenID:       cluster.APITokenID,
		TokenSecret:   cluster.APITokenSecret,
		SkipTLSVerify: cluster.SkipTLSVerify,
	})

	return client, cluster, nil
}

func (r *ToolRegistry) resolveCluster(ctx context.Context, params proxmoxToolArgs) (*models.Cluster, error) {
	if r.clusterStore == nil {
		return nil, fmt.Errorf("cluster store is not configured")
	}

	if clusterID := strings.TrimSpace(params.ClusterID); clusterID != "" {
		cluster, err := r.clusterStore.GetByID(ctx, clusterID)
		if err != nil {
			return nil, fmt.Errorf("load cluster %s: %w", clusterID, err)
		}
		return cluster, nil
	}

	clusters, err := r.clusterStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}
	if len(clusters) == 0 {
		return nil, fmt.Errorf("no Proxmox clusters are configured")
	}

	if clusterName := strings.TrimSpace(params.ClusterName); clusterName != "" {
		for _, cluster := range clusters {
			if strings.EqualFold(cluster.Name, clusterName) {
				return r.clusterStore.GetByID(ctx, cluster.ID)
			}
		}
		return nil, fmt.Errorf("cluster named %q was not found", clusterName)
	}

	if r.defaultClusterID != "" {
		cluster, err := r.clusterStore.GetByID(ctx, r.defaultClusterID)
		if err == nil {
			return cluster, nil
		}
	}

	if len(clusters) == 1 {
		return r.clusterStore.GetByID(ctx, clusters[0].ID)
	}

	clusterNames := make([]string, 0, len(clusters))
	for _, cluster := range clusters {
		clusterNames = append(clusterNames, cluster.Name)
	}

	return nil, fmt.Errorf("multiple clusters are configured (%s); specify cluster_id or cluster_name", strings.Join(clusterNames, ", "))
}

func (p proxmoxToolArgs) proxmoxToolArgs() proxmoxToolArgs {
	return p
}

func (r *ToolRegistry) resolveKubernetesCluster(ctx context.Context, clusterID string, clusterName string) (*models.KubernetesCluster, error) {
	if r.kubernetesStore == nil {
		return nil, fmt.Errorf("kubernetes cluster store is not configured")
	}

	if selectedID := strings.TrimSpace(clusterID); selectedID != "" {
		cluster, err := r.kubernetesStore.GetByID(ctx, selectedID)
		if err != nil {
			return nil, fmt.Errorf("load kubernetes cluster %s: %w", selectedID, err)
		}
		return cluster, nil
	}

	clusters, err := r.kubernetesStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list kubernetes clusters: %w", err)
	}
	if len(clusters) == 0 {
		return nil, fmt.Errorf("no Kubernetes clusters are configured")
	}

	if selectedName := strings.TrimSpace(clusterName); selectedName != "" {
		for _, cluster := range clusters {
			if strings.EqualFold(cluster.Name, selectedName) {
				return r.kubernetesStore.GetByID(ctx, cluster.ID)
			}
		}
		return nil, fmt.Errorf("kubernetes cluster named %q was not found", selectedName)
	}

	if r.defaultKubernetesClusterID != "" {
		cluster, err := r.kubernetesStore.GetByID(ctx, r.defaultKubernetesClusterID)
		if err == nil {
			return cluster, nil
		}
	}

	if len(clusters) == 1 {
		return r.kubernetesStore.GetByID(ctx, clusters[0].ID)
	}

	names := make([]string, 0, len(clusters))
	for _, cluster := range clusters {
		names = append(names, cluster.Name)
	}

	return nil, fmt.Errorf("multiple Kubernetes clusters are configured (%s); specify cluster_id or cluster_name", strings.Join(names, ", "))
}

func (r *ToolRegistry) resolveKubernetesSession(ctx context.Context, clusterID string, clusterName string) (*ik8s.Session, *models.KubernetesCluster, error) {
	cluster, err := r.resolveKubernetesCluster(ctx, clusterID, clusterName)
	if err != nil {
		return nil, nil, err
	}

	session, err := ik8s.NewSessionFromKubeconfig(cluster.Kubeconfig, cluster.ContextName)
	if err != nil {
		return nil, nil, fmt.Errorf("initialize kubernetes session for cluster %s: %w", cluster.Name, err)
	}

	return session, cluster, nil
}
