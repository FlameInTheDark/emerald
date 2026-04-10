package llm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/skills"
)

type stubClusterStore struct {
	clusters []models.Cluster
}

func (s stubClusterStore) List(ctx context.Context) ([]models.Cluster, error) {
	return s.clusters, nil
}

func (s stubClusterStore) GetByID(ctx context.Context, id string) (*models.Cluster, error) {
	for _, cluster := range s.clusters {
		if cluster.ID == id {
			copy := cluster
			return &copy, nil
		}
	}
	return nil, context.Canceled
}

type stubKubernetesClusterStore struct {
	clusters []models.KubernetesCluster
}

type stubSkillReader struct {
	skill skills.Skill
}

func (s stubKubernetesClusterStore) List(context.Context) ([]models.KubernetesCluster, error) {
	return s.clusters, nil
}

func (s stubKubernetesClusterStore) GetByID(_ context.Context, id string) (*models.KubernetesCluster, error) {
	for _, cluster := range s.clusters {
		if cluster.ID == id {
			copy := cluster
			return &copy, nil
		}
	}
	return nil, context.Canceled
}

func (s stubSkillReader) List() []skills.Summary {
	return []skills.Summary{{
		Name:        s.skill.Name,
		Description: s.skill.Description,
		Path:        s.skill.Path,
	}}
}

func (s stubSkillReader) SummaryText() string {
	return "- " + s.skill.Name + ": " + s.skill.Description
}

func (s stubSkillReader) GetByName(name string) (skills.Skill, bool) {
	if strings.EqualFold(strings.TrimSpace(name), s.skill.Name) {
		return s.skill, true
	}
	return skills.Skill{}, false
}

func TestToolRegistryResolveClusterUsesOnlyConfiguredCluster(t *testing.T) {
	t.Parallel()

	registry := NewToolRegistry(stubClusterStore{
		clusters: []models.Cluster{
			{ID: "cluster-1", Name: "Local", Host: "127.0.0.1", Port: 8006, APITokenID: "token", APITokenSecret: "secret"},
		},
	}, nil, nil, nil, "", nil, nil)

	cluster, err := registry.resolveCluster(context.Background(), proxmoxToolArgs{})
	if err != nil {
		t.Fatalf("resolveCluster returned error: %v", err)
	}

	if cluster.ID != "cluster-1" {
		t.Fatalf("cluster.ID = %q, want cluster-1", cluster.ID)
	}
}

func TestToolRegistryResolveClusterUsesDefaultClusterID(t *testing.T) {
	t.Parallel()

	registry := NewToolRegistry(stubClusterStore{
		clusters: []models.Cluster{
			{ID: "cluster-1", Name: "Local"},
			{ID: "cluster-2", Name: "Lab"},
		},
	}, nil, nil, nil, "cluster-2", nil, nil)

	cluster, err := registry.resolveCluster(context.Background(), proxmoxToolArgs{})
	if err != nil {
		t.Fatalf("resolveCluster returned error: %v", err)
	}

	if cluster.ID != "cluster-2" {
		t.Fatalf("cluster.ID = %q, want cluster-2", cluster.ID)
	}
}

func TestToolRegistryResolveClusterByName(t *testing.T) {
	t.Parallel()

	registry := NewToolRegistry(stubClusterStore{
		clusters: []models.Cluster{
			{ID: "cluster-1", Name: "Local"},
			{ID: "cluster-2", Name: "Lab"},
		},
	}, nil, nil, nil, "", nil, nil)

	cluster, err := registry.resolveCluster(context.Background(), proxmoxToolArgs{ClusterName: "lab"})
	if err != nil {
		t.Fatalf("resolveCluster returned error: %v", err)
	}

	if cluster.ID != "cluster-2" {
		t.Fatalf("cluster.ID = %q, want cluster-2", cluster.ID)
	}
}

func TestToolRegistryResolveClusterRequiresSelectionWhenMultipleExist(t *testing.T) {
	t.Parallel()

	registry := NewToolRegistry(stubClusterStore{
		clusters: []models.Cluster{
			{ID: "cluster-1", Name: "Local"},
			{ID: "cluster-2", Name: "Lab"},
		},
	}, nil, nil, nil, "", nil, nil)

	_, err := registry.resolveCluster(context.Background(), proxmoxToolArgs{})
	if err == nil {
		t.Fatal("expected resolveCluster to fail when multiple clusters exist")
	}

	if !strings.Contains(err.Error(), "multiple clusters are configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestToolRegistryResolveKubernetesClusterUsesDefaultClusterID(t *testing.T) {
	t.Parallel()

	registry := NewToolRegistryWithOptions(ToolRegistryOptions{
		KubernetesStore: stubKubernetesClusterStore{
			clusters: []models.KubernetesCluster{
				{ID: "k8s-1", Name: "Dev"},
				{ID: "k8s-2", Name: "Prod"},
			},
		},
		DefaultKubernetesClusterID: "k8s-2",
		EnableKubernetes:           true,
	})

	cluster, err := registry.resolveKubernetesCluster(context.Background(), "", "")
	if err != nil {
		t.Fatalf("resolveKubernetesCluster returned error: %v", err)
	}

	if cluster.ID != "k8s-2" {
		t.Fatalf("cluster.ID = %q, want k8s-2", cluster.ID)
	}
}

func TestToolRegistryResolveKubernetesClusterRequiresSelectionWhenMultipleExist(t *testing.T) {
	t.Parallel()

	registry := NewToolRegistryWithOptions(ToolRegistryOptions{
		KubernetesStore: stubKubernetesClusterStore{
			clusters: []models.KubernetesCluster{
				{ID: "k8s-1", Name: "Dev"},
				{ID: "k8s-2", Name: "Prod"},
			},
		},
		EnableKubernetes: true,
	})

	_, err := registry.resolveKubernetesCluster(context.Background(), "", "")
	if err == nil {
		t.Fatal("expected resolveKubernetesCluster to fail when multiple clusters exist")
	}

	if !strings.Contains(err.Error(), "multiple Kubernetes clusters are configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestToolRegistryRegistersSkillToolWhenSkillStoreIsConfigured(t *testing.T) {
	t.Parallel()

	registry := NewToolRegistryWithOptions(ToolRegistryOptions{
		SkillStore: stubSkillReader{
			skill: skills.Skill{
				Name:        "pipeline-builder",
				Description: "Build valid pipeline definitions.",
				Path:        "/workspace/.agents/skills/pipeline-builder/SKILL.md",
				Content:     "# Pipeline Builder",
			},
		},
	})

	toolNames := make([]string, 0, len(registry.GetAllTools()))
	for _, tool := range registry.GetAllTools() {
		toolNames = append(toolNames, tool.Function.Name)
	}

	if !containsString(toolNames, "get_skill") {
		t.Fatalf("expected get_skill tool to be registered, got %v", toolNames)
	}

	result, err := registry.Execute(context.Background(), "get_skill", json.RawMessage(`{"name":"pipeline-builder"}`))
	if err != nil {
		t.Fatalf("Execute get_skill returned error: %v", err)
	}

	payload, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("unexpected result type %T", result)
	}
	if payload["name"] != "pipeline-builder" {
		t.Fatalf("skill name = %v, want pipeline-builder", payload["name"])
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
