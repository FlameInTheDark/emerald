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

// KubernetesClusterStore persists normalized Kubernetes cluster definitions.
type KubernetesClusterStore struct {
	db        *sql.DB
	encryptor *crypto.Encryptor
}

// NewKubernetesClusterStore creates a store for Kubernetes clusters.
func NewKubernetesClusterStore(db *sql.DB, encryptor *crypto.Encryptor) *KubernetesClusterStore {
	return &KubernetesClusterStore{db: db, encryptor: encryptor}
}

// Create inserts a new Kubernetes cluster.
func (s *KubernetesClusterStore) Create(ctx context.Context, cluster *models.KubernetesCluster) error {
	cluster.ID = uuid.New().String()

	encryptedKubeconfig := cluster.Kubeconfig
	if s.encryptor != nil && encryptedKubeconfig != "" {
		var err error
		encryptedKubeconfig, err = s.encryptor.Encrypt(encryptedKubeconfig)
		if err != nil {
			return fmt.Errorf("encrypt kubeconfig: %w", err)
		}
	}

	query, args, err := psql.Insert("kubernetes_clusters").
		Columns("id", "name", "source_type", "kubeconfig", "context_name", "default_namespace", "server").
		Values(
			cluster.ID,
			cluster.Name,
			cluster.SourceType,
			encryptedKubeconfig,
			cluster.ContextName,
			cluster.DefaultNamespace,
			cluster.Server,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

// GetByID returns a Kubernetes cluster including decrypted kubeconfig data.
func (s *KubernetesClusterStore) GetByID(ctx context.Context, id string) (*models.KubernetesCluster, error) {
	query, args, err := psql.Select(
		"id",
		"name",
		"source_type",
		"kubeconfig",
		"(CASE WHEN kubeconfig != '' THEN 1 ELSE 0 END) AS has_secret",
		"context_name",
		"default_namespace",
		"server",
		"created_at",
		"updated_at",
	).From("kubernetes_clusters").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	var cluster models.KubernetesCluster
	err = s.db.QueryRowContext(ctx, query, args...).Scan(
		&cluster.ID,
		&cluster.Name,
		&cluster.SourceType,
		&cluster.Kubeconfig,
		&cluster.HasSecret,
		&cluster.ContextName,
		&cluster.DefaultNamespace,
		&cluster.Server,
		&cluster.CreatedAt,
		&cluster.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("query kubernetes cluster: %w", err)
	}

	if s.encryptor != nil && cluster.Kubeconfig != "" {
		cluster.Kubeconfig, err = s.encryptor.DecryptCompat(cluster.Kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("decrypt kubeconfig: %w", err)
		}
	}

	return &cluster, nil
}

// List returns Kubernetes clusters without decrypting kubeconfig payloads.
func (s *KubernetesClusterStore) List(ctx context.Context) ([]models.KubernetesCluster, error) {
	query, args, err := psql.Select(
		"id",
		"name",
		"source_type",
		"(CASE WHEN kubeconfig != '' THEN 1 ELSE 0 END) AS has_secret",
		"context_name",
		"default_namespace",
		"server",
		"created_at",
		"updated_at",
	).From("kubernetes_clusters").
		OrderBy("created_at DESC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query kubernetes clusters: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var clusters []models.KubernetesCluster
	for rows.Next() {
		var cluster models.KubernetesCluster
		if err := rows.Scan(
			&cluster.ID,
			&cluster.Name,
			&cluster.SourceType,
			&cluster.HasSecret,
			&cluster.ContextName,
			&cluster.DefaultNamespace,
			&cluster.Server,
			&cluster.CreatedAt,
			&cluster.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan kubernetes cluster: %w", err)
		}
		clusters = append(clusters, cluster)
	}

	return clusters, rows.Err()
}

// Update replaces the normalized Kubernetes cluster definition.
func (s *KubernetesClusterStore) Update(ctx context.Context, cluster *models.KubernetesCluster) error {
	encryptedKubeconfig := cluster.Kubeconfig
	if s.encryptor != nil && encryptedKubeconfig != "" {
		var err error
		encryptedKubeconfig, err = s.encryptor.Encrypt(encryptedKubeconfig)
		if err != nil {
			return fmt.Errorf("encrypt kubeconfig: %w", err)
		}
	}

	query, args, err := psql.Update("kubernetes_clusters").
		Set("name", cluster.Name).
		Set("source_type", cluster.SourceType).
		Set("kubeconfig", encryptedKubeconfig).
		Set("context_name", cluster.ContextName).
		Set("default_namespace", cluster.DefaultNamespace).
		Set("server", cluster.Server).
		Set("updated_at", "CURRENT_TIMESTAMP").
		Where(sq.Eq{"id": cluster.ID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

// Delete removes the Kubernetes cluster definition.
func (s *KubernetesClusterStore) Delete(ctx context.Context, id string) error {
	query, args, err := psql.Delete("kubernetes_clusters").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}
