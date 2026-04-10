package query

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/FlameInTheDark/emerald/internal/db"
	"github.com/FlameInTheDark/emerald/internal/db/models"
)

func TestChatStoreCreateListAndAppendTurn(t *testing.T) {
	t.Parallel()

	store := newChatStoreForTest(t)
	ctx := context.Background()

	conversation := &models.ChatConversation{
		UserID:            "user-1",
		Title:             "First chat",
		ProxmoxEnabled:    true,
		KubernetesEnabled: false,
	}
	userMessage := &models.ChatMessage{Role: "user", Content: "Hello there"}
	assistantMessage := &models.ChatMessage{Role: "assistant", Content: "Hi!"}

	if err := store.AppendTurn(ctx, conversation, true, userMessage, assistantMessage); err != nil {
		t.Fatalf("AppendTurn(create): %v", err)
	}
	if conversation.ID == "" {
		t.Fatal("expected conversation ID to be assigned")
	}

	listed, err := store.ListByUser(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("listed conversations = %d, want 1", len(listed))
	}
	if listed[0].Title != "First chat" {
		t.Fatalf("title = %q, want %q", listed[0].Title, "First chat")
	}

	messages, err := store.ListMessages(ctx, conversation.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("message count = %d, want 2", len(messages))
	}
	if messages[0].Role != "user" || messages[1].Role != "assistant" {
		t.Fatalf("unexpected message order: %+v", messages)
	}

	conversation.KubernetesEnabled = true
	secondUserMessage := &models.ChatMessage{Role: "user", Content: "Show me the clusters"}
	secondAssistantMessage := &models.ChatMessage{Role: "assistant", Content: "Here they are"}
	if err := store.AppendTurn(ctx, conversation, false, secondUserMessage, secondAssistantMessage); err != nil {
		t.Fatalf("AppendTurn(update): %v", err)
	}

	reloaded, err := store.GetConversationByID(ctx, "user-1", conversation.ID)
	if err != nil {
		t.Fatalf("GetConversationByID: %v", err)
	}
	if reloaded == nil {
		t.Fatal("expected conversation to exist")
	}
	if !reloaded.KubernetesEnabled {
		t.Fatal("expected updated kubernetes setting to persist")
	}

	messages, err = store.ListMessages(ctx, conversation.ID)
	if err != nil {
		t.Fatalf("ListMessages after append: %v", err)
	}
	if len(messages) != 4 {
		t.Fatalf("message count after append = %d, want 4", len(messages))
	}
}

