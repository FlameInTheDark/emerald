import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import LLMChat from './LLMChat'

const { mockApi } = vi.hoisted(() => ({
  mockApi: {
    llmProviders: {
      list: vi.fn(),
    },
    clusters: {
      list: vi.fn(),
    },
    kubernetesClusters: {
      list: vi.fn(),
    },
    llm: {
      chat: vi.fn(),
      chatStream: vi.fn(),
      conversations: {
        list: vi.fn(),
        get: vi.fn(),
        update: vi.fn(),
        delete: vi.fn(),
      },
    },
  },
}))

const queryClients: QueryClient[] = []

vi.mock('../api/client', () => ({
  api: mockApi,
}))

describe('LLMChat page', () => {
  afterEach(() => {
    for (const queryClient of queryClients) {
      queryClient.clear()
    }
    queryClients.length = 0
    cleanup()
  })

  beforeEach(() => {
    vi.clearAllMocks()

    mockApi.llmProviders.list.mockResolvedValue([
      {
        id: 'provider-1',
        name: 'Default Provider',
        provider_type: 'custom',
        model: 'test-model',
        is_default: true,
        created_at: '2026-04-08T10:00:00Z',
        updated_at: '2026-04-08T10:00:00Z',
      },
    ])
    mockApi.clusters.list.mockResolvedValue([
      {
        id: 'cluster-1',
        name: 'Primary Proxmox',
        host: 'localhost',
        port: 8006,
        api_token_id: 'token',
        skip_tls_verify: true,
        created_at: '2026-04-08T10:00:00Z',
        updated_at: '2026-04-08T10:00:00Z',
      },
    ])
    mockApi.kubernetesClusters.list.mockResolvedValue([
      {
        id: 'k8s-1',
        name: 'Primary Kubernetes',
        source_type: 'manual',
        context_name: 'default',
        default_namespace: 'default',
        server: 'https://localhost',
        created_at: '2026-04-08T10:00:00Z',
        updated_at: '2026-04-08T10:00:00Z',
      },
    ])
    mockApi.llm.conversations.list.mockResolvedValue([])
    mockApi.llm.conversations.get.mockResolvedValue({
      id: 'conv-1',
      title: 'Existing conversation',
      provider_id: 'provider-1',
      proxmox_enabled: true,
      proxmox_cluster_id: 'cluster-1',
      kubernetes_enabled: false,
      kubernetes_cluster_id: undefined,
      compaction_count: 0,
      compacted_at: undefined,
      context_window: 128000,
      context_token_count: 1024,
      last_prompt_tokens: 10,
      last_completion_tokens: 5,
      last_total_tokens: 15,
      last_message_at: '2026-04-08T10:05:00Z',
      created_at: '2026-04-08T10:00:00Z',
      updated_at: '2026-04-08T10:05:00Z',
      messages: [
        {
          id: 'msg-1',
          role: 'assistant',
          content: 'Saved answer',
          created_at: '2026-04-08T10:05:00Z',
        },
      ],
    })
    const defaultChatResponse = {
      conversation_id: 'conv-1',
      conversation: {
        id: 'conv-1',
        title: 'Existing conversation',
        provider_id: 'provider-1',
        proxmox_enabled: true,
        proxmox_cluster_id: 'cluster-1',
        kubernetes_enabled: false,
        kubernetes_cluster_id: undefined,
        compaction_count: 0,
        compacted_at: undefined,
        context_window: 128000,
        context_token_count: 1200,
        last_prompt_tokens: 10,
        last_completion_tokens: 5,
        last_total_tokens: 15,
        last_message_at: '2026-04-08T10:06:00Z',
        created_at: '2026-04-08T10:00:00Z',
        updated_at: '2026-04-08T10:06:00Z',
      },
      content: 'Follow-up answer',
      tool_calls: [],
      tool_results: [],
      usage: {
        prompt_tokens: 10,
        completion_tokens: 5,
        total_tokens: 15,
      },
    }
    mockApi.llm.chat.mockResolvedValue(defaultChatResponse)
    mockApi.llm.chatStream.mockImplementation(async (_data: unknown, handlers?: { onEvent?: (event: any) => void }) => {
      handlers?.onEvent?.({ type: 'assistant_delta', delta: defaultChatResponse.content })
      handlers?.onEvent?.({ type: 'usage', usage: defaultChatResponse.usage })
      return defaultChatResponse
    })
    mockApi.llm.conversations.update.mockResolvedValue({
      id: 'conv-1',
      title: 'Existing conversation',
      provider_id: 'provider-1',
      proxmox_enabled: false,
      proxmox_cluster_id: undefined,
      kubernetes_enabled: true,
      kubernetes_cluster_id: 'k8s-1',
      compaction_count: 0,
      compacted_at: undefined,
      context_window: 128000,
      context_token_count: 1024,
      last_prompt_tokens: 10,
      last_completion_tokens: 5,
      last_total_tokens: 15,
      last_message_at: '2026-04-08T10:06:00Z',
      created_at: '2026-04-08T10:00:00Z',
      updated_at: '2026-04-08T10:06:00Z',
    })
  })

  it('shows the blank chat state on /chat', async () => {
    renderChat('/chat')

    expect(await screen.findByText('New conversation')).toBeInTheDocument()
    expect(screen.getByText('Pick a starter or start typing below.')).toBeInTheDocument()
    expect(screen.getByPlaceholderText(/Message Automator/i)).toBeInTheDocument()
    expect(screen.getByText('Your first sent message will create one here.')).toBeInTheDocument()
  })

  it('keeps Shift+Enter as a newline in the composer', async () => {
    const user = userEvent.setup()
    renderChat('/chat')

    const composer = await screen.findByPlaceholderText(/Message Automator/i)
    await waitFor(() => expect(composer).toBeEnabled())
    await user.type(composer, 'Hello{shift>}{enter}{/shift}there')

    expect(mockApi.llm.chatStream).not.toHaveBeenCalled()
    expect(composer).toHaveValue('Hello\nthere')
  })

  it('creates a conversation on first send and navigates to it', async () => {
    const user = userEvent.setup()
    const firstResponse = {
      conversation_id: 'conv-2',
      conversation: {
        id: 'conv-2',
        title: 'Hello world',
        provider_id: 'provider-1',
        proxmox_enabled: true,
        proxmox_cluster_id: 'cluster-1',
        kubernetes_enabled: false,
        kubernetes_cluster_id: undefined,
        compaction_count: 0,
        compacted_at: undefined,
        context_window: 128000,
        context_token_count: 900,
        last_prompt_tokens: 8,
        last_completion_tokens: 4,
        last_total_tokens: 12,
        last_message_at: '2026-04-08T10:01:00Z',
        created_at: '2026-04-08T10:01:00Z',
        updated_at: '2026-04-08T10:01:00Z',
      },
      content: 'First answer',
      tool_calls: [],
      tool_results: [],
      usage: {
        prompt_tokens: 8,
        completion_tokens: 4,
        total_tokens: 12,
      },
    }
    mockApi.llm.chat.mockResolvedValueOnce(firstResponse)
    mockApi.llm.chatStream.mockImplementationOnce(async (_data: unknown, handlers?: { onEvent?: (event: any) => void }) => {
      handlers?.onEvent?.({ type: 'assistant_delta', delta: 'First ' })
      handlers?.onEvent?.({ type: 'assistant_delta', delta: 'answer' })
      handlers?.onEvent?.({ type: 'usage', usage: firstResponse.usage })
      return firstResponse
    })
    mockApi.llm.conversations.get.mockResolvedValueOnce({
      id: 'conv-2',
      title: 'Hello world',
      provider_id: 'provider-1',
      proxmox_enabled: true,
      proxmox_cluster_id: 'cluster-1',
      kubernetes_enabled: false,
      kubernetes_cluster_id: undefined,
      compaction_count: 0,
      compacted_at: undefined,
      context_window: 128000,
      context_token_count: 900,
      last_prompt_tokens: 8,
      last_completion_tokens: 4,
      last_total_tokens: 12,
      last_message_at: '2026-04-08T10:01:00Z',
      created_at: '2026-04-08T10:01:00Z',
      updated_at: '2026-04-08T10:01:00Z',
      messages: [
        { id: 'user-1', role: 'user', content: 'Hello world', created_at: '2026-04-08T10:01:00Z' },
        { id: 'assistant-1', role: 'assistant', content: 'First answer', created_at: '2026-04-08T10:01:01Z' },
      ],
    })

    renderChat('/chat')

    const composer = await screen.findByPlaceholderText(/Message Automator/i)
    await waitFor(() => expect(composer).toBeEnabled())
    await user.type(composer, 'Hello world{enter}')

    await waitFor(() => expect(mockApi.llm.chatStream).toHaveBeenCalled())
    expect(mockApi.llm.chatStream.mock.calls[0][0]).toMatchObject({
      message: 'Hello world',
      provider_id: 'provider-1',
    })
    expect(mockApi.llm.chatStream.mock.calls[0][0].conversation_id).toBeUndefined()

    await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/chat/conv-2'))
    expect(await screen.findByText('First answer')).toBeInTheDocument()
  })

  it('shows streamed assistant text while waiting for completion', async () => {
    const user = userEvent.setup()
    let resolveStream: ((value: any) => void) | null = null

    mockApi.llm.chatStream.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveStream = resolve
        }),
    )

    renderChat('/chat')

    const composer = await screen.findByPlaceholderText(/Message Automator/i)
    await waitFor(() => expect(composer).toBeEnabled())
    await user.type(composer, 'Stream this{enter}')

    const streamHandler = mockApi.llm.chatStream.mock.calls[0][1]?.onEvent
    await act(async () => {
      streamHandler?.({ type: 'assistant_delta', delta: 'Streaming' })
      streamHandler?.({ type: 'assistant_delta', delta: ' now' })
    })

    expect(await screen.findByText('Streaming now')).toBeInTheDocument()

    await act(async () => {
      resolveStream?.({
        conversation_id: 'conv-1',
        conversation: {
          id: 'conv-1',
          title: 'Existing conversation',
          provider_id: 'provider-1',
          proxmox_enabled: true,
          proxmox_cluster_id: 'cluster-1',
          kubernetes_enabled: false,
          kubernetes_cluster_id: undefined,
          compaction_count: 0,
          compacted_at: undefined,
          context_window: 128000,
          context_token_count: 1100,
          last_prompt_tokens: 10,
          last_completion_tokens: 5,
          last_total_tokens: 15,
          last_message_at: '2026-04-08T10:06:00Z',
          created_at: '2026-04-08T10:00:00Z',
          updated_at: '2026-04-08T10:06:00Z',
        },
        content: 'Streaming now',
        tool_calls: [],
        tool_results: [],
        usage: {
          prompt_tokens: 10,
          completion_tokens: 5,
          total_tokens: 15,
        },
      })
      await Promise.resolve()
    })

    await waitFor(() => expect(screen.getAllByText('Streaming now').length).toBeGreaterThan(0))
  })

  it('shows a temporary rate-limit system notice and clears it on the next send', async () => {
    const user = userEvent.setup()

    mockApi.llm.chatStream
      .mockRejectedValueOnce(Object.assign(new Error('LLM request failed: API error (status 429): Too Many Requests'), { status: 429 }))
      .mockResolvedValueOnce({
        conversation_id: 'conv-2',
        conversation: {
          id: 'conv-2',
          title: 'Retry message',
          provider_id: 'provider-1',
          proxmox_enabled: true,
          proxmox_cluster_id: 'cluster-1',
          kubernetes_enabled: false,
          kubernetes_cluster_id: undefined,
          compaction_count: 0,
          compacted_at: undefined,
          context_window: 128000,
          context_token_count: 950,
          last_prompt_tokens: 9,
          last_completion_tokens: 4,
          last_total_tokens: 13,
          last_message_at: '2026-04-08T10:02:00Z',
          created_at: '2026-04-08T10:02:00Z',
          updated_at: '2026-04-08T10:02:00Z',
        },
        content: 'Recovered answer',
        tool_calls: [],
        tool_results: [],
        usage: {
          prompt_tokens: 9,
          completion_tokens: 4,
          total_tokens: 13,
        },
      })

    mockApi.llm.conversations.get.mockResolvedValueOnce({
      id: 'conv-2',
      title: 'Retry message',
      provider_id: 'provider-1',
      proxmox_enabled: true,
      proxmox_cluster_id: 'cluster-1',
      kubernetes_enabled: false,
      kubernetes_cluster_id: undefined,
      compaction_count: 0,
      compacted_at: undefined,
      context_window: 128000,
      context_token_count: 950,
      last_prompt_tokens: 9,
      last_completion_tokens: 4,
      last_total_tokens: 13,
      last_message_at: '2026-04-08T10:02:00Z',
      created_at: '2026-04-08T10:02:00Z',
      updated_at: '2026-04-08T10:02:00Z',
      messages: [
        { id: 'user-2', role: 'user', content: 'Retry message', created_at: '2026-04-08T10:02:00Z' },
        { id: 'assistant-2', role: 'assistant', content: 'Recovered answer', created_at: '2026-04-08T10:02:01Z' },
      ],
    })

    renderChat('/chat')

    const composer = await screen.findByPlaceholderText(/Message Automator/i)
    await waitFor(() => expect(composer).toBeEnabled())

    await user.type(composer, 'Rate limit me{enter}')

    expect(await screen.findByText('Rate limited')).toBeInTheDocument()
    expect(screen.getByText(/model provider is rate limiting/i)).toBeInTheDocument()
    expect(composer).toHaveValue('Rate limit me')

    await user.clear(composer)
    await user.type(composer, 'Retry message{enter}')

    await waitFor(() => expect(screen.queryByText('Rate limited')).not.toBeInTheDocument())
    await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/chat/conv-2'))
    expect(await screen.findByText('Recovered answer')).toBeInTheDocument()
  })

  it('loads an existing conversation and saves settings from the drawer', async () => {
    const user = userEvent.setup()
    mockApi.llm.conversations.list.mockResolvedValue([
      {
        id: 'conv-1',
        title: 'Existing conversation',
        provider_id: 'provider-1',
        proxmox_enabled: true,
        proxmox_cluster_id: 'cluster-1',
        kubernetes_enabled: false,
        kubernetes_cluster_id: undefined,
        compaction_count: 0,
        compacted_at: undefined,
        context_window: 128000,
        context_token_count: 1024,
        last_prompt_tokens: 10,
        last_completion_tokens: 5,
        last_total_tokens: 15,
        last_message_at: '2026-04-08T10:05:00Z',
        created_at: '2026-04-08T10:00:00Z',
        updated_at: '2026-04-08T10:05:00Z',
      },
    ])

    renderChat('/chat/conv-1')

    expect(await screen.findByText('Saved answer')).toBeInTheDocument()
    expect(mockApi.llm.conversations.get).toHaveBeenCalledWith('conv-1')

    await user.click(screen.getByRole('button', { name: /Open advanced settings/i }))
    const settingsDialog = await screen.findByRole('dialog')
    await user.click(within(settingsDialog).getByRole('button', { name: /Kubernetes integration toggle/i }))
    await user.selectOptions(within(settingsDialog).getByRole('combobox', { name: /Kubernetes cluster/i }), 'k8s-1')
    await user.click(within(settingsDialog).getByRole('button', { name: /Save Settings/i }))

    await waitFor(() => expect(mockApi.llm.conversations.update).toHaveBeenCalled())
    expect(mockApi.llm.conversations.update).toHaveBeenCalledWith('conv-1', {
      provider_id: 'provider-1',
      integrations: {
        proxmox: {
          enabled: true,
          cluster_id: 'cluster-1',
        },
        kubernetes: {
          enabled: true,
          cluster_id: 'k8s-1',
        },
      },
    })
  })

  it('deletes a conversation from the rail', async () => {
    const user = userEvent.setup()
    mockApi.llm.conversations.list.mockResolvedValue([
      {
        id: 'conv-1',
        title: 'Existing conversation',
        provider_id: 'provider-1',
        proxmox_enabled: true,
        proxmox_cluster_id: 'cluster-1',
        kubernetes_enabled: false,
        kubernetes_cluster_id: undefined,
        compaction_count: 0,
        compacted_at: undefined,
        context_window: 128000,
        context_token_count: 1024,
        last_prompt_tokens: 10,
        last_completion_tokens: 5,
        last_total_tokens: 15,
        last_message_at: '2026-04-08T10:05:00Z',
        created_at: '2026-04-08T10:00:00Z',
        updated_at: '2026-04-08T10:05:00Z',
      },
    ])
    mockApi.llm.conversations.delete.mockResolvedValue(undefined)

    renderChat('/chat/conv-1')

    expect(await screen.findByText('Saved answer')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /Delete conversation Existing conversation/i }))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /^Delete$/i }))

    await waitFor(() => expect(mockApi.llm.conversations.delete).toHaveBeenCalledWith('conv-1'))
    await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/chat'))
  })
})

function renderChat(initialEntry: string) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: Infinity,
      },
    },
  })
  queryClients.push(queryClient)

  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          <Route path="/chat" element={<LLMChat />} />
          <Route path="/chat/:conversationId" element={<LLMChat />} />
        </Routes>
        <LocationDisplay />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function LocationDisplay() {
  const location = useLocation()
  return <div data-testid="location">{location.pathname}</div>
}
