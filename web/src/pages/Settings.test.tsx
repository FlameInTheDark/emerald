import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import Settings from './Settings'

const { mockApi, mockUseNodeDefinitions, mockUseAuthSession } = vi.hoisted(() => ({
  mockApi: {
    clusters: {
      list: vi.fn(),
    },
    channels: {
      list: vi.fn(),
    },
    llmProviders: {
      list: vi.fn(),
    },
    webTools: {
      getConfig: vi.fn(),
      updateConfig: vi.fn(),
    },
    secrets: {
      list: vi.fn(),
    },
    users: {
      list: vi.fn(),
    },
  },
  mockUseNodeDefinitions: vi.fn(),
  mockUseAuthSession: vi.fn(),
}))

const queryClients: QueryClient[] = []

vi.mock('../api/client', () => ({
  api: mockApi,
}))

vi.mock('../hooks/useNodeDefinitions', () => ({
  useNodeDefinitions: mockUseNodeDefinitions,
}))

vi.mock('../lib/auth', () => ({
  AUTH_SESSION_QUERY_KEY: ['auth-session'],
  useAuthSession: mockUseAuthSession,
}))

vi.mock('../components/Settings/KubernetesClusterSettings', () => ({
  default: () => <div>Kubernetes Section Mock</div>,
}))

vi.mock('../components/Settings/AssistantProfilesSettings', () => ({
  default: () => <div>Assistant Profiles Mock</div>,
}))

vi.mock('../components/Settings/ProviderModelSelector', () => ({
  default: ({ value, onChange }: { value?: string; onChange: (value: string) => void }) => (
    <input
      aria-label="Provider model"
      value={value ?? ''}
      onChange={(event) => onChange(event.target.value)}
    />
  ),
}))

function LocationProbe() {
  const location = useLocation()
  return <div data-testid="location">{`${location.pathname}${location.search}`}</div>
}

function renderSettings(initialEntry = '/settings') {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  })
  queryClients.push(queryClient)

  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          <Route
            path="/settings"
            element={(
              <>
                <Settings />
                <LocationProbe />
              </>
            )}
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('Settings page', () => {
  beforeEach(() => {
    vi.clearAllMocks()

    mockApi.clusters.list.mockResolvedValue([])
    mockApi.channels.list.mockResolvedValue([])
    mockApi.llmProviders.list.mockResolvedValue([])
    mockApi.webTools.getConfig.mockResolvedValue({
      search_provider: 'disabled',
      page_observation_mode: 'http',
      searxng_base_url: 'http://localhost:8080',
      jina_search_base_url: 'https://s.jina.ai',
      jina_reader_base_url: 'https://r.jina.ai',
      search_ready: false,
      page_read_ready: true,
      warnings: [],
    })
    mockApi.secrets.list.mockResolvedValue([])
    mockApi.users.list.mockResolvedValue([])

    mockUseNodeDefinitions.mockReturnValue({
      plugins: [
        {
          id: 'plugin-1',
          name: 'Acme Toolkit',
          version: '1.0.0',
          description: 'Adds Acme nodes',
          path: '.agents/plugins/acme',
          healthy: true,
          node_count: 2,
        },
      ],
      isLoading: false,
    })

    mockUseAuthSession.mockReturnValue({
      data: {
        authenticated: true,
        username: 'admin',
        expires_at: '2026-04-11T00:00:00Z',
      },
      isLoading: false,
    })
  })

  afterEach(() => {
    for (const queryClient of queryClients) {
      queryClient.clear()
    }
    queryClients.length = 0
    cleanup()
  })

  it('defaults to the proxmox section and normalizes the URL query', async () => {
    renderSettings()

    expect(await screen.findByText('Proxmox Clusters')).toBeInTheDocument()
    await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/settings?section=proxmox'))
    expect(screen.getByRole('button', { name: 'Proxmox' })).toHaveAttribute('aria-current', 'page')
  })

  it('supports deep links and AI subcategory routing', async () => {
    const user = userEvent.setup()
    renderSettings('/settings?section=ai.providers')

    expect(await screen.findByText('LLM Providers')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Providers' })).toHaveAttribute('aria-current', 'page')

    await user.click(screen.getByRole('button', { name: 'Assistants' }))

    expect(await screen.findByText('Assistant Profiles Mock')).toBeInTheDocument()
    await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/settings?section=ai.assistants'))
  })

  it('includes the web tools AI subsection', async () => {
    const user = userEvent.setup()
    renderSettings('/settings?section=ai.providers')

    await user.click(screen.getByRole('button', { name: 'Web Tools' }))

    expect(await screen.findByRole('heading', { name: 'Web Tools' })).toBeInTheDocument()
    expect(screen.getByText(/search can use SearXNG or Jina/i)).toBeInTheDocument()
    await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/settings?section=ai.web_tools'))
  })

  it('moves plugin health into its own section instead of secrets', async () => {
    const user = userEvent.setup()
    renderSettings('/settings?section=secrets')

    expect(await screen.findByText('Secret Vault')).toBeInTheDocument()
    expect(screen.queryByText('Installed Plugin Bundles')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Plugins' }))

    expect(await screen.findByText('Installed Plugin Bundles')).toBeInTheDocument()
    expect(screen.getByText('Acme Toolkit')).toBeInTheDocument()
    await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/settings?section=plugins'))
  })

  it('updates the active section from the mobile selector fallback', async () => {
    const user = userEvent.setup()
    renderSettings('/settings?section=proxmox')

    await user.selectOptions(screen.getByLabelText('Settings section'), 'channels')

    expect(await screen.findByText('No channels configured')).toBeInTheDocument()
    await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/settings?section=channels'))
  })
})
