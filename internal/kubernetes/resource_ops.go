package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
)

// ListAPIResources returns flattened API discovery information.
func (s *Session) ListAPIResources() ([]APIResourceInfo, error) {
	resourceLists, err := s.discovery.ServerPreferredResources()
	if err != nil && !discovery.IsGroupDiscoveryFailedError(err) {
		return nil, fmt.Errorf("discover api resources: %w", err)
	}

	resources := make([]APIResourceInfo, 0)
	for _, resourceList := range resourceLists {
		for _, resource := range resourceList.APIResources {
			resources = append(resources, APIResourceInfo{
				GroupVersion: resourceList.GroupVersion,
				Name:         resource.Name,
				SingularName: resource.SingularName,
				Kind:         resource.Kind,
				Namespaced:   resource.Namespaced,
				ShortNames:   append([]string(nil), resource.ShortNames...),
				Verbs:        append([]string(nil), resource.Verbs...),
			})
		}
	}

	return resources, nil
}

// ListResources lists unstructured resources for the requested reference.
func (s *Session) ListResources(ctx context.Context, opts ListOptions) ([]map[string]any, ResourceReference, error) {
	ref := ResourceReference{
		APIVersion: strings.TrimSpace(opts.APIVersion),
		Kind:       strings.TrimSpace(opts.Kind),
		Resource:   strings.TrimSpace(opts.Resource),
	}
	mapping, resolvedRef, err := s.resolveMapping(ref)
	if err != nil {
		return nil, ResourceReference{}, err
	}

	namespace := s.resolveNamespace(mapping.Scope.Name() == meta.RESTScopeNameNamespace, opts.Namespace, opts.AllNamespaces)
	resourceClient := s.resourceClient(mapping, namespace)

	listOptions := metav1.ListOptions{
		LabelSelector: strings.TrimSpace(opts.LabelSelector),
		FieldSelector: strings.TrimSpace(opts.FieldSelector),
	}
	if opts.Limit > 0 {
		listOptions.Limit = opts.Limit
	}

	list, err := resourceClient.List(ctx, listOptions)
	if err != nil {
		return nil, ResourceReference{}, fmt.Errorf("list %s: %w", mapping.Resource.Resource, err)
	}

	items := make([]map[string]any, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, sanitizeObject(item.Object))
	}

	resolvedRef.Namespace = namespace
	return items, resolvedRef, nil
}

// GetResource returns a single unstructured resource.
func (s *Session) GetResource(ctx context.Context, opts GetOptions) (map[string]any, ResourceReference, error) {
	ref := ResourceReference{
		APIVersion: strings.TrimSpace(opts.APIVersion),
		Kind:       strings.TrimSpace(opts.Kind),
		Resource:   strings.TrimSpace(opts.Resource),
		Name:       strings.TrimSpace(opts.Name),
	}
	if ref.Name == "" {
		return nil, ResourceReference{}, fmt.Errorf("name is required")
	}

	mapping, resolvedRef, err := s.resolveMapping(ref)
	if err != nil {
		return nil, ResourceReference{}, err
	}

	namespace := s.resolveNamespace(mapping.Scope.Name() == meta.RESTScopeNameNamespace, opts.Namespace, false)
	resourceClient := s.resourceClient(mapping, namespace)
	obj, err := resourceClient.Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil, ResourceReference{}, fmt.Errorf("get %s/%s: %w", mapping.Resource.Resource, ref.Name, err)
	}

	resolvedRef.Name = ref.Name
	resolvedRef.Namespace = namespace
	return sanitizeObject(obj.Object), resolvedRef, nil
}

