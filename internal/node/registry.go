package node

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/FlameInTheDark/emerald/internal/llm"
)

type NodeType string

const (
	TypeTriggerManual                  NodeType = "trigger:manual"
	TypeTriggerCron                    NodeType = "trigger:cron"
	TypeTriggerWebhook                 NodeType = "trigger:webhook"
	TypeTriggerChannel                 NodeType = "trigger:channel_message"
	TypeActionListNodes                NodeType = "action:proxmox_list_nodes"
	TypeActionListVMsCTs               NodeType = "action:proxmox_list_workloads"
	TypeActionVMStart                  NodeType = "action:vm_start"
	TypeActionVMStop                   NodeType = "action:vm_stop"
	TypeActionVMClone                  NodeType = "action:vm_clone"
	TypeActionKubernetesAPIResources   NodeType = "action:kubernetes_api_resources"
	TypeActionKubernetesListResources  NodeType = "action:kubernetes_list_resources"
	TypeActionKubernetesGetResource    NodeType = "action:kubernetes_get_resource"
	TypeActionKubernetesApplyManifest  NodeType = "action:kubernetes_apply_manifest"
	TypeActionKubernetesPatchResource  NodeType = "action:kubernetes_patch_resource"
	TypeActionKubernetesDeleteResource NodeType = "action:kubernetes_delete_resource"
	TypeActionKubernetesScaleResource  NodeType = "action:kubernetes_scale_resource"
	TypeActionKubernetesRolloutRestart NodeType = "action:kubernetes_rollout_restart"
	TypeActionKubernetesRolloutStatus  NodeType = "action:kubernetes_rollout_status"
	TypeActionKubernetesPodLogs        NodeType = "action:kubernetes_pod_logs"
	TypeActionKubernetesPodExec        NodeType = "action:kubernetes_pod_exec"
	TypeActionKubernetesEvents         NodeType = "action:kubernetes_events"
	TypeActionHTTP                     NodeType = "action:http"
	TypeActionShell                    NodeType = "action:shell_command"
	TypeActionLua                      NodeType = "action:lua"
	TypeActionChannelSend              NodeType = "action:channel_send_message"
	TypeActionChannelReply             NodeType = "action:channel_reply_message"
	TypeActionChannelEdit              NodeType = "action:channel_edit_message"
	TypeActionChannelWait              NodeType = "action:channel_send_and_wait"
	TypeActionGetPipeline              NodeType = "action:pipeline_get"
	TypeActionRunPipeline              NodeType = "action:pipeline_run"
	TypeToolListNodes                  NodeType = "tool:proxmox_list_nodes"
	TypeToolListVMsCTs                 NodeType = "tool:proxmox_list_workloads"
	TypeToolVMStart                    NodeType = "tool:vm_start"
	TypeToolVMStop                     NodeType = "tool:vm_stop"
	TypeToolVMClone                    NodeType = "tool:vm_clone"
	TypeToolKubernetesAPIResources     NodeType = "tool:kubernetes_api_resources"
	TypeToolKubernetesListResources    NodeType = "tool:kubernetes_list_resources"
	TypeToolKubernetesGetResource      NodeType = "tool:kubernetes_get_resource"
	TypeToolKubernetesApplyManifest    NodeType = "tool:kubernetes_apply_manifest"
	TypeToolKubernetesPatchResource    NodeType = "tool:kubernetes_patch_resource"
	TypeToolKubernetesDeleteResource   NodeType = "tool:kubernetes_delete_resource"
	TypeToolKubernetesScaleResource    NodeType = "tool:kubernetes_scale_resource"
	TypeToolKubernetesRolloutRestart   NodeType = "tool:kubernetes_rollout_restart"
	TypeToolKubernetesRolloutStatus    NodeType = "tool:kubernetes_rollout_status"
	TypeToolKubernetesPodLogs          NodeType = "tool:kubernetes_pod_logs"
	TypeToolKubernetesPodExec          NodeType = "tool:kubernetes_pod_exec"
	TypeToolKubernetesEvents           NodeType = "tool:kubernetes_events"
	TypeToolHTTP                       NodeType = "tool:http"
	TypeToolShell                      NodeType = "tool:shell_command"
	TypeToolListPipelines              NodeType = "tool:pipeline_list"
	TypeToolGetPipeline                NodeType = "tool:pipeline_get"
	TypeToolCreatePipeline             NodeType = "tool:pipeline_create"
	TypeToolUpdatePipeline             NodeType = "tool:pipeline_update"
	TypeToolDeletePipeline             NodeType = "tool:pipeline_delete"
	TypeToolRunPipeline                NodeType = "tool:pipeline_run"
	TypeToolChannelWait                NodeType = "tool:channel_send_and_wait"
	TypeLogicCondition                 NodeType = "logic:condition"
	TypeLogicSwitch                    NodeType = "logic:switch"
	TypeLogicMerge                     NodeType = "logic:merge"
	TypeLogicAggregate                 NodeType = "logic:aggregate"
	TypeLogicReturn                    NodeType = "logic:return"
	TypeLLMPrompt                      NodeType = "llm:prompt"
	TypeLLMPromptLegacy                NodeType = "logic:llm_prompt"
	TypeLLMAgent                       NodeType = "llm:agent"
)

type NodeConfig struct {
	ID     string          `json:"id"`
	Type   NodeType        `json:"type"`
	Label  string          `json:"label"`
	Config json.RawMessage `json:"config"`
}

type NodeResult struct {
	Output      json.RawMessage `json:"output"`
	Error       error           `json:"error,omitempty"`
	ReturnValue any             `json:"-"`
}

type NodeExecutor interface {
	Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*NodeResult, error)
	Validate(config json.RawMessage) error
}

type ToolNodeMetadata struct {
	NodeID string
	Label  string
}

type ToolNodeExecutor interface {
	NodeExecutor
	ToolDefinition(ctx context.Context, meta ToolNodeMetadata, config json.RawMessage) (*llm.ToolDefinition, error)
	ExecuteTool(ctx context.Context, config json.RawMessage, args json.RawMessage, input map[string]any) (any, error)
}

type DynamicResolver func(nodeType NodeType) (NodeExecutor, bool)

type Registry struct {
	executors       map[NodeType]NodeExecutor
	dynamicResolver DynamicResolver
}

func NewRegistry() *Registry {
	return &Registry{
		executors: make(map[NodeType]NodeExecutor),
	}
}

func (r *Registry) Register(nodeType NodeType, executor NodeExecutor) {
	r.executors[nodeType] = executor
}

func (r *Registry) SetDynamicResolver(resolver DynamicResolver) {
	r.dynamicResolver = resolver
}

func (r *Registry) Get(nodeType NodeType) (NodeExecutor, error) {
	exec, ok := r.executors[nodeType]
	if !ok {
		if r.dynamicResolver != nil {
			if dynamicExec, dynamicOK := r.dynamicResolver(nodeType); dynamicOK {
				return dynamicExec, nil
			}
		}
		return nil, fmt.Errorf("unknown node type: %s", nodeType)
	}
	return exec, nil
}

func (r *Registry) ListTypes() []NodeType {
	types := make([]NodeType, 0, len(r.executors))
	for t := range r.executors {
		types = append(types, t)
	}
	return types
}
