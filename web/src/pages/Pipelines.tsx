import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { Plus, GitBranch, Trash2, ExternalLink, Power, Download } from 'lucide-react'
import { api } from '../api/client'
import { Card, CardContent } from '../components/ui/Card'
import Button from '../components/ui/Button'
import Input from '../components/ui/Input'
import { Label } from '../components/ui/Form'
import Badge from '../components/ui/Badge'
import Modal from '../components/ui/Modal'
import Skeleton from '../components/ui/Skeleton'
import { formatDate } from '../lib/utils'
import { downloadJSON, sanitizeFilename } from '../lib/download'
import { useUIStore } from '../store/ui'
import type { Pipeline } from '../types'

export default function Pipelines() {
  const queryClient = useQueryClient()
  const { addToast } = useUIStore()
  const [showCreate, setShowCreate] = useState(false)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [exportingPipelineId, setExportingPipelineId] = useState<string | null>(null)
  const [pipelinePendingDelete, setPipelinePendingDelete] = useState<Pipeline | null>(null)

  const { data: pipelines, isLoading } = useQuery<Pipeline[]>({
    queryKey: ['pipelines'],
    queryFn: () => api.pipelines.list(),
  })

  const createMutation = useMutation({
    mutationFn: async () => {
      return api.pipelines.create({ name, description })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['pipelines'] })
      setShowCreate(false)
      setName('')
      setDescription('')
      addToast({ type: 'success', title: 'Pipeline created' })
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to create', message: err.message })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.pipelines.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['pipelines'] })
      setPipelinePendingDelete(null)
      addToast({ type: 'success', title: 'Pipeline deleted' })
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to delete', message: err.message })
    },
  })

  const toggleStatusMutation = useMutation({
    mutationFn: ({ id, status }: { id: string; status: string }) => api.pipelines.update(id, { status }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['pipelines'] })
      addToast({ type: 'success', title: 'Pipeline status updated' })
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to update status', message: err.message })
    },
  })

  const handleCreate = (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    createMutation.mutate()
  }

  const handleExportPipeline = async (pipeline: Pipeline) => {
    try {
      setExportingPipelineId(pipeline.id)
      const document = await api.pipelines.export(pipeline.id)
      downloadJSON(
        sanitizeFilename(pipeline.name || 'pipeline', 'pipeline'),
        document,
      )
      addToast({ type: 'success', title: 'Pipeline exported as JSON' })
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Failed to export pipeline',
        message: err instanceof Error ? err.message : 'Unknown error',
      })
    } finally {
      setExportingPipelineId(null)
    }
  }

  return (
    <div className="p-6 max-w-7xl mx-auto">
      <div className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-2xl font-bold text-text">Pipelines</h1>
          <p className="text-text-muted mt-1">Manage your automation workflows</p>
        </div>
        <Button onClick={() => setShowCreate(!showCreate)}>
          <Plus className="w-4 h-4" />
          New Pipeline
        </Button>
      </div>

      {showCreate && (
        <Card className="mb-6">
          <CardContent className="p-6">
            <form onSubmit={handleCreate} className="space-y-4">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div>
                  <Label>Name</Label>
                  <Input
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="My Pipeline"
                    required
                  />
                </div>
                <div>
                  <Label>Description</Label>
                  <Input
                    value={description}
                    onChange={(e) => setDescription(e.target.value)}
                    placeholder="Optional description"
                  />
                </div>
              </div>
              <div className="flex gap-2">
                <Button type="submit" loading={createMutation.isPending}>
                  Create Pipeline
                </Button>
                <Button type="button" variant="ghost" onClick={() => setShowCreate(false)}>
                  Cancel
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      )}

      {isLoading ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {[1, 2, 3].map((i) => (
            <Card key={i}>
              <CardContent className="p-5">
                <Skeleton className="h-5 w-3/4 mb-2" />
                <Skeleton className="h-4 w-full mb-4" />
                <div className="flex justify-between">
                  <Skeleton className="h-5 w-16" />
                  <Skeleton className="h-4 w-20" />
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      ) : pipelines?.length === 0 ? (
        <Card>
          <CardContent className="p-12 text-center">
            <GitBranch className="w-12 h-12 text-text-dimmed mx-auto mb-4" />
            <h3 className="text-lg font-medium text-text mb-2">No pipelines yet</h3>
            <p className="text-text-muted mb-6">Create your first pipeline to start automating</p>
            <Button onClick={() => setShowCreate(true)}>
              <Plus className="w-4 h-4" />
              Create Pipeline
            </Button>
          </CardContent>
        </Card>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {pipelines?.map((pipeline) => (
            <Card key={pipeline.id} className="group h-full hover:border-accent/50 transition-colors">
              <CardContent className="flex h-full flex-col p-5">
                <div className="mb-3 flex items-start justify-between gap-3">
                  <Link
                    to={`/pipelines/${pipeline.id}`}
                    className="flex min-w-0 flex-1 items-center gap-2 text-text hover:text-accent transition-colors"
                  >
                    <GitBranch className="w-4 h-4 flex-shrink-0" />
                    <span className="text-sm font-semibold truncate">{pipeline.name}</span>
                    <ExternalLink className="w-3 h-3 opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0" />
                  </Link>
                  <div className="flex items-center gap-1">
                    <Button
                      variant="ghost"
                      size="sm"
                      className="opacity-0 group-hover:opacity-100 transition-opacity"
                      loading={exportingPipelineId === pipeline.id}
                      onClick={() => handleExportPipeline(pipeline)}
                    >
                      <Download className="w-3.5 h-3.5" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => toggleStatusMutation.mutate({
                        id: pipeline.id,
                        status: pipeline.status === 'active' ? 'draft' : 'active',
                      })}
                    >
                      <Power className={`w-3.5 h-3.5 ${pipeline.status === 'active' ? 'text-amber-400' : 'text-green-400'}`} />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="opacity-0 group-hover:opacity-100 transition-opacity"
                      onClick={() => setPipelinePendingDelete(pipeline)}
                    >
                      <Trash2 className="w-3.5 h-3.5 text-red-400" />
                    </Button>
                  </div>
                </div>
                <div className="min-h-[2.5rem]">
                  {pipeline.description && (
                    <p className="line-clamp-2 text-xs leading-5 text-text-muted">{pipeline.description}</p>
                  )}
                </div>
                <div className="mt-4 flex items-end justify-between gap-3">
                  <Badge
                    variant={
                      pipeline.status === 'active'
                        ? 'success'
                        : pipeline.status === 'draft'
                        ? 'warning'
                        : 'default'
                    }
                  >
                    {pipeline.status}
                  </Badge>
                  <span className="text-xs text-text-dimmed">
                    {formatDate(pipeline.updated_at, pipeline.created_at)}
                  </span>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <Modal
        open={pipelinePendingDelete !== null}
        title="Delete Pipeline"
        description="This will permanently remove the pipeline and its saved graph."
        onClose={() => {
          if (!deleteMutation.isPending) {
            setPipelinePendingDelete(null)
          }
        }}
        className="max-w-md"
      >
        {pipelinePendingDelete && (
          <div className="space-y-4">
            <div className="rounded-xl border border-red-600/30 bg-red-600/10 px-4 py-3">
              <p className="text-sm font-medium text-text">{pipelinePendingDelete.name}</p>
              <div className="mt-2 flex items-center gap-2">
                <Badge
                  variant={
                    pipelinePendingDelete.status === 'active'
                      ? 'success'
                      : pipelinePendingDelete.status === 'draft'
                        ? 'warning'
                        : 'default'
                  }
                >
                  {pipelinePendingDelete.status}
                </Badge>
                <span className="text-xs text-text-dimmed">
                  Updated {formatDate(pipelinePendingDelete.updated_at, pipelinePendingDelete.created_at)}
                </span>
              </div>
            </div>

            <p className="text-sm text-text-muted">
              This action cannot be undone.
            </p>

            <div className="flex justify-end gap-2">
              <Button
                variant="ghost"
                onClick={() => setPipelinePendingDelete(null)}
                disabled={deleteMutation.isPending}
              >
                Cancel
              </Button>
              <Button
                variant="danger"
                loading={deleteMutation.isPending}
                onClick={() => deleteMutation.mutate(pipelinePendingDelete.id)}
              >
                <Trash2 className="w-4 h-4" />
                Delete Pipeline
              </Button>
            </div>
          </div>
        )}
      </Modal>
    </div>
  )
}
