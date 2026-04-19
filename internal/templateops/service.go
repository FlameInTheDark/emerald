package templateops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/pipelineops"
)

const (
	DocumentVersion    = "v1"
	KindPipeline       = "emerald-pipeline"
	KindTemplate       = "emerald-template"
	KindTemplateBundle = "emerald-template-bundle"
	DefaultCategory    = "custom"
)

type Definition struct {
	Nodes    json.RawMessage `json:"nodes"`
	Edges    json.RawMessage `json:"edges"`
	Viewport json.RawMessage `json:"viewport,omitempty"`
}

type PipelineDocument struct {
	Version     string     `json:"version"`
	Kind        string     `json:"kind"`
	Name        string     `json:"name"`
	Description *string    `json:"description,omitempty"`
	Status      string     `json:"status,omitempty"`
	Definition  Definition `json:"definition"`
}

type TemplateDocument struct {
	Version     string     `json:"version"`
	Kind        string     `json:"kind"`
	Name        string     `json:"name"`
	Description *string    `json:"description,omitempty"`
	Definition  Definition `json:"definition"`
}

type TemplateBundle struct {
	Version   string             `json:"version"`
	Kind      string             `json:"kind"`
	Templates []TemplateDocument `json:"templates"`
}

type TemplateSummary struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	Category    string    `json:"category"`
	CreatedAt   time.Time `json:"created_at"`
}

type TemplateDetail struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description *string    `json:"description,omitempty"`
	Category    string     `json:"category"`
	CreatedAt   time.Time  `json:"created_at"`
	Definition  Definition `json:"definition"`
}

type ImportFailure struct {
	Index int    `json:"index"`
	Name  string `json:"name,omitempty"`
	Error string `json:"error"`
}

type ImportResult struct {
	Created      []TemplateSummary `json:"created"`
	Errors       []ImportFailure   `json:"errors"`
	CreatedCount int               `json:"created_count"`
	FailedCount  int               `json:"failed_count"`
}

type CreateTemplateInput struct {
	Name        string
	Description *string
	Category    string
	Definition  Definition
}

type TemplateStore interface {
	List(ctx context.Context) ([]models.Template, error)
	GetByID(ctx context.Context, id string) (*models.Template, error)
	Create(ctx context.Context, template *models.Template) error
	Delete(ctx context.Context, id string) error
}

type PipelineCreator interface {
	Create(ctx context.Context, pipeline *models.Pipeline) error
}

type DefinitionValidator interface {
	ValidateDefinition(ctx context.Context, nodesJSON string, edgesJSON string, allowUnavailablePlugins bool) error
}

type Service struct {
	templates TemplateStore
	pipelines PipelineCreator
	validator DefinitionValidator
}

type storedDefinition struct {
	Nodes    string
	Edges    string
	Viewport *string
}

func NewService(templates TemplateStore, pipelines PipelineCreator, validators ...DefinitionValidator) *Service {
	service := &Service{
		templates: templates,
		pipelines: pipelines,
	}
	if len(validators) > 0 {
		service.validator = validators[0]
	}
	return service
}

func (s *Service) List(ctx context.Context) ([]TemplateSummary, error) {
	if s.templates == nil {
		return nil, fmt.Errorf("template store is not configured")
	}

	templates, err := s.templates.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}

	result := make([]TemplateSummary, 0, len(templates))
	for _, template := range templates {
		result = append(result, buildTemplateSummary(template))
	}

	return result, nil
}

func (s *Service) Create(ctx context.Context, input CreateTemplateInput) (*TemplateDetail, error) {
	if s.templates == nil {
		return nil, fmt.Errorf("template store is not configured")
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	category := strings.TrimSpace(input.Category)
	if category == "" {
		category = DefaultCategory
	}

	stored, err := s.canonicalizeDefinition(ctx, input.Definition)
	if err != nil {
		return nil, err
	}

	template := &models.Template{
		Name:         name,
		Description:  normalizeOptionalString(input.Description),
		Category:     category,
		PipelineData: stored.mustMarshal(),
	}
	if err := s.templates.Create(ctx, template); err != nil {
		return nil, fmt.Errorf("create template: %w", err)
	}

	return s.Get(ctx, template.ID)
}

func (s *Service) Get(ctx context.Context, id string) (*TemplateDetail, error) {
	if s.templates == nil {
		return nil, fmt.Errorf("template store is not configured")
	}

	template, err := s.templates.GetByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, fmt.Errorf("load template %s: %w", id, err)
	}
	if template == nil {
		return nil, nil
	}

	return buildTemplateDetail(*template)
}

