import { useEffect, useRef, useState, type Dispatch, type MutableRefObject, type SetStateAction } from 'react'
import { createPortal } from 'react-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useParams } from 'react-router-dom'
import { AlertTriangle, Bot, Brain, Check, ChevronDown, Loader2, Menu, MessageSquare, Plus, Send, Server, Settings, Shield, Square, Trash2, User, X } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import type { Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'

import { api } from '../api/client'
import Button from '../components/ui/Button'
import { Card } from '../components/ui/Card'
import Modal from '../components/ui/Modal'
import Select from '../components/ui/Select'
import Skeleton from '../components/ui/Skeleton'
import { useUIStore } from '../store/ui'
import { cn, formatDate } from '../lib/utils'
import type {
  Cluster,
  KubernetesCluster,
  LLMChatResponse,
  LLMConversation,
  LLMConversationMessage,
  LLMConversationSummary,
  LLMProvider,
  LLMToolCall,
  LLMToolResult,
  LLMUsage,
} from '../types'

type ChatSettingsState = {
  providerId: string
  proxmoxEnabled: boolean
  proxmoxClusterId: string
  kubernetesEnabled: boolean
  kubernetesClusterId: string
}

type QuickPickerType = 'model' | 'proxmox' | 'kubernetes' | null

type StreamingToolActivity = {
  id: string
  toolCall?: LLMToolCall
  result?: LLMToolResult
  status: 'running' | 'completed' | 'failed'
}

type ChatSystemNotice = {
  kind: 'rate_limit'
  title: string
  message: string
}

const EMPTY_SETTINGS: ChatSettingsState = {
  providerId: '',
  proxmoxEnabled: false,
  proxmoxClusterId: '',
  kubernetesEnabled: false,
  kubernetesClusterId: '',
}

const EMPTY_PROVIDERS: LLMProvider[] = []
const EMPTY_CLUSTERS: Cluster[] = []
const EMPTY_KUBERNETES_CLUSTERS: KubernetesCluster[] = []
const EMPTY_CONVERSATIONS: LLMConversationSummary[] = []

const STARTER_PROMPTS = [
  'List the available Proxmox nodes',
  'Show recent Kubernetes events',
  'Summarize the automation tools I can use here',
  'Help me create a pipeline to inspect cluster health',
]

const markdownComponents: Components = {
  table: ({ children, ...props }) => (
    <div className="chat-table-wrap">
      <table {...props}>{children}</table>
    </div>
  ),
}

export default function LLMChat() {
  const { conversationId } = useParams<{ conversationId: string }>()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { addToast } = useUIStore()

  const [input, setInput] = useState('')
  const [blankSettings, setBlankSettings] = useState<ChatSettingsState>(EMPTY_SETTINGS)
  const [blankSettingsTouched, setBlankSettingsTouched] = useState(false)
  const [conversationSettings, setConversationSettings] = useState<ChatSettingsState | null>(null)
  const [isSending, setIsSending] = useState(false)
  const [isSavingSettings, setIsSavingSettings] = useState(false)
  const [showConversationDrawer, setShowConversationDrawer] = useState(false)
  const [showSettingsDrawer, setShowSettingsDrawer] = useState(false)
  const [sendingDraft, setSendingDraft] = useState<string | null>(null)
  const [systemNotice, setSystemNotice] = useState<ChatSystemNotice | null>(null)
  const [streamingAssistant, setStreamingAssistant] = useState('')
  const [streamingToolActivity, setStreamingToolActivity] = useState<StreamingToolActivity[]>([])
  const [streamingUsage, setStreamingUsage] = useState<LLMUsage | null>(null)
  const [openQuickPicker, setOpenQuickPicker] = useState<QuickPickerType>(null)
  const [conversationToDelete, setConversationToDelete] = useState<LLMConversationSummary | null>(null)
  const [deletingConversationId, setDeletingConversationId] = useState<string | null>(null)

  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const quickPickerRef = useRef<HTMLDivElement>(null)
  const streamingDeltaQueueRef = useRef<string[]>([])
  const streamingDeltaTimerRef = useRef<number | null>(null)
  const streamingPlaybackResolversRef = useRef<Array<() => void>>([])
  const streamAbortRef = useRef<AbortController | null>(null)

  const providersQuery = useQuery<LLMProvider[]>({
    queryKey: ['llm-providers'],
    queryFn: () => api.llmProviders.list(),
  })
  const providers = providersQuery.data ?? EMPTY_PROVIDERS

  const proxmoxClustersQuery = useQuery<Cluster[]>({
    queryKey: ['clusters'],
    queryFn: () => api.clusters.list(),
  })
  const proxmoxClusters = proxmoxClustersQuery.data ?? EMPTY_CLUSTERS

  const kubernetesClustersQuery = useQuery<KubernetesCluster[]>({
    queryKey: ['kubernetes-clusters'],
    queryFn: () => api.kubernetesClusters.list(),
  })
  const kubernetesClusters = kubernetesClustersQuery.data ?? EMPTY_KUBERNETES_CLUSTERS

  const conversationsQuery = useQuery<LLMConversationSummary[]>({
    queryKey: ['llm-conversations'],
    queryFn: () => api.llm.conversations.list(),
  })

  const conversationQuery = useQuery<LLMConversation, Error>({
    queryKey: ['llm-conversation', conversationId],
    queryFn: () => api.llm.conversations.get(conversationId!),
    enabled: Boolean(conversationId),
  })

  const activeConversation = conversationQuery.data ?? null
  const activeConversationMeta =
    activeConversation ??
    (conversationsQuery.data ?? EMPTY_CONVERSATIONS).find((conversation) => conversation.id === conversationId) ??
    null

  useEffect(() => {
    if (conversationId || blankSettingsTouched) {
      return
    }
    setBlankSettings(buildDefaultSettings(providers, proxmoxClusters, kubernetesClusters))
  }, [conversationId, blankSettingsTouched, providers, proxmoxClusters, kubernetesClusters])

  useEffect(() => {
    if (!conversationId) {
      setConversationSettings(null)
      return
    }
    if (!activeConversation) {
      return
    }
    setConversationSettings(settingsFromConversation(activeConversation, providers, proxmoxClusters, kubernetesClusters))
  }, [conversationId, activeConversation, providers, proxmoxClusters, kubernetesClusters])

  useEffect(() => {
    setShowConversationDrawer(false)
  }, [conversationId])

  useEffect(() => {
    setOpenQuickPicker(null)
  }, [conversationId])

  useEffect(() => {
    setSystemNotice(null)
  }, [conversationId])

  useEffect(() => {
    if (!openQuickPicker) {
      return
    }

    const handlePointerDown = (event: MouseEvent | TouchEvent) => {
      const target = event.target
      if (target instanceof Node && quickPickerRef.current?.contains(target)) {
        return
      }
      setOpenQuickPicker(null)
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setOpenQuickPicker(null)
      }
    }

    document.addEventListener('mousedown', handlePointerDown)
    document.addEventListener('touchstart', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)

    return () => {
      document.removeEventListener('mousedown', handlePointerDown)
      document.removeEventListener('touchstart', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [openQuickPicker])

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [activeConversation?.messages.length, isSending, sendingDraft])

  useEffect(() => {
    textareaRef.current?.focus()
  }, [conversationId])

  useEffect(() => {
    return () => {
      streamAbortRef.current?.abort()
      resetStreamingPlayback(
        streamingDeltaQueueRef,
        streamingDeltaTimerRef,
        streamingPlaybackResolversRef,
      )
    }
  }, [])

  const currentSettings = conversationId
    ? conversationSettings ?? buildDefaultSettings(providers, proxmoxClusters, kubernetesClusters)
    : blankSettings
  const renderedMessages = activeConversation?.messages ?? []
  const canSend = Boolean(input.trim()) && Boolean(currentSettings.providerId) && !isSending
  const providerLabel = providers.find((provider) => provider.id === currentSettings.providerId)?.name ?? 'Choose model'
  const proxmoxLabel = currentSettings.proxmoxEnabled
    ? lookupName(proxmoxClusters, currentSettings.proxmoxClusterId, proxmoxClusters.length > 0 ? 'Select Proxmox' : 'No Proxmox')
    : proxmoxClusters.length > 0
      ? 'Proxmox off'
      : 'No Proxmox'
  const kubernetesLabel = currentSettings.kubernetesEnabled
    ? lookupName(
        kubernetesClusters,
        currentSettings.kubernetesClusterId,
        kubernetesClusters.length > 0 ? 'Select Kubernetes' : 'No Kubernetes',
      )
    : kubernetesClusters.length > 0
      ? 'Kubernetes off'
      : 'No Kubernetes'
  const showContextUsage = Boolean(activeConversationMeta && activeConversationMeta.context_window > 0)

  function updateSettings(updater: (previous: ChatSettingsState) => ChatSettingsState) {
    if (conversationId) {
      setConversationSettings((previous) => updater(previous ?? buildDefaultSettings(providers, proxmoxClusters, kubernetesClusters)))
      return
    }

    setBlankSettingsTouched(true)
    setBlankSettings((previous) => updater(previous))
  }

  async function persistConversationSettings(nextSettings: ChatSettingsState) {
    if (!conversationId) {
      return
    }

    setIsSavingSettings(true)
    try {
      const updated = await api.llm.conversations.update(conversationId, buildSettingsPayload(nextSettings))
      queryClient.setQueryData<LLMConversation>(['llm-conversation', conversationId], (previous) =>
        previous
          ? { ...previous, ...updated }
          : { ...updated, messages: [] },
      )
      queryClient.setQueryData<LLMConversationSummary[]>(['llm-conversations'], (previous) => mergeConversationSummary(previous, updated))
      setConversationSettings(settingsFromConversation(updated, providers, proxmoxClusters, kubernetesClusters))
      void queryClient.invalidateQueries({ queryKey: ['llm-conversation', conversationId] })
      void queryClient.invalidateQueries({ queryKey: ['llm-conversations'] })
      return updated
    } catch (error) {
      addToast({
        type: 'error',
        title: 'Failed to update conversation',
        message: error instanceof Error ? error.message : 'Unknown error',
      })
      throw error
    } finally {
      setIsSavingSettings(false)
    }
  }

  function applyQuickSettings(updater: (previous: ChatSettingsState) => ChatSettingsState) {
    if (!conversationId) {
      updateSettings(updater)
      return
    }

    const nextSettings = updater(conversationSettings ?? buildDefaultSettings(providers, proxmoxClusters, kubernetesClusters))
    setConversationSettings(nextSettings)
    void persistConversationSettings(nextSettings)
  }

  function handleNewConversation() {
    streamAbortRef.current?.abort()
    setInput('')
    setSendingDraft(null)
    setSystemNotice(null)
    setStreamingAssistant('')
    setStreamingToolActivity([])
    setStreamingUsage(null)
    setBlankSettingsTouched(false)
    setBlankSettings(buildDefaultSettings(providers, proxmoxClusters, kubernetesClusters))
    setConversationSettings(null)
    setShowConversationDrawer(false)
    setShowSettingsDrawer(false)
    navigate('/chat')
  }

  function toggleProxmoxQuickSetting() {
    applyQuickSettings((previous) => ({
      ...previous,
      proxmoxEnabled: !previous.proxmoxEnabled,
      proxmoxClusterId: !previous.proxmoxEnabled ? previous.proxmoxClusterId || proxmoxClusters[0]?.id || '' : '',
    }))
  }

  function toggleKubernetesQuickSetting() {
    applyQuickSettings((previous) => ({
      ...previous,
      kubernetesEnabled: !previous.kubernetesEnabled,
      kubernetesClusterId: !previous.kubernetesEnabled ? previous.kubernetesClusterId || kubernetesClusters[0]?.id || '' : '',
    }))
  }

  async function handleSend() {
    const message = input.trim()
    if (!message || !currentSettings.providerId || isSending) {
      return
    }

    const controller = new AbortController()
    streamAbortRef.current = controller
    setIsSending(true)
    setSendingDraft(message)
    setSystemNotice(null)
    setStreamingAssistant('')
    setStreamingToolActivity([])
    setStreamingUsage(null)
    resetStreamingPlayback(
      streamingDeltaQueueRef,
      streamingDeltaTimerRef,
      streamingPlaybackResolversRef,
    )
    setInput('')
    syncComposerHeight(textareaRef.current)

    try {
      const response = await api.llm.chatStream(buildChatPayload(message, currentSettings, conversationId), {
        onEvent: (event) => {
          switch (event.type) {
            case 'assistant_delta':
              enqueueStreamingDelta(
                event.delta,
                setStreamingAssistant,
                streamingDeltaQueueRef,
                streamingDeltaTimerRef,
                streamingPlaybackResolversRef,
              )
              break
            case 'tool_started':
              setStreamingToolActivity((previous) => upsertStreamingToolActivity(previous, event.tool_call, undefined))
              break
            case 'tool_finished':
              setStreamingToolActivity((previous) => upsertStreamingToolActivity(previous, event.tool_call, event.tool_result))
              break
            case 'usage':
              setStreamingUsage(event.usage ?? null)
              break
          }
        },
        signal: controller.signal,
      })
      await waitForStreamingPlayback(streamingDeltaQueueRef, streamingDeltaTimerRef, streamingPlaybackResolversRef)
      const userMessage = makeClientMessage('user', message)
      const assistantMessage = makeAssistantClientMessage(response)
      const nextConversationId = response.conversation_id

      queryClient.setQueryData<LLMConversation>(['llm-conversation', nextConversationId], (previous) => ({
        ...(previous ?? { ...response.conversation, messages: [] }),
        ...response.conversation,
        messages: [...(previous?.messages ?? renderedMessages), userMessage, assistantMessage],
      }))
      queryClient.setQueryData<LLMConversationSummary[]>(['llm-conversations'], (previous) => mergeConversationSummary(previous, response.conversation))

      if (!conversationId) {
        navigate(`/chat/${nextConversationId}`)
      }

      setConversationSettings(settingsFromConversation(response.conversation, providers, proxmoxClusters, kubernetesClusters))
      void queryClient.invalidateQueries({ queryKey: ['llm-conversation', nextConversationId] })
      void queryClient.invalidateQueries({ queryKey: ['llm-conversations'] })
    } catch (error) {
      setInput(message)
      syncComposerHeight(textareaRef.current)
      if (isAbortError(error)) {
        return
      }

      if (isRateLimitError(error)) {
        setSystemNotice({
          kind: 'rate_limit',
          title: 'Rate limited',
          message: 'The model provider is rate limiting this conversation right now. Wait a moment, then send again.',
        })
      } else {
        addToast({
          type: 'error',
          title: 'Failed to send message',
          message: error instanceof Error ? error.message : 'Unknown error',
        })
      }
    } finally {
      if (streamAbortRef.current === controller) {
        streamAbortRef.current = null
      }
      setIsSending(false)
      setSendingDraft(null)
      setStreamingAssistant('')
      setStreamingToolActivity([])
      setStreamingUsage(null)
    }
  }

  async function handleSaveSettings() {
    if (!conversationId || !conversationSettings) {
      setShowSettingsDrawer(false)
      return
    }

    try {
      await persistConversationSettings(conversationSettings)
      setShowSettingsDrawer(false)
      addToast({ type: 'success', title: 'Conversation settings updated' })
    } catch {
      // handled in persistConversationSettings
    }
  }

  async function handleConfirmDeleteConversation() {
    if (!conversationToDelete) {
      return
    }

    setDeletingConversationId(conversationToDelete.id)
    try {
      await api.llm.conversations.delete(conversationToDelete.id)
      queryClient.setQueryData<LLMConversationSummary[]>(['llm-conversations'], (previous) =>
        (previous ?? []).filter((conversation) => conversation.id !== conversationToDelete.id),
      )
      queryClient.removeQueries({ queryKey: ['llm-conversation', conversationToDelete.id] })

      if (conversationId === conversationToDelete.id) {
        handleNewConversation()
      }

      setConversationToDelete(null)
      addToast({
        type: 'success',
        title: 'Conversation deleted',
      })
    } catch (error) {
      addToast({
        type: 'error',
        title: 'Failed to delete conversation',
        message: error instanceof Error ? error.message : 'Unknown error',
      })
    } finally {
      setDeletingConversationId(null)
    }
  }

  function handleComposerKeyDown(event: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault()
      void handleSend()
    }
  }

  function handleInputChange(event: React.ChangeEvent<HTMLTextAreaElement>) {
    setInput(event.target.value)
    syncComposerHeight(event.target)
  }

  function handleStopSending() {
    streamAbortRef.current?.abort()
  }

  const emptyState = !conversationId && renderedMessages.length === 0 && !sendingDraft && !systemNotice
  const conversationNotFound = Boolean(conversationId) && conversationQuery.isError && !activeConversation
  const isConversationLoading = Boolean(conversationId) && conversationQuery.isLoading && !activeConversation

  return (
    <div className="relative flex h-full min-h-full overflow-hidden bg-bg text-text">
      <ConversationRail
        conversations={conversationsQuery.data ?? EMPTY_CONVERSATIONS}
        activeConversationId={conversationId}
        onSelectConversation={(id) => navigate(`/chat/${id}`)}
        onNewConversation={handleNewConversation}
        onDeleteConversation={(conversation) => setConversationToDelete(conversation)}
        deletingConversationId={deletingConversationId}
        className="hidden lg:flex"
      />

      <SideSheet
        open={showConversationDrawer}
        side="left"
        title="Conversations"
        description="Jump between saved chats or start fresh."
        onClose={() => setShowConversationDrawer(false)}
      >
        <ConversationRail
          conversations={conversationsQuery.data ?? EMPTY_CONVERSATIONS}
          activeConversationId={conversationId}
          onSelectConversation={(id) => {
            setShowConversationDrawer(false)
            navigate(`/chat/${id}`)
          }}
          onNewConversation={handleNewConversation}
          onDeleteConversation={(conversation) => setConversationToDelete(conversation)}
          deletingConversationId={deletingConversationId}
          className="flex h-full"
        />
      </SideSheet>

      <Modal
        open={Boolean(conversationToDelete)}
        title="Delete Conversation"
        description="This removes the conversation and all of its stored messages."
        onClose={() => {
          if (!deletingConversationId) {
            setConversationToDelete(null)
          }
        }}
        className="max-w-md"
      >
        <div className="space-y-5">
          <div className="rounded-xl border border-red-500/20 bg-red-500/8 px-4 py-3 text-sm text-text-muted">
            <span className="font-medium text-text">{conversationToDelete?.title ?? 'This conversation'}</span> will be permanently deleted.
          </div>

          <div className="flex items-center justify-end gap-2">
            <Button
              variant="ghost"
              onClick={() => setConversationToDelete(null)}
              disabled={Boolean(deletingConversationId)}
            >
              Cancel
            </Button>
            <Button
              variant="danger"
              onClick={() => void handleConfirmDeleteConversation()}
              loading={Boolean(deletingConversationId)}
            >
              Delete
            </Button>
          </div>
        </div>
      </Modal>

      <Modal
        open={showSettingsDrawer}
        title={conversationId ? 'Conversation Settings' : 'Draft Settings'}
        description={conversationId ? 'These settings are saved with this conversation.' : 'These settings will be used when you send the first message.'}
        onClose={() => setShowSettingsDrawer(false)}
        className="max-w-lg"
      >
        <div className="space-y-5">
          <div className="space-y-2">
            <p className="text-xs font-semibold uppercase tracking-[0.2em] text-text-dimmed">Model</p>
            <Select
              aria-label="Model provider"
              value={currentSettings.providerId}
              onChange={(event) => updateSettings((previous) => ({ ...previous, providerId: event.target.value }))}
            >
              {providers.length === 0 ? (
                <option value="">No providers configured</option>
              ) : (
                providers.map((provider) => (
                  <option key={provider.id} value={provider.id}>
                    {provider.name} ({provider.model})
                  </option>
                ))
              )}
            </Select>
          </div>

          <IntegrationControl
            label="Proxmox"
            enabled={currentSettings.proxmoxEnabled}
            selectedValue={currentSettings.proxmoxClusterId}
            options={proxmoxClusters.map((cluster) => ({ value: cluster.id, label: cluster.name }))}
            emptyLabel="No Proxmox clusters available"
            onToggle={(enabled) =>
              updateSettings((previous) => ({
                ...previous,
                proxmoxEnabled: enabled,
                proxmoxClusterId: enabled ? previous.proxmoxClusterId || proxmoxClusters[0]?.id || '' : '',
              }))
            }
            onSelect={(value) =>
              updateSettings((previous) => ({
                ...previous,
                proxmoxEnabled: true,
                proxmoxClusterId: value,
              }))
            }
          />

          <IntegrationControl
            label="Kubernetes"
            enabled={currentSettings.kubernetesEnabled}
            selectedValue={currentSettings.kubernetesClusterId}
            options={kubernetesClusters.map((cluster) => ({ value: cluster.id, label: cluster.name }))}
            emptyLabel="No Kubernetes clusters available"
            onToggle={(enabled) =>
              updateSettings((previous) => ({
                ...previous,
                kubernetesEnabled: enabled,
                kubernetesClusterId: enabled ? previous.kubernetesClusterId || kubernetesClusters[0]?.id || '' : '',
              }))
            }
            onSelect={(value) =>
              updateSettings((previous) => ({
                ...previous,
                kubernetesEnabled: true,
                kubernetesClusterId: value,
              }))
            }
          />

          <div className="rounded-lg border border-border bg-bg-input px-4 py-3 text-sm text-text-muted">
            {conversationId
              ? 'Changes here are saved to the open conversation and will be restored when you reopen it.'
              : 'Nothing is stored yet. Your first sent message creates the conversation and saves these settings.'}
          </div>

          <div className="flex items-center justify-end gap-2">
            <Button variant="ghost" onClick={() => setShowSettingsDrawer(false)}>
              Close
            </Button>
            {conversationId && (
              <Button onClick={() => void handleSaveSettings()} loading={isSavingSettings}>
                Save Settings
              </Button>
            )}
          </div>
        </div>
      </Modal>

      <section className="relative flex min-w-0 flex-1 flex-col bg-bg">
        <div className="pointer-events-none absolute left-4 top-4 z-10 lg:hidden">
          <Button
            variant="secondary"
            size="sm"
            className="pointer-events-auto shadow-sm shadow-black/10"
            onClick={() => setShowConversationDrawer(true)}
            aria-label="Open conversations"
          >
            <Menu className="h-4 w-4" />
          </Button>
        </div>

        <div className="flex-1 overflow-y-auto px-4 pb-6 pt-16 sm:px-6 lg:pt-6">
          <div className="mx-auto flex min-h-full w-full max-w-4xl flex-col">
            {conversationNotFound ? (
              <Card className="mx-auto mt-12 w-full max-w-xl p-8 text-center">
                <MessageSquare className="mx-auto mb-4 h-12 w-12 text-text-dimmed" />
                <h2 className="text-xl font-semibold text-text">Conversation not found</h2>
                <p className="mt-2 text-sm text-text-muted">
                  This conversation may have been removed or belongs to another account.
                </p>
                <div className="mt-6">
                  <Button onClick={handleNewConversation}>
                    <Plus className="h-4 w-4" />
                    Start a New Conversation
                  </Button>
                </div>
              </Card>
            ) : isConversationLoading ? (
              <div className="mx-auto flex w-full max-w-4xl flex-col gap-5">
                {[1, 2, 3].map((item) => (
                  <div key={item} className={cn('flex gap-3', item % 2 === 0 ? 'justify-end' : '')}>
                    <Skeleton className={cn('h-28 rounded-xl', item % 2 === 0 ? 'w-[55%]' : 'w-[68%]')} />
                  </div>
                ))}
              </div>
            ) : emptyState ? (
              <EmptyState
                providerReady={providers.length > 0}
                onUsePrompt={(prompt) => {
                  setInput(prompt)
                  syncComposerHeight(textareaRef.current)
                  textareaRef.current?.focus()
                }}
              />
            ) : (
              <div className="space-y-6">
                {renderedMessages.map((message) => (
                  <MessageBubble key={message.id} message={message} />
                ))}

                {sendingDraft && (
                  <MessageBubble message={makeClientMessage('user', sendingDraft)} />
                )}

                {systemNotice && (
                  <SystemNoticeBubble notice={systemNotice} />
                )}

                {isSending && (
                  <StreamingAssistantBubble
                    content={streamingAssistant}
                    toolActivity={streamingToolActivity}
                    usage={streamingUsage}
                  />
                )}
              </div>
            )}
            <div ref={messagesEndRef} />
          </div>
        </div>

        <div className="bg-bg px-4 pb-6 pt-4 sm:px-6">
          <div className="mx-auto w-full max-w-4xl">
            {providers.length === 0 && (
              <div className="mb-3 rounded-xl border border-amber-600/30 bg-amber-600/10 px-4 py-3 text-sm text-amber-400">
                Configure an LLM provider in Settings before starting a conversation.
              </div>
            )}

            <div className="rounded-2xl border border-border bg-bg-elevated shadow-sm shadow-black/10">
              <textarea
                ref={textareaRef}
                value={input}
                onChange={handleInputChange}
                onKeyDown={handleComposerKeyDown}
                placeholder={conversationId ? 'Reply in this conversation…' : 'Message Automator about infrastructure, pipelines, or local tooling…'}
                className="min-h-[92px] max-h-56 w-full resize-none border-0 bg-transparent px-4 pb-2 pt-4 text-sm text-text placeholder:text-text-dimmed focus:outline-none"
                disabled={isSending || providers.length === 0}
                rows={1}
              />
              <div className="flex flex-wrap items-end justify-between gap-3 px-4 pb-4 pt-2">
                <div ref={quickPickerRef} className="relative flex min-w-0 flex-1 flex-wrap items-center gap-2">
                  {openQuickPicker && (
                    <QuickPickerPanel>
                      {openQuickPicker === 'model' ? (
                        <QuickModelPicker
                          providers={providers}
                          selectedProviderId={currentSettings.providerId}
                          onSelect={(providerId) => {
                            applyQuickSettings((previous) => ({ ...previous, providerId }))
                            setOpenQuickPicker(null)
                          }}
                        />
                      ) : (
                        <QuickIntegrationPicker
                          icon={openQuickPicker === 'proxmox' ? Server : Shield}
                          label={openQuickPicker === 'proxmox' ? 'Proxmox' : 'Kubernetes'}
                          enabled={openQuickPicker === 'proxmox' ? currentSettings.proxmoxEnabled : currentSettings.kubernetesEnabled}
                          options={(openQuickPicker === 'proxmox' ? proxmoxClusters : kubernetesClusters).map((cluster) => ({
                            value: cluster.id,
                            label: cluster.name,
                          }))}
                          selectedValue={openQuickPicker === 'proxmox' ? currentSettings.proxmoxClusterId : currentSettings.kubernetesClusterId}
                          disabled={isSavingSettings}
                          emptyLabel={openQuickPicker === 'proxmox' ? 'No Proxmox clusters available yet.' : 'No Kubernetes clusters available yet.'}
                          onToggle={() => {
                            if (openQuickPicker === 'proxmox') {
                              toggleProxmoxQuickSetting()
                              return
                            }
                            toggleKubernetesQuickSetting()
                          }}
                          onSelect={(value) => {
                            applyQuickSettings((previous) => ({
                              ...previous,
                              ...(openQuickPicker === 'proxmox'
                                ? { proxmoxEnabled: true, proxmoxClusterId: value }
                                : { kubernetesEnabled: true, kubernetesClusterId: value }),
                            }))
                            setOpenQuickPicker(null)
                          }}
                        />
                      )}
                    </QuickPickerPanel>
                  )}

                  <QuickPickerButton
                    icon={Brain}
                    label={providerLabel}
                    active={openQuickPicker === 'model'}
                    disabled={providers.length === 0 || isSavingSettings || isSending}
                    onClick={() => setOpenQuickPicker((previous) => (previous === 'model' ? null : 'model'))}
                  />

                  <QuickPickerButton
                    icon={Server}
                    label={proxmoxLabel}
                    active={openQuickPicker === 'proxmox'}
                    muted={!currentSettings.proxmoxEnabled}
                    disabled={proxmoxClusters.length === 0 || isSavingSettings || isSending}
                    onClick={() => setOpenQuickPicker((previous) => (previous === 'proxmox' ? null : 'proxmox'))}
                  />

                  <QuickPickerButton
                    icon={Shield}
                    label={kubernetesLabel}
                    active={openQuickPicker === 'kubernetes'}
                    muted={!currentSettings.kubernetesEnabled}
                    disabled={kubernetesClusters.length === 0 || isSavingSettings || isSending}
                    onClick={() => setOpenQuickPicker((previous) => (previous === 'kubernetes' ? null : 'kubernetes'))}
                  />

                  <button
                    type="button"
                    onClick={() => {
                      setOpenQuickPicker(null)
                      setShowSettingsDrawer(true)
                    }}
                    className="inline-flex h-8 w-8 items-center justify-center rounded-full border border-border bg-bg text-text-muted transition-colors hover:border-accent/30 hover:text-text disabled:cursor-not-allowed disabled:opacity-50"
                    aria-label="Open advanced settings"
                    disabled={isSending}
                  >
                    <Settings className="h-3.5 w-3.5 text-accent" />
                  </button>

                  {showContextUsage && activeConversationMeta && (
                    <ContextUsagePill
                      used={activeConversationMeta.context_token_count}
                      limit={activeConversationMeta.context_window}
                      compacted={activeConversationMeta.compaction_count > 0}
                      lastRequestTokens={activeConversationMeta.last_total_tokens}
                    />
                  )}

                  {isSavingSettings && (
                    <span className="inline-flex h-8 items-center gap-1 rounded-full border border-border bg-bg px-2.5 text-[11px] text-text-dimmed">
                      <Loader2 className="h-3 w-3 animate-spin" />
                      Saving
                    </span>
                  )}
                </div>

                <button
                  type="button"
                  onClick={() => {
                    if (isSending) {
                      handleStopSending()
                      return
                    }
                    void handleSend()
                  }}
                  disabled={isSending ? false : !canSend}
                  className={cn(
                    'inline-flex h-10 w-10 items-center justify-center rounded-lg border transition-colors',
                    isSending
                      ? 'border-red-500/40 bg-red-500/12 text-red-300 hover:bg-red-500/18'
                      : canSend
                        ? 'border-accent bg-accent text-white hover:bg-accent-hover'
                        : 'border-border bg-bg-input text-text-dimmed',
                  )}
                  aria-label={isSending ? 'Stop response' : 'Send message'}
                >
                  {isSending ? <Square className="h-4 w-4 fill-current" /> : <Send className="h-4 w-4" />}
                </button>
              </div>
            </div>
          </div>
        </div>
      </section>
    </div>
  )
}

function ConversationRail({
  conversations,
  activeConversationId,
  onSelectConversation,
  onNewConversation,
  onDeleteConversation,
  deletingConversationId,
  className,
}: {
  conversations: LLMConversationSummary[]
  activeConversationId?: string
  onSelectConversation: (id: string) => void
  onNewConversation: () => void
  onDeleteConversation: (conversation: LLMConversationSummary) => void
  deletingConversationId: string | null
  className?: string
}) {
  const sortedConversations = [...conversations].sort(
    (left, right) => new Date(right.last_message_at).getTime() - new Date(left.last_message_at).getTime(),
  )

  return (
    <aside className={cn('w-[310px] shrink-0 flex-col border-r border-border bg-bg-elevated', className)}>
      <div className="px-4 pb-3 pt-4">
        <Button className="w-full justify-center" onClick={onNewConversation}>
          <Plus className="h-4 w-4" />
          New Conversation
        </Button>
      </div>

      <div className="flex-1 space-y-2 overflow-y-auto px-3 py-4">
        {sortedConversations.length === 0 ? (
          <div className="rounded-xl border border-dashed border-border bg-bg-input px-4 py-6 text-center">
            <MessageSquare className="mx-auto mb-3 h-8 w-8 text-text-dimmed" />
            <p className="text-sm font-medium text-text">No saved conversations</p>
            <p className="mt-1 text-xs text-text-muted">Your first sent message will create one here.</p>
          </div>
        ) : (
          sortedConversations.map((conversation) => {
            const active = conversation.id === activeConversationId
            const deleting = deletingConversationId === conversation.id
            return (
              <div
                key={conversation.id}
                className={cn(
                  'group flex items-stretch gap-2 rounded-xl border p-2 transition-colors',
                  active
                    ? 'border-accent/40 bg-accent/12 shadow-sm shadow-accent/10'
                    : 'border-transparent bg-transparent hover:border-border hover:bg-bg-input',
                )}
              >
                <button
                  type="button"
                  onClick={() => onSelectConversation(conversation.id)}
                  className="min-w-0 flex-1 rounded-lg px-2 py-1.5 text-left"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className={cn('truncate text-sm font-medium', active ? 'text-text' : 'text-text-muted')}>{conversation.title}</p>
                      <p className="mt-1 line-clamp-2 text-xs text-text-dimmed">
                        Updated {formatSidebarTimestamp(conversation.last_message_at)}
                      </p>
                    </div>
                    <div className={cn('mt-0.5 h-2.5 w-2.5 rounded-full', active ? 'bg-accent' : 'bg-transparent')} />
                  </div>
                </button>

                <button
                  type="button"
                  onClick={() => onDeleteConversation(conversation)}
                  disabled={deleting}
                  className={cn(
                    'inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-transparent text-text-dimmed transition-colors hover:border-border hover:bg-bg hover:text-red-300 disabled:cursor-not-allowed disabled:opacity-60',
                    active && 'border-border/60 bg-bg/50',
                  )}
                  aria-label={`Delete conversation ${conversation.title}`}
                >
                  {deleting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}
                </button>
              </div>
            )
          })
        )}
      </div>
    </aside>
  )
}

function EmptyState({
  providerReady,
  onUsePrompt,
}: {
  providerReady: boolean
  onUsePrompt: (prompt: string) => void
}) {
  return (
    <div className="mx-auto flex w-full max-w-2xl flex-1 flex-col justify-center py-8 text-center">
      <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-2xl border border-border bg-bg-elevated shadow-sm shadow-black/10">
        <Bot className="h-6 w-6 text-accent" />
      </div>
      <h2 className="text-lg font-semibold text-text">New conversation</h2>
      <p className="mx-auto mt-2 max-w-lg text-sm text-text-muted">
        Pick a starter or start typing below.
      </p>
      {!providerReady && <p className="mx-auto mt-2 text-xs text-text-dimmed">Add a model below to send your first message.</p>}

      <div className="mt-5 grid gap-2.5 sm:grid-cols-2">
        {STARTER_PROMPTS.map((prompt) => (
          <button
            key={prompt}
            type="button"
            onClick={() => onUsePrompt(prompt)}
            className="group rounded-xl border border-border bg-bg-elevated px-4 py-3 text-left transition-colors hover:border-accent/30 hover:bg-bg-input"
          >
            <div className="flex items-center gap-3">
              <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-bg-input text-accent">
                <Brain className="h-4 w-4" />
              </div>
              <div className="min-w-0">
                <p className="text-sm font-medium text-text">{prompt}</p>
              </div>
            </div>
          </button>
        ))}
      </div>
    </div>
  )
}

function MessageBubble({ message }: { message: LLMConversationMessage }) {
  const isUser = message.role === 'user'

  return (
    <div className={cn('flex gap-3', isUser ? 'justify-end' : '')}>
      {!isUser && (
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-border bg-bg-elevated text-text-muted">
          <Bot className="h-4 w-4" />
        </div>
      )}

      <div className={cn('w-full max-w-3xl', isUser ? 'flex flex-col items-end' : '')}>
        <div
          className={cn(
            'overflow-hidden rounded-xl border px-4 py-4',
            isUser
              ? 'max-w-[85%] border-accent/30 bg-accent/10 text-text'
              : 'border-border bg-bg-elevated',
          )}
        >
          {isUser ? (
            <p className="whitespace-pre-wrap text-sm leading-6 text-text">{message.content}</p>
          ) : (
            <div className="chat-markdown text-sm text-text">
              <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
                {message.content}
              </ReactMarkdown>
            </div>
          )}

          {message.tool_results && message.tool_results.length > 0 && (
            <div className="mt-4 space-y-3 border-t border-border pt-4">
              {message.tool_results.map((result, index) => (
                <ToolResultCard key={`${result.tool}-${index}`} result={result} />
              ))}
            </div>
          )}
        </div>

        {isUser ? (
          <div className="mt-1.5 px-1 text-[11px] text-text-dimmed">
            {formatMessageTimestamp(message.created_at)}
          </div>
        ) : (
          <div className="mt-2 flex items-center gap-2 text-xs text-text-dimmed">
            <MessageSquare className="h-3.5 w-3.5" />
            <span>{formatMessageTimestamp(message.created_at)}</span>
            {message.usage && <span>{message.usage.total_tokens} tokens</span>}
          </div>
        )}
      </div>

      {isUser && (
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-accent/30 bg-accent/10 text-accent">
          <User className="h-4 w-4" />
        </div>
      )}
    </div>
  )
}

function StreamingAssistantBubble({
  content,
  toolActivity,
  usage,
}: {
  content: string
  toolActivity: StreamingToolActivity[]
  usage: LLMUsage | null
}) {
  const hasContent = content.trim().length > 0

  return (
    <div className="flex gap-3">
      <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-border bg-bg-elevated text-text-muted">
        <Bot className="h-4 w-4" />
      </div>

      <div className="w-full max-w-3xl">
        <div className="overflow-hidden rounded-xl border border-border bg-bg-elevated px-4 py-4">
          {hasContent ? (
            <div className="chat-markdown text-sm text-text">
              <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
                {content}
              </ReactMarkdown>
            </div>
          ) : (
            <div className="flex items-center gap-2 text-sm text-text-muted">
              <Loader2 className="h-4 w-4 animate-spin text-accent" />
              <span>{toolActivity.length > 0 ? 'Working with tools…' : 'Thinking…'}</span>
            </div>
          )}

          {toolActivity.length > 0 && (
            <div className="mt-4 space-y-3 border-t border-border pt-4">
              {toolActivity.map((activity) => (
                <StreamingToolActivityCard key={activity.id} activity={activity} />
              ))}
            </div>
          )}
        </div>

        <div className="mt-2 flex items-center gap-2 text-xs text-text-dimmed">
          <MessageSquare className="h-3.5 w-3.5" />
          <span>Streaming response</span>
          {usage && usage.total_tokens > 0 && <span>{usage.total_tokens} tokens</span>}
        </div>
      </div>
    </div>
  )
}

function SystemNoticeBubble({ notice }: { notice: ChatSystemNotice }) {
  return (
    <div className="mx-auto w-full max-w-2xl">
      <div className="rounded-xl border border-amber-500/25 bg-amber-500/10 px-4 py-4 text-sm text-amber-100">
        <div className="flex items-start gap-3">
          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-amber-500/20 bg-amber-500/10 text-amber-300">
            <AlertTriangle className="h-4 w-4" />
          </div>
          <div className="min-w-0">
            <p className="font-medium text-amber-200">{notice.title}</p>
            <p className="mt-1 text-sm leading-6 text-amber-100/90">{notice.message}</p>
          </div>
        </div>
      </div>
    </div>
  )
}

function StreamingToolActivityCard({ activity }: { activity: StreamingToolActivity }) {
  const label = activity.toolCall?.function.name ?? activity.result?.tool ?? 'Tool'
  const finished = activity.status !== 'running'
  const failed = activity.status === 'failed'

  return (
    <details open={!finished} className="group rounded-lg border border-border bg-bg-input">
      <summary className="flex cursor-pointer list-none items-center justify-between gap-3 px-4 py-3">
        <div className="min-w-0">
          <p className="truncate font-mono text-xs text-text">{label}</p>
          <p className="mt-1 text-[11px] uppercase tracking-[0.18em] text-text-dimmed">
            {activity.status === 'running' ? 'Running now' : failed ? 'Execution failed' : 'Execution complete'}
          </p>
        </div>
        <div
          className={cn(
            'inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-[11px] font-medium',
            activity.status === 'running'
              ? 'border-border bg-bg text-text-muted'
              : failed
                ? 'border-red-500/30 bg-red-500/10 text-red-300'
                : 'border-accent/30 bg-accent/10 text-accent',
          )}
        >
          {activity.status === 'running' && <Loader2 className="h-3 w-3 animate-spin" />}
          <span>{activity.status === 'running' ? 'Running' : failed ? 'Failed' : 'Success'}</span>
        </div>
      </summary>

      {(activity.toolCall?.function.arguments || activity.result) && (
        <div className="space-y-3 border-t border-border px-4 py-4">
          {activity.toolCall?.function.arguments && (
            <div>
              <p className="mb-1 text-[11px] uppercase tracking-[0.16em] text-text-dimmed">Arguments</p>
              <pre className="overflow-x-auto whitespace-pre-wrap rounded-lg bg-bg px-3 py-2 text-xs text-text-muted">
                {formatToolArguments(activity.toolCall.function.arguments)}
              </pre>
            </div>
          )}

          {activity.result && (
            <div>
              <p className="mb-1 text-[11px] uppercase tracking-[0.16em] text-text-dimmed">
                {activity.result.error ? 'Error' : 'Result'}
              </p>
              <pre
                className={cn(
                  'overflow-x-auto whitespace-pre-wrap rounded-lg px-3 py-2 text-xs',
                  activity.result.error ? 'bg-red-500/10 text-red-300' : 'bg-bg/80 text-text-muted',
                )}
              >
                {activity.result.error || JSON.stringify(activity.result.result, null, 2)}
              </pre>
            </div>
          )}
        </div>
      )}
    </details>
  )
}

function ToolResultCard({ result }: { result: LLMToolResult }) {
  return (
    <details className="group rounded-lg border border-border bg-bg-input">
      <summary className="flex cursor-pointer list-none items-center justify-between gap-3 px-4 py-3">
        <div className="min-w-0">
          <p className="truncate font-mono text-xs text-text">{result.tool}</p>
          <p className="mt-1 text-[11px] uppercase tracking-[0.18em] text-text-dimmed">
            {result.error ? 'Execution failed' : 'Execution details'}
          </p>
        </div>
        <div
          className={cn(
            'rounded-full border px-2.5 py-1 text-[11px] font-medium',
            result.error
              ? 'border-red-500/30 bg-red-500/10 text-red-300'
              : 'border-accent/30 bg-accent/10 text-accent',
          )}
        >
          {result.error ? 'Failed' : 'Success'}
        </div>
      </summary>
      <div className="space-y-3 border-t border-border px-4 py-4">
        {result.arguments !== undefined && (
          <div>
            <p className="mb-1 text-[11px] uppercase tracking-[0.16em] text-text-dimmed">Arguments</p>
            <pre className="overflow-x-auto whitespace-pre-wrap rounded-lg bg-bg px-3 py-2 text-xs text-text-muted">
              {JSON.stringify(result.arguments, null, 2)}
            </pre>
          </div>
        )}
        <div>
          <p className="mb-1 text-[11px] uppercase tracking-[0.16em] text-text-dimmed">
            {result.error ? 'Error' : 'Result'}
          </p>
          <pre
            className={cn(
              'overflow-x-auto whitespace-pre-wrap rounded-lg px-3 py-2 text-xs',
              result.error ? 'bg-red-500/10 text-red-300' : 'bg-bg/80 text-text-muted',
            )}
          >
            {result.error || JSON.stringify(result.result, null, 2)}
          </pre>
        </div>
      </div>
    </details>
  )
}

function QuickPickerPanel({ children }: { children: React.ReactNode }) {
  return (
    <div className="absolute bottom-full left-0 z-20 mb-3 w-[min(24rem,calc(100vw-3rem))] overflow-hidden rounded-2xl border border-border bg-bg-elevated shadow-2xl shadow-black/25">
      {children}
    </div>
  )
}

function QuickPickerButton({
  icon: Icon,
  label,
  active,
  muted,
  disabled,
  onClick,
}: {
  icon: React.ElementType
  label: string
  active?: boolean
  muted?: boolean
  disabled?: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      aria-expanded={active}
      className={cn(
        'inline-flex h-8 max-w-full items-center gap-1.5 rounded-full border px-2.5 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50',
        active
          ? 'border-accent/40 bg-accent/10 text-text shadow-sm shadow-accent/10'
          : 'border-border bg-bg text-text-muted hover:border-accent/30 hover:text-text',
      )}
    >
      <ChevronDown className={cn('h-3.5 w-3.5 shrink-0 transition-transform', active && 'rotate-180')} />
      <Icon className={cn('h-3.5 w-3.5 shrink-0', muted ? 'text-text-dimmed' : 'text-accent')} />
      <span className="max-w-[8rem] truncate sm:max-w-[11rem]">{label}</span>
    </button>
  )
}

function ContextUsagePill({
  used,
  limit,
  compacted,
  lastRequestTokens,
}: {
  used: number
  limit: number
  compacted: boolean
  lastRequestTokens: number
}) {
  const ratio = limit > 0 ? used / limit : 0

  return (
    <div
      className={cn(
        'inline-flex h-8 items-center gap-2 rounded-full border px-2.5 text-[11px] text-text-dimmed',
        ratio >= 0.8 ? 'border-amber-500/30 bg-amber-500/10 text-amber-300' : 'border-border bg-bg',
      )}
      title={lastRequestTokens > 0 ? `Last request used ${formatCompactTokens(lastRequestTokens)} tokens.` : undefined}
    >
      <Brain className="h-3.5 w-3.5" />
      <span>{formatCompactTokens(used)} / {formatCompactTokens(limit)}</span>
      {compacted && <span className="rounded-full bg-bg-input px-1.5 py-0.5 text-[10px] uppercase tracking-[0.14em]">Compacted</span>}
    </div>
  )
}

function QuickModelPicker({
  providers,
  selectedProviderId,
  onSelect,
}: {
  providers: LLMProvider[]
  selectedProviderId: string
  onSelect: (providerId: string) => void
}) {
  return (
    <div>
      <div className="border-b border-border px-4 py-3">
        <div className="flex items-center gap-2 text-sm font-semibold text-text">
          <Brain className="h-4 w-4 text-accent" />
          Choose Model
        </div>
        <p className="mt-1 text-xs text-text-dimmed">Select the model provider for this chat.</p>
      </div>
      <div className="max-h-72 space-y-1 overflow-y-auto p-2">
        {providers.length === 0 ? (
          <div className="rounded-xl border border-dashed border-border bg-bg-input px-4 py-5 text-sm text-text-muted">
            No providers configured yet.
          </div>
        ) : (
          providers.map((provider) => {
            const selected = provider.id === selectedProviderId
            return (
              <button
                key={provider.id}
                type="button"
                onClick={() => onSelect(provider.id)}
                className={cn(
                  'flex w-full items-start justify-between gap-3 rounded-xl px-3 py-3 text-left transition-colors',
                  selected ? 'bg-accent/10 text-text' : 'text-text-muted hover:bg-bg-input hover:text-text',
                )}
              >
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{provider.name}</p>
                  <p className="mt-1 truncate text-xs text-text-dimmed">{provider.model}</p>
                </div>
                <Check className={cn('mt-0.5 h-4 w-4 shrink-0', selected ? 'text-accent' : 'text-transparent')} />
              </button>
            )
          })
        )}
      </div>
    </div>
  )
}

function QuickIntegrationPicker({
  icon: Icon,
  label,
  enabled,
  options,
  selectedValue,
  emptyLabel,
  disabled,
  onToggle,
  onSelect,
}: {
  icon: React.ElementType
  label: string
  enabled: boolean
  options: Array<{ value: string; label: string }>
  selectedValue: string
  emptyLabel: string
  disabled?: boolean
  onToggle: () => void
  onSelect: (value: string) => void
}) {
  const selectedLabel = options.find((option) => option.value === selectedValue)?.label ?? ''

  return (
    <div>
      <div className="border-b border-border px-4 py-3">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex items-center gap-2 text-sm font-semibold text-text">
              <Icon className="h-4 w-4 text-accent" />
              {label}
            </div>
            <p className="mt-1 text-xs text-text-dimmed">
              {enabled ? selectedLabel || `Choose a ${label} cluster.` : `Enable ${label} access for this chat.`}
            </p>
          </div>
          <button
            type="button"
            onClick={onToggle}
            disabled={disabled || options.length === 0}
            className={cn(
              'inline-flex min-w-[3.75rem] items-center justify-center rounded-full border px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.12em] transition-colors disabled:cursor-not-allowed disabled:opacity-50',
              enabled
                ? 'border-accent/30 bg-accent/10 text-accent'
                : 'border-border bg-bg text-text-dimmed hover:text-text',
            )}
          >
            {enabled ? 'On' : 'Off'}
          </button>
        </div>
      </div>
      <div className="max-h-72 space-y-1 overflow-y-auto p-2">
        {options.length === 0 ? (
          <div className="rounded-xl border border-dashed border-border bg-bg-input px-4 py-5 text-sm text-text-muted">
            {emptyLabel}
          </div>
        ) : (
          options.map((option) => {
            const selected = enabled && option.value === selectedValue
            return (
              <button
                key={option.value}
                type="button"
                onClick={() => onSelect(option.value)}
                disabled={disabled}
                className={cn(
                  'flex w-full items-center justify-between gap-3 rounded-xl px-3 py-3 text-left transition-colors disabled:cursor-not-allowed disabled:opacity-50',
                  selected ? 'bg-accent/10 text-text' : 'text-text-muted hover:bg-bg-input hover:text-text',
                )}
              >
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{option.label}</p>
                  <p className="mt-1 text-xs text-text-dimmed">{selected ? `${label} is active` : `Use this ${label} cluster`}</p>
                </div>
                <Check className={cn('h-4 w-4 shrink-0', selected ? 'text-accent' : 'text-transparent')} />
              </button>
            )
          })
        )}
      </div>
    </div>
  )
}

function IntegrationControl({
  label,
  enabled,
  selectedValue,
  options,
  emptyLabel,
  onToggle,
  onSelect,
}: {
  label: string
  enabled: boolean
  selectedValue: string
  options: Array<{ value: string; label: string }>
  emptyLabel: string
  onToggle: (enabled: boolean) => void
  onSelect: (value: string) => void
}) {
  return (
    <div className="space-y-3 rounded-xl border border-border bg-bg-input p-4">
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="text-sm font-medium text-text">{label}</p>
          <p className="mt-1 text-xs text-text-dimmed">
            {enabled ? 'Enabled for this conversation.' : 'Disabled for this conversation.'}
          </p>
        </div>
        <button
          type="button"
          onClick={() => onToggle(!enabled)}
          aria-label={`${label} integration toggle`}
          className={cn(
            'relative inline-flex h-7 w-12 items-center rounded-full transition-colors',
            enabled ? 'bg-accent' : 'bg-bg-overlay border border-border',
          )}
          aria-pressed={enabled}
        >
          <span
            className={cn(
              'inline-block h-5 w-5 transform rounded-full bg-white transition-transform',
              enabled ? 'translate-x-6' : 'translate-x-1',
            )}
          />
        </button>
      </div>

      <Select
        aria-label={`${label} cluster`}
        value={selectedValue}
        onChange={(event) => onSelect(event.target.value)}
        disabled={!enabled || options.length === 0}
      >
        {options.length === 0 ? (
          <option value="">{emptyLabel}</option>
        ) : (
          <>
            <option value="">Select a cluster</option>
            {options.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </>
        )}
      </Select>
    </div>
  )
}

function SideSheet({
  open,
  side,
  title,
  description,
  onClose,
  children,
}: {
  open: boolean
  side: 'left' | 'right'
  title: string
  description: string
  onClose: () => void
  children: React.ReactNode
}) {
  useEffect(() => {
    if (!open) {
      return
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        onClose()
      }
    }

    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [open, onClose])

  if (!open || typeof document === 'undefined') {
    return null
  }

  return createPortal(
    <div
      className="fixed inset-0 z-[90] bg-black/55"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) {
          onClose()
        }
      }}
    >
      <div
        className={cn(
          'absolute inset-y-0 w-full max-w-md border-border bg-bg-elevated shadow-2xl',
          side === 'left' ? 'left-0 border-r' : 'right-0 border-l',
        )}
      >
        <div className="flex items-start justify-between gap-4 border-b border-border px-5 py-4">
          <div>
            <h2 className="text-lg font-semibold text-text">{title}</h2>
            <p className="mt-1 text-sm text-text-muted">{description}</p>
          </div>
          <Button variant="ghost" size="sm" onClick={onClose}>
            <X className="h-4 w-4" />
          </Button>
        </div>
        <div className="h-[calc(100%-88px)] overflow-y-auto">{children}</div>
      </div>
    </div>,
    document.body,
  )
}

