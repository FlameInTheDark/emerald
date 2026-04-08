export interface Cluster {
  id: string
  name: string
  host: string
  port: number
  api_token_id: string
  api_token_secret?: string
  skip_tls_verify: boolean
  created_at: string
  updated_at: string
}

export interface KubernetesManualAuthConfig {
  server: string
  token: string
  username: string
  password: string
  ca_data: string
  client_certificate_data: string
  client_key_data: string
  insecure_skip_tls_verify: boolean
}

export interface KubernetesCluster {
  id: string
  name: string
  source_type: string
  kubeconfig?: string
  context_name: string
  default_namespace: string
  server: string
  manual?: KubernetesManualAuthConfig
  created_at: string
  updated_at: string
}

export interface KubernetesTestConnectionResult {
  contexts: string[]
  effective_context: string
  default_namespace: string
  server: string
  server_version: string
}

export interface Channel {
  id: string
  name: string
  type: string
  config?: string
  welcome_message: string
  connect_url?: string
  enabled: boolean
  state?: string
  created_at: string
  updated_at: string
}

export interface ChannelContact {
  id: string
  channel_id: string
  external_user_id: string
  external_chat_id: string
  username?: string
  display_name?: string
  connection_code?: string
  code_expires_at?: string
  connected_at?: string
  last_message_at?: string
  created_at: string
  updated_at: string
}

export interface LLMProvider {
  id: string
  name: string
  provider_type: string
  api_key?: string
  base_url?: string
  model: string
  config?: string
  is_default: boolean
  created_at: string
  updated_at: string
}

export interface Pipeline {
  id: string
  name: string
  description: string | null
  nodes: string
  edges: string
  viewport?: string
  status: 'draft' | 'active' | 'archived'
  created_at: string
  updated_at: string
}

export interface FlowDefinitionDocument {
  nodes: unknown[]
  edges: unknown[]
  viewport?: Record<string, unknown>
}

export interface PipelineDocument {
  version: string
  kind: 'automator-pipeline'
  name: string
  description?: string | null
  status?: Pipeline['status'] | string
  definition: FlowDefinitionDocument
}

export interface TemplateSummary {
  id: string
  name: string
  description?: string | null
  category: string
  created_at: string
}

export interface TemplateDetail extends TemplateSummary {
  definition: FlowDefinitionDocument
}

export interface TemplateDocument {
  version: string
  kind: 'automator-template'
  name: string
  description?: string | null
  definition: FlowDefinitionDocument
}

export interface TemplateBundle {
  version: string
  kind: 'automator-template-bundle'
  templates: TemplateDocument[]
}

export interface TemplateImportFailure {
  index: number
  name?: string
  error: string
}

export interface TemplateImportResult {
  created: TemplateSummary[]
  errors: TemplateImportFailure[]
  created_count: number
  failed_count: number
}

export interface DashboardStats {
  clusters: number
  pipelines: number
  active_pipelines: number
  active_jobs: number
  executions_24h: number
  channels: number
}

export interface AuthSession {
  authenticated: boolean
  username: string
  expires_at: string
}

export interface User {
  id: string
  username: string
  created_at: string
  updated_at: string
}

export interface Execution {
  id: string
  pipeline_id: string
  trigger_type: string
  status: 'running' | 'completed' | 'failed' | 'cancelled'
  started_at: string
  completed_at?: string
  error?: string
  context?: string
}

export interface ActiveExecution {
  execution_id: string
  pipeline_id: string
  trigger_type: string
  status: 'running' | 'cancelling'
  started_at: string
  current_node_id?: string
  current_node_type?: string
  current_node_started_at?: string
}

export interface NodeExecution {
  id: string
  execution_id: string
  node_id: string
  node_type: string
  status: string
  input?: string
  output?: string
  error?: string
  started_at?: string
  completed_at?: string
}

export interface ExecutionDetail {
  execution: Execution
  node_executions: NodeExecution[]
}

