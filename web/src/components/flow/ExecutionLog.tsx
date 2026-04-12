import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Clock, CheckCircle, XCircle, AlertCircle, ChevronDown, ChevronRight, Terminal, Square, Brain } from 'lucide-react'
import { api } from '../../api/client'
import { cn, formatDate } from '../../lib/utils'
import type { ActiveExecution, Execution, ExecutionDetail, NodeExecution, NodeExecutionLogData } from '../../types'
import { useUIStore } from '../../store/ui'
import Badge from '../ui/Badge'
import Button from '../ui/Button'
import Skeleton from '../ui/Skeleton'

interface ExecutionEvent {
  type: 'execution_started' | 'execution_completed' | 'execution_cancelling' | 'node_started' | 'node_completed'
  pipeline?: string
  execution?: string
  trigger_type?: string
  node_id?: string
  node_type?: string
  input?: string
  status?: string
  error?: string
  output?: string
  started_at?: string
  completed_at?: string
}

interface ExecutionLogProps {
  pipelineId: string
  isOpen: boolean
  onClose: () => void
  preferredExecutionId?: string | null
  onExecutionSelect?: (data: {
    nodeIds: string[]
    nodeStatuses: Record<string, string>
    nodeErrors: Record<string, string | undefined>
    nodeLogs: Record<string, NodeExecutionLogData>
  } | null) => void
  onAddToAssistant?: (detail: ExecutionDetail) => void
  onRealtimeStatusChange?: (isReady: boolean) => void
}

function parseExecutionValue(value?: string): unknown {
  if (!value) return undefined

  try {
    return JSON.parse(value)
  } catch {
    return value
  }
}

function getNodeExecutions(detail?: ExecutionDetail | null): NodeExecution[] {
  if (!detail || !Array.isArray(detail.node_executions)) {
    return []
  }

  return detail.node_executions
}

function buildExecutionSelection(detail: ExecutionDetail) {
  const nodeExecutions = getNodeExecutions(detail)
  const nodeIds = nodeExecutions.map((ne) => ne.node_id)
  const nodeStatuses: Record<string, string> = {}
  const nodeErrors: Record<string, string | undefined> = {}
  const nodeLogs: Record<string, NodeExecutionLogData> = {}

  nodeExecutions.forEach((ne) => {
    nodeStatuses[ne.node_id] = ne.status
    nodeErrors[ne.node_id] = ne.error
    nodeLogs[ne.node_id] = {
      status: ne.status,
      node_type: ne.node_type,
      input: parseExecutionValue(ne.input),
      output: parseExecutionValue(ne.output),
      error: ne.error,
    }
  })

  return { nodeIds, nodeStatuses, nodeErrors, nodeLogs }
}

function sortActiveExecutions(executions: ActiveExecution[]): ActiveExecution[] {
  return [...executions].sort(
    (left, right) => new Date(right.started_at).getTime() - new Date(left.started_at).getTime(),
  )
}

function updateActiveExecutionsFromEvent(
  executions: ActiveExecution[],
  event: ExecutionEvent,
): ActiveExecution[] {
  if (!event.execution) {
    return executions
  }

  const index = executions.findIndex((execution) => execution.execution_id === event.execution)
  const current = index >= 0 ? executions[index] : undefined

  if (event.type === 'execution_completed') {
    if (index < 0) {
      return executions
    }

    return executions.filter((execution) => execution.execution_id !== event.execution)
  }

  if (event.type === 'execution_started') {
    const next: ActiveExecution = {
      execution_id: event.execution,
      pipeline_id: event.pipeline || current?.pipeline_id || '',
      trigger_type: event.trigger_type || current?.trigger_type || 'manual',
      status: (event.status as ActiveExecution['status']) || current?.status || 'running',
      started_at: event.started_at || current?.started_at || new Date().toISOString(),
      current_node_id: current?.current_node_id,
      current_node_type: current?.current_node_type,
      current_node_started_at: current?.current_node_started_at,
    }

    if (index < 0) {
      return sortActiveExecutions([...executions, next])
    }

    const updated = [...executions]
    updated[index] = next
    return sortActiveExecutions(updated)
  }

  if (!current) {
    return executions
  }

  const next: ActiveExecution = { ...current }

  if (event.type === 'execution_cancelling') {
    next.status = 'cancelling'
  }

  if (event.type === 'node_started') {
    next.current_node_id = event.node_id
    next.current_node_type = event.node_type
    next.current_node_started_at = event.started_at
  }

  if (event.type === 'node_completed' && (!event.node_id || next.current_node_id === event.node_id)) {
    delete next.current_node_id
    delete next.current_node_type
    delete next.current_node_started_at
  }

  const updated = [...executions]
  updated[index] = next
  return sortActiveExecutions(updated)
}

