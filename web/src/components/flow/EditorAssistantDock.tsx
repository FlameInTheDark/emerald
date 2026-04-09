import { useEffect, useRef, useState } from 'react'
import { Panel } from '@xyflow/react'
import { Brain, Check, ChevronDown, Loader2, Send, Square, Trash2, Wrench, X } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

import { api } from '../../api/client'
import { cn } from '../../lib/utils'
import { useUIStore } from '../../store/ui'
import type {
  EditorAssistantMessage,
  EditorAssistantMode,
  EditorAssistantPipelineSnapshot,
  EditorAssistantResponse,
  EditorAssistantSelection,
  LLMProvider,
  LLMToolCall,
  LLMToolResult,
  LLMUsage,
  LivePipelineOperation,
} from '../../types'

type QuickPickerType = 'model' | null

type StreamingToolActivity = {
  id: string
  name: string
  status: 'running' | 'completed' | 'failed'
}

const assistantMarkdownClassName = cn(
  'prose prose-invert max-w-none overflow-hidden break-words text-sm',
  'prose-p:my-2 prose-p:break-words prose-li:break-words prose-headings:break-words',
  'prose-a:break-all prose-code:break-all prose-code:text-sky-200',
  'prose-pre:max-w-full prose-pre:overflow-x-auto prose-pre:rounded-xl prose-pre:border prose-pre:border-border prose-pre:bg-bg',
)

type EditorAssistantDockProps = {
  pipelineKey: string
  pipeline: EditorAssistantPipelineSnapshot
  selection: EditorAssistantSelection
  providers: LLMProvider[]
  onApplyOperations: (operations: LivePipelineOperation[]) => Promise<void> | void
  onEditLockChange: (locked: boolean) => void
}