func TestChatStoreScopesConversationsByUser(t *testing.T) {
	t.Parallel()

	store := newChatStoreForTest(t)
	ctx := context.Background()

	first := &models.ChatConversation{
		UserID:            "user-1",
		Title:             "User one chat",
		ProxmoxEnabled:    true,
		KubernetesEnabled: false,
	}
	second := &models.ChatConversation{
		UserID:            "user-2",
		Title:             "User two chat",
		ProxmoxEnabled:    false,
		KubernetesEnabled: true,
	}

	if err := store.AppendTurn(ctx, first, true, &models.ChatMessage{Role: "user", Content: "Hi"}, &models.ChatMessage{Role: "assistant", Content: "Hello"}); err != nil {
		t.Fatalf("AppendTurn first: %v", err)
	}
	if err := store.AppendTurn(ctx, second, true, &models.ChatMessage{Role: "user", Content: "Hey"}, &models.ChatMessage{Role: "assistant", Content: "Hi"}); err != nil {
		t.Fatalf("AppendTurn second: %v", err)
	}

	listed, err := store.ListByUser(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != first.ID {
		t.Fatalf("unexpected scoped conversations: %+v", listed)
	}

	conversation, err := store.GetConversationByID(ctx, "user-1", second.ID)
	if err != nil {
		t.Fatalf("GetConversationByID cross-user: %v", err)
	}
	if conversation != nil {
		t.Fatalf("expected no conversation for other user, got %+v", conversation)
	}
}

func TestChatStoreUpdateConversationSettings(t *testing.T) {
	t.Parallel()

	store := newChatStoreForTest(t)
	ctx := context.Background()
	providerID := "provider-1"
	clusterID := "cluster-1"

	conversation := &models.ChatConversation{
		UserID:              "user-1",
		Title:               "Persist settings",
		ProviderID:          &providerID,
		ProxmoxEnabled:      true,
		ProxmoxClusterID:    &clusterID,
		KubernetesEnabled:   false,
		KubernetesClusterID: nil,
	}
	if err := store.AppendTurn(ctx, conversation, true, &models.ChatMessage{Role: "user", Content: "Hello"}, &models.ChatMessage{Role: "assistant", Content: "Hi"}); err != nil {
		t.Fatalf("AppendTurn: %v", err)
	}

	conversation.KubernetesEnabled = true
	conversation.KubernetesClusterID = &clusterID
	if err := store.UpdateConversationSettings(ctx, conversation); err != nil {
		t.Fatalf("UpdateConversationSettings: %v", err)
	}

	reloaded, err := store.GetConversationByID(ctx, "user-1", conversation.ID)
	if err != nil {
		t.Fatalf("GetConversationByID: %v", err)
	}
	if reloaded == nil {
		t.Fatal("expected conversation")
	}
	if reloaded.ProviderID == nil || *reloaded.ProviderID != providerID {
		t.Fatalf("provider id = %v, want %q", reloaded.ProviderID, providerID)
	}
	if !reloaded.KubernetesEnabled {
		t.Fatal("expected kubernetes to be enabled")
	}
	if reloaded.KubernetesClusterID == nil || *reloaded.KubernetesClusterID != clusterID {
		t.Fatalf("kubernetes cluster id = %v, want %q", reloaded.KubernetesClusterID, clusterID)
	}
}

func TestChatStorePersistsHiddenContextFields(t *testing.T) {
	t.Parallel()

	store := newChatStoreForTest(t)
	ctx := context.Background()
	providerID := "provider-1"
	summary := "- User wants cluster health summaries"
	contextMessages := `[{"role":"assistant","content":"Working on it"}]`

	conversation := &models.ChatConversation{
		UserID:                "user-1",
		Title:                 "Hidden context",
		ProviderID:            &providerID,
		ProxmoxEnabled:        true,
		ContextSummary:        &summary,
		CompactedMessageCount: 2,
		CompactionCount:       1,
		ContextWindow:         128000,
		ContextTokenCount:     2048,
		LastPromptTokens:      321,
		LastCompletionTokens:  87,
		LastTotalTokens:       408,
	}

	if err := store.AppendTurn(
		ctx,
		conversation,
		true,
		&models.ChatMessage{Role: "user", Content: "Summarize the cluster"},
		&models.ChatMessage{
			Role:            "assistant",
			Content:         "Here is the summary",
			ContextMessages: &contextMessages,
		},
	); err != nil {
		t.Fatalf("AppendTurn: %v", err)
	}

	reloaded, err := store.GetConversationByID(ctx, "user-1", conversation.ID)
	if err != nil {
		t.Fatalf("GetConversationByID: %v", err)
	}
	if reloaded == nil {
		t.Fatal("expected conversation")
	}
	if reloaded.ContextSummary == nil || *reloaded.ContextSummary != summary {
		t.Fatalf("context summary = %v, want %q", reloaded.ContextSummary, summary)
	}
	if reloaded.CompactedMessageCount != 2 {
		t.Fatalf("compacted message count = %d, want 2", reloaded.CompactedMessageCount)
	}
	if reloaded.CompactionCount != 1 {
		t.Fatalf("compaction count = %d, want 1", reloaded.CompactionCount)
	}
	if reloaded.ContextWindow != 128000 || reloaded.ContextTokenCount != 2048 {
		t.Fatalf("unexpected context stats: %+v", reloaded)
	}
	if reloaded.LastPromptTokens != 321 || reloaded.LastCompletionTokens != 87 || reloaded.LastTotalTokens != 408 {
		t.Fatalf("unexpected usage stats: %+v", reloaded)
	}

	messages, err := store.ListMessages(ctx, conversation.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("message count = %d, want 2", len(messages))
	}
	if messages[1].ContextMessages == nil || *messages[1].ContextMessages != contextMessages {
		t.Fatalf("context messages = %v, want %q", messages[1].ContextMessages, contextMessages)
	}
}

func TestChatStoreDeleteConversationRemovesMessages(t *testing.T) {
	t.Parallel()

	store := newChatStoreForTest(t)
	ctx := context.Background()

	conversation := &models.ChatConversation{
		UserID:            "user-1",
		Title:             "Delete me",
		ProxmoxEnabled:    false,
		KubernetesEnabled: false,
	}
	if err := store.AppendTurn(ctx, conversation, true, &models.ChatMessage{Role: "user", Content: "Hello"}, &models.ChatMessage{Role: "assistant", Content: "Hi"}); err != nil {
		t.Fatalf("AppendTurn: %v", err)
	}

	if err := store.DeleteConversation(ctx, "user-1", conversation.ID); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}

	reloaded, err := store.GetConversationByID(ctx, "user-1", conversation.ID)
	if err != nil {
		t.Fatalf("GetConversationByID: %v", err)
	}
	if reloaded != nil {
		t.Fatalf("expected conversation to be deleted, got %+v", reloaded)
	}

	messages, err := store.ListMessages(ctx, conversation.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected messages to be deleted, got %d", len(messages))
	}
}

func newChatStoreForTest(t *testing.T) *ChatStore {
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

	if _, err := database.Exec(`INSERT INTO users (id, username, password) VALUES (?, ?, ?), (?, ?, ?)`,
		"user-1", "user-one", "secret",
		"user-2", "user-two", "secret",
	); err != nil {
		t.Fatalf("seed users: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO llm_providers (id, name, provider_type, api_key, model, is_default) VALUES (?, ?, ?, ?, ?, ?)`,
		"provider-1", "Default", "openai", "secret", "gpt-test", 1,
	); err != nil {
		t.Fatalf("seed llm providers: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO clusters (id, name, host, port, api_token_id, api_token_secret, skip_tls_verify) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"cluster-1", "Primary", "localhost", 8006, "token-id", "token-secret", 1,
	); err != nil {
		t.Fatalf("seed clusters: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO kubernetes_clusters (id, name, source_type, kubeconfig, context_name, default_namespace, server) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"cluster-1", "Primary", "manual", "apiVersion: v1", "default", "default", "https://localhost",
	); err != nil {
		t.Fatalf("seed kubernetes clusters: %v", err)
	}

	return NewChatStore(database.DB)
}