function buildDefaultSettings(
  providers: LLMProvider[],
  proxmoxClusters: Cluster[],
  kubernetesClusters: KubernetesCluster[],
): ChatSettingsState {
  const defaultProvider = providers.find((provider) => provider.is_default)?.id ?? providers[0]?.id ?? ''
  const defaultProxmoxCluster = proxmoxClusters[0]?.id ?? ''
  const defaultKubernetesCluster = kubernetesClusters[0]?.id ?? ''

  if (proxmoxClusters.length > 0) {
    return {
      providerId: defaultProvider,
      proxmoxEnabled: true,
      proxmoxClusterId: defaultProxmoxCluster,
      kubernetesEnabled: false,
      kubernetesClusterId: '',
    }
  }

  if (kubernetesClusters.length > 0) {
    return {
      providerId: defaultProvider,
      proxmoxEnabled: false,
      proxmoxClusterId: '',
      kubernetesEnabled: true,
      kubernetesClusterId: defaultKubernetesCluster,
    }
  }

  return {
    providerId: defaultProvider,
    proxmoxEnabled: false,
    proxmoxClusterId: '',
    kubernetesEnabled: false,
    kubernetesClusterId: '',
  }
}

function settingsFromConversation(
  conversation: Pick<LLMConversationSummary, 'provider_id' | 'proxmox_enabled' | 'proxmox_cluster_id' | 'kubernetes_enabled' | 'kubernetes_cluster_id'>,
  providers: LLMProvider[],
  proxmoxClusters: Cluster[],
  kubernetesClusters: KubernetesCluster[],
): ChatSettingsState {
  const defaults = buildDefaultSettings(providers, proxmoxClusters, kubernetesClusters)
  return {
    providerId: conversation.provider_id ?? defaults.providerId,
    proxmoxEnabled: conversation.proxmox_enabled,
    proxmoxClusterId: conversation.proxmox_cluster_id ?? defaults.proxmoxClusterId,
    kubernetesEnabled: conversation.kubernetes_enabled,
    kubernetesClusterId: conversation.kubernetes_cluster_id ?? defaults.kubernetesClusterId,
  }
}

