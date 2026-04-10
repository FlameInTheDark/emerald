package action

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	ik8s "github.com/FlameInTheDark/emerald/internal/kubernetes"
	"github.com/FlameInTheDark/emerald/internal/llm"
	"github.com/FlameInTheDark/emerald/internal/node"
	"github.com/FlameInTheDark/emerald/internal/templating"
)

type KubernetesClusterStore interface {
	GetByID(ctx context.Context, id string) (*models.KubernetesCluster, error)
}

type KubernetesOperation string

const (
	KubernetesOperationAPIResources   KubernetesOperation = "api_resources"
	KubernetesOperationListResources  KubernetesOperation = "list_resources"
	KubernetesOperationGetResource    KubernetesOperation = "get_resource"
	KubernetesOperationApplyManifest  KubernetesOperation = "apply_manifest"
	KubernetesOperationPatchResource  KubernetesOperation = "patch_resource"
	KubernetesOperationDeleteResource KubernetesOperation = "delete_resource"
	KubernetesOperationScaleResource  KubernetesOperation = "scale_resource"
	KubernetesOperationRolloutRestart KubernetesOperation = "rollout_restart"
	KubernetesOperationRolloutStatus  KubernetesOperation = "rollout_status"
	KubernetesOperationPodLogs        KubernetesOperation = "pod_logs"
	KubernetesOperationPodExec        KubernetesOperation = "pod_exec"
	KubernetesOperationEvents         KubernetesOperation = "events"
)

type kubernetesNodeConfig struct {
	ClusterID          string   `json:"clusterId"`
	Namespace          string   `json:"namespace"`
	APIVersion         string   `json:"apiVersion"`
	Kind               string   `json:"kind"`
	Resource           string   `json:"resource"`
	Name               string   `json:"name"`
	LabelSelector      string   `json:"labelSelector"`
	FieldSelector      string   `json:"fieldSelector"`
	AllNamespaces      bool     `json:"allNamespaces"`
	Manifest           string   `json:"manifest"`
	FieldManager       string   `json:"fieldManager"`
	Force              bool     `json:"force"`
	Patch              string   `json:"patch"`
	PatchType          string   `json:"patchType"`
	PropagationPolicy  string   `json:"propagationPolicy"`
	Replicas           int64    `json:"replicas"`
	TimeoutSeconds     int      `json:"timeoutSeconds"`
	Container          string   `json:"container"`
	TailLines          int64    `json:"tailLines"`
	SinceSeconds       int64    `json:"sinceSeconds"`
	Timestamps         bool     `json:"timestamps"`
	Previous           bool     `json:"previous"`
	Command            []string `json:"command"`
	Limit              int64    `json:"limit"`
	InvolvedObjectName string   `json:"involvedObjectName"`
	InvolvedObjectKind string   `json:"involvedObjectKind"`
	InvolvedObjectUID  string   `json:"involvedObjectUID"`
}

type kubernetesOperationRunner struct {
	Clusters  KubernetesClusterStore
	Operation KubernetesOperation
}

type KubernetesActionNode struct {
	runner kubernetesOperationRunner
}

type KubernetesToolNode struct {
	runner kubernetesOperationRunner
}

func NewKubernetesActionNode(clusters KubernetesClusterStore, operation KubernetesOperation) *KubernetesActionNode {
	return &KubernetesActionNode{
		runner: kubernetesOperationRunner{
			Clusters:  clusters,
			Operation: operation,
		},
	}
}

func NewKubernetesToolNode(clusters KubernetesClusterStore, operation KubernetesOperation) *KubernetesToolNode {
	return &KubernetesToolNode{
		runner: kubernetesOperationRunner{
			Clusters:  clusters,
			Operation: operation,
		},
	}
}

func (e *KubernetesActionNode) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	return e.runner.Execute(ctx, config, input)
}

func (e *KubernetesActionNode) Validate(config json.RawMessage) error {
	return e.runner.Validate(config, false)
}

func (e *KubernetesToolNode) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	return e.runner.Execute(ctx, config, input)
}

func (e *KubernetesToolNode) Validate(config json.RawMessage) error {
	return e.runner.Validate(config, true)
}

