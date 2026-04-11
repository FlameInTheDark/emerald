package nodedefs

import (
	"strings"

	"github.com/FlameInTheDark/emerald/pkg/pluginapi"
)

const (
	colorTrigger = "#f59e0b"
	colorAction  = "#10b981"
	colorTool    = "#38bdf8"
	colorLogic   = "#8b5cf6"
	colorLLM     = "#ec4899"
	colorVisual  = "#64748b"
)

func BuiltinDefinitions() []Definition {
	return []Definition{
		builtin("trigger:manual", "trigger", "Manual Trigger", "Start pipeline manually", "zap", colorTrigger, map[string]any{}),
		builtin("trigger:cron", "trigger", "Cron Trigger", "Schedule pipeline with cron expression", "clock", colorTrigger, map[string]any{"schedule": "0 * * * *", "timezone": "UTC"}),
		builtin("trigger:webhook", "trigger", "Webhook Trigger", "Trigger pipeline via HTTP webhook", "webhook", colorTrigger, map[string]any{"path": "", "method": "POST"}),
		builtin("trigger:channel_message", "trigger", "Channel Message", "Trigger when a connected channel user sends a message", "message-square", colorTrigger, map[string]any{"channelId": ""}),

		builtin("action:proxmox_list_nodes", "action", "List Nodes", "List nodes in a selected Proxmox cluster", "globe", colorAction, map[string]any{"clusterId": ""}),
		builtin("action:proxmox_list_workloads", "action", "List VMs/CTs", "List virtual machines and containers in a cluster", "link", colorAction, map[string]any{"clusterId": "", "node": ""}),
		builtin("action:vm_start", "action", "Start VM", "Start a Proxmox VM", "play", colorAction, map[string]any{"clusterId": "", "node": "", "vmid": 0}),
		builtin("action:vm_stop", "action", "Stop VM", "Stop a Proxmox VM immediately", "square", colorAction, map[string]any{"clusterId": "", "node": "", "vmid": 0}),
		builtin("action:vm_clone", "action", "Clone VM", "Clone a Proxmox VM", "copy", colorAction, map[string]any{"clusterId": "", "node": "", "vmid": 0, "newName": "", "newId": 0}),
		builtin("action:http", "action", "HTTP Request", "Make an HTTP request", "globe", colorAction, map[string]any{"url": "", "method": "GET", "headers": map[string]any{}, "body": ""}),
		builtin("action:shell_command", "action", "Shell Command", "Run a local shell command in the workspace", "code", colorAction, map[string]any{"command": "", "workingDirectory": "", "timeoutSeconds": 60}),
		builtin("action:lua", "action", "Lua Script", "Execute custom Lua code", "code", colorAction, map[string]any{"script": "-- Write your Lua code here\nreturn { status = \"ok\" }"}),
		builtin("action:channel_send_message", "action", "Send Channel Message", "Send a reply or outbound message through an active channel", "send", colorAction, map[string]any{"channelId": "", "recipient": "", "message": ""}),
		builtin("action:channel_reply_message", "action", "Reply To Message", "Send a new message as a reply to an existing channel message", "corner-down-left", colorAction, map[string]any{"channelId": "", "recipient": "", "replyToMessageId": "", "message": ""}),
		builtin("action:channel_edit_message", "action", "Edit Channel Message", "Edit a previously sent channel message by message ID", "wrench", colorAction, map[string]any{"channelId": "", "recipient": "", "messageId": "", "message": ""}),
		builtin("action:channel_send_and_wait", "action", "Send And Wait", "Send a channel message and wait for the user reply", "message-square", colorAction, map[string]any{"channelId": "", "recipient": "", "message": "", "timeoutSeconds": 300}),
		builtin("action:pipeline_get", "action", "Get Pipeline", "Load pipeline data for a selected pipeline", "workflow", colorAction, map[string]any{"pipelineId": "", "includeDefinition": true}),
		builtin("action:pipeline_run", "action", "Run Pipeline", "Run another pipeline manually and pass JSON parameters", "workflow", colorAction, map[string]any{"pipelineId": "", "params": ""}),

		builtin("action:kubernetes_api_resources", "action", "K8s API Resources", "Read the API resources available on a Kubernetes cluster", "globe", colorAction, map[string]any{"clusterId": ""}),
		builtin("action:kubernetes_list_resources", "action", "K8s List Resources", "List Kubernetes resources by apiVersion and kind or resource name", "list", colorAction, map[string]any{"clusterId": "", "namespace": "", "apiVersion": "v1", "kind": "", "resource": "", "labelSelector": "", "fieldSelector": "", "allNamespaces": false, "limit": 0}),
		builtin("action:kubernetes_get_resource", "action", "K8s Get Resource", "Fetch a single Kubernetes resource", "workflow", colorAction, map[string]any{"clusterId": "", "namespace": "", "apiVersion": "v1", "kind": "", "resource": "", "name": ""}),
		builtin("action:kubernetes_apply_manifest", "action", "K8s Apply Manifest", "Apply Kubernetes manifests with server-side apply", "copy", colorAction, map[string]any{"clusterId": "", "namespace": "", "manifest": "", "fieldManager": "emerald", "force": false}),
		builtin("action:kubernetes_patch_resource", "action", "K8s Patch Resource", "Patch a Kubernetes resource", "wrench", colorAction, map[string]any{"clusterId": "", "namespace": "", "apiVersion": "apps/v1", "kind": "", "resource": "", "name": "", "patchType": "merge", "patch": ""}),
		builtin("action:kubernetes_delete_resource", "action", "K8s Delete Resource", "Delete a Kubernetes resource or matching collection", "trash-2", colorAction, map[string]any{"clusterId": "", "namespace": "", "apiVersion": "apps/v1", "kind": "", "resource": "", "name": "", "labelSelector": "", "fieldSelector": "", "propagationPolicy": "background"}),
		builtin("action:kubernetes_scale_resource", "action", "K8s Scale Resource", "Update workload replica count", "workflow", colorAction, map[string]any{"clusterId": "", "namespace": "", "apiVersion": "apps/v1", "kind": "Deployment", "resource": "", "name": "", "replicas": 1}),
		builtin("action:kubernetes_rollout_restart", "action", "K8s Rollout Restart", "Restart a rollout-capable workload", "refresh-cw", colorAction, map[string]any{"clusterId": "", "namespace": "", "apiVersion": "apps/v1", "kind": "Deployment", "resource": "", "name": ""}),
		builtin("action:kubernetes_rollout_status", "action", "K8s Rollout Status", "Wait for a workload rollout to become ready", "refresh-cw", colorAction, map[string]any{"clusterId": "", "namespace": "", "apiVersion": "apps/v1", "kind": "Deployment", "resource": "", "name": "", "timeoutSeconds": 300}),
		builtin("action:kubernetes_pod_logs", "action", "K8s Pod Logs", "Read logs from a Kubernetes pod", "list", colorAction, map[string]any{"clusterId": "", "namespace": "", "name": "", "container": "", "tailLines": 0, "sinceSeconds": 0, "timestamps": false, "previous": false}),
		builtin("action:kubernetes_pod_exec", "action", "K8s Pod Exec", "Run a command in a Kubernetes pod", "code", colorAction, map[string]any{"clusterId": "", "namespace": "", "name": "", "container": "", "command": []string{"sh", "-c", "echo hello"}}),
		builtin("action:kubernetes_events", "action", "K8s Events", "List recent Kubernetes events", "message-square", colorAction, map[string]any{"clusterId": "", "namespace": "", "limit": 50, "fieldSelector": "", "involvedObjectName": "", "involvedObjectKind": "", "involvedObjectUID": ""}),

		builtin("tool:proxmox_list_nodes", "tool", "List Nodes Tool", "Expose a tool that lists nodes in a selected Proxmox cluster", "globe", colorTool, map[string]any{"clusterId": ""}),
		builtin("tool:proxmox_list_workloads", "tool", "List VMs/CTs Tool", "Expose a tool that lists workloads in a selected Proxmox cluster", "link", colorTool, map[string]any{"clusterId": "", "node": ""}),
		builtin("tool:vm_start", "tool", "Start VM Tool", "Expose a tool that starts a VM in a selected Proxmox cluster", "play", colorTool, map[string]any{"clusterId": "", "node": "", "vmid": 0}),
		builtin("tool:vm_stop", "tool", "Stop VM Tool", "Expose a tool that stops a VM in a selected Proxmox cluster", "square", colorTool, map[string]any{"clusterId": "", "node": "", "vmid": 0}),
		builtin("tool:vm_clone", "tool", "Clone VM Tool", "Expose a tool that clones a VM in a selected Proxmox cluster", "copy", colorTool, map[string]any{"clusterId": "", "node": "", "vmid": 0, "newName": "", "newId": 0}),
		builtin("tool:http", "tool", "HTTP Request Tool", "Expose a tool that makes HTTP requests", "globe", colorTool, map[string]any{"url": "", "method": "GET", "headers": map[string]any{}, "body": ""}),
		builtin("tool:shell_command", "tool", "Shell Command Tool", "Expose a tool that runs local shell commands in the workspace", "code", colorTool, map[string]any{"command": "", "workingDirectory": "", "timeoutSeconds": 60}),
		builtin("tool:pipeline_list", "tool", "List Pipelines Tool", "Expose a tool that lists available pipelines to an agent", "list", colorTool, map[string]any{}),
		builtin("tool:pipeline_get", "tool", "Get Pipeline Tool", "Expose a tool that returns data for a selected pipeline", "workflow", colorTool, map[string]any{"pipelineId": "", "includeDefinition": true, "toolName": "", "toolDescription": "", "allowModelPipelineId": false}),
		builtin("tool:pipeline_create", "tool", "Create Pipeline Tool", "Expose a tool that creates new pipelines", "workflow", colorTool, map[string]any{"toolName": "", "toolDescription": ""}),
		builtin("tool:pipeline_update", "tool", "Update Pipeline Tool", "Expose a tool that updates existing pipelines", "wrench", colorTool, map[string]any{"toolName": "", "toolDescription": ""}),
		builtin("tool:pipeline_delete", "tool", "Delete Pipeline Tool", "Expose a tool that deletes pipelines", "trash-2", colorTool, map[string]any{"toolName": "", "toolDescription": ""}),
		builtin("tool:pipeline_run", "tool", "Run Pipeline Tool", "Expose a tool that runs a selected pipeline and returns its output", "wrench", colorTool, map[string]any{"pipelineId": "", "toolName": "", "toolDescription": "", "allowModelPipelineId": false, "arguments": []any{}}),
		builtin("tool:channel_send_and_wait", "tool", "Send And Wait Tool", "Expose a tool that messages a channel user and waits for their reply", "message-square", colorTool, map[string]any{"channelId": "", "recipient": "", "message": "", "timeoutSeconds": 300}),

		builtin("tool:kubernetes_api_resources", "tool", "K8s API Resources Tool", "Expose a tool that reads the API resources available on a Kubernetes cluster", "globe", colorTool, map[string]any{"clusterId": ""}),
		builtin("tool:kubernetes_list_resources", "tool", "K8s List Resources Tool", "Expose a tool that lists Kubernetes resources", "list", colorTool, map[string]any{"clusterId": "", "namespace": "", "apiVersion": "v1", "kind": "", "resource": "", "labelSelector": "", "fieldSelector": "", "allNamespaces": false, "limit": 0}),
		builtin("tool:kubernetes_get_resource", "tool", "K8s Get Resource Tool", "Expose a tool that fetches a single Kubernetes resource", "workflow", colorTool, map[string]any{"clusterId": "", "namespace": "", "apiVersion": "v1", "kind": "", "resource": "", "name": ""}),
		builtin("tool:kubernetes_apply_manifest", "tool", "K8s Apply Manifest Tool", "Expose a tool that applies Kubernetes manifests", "copy", colorTool, map[string]any{"clusterId": "", "namespace": "", "manifest": "", "fieldManager": "emerald", "force": false}),
		builtin("tool:kubernetes_patch_resource", "tool", "K8s Patch Resource Tool", "Expose a tool that patches a Kubernetes resource", "wrench", colorTool, map[string]any{"clusterId": "", "namespace": "", "apiVersion": "apps/v1", "kind": "", "resource": "", "name": "", "patchType": "merge", "patch": ""}),
		builtin("tool:kubernetes_delete_resource", "tool", "K8s Delete Resource Tool", "Expose a tool that deletes Kubernetes resources", "trash-2", colorTool, map[string]any{"clusterId": "", "namespace": "", "apiVersion": "apps/v1", "kind": "", "resource": "", "name": "", "labelSelector": "", "fieldSelector": "", "propagationPolicy": "background"}),
		builtin("tool:kubernetes_scale_resource", "tool", "K8s Scale Resource Tool", "Expose a tool that scales a workload", "workflow", colorTool, map[string]any{"clusterId": "", "namespace": "", "apiVersion": "apps/v1", "kind": "Deployment", "resource": "", "name": "", "replicas": 1}),
		builtin("tool:kubernetes_rollout_restart", "tool", "K8s Rollout Restart Tool", "Expose a tool that restarts a rollout-capable workload", "refresh-cw", colorTool, map[string]any{"clusterId": "", "namespace": "", "apiVersion": "apps/v1", "kind": "Deployment", "resource": "", "name": ""}),
		builtin("tool:kubernetes_rollout_status", "tool", "K8s Rollout Status Tool", "Expose a tool that waits for a rollout to become ready", "refresh-cw", colorTool, map[string]any{"clusterId": "", "namespace": "", "apiVersion": "apps/v1", "kind": "Deployment", "resource": "", "name": "", "timeoutSeconds": 300}),
		builtin("tool:kubernetes_pod_logs", "tool", "K8s Pod Logs Tool", "Expose a tool that reads logs from a Kubernetes pod", "list", colorTool, map[string]any{"clusterId": "", "namespace": "", "name": "", "container": "", "tailLines": 0, "sinceSeconds": 0, "timestamps": false, "previous": false}),
		builtin("tool:kubernetes_pod_exec", "tool", "K8s Pod Exec Tool", "Expose a tool that runs a command in a Kubernetes pod", "code", colorTool, map[string]any{"clusterId": "", "namespace": "", "name": "", "container": "", "command": []string{"sh", "-c", "echo hello"}}),
		builtin("tool:kubernetes_events", "tool", "K8s Events Tool", "Expose a tool that lists recent Kubernetes events", "message-square", colorTool, map[string]any{"clusterId": "", "namespace": "", "limit": 50, "fieldSelector": "", "involvedObjectName": "", "involvedObjectKind": "", "involvedObjectUID": ""}),

		builtin("logic:condition", "logic", "Condition", "Evaluate a condition (if/else)", "git-branch", colorLogic, map[string]any{"expression": ""}),
		builtin("logic:switch", "logic", "Switch", "Evaluate multiple conditions and fan out to every matching branch", "split", colorLogic, map[string]any{"conditions": []any{map[string]any{"id": "condition-1", "label": "Condition 1", "expression": ""}}}),
		builtin("logic:merge", "logic", "Merge", "Merge multiple upstream object outputs into one payload", "workflow", colorLogic, map[string]any{"mode": "shallow"}),
		builtin("logic:aggregate", "logic", "Aggregate", "Collect multiple upstream outputs into ordered arrays with source metadata and optional output id overrides", "list", colorLogic, map[string]any{"idOverrides": map[string]any{}}),
		builtin("logic:return", "logic", "Return", "Return data from this pipeline to the caller and stop execution", "corner-down-left", colorLogic, map[string]any{"value": ""}),

		builtin("llm:prompt", "llm", "LLM Prompt", "Send a prompt to an LLM provider", "brain", colorLLM, map[string]any{"providerId": "", "prompt": "", "model": "", "temperature": 0.7, "max_tokens": 1024}),
		builtin("llm:agent", "llm", "LLM Agent", "Run a multi-turn LLM agent with connected tool nodes", "bot", colorLLM, map[string]any{"providerId": "", "prompt": "", "model": "", "temperature": 0.7, "max_tokens": 1024, "enableSkills": false}),

		builtin("visual:group", "visual", "Group", "Visual canvas group that keeps related nodes together without affecting execution", "square", colorVisual, map[string]any{"color": colorVisual}),
	}
}