function buildChatPayload(message: string, settings: ChatSettingsState, conversationId?: string) {
  return {
    conversation_id: conversationId,
    message,
    provider_id: settings.providerId || undefined,
    integrations: {
      proxmox: {
        enabled: settings.proxmoxEnabled,
        cluster_id: settings.proxmoxEnabled ? settings.proxmoxClusterId || undefined : undefined,
      },
      kubernetes: {
        enabled: settings.kubernetesEnabled,
        cluster_id: settings.kubernetesEnabled ? settings.kubernetesClusterId || undefined : undefined,
      },
    },
  }
}

function buildSettingsPayload(settings: ChatSettingsState) {
  return {
    provider_id: settings.providerId || undefined,
    integrations: {
      proxmox: {
        enabled: settings.proxmoxEnabled,
        cluster_id: settings.proxmoxEnabled ? settings.proxmoxClusterId || undefined : undefined,
      },
      kubernetes: {
        enabled: settings.kubernetesEnabled,
        cluster_id: settings.kubernetesEnabled ? settings.kubernetesClusterId || undefined : undefined,
      },
    },
  }
}

function mergeConversationSummary(
  previous: LLMConversationSummary[] | undefined,
  conversation: LLMConversationSummary,
): LLMConversationSummary[] {
  const existing = previous ?? []
  const next = [conversation, ...existing.filter((item) => item.id !== conversation.id)]
  next.sort((left, right) => new Date(right.last_message_at).getTime() - new Date(left.last_message_at).getTime())
  return next
}

