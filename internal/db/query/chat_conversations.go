package query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
)

type ChatStore struct {
	db *sql.DB
}

func NewChatStore(db *sql.DB) *ChatStore {
	return &ChatStore{db: db}
}

func (s *ChatStore) ListByUser(ctx context.Context, userID string) ([]models.ChatConversation, error) {
	query, args, err := psql.
		Select(
			"id",
			"user_id",
			"title",
			"provider_id",
			"proxmox_enabled",
			"proxmox_cluster_id",
			"kubernetes_enabled",
			"kubernetes_cluster_id",
			"context_summary",
			"compacted_message_count",
			"compaction_count",
			"compacted_at",
			"context_window",
			"context_token_count",
			"last_prompt_tokens",
			"last_completion_tokens",
			"last_total_tokens",
			"last_message_at",
			"created_at",
			"updated_at",
		).
		From("chat_conversations").
		Where(sq.Eq{"user_id": strings.TrimSpace(userID)}).
		OrderBy("last_message_at DESC", "created_at DESC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query chat conversations: %w", err)
	}
	defer rows.Close()

	conversations := make([]models.ChatConversation, 0)
	for rows.Next() {
		conversation, err := scanChatConversation(rows)
		if err != nil {
			return nil, err
		}
		conversations = append(conversations, conversation)
	}

	return conversations, rows.Err()
}

func (s *ChatStore) GetConversationByID(ctx context.Context, userID string, conversationID string) (*models.ChatConversation, error) {
	query, args, err := psql.
		Select(
			"id",
			"user_id",
			"title",
			"provider_id",
			"proxmox_enabled",
			"proxmox_cluster_id",
			"kubernetes_enabled",
			"kubernetes_cluster_id",
			"context_summary",
			"compacted_message_count",
			"compaction_count",
			"compacted_at",
			"context_window",
			"context_token_count",
			"last_prompt_tokens",
			"last_completion_tokens",
			"last_total_tokens",
			"last_message_at",
			"created_at",
			"updated_at",
		).
		From("chat_conversations").
		Where(sq.Eq{
			"id":      strings.TrimSpace(conversationID),
			"user_id": strings.TrimSpace(userID),
		}).
		Limit(1).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	row := s.db.QueryRowContext(ctx, query, args...)
	conversation, err := scanChatConversation(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &conversation, nil
}

func (s *ChatStore) ListMessages(ctx context.Context, conversationID string) ([]models.ChatMessage, error) {
	query, args, err := psql.
		Select("id", "conversation_id", "role", "content", "tool_calls", "tool_results", "usage", "context_messages", "created_at").
		From("chat_messages").
		Where(sq.Eq{"conversation_id": strings.TrimSpace(conversationID)}).
		OrderBy("created_at ASC", "rowid ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query chat messages: %w", err)
	}
	defer rows.Close()

	messages := make([]models.ChatMessage, 0)
	for rows.Next() {
		message, err := scanChatMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}

	return messages, rows.Err()
}

func (s *ChatStore) CreateConversation(ctx context.Context, conversation *models.ChatConversation) error {
	if conversation == nil {
		return fmt.Errorf("conversation is required")
	}

	conversation.ID = uuid.New().String()
	query, args, err := psql.
		Insert("chat_conversations").
		Columns(
			"id",
			"user_id",
			"title",
			"provider_id",
			"proxmox_enabled",
			"proxmox_cluster_id",
			"kubernetes_enabled",
			"kubernetes_cluster_id",
			"context_summary",
			"compacted_message_count",
			"compaction_count",
			"compacted_at",
			"context_window",
			"context_token_count",
			"last_prompt_tokens",
			"last_completion_tokens",
			"last_total_tokens",
			"last_message_at",
		).
		Values(
			conversation.ID,
			strings.TrimSpace(conversation.UserID),
			strings.TrimSpace(conversation.Title),
			conversation.ProviderID,
			boolToInt(conversation.ProxmoxEnabled),
			conversation.ProxmoxClusterID,
			boolToInt(conversation.KubernetesEnabled),
			conversation.KubernetesClusterID,
			conversation.ContextSummary,
			conversation.CompactedMessageCount,
			conversation.CompactionCount,
			conversation.CompactedAt,
			conversation.ContextWindow,
			conversation.ContextTokenCount,
			conversation.LastPromptTokens,
			conversation.LastCompletionTokens,
			conversation.LastTotalTokens,
			sq.Expr("CURRENT_TIMESTAMP"),
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("create chat conversation: %w", err)
	}

	return nil
}