export default function EditorAssistantDock({
  pipelineKey,
  pipeline,
  selection,
  providers,
  onApplyOperations,
  onEditLockChange,
}: EditorAssistantDockProps) {
  const { addToast } = useUIStore()

  const [open, setOpen] = useState(false)
  const [mode, setMode] = useState<EditorAssistantMode>('ask')
  const [providerId, setProviderId] = useState('')
  const [messages, setMessages] = useState<EditorAssistantMessage[]>([])
  const [input, setInput] = useState('')
  const [isSending, setIsSending] = useState(false)
  const [streamingAssistant, setStreamingAssistant] = useState('')
  const [streamingUsage, setStreamingUsage] = useState<LLMUsage | null>(null)
  const [streamingToolActivity, setStreamingToolActivity] = useState<StreamingToolActivity[]>([])
  const [openQuickPicker, setOpenQuickPicker] = useState<QuickPickerType>(null)

  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const quickPickerRef = useRef<HTMLDivElement>(null)
  const streamAbortRef = useRef<AbortController | null>(null)

  const canSend = Boolean(input.trim()) && Boolean(providerId) && !isSending
  const canClearConversation = !isSending && (
    messages.length > 0 ||
    Boolean(input.trim()) ||
    Boolean(streamingAssistant.trim()) ||
    streamingToolActivity.length > 0 ||
    Boolean(streamingUsage)
  )
  const providerLabel = providers.find((provider) => provider.id === providerId)?.name ?? 'Choose model'

  useEffect(() => {
    if (providerId || providers.length === 0) {
      return
    }
    setProviderId(providers.find((provider) => provider.is_default)?.id ?? providers[0]?.id ?? '')
  }, [providerId, providers])

  useEffect(() => {
    streamAbortRef.current?.abort()
    setMessages([])
    setInput('')
    setStreamingAssistant('')
    setStreamingUsage(null)
    setStreamingToolActivity([])
    setOpenQuickPicker(null)
    setIsSending(false)
    setOpen(false)
  }, [pipelineKey])

  useEffect(() => {
    return () => {
      streamAbortRef.current?.abort()
      onEditLockChange(false)
    }
  }, [onEditLockChange])

  useEffect(() => {
    syncComposerHeight(textareaRef.current)
  }, [input, open])

  useEffect(() => {
    if (!open) {
      setOpenQuickPicker(null)
      return
    }

    textareaRef.current?.focus()
  }, [open])

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

  async function handleSend() {
    const message = input.trim()
    if (!message || !providerId || isSending) {
      return
    }

    const requestMode = mode
    const controller = new AbortController()
    streamAbortRef.current = controller
    setIsSending(true)
    setStreamingAssistant('')
    setStreamingUsage(null)
    setStreamingToolActivity([])
    setInput('')
    setOpenQuickPicker(null)
    if (requestMode === 'edit') {
      onEditLockChange(true)
    }

    try {
      const response = await api.llm.editorAssistantStream({
        provider_id: providerId,
        mode: requestMode,
        message,
        messages,
        pipeline,
        selection,
      }, {
        onEvent: (event) => {
          switch (event.type) {
            case 'assistant_delta':
              setStreamingAssistant((current) => current + event.delta)
              break
            case 'tool_started':
              setStreamingToolActivity((current) => upsertStreamingToolActivity(current, event.tool_call, undefined))
              break
            case 'tool_finished':
              setStreamingToolActivity((current) => upsertStreamingToolActivity(current, event.tool_call, event.tool_result))
              break
            case 'usage':
              setStreamingUsage(event.usage ?? null)
              break
          }
        },
        signal: controller.signal,
      })

      await applyAssistantResponse(response, requestMode)
      setMessages((current) => current.concat(
        { role: 'user', content: message },
        {
          role: 'assistant',
          content: response.content,
          tool_calls: response.tool_calls,
          tool_results: response.tool_results,
        },
      ))
      setStreamingAssistant('')
      setStreamingUsage(null)
      setStreamingToolActivity([])
    } catch (error) {
      setInput(message)
      if (!isAbortError(error)) {
        addToast({
          type: 'error',
          title: 'Assistant request failed',
          message: error instanceof Error ? error.message : 'Unknown error',
        })
      }
    } finally {
      if (streamAbortRef.current === controller) {
        streamAbortRef.current = null
      }
      if (requestMode === 'edit') {
        onEditLockChange(false)
      }
      setIsSending(false)
    }
  }

  async function applyAssistantResponse(response: EditorAssistantResponse, requestMode: EditorAssistantMode) {
    if (requestMode !== 'edit' || !response.operations || response.operations.length === 0) {
      return
    }

    await Promise.resolve(onApplyOperations(response.operations))
    addToast({
      type: 'success',
      title: 'Live edits applied',
      message: 'The assistant updated the canvas. Save the pipeline when you are ready.',
    })
  }

  function handleClearConversation() {
    if (!canClearConversation) {
      return
    }

    setMessages([])
    setInput('')
    setStreamingAssistant('')
    setStreamingUsage(null)
    setStreamingToolActivity([])
    setOpenQuickPicker(null)
    onEditLockChange(false)
    addToast({
      type: 'success',
      title: 'Conversation cleared',
      message: 'The live assistant session was reset for this pipeline.',
    })
  }

  function handleStopStreaming() {
    streamAbortRef.current?.abort()
  }

  return (
    <Panel position="bottom-center" className="!mb-4 pointer-events-none z-30">
      <div className="flex w-[min(32rem,calc(100vw-1rem))] max-w-full flex-col items-center gap-3 sm:w-[30rem]">
        {open && (
          <div className="pointer-events-auto w-full origin-bottom overflow-hidden rounded-[28px] border border-border/80 bg-bg-elevated/95 shadow-[0_24px_70px_rgba(0,0,0,0.45)] backdrop-blur-xl animate-dock-in">
            <div className="flex items-center justify-between gap-3 px-4 pb-3 pt-4">
              <div className="flex min-w-0 items-center gap-3">
                <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl border border-accent/20 bg-accent/10 text-accent">
                  <Brain className="h-4.5 w-4.5" />
                </span>
                <div className="min-w-0">
                  <h3 className="truncate text-sm font-semibold text-text">Node editor assistant</h3>
                  <p className="truncate text-xs text-text-dimmed">
                    {mode === 'ask'
                      ? 'Reads the current unsaved canvas.'
                      : 'Applies live unsaved changes to the canvas.'}
                  </p>
                </div>
              </div>

              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={handleClearConversation}
                  disabled={!canClearConversation}
                  className="inline-flex h-8 items-center gap-1.5 rounded-full border border-border bg-bg/70 px-2.5 text-[11px] font-medium text-text-muted transition-colors hover:border-accent/30 hover:text-text disabled:cursor-not-allowed disabled:opacity-50"
                  aria-label="Clear assistant conversation"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                  Clear
                </button>

                <button
                  type="button"
                  onClick={() => setOpen(false)}
                  disabled={isSending}
                  className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-border bg-bg/70 text-text-muted transition-colors hover:border-accent/30 hover:text-text"
                  aria-label="Close AI assistant"
                >
                  <X className="h-4 w-4" />
                </button>
              </div>
            </div>

            <div className="max-h-[24rem] min-h-[9rem] space-y-3 overflow-x-hidden overflow-y-auto px-4 pb-3">
              {messages.length === 0 && !isSending ? (
                <div className="flex min-h-[8.5rem] items-center justify-center px-4 py-2 text-center">
                  <div className="max-w-sm">
                    <p className="text-sm font-medium text-text">
                      {mode === 'ask'
                        ? 'Ask about what is already on the canvas.'
                        : 'Describe the change you want and I can apply it live.'}
                    </p>
                    <p className="mt-2 text-xs leading-5 text-text-dimmed">
                      {mode === 'ask'
                        ? 'I can inspect the current unsaved pipeline snapshot. If you want me to change nodes or edges, switch to Edit.'
                        : 'Changes stay in the browser until you save the pipeline.'}
                    </p>
                  </div>
                </div>
              ) : (
                <>
                  {messages.map((message, index) => (
                    <AssistantMessageBubble key={`${message.role}-${index}`} message={message} />
                  ))}

                  {isSending && (
                    <AssistantStreamingBubble
                      content={streamingAssistant}
                      toolActivity={streamingToolActivity}
                      usage={streamingUsage}
                    />
                  )}
                </>
              )}
            </div>

            {providers.length === 0 && (
              <div className="border-t border-border/70 px-4 py-3">
                <div className="rounded-2xl border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-300">
                  Configure an LLM provider in Settings before using the editor assistant.
                </div>
              </div>
            )}

            <div className="p-3 pt-0">
              <div className="rounded-[24px] border border-border bg-bg/70 shadow-inner shadow-black/10">
                <textarea
                  ref={textareaRef}
                  value={input}
                  onChange={(event) => setInput(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter' && !event.shiftKey) {
                      event.preventDefault()
                      void handleSend()
                    }
                  }}
                  disabled={isSending || providers.length === 0}
                  placeholder={mode === 'ask'
                    ? 'Ask about this pipeline...'
                    : 'Describe the live change you want...'}
                  className="min-h-[92px] max-h-56 w-full resize-none border-0 bg-transparent px-4 pb-2 pt-4 text-sm text-text placeholder:text-text-dimmed focus:outline-none"
                  rows={1}
                />

                <div className="flex flex-wrap items-end justify-between gap-3 border-t border-border/70 px-4 pb-4 pt-3">
                  <div ref={quickPickerRef} className="relative flex min-w-0 flex-1 flex-wrap items-center gap-2">
                    {openQuickPicker === 'model' && (
                      <AssistantQuickPickerPanel>
                        <AssistantQuickModelPicker
                          providers={providers}
                          selectedProviderId={providerId}
                          onSelect={(nextProviderId) => {
                            setProviderId(nextProviderId)
                            setOpenQuickPicker(null)
                          }}
                        />
                      </AssistantQuickPickerPanel>
                    )}

                    <QuickPickerButton
                      icon={Brain}
                      label={providerLabel}
                      active={openQuickPicker === 'model'}
                      disabled={providers.length === 0 || isSending}
                      onClick={() => setOpenQuickPicker((current) => (current === 'model' ? null : 'model'))}
                    />

                    <AssistantModeSwitch
                      mode={mode}
                      disabled={isSending}
                      onChange={setMode}
                    />

                    {isSending && streamingUsage && (
                      <span className="inline-flex h-8 items-center rounded-full border border-border bg-bg px-2.5 text-[11px] text-text-dimmed">
                        {streamingUsage.total_tokens} tokens
                      </span>
                    )}
                  </div>

                  <button
                    type="button"
                    onClick={() => {
                      if (isSending) {
                        handleStopStreaming()
                        return
                      }
                      void handleSend()
                    }}
                    disabled={isSending ? false : !canSend}
                    className={cn(
                      'inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border transition-colors',
                      isSending
                        ? 'border-red-500/40 bg-red-500/12 text-red-300 hover:bg-red-500/18'
                        : canSend
                          ? 'border-accent bg-accent text-white hover:bg-accent-hover'
                          : 'border-border bg-bg-input text-text-dimmed',
                    )}
                    aria-label={isSending ? 'Stop assistant response' : 'Send assistant message'}
                  >
                    {isSending ? <Square className="h-4 w-4 fill-current" /> : <Send className="h-4 w-4" />}
                  </button>
                </div>
              </div>
            </div>
          </div>
        )}

        <button
          type="button"
          onClick={() => setOpen((current) => !current)}
          disabled={isSending}
          className={cn(
            'pointer-events-auto flex h-14 w-14 items-center justify-center rounded-full border shadow-[0_18px_40px_rgba(0,0,0,0.28)] transition-[transform,background-color,border-color,color,box-shadow] duration-200 disabled:cursor-not-allowed disabled:opacity-70',
            open
              ? '-translate-y-0.5 scale-[1.03] border-accent/40 bg-accent text-white'
              : 'border-border bg-bg-elevated/95 text-accent backdrop-blur hover:border-accent/30 hover:bg-bg-overlay',
          )}
          aria-label={open ? 'Close AI assistant' : 'Open AI assistant'}
        >
          <Brain className="h-6 w-6" />
        </button>
      </div>
    </Panel>
  )
}