function makeClientMessage(role: 'user' | 'assistant', content: string): LLMConversationMessage {
  return {
    id: `${role}-${Date.now()}-${Math.random().toString(16).slice(2)}`,
    role,
    content,
    created_at: new Date().toISOString(),
  }
}

function makeAssistantClientMessage(response: LLMChatResponse): LLMConversationMessage {
  return {
    ...makeClientMessage('assistant', response.content || ''),
    tool_calls: response.tool_calls,
    tool_results: response.tool_results,
    usage: response.usage,
  }
}

function enqueueStreamingDelta(
  delta: string,
  setStreamingAssistant: Dispatch<SetStateAction<string>>,
  queueRef: MutableRefObject<string[]>,
  timerRef: MutableRefObject<number | null>,
  resolversRef: MutableRefObject<Array<() => void>>,
) {
  const segments = splitStreamingDelta(delta)
  if (segments.length === 0) {
    return
  }

  queueRef.current.push(...segments)
  if (timerRef.current !== null) {
    return
  }

  drainStreamingDeltaQueue(setStreamingAssistant, queueRef, timerRef, resolversRef)
}

function drainStreamingDeltaQueue(
  setStreamingAssistant: Dispatch<SetStateAction<string>>,
  queueRef: MutableRefObject<string[]>,
  timerRef: MutableRefObject<number | null>,
  resolversRef: MutableRefObject<Array<() => void>>,
) {
  const next = queueRef.current.shift()
  if (next === undefined) {
    timerRef.current = null
    resolveStreamingPlaybackWaiters(resolversRef)
    return
  }

  setStreamingAssistant((previous) => previous + next)

  const remaining = queueRef.current.length
  const delay =
    remaining > 48 ? 0 :
    remaining > 24 ? 8 :
    remaining > 8 ? 14 :
    20

  timerRef.current = window.setTimeout(() => {
    drainStreamingDeltaQueue(setStreamingAssistant, queueRef, timerRef, resolversRef)
  }, delay)
}