func (s *ChatStore) UpdateConversationSettings(ctx context.Context, conversation *models.ChatConversation) error {
	if conversation == nil {
		return fmt.Errorf("conversation is required")
	}

	query, args, err := psql.
		Update("chat_conversations").
		Set("provider_id", conversation.ProviderID).
		Set("proxmox_enabled", boolToInt(conversation.ProxmoxEnabled)).
		Set("proxmox_cluster_id", conversation.ProxmoxClusterID).
		Set("kubernetes_enabled", boolToInt(conversation.KubernetesEnabled)).
		Set("kubernetes_cluster_id", conversation.KubernetesClusterID).
		Set("context_summary", conversation.ContextSummary).
		Set("compacted_message_count", conversation.CompactedMessageCount).
		Set("compaction_count", conversation.CompactionCount).
		Set("compacted_at", conversation.CompactedAt).
		Set("context_window", conversation.ContextWindow).
		Set("context_token_count", conversation.ContextTokenCount).
		Set("last_prompt_tokens", conversation.LastPromptTokens).
		Set("last_completion_tokens", conversation.LastCompletionTokens).
		Set("last_total_tokens", conversation.LastTotalTokens).
		Set("updated_at", sq.Expr("CURRENT_TIMESTAMP")).
		Where(sq.Eq{
			"id":      strings.TrimSpace(conversation.ID),
			"user_id": strings.TrimSpace(conversation.UserID),
		}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update chat conversation: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check updated rows: %w", err)
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (s *ChatStore) DeleteConversation(ctx context.Context, userID string, conversationID string) error {
	query, args, err := psql.
		Delete("chat_conversations").
		Where(sq.Eq{
			"id":      strings.TrimSpace(conversationID),
			"user_id": strings.TrimSpace(userID),
		}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("delete chat conversation: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check deleted rows: %w", err)
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (s *ChatStore) AppendTurn(
	ctx context.Context,
	conversation *models.ChatConversation,
	createConversation bool,
	userMessage *models.ChatMessage,
	assistantMessage *models.ChatMessage,
) error {
	if conversation == nil {
		return fmt.Errorf("conversation is required")
	}
	if userMessage == nil || assistantMessage == nil {
		return fmt.Errorf("chat turn messages are required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if createConversation {
		conversation.ID = uuid.New().String()
		insertConversationSQL, insertConversationArgs, buildErr := psql.
			Insert("chat_conversations").
			Columns(
				"id",
				"user_id",
				"title",
				"provider_id",
				"proxmox_enabled",
				"proxmox_cluster_id",
				"kubernetes_enabled",
				"kubernetes_cluster_id",
				"context_summary",
				"compacted_message_count",
				"compaction_count",
				"compacted_at",
				"context_window",
				"context_token_count",
				"last_prompt_tokens",
				"last_completion_tokens",
				"last_total_tokens",
				"last_message_at",
			).
			Values(
				conversation.ID,
				strings.TrimSpace(conversation.UserID),
				strings.TrimSpace(conversation.Title),
				conversation.ProviderID,
				boolToInt(conversation.ProxmoxEnabled),
				conversation.ProxmoxClusterID,
				boolToInt(conversation.KubernetesEnabled),
				conversation.KubernetesClusterID,
				conversation.ContextSummary,
				conversation.CompactedMessageCount,
				conversation.CompactionCount,
				conversation.CompactedAt,
				conversation.ContextWindow,
				conversation.ContextTokenCount,
				conversation.LastPromptTokens,
				conversation.LastCompletionTokens,
				conversation.LastTotalTokens,
				sq.Expr("CURRENT_TIMESTAMP"),
			).
			ToSql()
		if buildErr != nil {
			err = fmt.Errorf("build query: %w", buildErr)
			return err
		}
		if _, execErr := tx.ExecContext(ctx, insertConversationSQL, insertConversationArgs...); execErr != nil {
			err = fmt.Errorf("create chat conversation: %w", execErr)
			return err
		}
	}

	userMessage.ConversationID = conversation.ID
	assistantMessage.ConversationID = conversation.ID

	if err = insertChatMessage(ctx, tx, userMessage); err != nil {
		return err
	}
	if err = insertChatMessage(ctx, tx, assistantMessage); err != nil {
		return err
	}

	updateConversationSQL, updateConversationArgs, buildErr := psql.
		Update("chat_conversations").
		Set("provider_id", conversation.ProviderID).
		Set("proxmox_enabled", boolToInt(conversation.ProxmoxEnabled)).
		Set("proxmox_cluster_id", conversation.ProxmoxClusterID).
		Set("kubernetes_enabled", boolToInt(conversation.KubernetesEnabled)).
		Set("kubernetes_cluster_id", conversation.KubernetesClusterID).
		Set("context_summary", conversation.ContextSummary).
		Set("compacted_message_count", conversation.CompactedMessageCount).
		Set("compaction_count", conversation.CompactionCount).
		Set("compacted_at", conversation.CompactedAt).
		Set("context_window", conversation.ContextWindow).
		Set("context_token_count", conversation.ContextTokenCount).
		Set("last_prompt_tokens", conversation.LastPromptTokens).
		Set("last_completion_tokens", conversation.LastCompletionTokens).
		Set("last_total_tokens", conversation.LastTotalTokens).
		Set("last_message_at", sq.Expr("CURRENT_TIMESTAMP")).
		Set("updated_at", sq.Expr("CURRENT_TIMESTAMP")).
		Where(sq.Eq{
			"id":      strings.TrimSpace(conversation.ID),
			"user_id": strings.TrimSpace(conversation.UserID),
		}).
		ToSql()
	if buildErr != nil {
		err = fmt.Errorf("build query: %w", buildErr)
		return err
	}
	result, execErr := tx.ExecContext(ctx, updateConversationSQL, updateConversationArgs...)
	if execErr != nil {
		err = fmt.Errorf("touch chat conversation: %w", execErr)
		return err
	}
	rowsAffected, rowsErr := result.RowsAffected()
	if rowsErr != nil {
		err = fmt.Errorf("check updated rows: %w", rowsErr)
		return err
	}
	if rowsAffected == 0 {
		err = sql.ErrNoRows
		return err
	}

	if commitErr := tx.Commit(); commitErr != nil {
		err = fmt.Errorf("commit transaction: %w", commitErr)
		return err
	}

	return nil
}

func insertChatMessage(ctx context.Context, exec sqlExecutor, message *models.ChatMessage) error {
	message.ID = uuid.New().String()

	query, args, err := psql.
		Insert("chat_messages").
		Columns("id", "conversation_id", "role", "content", "tool_calls", "tool_results", "usage", "context_messages").
		Values(
			message.ID,
			strings.TrimSpace(message.ConversationID),
			strings.TrimSpace(message.Role),
			message.Content,
			message.ToolCalls,
			message.ToolResults,
			message.Usage,
			message.ContextMessages,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	if _, err := exec.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("create chat message: %w", err)
	}

	return nil
}

type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type chatConversationScanner interface {
	Scan(dest ...any) error
}

func scanChatConversation(scanner chatConversationScanner) (models.ChatConversation, error) {
	var conversation models.ChatConversation
	var providerID sql.NullString
	var proxmoxClusterID sql.NullString
	var kubernetesClusterID sql.NullString
	var contextSummary sql.NullString
	var compactedAt sql.NullTime
	if err := scanner.Scan(
		&conversation.ID,
		&conversation.UserID,
		&conversation.Title,
		&providerID,
		&conversation.ProxmoxEnabled,
		&proxmoxClusterID,
		&conversation.KubernetesEnabled,
		&kubernetesClusterID,
		&contextSummary,
		&conversation.CompactedMessageCount,
		&conversation.CompactionCount,
		&compactedAt,
		&conversation.ContextWindow,
		&conversation.ContextTokenCount,
		&conversation.LastPromptTokens,
		&conversation.LastCompletionTokens,
		&conversation.LastTotalTokens,
		&conversation.LastMessageAt,
		&conversation.CreatedAt,
		&conversation.UpdatedAt,
	); err != nil {
		return models.ChatConversation{}, fmt.Errorf("scan chat conversation: %w", err)
	}

	conversation.ProviderID = nullableString(providerID)
	conversation.ProxmoxClusterID = nullableString(proxmoxClusterID)
	conversation.KubernetesClusterID = nullableString(kubernetesClusterID)
	conversation.ContextSummary = nullableString(contextSummary)
	conversation.CompactedAt = nullableTime(compactedAt)
	return conversation, nil
}

type chatMessageScanner interface {
	Scan(dest ...any) error
}

func scanChatMessage(scanner chatMessageScanner) (models.ChatMessage, error) {
	var message models.ChatMessage
	var toolCalls sql.NullString
	var toolResults sql.NullString
	var usage sql.NullString
	var contextMessages sql.NullString
	if err := scanner.Scan(
		&message.ID,
		&message.ConversationID,
		&message.Role,
		&message.Content,
		&toolCalls,
		&toolResults,
		&usage,
		&contextMessages,
		&message.CreatedAt,
	); err != nil {
		return models.ChatMessage{}, fmt.Errorf("scan chat message: %w", err)
	}

	message.ToolCalls = nullableString(toolCalls)
	message.ToolResults = nullableString(toolResults)
	message.Usage = nullableString(usage)
	message.ContextMessages = nullableString(contextMessages)
	return message, nil
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}

	trimmed := strings.TrimSpace(value.String)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func nullableTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}

	copied := value.Time
	return &copied
}
