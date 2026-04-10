package query

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/crypto"
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

func (s *AppConfigStore) EnsureEncryptionKey(ctx context.Context, seed string) (string, error) {
	existing, err := s.Get(ctx, AppConfigKeyEncryptionKey)
	if err != nil {
		return "", err
	}
	if existing != nil && strings.TrimSpace(existing.Value) != "" {
		return existing.Value, nil
	}

	key := strings.TrimSpace(seed)
	if key != "" {
		if _, err := crypto.NewEncryptor(key); err != nil {
			return "", fmt.Errorf("validate encryption key seed: %w", err)
		}
	} else {
		key, err = generateEncryptionKey()
		if err != nil {
			return "", err
		}
	}

	if err := s.Set(ctx, AppConfigKeyEncryptionKey, key); err != nil {
		return "", fmt.Errorf("store encryption key: %w", err)
	}

	return key, nil
}

func generateEncryptionKey() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate encryption key: %w", err)
	}

	return hex.EncodeToString(buf), nil
}