function waitForStreamingPlayback(
  queueRef: MutableRefObject<string[]>,
  timerRef: MutableRefObject<number | null>,
  resolversRef: MutableRefObject<Array<() => void>>,
) {
  if (queueRef.current.length === 0 && timerRef.current === null) {
    return Promise.resolve()
  }

  return new Promise<void>((resolve) => {
    resolversRef.current.push(resolve)
  })
}

function resolveStreamingPlaybackWaiters(
  resolversRef: MutableRefObject<Array<() => void>>,
) {
  if (resolversRef.current.length === 0) {
    return
  }

  const resolvers = [...resolversRef.current]
  resolversRef.current = []
  for (const resolve of resolvers) {
    resolve()
  }
}

function resetStreamingPlayback(
  queueRef: MutableRefObject<string[]>,
  timerRef: MutableRefObject<number | null>,
  resolversRef: MutableRefObject<Array<() => void>>,
) {
  queueRef.current = []
  if (timerRef.current !== null) {
    window.clearTimeout(timerRef.current)
    timerRef.current = null
  }
  resolveStreamingPlaybackWaiters(resolversRef)
}

function splitStreamingDelta(delta: string): string[] {
  if (!delta) {
    return []
  }

  if (delta.length <= 24 && !delta.includes('\n')) {
    return [delta]
  }

  const parts = delta.match(/\S+\s*|\s+/g)
  if (!parts || parts.length === 0) {
    return [delta]
  }

  return parts.filter((part) => part.length > 0)
}