function AssistantStreamingBubble({
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
    <div className="flex justify-start">
      <div className="min-w-0 max-w-[88%] overflow-hidden rounded-2xl border border-border/80 bg-bg/65 px-4 py-3 text-sm text-text">
        {hasContent ? (
          <div className={assistantMarkdownClassName}>
            <ReactMarkdown remarkPlugins={[remarkGfm]}>
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
          <div className="mt-3 flex flex-wrap gap-2">
            {toolActivity.map((activity) => (
              <span
                key={activity.id}
                className={cn(
                  'inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] font-medium',
                  activity.status === 'running' && 'border-accent/30 bg-accent/10 text-accent',
                  activity.status === 'completed' && 'border-emerald-500/20 bg-emerald-500/10 text-emerald-300',
                  activity.status === 'failed' && 'border-red-500/20 bg-red-500/10 text-red-300',
                )}
              >
                <Wrench className="h-3 w-3" />
                <span className="truncate">{activity.name}</span>
              </span>
            ))}
          </div>
        )}

        <div className="mt-3 flex items-center gap-2 text-[11px] text-text-dimmed">
          <Brain className="h-3.5 w-3.5" />
          <span>Streaming response</span>
          {usage && usage.total_tokens > 0 && <span>{usage.total_tokens} tokens</span>}
        </div>
      </div>
    </div>
  )
}

