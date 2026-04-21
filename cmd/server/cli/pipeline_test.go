package cli

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlameInTheDark/emerald/internal/db"
	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
)

func TestRunPipelineCommandUsesExecutionRunnerExecutionID(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "emerald.db")

	t.Setenv("EMERALD_DB_PATH", dbPath)
	t.Setenv("EMERALD_SKILLS_DIR", filepath.Join(tempDir, "skills"))
	t.Setenv("EMERALD_PLUGINS_DIR", filepath.Join(tempDir, "plugins"))
	t.Setenv("EMERALD_ENCRYPTION_KEY", strings.Repeat("k", 32))

	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("initialize test database: %v", err)
	}
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	pipelineStore := query.NewPipelineStore(database.DB)
	pipelineModel := &models.Pipeline{
		Name:   "CLI Run Test",
		Status: "draft",
		Nodes: `[
			{"id":"trigger-1","type":"trigger:manual","data":{"label":"Manual Trigger","type":"trigger:manual","config":{},"enabled":true}},
			{"id":"return-1","type":"logic:return","data":{"label":"Return","type":"logic:return","config":{}}}
		]`,
		Edges: `[{"id":"edge-1","source":"trigger-1","target":"return-1"}]`,
	}
	if err := pipelineStore.Create(ctx, pipelineModel); err != nil {
		t.Fatalf("create test pipeline: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close setup database: %v", err)
	}

	output := &bytes.Buffer{}
	progressWriter := &fakeProgressWriter{}
	err = runPipelineCommand(
		ctx,
		pipelineModel.ID,
		`{"hello":"world"}`,
		output,
		newCLIRuntime,
		func(io.Writer) cliProgressWriter { return progressWriter },
	)
	if err != nil {
		t.Fatalf("run pipeline command: %v", err)
	}

	verifyDB, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("reopen test database: %v", err)
	}
	defer func() {
		_ = verifyDB.Close()
	}()

	executionStore := query.NewExecutionStore(verifyDB.DB)
	executions, err := executionStore.ListByPipeline(ctx, pipelineModel.ID)
	if err != nil {
		t.Fatalf("list executions: %v", err)
	}
	if len(executions) != 1 {
		t.Fatalf("expected exactly one execution row, got %d", len(executions))
	}
	if !strings.Contains(output.String(), executions[0].ID) {
		t.Fatalf("expected command output to contain execution id %s, got %q", executions[0].ID, output.String())
	}
	if !progressWriter.rendered || !progressWriter.stopped {
		t.Fatalf("expected progress writer to render and stop")
	}
}
