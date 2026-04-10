import { useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy, Download, FileJson, GitBranch, Trash2 } from 'lucide-react'
import { useNavigate } from 'react-router-dom'

import { api } from '../api/client'
import { Card, CardContent } from '../components/ui/Card'
import Button from '../components/ui/Button'
import Badge from '../components/ui/Badge'
import Modal from '../components/ui/Modal'
import Skeleton from '../components/ui/Skeleton'
import { formatDate } from '../lib/utils'
import { downloadJSON, sanitizeFilename } from '../lib/download'
import { useUIStore } from '../store/ui'
import type { TemplateSummary } from '../types'

export default function Templates() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { addToast } = useUIStore()
  const importInputRef = useRef<HTMLInputElement>(null)
  const [busyTemplateId, setBusyTemplateId] = useState<string | null>(null)
  const [isImporting, setIsImporting] = useState(false)
  const [isExportingLibrary, setIsExportingLibrary] = useState(false)
  const [templatePendingDelete, setTemplatePendingDelete] = useState<TemplateSummary | null>(null)

  const { data: templates, isLoading } = useQuery<TemplateSummary[]>({
    queryKey: ['templates'],
    queryFn: () => api.templates.list(),
  })

  const cloneMutation = useMutation({
    mutationFn: (id: string) => api.templates.clone(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['templates'] })
      addToast({ type: 'success', title: 'Template duplicated' })
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to duplicate template', message: err.message })
    },
    onSettled: () => setBusyTemplateId(null),
  })

  const createPipelineMutation = useMutation({
    mutationFn: (id: string) => api.templates.createPipeline(id),
    onSuccess: (pipeline) => {
      queryClient.invalidateQueries({ queryKey: ['pipelines'] })
      addToast({ type: 'success', title: 'Pipeline created from template' })
      navigate(`/pipelines/${pipeline.id}`)
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to create pipeline', message: err.message })
    },
    onSettled: () => setBusyTemplateId(null),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.templates.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['templates'] })
      setTemplatePendingDelete(null)
      addToast({ type: 'success', title: 'Template deleted' })
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to delete template', message: err.message })
    },
  })

  async function handleImportFile(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return

    try {
      setIsImporting(true)
      const raw = await file.text()
      const result = await api.templates.import(raw)
      queryClient.invalidateQueries({ queryKey: ['templates'] })

      if (result.created_count > 0) {
        addToast({
          type: 'success',
          title: 'Templates imported',
          message: `${result.created_count} template${result.created_count === 1 ? '' : 's'} added.`,
        })
      }
      if (result.failed_count > 0) {
        const firstError = result.errors[0]?.error ?? 'Some templates could not be imported.'
        addToast({
          type: 'warning',
          title: 'Import completed with issues',
          message: `${result.failed_count} failed. ${firstError}`,
          duration: 6500,
        })
      }
      if (result.created_count === 0 && result.failed_count === 0) {
        addToast({ type: 'info', title: 'No templates imported' })
      }
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Failed to import templates',
        message: err instanceof Error ? err.message : 'Unknown error',
      })
    } finally {
      setIsImporting(false)
    }
  }

  async function handleExportLibrary() {
    try {
      setIsExportingLibrary(true)
      const document = await api.templates.exportAll()
      downloadJSON('emerald-templates.json', document)
      addToast({ type: 'success', title: 'Template library exported' })
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Failed to export template library',
        message: err instanceof Error ? err.message : 'Unknown error',
      })
    } finally {
      setIsExportingLibrary(false)
    }
  }

  async function handleExportTemplate(template: TemplateSummary) {
    try {
      setBusyTemplateId(template.id)
      const document = await api.templates.export(template.id)
      downloadJSON(sanitizeFilename(template.name, 'template'), document)
      addToast({ type: 'success', title: 'Template exported' })
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Failed to export template',
        message: err instanceof Error ? err.message : 'Unknown error',
      })
    } finally {
      setBusyTemplateId(null)
    }
  }

  return (
    <div className="mx-auto max-w-7xl p-6">
      <div className="mb-8 flex flex-wrap items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-text">Templates</h1>
          <p className="mt-1 text-text-muted">Reuse, import, export, and promote pipeline building blocks.</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <input
            ref={importInputRef}
            type="file"
            accept="application/json,.json"
            className="hidden"
            onChange={handleImportFile}
          />
          <Button
            variant="secondary"
            onClick={() => importInputRef.current?.click()}
            loading={isImporting}
          >
            <FileJson className="h-4 w-4" />
            Import JSON
          </Button>
          <Button variant="secondary" onClick={handleExportLibrary} loading={isExportingLibrary}>
            <Download className="h-4 w-4" />
            Export Library
          </Button>
        </div>
      </div>

      {isLoading ? (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {[1, 2, 3].map((item) => (
            <Card key={item}>
              <CardContent className="space-y-4 p-5">
                <Skeleton className="h-5 w-1/2" />
                <Skeleton className="h-4 w-full" />
                <Skeleton className="h-4 w-2/3" />
                <div className="flex gap-2">
                  <Skeleton className="h-8 w-28" />
                  <Skeleton className="h-8 w-28" />
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      ) : templates?.length ? (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {templates.map((template) => {
            const isBusy = busyTemplateId === template.id

            return (
              <Card key={template.id} className="border-border/80">
                <CardContent className="p-5">
                  <div className="mb-4 flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <h2 className="truncate text-base font-semibold text-text">{template.name}</h2>
                      <p className="mt-1 text-xs text-text-muted">
                        {template.description?.trim() || 'No description'}
                      </p>
                    </div>
                    <Badge>{template.category}</Badge>
                  </div>

                  <p className="mb-4 text-xs text-text-dimmed">
                    Created {formatDate(template.created_at)}
                  </p>

                  <div className="flex flex-wrap gap-2">
                    <Button
                      size="sm"
                      onClick={() => {
                        setBusyTemplateId(template.id)
                        createPipelineMutation.mutate(template.id)
                      }}
                      loading={isBusy && createPipelineMutation.isPending}
                    >
                      <GitBranch className="h-4 w-4" />
                      Use as Pipeline
                    </Button>
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={() => {
                        setBusyTemplateId(template.id)
                        cloneMutation.mutate(template.id)
                      }}
                      loading={isBusy && cloneMutation.isPending}
                    >
                      <Copy className="h-4 w-4" />
                      Duplicate
                    </Button>
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={() => handleExportTemplate(template)}
                      loading={isBusy && !cloneMutation.isPending && !createPipelineMutation.isPending}
                    >
                      <Download className="h-4 w-4" />
                      Export JSON
                    </Button>
                    <Button
                      size="sm"
                      variant="danger"
                      onClick={() => setTemplatePendingDelete(template)}
                      disabled={isBusy}
                    >
                      <Trash2 className="h-4 w-4" />
                      Delete
                    </Button>
                  </div>
                </CardContent>
              </Card>
            )
          })}
        </div>
      ) : (
        <Card>
          <CardContent className="p-12 text-center">
            <Copy className="mx-auto mb-4 h-12 w-12 text-text-dimmed" />
            <h3 className="mb-2 text-lg font-medium text-text">No templates yet</h3>
            <p className="mb-6 text-text-muted">
              Save a pipeline as a template from the editor, or import template JSON here.
            </p>
            <Button variant="secondary" onClick={() => importInputRef.current?.click()}>
              <FileJson className="h-4 w-4" />
              Import JSON
            </Button>
          </CardContent>
        </Card>
      )}

      <Modal
        open={templatePendingDelete !== null}
        title="Delete Template"
        description="This will permanently remove the template from your reusable library."
        onClose={() => {
          if (!deleteMutation.isPending) {
            setTemplatePendingDelete(null)
          }
        }}
        className="max-w-md"
      >
        {templatePendingDelete && (
          <div className="space-y-4">
            <div className="rounded-xl border border-red-600/30 bg-red-600/10 px-4 py-3">
              <p className="text-sm font-medium text-text">{templatePendingDelete.name}</p>
              <div className="mt-2 flex items-center gap-2">
                <Badge>{templatePendingDelete.category}</Badge>
                <span className="text-xs text-text-dimmed">
                  Created {formatDate(templatePendingDelete.created_at)}
                </span>
              </div>
            </div>

            <p className="text-sm text-text-muted">
              This action cannot be undone.
            </p>

            <div className="flex justify-end gap-2">
              <Button
                variant="ghost"
                onClick={() => setTemplatePendingDelete(null)}
                disabled={deleteMutation.isPending}
              >
                Cancel
              </Button>
              <Button
                variant="danger"
                loading={deleteMutation.isPending}
                onClick={() => deleteMutation.mutate(templatePendingDelete.id)}
              >
                <Trash2 className="h-4 w-4" />
                Delete Template
              </Button>
            </div>
          </div>
        )}
      </Modal>
    </div>
  )
}
