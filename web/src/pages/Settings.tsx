import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Server, Shield, Brain, Bot, Plus, Trash2, Edit2, MessageSquare, Power, Users } from 'lucide-react'
import { api } from '../api/client'
import { Card, CardContent } from '../components/ui/Card'
import Button from '../components/ui/Button'
import Input from '../components/ui/Input'
import { Label, Checkbox, Textarea } from '../components/ui/Form'
import Badge from '../components/ui/Badge'
import Skeleton from '../components/ui/Skeleton'
import KubernetesClusterSettings from '../components/Settings/KubernetesClusterSettings'
import AssistantProfilesSettings from '../components/Settings/AssistantProfilesSettings'
import { useUIStore } from '../store/ui'
import { cn } from '../lib/utils'
import { AUTH_SESSION_QUERY_KEY, useAuthSession } from '../lib/auth'
import type { AuthSession, Channel, Cluster, LLMProvider, User } from '../types'

type ClusterFormState = {
  name: string
  host: string
  port: number
  api_token_id: string
  api_token_secret: string
  skip_tls_verify: boolean
}

type ProviderFormState = {
  name: string
  provider_type: string
  api_key: string
  base_url: string
  model: string
  is_default: boolean
}

type ChannelFormState = {
  name: string
  type: string
  bot_token: string
  welcome_message: string
  enabled: boolean
}

type UserFormState = {
  username: string
  password: string
}

type PasswordChangeFormState = {
  current_password: string
  new_password: string
  confirm_password: string
}

function getDefaultClusterForm(): ClusterFormState {
  return {
    name: '',
    host: '',
    port: 8006,
    api_token_id: '',
    api_token_secret: '',
    skip_tls_verify: false,
  }
}

function getProviderDefaultBaseURL(providerType: string): string {
  switch (providerType) {
    case 'openai':
      return 'https://api.openai.com/v1'
    case 'openrouter':
      return 'https://openrouter.ai/api/v1'
    case 'ollama':
      return 'http://localhost:11434'
    default:
      return ''
  }
}

function getProviderModelPlaceholder(providerType: string): string {
  switch (providerType) {
    case 'openrouter':
      return 'openai/gpt-4o-mini'
    case 'ollama':
      return 'llama3.2'
    default:
      return 'gpt-4o'
  }
}

function getDefaultProviderForm(): ProviderFormState {
  return {
    name: '',
    provider_type: 'openai',
    api_key: '',
    base_url: getProviderDefaultBaseURL('openai'),
    model: '',
    is_default: false,
  }
}

function getDefaultChannelForm(): ChannelFormState {
  return {
    name: '',
    type: 'telegram',
    bot_token: '',
    welcome_message: 'Welcome! Use this one-time code to connect this chat to Automator.',
    enabled: true,
  }
}

function getDefaultUserForm(): UserFormState {
  return {
    username: '',
    password: '',
  }
}

function getDefaultPasswordChangeForm(): PasswordChangeFormState {
  return {
    current_password: '',
    new_password: '',
    confirm_password: '',
  }
}

function getChannelConnectURL(): string | undefined {
  if (typeof window === 'undefined' || !window.location?.origin) {
    return undefined
  }

  return new URL('/channels/connect', window.location.origin).toString()
}

function clusterToForm(cluster: Cluster): ClusterFormState {
  return {
    name: cluster.name,
    host: cluster.host,
    port: cluster.port,
    api_token_id: cluster.api_token_id,
    api_token_secret: cluster.api_token_secret || '',
    skip_tls_verify: cluster.skip_tls_verify,
  }
}

function providerToForm(provider: LLMProvider): ProviderFormState {
  return {
    name: provider.name,
    provider_type: provider.provider_type,
    api_key: provider.api_key || '',
    base_url: provider.base_url || getProviderDefaultBaseURL(provider.provider_type),
    model: provider.model,
    is_default: provider.is_default,
  }
}

function parseChannelConfig(config?: string) {
  if (!config) return {}
  try {
    return JSON.parse(config) as { botToken?: string }
  } catch {
    return {}
  }
}

function channelToForm(channel: Channel): ChannelFormState {
  const config = parseChannelConfig(channel.config)
  return {
    name: channel.name,
    type: channel.type,
    bot_token: config.botToken || '',
    welcome_message: channel.welcome_message || getDefaultChannelForm().welcome_message,
    enabled: channel.enabled,
  }
}

