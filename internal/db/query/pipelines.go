package query

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/FlameInTheDark/automator/internal/db/models"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
)

type PipelineStore struct {
	db *sql.DB
}

func NewPipelineStore(db *sql.DB) *PipelineStore {
	return &PipelineStore{db: db}
}

func (s *PipelineStore) Create(ctx context.Context, p *models.Pipeline) error {
	p.ID = uuid.New().String()

	query, args, err := psql.Insert("pipelines").
		Columns("id", "name", "description", "nodes", "edges", "viewport", "status").
		Values(p.ID, p.Name, p.Description, p.Nodes, p.Edges, p.Viewport, p.Status).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *PipelineStore) GetByID(ctx context.Context, id string) (*models.Pipeline, error) {
	query, args, err := psql.Select("id", "name", "description", "nodes", "edges", "viewport", "status", "created_at", "updated_at").
		From("pipelines").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	var p models.Pipeline
	err = s.db.QueryRowContext(ctx, query, args...).Scan(
		&p.ID, &p.Name, &p.Description, &p.Nodes, &p.Edges, &p.Viewport, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("query pipeline: %w", err)
	}

	return &p, nil
}

func (s *PipelineStore) List(ctx context.Context) ([]models.Pipeline, error) {
	query, args, err := psql.Select("id", "name", "description", "nodes", "edges", "status", "created_at", "updated_at").
		From("pipelines").
		OrderBy("created_at DESC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query pipelines: %w", err)
	}
	defer rows.Close()

	var pipelines []models.Pipeline
	for rows.Next() {
		var p models.Pipeline
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Nodes, &p.Edges, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan pipeline: %w", err)
		}
		pipelines = append(pipelines, p)
	}

	return pipelines, rows.Err()
}

func (s *PipelineStore) ListActive(ctx context.Context) ([]models.Pipeline, error) {
	query, args, err := psql.Select("id", "name", "description", "nodes", "edges", "viewport", "status", "created_at", "updated_at").
		From("pipelines").
		Where(sq.Eq{"status": "active"}).
		OrderBy("created_at DESC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query active pipelines: %w", err)
	}
	defer rows.Close()

	var pipelines []models.Pipeline
	for rows.Next() {
		var p models.Pipeline
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Nodes, &p.Edges, &p.Viewport, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan active pipeline: %w", err)
		}
		pipelines = append(pipelines, p)
	}

	return pipelines, rows.Err()
}

func (s *PipelineStore) Count(ctx context.Context) (int, error) {
	query, args, err := psql.Select("COUNT(*)").From("pipelines").ToSql()
	if err != nil {
		return 0, fmt.Errorf("build query: %w", err)
	}

	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count pipelines: %w", err)
	}

	return count, nil
}

func (s *PipelineStore) CountByStatus(ctx context.Context, status string) (int, error) {
	query, args, err := psql.Select("COUNT(*)").From("pipelines").Where(sq.Eq{"status": status}).ToSql()
	if err != nil {
		return 0, fmt.Errorf("build query: %w", err)
	}

	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count pipelines by status: %w", err)
	}

	return count, nil
}

func (s *PipelineStore) Update(ctx context.Context, p *models.Pipeline) error {
	query, args, err := psql.Update("pipelines").
		Set("name", p.Name).
		Set("description", p.Description).
		Set("nodes", p.Nodes).
		Set("edges", p.Edges).
		Set("viewport", p.Viewport).
		Set("status", p.Status).
		Set("updated_at", sq.Expr("CURRENT_TIMESTAMP")).
		Where(sq.Eq{"id": p.ID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *PipelineStore) Delete(ctx context.Context, id string) error {
	query, args, err := psql.Delete("pipelines").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}