func (e *KubernetesToolNode) ToolDefinition(ctx context.Context, meta node.ToolNodeMetadata, config json.RawMessage) (*llm.ToolDefinition, error) {
	return e.runner.ToolDefinition(ctx, meta, config)
}

func (e *KubernetesToolNode) ExecuteTool(ctx context.Context, config json.RawMessage, args json.RawMessage, input map[string]any) (any, error) {
	return e.runner.ExecuteTool(ctx, config, args, input)
}

func (r kubernetesOperationRunner) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	cfg, err := parseKubernetesNodeConfig(config)
	if err != nil {
		return nil, err
	}
	if err := renderKubernetesNodeConfig(&cfg, input); err != nil {
		return nil, err
	}

	output, err := r.executeWithConfig(ctx, cfg, input)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("marshal kubernetes output: %w", err)
	}

	return &node.NodeResult{Output: data}, nil
}

func (r kubernetesOperationRunner) Validate(config json.RawMessage, tool bool) error {
	cfg, err := parseKubernetesNodeConfig(config)
	if err != nil {
		return err
	}

	return r.validateConfig(config, cfg, tool)
}

func (r kubernetesOperationRunner) ToolDefinition(_ context.Context, meta node.ToolNodeMetadata, config json.RawMessage) (*llm.ToolDefinition, error) {
	cfg, err := parseKubernetesNodeConfig(config)
	if err != nil {
		return nil, err
	}
	if err := r.validateConfig(config, cfg, true); err != nil {
		return nil, err
	}

	name, description, parameters := r.toolSchema(meta, config, cfg)
	return &llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolSpec{
			Name:        name,
			Description: description,
			Parameters:  parameters,
		},
	}, nil
}

func (r kubernetesOperationRunner) ExecuteTool(ctx context.Context, config json.RawMessage, args json.RawMessage, input map[string]any) (any, error) {
	cfg, err := parseKubernetesNodeConfig(config)
	if err != nil {
		return nil, err
	}

	payload, err := parseToolArgMap(args)
	if err != nil {
		return nil, err
	}
	if err := applyKubernetesToolArgs(&cfg, payload); err != nil {
		return nil, err
	}
	if err := renderKubernetesNodeConfig(&cfg, input); err != nil {
		return nil, err
	}

	return r.executeWithConfig(ctx, cfg, input)
}