// ApplyManifest applies one or more manifests using server-side apply.
func (s *Session) ApplyManifest(ctx context.Context, opts ApplyOptions) ([]map[string]any, []ResourceReference, error) {
	manifest := strings.TrimSpace(opts.Manifest)
	if manifest == "" {
		return nil, nil, fmt.Errorf("manifest is required")
	}

	fieldManager := strings.TrimSpace(opts.FieldManager)
	if fieldManager == "" {
		fieldManager = "emerald"
	}

	decoder := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	applied := make([]map[string]any, 0)
	refs := make([]ResourceReference, 0)

	for {
		var raw map[string]any
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				break
			}
			return nil, nil, fmt.Errorf("decode manifest: %w", err)
		}
		if len(raw) == 0 {
			continue
		}

		obj := &unstructured.Unstructured{Object: raw}
		ref := ResourceReference{
			APIVersion: obj.GetAPIVersion(),
			Kind:       obj.GetKind(),
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
		}
		mapping, resolvedRef, err := s.resolveMapping(ref)
		if err != nil {
			return nil, nil, err
		}

		namespace := resolvedRef.Namespace
		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			namespace = strings.TrimSpace(opts.Namespace)
			if namespace == "" {
				namespace = strings.TrimSpace(obj.GetNamespace())
			}
			if namespace == "" {
				namespace = s.Namespace()
			}
			if namespace == "" {
				namespace = corev1.NamespaceDefault
			}
			obj.SetNamespace(namespace)
			resolvedRef.Namespace = namespace
		}
		if strings.TrimSpace(obj.GetName()) == "" {
			return nil, nil, fmt.Errorf("manifest resource %s is missing metadata.name", resolvedRef.Kind)
		}

		data, err := json.Marshal(obj.Object)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal manifest: %w", err)
		}

		resourceClient := s.resourceClient(mapping, namespace)
		force := opts.Force
		appliedObject, err := resourceClient.Patch(
			ctx,
			obj.GetName(),
			types.ApplyPatchType,
			data,
			metav1.PatchOptions{
				FieldManager: fieldManager,
				Force:        &force,
			},
		)
		if err != nil {
			return nil, nil, fmt.Errorf("apply %s/%s: %w", resolvedRef.Kind, obj.GetName(), err)
		}

		resolvedRef.Name = obj.GetName()
		applied = append(applied, sanitizeObject(appliedObject.Object))
		refs = append(refs, resolvedRef)
	}

	return applied, refs, nil
}

// PatchResource applies a JSON, merge, or strategic merge patch.
func (s *Session) PatchResource(ctx context.Context, opts PatchOptions) (map[string]any, ResourceReference, error) {
	ref := ResourceReference{
		APIVersion: strings.TrimSpace(opts.APIVersion),
		Kind:       strings.TrimSpace(opts.Kind),
		Resource:   strings.TrimSpace(opts.Resource),
		Name:       strings.TrimSpace(opts.Name),
	}
	if ref.Name == "" {
		return nil, ResourceReference{}, fmt.Errorf("name is required")
	}
	if strings.TrimSpace(opts.Patch) == "" {
		return nil, ResourceReference{}, fmt.Errorf("patch is required")
	}

	patchType, err := parsePatchType(opts.PatchType)
	if err != nil {
		return nil, ResourceReference{}, err
	}

	mapping, resolvedRef, err := s.resolveMapping(ref)
	if err != nil {
		return nil, ResourceReference{}, err
	}

	namespace := s.resolveNamespace(mapping.Scope.Name() == meta.RESTScopeNameNamespace, opts.Namespace, false)
	resourceClient := s.resourceClient(mapping, namespace)
	patchedObject, err := resourceClient.Patch(ctx, ref.Name, patchType, []byte(opts.Patch), metav1.PatchOptions{})
	if err != nil {
		return nil, ResourceReference{}, fmt.Errorf("patch %s/%s: %w", mapping.Resource.Resource, ref.Name, err)
	}

	resolvedRef.Name = ref.Name
	resolvedRef.Namespace = namespace
	return sanitizeObject(patchedObject.Object), resolvedRef, nil
}