export default function Settings() {
  const [activeTab, setActiveTab] = useState<'clusters' | 'kubernetes' | 'channels' | 'ai' | 'users'>('clusters')

  return (
    <div className="p-6 max-w-4xl mx-auto">
      <div className="mb-8">
        <h1 className="text-2xl font-bold text-text">Settings</h1>
        <p className="text-text-muted mt-1">Configure clusters, channels, AI, and workspace access</p>
      </div>

      <div className="mb-6 overflow-x-auto pb-1 -mb-1">
        <div className="inline-flex min-w-max gap-1 rounded-lg border border-border bg-bg-input p-1">
          {[
            { id: 'clusters' as const, label: 'Proxmox Clusters', icon: Server },
            { id: 'kubernetes' as const, label: 'Kubernetes Clusters', icon: Shield },
            { id: 'channels' as const, label: 'Channels', icon: MessageSquare },
            { id: 'ai' as const, label: 'AI', icon: Brain },
            { id: 'users' as const, label: 'Users', icon: Users },
          ].map(({ id, label, icon: Icon }) => (
            <button
              key={id}
              onClick={() => setActiveTab(id)}
              className={cn(
                'flex shrink-0 items-center gap-2 whitespace-nowrap rounded-md px-4 py-2 text-sm font-medium transition-all',
                activeTab === id
                  ? 'bg-bg-elevated text-text shadow-sm'
                  : 'text-text-muted hover:text-text',
              )}
            >
              <Icon className="w-4 h-4" />
              {label}
            </button>
          ))}
        </div>
      </div>

      {activeTab === 'clusters' && <ClusterSettings />}
      {activeTab === 'kubernetes' && <KubernetesClusterSettings />}
      {activeTab === 'channels' && <ChannelSettings />}
      {activeTab === 'ai' && <AISettings />}
      {activeTab === 'users' && <UserSettings />}
    </div>
  )
}

function AISettings() {
  const [activeTab, setActiveTab] = useState<'providers' | 'assistants'>('providers')

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold text-text">AI</h2>
        <p className="mt-1 text-sm text-text-muted">
          Manage provider connections and the default instruction profiles used by assistant experiences.
        </p>
      </div>

      <div className="overflow-x-auto pb-1 -mb-1">
        <div className="inline-flex min-w-max gap-1 rounded-xl border border-border bg-bg-input p-1">
          {[
            { id: 'providers' as const, label: 'Providers', icon: Brain },
            { id: 'assistants' as const, label: 'Assistants', icon: Bot },
          ].map(({ id, label, icon: Icon }) => (
            <button
              key={id}
              type="button"
              onClick={() => setActiveTab(id)}
              className={cn(
                'flex shrink-0 items-center gap-2 whitespace-nowrap rounded-lg px-4 py-2 text-sm font-medium transition-colors',
                activeTab === id
                  ? 'bg-bg-elevated text-text shadow-sm'
                  : 'text-text-muted hover:text-text',
              )}
            >
              <Icon className="h-4 w-4" />
              {label}
            </button>
          ))}
        </div>
      </div>

      {activeTab === 'providers' ? <LLMSettings /> : <AssistantProfilesSettings />}
    </div>
  )
}

