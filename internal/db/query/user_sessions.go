package query

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/FlameInTheDark/emerald/internal/auth"
	sq "github.com/Masterminds/squirrel"
)

type UserSessionStore struct {
	db *sql.DB
}

func NewUserSessionStore(db *sql.DB) *UserSessionStore {
	return &UserSessionStore{db: db}
}

func (s *UserSessionStore) Create(ctx context.Context, token string, session auth.Session) error {
	query, args, err := psql.Insert("user_sessions").
		Columns("token_hash", "user_id", "username", "is_super_admin", "expires_at").
		Values(hashSessionToken(token), strings.TrimSpace(session.UserID), strings.TrimSpace(session.Username), boolToInt(session.IsSuperAdmin), session.ExpiresAt.UTC()).
		Suffix("ON CONFLICT(token_hash) DO UPDATE SET user_id = excluded.user_id, username = excluded.username, is_super_admin = excluded.is_super_admin, expires_at = excluded.expires_at, updated_at = CURRENT_TIMESTAMP").
		ToSql()
	if err != nil {
		return fmt.Errorf("build user session insert: %w", err)
	}

	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("insert user session: %w", err)
	}
	return nil
}

func (s *UserSessionStore) GetByToken(ctx context.Context, token string, now time.Time) (auth.Session, bool, error) {
	trimmedToken := strings.TrimSpace(token)
	if trimmedToken == "" {
		return auth.Session{}, false, nil
	}

	query, args, err := psql.Select("user_id", "username", "is_super_admin", "expires_at").
		From("user_sessions").
		Where(sq.Eq{"token_hash": hashSessionToken(trimmedToken)}).
		Limit(1).
		ToSql()
	if err != nil {
		return auth.Session{}, false, fmt.Errorf("build user session query: %w", err)
	}

	var session auth.Session
	err = s.db.QueryRowContext(ctx, query, args...).Scan(&session.UserID, &session.Username, &session.IsSuperAdmin, &session.ExpiresAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return auth.Session{}, false, nil
		}
		return auth.Session{}, false, fmt.Errorf("query user session: %w", err)
	}

	if !session.ExpiresAt.After(now) {
		if deleteErr := s.Delete(ctx, trimmedToken); deleteErr != nil {
			return auth.Session{}, false, deleteErr
		}
		return auth.Session{}, false, nil
	}

	return session, true, nil
}

func (s *UserSessionStore) Delete(ctx context.Context, token string) error {
	query, args, err := psql.Delete("user_sessions").
		Where(sq.Eq{"token_hash": hashSessionToken(token)}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build user session delete: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("delete user session: %w", err)
	}
	return nil
}

func (s *UserSessionStore) DeleteByUserID(ctx context.Context, userID string) error {
	query, args, err := psql.Delete("user_sessions").
		Where(sq.Eq{"user_id": strings.TrimSpace(userID)}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build user session delete by user: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("delete user sessions by user: %w", err)
	}
	return nil
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}