func (s *Service) Clone(ctx context.Context, id string) (*TemplateDetail, error) {
	original, err := s.getRequiredTemplate(ctx, id)
	if err != nil {
		return nil, err
	}

	existing, err := s.templates.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list templates for clone: %w", err)
	}

	cloneName := nextCloneName(original.Name, existing)
	clone := &models.Template{
		Name:         cloneName,
		Description:  original.Description,
		Category:     original.Category,
		PipelineData: original.PipelineData,
	}
	if err := s.templates.Create(ctx, clone); err != nil {
		return nil, fmt.Errorf("clone template %s: %w", id, err)
	}

	return s.Get(ctx, clone.ID)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	template, err := s.getRequiredTemplate(ctx, id)
	if err != nil {
		return err
	}

	if err := s.templates.Delete(ctx, template.ID); err != nil {
		return fmt.Errorf("delete template %s: %w", template.ID, err)
	}

	return nil
}

func (s *Service) CreatePipeline(ctx context.Context, id string) (*models.Pipeline, error) {
	if s.pipelines == nil {
		return nil, fmt.Errorf("pipeline service is not configured")
	}

	template, err := s.getRequiredTemplate(ctx, id)
	if err != nil {
		return nil, err
	}

	definition, err := parseStoredDefinition(template.PipelineData)
	if err != nil {
		return nil, fmt.Errorf("decode template %s definition: %w", template.ID, err)
	}

	pipelineModel := &models.Pipeline{
		Name:        strings.TrimSpace(template.Name),
		Description: template.Description,
		Nodes:       definition.Nodes,
		Edges:       definition.Edges,
		Viewport:    definition.Viewport,
		Status:      pipelineops.StatusDraft,
	}
	if err := s.pipelines.Create(ctx, pipelineModel); err != nil {
		return nil, fmt.Errorf("create pipeline from template %s: %w", template.ID, err)
	}

	return pipelineModel, nil
}

func (s *Service) ExportTemplate(ctx context.Context, id string) (*TemplateDocument, error) {
	template, err := s.getRequiredTemplate(ctx, id)
	if err != nil {
		return nil, err
	}

	document, err := BuildTemplateDocument(*template)
	if err != nil {
		return nil, err
	}

	return document, nil
}

func (s *Service) ExportBundle(ctx context.Context) (*TemplateBundle, error) {
	if s.templates == nil {
		return nil, fmt.Errorf("template store is not configured")
	}

	templates, err := s.templates.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}

	documents := make([]TemplateDocument, 0, len(templates))
	for _, template := range templates {
		document, err := BuildTemplateDocument(template)
		if err != nil {
			return nil, err
		}
		documents = append(documents, *document)
	}

	return &TemplateBundle{
		Version:   DocumentVersion,
		Kind:      KindTemplateBundle,
		Templates: documents,
	}, nil
}

func (s *Service) Import(ctx context.Context, raw []byte) (*ImportResult, error) {
	if s.templates == nil {
		return nil, fmt.Errorf("template store is not configured")
	}

	var envelope struct {
		Version string `json:"version"`
		Kind    string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("parse import document: %w", err)
	}

	switch envelope.Kind {
	case KindTemplate:
		document, err := parseTemplateDocument(raw)
		if err != nil {
			return nil, err
		}
		created, err := s.Create(ctx, CreateTemplateInput{
			Name:        document.Name,
			Description: document.Description,
			Category:    DefaultCategory,
			Definition:  document.Definition,
		})
		if err != nil {
			return nil, err
		}
		summary := TemplateSummary{
			ID:          created.ID,
			Name:        created.Name,
			Description: created.Description,
			Category:    created.Category,
			CreatedAt:   created.CreatedAt,
		}
		return &ImportResult{
			Created:      []TemplateSummary{summary},
			CreatedCount: 1,
			FailedCount:  0,
		}, nil
	case KindPipeline:
		document, err := parsePipelineDocument(raw)
		if err != nil {
			return nil, err
		}
		created, err := s.Create(ctx, CreateTemplateInput{
			Name:        document.Name,
			Description: document.Description,
			Category:    DefaultCategory,
			Definition:  document.Definition,
		})
		if err != nil {
			return nil, err
		}
		summary := TemplateSummary{
			ID:          created.ID,
			Name:        created.Name,
			Description: created.Description,
			Category:    created.Category,
			CreatedAt:   created.CreatedAt,
		}
		return &ImportResult{
			Created:      []TemplateSummary{summary},
			CreatedCount: 1,
			FailedCount:  0,
		}, nil
	case KindTemplateBundle:
		document, err := parseTemplateBundle(raw)
		if err != nil {
			return nil, err
		}

		result := &ImportResult{}
		for idx, item := range document.Templates {
			created, createErr := s.Create(ctx, CreateTemplateInput{
				Name:        item.Name,
				Description: item.Description,
				Category:    DefaultCategory,
				Definition:  item.Definition,
			})
			if createErr != nil {
				result.Errors = append(result.Errors, ImportFailure{
					Index: idx,
					Name:  item.Name,
					Error: createErr.Error(),
				})
				continue
			}
			result.Created = append(result.Created, TemplateSummary{
				ID:          created.ID,
				Name:        created.Name,
				Description: created.Description,
				Category:    created.Category,
				CreatedAt:   created.CreatedAt,
			})
		}

		result.CreatedCount = len(result.Created)
		result.FailedCount = len(result.Errors)
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported import kind %q", envelope.Kind)
	}
}

