import { useDeferredValue, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Check, ChevronsUpDown, Loader2, RefreshCw, Search } from 'lucide-react'
import { api } from '../../api/client'
import { cn } from '../../lib/utils'
import type { LLMModelInfo } from '../../types'
import Button from '../ui/Button'
import Input from '../ui/Input'

interface ProviderModelSelectorProps {
  value: string
  providerType: string
  apiKey: string
  baseURL: string
  placeholder: string
  onChange: (value: string) => void
}

type DiscoveryStatus = 'idle' | 'loading' | 'ready' | 'error'

type MenuPosition = {
  left: number
  top: number
  width: number
}

const MENU_MIN_WIDTH = 340
const MENU_MAX_WIDTH = 560
const MENU_MARGIN = 12
const MENU_OFFSET = 8
const MENU_FALLBACK_HEIGHT = 420
const DISCOVERY_PROVIDER_TYPES = new Set(['openai', 'openrouter', 'ollama', 'lmstudio', 'custom'])

function normalizeQuery(value: string): string {
  return value.trim().toLowerCase()
}

function supportsModelDiscovery(providerType: string): boolean {
  return DISCOVERY_PROVIDER_TYPES.has(providerType)
}

function formatContextLength(value?: number): string | null {
  if (!value || value <= 0) {
    return null
  }

  if (value >= 1_000_000) {
    return `${Math.round(value / 100_000) / 10}M ctx`
  }

  if (value >= 1_000) {
    return `${Math.round(value / 100) / 10}k ctx`
  }

  return `${value} ctx`
}

function getDiscoveryBlockedMessage(providerType: string, baseURL: string): string | null {
  if (!supportsModelDiscovery(providerType)) {
    return 'Automatic model discovery is not available for this provider yet. You can still enter any model ID manually.'
  }

  if (providerType === 'custom' && !baseURL.trim()) {
    return 'Add a base URL first so we know where to discover models from.'
  }

  return null
}

function getModelSearchScore(model: LLMModelInfo, query: string): number {
  if (!query) {
    return 0
  }

  const id = normalizeQuery(model.id)
  const name = normalizeQuery(model.name ?? '')
  const description = normalizeQuery(model.description ?? '')

  if (id === query || name === query) {
    return 0
  }

  if (id.startsWith(query) || name.startsWith(query)) {
    return 1
  }

  if (id.includes(`/${query}`) || name.includes(` ${query}`)) {
    return 2
  }

  if (id.includes(query) || name.includes(query) || description.includes(query)) {
    return 3
  }

  return Number.POSITIVE_INFINITY
}

