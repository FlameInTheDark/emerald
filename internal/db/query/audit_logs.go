package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/google/uuid"
)

type AuditLogStore struct {
	db *sql.DB
}

func NewAuditLogStore(db *sql.DB) *AuditLogStore {
	return &AuditLogStore{db: db}
}

func (s *AuditLogStore) Create(ctx context.Context, entry models.AuditLog) error {
	entry.ID = uuid.New().String()

	query, args, err := psql.Insert("audit_log").
		Columns("id", "actor_type", "actor_id", "action", "resource_type", "resource_id", "details").
		Values(
			entry.ID,
			strings.TrimSpace(entry.ActorType),
			entry.ActorID,
			strings.TrimSpace(entry.Action),
			entry.ResourceType,
			entry.ResourceID,
			entry.Details,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("build audit log insert: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}

func MarshalAuditDetails(details any) *string {
	if details == nil {
		return nil
	}
	encoded, err := json.Marshal(details)
	if err != nil {
		return nil
	}
	value := string(encoded)
	return &value
}
