package query

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/FlameInTheDark/emerald/internal/crypto"
	"github.com/FlameInTheDark/emerald/internal/db/models"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
)

type LLMProviderStore struct {
	db        *sql.DB
	encryptor *crypto.Encryptor
}

func NewLLMProviderStore(db *sql.DB, encryptor *crypto.Encryptor) *LLMProviderStore {
	return &LLMProviderStore{db: db, encryptor: encryptor}
}

func (s *LLMProviderStore) Create(ctx context.Context, p *models.LLMProvider) error {
	p.ID = uuid.New().String()

	encryptedKey := p.APIKey
	if s.encryptor != nil && encryptedKey != "" {
		var err error
		encryptedKey, err = s.encryptor.Encrypt(encryptedKey)
		if err != nil {
			return fmt.Errorf("encrypt api key: %w", err)
		}
	}

	query, args, err := psql.Insert("llm_providers").
		Columns("id", "name", "provider_type", "api_key", "base_url", "model", "config", "is_default").
		Values(p.ID, p.Name, p.ProviderType, encryptedKey, p.BaseURL, p.Model, p.Config, boolToInt(p.IsDefault)).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *LLMProviderStore) GetByID(ctx context.Context, id string) (*models.LLMProvider, error) {
	query, args, err := psql.Select("id", "name", "provider_type", "api_key", "(CASE WHEN api_key != '' THEN 1 ELSE 0 END) AS has_secret", "base_url", "model", "config", "is_default", "created_at", "updated_at").
		From("llm_providers").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	var p models.LLMProvider
	err = s.db.QueryRowContext(ctx, query, args...).Scan(
		&p.ID, &p.Name, &p.ProviderType, &p.APIKey, &p.HasSecret, &p.BaseURL, &p.Model, &p.Config, &p.IsDefault, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("query llm provider: %w", err)
	}

	if s.encryptor != nil && p.APIKey != "" {
		p.APIKey, err = s.encryptor.DecryptCompat(p.APIKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt api key: %w", err)
		}
	}

	return &p, nil
}

func (s *LLMProviderStore) List(ctx context.Context) ([]models.LLMProvider, error) {
	query, args, err := psql.Select("id", "name", "provider_type", "(CASE WHEN api_key != '' THEN 1 ELSE 0 END) AS has_secret", "base_url", "model", "is_default", "created_at", "updated_at").
		From("llm_providers").
		OrderBy("created_at DESC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query llm providers: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var providers []models.LLMProvider
	for rows.Next() {
		var p models.LLMProvider
		if err := rows.Scan(&p.ID, &p.Name, &p.ProviderType, &p.HasSecret, &p.BaseURL, &p.Model, &p.IsDefault, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan llm provider: %w", err)
		}
		providers = append(providers, p)
	}

	return providers, rows.Err()
}

func (s *LLMProviderStore) Update(ctx context.Context, p *models.LLMProvider) error {
	encryptedKey := p.APIKey
	if s.encryptor != nil && encryptedKey != "" {
		var err error
		encryptedKey, err = s.encryptor.Encrypt(encryptedKey)
		if err != nil {
			return fmt.Errorf("encrypt api key: %w", err)
		}
	}

	query, args, err := psql.Update("llm_providers").
		Set("name", p.Name).
		Set("provider_type", p.ProviderType).
		Set("api_key", encryptedKey).
		Set("base_url", p.BaseURL).
		Set("model", p.Model).
		Set("config", p.Config).
		Set("is_default", boolToInt(p.IsDefault)).
		Set("updated_at", "CURRENT_TIMESTAMP").
		Where(sq.Eq{"id": p.ID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *LLMProviderStore) Delete(ctx context.Context, id string) error {
	query, args, err := psql.Delete("llm_providers").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *LLMProviderStore) GetDefault(ctx context.Context) (*models.LLMProvider, error) {
	query, args, err := psql.Select("id", "name", "provider_type", "api_key", "base_url", "model", "config", "is_default", "created_at", "updated_at").
		From("llm_providers").
		Where(sq.Eq{"is_default": 1}).
		Limit(1).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	var p models.LLMProvider
	err = s.db.QueryRowContext(ctx, query, args...).Scan(
		&p.ID, &p.Name, &p.ProviderType, &p.APIKey, &p.BaseURL, &p.Model, &p.Config, &p.IsDefault, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("query default llm provider: %w", err)
	}

	if s.encryptor != nil && p.APIKey != "" {
		p.APIKey, err = s.encryptor.DecryptCompat(p.APIKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt api key: %w", err)
		}
	}

	return &p, nil
}
