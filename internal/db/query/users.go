package query

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/auth"
	"github.com/FlameInTheDark/emerald/internal/crypto"
	"github.com/FlameInTheDark/emerald/internal/db/models"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
)

type UserStore struct {
	db        *sql.DB
	encryptor *crypto.Encryptor
}

func NewUserStore(db *sql.DB, encryptor *crypto.Encryptor) *UserStore {
	return &UserStore{db: db, encryptor: encryptor}
}

func (s *UserStore) Create(ctx context.Context, user *models.User) error {
	if user == nil {
		return fmt.Errorf("user is required")
	}
	if strings.TrimSpace(user.Username) == "" {
		return fmt.Errorf("username is required")
	}
	if user.Password == "" {
		return fmt.Errorf("password is required")
	}
	hashedPassword, err := auth.HashPassword(user.Password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	user.ID = uuid.New().String()
	query, args, err := psql.Insert("users").
		Columns("id", "username", "password", "is_super_admin").
		Values(user.ID, strings.TrimSpace(user.Username), hashedPassword, boolToInt(user.IsSuperAdmin)).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *UserStore) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	query, args, err := psql.Select("id", "username", "password", "is_super_admin", "created_at", "updated_at").
		From("users").
		Where(sq.Eq{"username": strings.TrimSpace(username)}).
		Limit(1).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	var user models.User
	err = s.db.QueryRowContext(ctx, query, args...).Scan(&user.ID, &user.Username, &user.Password, &user.IsSuperAdmin, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query user: %w", err)
	}

	return &user, nil
}

func (s *UserStore) GetByID(ctx context.Context, id string) (*models.User, error) {
	query, args, err := psql.Select("id", "username", "password", "is_super_admin", "created_at", "updated_at").
		From("users").
		Where(sq.Eq{"id": strings.TrimSpace(id)}).
		Limit(1).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	var user models.User
	err = s.db.QueryRowContext(ctx, query, args...).Scan(&user.ID, &user.Username, &user.Password, &user.IsSuperAdmin, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query user: %w", err)
	}

	return &user, nil
}

func (s *UserStore) List(ctx context.Context) ([]models.User, error) {
	query, args, err := psql.Select("id", "username", "is_super_admin", "created_at", "updated_at").
		From("users").
		OrderBy("created_at ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query users: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	users := make([]models.User, 0)
	for rows.Next() {
		var user models.User
		if err := rows.Scan(&user.ID, &user.Username, &user.IsSuperAdmin, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, user)
	}

	return users, rows.Err()
}

func (s *UserStore) Delete(ctx context.Context, id string) error {
	query, args, err := psql.Delete("users").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *UserStore) UpdatePassword(ctx context.Context, id string, password string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("user id is required")
	}
	if password == "" {
		return fmt.Errorf("password is required")
	}
	hashedPassword, err := auth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	query, args, err := psql.Update("users").
		Set("password", hashedPassword).
		Set("updated_at", "CURRENT_TIMESTAMP").
		Where(sq.Eq{"id": strings.TrimSpace(id)}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *UserStore) EnsureDefaultUser(ctx context.Context, username string, password string) error {
	count, err := s.Count(ctx)
	if err != nil {
		return err
	}
	if count == 0 {
		username = strings.TrimSpace(username)
		if username == "" {
			return fmt.Errorf("username is required")
		}
		return s.Create(ctx, &models.User{
			Username:     username,
			Password:     password,
			IsSuperAdmin: true,
		})
	}

	hasSuperAdmin, err := s.HasSuperAdmin(ctx)
	if err != nil {
		return err
	}
	if hasSuperAdmin {
		return nil
	}

	return s.promoteOldestUserToSuperAdmin(ctx)
}

func (s *UserStore) Count(ctx context.Context) (int, error) {
	query, args, err := psql.Select("COUNT(*)").From("users").ToSql()
	if err != nil {
		return 0, fmt.Errorf("build query: %w", err)
	}

	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}

func (s *UserStore) HasSuperAdmin(ctx context.Context) (bool, error) {
	query, args, err := psql.Select("COUNT(*)").From("users").Where(sq.Eq{"is_super_admin": 1}).ToSql()
	if err != nil {
		return false, fmt.Errorf("build query: %w", err)
	}

	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return false, fmt.Errorf("count super admin users: %w", err)
	}
	return count > 0, nil
}

func (s *UserStore) VerifyLegacyPassword(stored string, provided string) (bool, error) {
	if strings.TrimSpace(stored) == "" || s.encryptor == nil {
		return false, nil
	}

	decrypted, err := s.encryptor.DecryptCompat(stored)
	if err != nil {
		return false, nil
	}

	return subtle.ConstantTimeCompare([]byte(decrypted), []byte(provided)) == 1, nil
}

func (s *UserStore) promoteOldestUserToSuperAdmin(ctx context.Context) error {
	query, args, err := psql.Update("users").
		Set("is_super_admin", 1).
		Set("updated_at", "CURRENT_TIMESTAMP").
		Where(sq.Expr("id = (SELECT id FROM users ORDER BY created_at ASC, rowid ASC LIMIT 1)")).
		ToSql()
	if err != nil {
		return fmt.Errorf("build promote super admin query: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("promote super admin: %w", err)
	}
	return nil
}