function isRateLimitError(error: unknown): boolean {
  return typeof error === 'object' && error !== null && 'status' in error && Number((error as { status?: unknown }).status) === 429
}

function isAbortError(error: unknown): boolean {
  return (
    error instanceof DOMException && error.name === 'AbortError'
  ) || (
    typeof error === 'object'
    && error !== null
    && 'name' in error
    && (error as { name?: unknown }).name === 'AbortError'
  )
}

function upsertStreamingToolActivity(
  previous: StreamingToolActivity[],
  toolCall?: LLMToolCall,
  toolResult?: LLMToolResult,
): StreamingToolActivity[] {
  const id = toolCall?.id ?? `${toolResult?.tool ?? 'tool'}-${previous.length + 1}`
  const nextStatus = toolResult ? (toolResult.error ? 'failed' : 'completed') : 'running'
  const existingIndex = previous.findIndex((item) => item.id === id)

  if (existingIndex === -1) {
    return [
      ...previous,
      {
        id,
        toolCall,
        result: toolResult,
        status: nextStatus,
      },
    ]
  }

  const next = [...previous]
  next[existingIndex] = {
    ...next[existingIndex],
    toolCall: toolCall ?? next[existingIndex].toolCall,
    result: toolResult ?? next[existingIndex].result,
    status: nextStatus,
  }
  return next
}