// DeleteResource deletes either a single resource or a matching collection.
func (s *Session) DeleteResource(ctx context.Context, opts DeleteOptions) (map[string]any, ResourceReference, error) {
	ref := ResourceReference{
		APIVersion: strings.TrimSpace(opts.APIVersion),
		Kind:       strings.TrimSpace(opts.Kind),
		Resource:   strings.TrimSpace(opts.Resource),
		Name:       strings.TrimSpace(opts.Name),
	}
	mapping, resolvedRef, err := s.resolveMapping(ref)
	if err != nil {
		return nil, ResourceReference{}, err
	}

	namespace := s.resolveNamespace(mapping.Scope.Name() == meta.RESTScopeNameNamespace, opts.Namespace, false)
	resourceClient := s.resourceClient(mapping, namespace)

	deleteOptions := metav1.DeleteOptions{}
	switch strings.TrimSpace(strings.ToLower(opts.PropagationPolicy)) {
	case "foreground":
		policy := metav1.DeletePropagationForeground
		deleteOptions.PropagationPolicy = &policy
	case "orphan":
		policy := metav1.DeletePropagationOrphan
		deleteOptions.PropagationPolicy = &policy
	case "background", "":
		policy := metav1.DeletePropagationBackground
		deleteOptions.PropagationPolicy = &policy
	default:
		return nil, ResourceReference{}, fmt.Errorf("unsupported propagation policy %q", opts.PropagationPolicy)
	}

	resolvedRef.Namespace = namespace
	if ref.Name != "" {
		if err := resourceClient.Delete(ctx, ref.Name, deleteOptions); err != nil {
			return nil, ResourceReference{}, fmt.Errorf("delete %s/%s: %w", mapping.Resource.Resource, ref.Name, err)
		}
		resolvedRef.Name = ref.Name
		return map[string]any{"deleted": 1, "mode": "single"}, resolvedRef, nil
	}

	if strings.TrimSpace(opts.LabelSelector) == "" && strings.TrimSpace(opts.FieldSelector) == "" {
		return nil, ResourceReference{}, fmt.Errorf("name, labelSelector, or fieldSelector is required")
	}

	list, err := resourceClient.List(ctx, metav1.ListOptions{
		LabelSelector: strings.TrimSpace(opts.LabelSelector),
		FieldSelector: strings.TrimSpace(opts.FieldSelector),
	})
	if err != nil {
		return nil, ResourceReference{}, fmt.Errorf("list resources before delete: %w", err)
	}

	if err := resourceClient.DeleteCollection(ctx, deleteOptions, metav1.ListOptions{
		LabelSelector: strings.TrimSpace(opts.LabelSelector),
		FieldSelector: strings.TrimSpace(opts.FieldSelector),
	}); err != nil {
		return nil, ResourceReference{}, fmt.Errorf("delete %s collection: %w", mapping.Resource.Resource, err)
	}

	return map[string]any{"deleted": len(list.Items), "mode": "collection"}, resolvedRef, nil
}

// ScaleResource updates spec.replicas on scalable resources.
func (s *Session) ScaleResource(ctx context.Context, opts ScaleOptions) (map[string]any, ResourceReference, error) {
	ref := ResourceReference{
		APIVersion: strings.TrimSpace(opts.APIVersion),
		Kind:       strings.TrimSpace(opts.Kind),
		Resource:   strings.TrimSpace(opts.Resource),
		Name:       strings.TrimSpace(opts.Name),
	}
	if ref.Name == "" {
		return nil, ResourceReference{}, fmt.Errorf("name is required")
	}
	if opts.Replicas < 0 {
		return nil, ResourceReference{}, fmt.Errorf("replicas must be 0 or greater")
	}

	mapping, resolvedRef, err := s.resolveMapping(ref)
	if err != nil {
		return nil, ResourceReference{}, err
	}

	namespace := s.resolveNamespace(mapping.Scope.Name() == meta.RESTScopeNameNamespace, opts.Namespace, false)
	resourceClient := s.resourceClient(mapping, namespace)

	obj, err := resourceClient.Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil, ResourceReference{}, fmt.Errorf("get %s/%s: %w", mapping.Resource.Resource, ref.Name, err)
	}

	if err := unstructured.SetNestedField(obj.Object, opts.Replicas, "spec", "replicas"); err != nil {
		return nil, ResourceReference{}, fmt.Errorf("set replicas: %w", err)
	}

	updatedObject, err := resourceClient.Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return nil, ResourceReference{}, fmt.Errorf("update replicas on %s/%s: %w", mapping.Resource.Resource, ref.Name, err)
	}

	resolvedRef.Name = ref.Name
	resolvedRef.Namespace = namespace
	return sanitizeObject(updatedObject.Object), resolvedRef, nil
}

// RolloutRestart updates the pod template annotation to trigger a rollout.
func (s *Session) RolloutRestart(ctx context.Context, opts RolloutRestartOptions) (map[string]any, ResourceReference, error) {
	ref := ResourceReference{
		APIVersion: strings.TrimSpace(opts.APIVersion),
		Kind:       strings.TrimSpace(opts.Kind),
		Resource:   strings.TrimSpace(opts.Resource),
		Name:       strings.TrimSpace(opts.Name),
	}
	if ref.Name == "" {
		return nil, ResourceReference{}, fmt.Errorf("name is required")
	}

	mapping, resolvedRef, err := s.resolveMapping(ref)
	if err != nil {
		return nil, ResourceReference{}, err
	}

	namespace := s.resolveNamespace(mapping.Scope.Name() == meta.RESTScopeNameNamespace, opts.Namespace, false)
	resourceClient := s.resourceClient(mapping, namespace)

	obj, err := resourceClient.Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil, ResourceReference{}, fmt.Errorf("get %s/%s: %w", mapping.Resource.Resource, ref.Name, err)
	}

	annotations, _, err := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "annotations")
	if err != nil {
		return nil, ResourceReference{}, fmt.Errorf("read pod template annotations: %w", err)
	}
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().UTC().Format(time.RFC3339)
	if err := unstructured.SetNestedStringMap(obj.Object, annotations, "spec", "template", "metadata", "annotations"); err != nil {
		return nil, ResourceReference{}, fmt.Errorf("set pod template annotations: %w", err)
	}

	updatedObject, err := resourceClient.Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return nil, ResourceReference{}, fmt.Errorf("restart rollout for %s/%s: %w", mapping.Resource.Resource, ref.Name, err)
	}

	resolvedRef.Name = ref.Name
	resolvedRef.Namespace = namespace
	return sanitizeObject(updatedObject.Object), resolvedRef, nil
}