function updateExecutionDetailFromEvent(
  detail: ExecutionDetail | undefined,
  event: ExecutionEvent,
): ExecutionDetail | undefined {
  if (!detail || detail.execution.id !== event.execution) {
    return detail
  }

  if (event.type === 'execution_completed') {
    return {
      ...detail,
      execution: {
        ...detail.execution,
        status: (event.status as Execution['status']) || detail.execution.status,
        completed_at: event.completed_at || detail.execution.completed_at,
        error: event.error || detail.execution.error,
      },
    }
  }

  if (event.type !== 'node_started' && event.type !== 'node_completed') {
    return detail
  }

  if (!event.node_id) {
    return detail
  }

  const nextNodeExecutions = [...getNodeExecutions(detail)]
  const existingIndex = nextNodeExecutions.findIndex((nodeExecution) => nodeExecution.node_id === event.node_id)

  if (event.type === 'node_started') {
    const runningNode: NodeExecution = {
      id: existingIndex >= 0 ? nextNodeExecutions[existingIndex].id : `ws-${event.execution}-${event.node_id}`,
      execution_id: event.execution!,
      node_id: event.node_id,
      node_type: event.node_type || (existingIndex >= 0 ? nextNodeExecutions[existingIndex].node_type : ''),
      status: 'running',
      input: event.input || (existingIndex >= 0 ? nextNodeExecutions[existingIndex].input : undefined),
      output: undefined,
      error: undefined,
      started_at: event.started_at || (existingIndex >= 0 ? nextNodeExecutions[existingIndex].started_at : undefined),
      completed_at: undefined,
    }

    if (existingIndex >= 0) {
      nextNodeExecutions[existingIndex] = runningNode
    } else {
      nextNodeExecutions.push(runningNode)
    }
  }

  if (event.type === 'node_completed') {
    const existing = existingIndex >= 0 ? nextNodeExecutions[existingIndex] : undefined
    const completedNode: NodeExecution = {
      id: existing?.id || `ws-${event.execution}-${event.node_id}`,
      execution_id: event.execution!,
      node_id: event.node_id,
      node_type: event.node_type || existing?.node_type || '',
      status: event.status || existing?.status || 'completed',
      input: event.input || existing?.input,
      output: event.output || existing?.output,
      error: event.error || existing?.error,
      started_at: existing?.started_at,
      completed_at: event.completed_at || existing?.completed_at,
    }

    if (existingIndex >= 0) {
      nextNodeExecutions[existingIndex] = completedNode
    } else {
      nextNodeExecutions.push(completedNode)
    }
  }

  return {
    ...detail,
    node_executions: nextNodeExecutions,
  }
}