func builtin(nodeType string, category string, label string, description string, icon string, color string, defaultConfig map[string]any) Definition {
	return Definition{
		Type:          nodeType,
		Category:      category,
		Source:        SourceBuiltin,
		Label:         label,
		Description:   description,
		Icon:          icon,
		Color:         color,
		MenuPath:      defaultMenuPathForDefinition(nodeType, category, nil),
		DefaultConfig: defaultConfig,
		Fields:        nil,
		Outputs:       nil,
		OutputHints:   nil,
	}
}

func pluginDefinitionFromBinding(binding anyBinding) Definition {
	return Definition{
		Type:          binding.Type,
		Category:      string(binding.Spec.Kind),
		Source:        SourcePlugin,
		PluginID:      binding.PluginID,
		PluginName:    binding.PluginName,
		Label:         binding.Spec.Label,
		Description:   binding.Spec.Description,
		Icon:          binding.Spec.Icon,
		Color:         binding.Spec.Color,
		MenuPath:      defaultMenuPathForDefinition(binding.Type, string(binding.Spec.Kind), binding.Spec.MenuPath),
		DefaultConfig: binding.Spec.DefaultConfig,
		Fields:        append([]pluginapi.FieldSpec(nil), binding.Spec.Fields...),
		Outputs:       append([]pluginapi.OutputHandle(nil), binding.Spec.Outputs...),
		OutputHints:   append([]pluginapi.OutputHint(nil), binding.Spec.OutputHints...),
	}
}

func defaultMenuPathForDefinition(nodeType string, category string, configured []string) []string {
	if configured != nil {
		return normalizeMenuPath(configured)
	}

	if category != "action" && category != "tool" {
		return nil
	}

	switch {
	case isProxmoxNodeType(nodeType):
		return []string{"Proxmox"}
	case isKubernetesNodeType(nodeType):
		return []string{"Kubernetes"}
	default:
		return []string{"General"}
	}
}

func normalizeMenuPath(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

func isProxmoxNodeType(nodeType string) bool {
	switch nodeType {
	case "action:proxmox_list_nodes",
		"action:proxmox_list_workloads",
		"action:vm_start",
		"action:vm_stop",
		"action:vm_clone",
		"tool:proxmox_list_nodes",
		"tool:proxmox_list_workloads",
		"tool:vm_start",
		"tool:vm_stop",
		"tool:vm_clone":
		return true
	default:
		return false
	}
}

func isKubernetesNodeType(nodeType string) bool {
	return strings.Contains(nodeType, ":kubernetes_")
}
