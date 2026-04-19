package query

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/FlameInTheDark/emerald/internal/crypto"
	"github.com/FlameInTheDark/emerald/internal/db"
)

func TestChannelStoreLoadsLegacyPlaintextConfig(t *testing.T) {
	t.Parallel()

	database, encryptor := newCompatTestDB(t)
	config := `{"botToken":"legacy-token"}`

	_, err := database.ExecContext(
		context.Background(),
		`INSERT INTO channels (id, name, type, config, welcome_message, enabled) VALUES (?, ?, ?, ?, ?, ?)`,
		"channel-1",
		"Legacy Telegram",
		"telegram",
		config,
		"Welcome",
		1,
	)
	if err != nil {
		t.Fatalf("insert channel: %v", err)
	}

	store := NewChannelStore(database.DB, encryptor)

	channel, err := store.GetByID(context.Background(), "channel-1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if channel == nil || channel.Config == nil || *channel.Config != config {
		t.Fatalf("unexpected config from GetByID: %#v", channel)
	}

	channels, err := store.ListEnabled(context.Background())
	if err != nil {
		t.Fatalf("ListEnabled: %v", err)
	}
	if len(channels) != 1 || channels[0].Config == nil || *channels[0].Config != config {
		t.Fatalf("unexpected channels from ListEnabled: %#v", channels)
	}
}

func TestClusterStoreLoadsLegacyPlaintextSecret(t *testing.T) {
	t.Parallel()

	database, encryptor := newCompatTestDB(t)

	_, err := database.ExecContext(
		context.Background(),
		`INSERT INTO clusters (id, name, host, port, api_token_id, api_token_secret, skip_tls_verify) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"cluster-1",
		"Legacy Cluster",
		"127.0.0.1",
		8006,
		"root@pam!emerald",
		"legacy-secret",
		0,
	)
	if err != nil {
		t.Fatalf("insert cluster: %v", err)
	}

	store := NewClusterStore(database.DB, encryptor)
	cluster, err := store.GetByID(context.Background(), "cluster-1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if cluster == nil || cluster.APITokenSecret != "legacy-secret" {
		t.Fatalf("unexpected cluster secret: %#v", cluster)
	}
}

func TestLLMProviderStoreLoadsLegacyPlaintextAPIKey(t *testing.T) {
	t.Parallel()

	database, encryptor := newCompatTestDB(t)

	_, err := database.ExecContext(
		context.Background(),
		`INSERT INTO llm_providers (id, name, provider_type, api_key, model, is_default) VALUES (?, ?, ?, ?, ?, ?)`,
		"provider-1",
		"Legacy OpenAI",
		"openai",
		"sk-legacy",
		"gpt-4o",
		1,
	)
	if err != nil {
		t.Fatalf("insert llm provider: %v", err)
	}

	store := NewLLMProviderStore(database.DB, encryptor)

	provider, err := store.GetByID(context.Background(), "provider-1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if provider == nil || provider.APIKey != "sk-legacy" {
		t.Fatalf("unexpected provider from GetByID: %#v", provider)
	}

	defaultProvider, err := store.GetDefault(context.Background())
	if err != nil {
		t.Fatalf("GetDefault: %v", err)
	}
	if defaultProvider == nil || defaultProvider.APIKey != "sk-legacy" {
		t.Fatalf("unexpected provider from GetDefault: %#v", defaultProvider)
	}
}

func TestKubernetesClusterStoreLoadsLegacyPlaintextKubeconfig(t *testing.T) {
	t.Parallel()

	database, encryptor := newCompatTestDB(t)
	kubeconfig := "apiVersion: v1\nclusters: []\ncontexts: []\ncurrent-context: \"\"\nkind: Config\nusers: []\n"

	_, err := database.ExecContext(
		context.Background(),
		`INSERT INTO kubernetes_clusters (id, name, source_type, kubeconfig, context_name, default_namespace, server) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"k8s-1",
		"Legacy K8s",
		"kubeconfig",
		kubeconfig,
		"default",
		"default",
		"https://cluster.example:6443",
	)
	if err != nil {
		t.Fatalf("insert kubernetes cluster: %v", err)
	}

	store := NewKubernetesClusterStore(database.DB, encryptor)
	cluster, err := store.GetByID(context.Background(), "k8s-1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if cluster == nil || cluster.Kubeconfig != kubeconfig {
		t.Fatalf("unexpected kubeconfig: %#v", cluster)
	}
}

func newCompatTestDB(t *testing.T) (*db.DB, *crypto.Encryptor) {
	t.Helper()

	database, err := db.New(filepath.Join(t.TempDir(), "emerald.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})

	if err := db.Migrate(database); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	appConfigStore := NewAppConfigStore(database.DB)
	key, err := appConfigStore.EnsureEncryptionKey(context.Background(), "")
	if err != nil {
		t.Fatalf("EnsureEncryptionKey: %v", err)
	}

	encryptor, err := crypto.NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	return database, encryptor
}