export default function ExecutionLog({
  pipelineId,
  isOpen,
  onClose,
  preferredExecutionId,
  onExecutionSelect,
  onAddToAssistant,
  onRealtimeStatusChange,
}: ExecutionLogProps) {
  const queryClient = useQueryClient()
  const { addToast } = useUIStore()
  const [selectedExecution, setSelectedExecution] = useState<string | null>(null)
  const [expandedNodes, setExpandedNodes] = useState<Set<string>>(new Set())
  const [nodeResultsCollapsed, setNodeResultsCollapsed] = useState(false)

  const { data: executions, isLoading } = useQuery<Execution[]>({
    queryKey: ['executions', pipelineId],
    queryFn: () => api.executions.listByPipeline(pipelineId),
    enabled: isOpen && !!pipelineId,
  })

  const { data: activeExecutions = [], isLoading: isActiveLoading } = useQuery<ActiveExecution[]>({
    queryKey: ['executions', 'active', pipelineId],
    queryFn: () => api.executions.activeByPipeline(pipelineId),
    enabled: isOpen && !!pipelineId,
    refetchOnWindowFocus: false,
  })

  const { data: executionDetail } = useQuery<ExecutionDetail>({
    queryKey: ['execution', selectedExecution],
    queryFn: () => api.executions.get(selectedExecution!) as Promise<ExecutionDetail>,
    enabled: isOpen && !!selectedExecution,
  })

  const cancelExecutionMutation = useMutation({
    mutationFn: (executionId: string) => api.executions.cancel(executionId),
    onSuccess: (execution) => {
      queryClient.setQueryData<ActiveExecution[]>(['executions', 'active', pipelineId], (current = []) => {
        const next = current.filter((item) => item.execution_id !== execution.execution_id)
        next.push(execution)
        return sortActiveExecutions(next)
      })
      addToast({
        type: 'warning',
        title: 'Stopping execution',
        message: `Execution ${execution.execution_id.slice(0, 8)} is being cancelled.`,
      })
    },
    onError: (error: Error) => {
      addToast({
        type: 'error',
        title: 'Failed to stop execution',
        message: error.message,
      })
    },
  })
  const nodeExecutions = getNodeExecutions(executionDetail)
  const activeExecutionIds = useMemo(() => new Set(activeExecutions.map((execution) => execution.execution_id)), [activeExecutions])
  const historyExecutions = useMemo(
    () => (executions ?? []).filter((execution) => !activeExecutionIds.has(execution.id)),
    [activeExecutionIds, executions],
  )

  useEffect(() => {
    if (!isOpen || !pipelineId) {
      onRealtimeStatusChange?.(false)
      return
    }

    const wsUrl = new URL(`/ws/${encodeURIComponent(`pipeline-${pipelineId}`)}`, window.location.origin)
    wsUrl.protocol = wsUrl.protocol === 'https:' ? 'wss:' : 'ws:'

    const socket = new WebSocket(wsUrl.toString())

    onRealtimeStatusChange?.(false)

    socket.onopen = () => {
      onRealtimeStatusChange?.(true)
      void queryClient.invalidateQueries({ queryKey: ['executions', 'active', pipelineId] })
      void queryClient.invalidateQueries({ queryKey: ['executions', pipelineId] })
    }

    socket.onerror = () => {
      onRealtimeStatusChange?.(false)
    }

    socket.onclose = () => {
      onRealtimeStatusChange?.(false)
    }

    socket.onmessage = (event) => {
      try {
        const payload = JSON.parse(event.data) as ExecutionEvent
        if (payload.pipeline !== pipelineId) {
          return
        }

        queryClient.setQueryData<ActiveExecution[]>(['executions', 'active', pipelineId], (current = []) => (
          updateActiveExecutionsFromEvent(current, payload)
        ))

        if (payload.type === 'execution_started' && payload.execution) {
          queryClient.setQueryData<ExecutionDetail>(['execution', payload.execution], (current) => (
            current || {
              execution: {
                id: payload.execution,
                pipeline_id: payload.pipeline || pipelineId,
                trigger_type: payload.trigger_type || 'manual',
                status: (payload.status as Execution['status']) || 'running',
                started_at: payload.started_at || new Date().toISOString(),
              },
              node_executions: [],
            }
          ))
          setSelectedExecution(payload.execution)
          setExpandedNodes(new Set())
        }

        if (payload.execution && (
          payload.type === 'node_started'
          || payload.type === 'node_completed'
          || payload.type === 'execution_completed'
        )) {
          queryClient.setQueryData<ExecutionDetail | undefined>(
            ['execution', payload.execution],
            (current) => updateExecutionDetailFromEvent(current, payload),
          )
        }

        if (payload.type === 'execution_completed') {
          queryClient.invalidateQueries({ queryKey: ['executions', pipelineId] })
        }
      } catch (err) {
        console.error('Failed to process execution websocket event', err)
      }
    }

    return () => {
      onRealtimeStatusChange?.(false)
      socket.close()
    }
  }, [isOpen, onRealtimeStatusChange, pipelineId, queryClient])

  useEffect(() => {
    if (!executionDetail) return
    onExecutionSelect?.(buildExecutionSelection(executionDetail))
  }, [executionDetail, onExecutionSelect])

  useEffect(() => {
    if (!isOpen || !preferredExecutionId) {
      return
    }

    setSelectedExecution((current) => (current === preferredExecutionId ? current : preferredExecutionId))
    setExpandedNodes(new Set())
  }, [isOpen, preferredExecutionId])

  useEffect(() => {
    if (!isOpen) return

    if (selectedExecution && selectedExecution === preferredExecutionId) {
      return
    }

    if (selectedExecution && (activeExecutionIds.has(selectedExecution) || executions?.some((execution) => execution.id === selectedExecution))) {
      return
    }

    const fallbackExecutionId = activeExecutions[0]?.execution_id ?? executions?.[0]?.id ?? null
    setSelectedExecution(fallbackExecutionId)
    setExpandedNodes(new Set())
  }, [activeExecutionIds, activeExecutions, executions, isOpen, preferredExecutionId, selectedExecution])

  const toggleNode = (nodeId: string) => {
    setExpandedNodes((prev) => {
      const next = new Set(prev)
      if (next.has(nodeId)) {
        next.delete(nodeId)
      } else {
        next.add(nodeId)
      }
      return next
    })
  }

  const formatDuration = (started: string, completed?: string) => {
    const start = new Date(started).getTime()
    const end = completed ? new Date(completed).getTime() : Date.now()
    const ms = end - start
    if (ms < 1000) return `${ms}ms`
    return `${(ms / 1000).toFixed(1)}s`
  }

  const handleAddToAssistant = async (executionId: string) => {
    if (!onAddToAssistant) {
      return
    }

    const detail = selectedExecution === executionId && executionDetail
      ? executionDetail
      : await queryClient.fetchQuery<ExecutionDetail>({
          queryKey: ['execution', executionId],
          queryFn: () => api.executions.get(executionId) as Promise<ExecutionDetail>,
        })

    onAddToAssistant(detail)
  }

  if (!isOpen) return null

  const hasAnyExecutions = activeExecutions.length > 0 || (executions?.length ?? 0) > 0

  return (
    <div className="flex w-[22rem] max-w-[calc(100vw-2rem)] max-h-[calc(100vh-9rem)] min-h-0 flex-col overflow-hidden rounded-xl border border-border bg-bg-elevated shadow-xl">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-border">
        <h3 className="text-sm font-semibold text-text">Execution Log</h3>
        <div className="flex items-center gap-2">
          {onAddToAssistant && (
            <Button
              variant="secondary"
              size="sm"
              disabled={!selectedExecution}
              title="Add selected run to agent"
              aria-label="Add selected run to agent"
              onClick={() => {
                if (selectedExecution) {
                  void handleAddToAssistant(selectedExecution)
                }
              }}
            >
              <Brain className="w-3.5 h-3.5" />
            </Button>
          )}
          <button onClick={onClose} className="text-text-dimmed hover:text-text transition-colors">
            <XCircle className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Execution List */}
      <div className="flex-1 overflow-y-auto">
        {isLoading || isActiveLoading ? (
          <div className="p-4 space-y-3">
            {[1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-16 w-full" />
            ))}
          </div>
        ) : !hasAnyExecutions ? (
          <div className="p-8 text-center">
            <Clock className="w-8 h-8 text-text-dimmed mx-auto mb-3" />
            <p className="text-sm text-text-muted">No executions yet</p>
            <p className="text-xs text-text-dimmed mt-1">Run the pipeline to see results</p>
          </div>
        ) : (
          <div>
            {activeExecutions.length > 0 && (
              <div className="border-b border-border">
                <div className="px-4 py-2 border-b border-border bg-bg-overlay/70">
                  <p className="text-xs font-medium text-text-muted">In Flight</p>
                </div>
                <div className="divide-y divide-border">
                  {activeExecutions.map((execution) => (
                    <button
                      key={execution.execution_id}
                      onClick={() => {
                        setSelectedExecution(execution.execution_id)
                        setExpandedNodes(new Set())
                      }}
                      className={cn(
                        'w-full px-4 py-3 text-left transition-colors hover:bg-bg-overlay',
                        selectedExecution === execution.execution_id && 'bg-bg-overlay',
                      )}
                    >
                      <div className="flex items-start justify-between gap-3 mb-2">
                        <div className="min-w-0">
                          <div className="flex items-center gap-2">
                            <Clock className="w-3.5 h-3.5 text-amber-400 animate-pulse" />
                            <span className="text-xs text-text-muted capitalize">{execution.trigger_type}</span>
                          </div>
                          <p className="mt-1 text-xs text-text-dimmed truncate">
                            {execution.current_node_id
                              ? `Current node: ${execution.current_node_id}${execution.current_node_type ? ` (${execution.current_node_type})` : ''}`
                              : 'Waiting for next node update'}
                          </p>
                        </div>
                        <Badge variant="warning">{execution.status}</Badge>
                      </div>
                      <div className="flex items-center justify-between gap-2">
                        <span className="text-xs text-text-dimmed">{formatDate(execution.started_at)}</span>
                        <span className="text-xs text-text-dimmed">
                          {formatDuration(execution.started_at)}
                        </span>
                      </div>
                      <div className="mt-2 flex items-center justify-between gap-2">
                        <div className="flex items-center gap-3">
                          {onAddToAssistant && (
                            <button
                              onClick={(event) => {
                                event.stopPropagation()
                                setSelectedExecution(execution.execution_id)
                                void handleAddToAssistant(execution.execution_id)
                              }}
                              className="inline-flex items-center justify-center rounded-md p-1 text-accent transition-colors hover:bg-accent/10 hover:text-accent-hover"
                              title="Add run to agent"
                              aria-label="Add run to agent"
                            >
                              <Brain className="w-3 h-3" />
                            </button>
                          )}
                        </div>
                        <Button
                          variant="danger"
                          size="sm"
                          loading={cancelExecutionMutation.isPending && cancelExecutionMutation.variables === execution.execution_id}
                          disabled={execution.status === 'cancelling'}
                          onClick={(event) => {
                            event.stopPropagation()
                            cancelExecutionMutation.mutate(execution.execution_id)
                          }}
                        >
                          <Square className="w-3.5 h-3.5" />
                          {execution.status === 'cancelling' ? 'Stopping...' : 'Force stop'}
                        </Button>
                      </div>
                    </button>
                  ))}
                </div>
              </div>
            )}

            <div className="divide-y divide-border">
              {historyExecutions.map((exec) => (
                <button
                  key={exec.id}
                  onClick={() => {
                    setSelectedExecution(exec.id)
                    setExpandedNodes(new Set())
                  }}
                  className={cn(
                    'w-full px-4 py-3 text-left transition-colors hover:bg-bg-overlay',
                    selectedExecution === exec.id && 'bg-bg-overlay',
                  )}
                >
                  <div className="flex items-center justify-between mb-1">
                    <div className="flex items-center gap-2">
                      {exec.status === 'completed' && <CheckCircle className="w-3.5 h-3.5 text-green-400" />}
                      {exec.status === 'failed' && <XCircle className="w-3.5 h-3.5 text-red-400" />}
                      {exec.status === 'running' && <Clock className="w-3.5 h-3.5 text-amber-400 animate-pulse" />}
                      {exec.status === 'cancelled' && <XCircle className="w-3.5 h-3.5 text-red-400" />}
                      <span className="text-xs text-text-muted capitalize">{exec.trigger_type}</span>
                    </div>
                    <Badge
                      variant={
                        exec.status === 'completed'
                          ? 'success'
                          : exec.status === 'failed' || exec.status === 'cancelled'
                          ? 'error'
                          : exec.status === 'running'
                          ? 'warning'
                          : 'default'
                      }
                    >
                      {exec.status}
                    </Badge>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-xs text-text-dimmed">{formatDate(exec.started_at)}</span>
                    <span className="text-xs text-text-dimmed">
                      {formatDuration(exec.started_at, exec.completed_at)}
                    </span>
                  </div>
                  {exec.error && (
                    <p className="text-xs text-red-400 mt-1 truncate">{exec.error}</p>
                  )}
                  <div className="mt-1 flex justify-end gap-3">
                    {onAddToAssistant && (
                      <button
                        onClick={(e) => {
                          e.stopPropagation()
                          setSelectedExecution(exec.id)
                          void handleAddToAssistant(exec.id)
                        }}
                        className="inline-flex items-center justify-center rounded-md p-1 text-accent transition-colors hover:bg-accent/10 hover:text-accent-hover"
                        title="Add run to agent"
                        aria-label="Add run to agent"
                      >
                        <Brain className="w-3 h-3" />
                      </button>
                    )}
                  </div>
                </button>
              ))}
            </div>
          </div>
        )}
      </div>

      {/* Node Execution Details */}
      {executionDetail && (
        <div className="border-t border-border max-h-1/2 overflow-y-auto">
          <div className="flex items-center justify-between gap-3 px-4 py-2 border-b border-border bg-bg-overlay">
            <p className="text-xs font-medium text-text-muted">Node Results</p>
            <button
              type="button"
              onClick={() => setNodeResultsCollapsed((current) => !current)}
              className="inline-flex items-center gap-1 text-xs text-text-dimmed transition-colors hover:text-text"
              aria-expanded={!nodeResultsCollapsed}
              aria-label={nodeResultsCollapsed ? 'Expand node results' : 'Collapse node results'}
            >
              {nodeResultsCollapsed ? (
                <ChevronRight className="w-3 h-3" />
              ) : (
                <ChevronDown className="w-3 h-3" />
              )}
              <span>{nodeResultsCollapsed ? 'Expand' : 'Collapse'}</span>
            </button>
          </div>
          {!nodeResultsCollapsed && (
            <div className="divide-y divide-border">
              {nodeExecutions.map((ne) => (
                <div key={ne.id}>
                  <button
                    onClick={() => toggleNode(ne.node_id)}
                    className="w-full px-4 py-2.5 text-left flex items-center gap-2 hover:bg-bg-overlay transition-colors"
                  >
                    {expandedNodes.has(ne.node_id) ? (
                      <ChevronDown className="w-3 h-3 text-text-dimmed flex-shrink-0" />
                    ) : (
                      <ChevronRight className="w-3 h-3 text-text-dimmed flex-shrink-0" />
                    )}
                    {ne.status === 'completed' ? (
                      <CheckCircle className="w-3.5 h-3.5 text-green-400 flex-shrink-0" />
                    ) : ne.status === 'running' ? (
                      <Clock className="w-3.5 h-3.5 text-amber-400 animate-pulse flex-shrink-0" />
                    ) : (
                      <XCircle className="w-3.5 h-3.5 text-red-400 flex-shrink-0" />
                    )}
                    <div className="min-w-0 flex-1">
                      <p className="text-xs font-medium text-text truncate">{ne.node_id}</p>
                      <p className="text-xs text-text-dimmed truncate">{ne.node_type}</p>
                    </div>
                    {ne.started_at && ne.completed_at && (
                      <span className="text-xs text-text-dimmed flex-shrink-0">
                        {formatDuration(ne.started_at, ne.completed_at)}
                      </span>
                    )}
                  </button>

                  {expandedNodes.has(ne.node_id) && (
                    <div className="px-4 pb-3 pl-10 space-y-2">
                      {ne.error ? (
                        <div className="bg-red-600/10 border border-red-600/30 rounded-lg p-3">
                          <div className="flex items-center gap-2 mb-1">
                            <AlertCircle className="w-3.5 h-3.5 text-red-400" />
                            <span className="text-xs font-medium text-red-400">Error</span>
                          </div>
                          <pre className="text-xs text-red-300 whitespace-pre-wrap break-all font-mono">
                            {ne.error}
                          </pre>
                        </div>
                      ) : ne.status === 'running' ? (
                        <div className="bg-amber-500/10 border border-amber-500/30 rounded-lg p-3">
                          <div className="flex items-center gap-2 mb-1">
                            <Clock className="w-3.5 h-3.5 text-amber-400 animate-pulse" />
                            <span className="text-xs font-medium text-amber-300">Running</span>
                          </div>
                          <p className="text-xs text-amber-100/80">This node is currently executing.</p>
                        </div>
                      ) : (
                        ne.output && (
                          <div className="bg-bg-input border border-border rounded-lg overflow-hidden">
                            <div className="flex items-center gap-2 px-3 py-1.5 border-b border-border bg-bg-overlay">
                              <Terminal className="w-3 h-3 text-text-dimmed" />
                              <span className="text-xs text-text-muted">Output</span>
                            </div>
                            <pre className="p-3 text-xs text-text font-mono overflow-x-auto whitespace-pre-wrap max-h-48">
                              {(() => {
                                try {
                                  const parsed = JSON.parse(ne.output)
                                  return JSON.stringify(parsed, null, 2)
                                } catch {
                                  return ne.output
                                }
                              })()}
                            </pre>
                          </div>
                        )
                      )}
                    </div>
                  )}
                </div>
              ))}
              {nodeExecutions.length === 0 && (
                <div className="px-4 py-5 text-sm text-text-muted">
                  No node-level results were recorded for this execution.
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
