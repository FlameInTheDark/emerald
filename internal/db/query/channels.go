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

type ChannelStore struct {
	db        *sql.DB
	encryptor *crypto.Encryptor
}

func NewChannelStore(db *sql.DB, encryptor *crypto.Encryptor) *ChannelStore {
	return &ChannelStore{db: db, encryptor: encryptor}
}

func (s *ChannelStore) Create(ctx context.Context, channel *models.Channel) error {
	channel.ID = uuid.New().String()

	encryptedConfig, err := s.encryptConfig(channel.Config)
	if err != nil {
		return err
	}

	query, args, err := psql.Insert("channels").
		Columns("id", "name", "type", "config", "welcome_message", "connect_url", "enabled", "state").
		Values(
			channel.ID,
			channel.Name,
			channel.Type,
			encryptedConfig,
			channel.WelcomeMessage,
			channel.ConnectURL,
			boolToInt(channel.Enabled),
			channel.State,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *ChannelStore) GetByID(ctx context.Context, id string) (*models.Channel, error) {
	query, args, err := psql.Select(
		"id", "name", "type", "config", "(CASE WHEN config IS NOT NULL AND config != '' THEN 1 ELSE 0 END) AS has_secret", "welcome_message", "connect_url", "enabled", "state", "created_at", "updated_at",
	).
		From("channels").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	var channel models.Channel
	err = s.db.QueryRowContext(ctx, query, args...).Scan(
		&channel.ID,
		&channel.Name,
		&channel.Type,
		&channel.Config,
		&channel.HasSecret,
		&channel.WelcomeMessage,
		&channel.ConnectURL,
		&channel.Enabled,
		&channel.State,
		&channel.CreatedAt,
		&channel.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("query channel: %w", err)
	}

	if err := s.decryptConfig(&channel); err != nil {
		return nil, err
	}

	return &channel, nil
}

func (s *ChannelStore) List(ctx context.Context) ([]models.Channel, error) {
	query, args, err := psql.Select(
		"id", "name", "type", "(CASE WHEN config IS NOT NULL AND config != '' THEN 1 ELSE 0 END) AS has_secret", "welcome_message", "connect_url", "enabled", "created_at", "updated_at",
	).
		From("channels").
		OrderBy("created_at DESC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query channels: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var channels []models.Channel
	for rows.Next() {
		var channel models.Channel
		if err := rows.Scan(
			&channel.ID,
			&channel.Name,
			&channel.Type,
			&channel.HasSecret,
			&channel.WelcomeMessage,
			&channel.ConnectURL,
			&channel.Enabled,
			&channel.CreatedAt,
			&channel.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan channel: %w", err)
		}
		channels = append(channels, channel)
	}

	return channels, rows.Err()
}

func (s *ChannelStore) ListEnabled(ctx context.Context) ([]models.Channel, error) {
	query, args, err := psql.Select(
		"id", "name", "type", "config", "(CASE WHEN config IS NOT NULL AND config != '' THEN 1 ELSE 0 END) AS has_secret", "welcome_message", "connect_url", "enabled", "state", "created_at", "updated_at",
	).
		From("channels").
		Where(sq.Eq{"enabled": 1}).
		OrderBy("created_at DESC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query enabled channels: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var channels []models.Channel
	for rows.Next() {
		var channel models.Channel
		if err := rows.Scan(
			&channel.ID,
			&channel.Name,
			&channel.Type,
			&channel.Config,
			&channel.HasSecret,
			&channel.WelcomeMessage,
			&channel.ConnectURL,
			&channel.Enabled,
			&channel.State,
			&channel.CreatedAt,
			&channel.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan enabled channel: %w", err)
		}
		if err := s.decryptConfig(&channel); err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}

	return channels, rows.Err()
}

func (s *ChannelStore) Update(ctx context.Context, channel *models.Channel) error {
	encryptedConfig, err := s.encryptConfig(channel.Config)
	if err != nil {
		return err
	}

	query, args, err := psql.Update("channels").
		Set("name", channel.Name).
		Set("type", channel.Type).
		Set("config", encryptedConfig).
		Set("welcome_message", channel.WelcomeMessage).
		Set("connect_url", channel.ConnectURL).
		Set("enabled", boolToInt(channel.Enabled)).
		Set("state", channel.State).
		Set("updated_at", "CURRENT_TIMESTAMP").
		Where(sq.Eq{"id": channel.ID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *ChannelStore) UpdateState(ctx context.Context, id string, state *string) error {
	query, args, err := psql.Update("channels").
		Set("state", state).
		Set("updated_at", "CURRENT_TIMESTAMP").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *ChannelStore) Delete(ctx context.Context, id string) error {
	query, args, err := psql.Delete("channels").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *ChannelStore) Count(ctx context.Context) (int, error) {
	query, args, err := psql.Select("COUNT(*)").From("channels").ToSql()
	if err != nil {
		return 0, fmt.Errorf("build query: %w", err)
	}

	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count channels: %w", err)
	}

	return count, nil
}

func (s *ChannelStore) encryptConfig(config *string) (*string, error) {
	if config == nil || *config == "" || s.encryptor == nil {
		return config, nil
	}

	encrypted, err := s.encryptor.Encrypt(*config)
	if err != nil {
		return nil, fmt.Errorf("encrypt channel config: %w", err)
	}

	return &encrypted, nil
}

func (s *ChannelStore) decryptConfig(channel *models.Channel) error {
	if channel.Config == nil || *channel.Config == "" || s.encryptor == nil {
		return nil
	}

	decrypted, err := s.encryptor.DecryptCompat(*channel.Config)
	if err != nil {
		return fmt.Errorf("decrypt channel config: %w", err)
	}
	channel.Config = &decrypted
	return nil
}