func (r kubernetesOperationRunner) executeWithConfig(ctx context.Context, cfg kubernetesNodeConfig, input map[string]any) (map[string]any, error) {
	session, cluster, err := loadKubernetesSession(ctx, r.Clusters, cfg.ClusterID, input)
	if err != nil {
		return nil, err
	}

	namespace := resolveKubernetesNodeNamespace(cfg.Namespace, cluster)

	switch r.Operation {
	case KubernetesOperationAPIResources:
		resources, err := session.ListAPIResources()
		if err != nil {
			return nil, err
		}
		return buildKubernetesNodeOutput(cluster, namespace, nil, map[string]any{
			"count":     len(resources),
			"resources": resources,
		}), nil
	case KubernetesOperationListResources:
		items, ref, err := session.ListResources(ctx, ik8s.ListOptions{
			Namespace:     namespace,
			APIVersion:    cfg.APIVersion,
			Kind:          cfg.Kind,
			Resource:      cfg.Resource,
			LabelSelector: cfg.LabelSelector,
			FieldSelector: cfg.FieldSelector,
			AllNamespaces: cfg.AllNamespaces,
			Limit:         cfg.Limit,
		})
		if err != nil {
			return nil, err
		}
		return buildKubernetesNodeOutput(cluster, ref.Namespace, ref, map[string]any{
			"count":         len(items),
			"items":         items,
			"allNamespaces": cfg.AllNamespaces,
		}), nil
	case KubernetesOperationGetResource:
		item, ref, err := session.GetResource(ctx, ik8s.GetOptions{
			Namespace:  namespace,
			APIVersion: cfg.APIVersion,
			Kind:       cfg.Kind,
			Resource:   cfg.Resource,
			Name:       cfg.Name,
		})
		if err != nil {
			return nil, err
		}
		return buildKubernetesNodeOutput(cluster, ref.Namespace, ref, map[string]any{"item": item}), nil
	case KubernetesOperationApplyManifest:
		items, refs, err := session.ApplyManifest(ctx, ik8s.ApplyOptions{
			Namespace:    namespace,
			Manifest:     cfg.Manifest,
			FieldManager: cfg.FieldManager,
			Force:        cfg.Force,
		})
		if err != nil {
			return nil, err
		}
		return buildKubernetesNodeOutput(cluster, namespace, nil, map[string]any{
			"count": len(items),
			"items": items,
			"refs":  refs,
		}), nil
	case KubernetesOperationPatchResource:
		item, ref, err := session.PatchResource(ctx, ik8s.PatchOptions{
			Namespace:  namespace,
			APIVersion: cfg.APIVersion,
			Kind:       cfg.Kind,
			Resource:   cfg.Resource,
			Name:       cfg.Name,
			Patch:      cfg.Patch,
			PatchType:  cfg.PatchType,
		})
		if err != nil {
			return nil, err
		}
		return buildKubernetesNodeOutput(cluster, ref.Namespace, ref, map[string]any{"item": item}), nil
	case KubernetesOperationDeleteResource:
		result, ref, err := session.DeleteResource(ctx, ik8s.DeleteOptions{
			Namespace:         namespace,
			APIVersion:        cfg.APIVersion,
			Kind:              cfg.Kind,
			Resource:          cfg.Resource,
			Name:              cfg.Name,
			LabelSelector:     cfg.LabelSelector,
			FieldSelector:     cfg.FieldSelector,
			PropagationPolicy: cfg.PropagationPolicy,
		})
		if err != nil {
			return nil, err
		}
		return buildKubernetesNodeOutput(cluster, ref.Namespace, ref, result), nil
	case KubernetesOperationScaleResource:
		item, ref, err := session.ScaleResource(ctx, ik8s.ScaleOptions{
			Namespace:  namespace,
			APIVersion: cfg.APIVersion,
			Kind:       cfg.Kind,
			Resource:   cfg.Resource,
			Name:       cfg.Name,
			Replicas:   cfg.Replicas,
		})
		if err != nil {
			return nil, err
		}
		return buildKubernetesNodeOutput(cluster, ref.Namespace, ref, map[string]any{"item": item}), nil
	case KubernetesOperationRolloutRestart:
		item, ref, err := session.RolloutRestart(ctx, ik8s.RolloutRestartOptions{
			Namespace:  namespace,
			APIVersion: cfg.APIVersion,
			Kind:       cfg.Kind,
			Resource:   cfg.Resource,
			Name:       cfg.Name,
		})
		if err != nil {
			return nil, err
		}
		return buildKubernetesNodeOutput(cluster, ref.Namespace, ref, map[string]any{"item": item}), nil
	case KubernetesOperationRolloutStatus:
		status, ref, err := session.RolloutStatus(ctx, ik8s.RolloutStatusOptions{
			Namespace:      namespace,
			APIVersion:     cfg.APIVersion,
			Kind:           cfg.Kind,
			Resource:       cfg.Resource,
			Name:           cfg.Name,
			TimeoutSeconds: cfg.TimeoutSeconds,
		})
		if err != nil {
			return nil, err
		}
		return buildKubernetesNodeOutput(cluster, ref.Namespace, ref, map[string]any{"status": status}), nil
	case KubernetesOperationPodLogs:
		logs, ref, err := session.PodLogs(ctx, ik8s.PodLogOptions{
			Namespace:    namespace,
			Name:         cfg.Name,
			Container:    cfg.Container,
			TailLines:    cfg.TailLines,
			SinceSeconds: cfg.SinceSeconds,
			Timestamps:   cfg.Timestamps,
			Previous:     cfg.Previous,
		})
		if err != nil {
			return nil, err
		}
		return buildKubernetesNodeOutput(cluster, ref.Namespace, ref, map[string]any{"logs": logs}), nil
	case KubernetesOperationPodExec:
		result, ref, err := session.PodExec(ctx, ik8s.PodExecOptions{
			Namespace: namespace,
			Name:      cfg.Name,
			Container: cfg.Container,
			Command:   cfg.Command,
		})
		if err != nil {
			return nil, err
		}
		return buildKubernetesNodeOutput(cluster, ref.Namespace, ref, map[string]any{"result": result}), nil
	case KubernetesOperationEvents:
		items, ref, err := session.ListEvents(ctx, ik8s.EventOptions{
			Namespace:          namespace,
			Limit:              cfg.Limit,
			FieldSelector:      cfg.FieldSelector,
			InvolvedObjectName: cfg.InvolvedObjectName,
			InvolvedObjectKind: cfg.InvolvedObjectKind,
			InvolvedObjectUID:  cfg.InvolvedObjectUID,
		})
		if err != nil {
			return nil, err
		}
		return buildKubernetesNodeOutput(cluster, ref.Namespace, ref, map[string]any{
			"count": len(items),
			"items": items,
		}), nil
	default:
		return nil, fmt.Errorf("unsupported kubernetes operation %q", r.Operation)
	}
}

