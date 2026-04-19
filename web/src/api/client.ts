import type {
  ActiveExecution,
  AssistantProfile,
  AuthSession,
  Channel,
  ChannelContact,
  Cluster,
  DashboardStats,
  EditorAssistantRequest,
  EditorAssistantResponse,
  EditorAssistantStreamEvent,
  Execution,
  ExecutionDetail,
  LLMChatResponse,
  LLMChatStreamEvent,
  LLMConversation,
  LLMConversationSummary,
  LLMModelInfo,
  LLMProvider,
  KubernetesCluster,
  KubernetesTestConnectionResult,
  NodeDefinitionsResponse,
  Pipeline,
  PipelineDocument,
  PipelineRunResponse,
  SecretMetadata,
  TemplateBundle,
  TemplateDetail,
  TemplateDocument,
  TemplateImportResult,
  TemplateSummary,
  User,
  WebToolsConfig,
} from '../types'

const API_BASE = '/api/v1'

export class APIError extends Error {
  status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = 'APIError'
    this.status = status
  }
}

async function request<T>(endpoint: string, options?: RequestInit): Promise<T> {
  const headers = new Headers(options?.headers)
  if (options?.body && !(options.body instanceof FormData) && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const res = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    credentials: 'include',
    headers,
  })

  if (!res.ok) {
    const error = await res.json().catch(() => ({ error: res.statusText }))
    throw new APIError(error.error || `Request failed: ${res.status}`, res.status)
  }

  if (res.status === 204) return undefined as unknown as T
  return res.json() as Promise<T>
}

async function streamRequest<TEvent extends { type: string }>(
  endpoint: string,
  options: RequestInit | undefined,
  onEvent: (event: TEvent) => void,
): Promise<void> {
  const headers = new Headers(options?.headers)
  if (options?.body && !(options.body instanceof FormData) && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  headers.set('Accept', 'text/event-stream')

  const res = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    credentials: 'include',
    headers,
  })

  if (!res.ok) {
    const error = await res.json().catch(() => ({ error: res.statusText }))
    throw new APIError(error.error || `Request failed: ${res.status}`, res.status)
  }

  if (!res.body) {
    throw new APIError('Streaming is not available in this environment.', res.status)
  }

  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  try {
    while (true) {
      const { value, done } = await reader.read()
      if (done) {
        break
      }

      buffer += decoder.decode(value, { stream: true })
      buffer = dispatchSSEChunks(buffer, onEvent)
    }

    buffer += decoder.decode()
    dispatchSSEChunks(buffer, onEvent)
  } finally {
    reader.releaseLock()
  }
}

function dispatchSSEChunks<TEvent extends { type: string }>(
  buffer: string,
  onEvent: (event: TEvent) => void,
): string {
  let nextBuffer = buffer
  while (true) {
    const match = nextBuffer.match(/\r?\n\r?\n/)
    if (!match || match.index === undefined) {
      break
    }

    const separatorIndex = match.index
    const separatorLength = match[0].length
    const rawEvent = nextBuffer.slice(0, separatorIndex)
    nextBuffer = nextBuffer.slice(separatorIndex + separatorLength)

    const parsed = parseSSEEvent<TEvent>(rawEvent)
    if (parsed) {
      onEvent(parsed)
    }
  }

  return nextBuffer
}

function parseSSEEvent<TEvent extends { type: string }>(rawEvent: string): TEvent | null {
  const dataLines = rawEvent
    .split(/\r?\n/)
    .filter((line) => line.startsWith('data:'))
    .map((line) => line.slice(5).trim())

  if (dataLines.length === 0) {
    return null
  }

  return JSON.parse(dataLines.join('\n')) as TEvent
}