// RolloutStatus waits until the resource rollout reaches a ready state.
func (s *Session) RolloutStatus(ctx context.Context, opts RolloutStatusOptions) (*RolloutStatusResult, ResourceReference, error) {
	ref := ResourceReference{
		APIVersion: strings.TrimSpace(opts.APIVersion),
		Kind:       strings.TrimSpace(opts.Kind),
		Resource:   strings.TrimSpace(opts.Resource),
		Name:       strings.TrimSpace(opts.Name),
	}
	if ref.Name == "" {
		return nil, ResourceReference{}, fmt.Errorf("name is required")
	}

	mapping, resolvedRef, err := s.resolveMapping(ref)
	if err != nil {
		return nil, ResourceReference{}, err
	}

	namespace := s.resolveNamespace(mapping.Scope.Name() == meta.RESTScopeNameNamespace, opts.Namespace, false)
	resourceClient := s.resourceClient(mapping, namespace)
	resolvedRef.Name = ref.Name
	resolvedRef.Namespace = namespace

	timeout := 300 * time.Second
	if opts.TimeoutSeconds > 0 {
		timeout = time.Duration(opts.TimeoutSeconds) * time.Second
	}

	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		obj, err := resourceClient.Get(pollCtx, ref.Name, metav1.GetOptions{})
		if err != nil {
			return nil, ResourceReference{}, fmt.Errorf("get rollout status for %s/%s: %w", mapping.Resource.Resource, ref.Name, err)
		}

		result, err := buildRolloutStatus(obj)
		if err != nil {
			return nil, ResourceReference{}, err
		}
		if result.Ready {
			return result, resolvedRef, nil
		}

		select {
		case <-pollCtx.Done():
			return nil, ResourceReference{}, fmt.Errorf("wait for rollout status: %w", pollCtx.Err())
		case <-ticker.C:
		}
	}
}

func (s *Session) resolveMapping(ref ResourceReference) (*meta.RESTMapping, ResourceReference, error) {
	if strings.TrimSpace(ref.APIVersion) == "" {
		return nil, ResourceReference{}, fmt.Errorf("apiVersion is required")
	}

	groupVersion, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil {
		return nil, ResourceReference{}, fmt.Errorf("parse apiVersion %q: %w", ref.APIVersion, err)
	}

	if kind := strings.TrimSpace(ref.Kind); kind != "" {
		mapping, err := s.mapper.RESTMapping(groupVersion.WithKind(kind).GroupKind(), groupVersion.Version)
		if err != nil {
			return nil, ResourceReference{}, fmt.Errorf("resolve kind %s: %w", kind, err)
		}

		return mapping, ResourceReference{
			APIVersion: groupVersion.String(),
			Kind:       mapping.GroupVersionKind.Kind,
			Resource:   mapping.Resource.Resource,
			Namespace:  ref.Namespace,
			Name:       ref.Name,
		}, nil
	}

	resourceName := strings.TrimSpace(ref.Resource)
	if resourceName == "" {
		return nil, ResourceReference{}, fmt.Errorf("kind or resource is required")
	}

	gvr, err := s.mapper.ResourceFor(groupVersion.WithResource(resourceName))
	if err != nil {
		return nil, ResourceReference{}, fmt.Errorf("resolve resource %s: %w", resourceName, err)
	}

	gvk, err := s.mapper.KindFor(gvr)
	if err != nil {
		return nil, ResourceReference{}, fmt.Errorf("resolve kind for resource %s: %w", resourceName, err)
	}

	mapping, err := s.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, ResourceReference{}, fmt.Errorf("resolve mapping for resource %s: %w", resourceName, err)
	}

	return mapping, ResourceReference{
		APIVersion: groupVersion.String(),
		Kind:       mapping.GroupVersionKind.Kind,
		Resource:   mapping.Resource.Resource,
		Namespace:  ref.Namespace,
		Name:       ref.Name,
	}, nil
}