func (r kubernetesOperationRunner) validateConfig(raw json.RawMessage, cfg kubernetesNodeConfig, tool bool) error {
	if strings.TrimSpace(cfg.ClusterID) == "" {
		return fmt.Errorf("clusterId is required")
	}
	if cfg.TimeoutSeconds < 0 {
		return fmt.Errorf("timeoutSeconds must be 0 or greater")
	}
	if cfg.TailLines < 0 {
		return fmt.Errorf("tailLines must be 0 or greater")
	}
	if cfg.SinceSeconds < 0 {
		return fmt.Errorf("sinceSeconds must be 0 or greater")
	}
	if cfg.Limit < 0 {
		return fmt.Errorf("limit must be 0 or greater")
	}
	if cfg.Replicas < 0 {
		return fmt.Errorf("replicas must be 0 or greater")
	}

	if tool {
		return nil
	}

	requireResourceIdentity := func() error {
		if strings.TrimSpace(cfg.APIVersion) == "" {
			return fmt.Errorf("apiVersion is required")
		}
		if strings.TrimSpace(cfg.Kind) == "" && strings.TrimSpace(cfg.Resource) == "" {
			return fmt.Errorf("kind or resource is required")
		}
		return nil
	}

	switch r.Operation {
	case KubernetesOperationAPIResources, KubernetesOperationEvents:
		return nil
	case KubernetesOperationListResources:
		return requireResourceIdentity()
	case KubernetesOperationGetResource, KubernetesOperationPatchResource, KubernetesOperationRolloutRestart, KubernetesOperationRolloutStatus:
		if err := requireResourceIdentity(); err != nil {
			return err
		}
		if strings.TrimSpace(cfg.Name) == "" {
			return fmt.Errorf("name is required")
		}
		if r.Operation == KubernetesOperationPatchResource && strings.TrimSpace(cfg.Patch) == "" {
			return fmt.Errorf("patch is required")
		}
		return nil
	case KubernetesOperationApplyManifest:
		if strings.TrimSpace(cfg.Manifest) == "" {
			return fmt.Errorf("manifest is required")
		}
		return nil
	case KubernetesOperationDeleteResource:
		if err := requireResourceIdentity(); err != nil {
			return err
		}
		if strings.TrimSpace(cfg.Name) == "" && strings.TrimSpace(cfg.LabelSelector) == "" && strings.TrimSpace(cfg.FieldSelector) == "" {
			return fmt.Errorf("name, labelSelector, or fieldSelector is required")
		}
		return nil
	case KubernetesOperationScaleResource:
		if err := requireResourceIdentity(); err != nil {
			return err
		}
		if strings.TrimSpace(cfg.Name) == "" {
			return fmt.Errorf("name is required")
		}
		if !hasJSONField(raw, "replicas") {
			return fmt.Errorf("replicas is required")
		}
		return nil
	case KubernetesOperationPodLogs:
		if strings.TrimSpace(cfg.Name) == "" {
			return fmt.Errorf("name is required")
		}
		return nil
	case KubernetesOperationPodExec:
		if strings.TrimSpace(cfg.Name) == "" {
			return fmt.Errorf("name is required")
		}
		if len(cfg.Command) == 0 {
			return fmt.Errorf("command is required")
		}
		return nil
	default:
		return nil
	}
}

