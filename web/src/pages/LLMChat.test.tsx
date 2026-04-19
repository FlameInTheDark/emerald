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
    expect(screen.getByPlaceholderText(/Message Emerald/i)).toBeInTheDocument()
    expect(screen.getByText('Your first sent message will create one here.')).toBeInTheDocument()
  })

  it('keeps Shift+Enter as a newline in the composer', async () => {
    const user = userEvent.setup()
    renderChat('/chat')

    const composer = await screen.findByPlaceholderText(/Message Emerald/i)
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

    const composer = await screen.findByPlaceholderText(/Message Emerald/i)
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

    const composer = await screen.findByPlaceholderText(/Message Emerald/i)
    await waitFor(() => expect(composer).toBeEnabled())
    await user.type(composer, 'Stream this{enter}')

    const streamHandler = mockApi.llm.chatStream.mock.calls[0][1]?.onEvent
    await act(async () => {
      streamHandler?.({ type: 'assistant_delta', delta: 'Streaming' })
      streamHandler?.({ type: 'assistant_delta', delta: ' now' })
    })

    expect(await screen.findByText(/Streaming now/)).toBeInTheDocument()

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

    await waitFor(() => expect(screen.getAllByText(/Streaming now/).length).toBeGreaterThan(0))
  })

  it('keeps streamed progress after stopping once the turn has started', async () => {
    const user = userEvent.setup()

    mockApi.llm.conversations.get.mockImplementation(async (id: string) => {
      if (id === 'conv-partial') {
        return {
          id: 'conv-partial',
          title: 'Keep partial work',
          provider_id: 'provider-1',
          proxmox_enabled: true,
          proxmox_cluster_id: 'cluster-1',
          kubernetes_enabled: false,
          kubernetes_cluster_id: undefined,
          compaction_count: 0,
          compacted_at: undefined,
          context_window: 128000,
          context_token_count: 1000,
          last_prompt_tokens: 0,
          last_completion_tokens: 0,
          last_total_tokens: 0,
          last_message_at: '2026-04-08T10:06:00Z',
          created_at: '2026-04-08T10:06:00Z',
          updated_at: '2026-04-08T10:06:00Z',
          messages: [
            {
              id: 'user-partial',
              role: 'user',
              content: 'Keep partial work',
              created_at: '2026-04-08T10:06:00Z',
            },
            {
              id: 'assistant-partial',
              role: 'assistant',
              content: 'Partial response',
              created_at: '2026-04-08T10:06:01Z',
            },
          ],
        }
      }

      return {
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
      }
    })

    mockApi.llm.chatStream.mockImplementationOnce(
      async (_data: unknown, handlers?: { onEvent?: (event: any) => void; signal?: AbortSignal }) =>
        new Promise((_, reject) => {
          handlers?.signal?.addEventListener('abort', () => {
            reject(new DOMException('Aborted', 'AbortError'))
          })
        }),
    )

    renderChat('/chat')

    const composer = await screen.findByPlaceholderText(/Message Emerald/i)
    await waitFor(() => expect(composer).toBeEnabled())
    await user.type(composer, 'Keep partial work{enter}')

    const streamHandler = mockApi.llm.chatStream.mock.calls[0][1]?.onEvent
    await act(async () => {
      streamHandler?.({
        type: 'turn_started',
        turn: {
          conversation_id: 'conv-partial',
          conversation: {
            id: 'conv-partial',
            title: 'Keep partial work',
            provider_id: 'provider-1',
            proxmox_enabled: true,
            proxmox_cluster_id: 'cluster-1',
            kubernetes_enabled: false,
            kubernetes_cluster_id: undefined,
            compaction_count: 0,
            compacted_at: undefined,
            context_window: 128000,
            context_token_count: 1000,
            last_prompt_tokens: 0,
            last_completion_tokens: 0,
            last_total_tokens: 0,
            last_message_at: '2026-04-08T10:06:00Z',
            created_at: '2026-04-08T10:06:00Z',
            updated_at: '2026-04-08T10:06:00Z',
          },
          user_message: {
            id: 'user-partial',
            role: 'user',
            content: 'Keep partial work',
            created_at: '2026-04-08T10:06:00Z',
          },
          assistant_message: {
            id: 'assistant-partial',
            role: 'assistant',
            content: '',
            created_at: '2026-04-08T10:06:01Z',
          },
        },
      })
      streamHandler?.({ type: 'assistant_delta', delta: 'Partial response' })
    })

    expect(await screen.findByText(/Partial response/)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /Stop response/i }))

    await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/chat/conv-partial'))
    expect(await screen.findByText(/Partial response/)).toBeInTheDocument()
    expect(screen.getByText('Keep partial work')).toBeInTheDocument()
  })

  it('queues a follow-up while streaming and sends it as the next turn', async () => {
    const user = userEvent.setup()
    let firstRequestResolved = false

    mockApi.llm.conversations.get.mockImplementation(async (id: string) => {
      if (id === 'conv-queued') {
        return {
          id: 'conv-queued',
          title: 'Initial task',
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
          last_message_at: '2026-04-08T10:07:00Z',
          created_at: '2026-04-08T10:07:00Z',
          updated_at: '2026-04-08T10:07:00Z',
          messages: [
            {
              id: 'user-queued-1',
              role: 'user',
              content: 'Initial task',
              created_at: '2026-04-08T10:06:00Z',
            },
            {
              id: 'assistant-queued-1',
              role: 'assistant',
              content: 'Working on the first task',
              created_at: '2026-04-08T10:06:01Z',
            },
            {
              id: 'user-queued-2',
              role: 'user',
              content: 'Do this instead',
              created_at: '2026-04-08T10:07:00Z',
            },
            {
              id: 'assistant-queued-2',
              role: 'assistant',
              content: 'Updated answer',
              created_at: '2026-04-08T10:07:01Z',
            },
          ],
        }
      }

      return {
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
      }
    })

    mockApi.llm.chatStream
      .mockImplementationOnce(
        async (_data: unknown, handlers?: { onEvent?: (event: any) => void; signal?: AbortSignal }) =>
          new Promise((_, reject) => {
            handlers?.signal?.addEventListener('abort', () => {
              firstRequestResolved = true
              reject(new DOMException('Aborted', 'AbortError'))
            })
          }),
      )
      .mockImplementationOnce(async (_data: unknown, handlers?: { onEvent?: (event: any) => void }) => {
        handlers?.onEvent?.({
          type: 'turn_started',
          turn: {
            conversation_id: 'conv-queued',
            conversation: {
              id: 'conv-queued',
              title: 'Initial task',
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
              last_message_at: '2026-04-08T10:07:00Z',
              created_at: '2026-04-08T10:07:00Z',
              updated_at: '2026-04-08T10:07:00Z',
            },
            user_message: {
              id: 'user-queued-2',
              role: 'user',
              content: 'Do this instead',
              created_at: '2026-04-08T10:07:00Z',
            },
            assistant_message: {
              id: 'assistant-queued-2',
              role: 'assistant',
              content: '',
              created_at: '2026-04-08T10:07:01Z',
            },
          },
        })
        handlers?.onEvent?.({ type: 'assistant_delta', delta: 'Updated answer' })
        handlers?.onEvent?.({
          type: 'usage',
          usage: {
            prompt_tokens: 10,
            completion_tokens: 5,
            total_tokens: 15,
          },
        })
        return {
          conversation_id: 'conv-queued',
          conversation: {
            id: 'conv-queued',
            title: 'Initial task',
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
            last_message_at: '2026-04-08T10:07:00Z',
            created_at: '2026-04-08T10:07:00Z',
            updated_at: '2026-04-08T10:07:00Z',
          },
          content: 'Updated answer',
          tool_calls: [],
          tool_results: [],
          usage: {
            prompt_tokens: 10,
            completion_tokens: 5,
            total_tokens: 15,
          },
        }
      })

    renderChat('/chat')

    const composer = await screen.findByPlaceholderText(/Message Emerald/i)
    await waitFor(() => expect(composer).toBeEnabled())
    await user.type(composer, 'Initial task{enter}')

    const firstStreamHandler = mockApi.llm.chatStream.mock.calls[0][1]?.onEvent
    await act(async () => {
      firstStreamHandler?.({
        type: 'turn_started',
        turn: {
          conversation_id: 'conv-queued',
          conversation: {
            id: 'conv-queued',
            title: 'Initial task',
            provider_id: 'provider-1',
            proxmox_enabled: true,
            proxmox_cluster_id: 'cluster-1',
            kubernetes_enabled: false,
            kubernetes_cluster_id: undefined,
            compaction_count: 0,
            compacted_at: undefined,
            context_window: 128000,
            context_token_count: 1000,
            last_prompt_tokens: 0,
            last_completion_tokens: 0,
            last_total_tokens: 0,
            last_message_at: '2026-04-08T10:06:00Z',
            created_at: '2026-04-08T10:06:00Z',
            updated_at: '2026-04-08T10:06:00Z',
          },
          user_message: {
            id: 'user-queued-1',
            role: 'user',
            content: 'Initial task',
            created_at: '2026-04-08T10:06:00Z',
          },
          assistant_message: {
            id: 'assistant-queued-1',
            role: 'assistant',
            content: '',
            created_at: '2026-04-08T10:06:01Z',
          },
        },
      })
      firstStreamHandler?.({ type: 'assistant_delta', delta: 'Working on the first task' })
    })

    await user.type(composer, 'Do this instead{enter}')

    await waitFor(() => expect(firstRequestResolved).toBe(true))
    await waitFor(() => expect(mockApi.llm.chatStream).toHaveBeenCalledTimes(2))
    expect(mockApi.llm.chatStream.mock.calls[1][0]).toMatchObject({
      conversation_id: 'conv-queued',
      message: 'Do this instead',
      provider_id: 'provider-1',
    })
    expect(await screen.findByText('Updated answer')).toBeInTheDocument()
  })

  it('renders streamed tool activity inline between assistant text segments', async () => {
    const user = userEvent.setup()
    let resolveStream: ((value: any) => void) | null = null

    mockApi.llm.chatStream.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveStream = resolve
        }),
    )

    renderChat('/chat')

    const composer = await screen.findByPlaceholderText(/Message Emerald/i)
    await waitFor(() => expect(composer).toBeEnabled())
    await user.type(composer, 'Inspect the cluster{enter}')

    const toolCall = {
      id: 'tool-1',
      type: 'function',
      function: {
        name: 'list_nodes',
        arguments: '{"cluster_name":"Local"}',
      },
    }

    const streamHandler = mockApi.llm.chatStream.mock.calls[0][1]?.onEvent
    await act(async () => {
      streamHandler?.({ type: 'assistant_delta', delta: 'Checking the cluster. ' })
      streamHandler?.({ type: 'tool_started', tool_call: toolCall })
      streamHandler?.({
        type: 'tool_finished',
        tool_call: toolCall,
        tool_result: {
          tool: 'list_nodes',
          arguments: { cluster_name: 'Local' },
          result: { nodes: [{ name: 'pve' }] },
        },
      })
      streamHandler?.({ type: 'assistant_delta', delta: 'Found one node.' })
    })

    const beforeNode = await screen.findByText(/Checking the cluster/)
    const toolNode = await screen.findByText('list_nodes')
    const afterNode = await screen.findByText(/Found one node/)

    expect(screen.getByText('Done')).toBeInTheDocument()
    expect(screen.queryByText('Running')).not.toBeInTheDocument()
    expect(beforeNode.compareDocumentPosition(toolNode) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(toolNode.compareDocumentPosition(afterNode) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

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
        content: 'Checking the cluster. Found one node.',
        tool_calls: [toolCall],
        tool_results: [
          {
            tool: 'list_nodes',
            arguments: { cluster_name: 'Local' },
            result: { nodes: [{ name: 'pve' }] },
          },
        ],
        usage: {
          prompt_tokens: 10,
          completion_tokens: 5,
          total_tokens: 15,
        },
      })
      await Promise.resolve()
    })
  })

  it('renders structured diff tool results inside the chat transcript', async () => {
    mockApi.llm.conversations.get.mockResolvedValueOnce({
      id: 'conv-structured',
      title: 'Structured tool conversation',
      provider_id: 'provider-1',
      proxmox_enabled: false,
      proxmox_cluster_id: undefined,
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
          id: 'msg-structured',
          role: 'assistant',
          content: '',
          created_at: '2026-04-08T10:05:00Z',
          tool_calls: [
            {
              id: 'tool-diff',
              type: 'function',
              function: {
                name: 'edit_file',
                arguments: '{"path":"internal/llm/tools.go"}',
              },
            },
          ],
          tool_results: [
            {
              tool: 'edit_file',
              arguments: { path: 'internal/llm/tools.go' },
              result: {
                path: 'internal/llm/tools.go',
                operation: 'updated',
              },
              display: {
                kind: 'diff',
                title: 'internal/llm/tools.go',
                path: 'internal/llm/tools.go',
                summary: 'Updated (+2 -2)',
                diff: '--- internal/llm/tools.go\n+++ internal/llm/tools.go\n@@ -1,3 +1,3 @@\n-func oldValue() string {\n-  return \"old\"\n+func newValue() string {\n+  return \"new\"\n }',
                stats: {
                  additions: 2,
                  deletions: 2,
                },
              },
            },
          ],
        },
      ],
    })

    const user = userEvent.setup()
    renderChat('/chat/conv-structured')

    expect(await screen.findByText('internal/llm/tools.go')).toBeInTheDocument()
    expect(screen.getByText('Updated (+2 -2)')).toBeInTheDocument()
    expect(screen.getAllByText('Diff').length).toBeGreaterThan(0)

    await user.click(screen.getByText('internal/llm/tools.go'))

    const deletedLine = await screen.findByText((_, element) => element?.textContent === '-func oldValue() string {')
    const addedLine = screen.getByText((_, element) => element?.textContent === '+func newValue() string {')
    expect(document.querySelector('.chat-diff-block')).toBeTruthy()
    expect(screen.getByText('@@ -1,3 +1,3 @@').className).toContain('chat-diff-line--hunk')
    expect(deletedLine.className).toContain('chat-diff-line--deletion')
    expect(addedLine.className).toContain('chat-diff-line--addition')
    expect(document.querySelector('.chat-diff-code .hljs-keyword')).toBeTruthy()
    expect(document.querySelector('.chat-diff-code .hljs-string')).toBeTruthy()
  })

  it('renders read_file previews with syntax highlighting and a line-number gutter', async () => {
    mockApi.llm.conversations.get.mockResolvedValueOnce({
      id: 'conv-read',
      title: 'Read tool conversation',
      provider_id: 'provider-1',
      proxmox_enabled: false,
      proxmox_cluster_id: undefined,
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
          id: 'msg-read',
          role: 'assistant',
          content: '',
          created_at: '2026-04-08T10:05:00Z',
          tool_calls: [
            {
              id: 'tool-read',
              type: 'function',
              function: {
                name: 'read_file',
                arguments: '{"path":"test/main.go"}',
              },
            },
          ],
          tool_results: [
            {
              tool: 'read_file',
              arguments: { path: 'test/main.go' },
              result: {
                path: 'test/main.go',
              },
              display: {
                kind: 'read',
                title: 'test/main.go',
                path: 'test/main.go',
                summary: 'Lines 1-5 of 5',
                preview: '1: package main\n2: \n3: import (\n4: \t"fmt"\n5: )',
                stats: {
                  start_line: 1,
                  end_line: 5,
                  total_lines: 5,
                },
              },
            },
          ],
        },
      ],
    })

    const user = userEvent.setup()
    renderChat('/chat/conv-read')

    expect(await screen.findByText('test/main.go')).toBeInTheDocument()
    expect(screen.getByText('Lines 1-5 of 5')).toBeInTheDocument()

    await user.click(screen.getByText('test/main.go'))

    expect(document.querySelector('.chat-code-block')).toBeTruthy()
    const firstLine = await screen.findByText((_, element) => element?.textContent === '1package main')
    expect(firstLine.className).toContain('chat-code-line')
    expect(document.querySelector('.chat-code-gutter')?.textContent).toBe('1')
    expect(document.querySelector('.chat-code-content .hljs-keyword')).toBeTruthy()
    expect(document.querySelector('.chat-code-content .hljs-string')).toBeTruthy()
  })

  it('shows failed tools with a red outline in the chat transcript', async () => {
    const user = userEvent.setup()
    let resolveStream: ((value: any) => void) | null = null

    mockApi.llm.chatStream.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveStream = resolve
        }),
    )

    renderChat('/chat')

    const composer = await screen.findByPlaceholderText(/Message Emerald/i)
    await waitFor(() => expect(composer).toBeEnabled())
    await user.type(composer, 'Read a file{enter}')

    const toolCall = {
      id: 'tool-failed',
      type: 'function',
      function: {
        name: 'read_file',
        arguments: '{"path":"missing.txt"}',
      },
    }

    const streamHandler = mockApi.llm.chatStream.mock.calls[0][1]?.onEvent
    await act(async () => {
      streamHandler?.({ type: 'tool_started', tool_call: toolCall })
      streamHandler?.({
        type: 'tool_finished',
        tool_call: toolCall,
        tool_result: {
          tool: 'read_file',
          arguments: { path: 'missing.txt' },
          error: 'missing.txt was not found',
        },
      })
    })

    const toolLabel = await screen.findByText('read_file')
    const toolContainer = toolLabel.closest('details')
    expect(toolContainer).toBeTruthy()
    expect(toolContainer?.className).toContain('border-red-500/45')

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
        content: 'The file was missing.',
        tool_calls: [toolCall],
        tool_results: [
          {
            tool: 'read_file',
            arguments: { path: 'missing.txt' },
            error: 'missing.txt was not found',
          },
        ],
        usage: {
          prompt_tokens: 10,
          completion_tokens: 5,
          total_tokens: 15,
        },
      })
      await Promise.resolve()
    })
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

    const composer = await screen.findByPlaceholderText(/Message Emerald/i)
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
      reasoning_effort: '',
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

  it('renders stored reasoning and compact tool activity as transcript rows', async () => {
    mockApi.llm.conversations.get.mockResolvedValueOnce({
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
          content: 'The cluster has one node.',
          context_messages: [
            {
              role: 'assistant',
              reasoning: 'I should inspect the cluster before answering.',
              content: 'Checking the cluster now.',
              tool_calls: [
                {
                  id: 'tool-1',
                  type: 'function',
                  function: {
                    name: 'list_nodes',
                    arguments: '{"cluster_name":"Local"}',
                  },
                },
              ],
            },
            {
              role: 'tool',
              name: 'list_nodes',
              tool_call_id: 'tool-1',
              content: '{"result":{"nodes":[{"name":"pve"}]}}',
            },
            {
              role: 'assistant',
              content: 'The cluster has one node.',
            },
          ],
          created_at: '2026-04-08T10:05:00Z',
        },
      ],
    })

    renderChat('/chat/conv-1')

    expect(await screen.findByText('The cluster has one node.')).toBeInTheDocument()
    expect(screen.getAllByText('Reasoning').length).toBeGreaterThan(0)
    expect(screen.getByText('Done')).toBeInTheDocument()
    expect(screen.getByText('Tool use')).toBeInTheDocument()
    expect(screen.queryByText('Running')).not.toBeInTheDocument()
    expect(screen.queryByText('Folded by default')).not.toBeInTheDocument()
    expect(screen.queryByText('Tool request')).not.toBeInTheDocument()
    expect(screen.getAllByText('list_nodes').length).toBeGreaterThan(0)
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
