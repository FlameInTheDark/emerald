package templateops

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/FlameInTheDark/emerald/internal/db/models"
)

type stubTemplateStore struct {
	templates []models.Template
}

func (s *stubTemplateStore) List(context.Context) ([]models.Template, error) {
	result := make([]models.Template, len(s.templates))
	copy(result, s.templates)
	return result, nil
}

func (s *stubTemplateStore) GetByID(_ context.Context, id string) (*models.Template, error) {
	for _, template := range s.templates {
		if template.ID == id {
			copyTemplate := template
			return &copyTemplate, nil
		}
	}

	return nil, nil
}

func (s *stubTemplateStore) Create(_ context.Context, template *models.Template) error {
	if template.ID == "" {
		template.ID = "template-created"
	}
	template.CreatedAt = time.Unix(int64(len(s.templates)+1), 0)
	s.safelyAppend(*template)
	return nil
}

func (s *stubTemplateStore) Delete(_ context.Context, id string) error {
	filtered := s.templates[:0]
	for _, template := range s.templates {
		if template.ID != id {
			filtered = append(filtered, template)
		}
	}
	s.templates = filtered
	return nil
}

func (s *stubTemplateStore) safelyAppend(template models.Template) {
	s.templates = append(s.templates, template)
}

type stubPipelineCreator struct {
	created []*models.Pipeline
}

func (s *stubPipelineCreator) Create(_ context.Context, pipeline *models.Pipeline) error {
	pipeline.ID = "pipeline-created"
	s.created = append(s.created, pipeline)
	return nil
}

func TestServiceCloneIncrementsCopyName(t *testing.T) {
	t.Parallel()

	store := &stubTemplateStore{
		templates: []models.Template{
			{ID: "template-1", Name: "Deploy App", Category: DefaultCategory, PipelineData: `{"nodes":[],"edges":[]}`},
			{ID: "template-2", Name: "Deploy App (Copy)", Category: DefaultCategory, PipelineData: `{"nodes":[],"edges":[]}`},
			{ID: "template-3", Name: "Deploy App (Copy 2)", Category: DefaultCategory, PipelineData: `{"nodes":[],"edges":[]}`},
		},
	}

	service := NewService(store, nil)
	clone, err := service.Clone(context.Background(), "template-1")
	if err != nil {
		t.Fatalf("Clone returned error: %v", err)
	}

	if clone.Name != "Deploy App (Copy 3)" {
		t.Fatalf("clone name = %q, want %q", clone.Name, "Deploy App (Copy 3)")
	}
}

func TestServiceCreatePipelineForcesDraft(t *testing.T) {
	t.Parallel()

	viewport := `{"x":12,"y":24,"zoom":1.25}`
	store := &stubTemplateStore{
		templates: []models.Template{
			{
				ID:           "template-1",
				Name:         "Incident Workflow",
				Category:     DefaultCategory,
				PipelineData: `{"nodes":[{"id":"trigger-1","data":{"type":"trigger:manual"}}],"edges":[],"viewport":` + viewport + `}`,
			},
		},
	}
	pipelines := &stubPipelineCreator{}

	service := NewService(store, pipelines)
	pipelineModel, err := service.CreatePipeline(context.Background(), "template-1")
	if err != nil {
		t.Fatalf("CreatePipeline returned error: %v", err)
	}

	if pipelineModel.Status != "draft" {
		t.Fatalf("status = %q, want draft", pipelineModel.Status)
	}
	if pipelineModel.Viewport == nil || *pipelineModel.Viewport != viewport {
		t.Fatalf("viewport = %#v, want %s", pipelineModel.Viewport, viewport)
	}
	if len(pipelines.created) != 1 {
		t.Fatalf("expected 1 created pipeline, got %d", len(pipelines.created))
	}
}

func TestServiceDeleteRemovesTemplate(t *testing.T) {
	t.Parallel()

	store := &stubTemplateStore{
		templates: []models.Template{
			{ID: "template-1", Name: "Starter", Category: DefaultCategory, PipelineData: `{"nodes":[],"edges":[]}`},
			{ID: "template-2", Name: "Keep", Category: DefaultCategory, PipelineData: `{"nodes":[],"edges":[]}`},
		},
	}

	service := NewService(store, nil)
	if err := service.Delete(context.Background(), "template-1"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	if len(store.templates) != 1 {
		t.Fatalf("template count = %d, want 1", len(store.templates))
	}
	if store.templates[0].ID != "template-2" {
		t.Fatalf("remaining template = %q, want %q", store.templates[0].ID, "template-2")
	}
}

func TestServiceImportBundleReturnsPartialSuccess(t *testing.T) {
	t.Parallel()

	store := &stubTemplateStore{}
	service := NewService(store, nil)

	bundle := TemplateBundle{
		Version: DocumentVersion,
		Kind:    KindTemplateBundle,
		Templates: []TemplateDocument{
			{
				Version: DocumentVersion,
				Kind:    KindTemplate,
				Name:    "Healthy",
				Definition: Definition{
					Nodes: json.RawMessage(`[{"id":"trigger-1","data":{"type":"trigger:manual"}}]`),
					Edges: json.RawMessage(`[]`),
				},
			},
			{
				Version: DocumentVersion,
				Kind:    KindTemplate,
				Name:    "Invalid",
				Definition: Definition{
					Nodes: json.RawMessage(`[
						{"id":"return-1","data":{"type":"logic:return"}},
						{"id":"return-2","data":{"type":"logic:return"}}
					]`),
					Edges: json.RawMessage(`[]`),
				},
			},
		},
	}

	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	result, err := service.Import(context.Background(), raw)
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}

	if result.CreatedCount != 1 || len(result.Created) != 1 {
		t.Fatalf("expected 1 created template, got %+v", result)
	}
	if result.FailedCount != 1 || len(result.Errors) != 1 {
		t.Fatalf("expected 1 failed template, got %+v", result)
	}
	if result.Errors[0].Name != "Invalid" {
		t.Fatalf("unexpected failed template name: %+v", result.Errors[0])
	}
}

func TestBuildPipelineDocumentIncludesDefinition(t *testing.T) {
	t.Parallel()

	viewport := `{"x":4,"y":8,"zoom":1.1}`
	document, err := BuildPipelineDocument(models.Pipeline{
		ID:          "pipeline-1",
		Name:        "Support",
		Description: nil,
		Nodes:       `[{"id":"trigger-1","data":{"type":"trigger:manual"}}]`,
		Edges:       `[]`,
		Viewport:    &viewport,
		Status:      "active",
	})
	if err != nil {
		t.Fatalf("BuildPipelineDocument returned error: %v", err)
	}

	if document.Kind != KindPipeline {
		t.Fatalf("kind = %q, want %q", document.Kind, KindPipeline)
	}
	if document.Status != "active" {
		t.Fatalf("status = %q, want active", document.Status)
	}
	if string(document.Definition.Viewport) != viewport {
		t.Fatalf("viewport = %s, want %s", string(document.Definition.Viewport), viewport)
	}
}
