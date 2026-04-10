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

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Question)

type ClusterStore struct {
	db        *sql.DB
	encryptor *crypto.Encryptor
}

func NewClusterStore(db *sql.DB, encryptor *crypto.Encryptor) *ClusterStore {
	return &ClusterStore{db: db, encryptor: encryptor}
}

func (s *ClusterStore) Create(ctx context.Context, c *models.Cluster) error {
	c.ID = uuid.New().String()

	encryptedSecret := c.APITokenSecret
	if s.encryptor != nil && encryptedSecret != "" {
		var err error
		encryptedSecret, err = s.encryptor.Encrypt(encryptedSecret)
		if err != nil {
			return fmt.Errorf("encrypt secret: %w", err)
		}
	}

	query, args, err := psql.Insert("clusters").
		Columns("id", "name", "host", "port", "api_token_id", "api_token_secret", "skip_tls_verify").
		Values(c.ID, c.Name, c.Host, c.Port, c.APITokenID, encryptedSecret, boolToInt(c.SkipTLSVerify)).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *ClusterStore) GetByID(ctx context.Context, id string) (*models.Cluster, error) {
	query, args, err := psql.Select("id", "name", "host", "port", "api_token_id", "api_token_secret", "skip_tls_verify", "created_at", "updated_at").
		From("clusters").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	var c models.Cluster
	err = s.db.QueryRowContext(ctx, query, args...).Scan(
		&c.ID, &c.Name, &c.Host, &c.Port, &c.APITokenID, &c.APITokenSecret,
		&c.SkipTLSVerify, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("query cluster: %w", err)
	}

	if s.encryptor != nil && c.APITokenSecret != "" {
		c.APITokenSecret, err = s.encryptor.DecryptCompat(c.APITokenSecret)
		if err != nil {
			return nil, fmt.Errorf("decrypt secret: %w", err)
		}
	}

	return &c, nil
}

func (s *ClusterStore) List(ctx context.Context) ([]models.Cluster, error) {
	query, args, err := psql.Select("id", "name", "host", "port", "api_token_id", "skip_tls_verify", "created_at", "updated_at").
		From("clusters").
		OrderBy("created_at DESC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query clusters: %w", err)
	}
	defer rows.Close()

	var clusters []models.Cluster
	for rows.Next() {
		var c models.Cluster
		if err := rows.Scan(&c.ID, &c.Name, &c.Host, &c.Port, &c.APITokenID, &c.SkipTLSVerify, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan cluster: %w", err)
		}
		clusters = append(clusters, c)
	}

	return clusters, rows.Err()
}

func (s *ClusterStore) Update(ctx context.Context, c *models.Cluster) error {
	encryptedSecret := c.APITokenSecret
	if s.encryptor != nil && encryptedSecret != "" {
		var err error
		encryptedSecret, err = s.encryptor.Encrypt(encryptedSecret)
		if err != nil {
			return fmt.Errorf("encrypt secret: %w", err)
		}
	}

	query, args, err := psql.Update("clusters").
		Set("name", c.Name).
		Set("host", c.Host).
		Set("port", c.Port).
		Set("api_token_id", c.APITokenID).
		Set("api_token_secret", encryptedSecret).
		Set("skip_tls_verify", boolToInt(c.SkipTLSVerify)).
		Set("updated_at", "CURRENT_TIMESTAMP").
		Where(sq.Eq{"id": c.ID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *ClusterStore) Delete(ctx context.Context, id string) error {
	query, args, err := psql.Delete("clusters").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