func (r kubernetesOperationRunner) toolSchema(meta node.ToolNodeMetadata, raw json.RawMessage, cfg kubernetesNodeConfig) (string, string, map[string]any) {
	switch r.Operation {
	case KubernetesOperationAPIResources:
		return sanitizeToolName(meta.Label, "k8s_api_resources"),
			"List Kubernetes API resources that are available on the configured cluster.",
			kubernetesToolParameters(nil)
	case KubernetesOperationListResources:
		return sanitizeToolName(meta.Label, "k8s_list_resources"),
			"List Kubernetes resources using apiVersion and either kind or resource. Namespace is optional.",
			kubernetesToolParameters(map[string]any{
				"namespace":     toolStringProperty("Optional namespace. Leave empty to use the cluster default namespace."),
				"apiVersion":    toolStringProperty("API version such as v1 or apps/v1."),
				"kind":          toolStringProperty("Resource kind such as Pod or Deployment."),
				"resource":      toolStringProperty("Plural resource name such as pods or deployments."),
				"labelSelector": toolStringProperty("Optional Kubernetes label selector."),
				"fieldSelector": toolStringProperty("Optional Kubernetes field selector."),
				"allNamespaces": toolBooleanProperty("When true, list resources across all namespaces."),
				"limit":         toolIntegerProperty("Optional maximum number of resources to return."),
			}, requiredFields(cfg, raw, "apiVersion")...)
	case KubernetesOperationGetResource:
		return sanitizeToolName(meta.Label, "k8s_get_resource"),
			"Get a single Kubernetes resource using apiVersion and either kind or resource.",
			kubernetesToolParameters(map[string]any{
				"namespace":  toolStringProperty("Optional namespace."),
				"apiVersion": toolStringProperty("API version such as v1 or apps/v1."),
				"kind":       toolStringProperty("Resource kind such as Pod or Deployment."),
				"resource":   toolStringProperty("Plural resource name such as pods or deployments."),
				"name":       toolStringProperty("Resource name."),
			}, requiredFields(cfg, raw, "apiVersion", "name")...)
	case KubernetesOperationApplyManifest:
		return sanitizeToolName(meta.Label, "k8s_apply_manifest"),
			"Apply one or more Kubernetes manifests using server-side apply.",
			kubernetesToolParameters(map[string]any{
				"namespace":    toolStringProperty("Optional namespace override for namespaced resources."),
				"manifest":     toolStringProperty("YAML or JSON manifest to apply."),
				"fieldManager": toolStringProperty("Optional field manager name. Defaults to emerald."),
				"force":        toolBooleanProperty("Force ownership conflicts during apply."),
			}, requiredFields(cfg, raw, "manifest")...)
	case KubernetesOperationPatchResource:
		return sanitizeToolName(meta.Label, "k8s_patch_resource"),
			"Patch a Kubernetes resource using merge, json, or strategic patch types.",
			kubernetesToolParameters(map[string]any{
				"namespace":  toolStringProperty("Optional namespace."),
				"apiVersion": toolStringProperty("API version such as v1 or apps/v1."),
				"kind":       toolStringProperty("Resource kind."),
				"resource":   toolStringProperty("Plural resource name."),
				"name":       toolStringProperty("Resource name."),
				"patch":      toolStringProperty("Patch payload."),
				"patchType":  toolStringProperty("merge, json, or strategic."),
			}, requiredFields(cfg, raw, "apiVersion", "name", "patch")...)
	case KubernetesOperationDeleteResource:
		return sanitizeToolName(meta.Label, "k8s_delete_resource"),
			"Delete a Kubernetes resource by name or delete a collection using selectors.",
			kubernetesToolParameters(map[string]any{
				"namespace":         toolStringProperty("Optional namespace."),
				"apiVersion":        toolStringProperty("API version such as v1 or apps/v1."),
				"kind":              toolStringProperty("Resource kind."),
				"resource":          toolStringProperty("Plural resource name."),
				"name":              toolStringProperty("Resource name."),
				"labelSelector":     toolStringProperty("Optional label selector for collection deletes."),
				"fieldSelector":     toolStringProperty("Optional field selector for collection deletes."),
				"propagationPolicy": toolStringProperty("background, foreground, or orphan."),
			}, requiredFields(cfg, raw, "apiVersion")...)
	case KubernetesOperationScaleResource:
		return sanitizeToolName(meta.Label, "k8s_scale_resource"),
			"Scale a Kubernetes workload by updating spec.replicas.",
			kubernetesToolParameters(map[string]any{
				"namespace":  toolStringProperty("Optional namespace."),
				"apiVersion": toolStringProperty("API version such as apps/v1."),
				"kind":       toolStringProperty("Resource kind."),
				"resource":   toolStringProperty("Plural resource name."),
				"name":       toolStringProperty("Resource name."),
				"replicas":   toolIntegerProperty("Replica count."),
			}, requiredFields(cfg, raw, "apiVersion", "name", "replicas")...)
	case KubernetesOperationRolloutRestart:
		return sanitizeToolName(meta.Label, "k8s_rollout_restart"),
			"Restart a Deployment, StatefulSet, or DaemonSet by updating its pod template annotation.",
			kubernetesToolParameters(map[string]any{
				"namespace":  toolStringProperty("Optional namespace."),
				"apiVersion": toolStringProperty("API version such as apps/v1."),
				"kind":       toolStringProperty("Resource kind."),
				"resource":   toolStringProperty("Plural resource name."),
				"name":       toolStringProperty("Resource name."),
			}, requiredFields(cfg, raw, "apiVersion", "name")...)
	case KubernetesOperationRolloutStatus:
		return sanitizeToolName(meta.Label, "k8s_rollout_status"),
			"Wait for a Deployment, StatefulSet, or DaemonSet rollout to become ready.",
			kubernetesToolParameters(map[string]any{
				"namespace":      toolStringProperty("Optional namespace."),
				"apiVersion":     toolStringProperty("API version such as apps/v1."),
				"kind":           toolStringProperty("Resource kind."),
				"resource":       toolStringProperty("Plural resource name."),
				"name":           toolStringProperty("Resource name."),
				"timeoutSeconds": toolIntegerProperty("Optional timeout in seconds. Defaults to 300."),
			}, requiredFields(cfg, raw, "apiVersion", "name")...)
	case KubernetesOperationPodLogs:
		return sanitizeToolName(meta.Label, "k8s_pod_logs"),
			"Read logs from a Kubernetes pod.",
			kubernetesToolParameters(map[string]any{
				"namespace":    toolStringProperty("Optional namespace."),
				"name":         toolStringProperty("Pod name."),
				"container":    toolStringProperty("Optional container name."),
				"tailLines":    toolIntegerProperty("Optional number of lines from the end of the log."),
				"sinceSeconds": toolIntegerProperty("Optional log age window in seconds."),
				"timestamps":   toolBooleanProperty("Include timestamps in log output."),
				"previous":     toolBooleanProperty("Read logs from the previous container instance."),
			}, requiredFields(cfg, raw, "name")...)
	case KubernetesOperationPodExec:
		return sanitizeToolName(meta.Label, "k8s_pod_exec"),
			"Run a non-interactive command inside a Kubernetes pod.",
			kubernetesToolParameters(map[string]any{
				"namespace": toolStringProperty("Optional namespace."),
				"name":      toolStringProperty("Pod name."),
				"container": toolStringProperty("Optional container name."),
				"command": map[string]any{
					"type":        "array",
					"description": "Command and arguments to execute inside the pod.",
					"items":       map[string]any{"type": "string"},
				},
			}, requiredFields(cfg, raw, "name", "command")...)
	case KubernetesOperationEvents:
		return sanitizeToolName(meta.Label, "k8s_events"),
			"List Kubernetes events from the selected namespace or cluster default namespace.",
			kubernetesToolParameters(map[string]any{
				"namespace":          toolStringProperty("Optional namespace."),
				"limit":              toolIntegerProperty("Optional maximum number of events."),
				"fieldSelector":      toolStringProperty("Optional Kubernetes field selector."),
				"involvedObjectName": toolStringProperty("Optional involved object name."),
				"involvedObjectKind": toolStringProperty("Optional involved object kind."),
				"involvedObjectUID":  toolStringProperty("Optional involved object UID."),
			})
	default:
		return sanitizeToolName(meta.Label, "k8s_tool"), "Kubernetes tool.", kubernetesToolParameters(nil)
	}
}