function formatToolArguments(rawArguments: string): string {
  try {
    return JSON.stringify(JSON.parse(rawArguments), null, 2)
  } catch {
    return rawArguments
  }
}

function lookupName(
  items: Array<{ id: string; name: string }>,
  id: string,
  fallback: string,
): string {
  return items.find((item) => item.id === id)?.name ?? fallback
}

function formatSidebarTimestamp(value: string): string {
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) {
    return formatDate(value)
  }

  const diffMs = Date.now() - parsed.getTime()
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60))
  if (diffHours < 1) {
    const diffMinutes = Math.max(1, Math.floor(diffMs / (1000 * 60)))
    return `${diffMinutes}m ago`
  }
  if (diffHours < 24) {
    return `${diffHours}h ago`
  }
  if (diffHours < 24 * 7) {
    return `${Math.floor(diffHours / 24)}d ago`
  }
  return formatDate(value)
}

function formatMessageTimestamp(value: string): string {
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) {
    return formatDate(value)
  }

  return parsed.toLocaleTimeString('en-US', {
    hour: '2-digit',
    minute: '2-digit',
  })
}

function formatCompactTokens(value: number): string {
  if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(1)}m`
  }
  if (value >= 1000) {
    return `${(value / 1000).toFixed(1)}k`
  }
  return `${value}`
}

function syncComposerHeight(textarea: HTMLTextAreaElement | null) {
  if (!textarea) {
    return
  }
  textarea.style.height = '0px'
  textarea.style.height = `${Math.min(textarea.scrollHeight, 224)}px`
}