function AssistantMessageBubble({ message }: { message: EditorAssistantMessage }) {
  const isUser = message.role === 'user'

  return (
    <div className={cn('flex', isUser ? 'justify-end' : 'justify-start')}>
      <div
        className={cn(
          'min-w-0 max-w-[88%] overflow-hidden rounded-2xl px-4 py-3 text-sm',
          isUser
            ? 'border border-accent/20 bg-accent/12 text-text'
            : 'border border-border/80 bg-bg/65 text-text',
        )}
      >
        {isUser ? (
          <p className="whitespace-pre-wrap break-words">{message.content}</p>
        ) : (
          <div className={assistantMarkdownClassName}>
            <ReactMarkdown remarkPlugins={[remarkGfm]}>
              {message.content}
            </ReactMarkdown>
          </div>
        )}

        {!isUser && message.tool_calls && message.tool_calls.length > 0 && (
          <div className="mt-3 flex flex-wrap gap-2">
            {message.tool_calls.map((toolCall) => (
              <span key={toolCall.id} className="rounded-full border border-border bg-bg px-2 py-0.5 text-[11px] text-text-dimmed">
                {toolCall.function.name}
              </span>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

function AssistantQuickPickerPanel({ children }: { children: React.ReactNode }) {
  return (
    <div className="absolute bottom-full left-0 z-20 mb-3 w-[min(22rem,calc(100vw-3rem))] origin-bottom-left overflow-hidden rounded-2xl border border-border bg-bg-elevated shadow-2xl shadow-black/25 animate-picker-in">
      {children}
    </div>
  )
}

function QuickPickerButton({
  icon: Icon,
  label,
  active,
  disabled,
  onClick,
}: {
  icon: React.ElementType
  label: string
  active?: boolean
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
      <Icon className="h-3.5 w-3.5 shrink-0 text-accent" />
      <span className="max-w-[8rem] truncate sm:max-w-[11rem]">{label}</span>
    </button>
  )
}

function AssistantQuickModelPicker({
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
        <p className="mt-1 text-xs text-text-dimmed">Select the model provider for this assistant.</p>
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

function AssistantModeSwitch({
  mode,
  disabled,
  onChange,
}: {
  mode: EditorAssistantMode
  disabled?: boolean
  onChange: (mode: EditorAssistantMode) => void
}) {
  return (
    <div className="inline-flex h-8 items-center rounded-full border border-border bg-bg p-0.5">
      {(['ask', 'edit'] as const).map((value) => (
        <button
          key={value}
          type="button"
          onClick={() => onChange(value)}
          disabled={disabled}
          className={cn(
            'rounded-full px-2.5 py-1 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50',
            mode === value
              ? 'bg-accent text-white'
              : 'text-text-muted hover:text-text',
          )}
        >
          {value === 'ask' ? 'Ask' : 'Edit'}
        </button>
      ))}
    </div>
  )
}

function upsertStreamingToolActivity(
  current: StreamingToolActivity[],
  toolCall: LLMToolCall | undefined,
  toolResult: LLMToolResult | undefined,
): StreamingToolActivity[] {
  if (!toolCall) {
    return current
  }

  const next = current.filter((activity) => activity.id !== toolCall.id)
  next.push({
    id: toolCall.id,
    name: toolCall.function.name,
    status: toolResult?.error ? 'failed' : toolResult ? 'completed' : 'running',
  })
  return next
}

function syncComposerHeight(textarea: HTMLTextAreaElement | null) {
  if (!textarea) {
    return
  }

  textarea.style.height = '0px'
  textarea.style.height = `${Math.min(textarea.scrollHeight, 224)}px`
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
