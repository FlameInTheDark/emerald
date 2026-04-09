package models

import "time"

type Cluster struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Host           string    `json:"host"`
	Port           int       `json:"port"`
	APITokenID     string    `json:"api_token_id"`
	APITokenSecret string    `json:"api_token_secret,omitempty"`
	SkipTLSVerify  bool      `json:"skip_tls_verify"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type KubernetesCluster struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	SourceType       string    `json:"source_type"`
	Kubeconfig       string    `json:"kubeconfig,omitempty"`
	ContextName      string    `json:"context_name"`
	DefaultNamespace string    `json:"default_namespace"`
	Server           string    `json:"server"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type AppConfig struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Password  string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type LLMProvider struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	ProviderType string    `json:"provider_type"`
	APIKey       string    `json:"api_key,omitempty"`
	BaseURL      *string   `json:"base_url,omitempty"`
	Model        string    `json:"model"`
	Config       *string   `json:"config,omitempty"`
	IsDefault    bool      `json:"is_default"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Channel struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Type           string    `json:"type"`
	Config         *string   `json:"config,omitempty"`
	WelcomeMessage string    `json:"welcome_message"`
	ConnectURL     *string   `json:"connect_url,omitempty"`
	Enabled        bool      `json:"enabled"`
	State          *string   `json:"state,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ChannelContact struct {
	ID             string     `json:"id"`
	ChannelID      string     `json:"channel_id"`
	ExternalUserID string     `json:"external_user_id"`
	ExternalChatID string     `json:"external_chat_id"`
	Username       *string    `json:"username,omitempty"`
	DisplayName    *string    `json:"display_name,omitempty"`
	ConnectionCode *string    `json:"connection_code,omitempty"`
	CodeExpiresAt  *time.Time `json:"code_expires_at,omitempty"`
	ConnectedAt    *time.Time `json:"connected_at,omitempty"`
	LastMessageAt  *time.Time `json:"last_message_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type Pipeline struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	Nodes       string    `json:"nodes"`
	Edges       string    `json:"edges"`
	Viewport    *string   `json:"viewport,omitempty"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CronJob struct {
	ID         string     `json:"id"`
	PipelineID string     `json:"pipeline_id"`
	Schedule   string     `json:"schedule"`
	Timezone   string     `json:"timezone"`
	Enabled    bool       `json:"enabled"`
	LastRun    *time.Time `json:"last_run,omitempty"`
	NextRun    *time.Time `json:"next_run,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type Execution struct {
	ID          string     `json:"id"`
	PipelineID  string     `json:"pipeline_id"`
	TriggerType string     `json:"trigger_type"`
	Status      string     `json:"status"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Error       *string    `json:"error,omitempty"`
	Context     *string    `json:"context,omitempty"`
}

type NodeExecution struct {
	ID          string     `json:"id"`
	ExecutionID string     `json:"execution_id"`
	NodeID      string     `json:"node_id"`
	NodeType    string     `json:"node_type"`
	Status      string     `json:"status"`
	Input       *string    `json:"input,omitempty"`
	Output      *string    `json:"output,omitempty"`
	Error       *string    `json:"error,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type AuditLog struct {
	ID           string    `json:"id"`
	ActorType    string    `json:"actor_type"`
	ActorID      *string   `json:"actor_id,omitempty"`
	Action       string    `json:"action"`
	ResourceType *string   `json:"resource_type,omitempty"`
	ResourceID   *string   `json:"resource_id,omitempty"`
	Details      *string   `json:"details,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type Template struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  *string   `json:"description,omitempty"`
	Category     string    `json:"category"`
	PipelineData string    `json:"pipeline_data"`
	CreatedAt    time.Time `json:"created_at"`
}

type DashboardStats struct {
	Clusters        int `json:"clusters"`
	Pipelines       int `json:"pipelines"`
	ActivePipelines int `json:"active_pipelines"`
	ActiveJobs      int `json:"active_jobs"`
	Executions24h   int `json:"executions_24h"`
	Channels        int `json:"channels"`
}

type ChatConversation struct {
	ID                    string     `json:"id"`
	UserID                string     `json:"user_id"`
	Title                 string     `json:"title"`
	ProviderID            *string    `json:"provider_id,omitempty"`
	ProxmoxEnabled        bool       `json:"proxmox_enabled"`
	ProxmoxClusterID      *string    `json:"proxmox_cluster_id,omitempty"`
	KubernetesEnabled     bool       `json:"kubernetes_enabled"`
	KubernetesClusterID   *string    `json:"kubernetes_cluster_id,omitempty"`
	ContextSummary        *string    `json:"-"`
	CompactedMessageCount int        `json:"compacted_message_count"`
	CompactionCount       int        `json:"compaction_count"`
	CompactedAt           *time.Time `json:"compacted_at,omitempty"`
	ContextWindow         int        `json:"context_window"`
	ContextTokenCount     int        `json:"context_token_count"`
	LastPromptTokens      int        `json:"last_prompt_tokens"`
	LastCompletionTokens  int        `json:"last_completion_tokens"`
	LastTotalTokens       int        `json:"last_total_tokens"`
	LastMessageAt         time.Time  `json:"last_message_at"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type ChatMessage struct {
	ID              string    `json:"id"`
	ConversationID  string    `json:"conversation_id"`
	Role            string    `json:"role"`
	Content         string    `json:"content"`
	ToolCalls       *string   `json:"tool_calls,omitempty"`
	ToolResults     *string   `json:"tool_results,omitempty"`
	Usage           *string   `json:"usage,omitempty"`
	ContextMessages *string   `json:"-"`
	CreatedAt       time.Time `json:"created_at"`
}