export default function ProviderModelSelector({
  value,
  providerType,
  apiKey,
  baseURL,
  placeholder,
  onChange,
}: ProviderModelSelectorProps) {
  const [isOpen, setIsOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [models, setModels] = useState<LLMModelInfo[]>([])
  const [status, setStatus] = useState<DiscoveryStatus>('idle')
  const [errorMessage, setErrorMessage] = useState('')
  const [menuPosition, setMenuPosition] = useState<MenuPosition>({
    left: 0,
    top: 0,
    width: MENU_MIN_WIDTH,
  })
  const containerRef = useRef<HTMLDivElement>(null)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const searchInputRef = useRef<HTMLInputElement>(null)
  const deferredQuery = useDeferredValue(query)
  const deferredDiscoveryState = useDeferredValue(`${providerType}\n${apiKey}\n${baseURL}\n${value}`)
  const selectedValue = value.trim()
  const normalizedSelectedValue = normalizeQuery(selectedValue)
  const discoveryBlockedMessage = getDiscoveryBlockedMessage(providerType, baseURL)
  const canDiscover = !discoveryBlockedMessage

  useEffect(() => {
    if (canDiscover) {
      return
    }

    setModels([])
    setStatus('idle')
    setErrorMessage('')
  }, [canDiscover])

  useEffect(() => {
    if (!isOpen) {
      return undefined
    }

    const handlePointerDown = (event: MouseEvent) => {
      const target = event.target as Node
      if (!containerRef.current?.contains(target) && !menuRef.current?.contains(target)) {
        setIsOpen(false)
      }
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setIsOpen(false)
        buttonRef.current?.focus()
      }
    }

    document.addEventListener('mousedown', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)

    return () => {
      document.removeEventListener('mousedown', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [isOpen])

  useEffect(() => {
    if (!isOpen || !searchInputRef.current) {
      return
    }

    searchInputRef.current.focus()
    searchInputRef.current.select()
  }, [isOpen])

  useLayoutEffect(() => {
    if (!isOpen) {
      return
    }

    const updatePosition = () => {
      const button = buttonRef.current
      if (!button) {
        return
      }

      const rect = button.getBoundingClientRect()
      const menuHeight = menuRef.current?.offsetHeight ?? MENU_FALLBACK_HEIGHT
      const viewportWidth = window.innerWidth
      const viewportHeight = window.innerHeight
      const desiredWidth = Math.min(Math.max(rect.width, MENU_MIN_WIDTH), MENU_MAX_WIDTH)
      const width = Math.min(desiredWidth, viewportWidth - (MENU_MARGIN * 2))
      const preferredLeft = rect.left
      const left = Math.max(
        MENU_MARGIN,
        Math.min(preferredLeft, viewportWidth - width - MENU_MARGIN),
      )
      const preferredTop = rect.bottom + MENU_OFFSET
      const shouldOpenUpward = preferredTop + menuHeight > viewportHeight - MENU_MARGIN
        && rect.top - MENU_OFFSET - menuHeight >= MENU_MARGIN
      const top = shouldOpenUpward
        ? Math.max(MENU_MARGIN, rect.top - menuHeight - MENU_OFFSET)
        : Math.max(
            MENU_MARGIN,
            Math.min(preferredTop, viewportHeight - menuHeight - MENU_MARGIN),
          )

      setMenuPosition({ left, top, width })
    }

    updatePosition()
    window.addEventListener('resize', updatePosition)
    window.addEventListener('scroll', updatePosition, true)

    return () => {
      window.removeEventListener('resize', updatePosition)
      window.removeEventListener('scroll', updatePosition, true)
    }
  }, [isOpen, models, query, status, errorMessage, discoveryBlockedMessage])

  const discoverModels = async () => {
    if (!canDiscover) {
      setStatus('idle')
      setErrorMessage('')
      setModels([])
      return
    }

    setStatus('loading')
    setErrorMessage('')

    try {
      const nextModels = await api.llmProviders.discoverModels({
        provider_type: providerType,
        api_key: apiKey,
        base_url: baseURL,
        model: selectedValue,
      })

      setModels(nextModels)
      setStatus('ready')
    } catch (error) {
      setModels([])
      setStatus('error')
      setErrorMessage(error instanceof Error ? error.message : 'Failed to load models')
    }
  }

  useEffect(() => {
    if (!isOpen) {
      setQuery('')
    }

    if (!canDiscover || (!isOpen && !selectedValue)) {
      return
    }

    void discoverModels()
  }, [canDiscover, deferredDiscoveryState, isOpen, selectedValue])

  const filteredModels = useMemo(() => {
    const normalizedQuery = normalizeQuery(deferredQuery)
    if (!normalizedQuery) {
      return models
    }

    return [...models]
      .map((model) => ({ model, score: getModelSearchScore(model, normalizedQuery) }))
      .filter((entry) => Number.isFinite(entry.score))
      .sort((a, b) => {
        if (a.score !== b.score) {
          return a.score - b.score
        }

        return a.model.id.localeCompare(b.model.id)
      })
      .map((entry) => entry.model)
  }, [deferredQuery, models])

  const exactMatch = useMemo(() => {
    const normalizedQuery = normalizeQuery(query)
    if (!normalizedQuery) {
      return null
    }

    return models.find((model) => {
      const normalizedID = normalizeQuery(model.id)
      const normalizedName = normalizeQuery(model.name ?? '')
      return normalizedID === normalizedQuery || normalizedName === normalizedQuery
    }) ?? null
  }, [models, query])

  const selectedDiscoveredModel = useMemo(() => (
    models.find((model) => normalizeQuery(model.id) === normalizedSelectedValue) ?? null
  ), [models, normalizedSelectedValue])
  const selectedContextLength = formatContextLength(selectedDiscoveredModel?.context_length)

  const trimmedQuery = query.trim()
  const showCustomOption = trimmedQuery.length > 0 && !exactMatch && normalizeQuery(trimmedQuery) !== normalizedSelectedValue
  const helperText = selectedValue
    ? selectedDiscoveredModel?.name && selectedDiscoveredModel.name !== selectedValue
      ? selectedDiscoveredModel.id
      : selectedContextLength
        ? 'Live context window discovered from this provider.'
        : 'Search discovered models or use a custom model ID.'
    : supportsModelDiscovery(providerType)
      ? 'Search discovered models or use a custom model ID.'
      : 'Enter a model ID manually.'

  const selectValue = (nextValue: string) => {
    onChange(nextValue)
    setIsOpen(false)
    setQuery('')
  }

  const menu = isOpen && typeof document !== 'undefined'
    ? createPortal(
        <div
          ref={menuRef}
          className="fixed z-[120] overflow-hidden rounded-2xl border border-border bg-bg-elevated shadow-2xl"
          style={{
            left: menuPosition.left,
            top: menuPosition.top,
            width: menuPosition.width,
          }}
        >
          <div className="border-b border-border px-3 py-3">
            <div className="flex items-center gap-2">
              <div className="relative flex-1">
                <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-dimmed" />
                <Input
                  ref={searchInputRef}
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key !== 'Enter' || !trimmedQuery) {
                      return
                    }

                    event.preventDefault()
                    selectValue(exactMatch?.id ?? trimmedQuery)
                  }}
                  placeholder="Search models or paste a custom ID..."
                  className="py-2.5 pl-9 pr-3 text-sm"
                />
              </div>
              {supportsModelDiscovery(providerType) && (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => void discoverModels()}
                  loading={status === 'loading'}
                  className="shrink-0"
                  title="Reload models"
                >
                  {status === 'loading' ? null : <RefreshCw className="h-4 w-4" />}
                </Button>
              )}
            </div>
            <div className="mt-2 flex items-center justify-between gap-2 text-[11px] text-text-dimmed">
              <span className="min-w-0 truncate">
                {discoveryBlockedMessage
                  ?? (status === 'ready' ? `${models.length} models discovered for this provider.` : 'Discover models from the current provider settings.')}
              </span>
            </div>
          </div>
          <div className="max-h-96 overflow-y-auto p-2">
            {showCustomOption && (
              <button
                type="button"
                onClick={() => selectValue(trimmedQuery)}
                className="mb-1 flex w-full items-start justify-between gap-3 rounded-xl border border-accent/25 bg-accent/10 px-3 py-3 text-left transition-colors hover:border-accent/40 hover:bg-accent/15"
              >
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium text-text">Use custom model ID</p>
                  <p className="mt-1 truncate font-mono text-xs text-accent">{trimmedQuery}</p>
                </div>
                <span className="rounded-full bg-accent/15 px-2 py-1 text-[10px] font-semibold uppercase tracking-[0.14em] text-accent">
                  Custom
                </span>
              </button>
            )}

            {status === 'loading' ? (
              <div className="flex items-center justify-center gap-2 px-4 py-10 text-sm text-text-muted">
                <Loader2 className="h-4 w-4 animate-spin" />
                Loading models...
              </div>
            ) : status === 'error' ? (
              <div className="rounded-xl border border-amber-500/20 bg-amber-500/10 px-4 py-4">
                <p className="text-sm font-medium text-text">Couldn&apos;t discover models</p>
                <p className="mt-1 text-xs text-text-dimmed">{errorMessage}</p>
              </div>
            ) : filteredModels.length > 0 ? (
              filteredModels.map((model) => {
                const selected = normalizeQuery(model.id) === normalizedSelectedValue
                const contextLength = formatContextLength(model.context_length)

                return (
                  <button
                    key={model.id}
                    type="button"
                    onClick={() => selectValue(model.id)}
                    className={cn(
                      'mb-1 flex w-full items-start justify-between gap-3 rounded-xl px-3 py-3 text-left transition-colors last:mb-0',
                      selected
                        ? 'bg-accent/10 text-text'
                        : 'text-text-muted hover:bg-bg-overlay hover:text-text',
                    )}
                  >
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <p className="truncate text-sm font-medium text-text">{model.name || model.id}</p>
                        {contextLength && (
                          <span className="rounded-full border border-border/80 bg-bg px-2 py-0.5 text-[10px] font-medium uppercase tracking-[0.12em] text-text-dimmed">
                            {contextLength}
                          </span>
                        )}
                      </div>
                      {model.name && model.name !== model.id && (
                        <p className="mt-1 truncate font-mono text-[11px] text-text-dimmed">{model.id}</p>
                      )}
                      {model.description && (
                        <p className="mt-1 truncate text-xs text-text-dimmed">{model.description}</p>
                      )}
                    </div>
                    <Check className={cn('mt-0.5 h-4 w-4 shrink-0', selected ? 'text-accent' : 'text-transparent')} />
                  </button>
                )
              })
            ) : (
              <div className="px-4 py-10 text-center text-sm text-text-muted">
                {trimmedQuery
                  ? 'No discovered models match your search. Press Enter to use the typed value.'
                  : discoveryBlockedMessage ?? 'No models were returned for this provider.'}
              </div>
            )}
          </div>
        </div>,
        document.body,
      )
    : null

  return (
    <div ref={containerRef} className="relative">
      <button
        ref={buttonRef}
        type="button"
        onClick={() => setIsOpen((current) => !current)}
        aria-expanded={isOpen}
        className={cn(
          'flex min-h-[3.75rem] w-full items-center justify-between gap-3 rounded-xl border border-border bg-bg-input px-3 py-3 text-left transition-colors',
          isOpen ? 'border-accent/50 ring-2 ring-accent/20' : 'hover:border-accent/30',
        )}
      >
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <p className={cn(
              'truncate text-sm font-medium',
              selectedValue ? 'text-text' : 'text-text-dimmed',
            )}>
              {selectedDiscoveredModel?.name || selectedValue || placeholder}
            </p>
            {selectedContextLength && (
              <span className="shrink-0 rounded-full border border-border/80 bg-bg px-2 py-0.5 text-[10px] font-medium uppercase tracking-[0.12em] text-text-dimmed">
                {selectedContextLength}
              </span>
            )}
          </div>
          <p className="mt-1 truncate text-xs text-text-dimmed">{helperText}</p>
        </div>
        <ChevronsUpDown className="h-4 w-4 shrink-0 text-text-dimmed" />
      </button>
      {menu}
    </div>
  )
}
