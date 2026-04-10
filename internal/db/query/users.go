package query

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

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
	if s.encryptor == nil {
		return fmt.Errorf("encryptor is not configured")
	}

	encryptedPassword, err := s.encryptor.Encrypt(user.Password)
	if err != nil {
		return fmt.Errorf("encrypt password: %w", err)
	}

	user.ID = uuid.New().String()
	query, args, err := psql.Insert("users").
		Columns("id", "username", "password").
		Values(user.ID, strings.TrimSpace(user.Username), encryptedPassword).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *UserStore) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	query, args, err := psql.Select("id", "username", "password", "created_at", "updated_at").
		From("users").
		Where(sq.Eq{"username": strings.TrimSpace(username)}).
		Limit(1).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	var user models.User
	err = s.db.QueryRowContext(ctx, query, args...).Scan(&user.ID, &user.Username, &user.Password, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query user: %w", err)
	}

	if s.encryptor == nil {
		return nil, fmt.Errorf("encryptor is not configured")
	}

	user.Password, err = s.encryptor.Decrypt(user.Password)
	if err != nil {
		return nil, fmt.Errorf("decrypt password: %w", err)
	}

	return &user, nil
}

func (s *UserStore) List(ctx context.Context) ([]models.User, error) {
	query, args, err := psql.Select("id", "username", "created_at", "updated_at").
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
	defer rows.Close()

	users := make([]models.User, 0)
	for rows.Next() {
		var user models.User
		if err := rows.Scan(&user.ID, &user.Username, &user.CreatedAt, &user.UpdatedAt); err != nil {
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
	if s.encryptor == nil {
		return fmt.Errorf("encryptor is not configured")
	}

	encryptedPassword, err := s.encryptor.Encrypt(password)
	if err != nil {
		return fmt.Errorf("encrypt password: %w", err)
	}

	query, args, err := psql.Update("users").
		Set("password", encryptedPassword).
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
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username is required")
	}

	existing, err := s.GetByUsername(ctx, username)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	return s.Create(ctx, &models.User{
		Username: username,
		Password: password,
	})
}
