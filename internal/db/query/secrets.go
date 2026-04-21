package query

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/FlameInTheDark/emerald/internal/crypto"
	"github.com/FlameInTheDark/emerald/internal/db/models"
	sq "github.com/Masterminds/squirrel"
)

var secretNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type SecretStore struct {
	db        *sql.DB
	encryptor *crypto.Encryptor
}

func NewSecretStore(db *sql.DB, encryptor *crypto.Encryptor) *SecretStore {
	return &SecretStore{db: db, encryptor: encryptor}
}

func (s *SecretStore) List(ctx context.Context) ([]models.Secret, error) {
	query, args, err := psql.Select("id", "name", "(CASE WHEN value != '' THEN 1 ELSE 0 END) AS has_value", "created_at", "updated_at").
		From("secrets").
		OrderBy("name ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query secrets: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	secrets := make([]models.Secret, 0)
	for rows.Next() {
		var secret models.Secret
		if err := rows.Scan(&secret.ID, &secret.Name, &secret.HasValue, &secret.CreatedAt, &secret.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan secret: %w", err)
		}
		secrets = append(secrets, secret)
	}

	return secrets, rows.Err()
}

func (s *SecretStore) GetByID(ctx context.Context, id string) (*models.Secret, error) {
	query, args, err := psql.Select("id", "name", "(CASE WHEN value != '' THEN 1 ELSE 0 END) AS has_value", "created_at", "updated_at").
		From("secrets").
		Where(sq.Eq{"id": strings.TrimSpace(id)}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	var secret models.Secret
	err = s.db.QueryRowContext(ctx, query, args...).Scan(&secret.ID, &secret.Name, &secret.HasValue, &secret.CreatedAt, &secret.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query secret: %w", err)
	}

	return &secret, nil
}

func (s *SecretStore) GetValueByID(ctx context.Context, id string) (string, bool, error) {
	query, args, err := psql.Select("value").
		From("secrets").
		Where(sq.Eq{"id": strings.TrimSpace(id)}).
		Limit(1).
		ToSql()
	if err != nil {
		return "", false, fmt.Errorf("build query: %w", err)
	}

	var encryptedValue string
	err = s.db.QueryRowContext(ctx, query, args...).Scan(&encryptedValue)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, fmt.Errorf("query secret value: %w", err)
	}

	decryptedValue, err := s.decrypt(encryptedValue)
	if err != nil {
		return "", false, fmt.Errorf("decrypt secret value: %w", err)
	}

	return decryptedValue, true, nil
}

func (s *SecretStore) GetValueByName(ctx context.Context, name string) (string, bool, error) {
	query, args, err := psql.Select("value").
		From("secrets").
		Where(sq.Eq{"name": strings.TrimSpace(name)}).
		Limit(1).
		ToSql()
	if err != nil {
		return "", false, fmt.Errorf("build query: %w", err)
	}

	var encryptedValue string
	err = s.db.QueryRowContext(ctx, query, args...).Scan(&encryptedValue)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, fmt.Errorf("query secret value: %w", err)
	}

	decryptedValue, err := s.decrypt(encryptedValue)
	if err != nil {
		return "", false, fmt.Errorf("decrypt secret value: %w", err)
	}

	return decryptedValue, true, nil
}

func (s *SecretStore) Create(ctx context.Context, secret *models.Secret) error {
	if secret == nil {
		return fmt.Errorf("secret is required")
	}
	name := strings.TrimSpace(secret.Name)
	if err := validateSecretName(name); err != nil {
		return err
	}

	encryptedValue, err := s.encrypt(secret.Value)
	if err != nil {
		return err
	}

	secret.ID = uuid.New().String()
	secret.Name = name

	query, args, err := psql.Insert("secrets").
		Columns("id", "name", "value").
		Values(secret.ID, secret.Name, encryptedValue).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("insert secret: %w", err)
	}

	return nil
}

func (s *SecretStore) Update(ctx context.Context, secret *models.Secret, replaceValue bool) error {
	if secret == nil {
		return fmt.Errorf("secret is required")
	}
	name := strings.TrimSpace(secret.Name)
	if err := validateSecretName(name); err != nil {
		return err
	}

	builder := psql.Update("secrets").
		Set("name", name).
		Set("updated_at", "CURRENT_TIMESTAMP").
		Where(sq.Eq{"id": strings.TrimSpace(secret.ID)})

	if replaceValue {
		encryptedValue, err := s.encrypt(secret.Value)
		if err != nil {
			return err
		}
		builder = builder.Set("value", encryptedValue)
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("update secret: %w", err)
	}

	return nil
}

func (s *SecretStore) Delete(ctx context.Context, id string) error {
	query, args, err := psql.Delete("secrets").
		Where(sq.Eq{"id": strings.TrimSpace(id)}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}

	return nil
}

func (s *SecretStore) TemplateValues(ctx context.Context) (map[string]string, error) {
	query, args, err := psql.Select("name", "value").
		From("secrets").
		OrderBy("name ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query secrets: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	values := make(map[string]string)
	for rows.Next() {
		var name string
		var encryptedValue string
		if err := rows.Scan(&name, &encryptedValue); err != nil {
			return nil, fmt.Errorf("scan secret value: %w", err)
		}
		decryptedValue, err := s.decrypt(encryptedValue)
		if err != nil {
			return nil, fmt.Errorf("decrypt secret %s: %w", name, err)
		}
		values[name] = decryptedValue
	}

	return values, rows.Err()
}

func (s *SecretStore) encrypt(value string) (string, error) {
	if s.encryptor == nil {
		return "", fmt.Errorf("secret encryption is not configured")
	}
	encryptedValue, err := s.encryptor.Encrypt(value)
	if err != nil {
		return "", fmt.Errorf("encrypt secret value: %w", err)
	}
	return encryptedValue, nil
}

func (s *SecretStore) decrypt(value string) (string, error) {
	if s.encryptor == nil {
		return "", fmt.Errorf("secret encryption is not configured")
	}
	decryptedValue, err := s.encryptor.DecryptCompat(value)
	if err != nil {
		return "", err
	}
	return decryptedValue, nil
}

func validateSecretName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if !secretNamePattern.MatchString(name) {
		return fmt.Errorf("name must start with a letter or underscore and contain only letters, numbers, and underscores")
	}
	return nil
}
