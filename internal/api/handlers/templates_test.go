package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/db"
	"github.com/FlameInTheDark/emerald/internal/db/query"
	"github.com/FlameInTheDark/emerald/internal/pipelineops"
	"github.com/FlameInTheDark/emerald/internal/templateops"
)

func TestTemplateHandlerCreateCloneExportAndCreatePipeline(t *testing.T) {
	t.Parallel()

	templateHandler, pipelineHandler := newTemplateHandlerTestDeps(t)
	app := fiber.New()
	app.Post("/templates", templateHandler.Create)
	app.Get("/templates/:id", templateHandler.Get)
	app.Delete("/templates/:id", templateHandler.Delete)
	app.Post("/templates/:id/clone", templateHandler.Clone)
	app.Get("/templates/:id/export", templateHandler.Export)
	app.Post("/templates/:id/pipelines", templateHandler.CreatePipeline)
	app.Get("/pipelines/:id/export", pipelineHandler.Export)

	createBody := bytes.NewBufferString(`{
		"name":"Starter",
		"description":"Base workflow",
		"definition":{
			"nodes":[{"id":"trigger-1","data":{"type":"trigger:manual"}}],
			"edges":[],
			"viewport":{"x":10,"y":20,"zoom":1.2}
		}
	}`)
	createReq := httptest.NewRequest(http.MethodPost, "/templates", createBody)
	createReq.Header.Set("Content-Type", "application/json")
	createRes, err := app.Test(createReq)
	if err != nil {
		t.Fatalf("create app.Test: %v", err)
	}
	if createRes.StatusCode != fiber.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRes.StatusCode, fiber.StatusCreated)
	}

	var created templateops.TemplateDetail
	if err := json.NewDecoder(createRes.Body).Decode(&created); err != nil {
		t.Fatalf("decode created template: %v", err)
	}

	exportReq := httptest.NewRequest(http.MethodGet, "/templates/"+created.ID+"/export", nil)
	exportRes, err := app.Test(exportReq)
	if err != nil {
		t.Fatalf("export app.Test: %v", err)
	}
	if exportRes.StatusCode != fiber.StatusOK {
		t.Fatalf("export status = %d, want %d", exportRes.StatusCode, fiber.StatusOK)
	}

	var exported templateops.TemplateDocument
	if err := json.NewDecoder(exportRes.Body).Decode(&exported); err != nil {
		t.Fatalf("decode exported template: %v", err)
	}
	if exported.Kind != templateops.KindTemplate {
		t.Fatalf("export kind = %q, want %q", exported.Kind, templateops.KindTemplate)
	}

	cloneReq := httptest.NewRequest(http.MethodPost, "/templates/"+created.ID+"/clone", nil)
	cloneRes, err := app.Test(cloneReq)
	if err != nil {
		t.Fatalf("clone app.Test: %v", err)
	}
	if cloneRes.StatusCode != fiber.StatusCreated {
		t.Fatalf("clone status = %d, want %d", cloneRes.StatusCode, fiber.StatusCreated)
	}

	var cloned templateops.TemplateDetail
	if err := json.NewDecoder(cloneRes.Body).Decode(&cloned); err != nil {
		t.Fatalf("decode cloned template: %v", err)
	}
	if cloned.Name != "Starter (Copy)" {
		t.Fatalf("clone name = %q, want %q", cloned.Name, "Starter (Copy)")
	}

	createPipelineReq := httptest.NewRequest(http.MethodPost, "/templates/"+created.ID+"/pipelines", nil)
	createPipelineRes, err := app.Test(createPipelineReq)
	if err != nil {
		t.Fatalf("create pipeline app.Test: %v", err)
	}
	if createPipelineRes.StatusCode != fiber.StatusCreated {
		t.Fatalf("create pipeline status = %d, want %d", createPipelineRes.StatusCode, fiber.StatusCreated)
	}

	var createdPipeline struct {
		ID       string  `json:"id"`
		Status   string  `json:"status"`
		Viewport *string `json:"viewport"`
	}
	if err := json.NewDecoder(createPipelineRes.Body).Decode(&createdPipeline); err != nil {
		t.Fatalf("decode created pipeline: %v", err)
	}
	if createdPipeline.Status != pipelineops.StatusDraft {
		t.Fatalf("pipeline status = %q, want %q", createdPipeline.Status, pipelineops.StatusDraft)
	}
	if createdPipeline.Viewport == nil {
		t.Fatal("expected created pipeline viewport to be persisted")
	}

	pipelineExportReq := httptest.NewRequest(http.MethodGet, "/pipelines/"+createdPipeline.ID+"/export", nil)
	pipelineExportRes, err := app.Test(pipelineExportReq)
	if err != nil {
		t.Fatalf("pipeline export app.Test: %v", err)
	}
	if pipelineExportRes.StatusCode != fiber.StatusOK {
		t.Fatalf("pipeline export status = %d, want %d", pipelineExportRes.StatusCode, fiber.StatusOK)
	}

	var pipelineDocument templateops.PipelineDocument
	if err := json.NewDecoder(pipelineExportRes.Body).Decode(&pipelineDocument); err != nil {
		t.Fatalf("decode exported pipeline: %v", err)
	}
	if pipelineDocument.Kind != templateops.KindPipeline {
		t.Fatalf("pipeline export kind = %q, want %q", pipelineDocument.Kind, templateops.KindPipeline)
	}
	if pipelineDocument.Status != pipelineops.StatusDraft {
		t.Fatalf("pipeline export status = %q, want %q", pipelineDocument.Status, pipelineops.StatusDraft)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/templates/"+created.ID, nil)
	deleteRes, err := app.Test(deleteReq)
	if err != nil {
		t.Fatalf("delete app.Test: %v", err)
	}
	if deleteRes.StatusCode != fiber.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", deleteRes.StatusCode, fiber.StatusNoContent)
	}

	getDeletedReq := httptest.NewRequest(http.MethodGet, "/templates/"+created.ID, nil)
	getDeletedRes, err := app.Test(getDeletedReq)
	if err != nil {
		t.Fatalf("get deleted app.Test: %v", err)
	}
	if getDeletedRes.StatusCode != fiber.StatusNotFound {
		t.Fatalf("get deleted status = %d, want %d", getDeletedRes.StatusCode, fiber.StatusNotFound)
	}
}

