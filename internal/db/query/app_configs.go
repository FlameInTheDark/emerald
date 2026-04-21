package query

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	sq "github.com/Masterminds/squirrel"
)

const AppConfigKeyEncryptionKey = "encryption_key"

type AppConfigStore struct {
	db *sql.DB
}

func NewAppConfigStore(db *sql.DB) *AppConfigStore {
	return &AppConfigStore{db: db}
}

func (s *AppConfigStore) Get(ctx context.Context, key string) (*models.AppConfig, error) {
	query, args, err := psql.Select("key", "value", "created_at", "updated_at").
		From("app_configs").
		Where(sq.Eq{"key": key}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	var config models.AppConfig
	err = s.db.QueryRowContext(ctx, query, args...).Scan(&config.Key, &config.Value, &config.CreatedAt, &config.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query app config: %w", err)
	}

	return &config, nil
}

func (s *AppConfigStore) Set(ctx context.Context, key string, value string) error {
	query, args, err := psql.Insert("app_configs").
		Columns("key", "value").
		Values(key, value).
		Suffix("ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP").
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *AppConfigStore) GetEncryptionKey(ctx context.Context) (string, bool, error) {
	existing, err := s.Get(ctx, AppConfigKeyEncryptionKey)
	if err != nil {
		return "", false, err
	}
	if existing == nil || strings.TrimSpace(existing.Value) == "" {
		return "", false, nil
	}
	return strings.TrimSpace(existing.Value), true, nil
}
