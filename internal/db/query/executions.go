package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
)

type ExecutionStore struct {
	db *sql.DB
}

func NewExecutionStore(db *sql.DB) *ExecutionStore {
	return &ExecutionStore{db: db}
}

func (s *ExecutionStore) Create(ctx context.Context, e *models.Execution) error {
	e.ID = uuid.New().String()
	e.StartedAt = time.Now()

	query, args, err := psql.Insert("executions").
		Columns("id", "pipeline_id", "trigger_type", "status", "started_at", "context").
		Values(e.ID, e.PipelineID, e.TriggerType, e.Status, e.StartedAt, e.Context).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *ExecutionStore) UpdateStatus(ctx context.Context, id, status string, completedAt *time.Time, errMsg *string) error {
	builder := psql.Update("executions").
		Set("status", status).
		Where(sq.Eq{"id": id})

	if completedAt != nil {
		builder = builder.Set("completed_at", *completedAt)
	}
	if errMsg != nil {
		builder = builder.Set("error", *errMsg)
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *ExecutionStore) GetByID(ctx context.Context, id string) (*models.Execution, error) {
	query, args, err := psql.Select("id", "pipeline_id", "trigger_type", "status", "started_at", "completed_at", "error", "context").
		From("executions").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	var e models.Execution
	err = s.db.QueryRowContext(ctx, query, args...).Scan(
		&e.ID, &e.PipelineID, &e.TriggerType, &e.Status, &e.StartedAt,
		&e.CompletedAt, &e.Error, &e.Context,
	)
	if err != nil {
		return nil, fmt.Errorf("query execution: %w", err)
	}

	return &e, nil
}

func (s *ExecutionStore) ListByPipeline(ctx context.Context, pipelineID string) ([]models.Execution, error) {
	query, args, err := psql.Select("id", "pipeline_id", "trigger_type", "status", "started_at", "completed_at", "error").
		From("executions").
		Where(sq.Eq{"pipeline_id": pipelineID}).
		OrderBy("started_at DESC").
		Limit(50).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query executions: %w", err)
	}
	defer rows.Close()

	var executions []models.Execution
	for rows.Next() {
		var e models.Execution
		if err := rows.Scan(&e.ID, &e.PipelineID, &e.TriggerType, &e.Status, &e.StartedAt, &e.CompletedAt, &e.Error); err != nil {
			return nil, fmt.Errorf("scan execution: %w", err)
		}
		executions = append(executions, e)
	}

	return executions, rows.Err()
}

func (s *ExecutionStore) CreateNodeExecution(ctx context.Context, ne *models.NodeExecution) error {
	ne.ID = uuid.New().String()

	query, args, err := psql.Insert("node_executions").
		Columns("id", "execution_id", "node_id", "node_type", "status", "input", "output", "error", "started_at", "completed_at").
		Values(ne.ID, ne.ExecutionID, ne.NodeID, ne.NodeType, ne.Status, ne.Input, ne.Output, ne.Error, ne.StartedAt, ne.CompletedAt).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *ExecutionStore) UpdateNodeExecution(ctx context.Context, id string, status string, output json.RawMessage, errMsg *string, completedAt *time.Time) error {
	builder := psql.Update("node_executions").
		Set("status", status).
		Where(sq.Eq{"id": id})

	if output != nil {
		builder = builder.Set("output", string(output))
	}
	if errMsg != nil {
		builder = builder.Set("error", *errMsg)
	}
	if completedAt != nil {
		builder = builder.Set("completed_at", *completedAt)
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *ExecutionStore) ListByExecution(ctx context.Context, executionID string) ([]models.NodeExecution, error) {
	query, args, err := psql.Select("id", "execution_id", "node_id", "node_type", "status", "input", "output", "error", "started_at", "completed_at").
		From("node_executions").
		Where(sq.Eq{"execution_id": executionID}).
		OrderBy("started_at ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query node executions: %w", err)
	}
	defer rows.Close()

	nodeExecutions := make([]models.NodeExecution, 0)
	for rows.Next() {
		var ne models.NodeExecution
		if err := rows.Scan(&ne.ID, &ne.ExecutionID, &ne.NodeID, &ne.NodeType, &ne.Status, &ne.Input, &ne.Output, &ne.Error, &ne.StartedAt, &ne.CompletedAt); err != nil {
			return nil, fmt.Errorf("scan node execution: %w", err)
		}
		nodeExecutions = append(nodeExecutions, ne)
	}

	return nodeExecutions, rows.Err()
}

func (s *ExecutionStore) CountSince(ctx context.Context, since time.Time) (int, error) {
	query, args, err := psql.Select("COUNT(*)").
		From("executions").
		Where(sq.GtOrEq{"started_at": since}).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("build query: %w", err)
	}

	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count executions since: %w", err)
	}

	return count, nil
}