func TestTemplateHandlerImportBundleReturnsPartialSuccess(t *testing.T) {
	t.Parallel()

	templateHandler, _ := newTemplateHandlerTestDeps(t)
	app := fiber.New()
	app.Post("/templates/import", templateHandler.Import)

	body := bytes.NewBufferString(`{
		"version":"v1",
		"kind":"emerald-template-bundle",
		"templates":[
			{
				"version":"v1",
				"kind":"emerald-template",
				"name":"Healthy",
				"definition":{"nodes":[{"id":"trigger-1","data":{"type":"trigger:manual"}}],"edges":[]}
			},
			{
				"version":"v1",
				"kind":"emerald-template",
				"name":"Invalid",
				"definition":{"nodes":[{"id":"return-1","data":{"type":"logic:return"}},{"id":"return-2","data":{"type":"logic:return"}}],"edges":[]}
			}
		]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/templates/import", body)
	req.Header.Set("Content-Type", "application/json")

	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("import app.Test: %v", err)
	}
	if res.StatusCode != fiber.StatusOK {
		t.Fatalf("import status = %d, want %d", res.StatusCode, fiber.StatusOK)
	}

	var result templateops.ImportResult
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatalf("decode import result: %v", err)
	}
	if result.CreatedCount != 1 || result.FailedCount != 1 {
		t.Fatalf("unexpected import result: %+v", result)
	}
}

func newTemplateHandlerTestDeps(t *testing.T) (*TemplateHandler, *PipelineHandler) {
	t.Helper()

	database, err := db.New(filepath.Join(t.TempDir(), "emerald.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})

	if err := db.Migrate(database); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	templateStore := query.NewTemplateStore(database.DB)
	pipelineStore := query.NewPipelineStore(database.DB)
	pipelineService := pipelineops.NewService(pipelineStore, nil)

	return NewTemplateHandler(templateops.NewService(templateStore, pipelineService)), NewPipelineHandler(pipelineStore, nil)
}