function UserSettings() {
  const queryClient = useQueryClient()
  const { addToast } = useUIStore()
  const sessionQuery = useAuthSession()
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<UserFormState>(getDefaultUserForm())
  const [passwordForm, setPasswordForm] = useState<PasswordChangeFormState>(getDefaultPasswordChangeForm())

  const { data: users, isLoading } = useQuery<User[]>({
    queryKey: ['users'],
    queryFn: () => api.users.list(),
  })

  const createMutation = useMutation({
    mutationFn: () => api.users.create(form),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
      setForm(getDefaultUserForm())
      setShowForm(false)
      addToast({ type: 'success', title: 'User created' })
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to create user', message: err.message })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.users.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
      addToast({ type: 'success', title: 'User removed' })
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to remove user', message: err.message })
    },
  })

  const changePasswordMutation = useMutation({
    mutationFn: () => api.users.changePassword({
      current_password: passwordForm.current_password,
      new_password: passwordForm.new_password,
    }),
    onSuccess: (session) => {
      queryClient.setQueryData<AuthSession | null>(AUTH_SESSION_QUERY_KEY, session)
      setPasswordForm(getDefaultPasswordChangeForm())
      addToast({ type: 'success', title: 'Password updated' })
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to update password', message: err.message })
    },
  })

  function resetForm() {
    setForm(getDefaultUserForm())
    setShowForm(false)
  }

  function handlePasswordSubmit() {
    if (!passwordForm.current_password || !passwordForm.new_password || !passwordForm.confirm_password) {
      addToast({ type: 'warning', title: 'All password fields are required' })
      return
    }
    if (passwordForm.new_password !== passwordForm.confirm_password) {
      addToast({ type: 'warning', title: 'New passwords do not match' })
      return
    }
    if (passwordForm.current_password === passwordForm.new_password) {
      addToast({ type: 'warning', title: 'Choose a different new password' })
      return
    }

    changePasswordMutation.mutate()
  }

  function handleSubmit() {
    if (!form.username.trim() || !form.password) {
      addToast({ type: 'warning', title: 'Username and password are required' })
      return
    }

    createMutation.mutate()
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 className="text-lg font-semibold text-text">Users</h2>
          <p className="mt-1 text-sm text-text-muted">
            Manage users here, and update your own password without any public registration flow.
          </p>
        </div>
        <Button onClick={showForm ? resetForm : () => setShowForm(true)}>
          <Plus className="w-4 h-4" />
          {showForm ? 'Close' : 'Add User'}
        </Button>
      </div>

      <Card>
        <CardContent className="space-y-4 pt-6">
          <div>
            <h3 className="text-base font-semibold text-text">Change Your Password</h3>
            <p className="mt-1 text-sm text-text-muted">
              {sessionQuery.data?.username
                ? `Signed in as ${sessionQuery.data.username}.`
                : 'Update the password for your current account.'}
            </p>
          </div>
          <div className="grid gap-4 md:grid-cols-3">
            <div>
              <Label>Current Password</Label>
              <Input
                type="password"
                value={passwordForm.current_password}
                onChange={(event) => setPasswordForm((current) => ({ ...current, current_password: event.target.value }))}
                placeholder="Current password"
              />
            </div>
            <div>
              <Label>New Password</Label>
              <Input
                type="password"
                value={passwordForm.new_password}
                onChange={(event) => setPasswordForm((current) => ({ ...current, new_password: event.target.value }))}
                placeholder="New password"
              />
            </div>
            <div>
              <Label>Confirm New Password</Label>
              <Input
                type="password"
                value={passwordForm.confirm_password}
                onChange={(event) => setPasswordForm((current) => ({ ...current, confirm_password: event.target.value }))}
                placeholder="Repeat the new password"
              />
            </div>
          </div>
          <div className="flex gap-3">
            <Button onClick={handlePasswordSubmit} loading={changePasswordMutation.isPending}>
              Update Password
            </Button>
            <Button variant="secondary" onClick={() => setPasswordForm(getDefaultPasswordChangeForm())}>
              Reset
            </Button>
          </div>
        </CardContent>
      </Card>

      {showForm && (
        <Card>
          <CardContent className="space-y-4 pt-6">
            <div className="grid gap-4 md:grid-cols-2">
              <div>
                <Label>Username</Label>
                <Input
                  value={form.username}
                  onChange={(event) => setForm((current) => ({ ...current, username: event.target.value }))}
                  placeholder="operator"
                />
              </div>
              <div>
                <Label>Password</Label>
                <Input
                  type="password"
                  value={form.password}
                  onChange={(event) => setForm((current) => ({ ...current, password: event.target.value }))}
                  placeholder="Choose a password"
                />
              </div>
            </div>
            <div className="flex gap-3">
              <Button onClick={handleSubmit} loading={createMutation.isPending}>
                Create User
              </Button>
              <Button variant="secondary" onClick={resetForm}>
                Cancel
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      <div className="space-y-4">
        {isLoading && (
          <>
            <Skeleton className="h-24 w-full" />
            <Skeleton className="h-24 w-full" />
          </>
        )}

        {!isLoading && users?.map((user) => {
          const isCurrentUser = sessionQuery.data?.username === user.username
          return (
            <Card key={user.id}>
              <CardContent className="flex flex-col gap-4 py-5 sm:flex-row sm:items-center sm:justify-between">
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <h3 className="truncate text-base font-semibold text-text">{user.username}</h3>
                    {isCurrentUser && <Badge variant="info">Current</Badge>}
                    {user.username === 'admin' && <Badge variant="success">Default</Badge>}
                  </div>
                  <p className="mt-1 text-sm text-text-muted">
                    Created {new Date(user.created_at).toLocaleString()}
                  </p>
                </div>
                <Button
                  variant="danger"
                  onClick={() => deleteMutation.mutate(user.id)}
                  disabled={deleteMutation.isPending || isCurrentUser}
                  title={isCurrentUser ? 'You cannot delete your own account' : 'Delete user'}
                >
                  <Trash2 className="w-4 h-4" />
                  Delete
                </Button>
              </CardContent>
            </Card>
          )
        })}
      </div>
    </div>
  )
}

function ClusterSettings() {
  const queryClient = useQueryClient()
  const { addToast } = useUIStore()
  const [showForm, setShowForm] = useState(false)
  const [editingClusterId, setEditingClusterId] = useState<string | null>(null)
  const [loadingClusterId, setLoadingClusterId] = useState<string | null>(null)
  const [form, setForm] = useState<ClusterFormState>(getDefaultClusterForm())

  const { data: clusters, isLoading } = useQuery<Cluster[]>({
    queryKey: ['clusters'],
    queryFn: () => api.clusters.list(),
  })

  const createMutation = useMutation({
    mutationFn: () => api.clusters.create(form),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['clusters'] })
      resetForm()
      addToast({ type: 'success', title: 'Cluster added' })
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to add cluster', message: err.message })
    },
  })

  const updateMutation = useMutation({
    mutationFn: () => api.clusters.update(editingClusterId!, form),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['clusters'] })
      resetForm()
      addToast({ type: 'success', title: 'Cluster updated' })
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to update cluster', message: err.message })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.clusters.delete(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ['clusters'] })
      addToast({ type: 'success', title: 'Cluster removed' })
      if (editingClusterId === id) {
        resetForm()
      }
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to remove cluster', message: err.message })
    },
  })

  function resetForm() {
    setShowForm(false)
    setEditingClusterId(null)
    setLoadingClusterId(null)
    setForm(getDefaultClusterForm())
  }

  function startCreate() {
    setEditingClusterId(null)
    setForm(getDefaultClusterForm())
    setShowForm(true)
  }

  async function startEdit(id: string) {
    setLoadingClusterId(id)
    try {
      const cluster = await api.clusters.get(id)
      setEditingClusterId(id)
      setForm(clusterToForm(cluster))
      setShowForm(true)
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Failed to load cluster',
        message: err instanceof Error ? err.message : 'Unknown error',
      })
    } finally {
      setLoadingClusterId(null)
    }
  }

  const isSubmitting = createMutation.isPending || updateMutation.isPending

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (editingClusterId) {
      updateMutation.mutate()
      return
    }
    createMutation.mutate()
  }

  if (isLoading) {
    return (
      <div className="space-y-4">
        {[1, 2].map((i) => (
          <Card key={i}>
            <CardContent className="p-5">
              <Skeleton className="h-5 w-1/3 mb-2" />
              <Skeleton className="h-4 w-1/2" />
            </CardContent>
          </Card>
        ))}
      </div>
    )
  }

  return (
    <div>
      <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 className="text-lg font-semibold text-text">Proxmox Clusters</h2>
          <p className="mt-1 text-sm text-text-muted">
            Store the Proxmox connections used by your infrastructure nodes and tools.
          </p>
        </div>
        <Button onClick={showForm ? resetForm : startCreate}>
          <Plus className="w-4 h-4" />
          {showForm ? 'Close' : 'Add Cluster'}
        </Button>
      </div>

      {showForm && (
        <Card className="mb-6">
          <CardContent className="p-6">
            <div className="mb-4">
              <h2 className="text-lg font-semibold text-text">
                {editingClusterId ? 'Edit Cluster' : 'Add Cluster'}
              </h2>
              <p className="text-sm text-text-muted">
                {editingClusterId
                  ? 'Update the Proxmox cluster connection details.'
                  : 'Add a new Proxmox cluster connection.'}
              </p>
            </div>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div>
                  <Label>Name</Label>
                  <Input
                    value={form.name}
                    onChange={(e) => setForm({ ...form, name: e.target.value })}
                    placeholder="Production Cluster"
                    required
                  />
                </div>
                <div>
                  <Label>Host</Label>
                  <Input
                    value={form.host}
                    onChange={(e) => setForm({ ...form, host: e.target.value })}
                    placeholder="192.168.1.100"
                    required
                  />
                </div>
                <div>
                  <Label>Port</Label>
                  <Input
                    type="number"
                    value={form.port}
                    onChange={(e) => setForm({ ...form, port: parseInt(e.target.value) || 8006 })}
                  />
                </div>
                <div>
                  <Label>API Token ID</Label>
                  <Input
                    value={form.api_token_id}
                    onChange={(e) => setForm({ ...form, api_token_id: e.target.value })}
                    placeholder="root@pam!automator"
                    required
                  />
                </div>
                <div className="sm:col-span-2">
                  <Label>API Token Secret</Label>
                  <Input
                    type="password"
                    value={form.api_token_secret}
                    onChange={(e) => setForm({ ...form, api_token_secret: e.target.value })}
                    placeholder="Enter token secret"
                    required
                  />
                </div>
                <div className="sm:col-span-2 flex items-center gap-2">
                  <Checkbox
                    checked={form.skip_tls_verify}
                    onChange={(e) => setForm({ ...form, skip_tls_verify: e.target.checked })}
                  />
                  <span className="text-sm text-text-muted">Skip TLS Verification</span>
                </div>
              </div>
              <div className="flex gap-2">
                <Button type="submit" loading={isSubmitting}>
                  {editingClusterId ? 'Save Changes' : 'Save Cluster'}
                </Button>
                <Button type="button" variant="ghost" onClick={resetForm}>
                  Cancel
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      )}

      {clusters?.length === 0 ? (
        <Card>
          <CardContent className="p-12 text-center">
            <Server className="w-12 h-12 text-text-dimmed mx-auto mb-4" />
            <h3 className="text-lg font-medium text-text mb-2">No clusters configured</h3>
            <p className="text-text-muted">Add your first Proxmox cluster to get started</p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-3">
          {clusters?.map((cluster) => (
            <Card key={cluster.id} className="group">
              <CardContent className="p-5 flex items-center justify-between">
                <div className="flex items-center gap-4">
                  <div className="w-10 h-10 rounded-lg bg-accent/10 flex items-center justify-center">
                    <Server className="w-5 h-5 text-accent" />
                  </div>
                  <div>
                    <p className="text-sm font-medium text-text">{cluster.name}</p>
                    <p className="text-xs text-text-dimmed">{cluster.host}:{cluster.port}</p>
                    <p className="text-xs text-text-dimmed">Token: {cluster.api_token_id}</p>
                  </div>
                </div>
                <div className="flex items-center gap-3">
                  <Badge variant={cluster.skip_tls_verify ? 'warning' : 'success'}>
                    {cluster.skip_tls_verify ? 'TLS Skip' : 'TLS Enabled'}
                  </Badge>
                  <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => void startEdit(cluster.id)}
                      disabled={loadingClusterId === cluster.id || deleteMutation.isPending}
                      title="Edit cluster"
                    >
                      <Edit2 className="w-4 h-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => deleteMutation.mutate(cluster.id)}
                      disabled={loadingClusterId === cluster.id || deleteMutation.isPending}
                      title="Delete cluster"
                    >
                      <Trash2 className="w-4 h-4 text-red-400" />
                    </Button>
                  </div>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  )
}

function ChannelSettings() {
  const queryClient = useQueryClient()
  const { addToast } = useUIStore()
  const [showForm, setShowForm] = useState(false)
  const [editingChannelId, setEditingChannelId] = useState<string | null>(null)
  const [loadingChannelId, setLoadingChannelId] = useState<string | null>(null)
  const [form, setForm] = useState<ChannelFormState>(getDefaultChannelForm())

  const { data: channels, isLoading } = useQuery<Channel[]>({
    queryKey: ['channels'],
    queryFn: () => api.channels.list(),
  })

  const createMutation = useMutation({
    mutationFn: () => api.channels.create({
      name: form.name,
      type: form.type,
      config: JSON.stringify({ botToken: form.bot_token }),
      welcome_message: form.welcome_message,
      connect_url: getChannelConnectURL(),
      enabled: form.enabled,
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['channels'] })
      resetForm()
      addToast({ type: 'success', title: 'Channel added' })
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to add channel', message: err.message })
    },
  })

  const updateMutation = useMutation({
    mutationFn: () => api.channels.update(editingChannelId!, {
      name: form.name,
      type: form.type,
      config: JSON.stringify({ botToken: form.bot_token }),
      welcome_message: form.welcome_message,
      connect_url: getChannelConnectURL(),
      enabled: form.enabled,
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['channels'] })
      resetForm()
      addToast({ type: 'success', title: 'Channel updated' })
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to update channel', message: err.message })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.channels.delete(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ['channels'] })
      addToast({ type: 'success', title: 'Channel removed' })
      if (editingChannelId === id) {
        resetForm()
      }
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to remove channel', message: err.message })
    },
  })

  const toggleMutation = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) => api.channels.update(id, { enabled }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['channels'] })
      addToast({
        type: 'success',
        title: variables.enabled ? 'Channel activated' : 'Channel deactivated',
      })
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to update channel state', message: err.message })
    },
  })

  function resetForm() {
    setShowForm(false)
    setEditingChannelId(null)
    setLoadingChannelId(null)
    setForm(getDefaultChannelForm())
  }

  function startCreate() {
    setEditingChannelId(null)
    setForm(getDefaultChannelForm())
    setShowForm(true)
  }

  async function startEdit(id: string) {
    setLoadingChannelId(id)
    try {
      const channel = await api.channels.get(id)
      setEditingChannelId(id)
      setForm(channelToForm(channel))
      setShowForm(true)
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Failed to load channel',
        message: err instanceof Error ? err.message : 'Unknown error',
      })
    } finally {
      setLoadingChannelId(null)
    }
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (editingChannelId) {
      updateMutation.mutate()
      return
    }
    createMutation.mutate()
  }

  const isSubmitting = createMutation.isPending || updateMutation.isPending

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-24 w-full" />
      </div>
    )
  }

  return (
    <div>
      <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 className="text-lg font-semibold text-text">Channels</h2>
          <p className="mt-1 text-sm text-text-muted">
            Manage Telegram and Discord bot connections for message-based automation.
          </p>
        </div>
        <Button onClick={showForm ? resetForm : startCreate}>
          <Plus className="w-4 h-4" />
          {showForm ? 'Close' : 'Add Channel'}
        </Button>
      </div>

      {showForm && (
        <Card className="mb-6">
          <CardContent className="p-6">
            <div className="mb-4">
              <h2 className="text-lg font-semibold text-text">
                {editingChannelId ? 'Edit Channel' : 'Add Channel'}
              </h2>
              <p className="text-sm text-text-muted">
                Configure an external bot connection and its welcome/connect flow.
              </p>
            </div>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div>
                  <Label>Name</Label>
                  <Input
                    value={form.name}
                    onChange={(e) => setForm({ ...form, name: e.target.value })}
                    placeholder="Ops Telegram"
                    required
                  />
                </div>
                <div>
                  <Label>Channel Type</Label>
                  <select
                    value={form.type}
                    onChange={(e) => setForm({ ...form, type: e.target.value })}
                    className="w-full px-3 py-2 bg-bg-input border border-border rounded-lg text-text text-sm focus:outline-none focus:ring-2 focus:ring-accent/50 focus:border-accent"
                  >
                    <option value="telegram">Telegram</option>
                    <option value="discord">Discord</option>
                  </select>
                </div>
                <div className="sm:col-span-2">
                  <Label>Bot Token</Label>
                  <Input
                    type="password"
                    value={form.bot_token}
                    onChange={(e) => setForm({ ...form, bot_token: e.target.value })}
                    placeholder={form.type === 'telegram' ? '123456:ABC...' : 'Bot token'}
                    required
                  />
                </div>
                <div className="sm:col-span-2">
                  <Label>Welcome Message</Label>
                  <Textarea
                    value={form.welcome_message}
                    onChange={(e) => setForm({ ...form, welcome_message: e.target.value })}
                    rows={4}
                    placeholder="Welcome! Use this one-time code to connect this chat to Automator."
                  />
                  <p className="mt-2 text-xs text-text-dimmed">
                    The connect button link is generated automatically from this Automator URL.
                  </p>
                </div>
                <div className="sm:col-span-2 flex items-center gap-2">
                  <Checkbox
                    checked={form.enabled}
                    onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
                  />
                  <span className="text-sm text-text-muted">Start this bot after saving</span>
                </div>
              </div>
              <div className="flex gap-2">
                <Button type="submit" loading={isSubmitting}>
                  {editingChannelId ? 'Save Changes' : 'Save Channel'}
                </Button>
                <Button type="button" variant="ghost" onClick={resetForm}>
                  Cancel
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      )}

      {channels?.length === 0 ? (
        <Card>
          <CardContent className="p-12 text-center">
            <MessageSquare className="w-12 h-12 text-text-dimmed mx-auto mb-4" />
            <h3 className="text-lg font-medium text-text mb-2">No channels configured</h3>
            <p className="text-text-muted">Add a Telegram or Discord bot connection to start receiving messages</p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-3">
          {channels?.map((channel) => (
            <Card key={channel.id} className="group">
              <CardContent className="p-5 flex items-center justify-between">
                <div className="flex items-center gap-4">
                  <div className="w-10 h-10 rounded-lg bg-blue-500/10 flex items-center justify-center">
                    <MessageSquare className="w-5 h-5 text-blue-400" />
                  </div>
                  <div>
                    <div className="flex items-center gap-2">
                      <p className="text-sm font-medium text-text">{channel.name}</p>
                      <Badge variant={channel.enabled ? 'success' : 'default'}>
                        {channel.enabled ? 'Active' : 'Inactive'}
                      </Badge>
                    </div>
                    <p className="text-xs text-text-dimmed">{channel.type}</p>
                  </div>
                </div>
                <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => toggleMutation.mutate({ id: channel.id, enabled: !channel.enabled })}
                    disabled={toggleMutation.isPending}
                    title={channel.enabled ? 'Deactivate channel' : 'Activate channel'}
                  >
                    <Power className={cn('w-4 h-4', channel.enabled ? 'text-amber-400' : 'text-green-400')} />
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => void startEdit(channel.id)}
                    disabled={loadingChannelId === channel.id || deleteMutation.isPending}
                    title="Edit channel"
                  >
                    <Edit2 className="w-4 h-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => deleteMutation.mutate(channel.id)}
                    disabled={loadingChannelId === channel.id || deleteMutation.isPending}
                    title="Delete channel"
                  >
                    <Trash2 className="w-4 h-4 text-red-400" />
                  </Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  )
}

function LLMSettings() {
  const queryClient = useQueryClient()
  const { addToast } = useUIStore()
  const [showForm, setShowForm] = useState(false)
  const [editingProviderId, setEditingProviderId] = useState<string | null>(null)
  const [loadingProviderId, setLoadingProviderId] = useState<string | null>(null)
  const [form, setForm] = useState<ProviderFormState>(getDefaultProviderForm())

  const { data: providers, isLoading } = useQuery<LLMProvider[]>({
    queryKey: ['llm-providers'],
    queryFn: () => api.llmProviders.list(),
  })

  const createMutation = useMutation({
    mutationFn: () => api.llmProviders.create(form),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['llm-providers'] })
      resetForm()
      addToast({ type: 'success', title: 'LLM provider added' })
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to add provider', message: err.message })
    },
  })

  const updateMutation = useMutation({
    mutationFn: () => api.llmProviders.update(editingProviderId!, form),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['llm-providers'] })
      resetForm()
      addToast({ type: 'success', title: 'LLM provider updated' })
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to update provider', message: err.message })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.llmProviders.delete(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ['llm-providers'] })
      addToast({ type: 'success', title: 'Provider removed' })
      if (editingProviderId === id) {
        resetForm()
      }
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to remove provider', message: err.message })
    },
  })

  function resetForm() {
    setShowForm(false)
    setEditingProviderId(null)
    setLoadingProviderId(null)
    setForm(getDefaultProviderForm())
  }

  function startCreate() {
    setEditingProviderId(null)
    setForm(getDefaultProviderForm())
    setShowForm(true)
  }

  async function startEdit(id: string) {
    setLoadingProviderId(id)
    try {
      const provider = await api.llmProviders.get(id)
      setEditingProviderId(id)
      setForm(providerToForm(provider))
      setShowForm(true)
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Failed to load provider',
        message: err instanceof Error ? err.message : 'Unknown error',
      })
    } finally {
      setLoadingProviderId(null)
    }
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (editingProviderId) {
      updateMutation.mutate()
      return
    }
    createMutation.mutate()
  }

  const handleProviderTypeChange = (providerType: string) => {
    setForm((current) => {
      const currentDefault = getProviderDefaultBaseURL(current.provider_type)
      const nextDefault = getProviderDefaultBaseURL(providerType)
      const shouldReplaceBaseURL = !current.base_url || current.base_url === currentDefault

      return {
        ...current,
        provider_type: providerType,
        base_url: shouldReplaceBaseURL ? nextDefault : current.base_url,
      }
    })
  }

  const isSubmitting = createMutation.isPending || updateMutation.isPending

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-20 w-full" />
      </div>
    )
  }

  return (
    <div>
      <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 className="text-lg font-semibold text-text">LLM Providers</h2>
          <p className="mt-1 text-sm text-text-muted">
            Configure the AI providers available to prompt, chat, and agent nodes.
          </p>
        </div>
        <Button onClick={showForm ? resetForm : startCreate}>
          <Plus className="w-4 h-4" />
          {showForm ? 'Close' : 'Add Provider'}
        </Button>
      </div>

      {showForm && (
        <Card className="mb-6">
          <CardContent className="p-6">
            <div className="mb-4">
              <h2 className="text-lg font-semibold text-text">
                {editingProviderId ? 'Edit LLM Provider' : 'Add LLM Provider'}
              </h2>
              <p className="text-sm text-text-muted">
                {editingProviderId
                  ? 'Update provider credentials, defaults, and model settings.'
                  : 'Add a new provider for prompt and chat nodes.'}
              </p>
            </div>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div>
                  <Label>Name</Label>
                  <Input
                    value={form.name}
                    onChange={(e) => setForm({ ...form, name: e.target.value })}
                    placeholder="My OpenAI"
                    required
                  />
                </div>
                <div>
                  <Label>Provider Type</Label>
                  <select
                    value={form.provider_type}
                    onChange={(e) => handleProviderTypeChange(e.target.value)}
                    className="w-full px-3 py-2 bg-bg-input border border-border rounded-lg text-text text-sm focus:outline-none focus:ring-2 focus:ring-accent/50 focus:border-accent"
                  >
                    <option value="openai">OpenAI</option>
                    <option value="openrouter">OpenRouter</option>
                    <option value="ollama">Ollama</option>
                    <option value="anthropic">Anthropic</option>
                    <option value="custom">Custom (OpenAI-compatible)</option>
                  </select>
                </div>
                <div>
                  <Label>API Key</Label>
                  <Input
                    type="password"
                    value={form.api_key}
                    onChange={(e) => setForm({ ...form, api_key: e.target.value })}
                    placeholder={form.provider_type === 'openrouter' ? 'sk-or-...' : 'sk-...'}
                    required={form.provider_type !== 'ollama'}
                  />
                </div>
                <div>
                  <Label>Model</Label>
                  <Input
                    value={form.model}
                    onChange={(e) => setForm({ ...form, model: e.target.value })}
                    placeholder={getProviderModelPlaceholder(form.provider_type)}
                    required
                  />
                </div>
                <div className="sm:col-span-2">
                  <Label>Base URL</Label>
                  <Input
                    value={form.base_url}
                    onChange={(e) => setForm({ ...form, base_url: e.target.value })}
                    placeholder={getProviderDefaultBaseURL(form.provider_type) || 'https://api.openai.com/v1'}
                  />
                </div>
                <div className="sm:col-span-2 flex items-center gap-2">
                  <Checkbox
                    checked={form.is_default}
                    onChange={(e) => setForm({ ...form, is_default: e.target.checked })}
                  />
                  <span className="text-sm text-text-muted">Set as default provider</span>
                </div>
              </div>
              <div className="flex gap-2">
                <Button type="submit" loading={isSubmitting}>
                  {editingProviderId ? 'Save Changes' : 'Save Provider'}
                </Button>
                <Button type="button" variant="ghost" onClick={resetForm}>
                  Cancel
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      )}

      {providers?.length === 0 ? (
        <Card>
          <CardContent className="p-12 text-center">
            <Brain className="w-12 h-12 text-text-dimmed mx-auto mb-4" />
            <h3 className="text-lg font-medium text-text mb-2">No LLM providers configured</h3>
            <p className="text-text-muted">Add an LLM provider to enable AI-powered automation</p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-3">
          {providers?.map((provider) => (
            <Card key={provider.id} className="group">
              <CardContent className="p-5 flex items-center justify-between">
                <div className="flex items-center gap-4">
                  <div className="w-10 h-10 rounded-lg bg-purple-500/10 flex items-center justify-center">
                    <Brain className="w-5 h-5 text-purple-400" />
                  </div>
                  <div>
                    <div className="flex items-center gap-2">
                      <p className="text-sm font-medium text-text">{provider.name}</p>
                      {provider.is_default && <Badge variant="info">Default</Badge>}
                    </div>
                    <p className="text-xs text-text-dimmed">{provider.provider_type} / {provider.model}</p>
                    {provider.base_url && (
                      <p className="text-xs text-text-dimmed truncate max-w-[24rem]">{provider.base_url}</p>
                    )}
                  </div>
                </div>
                <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => void startEdit(provider.id)}
                    disabled={loadingProviderId === provider.id || deleteMutation.isPending}
                    title="Edit provider"
                  >
                    <Edit2 className="w-4 h-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => deleteMutation.mutate(provider.id)}
                    disabled={loadingProviderId === provider.id || deleteMutation.isPending}
                    title="Delete provider"
                  >
                    <Trash2 className="w-4 h-4 text-red-400" />
                  </Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  )
}
