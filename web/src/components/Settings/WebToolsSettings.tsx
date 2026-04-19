import { useEffect, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'

import { api } from '../../api/client'
import { Card, CardContent } from '../ui/Card'
import Button from '../ui/Button'
import Input from '../ui/Input'
import Select from '../ui/Select'
import Badge from '../ui/Badge'
import Skeleton from '../ui/Skeleton'
import { Label } from '../ui/Form'
import { useUIStore } from '../../store/ui'
import type { SecretMetadata, WebPageObservationMode, WebSearchProvider, WebToolsConfig } from '../../types'

type WebToolsFormState = {
  search_provider: WebSearchProvider
  page_observation_mode: WebPageObservationMode
  searxng_base_url: string
  jina_search_base_url: string
  jina_reader_base_url: string
  jina_api_key_secret_name: string
}

function normalizeConfig(config: WebToolsConfig): WebToolsConfig {
  return {
    search_provider: config.search_provider || 'disabled',
    page_observation_mode: config.page_observation_mode || 'http',
    searxng_base_url: config.searxng_base_url || 'http://localhost:8080',
    jina_search_base_url: config.jina_search_base_url || 'https://s.jina.ai',
    jina_reader_base_url: config.jina_reader_base_url || 'https://r.jina.ai',
    jina_api_key_secret_name: config.jina_api_key_secret_name || '',
    search_ready: Boolean(config.search_ready),
    page_read_ready: config.page_read_ready ?? true,
    warnings: Array.isArray(config.warnings) ? config.warnings : [],
  }
}

function configToForm(config: WebToolsConfig): WebToolsFormState {
  const normalized = normalizeConfig(config)
  return {
    search_provider: normalized.search_provider,
    page_observation_mode: normalized.page_observation_mode,
    searxng_base_url: normalized.searxng_base_url,
    jina_search_base_url: normalized.jina_search_base_url,
    jina_reader_base_url: normalized.jina_reader_base_url,
    jina_api_key_secret_name: normalized.jina_api_key_secret_name || '',
  }
}

export default function WebToolsSettings() {
  const { addToast } = useUIStore()
  const [form, setForm] = useState<WebToolsFormState | null>(null)

  const configQuery = useQuery<WebToolsConfig>({
    queryKey: ['web-tools-config'],
    queryFn: () => api.webTools.getConfig(),
  })

  const secretsQuery = useQuery<SecretMetadata[]>({
    queryKey: ['secrets'],
    queryFn: () => api.secrets.list(),
  })

  useEffect(() => {
    if (!configQuery.data) {
      return
    }
    setForm(configToForm(configQuery.data))
  }, [configQuery.data])

  const saveMutation = useMutation({
    mutationFn: (payload: WebToolsFormState) => api.webTools.updateConfig(payload),
    onSuccess: (updated) => {
      setForm(configToForm(updated))
      configQuery.refetch()
      addToast({ type: 'success', title: 'Web tools settings saved' })
    },
    onError: (error) => {
      addToast({
        type: 'error',
        title: 'Failed to save web tools settings',
        message: error instanceof Error ? error.message : 'Unknown error',
      })
    },
  })

  if (configQuery.isError) {
    return (
      <Card>
        <CardContent className="pt-6">
          <p className="text-sm text-red-300">
            {configQuery.error instanceof Error ? configQuery.error.message : 'Failed to load web tools settings.'}
          </p>
        </CardContent>
      </Card>
    )
  }

  if (configQuery.isLoading || !form) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-64 w-full" />
      </div>
    )
  }

  const savedConfig = normalizeConfig(configQuery.data)
  const warnings = savedConfig.warnings
  const secrets = secretsQuery.data || []
  const isSaving = saveMutation.isPending
  const showSearXNGFields = form.search_provider === 'searxng'
  const showJinaSearchFields = form.search_provider === 'jina'
  const showJinaReaderFields = form.page_observation_mode === 'jina'
  const showSharedJinaFields = showJinaSearchFields || showJinaReaderFields

  function handleSubmit(event: React.FormEvent) {
    event.preventDefault()
    saveMutation.mutate(form)
  }

  function handleReset() {
    if (!savedConfig) {
      return
    }
    setForm(configToForm(savedConfig))
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold text-text">Web Tools</h2>
        <p className="mt-1 text-sm text-text-muted">
          Configure the chat assistant’s web search and page-reading tools. Search can use SearXNG or Jina, while page reading can use direct HTTP fetching or Jina Reader.
        </p>
      </div>

      <Card>
        <CardContent className="space-y-4 pt-6">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant={savedConfig.search_ready ? 'success' : 'warning'}>
              {savedConfig.search_ready ? 'Search ready' : 'Search needs setup'}
            </Badge>
            <Badge variant={savedConfig.page_read_ready ? 'success' : 'warning'}>
              {savedConfig.page_read_ready ? 'Page reader ready' : 'Page reader needs setup'}
            </Badge>
          </div>

          {warnings.length > 0 && (
            <div className="rounded-xl border border-amber-600/30 bg-amber-600/10 px-4 py-3">
              <h3 className="text-sm font-semibold text-amber-300">Configuration notes</h3>
              <div className="mt-2 space-y-2 text-sm text-amber-100/90">
                {warnings.map((warning) => (
                  <p key={warning}>{warning}</p>
                ))}
              </div>
            </div>
          )}

          <form onSubmit={handleSubmit} className="space-y-5">
            <div className="grid gap-4 md:grid-cols-2">
              <div>
                <Label>Search Provider</Label>
                <Select
                  value={form.search_provider}
                  onChange={(event) => setForm((current) => current ? { ...current, search_provider: event.target.value as WebSearchProvider } : current)}
                >
                  <option value="disabled">Disabled</option>
                  <option value="searxng">SearXNG</option>
                  <option value="jina">Jina AI</option>
                </Select>
                <p className="mt-1 text-xs text-text-dimmed">
                  Disable web search entirely, point it at a SearXNG instance, or use Jina search with an API key secret.
                </p>
              </div>

              <div>
                <Label>Page Observation Mode</Label>
                <Select
                  value={form.page_observation_mode}
                  onChange={(event) => setForm((current) => current ? { ...current, page_observation_mode: event.target.value as WebPageObservationMode } : current)}
                >
                  <option value="http">Direct HTTP fetch</option>
                  <option value="jina">Jina Reader</option>
                </Select>
                <p className="mt-1 text-xs text-text-dimmed">
                  Direct HTTP is local and simple; Jina Reader returns cleaner, LLM-friendly page content.
                </p>
              </div>

              {showSearXNGFields && (
                <div className="md:col-span-2">
                  <Label>SearXNG Base URL</Label>
                  <Input
                    value={form.searxng_base_url}
                    onChange={(event) => setForm((current) => current ? { ...current, searxng_base_url: event.target.value } : current)}
                    placeholder="http://localhost:8080"
                  />
                  <p className="mt-1 text-xs text-text-dimmed">
                    Point this at your SearXNG root or search endpoint. Emerald will call the JSON search API for you.
                  </p>
                </div>
              )}

              {showSharedJinaFields && (
                <div className="md:col-span-2 rounded-xl border border-border bg-bg-overlay/40 p-4">
                  <div>
                    <h3 className="text-sm font-semibold text-text">Jina Settings</h3>
                    <p className="mt-1 text-xs text-text-dimmed">
                      Jina search currently requires an API key. Create a secret in the Secrets section, then select it here.
                    </p>
                  </div>

                  <div className="mt-4 grid gap-4 md:grid-cols-2">
                    {showJinaSearchFields && (
                      <div className="md:col-span-2">
                        <Label>Jina Search Base URL</Label>
                        <Input
                          value={form.jina_search_base_url}
                          onChange={(event) => setForm((current) => current ? { ...current, jina_search_base_url: event.target.value } : current)}
                          placeholder="https://s.jina.ai"
                        />
                      </div>
                    )}

                    {showJinaReaderFields && (
                      <div className="md:col-span-2">
                        <Label>Jina Reader Base URL</Label>
                        <Input
                          value={form.jina_reader_base_url}
                          onChange={(event) => setForm((current) => current ? { ...current, jina_reader_base_url: event.target.value } : current)}
                          placeholder="https://r.jina.ai"
                        />
                      </div>
                    )}

                    <div className="md:col-span-2">
                      <Label>Jina API Key Secret</Label>
                      <Select
                        value={form.jina_api_key_secret_name}
                        onChange={(event) => setForm((current) => current ? { ...current, jina_api_key_secret_name: event.target.value } : current)}
                      >
                        <option value="">No secret selected</option>
                        {secrets.map((secret) => (
                          <option key={secret.id} value={secret.name}>
                            {secret.name}
                          </option>
                        ))}
                      </Select>
                      <p className="mt-1 text-xs text-text-dimmed">
                        Jina Reader can work without a key, but Jina search needs one. The same secret is reused for both endpoints.
                      </p>
                    </div>
                  </div>
                </div>
              )}
            </div>

            <div className="flex gap-3">
              <Button type="submit" loading={isSaving}>
                Save Web Tools
              </Button>
              <Button type="button" variant="secondary" onClick={handleReset} disabled={isSaving}>
                Reset
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
