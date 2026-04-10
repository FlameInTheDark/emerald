package query

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
)

type ChannelContactStore struct {
	db *sql.DB
}

func NewChannelContactStore(db *sql.DB) *ChannelContactStore {
	return &ChannelContactStore{db: db}
}

func (s *ChannelContactStore) Create(ctx context.Context, contact *models.ChannelContact) error {
	contact.ID = uuid.New().String()

	query, args, err := psql.Insert("channel_contacts").
		Columns(
			"id",
			"channel_id",
			"external_user_id",
			"external_chat_id",
			"username",
			"display_name",
			"connection_code",
			"code_expires_at",
			"connected_at",
			"last_message_at",
		).
		Values(
			contact.ID,
			contact.ChannelID,
			contact.ExternalUserID,
			contact.ExternalChatID,
			contact.Username,
			contact.DisplayName,
			contact.ConnectionCode,
			contact.CodeExpiresAt,
			contact.ConnectedAt,
			contact.LastMessageAt,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *ChannelContactStore) Update(ctx context.Context, contact *models.ChannelContact) error {
	query, args, err := psql.Update("channel_contacts").
		Set("external_chat_id", contact.ExternalChatID).
		Set("username", contact.Username).
		Set("display_name", contact.DisplayName).
		Set("connection_code", contact.ConnectionCode).
		Set("code_expires_at", contact.CodeExpiresAt).
		Set("connected_at", contact.ConnectedAt).
		Set("last_message_at", contact.LastMessageAt).
		Set("updated_at", "CURRENT_TIMESTAMP").
		Where(sq.Eq{"id": contact.ID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *ChannelContactStore) GetByID(ctx context.Context, id string) (*models.ChannelContact, error) {
	query, args, err := psql.Select(
		"id",
		"channel_id",
		"external_user_id",
		"external_chat_id",
		"username",
		"display_name",
		"connection_code",
		"code_expires_at",
		"connected_at",
		"last_message_at",
		"created_at",
		"updated_at",
	).
		From("channel_contacts").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	var contact models.ChannelContact
	err = s.db.QueryRowContext(ctx, query, args...).Scan(
		&contact.ID,
		&contact.ChannelID,
		&contact.ExternalUserID,
		&contact.ExternalChatID,
		&contact.Username,
		&contact.DisplayName,
		&contact.ConnectionCode,
		&contact.CodeExpiresAt,
		&contact.ConnectedAt,
		&contact.LastMessageAt,
		&contact.CreatedAt,
		&contact.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("query channel contact: %w", err)
	}

	return &contact, nil
}

func (s *ChannelContactStore) GetByChannelAndExternalUser(ctx context.Context, channelID, externalUserID string) (*models.ChannelContact, error) {
	query, args, err := psql.Select(
		"id",
		"channel_id",
		"external_user_id",
		"external_chat_id",
		"username",
		"display_name",
		"connection_code",
		"code_expires_at",
		"connected_at",
		"last_message_at",
		"created_at",
		"updated_at",
	).
		From("channel_contacts").
		Where(sq.Eq{"channel_id": channelID, "external_user_id": externalUserID}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	var contact models.ChannelContact
	err = s.db.QueryRowContext(ctx, query, args...).Scan(
		&contact.ID,
		&contact.ChannelID,
		&contact.ExternalUserID,
		&contact.ExternalChatID,
		&contact.Username,
		&contact.DisplayName,
		&contact.ConnectionCode,
		&contact.CodeExpiresAt,
		&contact.ConnectedAt,
		&contact.LastMessageAt,
		&contact.CreatedAt,
		&contact.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query channel contact: %w", err)
	}

	return &contact, nil
}

func (s *ChannelContactStore) GetByConnectionCode(ctx context.Context, channelID, code string) (*models.ChannelContact, error) {
	builder := psql.Select(
		"id",
		"channel_id",
		"external_user_id",
		"external_chat_id",
		"username",
		"display_name",
		"connection_code",
		"code_expires_at",
		"connected_at",
		"last_message_at",
		"created_at",
		"updated_at",
	).
		From("channel_contacts").
		Where(sq.Eq{"connection_code": code})

	if channelID != "" {
		builder = builder.Where(sq.Eq{"channel_id": channelID})
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	var contact models.ChannelContact
	err = s.db.QueryRowContext(ctx, query, args...).Scan(
		&contact.ID,
		&contact.ChannelID,
		&contact.ExternalUserID,
		&contact.ExternalChatID,
		&contact.Username,
		&contact.DisplayName,
		&contact.ConnectionCode,
		&contact.CodeExpiresAt,
		&contact.ConnectedAt,
		&contact.LastMessageAt,
		&contact.CreatedAt,
		&contact.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query channel contact by code: %w", err)
	}

	return &contact, nil
}

func (s *ChannelContactStore) ListByChannel(ctx context.Context, channelID string) ([]models.ChannelContact, error) {
	query, args, err := psql.Select(
		"id",
		"channel_id",
		"external_user_id",
		"external_chat_id",
		"username",
		"display_name",
		"connection_code",
		"code_expires_at",
		"connected_at",
		"last_message_at",
		"created_at",
		"updated_at",
	).
		From("channel_contacts").
		Where(sq.Eq{"channel_id": channelID}).
		OrderBy("connected_at DESC", "created_at DESC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query channel contacts: %w", err)
	}
	defer rows.Close()

	var contacts []models.ChannelContact
	for rows.Next() {
		var contact models.ChannelContact
		if err := rows.Scan(
			&contact.ID,
			&contact.ChannelID,
			&contact.ExternalUserID,
			&contact.ExternalChatID,
			&contact.Username,
			&contact.DisplayName,
			&contact.ConnectionCode,
			&contact.CodeExpiresAt,
			&contact.ConnectedAt,
			&contact.LastMessageAt,
			&contact.CreatedAt,
			&contact.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan channel contact: %w", err)
		}
		contacts = append(contacts, contact)
	}

	return contacts, rows.Err()
}

func (s *ChannelContactStore) CountByChannel(ctx context.Context, channelID string) (int, error) {
	query, args, err := psql.Select("COUNT(*)").
		From("channel_contacts").
		Where(sq.Eq{"channel_id": channelID}).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("build query: %w", err)
	}

	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count channel contacts: %w", err)
	}

	return count, nil
}

func (s *ChannelContactStore) CountConnectedByChannel(ctx context.Context, channelID string) (int, error) {
	query, args, err := psql.Select("COUNT(*)").
		From("channel_contacts").
		Where(sq.Eq{"channel_id": channelID}).
		Where("connected_at IS NOT NULL").
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("build query: %w", err)
	}

	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count connected channel contacts: %w", err)
	}

	return count, nil
}

func (s *ChannelContactStore) Connect(ctx context.Context, contactID string, connectedAt time.Time) error {
	query, args, err := psql.Update("channel_contacts").
		Set("connected_at", connectedAt).
		Set("connection_code", nil).
		Set("code_expires_at", nil).
		Set("updated_at", "CURRENT_TIMESTAMP").
		Where(sq.Eq{"id": contactID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}