func parseKubernetesNodeConfig(config json.RawMessage) (kubernetesNodeConfig, error) {
	if len(config) == 0 {
		return kubernetesNodeConfig{}, nil
	}

	var cfg kubernetesNodeConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return kubernetesNodeConfig{}, fmt.Errorf("parse config: %w", err)
	}
	cfg.Command = normalizeCommandSlice(cfg.Command)
	return cfg, nil
}

func renderKubernetesNodeConfig(cfg *kubernetesNodeConfig, input map[string]any) error {
	if cfg == nil {
		return nil
	}
	if err := templating.RenderStrings(cfg, input); err != nil {
		return fmt.Errorf("render config: %w", err)
	}
	cfg.Command = normalizeCommandSlice(cfg.Command)
	return nil
}

func loadKubernetesSession(ctx context.Context, store KubernetesClusterStore, clusterID string, input map[string]any) (*ik8s.Session, *models.KubernetesCluster, error) {
	if store == nil {
		return nil, nil, fmt.Errorf("kubernetes cluster store is not configured")
	}

	resolvedClusterID := resolveClusterID(clusterID, input)
	if resolvedClusterID == "" {
		return nil, nil, fmt.Errorf("clusterId is required")
	}

	cluster, err := store.GetByID(ctx, resolvedClusterID)
	if err != nil {
		return nil, nil, fmt.Errorf("load kubernetes cluster %s: %w", resolvedClusterID, err)
	}

	session, err := ik8s.NewSessionFromKubeconfig(cluster.Kubeconfig, cluster.ContextName)
	if err != nil {
		return nil, nil, fmt.Errorf("initialize kubernetes session for cluster %s: %w", cluster.Name, err)
	}

	return session, cluster, nil
}