func BuildPipelineDocument(pipelineModel models.Pipeline) (*PipelineDocument, error) {
	definition := storedDefinition{
		Nodes:    pipelineModel.Nodes,
		Edges:    pipelineModel.Edges,
		Viewport: pipelineModel.Viewport,
	}
	if err := pipelineops.ValidateDefinition(definition.Nodes, definition.Edges); err != nil {
		return nil, err
	}

	return &PipelineDocument{
		Version:     DocumentVersion,
		Kind:        KindPipeline,
		Name:        pipelineModel.Name,
		Description: pipelineModel.Description,
		Status:      pipelineModel.Status,
		Definition:  definition.toDefinition(),
	}, nil
}

func BuildTemplateDocument(template models.Template) (*TemplateDocument, error) {
	definition, err := parseStoredDefinition(template.PipelineData)
	if err != nil {
		return nil, fmt.Errorf("decode template %s definition: %w", template.ID, err)
	}

	return &TemplateDocument{
		Version:     DocumentVersion,
		Kind:        KindTemplate,
		Name:        template.Name,
		Description: template.Description,
		Definition:  definition.toDefinition(),
	}, nil
}

func buildTemplateSummary(template models.Template) TemplateSummary {
	return TemplateSummary{
		ID:          template.ID,
		Name:        template.Name,
		Description: template.Description,
		Category:    template.Category,
		CreatedAt:   template.CreatedAt,
	}
}

func buildTemplateDetail(template models.Template) (*TemplateDetail, error) {
	definition, err := parseStoredDefinition(template.PipelineData)
	if err != nil {
		return nil, fmt.Errorf("decode template %s definition: %w", template.ID, err)
	}

	return &TemplateDetail{
		ID:          template.ID,
		Name:        template.Name,
		Description: template.Description,
		Category:    template.Category,
		CreatedAt:   template.CreatedAt,
		Definition:  definition.toDefinition(),
	}, nil
}

func (s *Service) getRequiredTemplate(ctx context.Context, id string) (*models.Template, error) {
	if s.templates == nil {
		return nil, fmt.Errorf("template store is not configured")
	}

	template, err := s.templates.GetByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, fmt.Errorf("load template %s: %w", id, err)
	}
	if template == nil {
		return nil, fmt.Errorf("template %s was not found", id)
	}

	return template, nil
}

func parseTemplateDocument(raw []byte) (*TemplateDocument, error) {
	var document TemplateDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("parse template document: %w", err)
	}
	if !matchesDocumentKind(document.Kind, KindTemplate) {
		return nil, fmt.Errorf("expected kind %q, got %q", KindTemplate, document.Kind)
	}
	if document.Version != DocumentVersion {
		return nil, fmt.Errorf("unsupported template document version %q", document.Version)
	}
	if strings.TrimSpace(document.Name) == "" {
		return nil, fmt.Errorf("template name is required")
	}
	if _, err := canonicalizeDefinition(document.Definition); err != nil {
		return nil, err
	}

	return &document, nil
}

func parsePipelineDocument(raw []byte) (*PipelineDocument, error) {
	var document PipelineDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("parse pipeline document: %w", err)
	}
	if !matchesDocumentKind(document.Kind, KindPipeline) {
		return nil, fmt.Errorf("expected kind %q, got %q", KindPipeline, document.Kind)
	}
	if document.Version != DocumentVersion {
		return nil, fmt.Errorf("unsupported pipeline document version %q", document.Version)
	}
	if strings.TrimSpace(document.Name) == "" {
		return nil, fmt.Errorf("pipeline name is required")
	}
	if _, err := canonicalizeDefinition(document.Definition); err != nil {
		return nil, err
	}

	return &document, nil
}

