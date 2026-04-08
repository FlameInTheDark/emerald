import type {
  ActiveExecution,
  AuthSession,
  Channel,
  ChannelContact,
  Cluster,
  DashboardStats,
  Execution,
  ExecutionDetail,
  LLMChatResponse,
  LLMModelInfo,
  LLMProvider,
  KubernetesCluster,
  KubernetesTestConnectionResult,
  Pipeline,
  PipelineDocument,
  PipelineRunResponse,
  TemplateBundle,
  TemplateDetail,
  TemplateDocument,
  TemplateImportResult,
  TemplateSummary,
  User,
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
    get: (id: string) => request<LLMProvider>(`/llm-providers/${id}`),
    update: (id: string, data: unknown) => request<LLMProvider>(`/llm-providers/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
    delete: (id: string) => request<void>(`/llm-providers/${id}`, { method: 'DELETE' }),
    models: (id: string) => request<LLMModelInfo[]>(`/llm-providers/${id}/models`),
  },
  executions: {
    listByPipeline: (pipelineId: string) => request<Execution[]>(`/executions/pipelines/${pipelineId}`),
    activeByPipeline: (pipelineId: string) => request<ActiveExecution[]>(`/executions/pipelines/${pipelineId}/active`),
    get: (executionId: string) => request<ExecutionDetail>(`/executions/${executionId}`),
    cancel: (executionId: string) => request<ActiveExecution>(`/executions/${executionId}/cancel`, { method: 'POST' }),
  },
  llm: {
    chat: (data: unknown) => request<LLMChatResponse>('/llm/chat', { method: 'POST', body: JSON.stringify(data) }),
  },
}