func buildKubernetesNodeOutput(cluster *models.KubernetesCluster, namespace string, ref any, payload map[string]any) map[string]any {
	effectiveNamespace := strings.TrimSpace(namespace)
	if effectiveNamespace == "" && cluster != nil {
		effectiveNamespace = strings.TrimSpace(cluster.DefaultNamespace)
	}

	result := map[string]any{
		"clusterId":        cluster.ID,
		"clusterName":      cluster.Name,
		"context":          cluster.ContextName,
		"defaultNamespace": cluster.DefaultNamespace,
		"namespace":        effectiveNamespace,
	}
	if ref != nil {
		result["resourceRef"] = ref
	}
	for key, value := range payload {
		result[key] = value
	}

	return result
}

func resolveKubernetesNodeNamespace(namespace string, cluster *models.KubernetesCluster) string {
	if trimmed := strings.TrimSpace(namespace); trimmed != "" {
		return trimmed
	}
	if cluster == nil {
		return ""
	}
	return strings.TrimSpace(cluster.DefaultNamespace)
}

func normalizeCommandSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	command := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		command = append(command, trimmed)
	}

	return command
}

func applyKubernetesToolArgs(cfg *kubernetesNodeConfig, payload map[string]json.RawMessage) error {
	if cfg == nil {
		return nil
	}

	stringFields := map[string]*string{
		"namespace":          &cfg.Namespace,
		"apiVersion":         &cfg.APIVersion,
		"kind":               &cfg.Kind,
		"resource":           &cfg.Resource,
		"name":               &cfg.Name,
		"labelSelector":      &cfg.LabelSelector,
		"fieldSelector":      &cfg.FieldSelector,
		"manifest":           &cfg.Manifest,
		"fieldManager":       &cfg.FieldManager,
		"patch":              &cfg.Patch,
		"patchType":          &cfg.PatchType,
		"propagationPolicy":  &cfg.PropagationPolicy,
		"container":          &cfg.Container,
		"involvedObjectName": &cfg.InvolvedObjectName,
		"involvedObjectKind": &cfg.InvolvedObjectKind,
		"involvedObjectUID":  &cfg.InvolvedObjectUID,
	}
	for key, target := range stringFields {
		if value, present, err := parseOptionalStringArg(payload, key); err != nil {
			return err
		} else if present {
			*target = strings.TrimSpace(value)
		}
	}

	boolAssignments := []struct {
		key    string
		target *bool
	}{
		{key: "allNamespaces", target: &cfg.AllNamespaces},
		{key: "force", target: &cfg.Force},
		{key: "timestamps", target: &cfg.Timestamps},
		{key: "previous", target: &cfg.Previous},
	}
	for _, assignment := range boolAssignments {
		if value, present, err := parseOptionalBoolJSONArg(payload, assignment.key); err != nil {
			return err
		} else if present {
			*assignment.target = value
		}
	}

	int64Assignments := []struct {
		key    string
		target *int64
	}{
		{key: "replicas", target: &cfg.Replicas},
		{key: "tailLines", target: &cfg.TailLines},
		{key: "sinceSeconds", target: &cfg.SinceSeconds},
		{key: "limit", target: &cfg.Limit},
	}
	for _, assignment := range int64Assignments {
		if value, present, err := parseOptionalInt64JSONArg(payload, assignment.key); err != nil {
			return err
		} else if present {
			*assignment.target = value
		}
	}

	if value, present, err := parseOptionalIntJSONArg(payload, "timeoutSeconds"); err != nil {
		return err
	} else if present {
		cfg.TimeoutSeconds = value
	}
	if value, present, err := parseOptionalStringSliceJSONArg(payload, "command"); err != nil {
		return err
	} else if present {
		cfg.Command = value
	}

	return nil
}

