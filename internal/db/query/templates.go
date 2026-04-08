package query

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/FlameInTheDark/automator/internal/db/models"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
)

type TemplateStore struct {
	db *sql.DB
}

func NewTemplateStore(db *sql.DB) *TemplateStore {
	return &TemplateStore{db: db}
}

func (s *TemplateStore) Create(ctx context.Context, template *models.Template) error {
	template.ID = uuid.New().String()

	query, args, err := psql.Insert("templates").
		Columns("id", "name", "description", "category", "pipeline_data").
		Values(template.ID, template.Name, template.Description, template.Category, template.PipelineData).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *TemplateStore) GetByID(ctx context.Context, id string) (*models.Template, error) {
	query, args, err := psql.Select("id", "name", "description", "category", "pipeline_data", "created_at").
		From("templates").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	var template models.Template
	err = s.db.QueryRowContext(ctx, query, args...).Scan(
		&template.ID,
		&template.Name,
		&template.Description,
		&template.Category,
		&template.PipelineData,
		&template.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query template: %w", err)
	}

	return &template, nil
}

func (s *TemplateStore) List(ctx context.Context) ([]models.Template, error) {
	query, args, err := psql.Select("id", "name", "description", "category", "pipeline_data", "created_at").
		From("templates").
		OrderBy("created_at DESC", "name ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query templates: %w", err)
	}
	defer rows.Close()

	var templates []models.Template
	for rows.Next() {
		var template models.Template
		if err := rows.Scan(
			&template.ID,
			&template.Name,
			&template.Description,
			&template.Category,
			&template.PipelineData,
			&template.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan template: %w", err)
		}
		templates = append(templates, template)
	}

	return templates, rows.Err()
}

func (s *TemplateStore) Delete(ctx context.Context, id string) error {
	query, args, err := psql.Delete("templates").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}
