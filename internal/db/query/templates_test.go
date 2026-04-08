package query

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/FlameInTheDark/automator/internal/db"
	"github.com/FlameInTheDark/automator/internal/db/models"
)

func TestTemplateStoreCreateGetAndList(t *testing.T) {
	t.Parallel()

	database, err := db.New(filepath.Join(t.TempDir(), "automator.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	store := NewTemplateStore(database.DB)
	template := &models.Template{
		Name:         "Template A",
		Category:     "custom",
		PipelineData: `{"nodes":[],"edges":[]}`,
	}
	if err := store.Create(context.Background(), template); err != nil {
		t.Fatalf("Create: %v", err)
	}

	stored, err := store.GetByID(context.Background(), template.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored == nil || stored.Name != "Template A" {
		t.Fatalf("unexpected stored template: %+v", stored)
	}

	list, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list length = %d, want 1", len(list))
	}

	if err := store.Delete(context.Background(), template.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	deleted, err := store.GetByID(context.Background(), template.ID)
	if err != nil {
		t.Fatalf("GetByID after delete: %v", err)
	}
	if deleted != nil {
		t.Fatalf("expected template to be deleted, got %+v", deleted)
	}
}