export interface PipelineRunResponse {
  execution_id: string
  status: 'completed' | 'failed' | 'cancelled'
  duration: string
  nodes_run: number
  error?: string
  returned?: boolean
  return_value?: unknown
}

export interface NodeExecutionLogData {
  status: string
  input?: unknown
  output?: unknown
  error?: string
  node_type?: string
}

export interface LLMModelInfo {
  id: string
  name?: string
  description?: string
  context_length?: number
}

export interface LLMToolFunction {
  name: string
  arguments: string
}

export interface LLMToolCall {
  id: string
  type: string
  function: LLMToolFunction
}

export interface LLMToolResult {
  tool: string
  arguments?: unknown
  result?: unknown
  error?: string
}

export interface LLMUsage {
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
}

export interface LLMChatResponse {
  content: string
  tool_calls?: LLMToolCall[]
  tool_results?: LLMToolResult[]
  usage?: LLMUsage
}

export interface TemplateSuggestion {
  template: string
  expression: string
  label: string
  description?: string
}

export type NodeType =
  | 'trigger:manual'
  | 'trigger:cron'
  | 'trigger:webhook'
  | 'trigger:channel_message'
  | 'action:proxmox_list_nodes'
  | 'action:proxmox_list_workloads'
  | 'action:vm_start'
  | 'action:vm_stop'
  | 'action:vm_clone'
  | 'action:kubernetes_api_resources'
  | 'action:kubernetes_list_resources'
  | 'action:kubernetes_get_resource'
  | 'action:kubernetes_apply_manifest'
  | 'action:kubernetes_patch_resource'
  | 'action:kubernetes_delete_resource'
  | 'action:kubernetes_scale_resource'
  | 'action:kubernetes_rollout_restart'
  | 'action:kubernetes_rollout_status'
  | 'action:kubernetes_pod_logs'
  | 'action:kubernetes_pod_exec'
  | 'action:kubernetes_events'
  | 'action:http'
  | 'action:shell_command'
  | 'action:lua'
  | 'action:channel_send_message'
  | 'action:channel_reply_message'
  | 'action:channel_edit_message'
  | 'action:channel_send_and_wait'
  | 'action:pipeline_get'
  | 'action:pipeline_run'
  | 'tool:proxmox_list_nodes'
  | 'tool:proxmox_list_workloads'
  | 'tool:vm_start'
  | 'tool:vm_stop'
  | 'tool:vm_clone'
  | 'tool:kubernetes_api_resources'
  | 'tool:kubernetes_list_resources'
  | 'tool:kubernetes_get_resource'
  | 'tool:kubernetes_apply_manifest'
  | 'tool:kubernetes_patch_resource'
  | 'tool:kubernetes_delete_resource'
  | 'tool:kubernetes_scale_resource'
  | 'tool:kubernetes_rollout_restart'
  | 'tool:kubernetes_rollout_status'
  | 'tool:kubernetes_pod_logs'
  | 'tool:kubernetes_pod_exec'
  | 'tool:kubernetes_events'
  | 'tool:http'
  | 'tool:shell_command'
  | 'tool:pipeline_list'
  | 'tool:pipeline_get'
  | 'tool:pipeline_create'
  | 'tool:pipeline_update'
  | 'tool:pipeline_delete'
  | 'tool:pipeline_run'
  | 'tool:channel_send_and_wait'
  | 'logic:condition'
  | 'logic:switch'
  | 'logic:merge'
  | 'logic:aggregate'
  | 'logic:return'
  | 'llm:prompt'
  | 'llm:agent'
  | 'visual:group'

export interface NodeTypeDefinition {
  type: NodeType
  label: string
  description: string
  icon: string
  category: string
  color: string
  defaultConfig: Record<string, unknown>
}

export interface NodeCategory {
  id: string
  label: string
  color: string
  types: NodeTypeDefinition[]
}

export interface Toast {
  id: string
  type: 'success' | 'error' | 'info' | 'warning'
  title: string
  message?: string
  duration?: number
}