export const api = {
  auth: {
    session: () => request<AuthSession>('/auth/session'),
    login: (data: { username: string; password: string }) => request<AuthSession>('/auth/login', { method: 'POST', body: JSON.stringify(data) }),
    logout: () => request<void>('/auth/logout', { method: 'POST' }),
  },
  users: {
    list: () => request<User[]>('/users'),
    create: (data: { username: string; password: string }) => request<User>('/users', { method: 'POST', body: JSON.stringify(data) }),
    changePassword: (data: { current_password: string; new_password: string }) => request<AuthSession>('/users/change-password', { method: 'POST', body: JSON.stringify(data) }),
    delete: (id: string) => request<void>(`/users/${id}`, { method: 'DELETE' }),
  },
  clusters: {
    list: () => request<Cluster[]>('/clusters'),
    create: (data: unknown) => request<Cluster>('/clusters', { method: 'POST', body: JSON.stringify(data) }),
    get: (id: string) => request<Cluster>(`/clusters/${id}`),
    update: (id: string, data: unknown) => request<Cluster>(`/clusters/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
    delete: (id: string) => request<void>(`/clusters/${id}`, { method: 'DELETE' }),
  },
  kubernetesClusters: {
    list: () => request<KubernetesCluster[]>('/kubernetes/clusters'),
    create: (data: unknown) => request<KubernetesCluster>('/kubernetes/clusters', { method: 'POST', body: JSON.stringify(data) }),
    get: (id: string) => request<KubernetesCluster>(`/kubernetes/clusters/${id}`),
    update: (id: string, data: unknown) => request<KubernetesCluster>(`/kubernetes/clusters/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
    delete: (id: string) => request<void>(`/kubernetes/clusters/${id}`, { method: 'DELETE' }),
    test: (data: unknown) => request<KubernetesTestConnectionResult>('/kubernetes/clusters/test', { method: 'POST', body: JSON.stringify(data) }),
  },
  dashboard: {
    stats: () => request<DashboardStats>('/dashboard/stats'),
  },
  nodeDefinitions: {
    list: () => request<NodeDefinitionsResponse>('/node-definitions'),
    refresh: () => request<NodeDefinitionsResponse>('/node-definitions/refresh', { method: 'POST' }),
  },
  channels: {
    list: () => request<Channel[]>('/channels'),
    create: (data: unknown) => request<Channel>('/channels', { method: 'POST', body: JSON.stringify(data) }),
    get: (id: string) => request<Channel>(`/channels/${id}`),
    update: (id: string, data: unknown) => request<Channel>(`/channels/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
    delete: (id: string) => request<void>(`/channels/${id}`, { method: 'DELETE' }),
    contacts: (id: string) => request<ChannelContact[]>(`/channels/${id}/contacts`),
    connect: (data: unknown) => request<ChannelContact>('/channels/connect', { method: 'POST', body: JSON.stringify(data) }),
  },
  pipelines: {
    list: () => request<Pipeline[]>('/pipelines'),
    create: (data: unknown) => request<Pipeline>('/pipelines', { method: 'POST', body: JSON.stringify(data) }),
    get: (id: string) => request<Pipeline>(`/pipelines/${id}`),
    update: (id: string, data: unknown) => request<Pipeline>(`/pipelines/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
    delete: (id: string) => request<void>(`/pipelines/${id}`, { method: 'DELETE' }),
    export: (id: string) => request<PipelineDocument>(`/pipelines/${id}/export`),
    run: (id: string) => request<PipelineRunResponse>(`/pipelines/${id}/run`, { method: 'POST' }),
  },
  templates: {
    list: () => request<TemplateSummary[]>('/templates'),
    create: (data: unknown) => request<TemplateDetail>('/templates', { method: 'POST', body: JSON.stringify(data) }),
    get: (id: string) => request<TemplateDetail>(`/templates/${id}`),
    delete: (id: string) => request<void>(`/templates/${id}`, { method: 'DELETE' }),
    clone: (id: string) => request<TemplateDetail>(`/templates/${id}/clone`, { method: 'POST' }),
    createPipeline: (id: string) => request<Pipeline>(`/templates/${id}/pipelines`, { method: 'POST' }),
    export: (id: string) => request<TemplateDocument>(`/templates/${id}/export`),
    exportAll: () => request<TemplateBundle>('/templates/export'),
    import: (raw: string) => request<TemplateImportResult>('/templates/import', { method: 'POST', body: raw }),
  },
  llmProviders: {
    list: () => request<LLMProvider[]>('/llm-providers'),
    create: (data: unknown) => request<LLMProvider>('/llm-providers', { method: 'POST', body: JSON.stringify(data) }),
    discoverModels: (data: unknown) => request<LLMModelInfo[]>('/llm-providers/discover-models', { method: 'POST', body: JSON.stringify(data) }),
    get: (id: string) => request<LLMProvider>(`/llm-providers/${id}`),
    update: (id: string, data: unknown) => request<LLMProvider>(`/llm-providers/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
    delete: (id: string) => request<void>(`/llm-providers/${id}`, { method: 'DELETE' }),
    models: (id: string) => request<LLMModelInfo[]>(`/llm-providers/${id}/models`),
  },
  webTools: {
    getConfig: () => request<WebToolsConfig>('/web-tools/config'),
    updateConfig: (data: unknown) => request<WebToolsConfig>('/web-tools/config', { method: 'PUT', body: JSON.stringify(data) }),
  },
  executions: {
    listByPipeline: (pipelineId: string) => request<Execution[]>(`/executions/pipelines/${pipelineId}`),
    activeByPipeline: (pipelineId: string) => request<ActiveExecution[]>(`/executions/pipelines/${pipelineId}/active`),
    get: (executionId: string) => request<ExecutionDetail>(`/executions/${executionId}`),
    cancel: (executionId: string) => request<ActiveExecution>(`/executions/${executionId}/cancel`, { method: 'POST' }),
  },
  secrets: {
    list: () => request<SecretMetadata[]>('/secrets'),
    create: (data: { name: string; value: string }) => request<SecretMetadata>('/secrets', { method: 'POST', body: JSON.stringify(data) }),
    get: (id: string) => request<SecretMetadata>(`/secrets/${id}`),
    update: (id: string, data: { name?: string; value?: string }) => request<SecretMetadata>(`/secrets/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
    delete: (id: string) => request<void>(`/secrets/${id}`, { method: 'DELETE' }),
  },
  llm: {
    chat: (data: unknown) => request<LLMChatResponse>('/llm/chat', { method: 'POST', body: JSON.stringify(data) }),
    chatStream: async (
      data: unknown,
      handlers?: {
        onEvent?: (event: Exclude<LLMChatStreamEvent, { type: 'done' }>) => void
        signal?: AbortSignal
      },
    ) => {
      let finalResponse: LLMChatResponse | null = null

      await streamRequest<LLMChatStreamEvent>('/llm/chat/stream', {
        method: 'POST',
        body: JSON.stringify(data),
        signal: handlers?.signal,
      }, (event) => {
        if (event.type === 'done') {
          finalResponse = event.response
          return
        }

        if (event.type === 'error') {
          throw new APIError(event.error || 'Streaming request failed', event.status ?? 500)
        }

        handlers?.onEvent?.(event)
      })

      if (!finalResponse) {
        throw new APIError('Stream ended before the response completed.', 500)
      }

      return finalResponse
    },
    editorAssistantStream: async (
      data: EditorAssistantRequest,
      handlers?: {
        onEvent?: (event: Exclude<EditorAssistantStreamEvent, { type: 'done' }>) => void
        signal?: AbortSignal
      },
    ) => {
      let finalResponse: EditorAssistantResponse | null = null

      await streamRequest<EditorAssistantStreamEvent>('/llm/editor-assistant/stream', {
        method: 'POST',
        body: JSON.stringify(data),
        signal: handlers?.signal,
      }, (event) => {
        if (event.type === 'done') {
          finalResponse = event.response
          return
        }

        if (event.type === 'error') {
          throw new APIError(event.error || 'Streaming request failed', event.status ?? 500)
        }

        handlers?.onEvent?.(event)
      })

      if (!finalResponse) {
        throw new APIError('Stream ended before the response completed.', 500)
      }

      return finalResponse
    },
    conversations: {
      list: () => request<LLMConversationSummary[]>('/llm/conversations'),
      get: (id: string) => request<LLMConversation>(`/llm/conversations/${id}`),
      update: (id: string, data: unknown) => request<LLMConversationSummary>(`/llm/conversations/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
      delete: (id: string) => request<void>(`/llm/conversations/${id}`, { method: 'DELETE' }),
    },
  },
  assistantProfiles: {
    get: (scope: 'pipeline_editor' | 'chat_window') => request<AssistantProfile>(`/assistant-profiles/${scope}`),
    update: (scope: 'pipeline_editor' | 'chat_window', data: Pick<AssistantProfile, 'system_instructions'> & Partial<Pick<AssistantProfile, 'enabled_modules'>>) =>
      request<AssistantProfile>(`/assistant-profiles/${scope}`, { method: 'PUT', body: JSON.stringify(data) }),
    restoreDefaults: (scope: 'pipeline_editor' | 'chat_window') =>
      request<AssistantProfile>(`/assistant-profiles/${scope}/restore-defaults`, { method: 'POST' }),
  },
}