func (s *Session) resolveNamespace(namespaced bool, explicitNamespace string, allNamespaces bool) string {
	if !namespaced || allNamespaces {
		return ""
	}
	if namespace := strings.TrimSpace(explicitNamespace); namespace != "" {
		return namespace
	}
	if namespace := strings.TrimSpace(s.namespace); namespace != "" {
		return namespace
	}
	return corev1.NamespaceDefault
}

func (s *Session) resourceClient(mapping *meta.RESTMapping, namespace string) dynamic.ResourceInterface {
	client := s.dynamic.Resource(mapping.Resource)
	if mapping.Scope.Name() != meta.RESTScopeNameNamespace {
		return client
	}
	return client.Namespace(namespace)
}

func parsePatchType(raw string) (types.PatchType, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", "merge":
		return types.MergePatchType, nil
	case "json":
		return types.JSONPatchType, nil
	case "strategic":
		return types.StrategicMergePatchType, nil
	default:
		return "", fmt.Errorf("unsupported patch type %q", raw)
	}
}

func sanitizeObject(value map[string]any) map[string]any {
	copied := make(map[string]any, len(value))
	for key, current := range value {
		if key == "managedFields" {
			continue
		}
		copied[key] = current
	}
	if metadata, ok := copied["metadata"].(map[string]any); ok {
		delete(metadata, "managedFields")
	}
	return copied
}

func buildRolloutStatus(obj *unstructured.Unstructured) (*RolloutStatusResult, error) {
	kind := obj.GetKind()
	name := obj.GetName()

	switch strings.ToLower(kind) {
	case "deployment":
		replicas, _, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas")
		observedGeneration, _, _ := unstructured.NestedInt64(obj.Object, "status", "observedGeneration")
		generation := obj.GetGeneration()
		updated, _, _ := unstructured.NestedInt64(obj.Object, "status", "updatedReplicas")
		ready, _, _ := unstructured.NestedInt64(obj.Object, "status", "readyReplicas")
		available, _, _ := unstructured.NestedInt64(obj.Object, "status", "availableReplicas")
		isReady := observedGeneration >= generation && updated >= replicas && ready >= replicas && available >= replicas

		return &RolloutStatusResult{
			Ready:         isReady,
			Message:       fmt.Sprintf("deployment/%s updated=%d ready=%d replicas=%d", name, updated, ready, replicas),
			Kind:          kind,
			Name:          name,
			Replicas:      replicas,
			Updated:       updated,
			ReadyReplicas: ready,
		}, nil
	case "statefulset":
		replicas, _, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas")
		observedGeneration, _, _ := unstructured.NestedInt64(obj.Object, "status", "observedGeneration")
		generation := obj.GetGeneration()
		updated, _, _ := unstructured.NestedInt64(obj.Object, "status", "updatedReplicas")
		ready, _, _ := unstructured.NestedInt64(obj.Object, "status", "readyReplicas")
		currentRevision, _, _ := unstructured.NestedString(obj.Object, "status", "currentRevision")
		updateRevision, _, _ := unstructured.NestedString(obj.Object, "status", "updateRevision")
		isReady := observedGeneration >= generation && updated >= replicas && ready >= replicas && currentRevision == updateRevision

		return &RolloutStatusResult{
			Ready:         isReady,
			Message:       fmt.Sprintf("statefulset/%s updated=%d ready=%d replicas=%d", name, updated, ready, replicas),
			Kind:          kind,
			Name:          name,
			Replicas:      replicas,
			Updated:       updated,
			ReadyReplicas: ready,
		}, nil
	case "daemonset":
		desired, _, _ := unstructured.NestedInt64(obj.Object, "status", "desiredNumberScheduled")
		updated, _, _ := unstructured.NestedInt64(obj.Object, "status", "updatedNumberScheduled")
		ready, _, _ := unstructured.NestedInt64(obj.Object, "status", "numberAvailable")
		observedGeneration, _, _ := unstructured.NestedInt64(obj.Object, "status", "observedGeneration")
		generation := obj.GetGeneration()
		isReady := observedGeneration >= generation && updated >= desired && ready >= desired

		return &RolloutStatusResult{
			Ready:         isReady,
			Message:       fmt.Sprintf("daemonset/%s updated=%d ready=%d desired=%d", name, updated, ready, desired),
			Kind:          kind,
			Name:          name,
			Replicas:      desired,
			Updated:       updated,
			ReadyReplicas: ready,
		}, nil
	default:
		return nil, fmt.Errorf("rollout status is supported for Deployment, StatefulSet, and DaemonSet, got %s", kind)
	}
}
