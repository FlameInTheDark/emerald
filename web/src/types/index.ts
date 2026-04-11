import type { Edge, Node, Viewport } from '@xyflow/react'

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
  kind: 'emerald-pipeline' | 'automator-pipeline'
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
  kind: 'emerald-template' | 'automator-template'
  name: string
  description?: string | null
  definition: FlowDefinitionDocument
}

export interface TemplateBundle {
  version: string
  kind: 'emerald-template-bundle' | 'automator-template-bundle'
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
  conversation_id: string
  conversation: LLMConversationSummary
  content: string
  tool_calls?: LLMToolCall[]
  tool_results?: LLMToolResult[]
  usage?: LLMUsage
}

export type AssistantProfileScope = 'pipeline_editor' | 'chat_window'

export type AssistantModuleId =
  | 'pipeline_graph_rules'
  | 'node_catalog'
  | 'templating_guide'
  | 'lua_scripting_guide'
  | 'logic_expression_guide'
  | 'llm_tool_edge_rules'

export interface AssistantProfile {
  scope: AssistantProfileScope
  system_instructions: string
  enabled_modules: AssistantModuleId[]
  created_at?: string
  updated_at?: string
}

export type EditorAssistantMode = 'ask' | 'edit'

export interface EditorAssistantMessage {
  role: 'user' | 'assistant'
  content: string
  tool_calls?: LLMToolCall[]
  tool_results?: LLMToolResult[]
}

export interface EditorAssistantSelection {
  selected_node_id?: string
  selected_node_ids?: string[]
}

export interface EditorAssistantPipelineSnapshot {
  name?: string
  description?: string
  status?: Pipeline['status'] | string
  nodes: Node[]
  edges: Edge[]
  viewport?: Viewport
}

export interface EditorAssistantExecutionLogNode {
  node_id: string
  node_type: string
  status: string
  input?: unknown
  output?: unknown
  error?: string
}

export interface EditorAssistantExecutionLogAttachment {
  id: string
  execution: {
    id: string
    trigger_type: string
    status: string
    started_at: string
    completed_at?: string
    error?: string
  }
  nodes: EditorAssistantExecutionLogNode[]
}

export interface EditorAssistantRequest {
  provider_id?: string
  mode: EditorAssistantMode
  message: string
  messages: EditorAssistantMessage[]
  pipeline: EditorAssistantPipelineSnapshot
  selection: EditorAssistantSelection
  attached_log?: EditorAssistantExecutionLogAttachment
}

export type LivePipelineOperationType =
  | 'add_nodes'
  | 'update_nodes'
  | 'delete_nodes'
  | 'add_edges'
  | 'update_edges'
  | 'delete_edges'
  | 'set_viewport'

export interface LivePipelineOperation {
  type: LivePipelineOperationType
  nodes?: Node[]
  edges?: Edge[]
  node_ids?: string[]
  edge_ids?: string[]
  viewport?: Viewport
}

export interface EditorAssistantResponse {
  content: string
  tool_calls?: LLMToolCall[]
  tool_results?: LLMToolResult[]
  usage?: LLMUsage
  operations?: LivePipelineOperation[]
}

export interface LLMChatStreamAssistantDeltaEvent {
  type: 'assistant_delta'
  delta: string
}

export interface LLMChatStreamToolStartedEvent {
  type: 'tool_started'
  tool_call?: LLMToolCall
}

export interface LLMChatStreamToolFinishedEvent {
  type: 'tool_finished'
  tool_call?: LLMToolCall
  tool_result?: LLMToolResult
}

export interface LLMChatStreamUsageEvent {
  type: 'usage'
  usage?: LLMUsage
}

export interface LLMChatStreamDoneEvent {
  type: 'done'
  response: LLMChatResponse
}

export interface LLMChatStreamErrorEvent {
  type: 'error'
  error: string
  status?: number
}

export type LLMChatStreamEvent =
  | LLMChatStreamAssistantDeltaEvent
  | LLMChatStreamToolStartedEvent
  | LLMChatStreamToolFinishedEvent
  | LLMChatStreamUsageEvent
  | LLMChatStreamDoneEvent
  | LLMChatStreamErrorEvent