func parseTemplateBundle(raw []byte) (*TemplateBundle, error) {
	var bundle TemplateBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return nil, fmt.Errorf("parse template bundle: %w", err)
	}
	if !matchesDocumentKind(bundle.Kind, KindTemplateBundle) {
		return nil, fmt.Errorf("expected kind %q, got %q", KindTemplateBundle, bundle.Kind)
	}
	if bundle.Version != DocumentVersion {
		return nil, fmt.Errorf("unsupported template bundle version %q", bundle.Version)
	}

	for idx, item := range bundle.Templates {
		if strings.TrimSpace(item.Name) == "" {
			return nil, fmt.Errorf("template %d name is required", idx)
		}
	}

	return &bundle, nil
}

func matchesDocumentKind(actual string, expected string) bool {
	actual = strings.TrimSpace(actual)
	return actual == expected
}

func canonicalizeDefinition(input Definition) (storedDefinition, error) {
	nodesJSON, err := pipelineops.CanonicalizeJSON(input.Nodes, "[]", "definition.nodes")
	if err != nil {
		return storedDefinition{}, err
	}
	edgesJSON, err := pipelineops.CanonicalizeJSON(input.Edges, "[]", "definition.edges")
	if err != nil {
		return storedDefinition{}, err
	}
	viewportJSON, err := pipelineops.CanonicalizeJSONPointer(input.Viewport, "definition.viewport")
	if err != nil {
		return storedDefinition{}, err
	}
	if err := pipelineops.ValidateDefinition(nodesJSON, edgesJSON); err != nil {
		return storedDefinition{}, err
	}

	return storedDefinition{
		Nodes:    nodesJSON,
		Edges:    edgesJSON,
		Viewport: viewportJSON,
	}, nil
}

func parseStoredDefinition(raw string) (storedDefinition, error) {
	var payload struct {
		Nodes    json.RawMessage `json:"nodes"`
		Edges    json.RawMessage `json:"edges"`
		Viewport json.RawMessage `json:"viewport"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return storedDefinition{}, fmt.Errorf("parse stored definition: %w", err)
	}

	return canonicalizeDefinition(Definition{
		Nodes:    payload.Nodes,
		Edges:    payload.Edges,
		Viewport: payload.Viewport,
	})
}

func (s *Service) canonicalizeDefinition(ctx context.Context, input Definition) (storedDefinition, error) {
	stored, err := canonicalizeDefinition(input)
	if err != nil {
		return storedDefinition{}, err
	}
	if s.validator != nil {
		if err := s.validator.ValidateDefinition(ctx, stored.Nodes, stored.Edges, true); err != nil {
			return storedDefinition{}, err
		}
	}
	return stored, nil
}

func (d storedDefinition) toDefinition() Definition {
	definition := Definition{
		Nodes: json.RawMessage(d.Nodes),
		Edges: json.RawMessage(d.Edges),
	}
	if d.Viewport != nil && strings.TrimSpace(*d.Viewport) != "" {
		definition.Viewport = json.RawMessage(*d.Viewport)
	}

	return definition
}

func (d storedDefinition) mustMarshal() string {
	payload := struct {
		Nodes    json.RawMessage `json:"nodes"`
		Edges    json.RawMessage `json:"edges"`
		Viewport json.RawMessage `json:"viewport,omitempty"`
	}{
		Nodes: json.RawMessage(d.Nodes),
		Edges: json.RawMessage(d.Edges),
	}
	if d.Viewport != nil && strings.TrimSpace(*d.Viewport) != "" {
		payload.Viewport = json.RawMessage(*d.Viewport)
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("marshal stored definition: %v", err))
	}

	return string(encoded)
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func nextCloneName(baseName string, existing []models.Template) string {
	trimmedBase := strings.TrimSpace(baseName)
	if trimmedBase == "" {
		trimmedBase = "Template"
	}

	names := make(map[string]struct{}, len(existing))
	for _, template := range existing {
		names[strings.ToLower(strings.TrimSpace(template.Name))] = struct{}{}
	}

	candidate := fmt.Sprintf("%s (Copy)", trimmedBase)
	if _, exists := names[strings.ToLower(candidate)]; !exists {
		return candidate
	}

	for idx := 2; ; idx++ {
		candidate = fmt.Sprintf("%s (Copy %d)", trimmedBase, idx)
		if _, exists := names[strings.ToLower(candidate)]; !exists {
			return candidate
		}
	}
}
