package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	ik8s "github.com/FlameInTheDark/emerald/internal/kubernetes"
)

type kubernetesToolArgs struct {
	ClusterID   string `json:"cluster_id"`
	ClusterName string `json:"cluster_name"`
}

func (r *ToolRegistry) registerKubernetesTools() {
	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "k8s_api_resources",
			Description: "List Kubernetes API resources that are available on the selected cluster.",
			Parameters:  r.kubernetesToolParameters(nil),
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var params kubernetesToolArgs
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}

		session, cluster, err := r.resolveKubernetesSession(ctx, params.ClusterID, params.ClusterName)
		if err != nil {
			return nil, err
		}

		resources, err := session.ListAPIResources()
		if err != nil {
			return nil, err
		}

		return map[string]any{
			"cluster_id":        cluster.ID,
			"cluster_name":      cluster.Name,
			"context":           cluster.ContextName,
			"default_namespace": cluster.DefaultNamespace,
			"resources":         resources,
		}, nil
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "k8s_list_resources",
			Description: "List Kubernetes resources using api_version and either kind or resource. Namespace is optional.",
			Parameters: r.kubernetesToolParameters(map[string]any{
				"namespace": map[string]any{
					"type":        "string",
					"description": "Optional namespace. Omit to use the cluster default namespace.",
				},
				"api_version": map[string]any{
					"type":        "string",
					"description": "API version such as v1 or apps/v1.",
				},
				"kind": map[string]any{
					"type":        "string",
					"description": "Resource kind such as Pod or Deployment. Optional when resource is provided.",
				},
				"resource": map[string]any{
					"type":        "string",
					"description": "Plural resource name such as pods or deployments. Optional when kind is provided.",
				},
				"label_selector": map[string]any{
					"type":        "string",
					"description": "Optional Kubernetes label selector.",
				},
				"field_selector": map[string]any{
					"type":        "string",
					"description": "Optional Kubernetes field selector.",
				},
				"all_namespaces": map[string]any{
					"type":        "boolean",
					"description": "When true, list resources across all namespaces.",
				},
			}, "api_version"),
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var params struct {
			kubernetesToolArgs
			Namespace     string `json:"namespace"`
			APIVersion    string `json:"api_version"`
			Kind          string `json:"kind"`
			Resource      string `json:"resource"`
			LabelSelector string `json:"label_selector"`
			FieldSelector string `json:"field_selector"`
			AllNamespaces bool   `json:"all_namespaces"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}

		session, cluster, err := r.resolveKubernetesSession(ctx, params.ClusterID, params.ClusterName)
		if err != nil {
			return nil, err
		}

		namespace := resolveKubernetesNamespace(params.Namespace, cluster)
		items, ref, err := session.ListResources(ctx, ik8s.ListOptions{
			Namespace:     namespace,
			APIVersion:    params.APIVersion,
			Kind:          params.Kind,
			Resource:      params.Resource,
			LabelSelector: params.LabelSelector,
			FieldSelector: params.FieldSelector,
			AllNamespaces: params.AllNamespaces,
		})
		if err != nil {
			return nil, err
		}

		return kubernetesToolResult(cluster, ref.Namespace, ref, map[string]any{
			"count": len(items),
			"items": items,
		}), nil
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "k8s_get_resource",
			Description: "Fetch a single Kubernetes resource using api_version and either kind or resource.",
			Parameters: r.kubernetesToolParameters(map[string]any{
				"namespace":   map[string]any{"type": "string", "description": "Optional namespace."},
				"api_version": map[string]any{"type": "string", "description": "API version such as v1 or apps/v1."},
				"kind":        map[string]any{"type": "string", "description": "Resource kind such as Pod or Deployment."},
				"resource":    map[string]any{"type": "string", "description": "Plural resource name such as pods or deployments."},
				"name":        map[string]any{"type": "string", "description": "Resource name."},
			}, "api_version", "name"),
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var params struct {
			kubernetesToolArgs
			Namespace  string `json:"namespace"`
			APIVersion string `json:"api_version"`
			Kind       string `json:"kind"`
			Resource   string `json:"resource"`
			Name       string `json:"name"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}

		session, cluster, err := r.resolveKubernetesSession(ctx, params.ClusterID, params.ClusterName)
		if err != nil {
			return nil, err
		}

		namespace := resolveKubernetesNamespace(params.Namespace, cluster)
		item, ref, err := session.GetResource(ctx, ik8s.GetOptions{
			Namespace:  namespace,
			APIVersion: params.APIVersion,
			Kind:       params.Kind,
			Resource:   params.Resource,
			Name:       params.Name,
		})
		if err != nil {
			return nil, err
		}

		return kubernetesToolResult(cluster, ref.Namespace, ref, map[string]any{"item": item}), nil
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "k8s_apply_manifest",
			Description: "Apply one or more Kubernetes manifests using server-side apply.",
			Parameters: r.kubernetesToolParameters(map[string]any{
				"namespace":     map[string]any{"type": "string", "description": "Optional namespace override for namespaced resources."},
				"manifest":      map[string]any{"type": "string", "description": "YAML or JSON manifest to apply."},
				"field_manager": map[string]any{"type": "string", "description": "Optional field manager name. Defaults to emerald."},
				"force":         map[string]any{"type": "boolean", "description": "Force ownership conflicts during apply."},
			}, "manifest"),
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var params struct {
			kubernetesToolArgs
			Namespace    string `json:"namespace"`
			Manifest     string `json:"manifest"`
			FieldManager string `json:"field_manager"`
			Force        bool   `json:"force"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}

		session, cluster, err := r.resolveKubernetesSession(ctx, params.ClusterID, params.ClusterName)
		if err != nil {
			return nil, err
		}

		namespace := resolveKubernetesNamespace(params.Namespace, cluster)
		items, refs, err := session.ApplyManifest(ctx, ik8s.ApplyOptions{
			Namespace:    namespace,
			Manifest:     params.Manifest,
			FieldManager: params.FieldManager,
			Force:        params.Force,
		})
		if err != nil {
			return nil, err
		}

		return kubernetesToolResult(cluster, namespace, nil, map[string]any{
			"count": len(items),
			"items": items,
			"refs":  refs,
		}), nil
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "k8s_patch_resource",
			Description: "Patch a Kubernetes resource using merge, json, or strategic patch types.",
			Parameters: r.kubernetesToolParameters(map[string]any{
				"namespace":   map[string]any{"type": "string", "description": "Optional namespace."},
				"api_version": map[string]any{"type": "string", "description": "API version such as v1 or apps/v1."},
				"kind":        map[string]any{"type": "string", "description": "Resource kind."},
				"resource":    map[string]any{"type": "string", "description": "Plural resource name."},
				"name":        map[string]any{"type": "string", "description": "Resource name."},
				"patch":       map[string]any{"type": "string", "description": "Patch payload."},
				"patch_type":  map[string]any{"type": "string", "description": "merge, json, or strategic."},
			}, "api_version", "name", "patch"),
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var params struct {
			kubernetesToolArgs
			Namespace  string `json:"namespace"`
			APIVersion string `json:"api_version"`
			Kind       string `json:"kind"`
			Resource   string `json:"resource"`
			Name       string `json:"name"`
			Patch      string `json:"patch"`
			PatchType  string `json:"patch_type"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}

		session, cluster, err := r.resolveKubernetesSession(ctx, params.ClusterID, params.ClusterName)
		if err != nil {
			return nil, err
		}

		namespace := resolveKubernetesNamespace(params.Namespace, cluster)
		item, ref, err := session.PatchResource(ctx, ik8s.PatchOptions{
			Namespace:  namespace,
			APIVersion: params.APIVersion,
			Kind:       params.Kind,
			Resource:   params.Resource,
			Name:       params.Name,
			Patch:      params.Patch,
			PatchType:  params.PatchType,
		})
		if err != nil {
			return nil, err
		}

		return kubernetesToolResult(cluster, ref.Namespace, ref, map[string]any{"item": item}), nil
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "k8s_delete_resource",
			Description: "Delete a Kubernetes resource by name or delete a collection using selectors.",
			Parameters: r.kubernetesToolParameters(map[string]any{
				"namespace":          map[string]any{"type": "string", "description": "Optional namespace."},
				"api_version":        map[string]any{"type": "string", "description": "API version such as v1 or apps/v1."},
				"kind":               map[string]any{"type": "string", "description": "Resource kind."},
				"resource":           map[string]any{"type": "string", "description": "Plural resource name."},
				"name":               map[string]any{"type": "string", "description": "Resource name."},
				"label_selector":     map[string]any{"type": "string", "description": "Optional label selector for collection deletes."},
				"field_selector":     map[string]any{"type": "string", "description": "Optional field selector for collection deletes."},
				"propagation_policy": map[string]any{"type": "string", "description": "background, foreground, or orphan."},
			}, "api_version"),
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var params struct {
			kubernetesToolArgs
			Namespace         string `json:"namespace"`
			APIVersion        string `json:"api_version"`
			Kind              string `json:"kind"`
			Resource          string `json:"resource"`
			Name              string `json:"name"`
			LabelSelector     string `json:"label_selector"`
			FieldSelector     string `json:"field_selector"`
			PropagationPolicy string `json:"propagation_policy"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}

		session, cluster, err := r.resolveKubernetesSession(ctx, params.ClusterID, params.ClusterName)
		if err != nil {
			return nil, err
		}

		namespace := resolveKubernetesNamespace(params.Namespace, cluster)
		result, ref, err := session.DeleteResource(ctx, ik8s.DeleteOptions{
			Namespace:         namespace,
			APIVersion:        params.APIVersion,
			Kind:              params.Kind,
			Resource:          params.Resource,
			Name:              params.Name,
			LabelSelector:     params.LabelSelector,
			FieldSelector:     params.FieldSelector,
			PropagationPolicy: params.PropagationPolicy,
		})
		if err != nil {
			return nil, err
		}

		return kubernetesToolResult(cluster, ref.Namespace, ref, result), nil
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "k8s_scale_resource",
			Description: "Scale a Kubernetes workload by updating spec.replicas.",
			Parameters: r.kubernetesToolParameters(map[string]any{
				"namespace":   map[string]any{"type": "string", "description": "Optional namespace."},
				"api_version": map[string]any{"type": "string", "description": "API version such as apps/v1."},
				"kind":        map[string]any{"type": "string", "description": "Resource kind."},
				"resource":    map[string]any{"type": "string", "description": "Plural resource name."},
				"name":        map[string]any{"type": "string", "description": "Resource name."},
				"replicas":    map[string]any{"type": "integer", "description": "Replica count."},
			}, "api_version", "name", "replicas"),
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var params struct {
			kubernetesToolArgs
			Namespace  string `json:"namespace"`
			APIVersion string `json:"api_version"`
			Kind       string `json:"kind"`
			Resource   string `json:"resource"`
			Name       string `json:"name"`
			Replicas   int64  `json:"replicas"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}

		session, cluster, err := r.resolveKubernetesSession(ctx, params.ClusterID, params.ClusterName)
		if err != nil {
			return nil, err
		}

		namespace := resolveKubernetesNamespace(params.Namespace, cluster)
		item, ref, err := session.ScaleResource(ctx, ik8s.ScaleOptions{
			Namespace:  namespace,
			APIVersion: params.APIVersion,
			Kind:       params.Kind,
			Resource:   params.Resource,
			Name:       params.Name,
			Replicas:   params.Replicas,
		})
		if err != nil {
			return nil, err
		}

		return kubernetesToolResult(cluster, ref.Namespace, ref, map[string]any{"item": item}), nil
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "k8s_rollout_restart",
			Description: "Restart a Deployment, StatefulSet, or DaemonSet by updating its pod template annotation.",
			Parameters: r.kubernetesToolParameters(map[string]any{
				"namespace":   map[string]any{"type": "string", "description": "Optional namespace."},
				"api_version": map[string]any{"type": "string", "description": "API version such as apps/v1."},
				"kind":        map[string]any{"type": "string", "description": "Resource kind."},
				"resource":    map[string]any{"type": "string", "description": "Plural resource name."},
				"name":        map[string]any{"type": "string", "description": "Resource name."},
			}, "api_version", "name"),
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var params struct {
			kubernetesToolArgs
			Namespace  string `json:"namespace"`
			APIVersion string `json:"api_version"`
			Kind       string `json:"kind"`
			Resource   string `json:"resource"`
			Name       string `json:"name"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}

		session, cluster, err := r.resolveKubernetesSession(ctx, params.ClusterID, params.ClusterName)
		if err != nil {
			return nil, err
		}

		namespace := resolveKubernetesNamespace(params.Namespace, cluster)
		item, ref, err := session.RolloutRestart(ctx, ik8s.RolloutRestartOptions{
			Namespace:  namespace,
			APIVersion: params.APIVersion,
			Kind:       params.Kind,
			Resource:   params.Resource,
			Name:       params.Name,
		})
		if err != nil {
			return nil, err
		}

		return kubernetesToolResult(cluster, ref.Namespace, ref, map[string]any{"item": item}), nil
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "k8s_rollout_status",
			Description: "Wait for a Deployment, StatefulSet, or DaemonSet rollout to become ready.",
			Parameters: r.kubernetesToolParameters(map[string]any{
				"namespace":       map[string]any{"type": "string", "description": "Optional namespace."},
				"api_version":     map[string]any{"type": "string", "description": "API version such as apps/v1."},
				"kind":            map[string]any{"type": "string", "description": "Resource kind."},
				"resource":        map[string]any{"type": "string", "description": "Plural resource name."},
				"name":            map[string]any{"type": "string", "description": "Resource name."},
				"timeout_seconds": map[string]any{"type": "integer", "description": "Optional timeout in seconds. Defaults to 300."},
			}, "api_version", "name"),
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var params struct {
			kubernetesToolArgs
			Namespace      string `json:"namespace"`
			APIVersion     string `json:"api_version"`
			Kind           string `json:"kind"`
			Resource       string `json:"resource"`
			Name           string `json:"name"`
			TimeoutSeconds int    `json:"timeout_seconds"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}

		session, cluster, err := r.resolveKubernetesSession(ctx, params.ClusterID, params.ClusterName)
		if err != nil {
			return nil, err
		}

		namespace := resolveKubernetesNamespace(params.Namespace, cluster)
		status, ref, err := session.RolloutStatus(ctx, ik8s.RolloutStatusOptions{
			Namespace:      namespace,
			APIVersion:     params.APIVersion,
			Kind:           params.Kind,
			Resource:       params.Resource,
			Name:           params.Name,
			TimeoutSeconds: params.TimeoutSeconds,
		})
		if err != nil {
			return nil, err
		}

		return kubernetesToolResult(cluster, ref.Namespace, ref, map[string]any{"status": status}), nil
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "k8s_pod_logs",
			Description: "Read logs from a Kubernetes pod.",
			Parameters: r.kubernetesToolParameters(map[string]any{
				"namespace":     map[string]any{"type": "string", "description": "Optional namespace."},
				"name":          map[string]any{"type": "string", "description": "Pod name."},
				"container":     map[string]any{"type": "string", "description": "Optional container name."},
				"tail_lines":    map[string]any{"type": "integer", "description": "Optional number of lines from the end of the log."},
				"since_seconds": map[string]any{"type": "integer", "description": "Optional log age window in seconds."},
				"timestamps":    map[string]any{"type": "boolean", "description": "Include timestamps in log output."},
				"previous":      map[string]any{"type": "boolean", "description": "Read logs from the previous container instance."},
			}, "name"),
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var params struct {
			kubernetesToolArgs
			Namespace    string `json:"namespace"`
			Name         string `json:"name"`
			Container    string `json:"container"`
			TailLines    int64  `json:"tail_lines"`
			SinceSeconds int64  `json:"since_seconds"`
			Timestamps   bool   `json:"timestamps"`
			Previous     bool   `json:"previous"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}

		session, cluster, err := r.resolveKubernetesSession(ctx, params.ClusterID, params.ClusterName)
		if err != nil {
			return nil, err
		}

		namespace := resolveKubernetesNamespace(params.Namespace, cluster)
		logs, ref, err := session.PodLogs(ctx, ik8s.PodLogOptions{
			Namespace:    namespace,
			Name:         params.Name,
			Container:    params.Container,
			TailLines:    params.TailLines,
			SinceSeconds: params.SinceSeconds,
			Timestamps:   params.Timestamps,
			Previous:     params.Previous,
		})
		if err != nil {
			return nil, err
		}

		return kubernetesToolResult(cluster, ref.Namespace, ref, map[string]any{"logs": logs}), nil
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "k8s_pod_exec",
			Description: "Run a non-interactive command inside a Kubernetes pod.",
			Parameters: r.kubernetesToolParameters(map[string]any{
				"namespace": map[string]any{"type": "string", "description": "Optional namespace."},
				"name":      map[string]any{"type": "string", "description": "Pod name."},
				"container": map[string]any{"type": "string", "description": "Optional container name."},
				"command": map[string]any{
					"type":        "array",
					"description": "Command and arguments to execute inside the pod.",
					"items":       map[string]any{"type": "string"},
				},
			}, "name", "command"),
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var params struct {
			kubernetesToolArgs
			Namespace string   `json:"namespace"`
			Name      string   `json:"name"`
			Container string   `json:"container"`
			Command   []string `json:"command"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}

		session, cluster, err := r.resolveKubernetesSession(ctx, params.ClusterID, params.ClusterName)
		if err != nil {
			return nil, err
		}

		namespace := resolveKubernetesNamespace(params.Namespace, cluster)
		result, ref, err := session.PodExec(ctx, ik8s.PodExecOptions{
			Namespace: namespace,
			Name:      params.Name,
			Container: params.Container,
			Command:   params.Command,
		})
		if err != nil {
			return nil, err
		}

		return kubernetesToolResult(cluster, ref.Namespace, ref, map[string]any{"result": result}), nil
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "k8s_events",
			Description: "List Kubernetes events from the selected namespace or cluster default namespace.",
			Parameters: r.kubernetesToolParameters(map[string]any{
				"namespace":            map[string]any{"type": "string", "description": "Optional namespace."},
				"limit":                map[string]any{"type": "integer", "description": "Optional maximum number of events."},
				"field_selector":       map[string]any{"type": "string", "description": "Optional Kubernetes field selector."},
				"involved_object_name": map[string]any{"type": "string", "description": "Optional involved object name."},
				"involved_object_kind": map[string]any{"type": "string", "description": "Optional involved object kind."},
				"involved_object_uid":  map[string]any{"type": "string", "description": "Optional involved object UID."},
			}),
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var params struct {
			kubernetesToolArgs
			Namespace          string `json:"namespace"`
			Limit              int64  `json:"limit"`
			FieldSelector      string `json:"field_selector"`
			InvolvedObjectName string `json:"involved_object_name"`
			InvolvedObjectKind string `json:"involved_object_kind"`
			InvolvedObjectUID  string `json:"involved_object_uid"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}

		session, cluster, err := r.resolveKubernetesSession(ctx, params.ClusterID, params.ClusterName)
		if err != nil {
			return nil, err
		}

		namespace := resolveKubernetesNamespace(params.Namespace, cluster)
		events, ref, err := session.ListEvents(ctx, ik8s.EventOptions{
			Namespace:          namespace,
			Limit:              params.Limit,
			FieldSelector:      params.FieldSelector,
			InvolvedObjectName: params.InvolvedObjectName,
			InvolvedObjectKind: params.InvolvedObjectKind,
			InvolvedObjectUID:  params.InvolvedObjectUID,
		})
		if err != nil {
			return nil, err
		}

		return kubernetesToolResult(cluster, ref.Namespace, ref, map[string]any{
			"count": len(events),
			"items": events,
		}), nil
	})
}

func resolveKubernetesNamespace(namespace string, cluster *models.KubernetesCluster) string {
	if trimmed := strings.TrimSpace(namespace); trimmed != "" {
		return trimmed
	}
	if cluster == nil {
		return ""
	}
	return strings.TrimSpace(cluster.DefaultNamespace)
}

func kubernetesToolResult(cluster *models.KubernetesCluster, namespace string, ref any, payload map[string]any) map[string]any {
	result := map[string]any{
		"cluster_id":        cluster.ID,
		"cluster_name":      cluster.Name,
		"context":           cluster.ContextName,
		"default_namespace": cluster.DefaultNamespace,
	}
	if strings.TrimSpace(namespace) != "" {
		result["namespace"] = strings.TrimSpace(namespace)
	}
	if ref != nil {
		result["resource_ref"] = ref
	}
	for key, value := range payload {
		result[key] = value
	}
	return result
}
