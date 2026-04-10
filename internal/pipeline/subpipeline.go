package pipeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/FlameInTheDark/emerald/internal/db/models"
)

type PipelineCatalog interface {
	GetByID(ctx context.Context, id string) (*models.Pipeline, error)
	List(ctx context.Context) ([]models.Pipeline, error)
}

type callStackContextKey struct{}

type RunResult struct {
	ExecutionID  string `json:"execution_id"`
	PipelineID   string `json:"pipeline_id"`
	PipelineName string `json:"pipeline_name"`
	Status       string `json:"status"`
	NodesRun     int    `json:"nodes_run"`
	Returned     bool   `json:"returned"`
	ReturnValue  any    `json:"return_value,omitempty"`
}

type Invoker struct {
	db     *sql.DB
	store  PipelineCatalog
	engine *Engine
	runner *ExecutionRunner
}

func NewInvoker(db *sql.DB, store PipelineCatalog, engine *Engine, runner *ExecutionRunner) *Invoker {
	return &Invoker{
		db:     db,
		store:  store,
		engine: engine,
		runner: runner,
	}
}

func (i *Invoker) List(ctx context.Context) ([]models.Pipeline, error) {
	if i.store == nil {
		return nil, fmt.Errorf("pipeline store is not configured")
	}

	return i.store.List(ctx)
}

func (i *Invoker) Run(ctx context.Context, pipelineID string, input map[string]any) (*RunResult, error) {
	if i.db == nil {
		return nil, fmt.Errorf("database is not configured")
	}
	if i.store == nil {
		return nil, fmt.Errorf("pipeline store is not configured")
	}
	if i.engine == nil {
		return nil, fmt.Errorf("pipeline engine is not configured")
	}

	stack := PipelineCallStackFromContext(ctx)
	for _, activePipelineID := range stack {
		if activePipelineID == pipelineID {
			return nil, fmt.Errorf("pipeline recursion is not allowed for %s", pipelineID)
		}
	}

	pipelineModel, err := i.store.GetByID(ctx, pipelineID)
	if err != nil {
		return nil, fmt.Errorf("load pipeline %s: %w", pipelineID, err)
	}

	flowData, err := ParseFlowData(pipelineModel.Nodes, pipelineModel.Edges)
	if err != nil {
		return nil, fmt.Errorf("parse pipeline %s: %w", pipelineModel.Name, err)
	}
	if err := ValidateFlowData(*flowData); err != nil {
		return nil, fmt.Errorf("validate pipeline %s: %w", pipelineModel.Name, err)
	}

	ctx = WithPipelineCall(ctx, pipelineID)
	if i.runner != nil {
		result, err := i.runner.Run(ctx, pipelineID, *flowData, "manual", copyExecutionContext(input))
		if err != nil {
			return nil, err
		}
		if result.Status == "failed" && result.Error != nil {
			return nil, fmt.Errorf("execute pipeline %s: %w", pipelineModel.Name, result.Error)
		}
		if result.Status == "cancelled" {
			return nil, fmt.Errorf("execute pipeline %s: %s", pipelineModel.Name, result.ErrorMessage)
		}

		return &RunResult{
			ExecutionID:  result.ExecutionID,
			PipelineID:   pipelineModel.ID,
			PipelineName: pipelineModel.Name,
			Status:       result.Status,
			NodesRun:     result.NodesRun,
			Returned:     result.Returned,
			ReturnValue:  result.ReturnValue,
		}, nil
	}

	executionID := uuid.New().String()
	startedAt := time.Now()

	var contextJSON *string
	if len(input) > 0 {
		if payload, err := json.Marshal(input); err == nil {
			serialized := string(payload)
			contextJSON = &serialized
		}
	}

	if _, err := i.db.ExecContext(
		ctx,
		`INSERT INTO executions (id, pipeline_id, trigger_type, status, started_at, context) VALUES (?, ?, ?, ?, ?, ?)`,
		executionID,
		pipelineID,
		"manual",
		"running",
		startedAt,
		contextJSON,
	); err != nil {
		return nil, fmt.Errorf("create execution record: %w", err)
	}

	state, execErr := i.engine.ExecuteWithInput(ctx, *flowData, "manual", copyExecutionContext(input))
	completedAt := time.Now()

	status := "completed"
	var errorText *string
	if execErr != nil {
		status = "failed"
		message := execErr.Error()
		errorText = &message
	}

	if _, err := i.db.ExecContext(
		ctx,
		`UPDATE executions SET status = ?, completed_at = ?, error = ? WHERE id = ?`,
		status,
		completedAt,
		errorText,
		executionID,
	); err != nil {
		return nil, fmt.Errorf("update execution status: %w", err)
	}

	if state != nil {
		if err := insertNodeRuns(ctx, i.db, executionID, state.NodeRuns); err != nil {
			return nil, err
		}
	}

	if execErr != nil {
		return nil, fmt.Errorf("execute pipeline %s: %w", pipelineModel.Name, execErr)
	}

	result := &RunResult{
		ExecutionID:  executionID,
		PipelineID:   pipelineModel.ID,
		PipelineName: pipelineModel.Name,
		Status:       status,
	}
	if state != nil {
		result.NodesRun = len(state.NodeRuns)
		if state.Returned {
			result.Returned = true
			result.ReturnValue = state.ReturnValue
		}
	}

	return result, nil
}

func insertNodeRuns(ctx context.Context, db *sql.DB, executionID string, runs []NodeRun) error {
	for _, run := range runs {
		var inputStr *string
		if len(run.Input) > 0 {
			if data, err := json.Marshal(run.Input); err == nil {
				str := string(data)
				inputStr = &str
			}
		}

		var outputStr *string
		if len(run.Result.Output) > 0 {
			str := string(run.Result.Output)
			outputStr = &str
		}

		ne := &models.NodeExecution{
			ID:          uuid.New().String(),
			ExecutionID: executionID,
			NodeID:      run.NodeID,
			NodeType:    run.NodeType,
			Status:      "completed",
			Input:       inputStr,
			Output:      outputStr,
			StartedAt:   &run.StartedAt,
			CompletedAt: &run.CompletedAt,
		}
		if run.Result.Error != nil {
			ne.Status = "failed"
			errStr := run.Result.Error.Error()
			ne.Error = &errStr
		}

		if _, err := db.ExecContext(
			ctx,
			`INSERT INTO node_executions (id, execution_id, node_id, node_type, status, input, output, error, started_at, completed_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			ne.ID,
			ne.ExecutionID,
			ne.NodeID,
			ne.NodeType,
			ne.Status,
			ne.Input,
			ne.Output,
			ne.Error,
			ne.StartedAt,
			ne.CompletedAt,
		); err != nil {
			return fmt.Errorf("insert node execution %s: %w", ne.NodeID, err)
		}
	}

	return nil
}

func PipelineCallStackFromContext(ctx context.Context) []string {
	stack, _ := ctx.Value(callStackContextKey{}).([]string)
	return stack
}

func WithPipelineCall(ctx context.Context, pipelineID string) context.Context {
	stack := append([]string{}, PipelineCallStackFromContext(ctx)...)
	stack = append(stack, pipelineID)
	return context.WithValue(ctx, callStackContextKey{}, stack)
}