export interface EditorAssistantStreamAssistantDeltaEvent {
  type: 'assistant_delta'
  delta: string
}

export interface EditorAssistantStreamToolStartedEvent {
  type: 'tool_started'
  tool_call?: LLMToolCall
}

export interface EditorAssistantStreamToolFinishedEvent {
  type: 'tool_finished'
  tool_call?: LLMToolCall
  tool_result?: LLMToolResult
}

export interface EditorAssistantStreamUsageEvent {
  type: 'usage'
  usage?: LLMUsage
}

export interface EditorAssistantStreamDoneEvent {
  type: 'done'
  response: EditorAssistantResponse
}

export interface EditorAssistantStreamErrorEvent {
  type: 'error'
  error: string
  status?: number
}

export type EditorAssistantStreamEvent =
  | EditorAssistantStreamAssistantDeltaEvent
  | EditorAssistantStreamToolStartedEvent
  | EditorAssistantStreamToolFinishedEvent
  | EditorAssistantStreamUsageEvent
  | EditorAssistantStreamDoneEvent
  | EditorAssistantStreamErrorEvent

export interface LLMConversationMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  tool_calls?: LLMToolCall[]
  tool_results?: LLMToolResult[]
  usage?: LLMUsage
  created_at: string
}

export interface LLMConversationSummary {
  id: string
  title: string
  provider_id?: string
  proxmox_enabled: boolean
  proxmox_cluster_id?: string
  kubernetes_enabled: boolean
  kubernetes_cluster_id?: string
  compaction_count: number
  compacted_at?: string
  context_window: number
  context_token_count: number
  last_prompt_tokens: number
  last_completion_tokens: number
  last_total_tokens: number
  last_message_at: string
  created_at: string
  updated_at: string
}

export interface LLMConversation extends LLMConversationSummary {
  messages: LLMConversationMessage[]
}

export interface TemplateSuggestion {
  template: string
  expression: string
  label: string
  description?: string
  kind?: 'template' | 'sample'
  preview?: string
  badge?: string
}

export type NodeType = string

export interface NodeDefinitionFieldOption {
  value: string
  label: string
}

export interface NodeDefinitionField {
  name: string
  label: string
  description?: string
  type: 'string' | 'textarea' | 'number' | 'boolean' | 'select' | 'json' | string
  required?: boolean
  placeholder?: string
  template_supported?: boolean
  options?: NodeDefinitionFieldOption[]
  default_string_value?: string
  default_bool_value?: boolean
  default_number_value?: number
}

export interface NodeDefinitionOutputHandle {
  id: string
  label: string
  color?: string
}

export interface NodeDefinitionOutputHint {
  expression: string
  label: string
  description?: string
}

export interface PluginBundleStatus {
  id: string
  name: string
  version?: string
  description?: string
  path: string
  healthy: boolean
  error?: string
  node_count: number
}

export interface NodeDefinition {
  type: NodeType
  category: string
  source: 'builtin' | 'plugin'
  plugin_id?: string
  plugin_name?: string
  label: string
  description: string
  icon: string
  color: string
  menu_path?: string[]
  default_config: Record<string, unknown>
  fields?: NodeDefinitionField[]
  outputs?: NodeDefinitionOutputHandle[]
  output_hints?: NodeDefinitionOutputHint[]
}

export interface NodeDefinitionsResponse {
  definitions: NodeDefinition[]
  plugins?: PluginBundleStatus[]
  error?: string
}

export interface SecretMetadata {
  id: string
  name: string
  created_at: string
  updated_at: string
}

export interface NodeTypeDefinition {
  type: NodeType
  source?: 'builtin' | 'plugin'
  pluginId?: string
  pluginName?: string
  label: string
  description: string
  icon: string
  category: string
  color: string
  menuPath?: string[]
  defaultConfig: Record<string, unknown>
  fields?: NodeDefinitionField[]
  outputs?: NodeDefinitionOutputHandle[]
  outputHints?: NodeDefinitionOutputHint[]
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