func parseOptionalBoolJSONArg(payload map[string]json.RawMessage, key string) (bool, bool, error) {
	raw, ok := payload[key]
	if !ok {
		return false, false, nil
	}
	if strings.TrimSpace(string(raw)) == "null" {
		return false, true, nil
	}

	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, true, fmt.Errorf("parse %s: %w", key, err)
	}

	return value, true, nil
}

func parseOptionalInt64JSONArg(payload map[string]json.RawMessage, key string) (int64, bool, error) {
	raw, ok := payload[key]
	if !ok {
		return 0, false, nil
	}
	if strings.TrimSpace(string(raw)) == "null" {
		return 0, true, nil
	}

	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, true, fmt.Errorf("parse %s: %w", key, err)
	}

	return value, true, nil
}

func parseOptionalIntJSONArg(payload map[string]json.RawMessage, key string) (int, bool, error) {
	raw, ok := payload[key]
	if !ok {
		return 0, false, nil
	}
	if strings.TrimSpace(string(raw)) == "null" {
		return 0, true, nil
	}

	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, true, fmt.Errorf("parse %s: %w", key, err)
	}

	return value, true, nil
}

func parseOptionalStringSliceJSONArg(payload map[string]json.RawMessage, key string) ([]string, bool, error) {
	raw, ok := payload[key]
	if !ok {
		return nil, false, nil
	}
	if strings.TrimSpace(string(raw)) == "null" {
		return nil, true, nil
	}

	var value []string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, true, fmt.Errorf("parse %s: %w", key, err)
	}

	return normalizeCommandSlice(value), true, nil
}

func kubernetesToolParameters(extraProperties map[string]any, required ...string) map[string]any {
	properties := map[string]any{}
	for key, value := range extraProperties {
		properties[key] = value
	}

	parameters := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		parameters["required"] = required
	}

	return parameters
}

func requiredFields(cfg kubernetesNodeConfig, raw json.RawMessage, fields ...string) []string {
	required := make([]string, 0, len(fields))
	for _, field := range fields {
		switch field {
		case "apiVersion":
			if strings.TrimSpace(cfg.APIVersion) == "" {
				required = append(required, field)
			}
		case "name":
			if strings.TrimSpace(cfg.Name) == "" {
				required = append(required, field)
			}
		case "manifest":
			if strings.TrimSpace(cfg.Manifest) == "" {
				required = append(required, field)
			}
		case "patch":
			if strings.TrimSpace(cfg.Patch) == "" {
				required = append(required, field)
			}
		case "command":
			if len(cfg.Command) == 0 {
				required = append(required, field)
			}
		case "replicas":
			if !hasJSONField(raw, field) {
				required = append(required, field)
			}
		}
	}

	return required
}

func toolStringProperty(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func toolBooleanProperty(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func toolIntegerProperty(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}
