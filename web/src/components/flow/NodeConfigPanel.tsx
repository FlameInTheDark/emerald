import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import type { Edge, Node } from '@xyflow/react'
import {
  X, Settings, Play, Square, Copy, Globe, Code, Zap, Clock, Webhook,
  GitBranch, Split, Brain, Link, Plus, Trash2, MessageSquare, Send, RefreshCw,
  Bot, Workflow, List, Wrench, CornerDownLeft, CircleHelp,
} from 'lucide-react'
import { cn } from '../../lib/utils'
import { NODE_TYPE_MAP, getNodeColor, getNodeLabel } from './nodeTypes'
import type { ExecutionDetail, LLMModelInfo, NodeDefinitionField, NodeType, Pipeline, TemplateSuggestion } from '../../types'
import { api } from '../../api/client'
import Input from '../ui/Input'
import { Checkbox, Label } from '../ui/Form'
import Select from '../ui/Select'
import Button from '../ui/Button'
import { TemplateInput, TemplateTextarea } from '../ui/TemplateFields'
import { buildPromptInsertSuggestions, buildTemplateSuggestions } from '../../lib/templates'
import LuaEditorModal from './LuaEditorModal'
import HelpTooltip from '../ui/HelpTooltip'
import KubernetesNodeConfigSection, { kubernetesNodeTypes } from './KubernetesNodeConfigSection'
import { useNodeDefinitions } from '../../hooks/useNodeDefinitions'

const iconMap: Record<string, React.ElementType> = {
  zap: Zap,
  clock: Clock,
  webhook: Webhook,
  'message-square': MessageSquare,
  play: Play,
  square: Square,
  copy: Copy,
  globe: Globe,
  link: Link,
  code: Code,
  send: Send,
  'git-branch': GitBranch,
  split: Split,
  brain: Brain,
  bot: Bot,
  workflow: Workflow,
  list: List,
  wrench: Wrench,
  'refresh-cw': RefreshCw,
  'trash-2': Trash2,
  'corner-down-left': CornerDownLeft,
}

const proxmoxNodeTypes = new Set<NodeType>([
  'action:proxmox_list_nodes',
  'action:proxmox_list_workloads',
  'action:vm_start',
  'action:vm_stop',
  'action:vm_clone',
  'tool:proxmox_list_nodes',
  'tool:proxmox_list_workloads',
  'tool:vm_start',
  'tool:vm_stop',
  'tool:vm_clone',
])

const channelNodeTypes = new Set<NodeType>([
  'trigger:channel_message',
  'action:channel_send_message',
  'action:channel_reply_message',
  'action:channel_edit_message',
  'action:channel_send_and_wait',
  'tool:channel_send_and_wait',
])

const pipelineMutationToolNodeTypes = new Set<NodeType>([
  'tool:pipeline_create',
  'tool:pipeline_update',
  'tool:pipeline_delete',
])

const EXPR_LANGUAGE_DOCS_URL = 'https://expr-lang.org/docs/language-definition'
const DEFAULT_GROUP_COLOR = '#64748b'

function supportsNodeErrorPolicy(nodeType?: string): boolean {
  if (typeof nodeType !== 'string' || !nodeType) {
    return false
  }

  return !nodeType.startsWith('tool:')
    && nodeType !== 'visual:group'
    && nodeType !== 'logic:return'
    && nodeType !== 'logic:condition'
    && nodeType !== 'logic:switch'
}

interface NodeConfigPanelProps {
  pipelineId: string
  nodes: Node[]
  edges: Edge[]
  nodeId: string
  nodeType?: NodeType | string
  nodeLabel: string
  config: Record<string, unknown>
  onUpdate: (config: Record<string, unknown>) => void
  onLabelChange: (label: string) => void
  onRemoveSourceHandles?: (handleIds: string[]) => void
  onOverlayOpenChange?: (open: boolean) => void
  onClose: () => void
}

type SwitchConditionConfig = {
  id: string
  label: string
  expression: string
}

type PipelineToolArgumentConfig = {
  name: string
  description: string
  required: boolean
}

type AggregateInputConfig = {
  nodeId: string
  label: string
  nodeType: string
  sourceHandles: string[]
}

function normalizeSwitchConditions(value: unknown): SwitchConditionConfig[] {
  if (!Array.isArray(value)) {
    return []
  }

  return value.map((condition, index) => {
    const record = typeof condition === 'object' && condition !== null
      ? condition as Record<string, unknown>
      : {}

    return {
      id: typeof record.id === 'string' && record.id.trim()
        ? record.id.trim()
        : `condition-${index + 1}`,
      label: typeof record.label === 'string' && record.label.trim()
        ? record.label.trim()
        : `Condition ${index + 1}`,
      expression: typeof record.expression === 'string' ? record.expression : '',
    }
  })
}

function createSwitchCondition(existing: SwitchConditionConfig[]): SwitchConditionConfig {
  const takenIds = new Set(existing.map((condition) => condition.id))
  let index = existing.length + 1
  let id = `condition-${index}`

  while (takenIds.has(id)) {
    index += 1
    id = `condition-${index}`
  }

  return {
    id,
    label: `Condition ${index}`,
    expression: '',
  }
}

function normalizePipelineToolArguments(value: unknown): PipelineToolArgumentConfig[] {
  if (!Array.isArray(value)) {
    return []
  }

  return value.map((argument) => {
    const record = typeof argument === 'object' && argument !== null
      ? argument as Record<string, unknown>
      : {}

    return {
      name: typeof record.name === 'string' ? record.name : '',
      description: typeof record.description === 'string' ? record.description : '',
      required: Boolean(record.required),
    }
  })
}

function createPipelineToolArgument(existing: PipelineToolArgumentConfig[]): PipelineToolArgumentConfig {
  const takenNames = new Set(existing.map((argument) => argument.name.trim()).filter(Boolean))
  let index = existing.length + 1
  let name = `argument_${index}`

  while (takenNames.has(name)) {
    index += 1
    name = `argument_${index}`
  }

  return {
    name,
    description: '',
    required: false,
  }
}

function FieldLabel({
  children,
  tooltip,
  docsHref,
  docsLabel,
}: {
  children: React.ReactNode
  tooltip?: React.ReactNode
  docsHref?: string
  docsLabel?: string
}) {
  return (
    <div className="mb-1.5 flex items-center gap-2">
      <Label className="mb-0">{children}</Label>
      {tooltip && (
        <HelpTooltip content={tooltip} label={typeof children === 'string' ? `Help for ${children}` : 'Show help'} />
      )}
      {docsHref && (
        <a
          href={docsHref}
          target="_blank"
          rel="noreferrer"
          className="inline-flex h-5 w-5 items-center justify-center rounded-full border border-border bg-bg-overlay text-text-muted transition-colors hover:border-accent/50 hover:text-accent"
          title={docsLabel}
          aria-label={docsLabel}
        >
          <CircleHelp className="h-3.5 w-3.5" />
        </a>
      )}
    </div>
  )
}

function ExpressionLabel() {
  return (
    <FieldLabel
      docsHref={EXPR_LANGUAGE_DOCS_URL}
      docsLabel="Open expression language documentation"
    >
      Expression
    </FieldLabel>
  )
}

function normalizeGroupColor(value: unknown): string {
  if (typeof value !== 'string') {
    return DEFAULT_GROUP_COLOR
  }

  return /^#(?:[0-9a-fA-F]{6})$/.test(value.trim())
    ? value.trim()
    : DEFAULT_GROUP_COLOR
}

function stringifyHeaders(value: unknown): string {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return '{}'
  }

  const normalized = Object.entries(value as Record<string, unknown>).reduce<Record<string, string>>((acc, [key, headerValue]) => {
    const trimmedKey = key.trim()
    if (!trimmedKey) {
      return acc
    }

    acc[trimmedKey] = typeof headerValue === 'string' ? headerValue : String(headerValue ?? '')
    return acc
  }, {})

  return JSON.stringify(normalized)
}

function parseHeadersJSON(raw: string): { value: Record<string, string> | null; error: string | null } {
  const trimmed = raw.trim()
  if (trimmed === '') {
    return { value: {}, error: null }
  }

  try {
    const parsed = JSON.parse(trimmed) as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return { value: null, error: 'Headers must be a JSON object.' }
    }

    const normalized = Object.entries(parsed as Record<string, unknown>).reduce<Record<string, string>>((acc, [key, headerValue]) => {
      const trimmedKey = key.trim()
      if (!trimmedKey) {
        return acc
      }

      acc[trimmedKey] = typeof headerValue === 'string' ? headerValue : String(headerValue ?? '')
      return acc
    }, {})

    return { value: normalized, error: null }
  } catch {
    return { value: null, error: 'Headers must be valid JSON.' }
  }
}

function normalizeAggregateIDOverrides(value: unknown): Record<string, string> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return {}
  }

  return Object.entries(value as Record<string, unknown>).reduce<Record<string, string>>((acc, [rawSourceId, rawAlias]) => {
    const sourceId = rawSourceId.trim()
    if (!sourceId || typeof rawAlias !== 'string') {
      return acc
    }

    const alias = rawAlias.trim()
    if (!alias) {
      return acc
    }

    acc[sourceId] = alias
    return acc
  }, {})
}

function defaultPluginFieldValue(field: NodeDefinitionField): unknown {
  switch (field.type) {
    case 'boolean':
      return field.default_bool_value ?? false
    case 'number':
      return field.default_number_value ?? 0
    default:
      return field.default_string_value ?? ''
  }
}

function pluginFieldValue(field: NodeDefinitionField, config: Record<string, unknown>): unknown {
  if (Object.prototype.hasOwnProperty.call(config, field.name)) {
    return config[field.name]
  }
  return defaultPluginFieldValue(field)
}

export default function NodeConfigPanel({
  pipelineId,
  nodes,
  edges,
  nodeId,
  nodeType,
  nodeLabel,
  config,
  onUpdate,
  onLabelChange,
  onRemoveSourceHandles,
  onOverlayOpenChange,
  onClose,
}: NodeConfigPanelProps) {
  const [activeTab, setActiveTab] = useState<'general' | 'config'>('general')
  const [label, setLabel] = useState(nodeLabel)
  const [localConfig, setLocalConfig] = useState(config)
  const [isLuaEditorOpen, setIsLuaEditorOpen] = useState(false)
  const [httpHeadersJSON, setHTTPHeadersJSON] = useState(() => stringifyHeaders(config.headers))
  const { map: nodeDefinitionMap } = useNodeDefinitions()
  const { data: clusters } = useQuery({
    queryKey: ['clusters'],
    queryFn: () => api.clusters.list(),
  })
  const { data: kubernetesClusters } = useQuery({
    queryKey: ['kubernetes-clusters'],
    queryFn: () => api.kubernetesClusters.list(),
  })
  const { data: llmProviders } = useQuery({
    queryKey: ['llm-providers'],
    queryFn: () => api.llmProviders.list(),
  })
  const { data: channels } = useQuery({
    queryKey: ['channels'],
    queryFn: () => api.channels.list(),
  })
  const { data: pipelines } = useQuery<Pipeline[]>({
    queryKey: ['pipelines'],
    queryFn: () => api.pipelines.list(),
  })
  const { data: executions } = useQuery({
    queryKey: ['executions', pipelineId],
    queryFn: () => api.executions.listByPipeline(pipelineId),
    enabled: !!pipelineId,
  })
  const { data: secrets } = useQuery({
    queryKey: ['secrets'],
    queryFn: () => api.secrets.list(),
  })

  const latestExecutionId = executions?.[0]?.id
  const { data: latestExecutionDetail } = useQuery<ExecutionDetail>({
    queryKey: ['execution', latestExecutionId],
    queryFn: () => api.executions.get(latestExecutionId!),
    enabled: !!latestExecutionId,
  })

  const defaultProvider = useMemo(
    () => llmProviders?.find((provider) => provider.is_default),
    [llmProviders],
  )
  const selectedProviderId = ((localConfig.providerId as string) || defaultProvider?.id || '').trim()
  const { data: providerModels } = useQuery<LLMModelInfo[]>({
    queryKey: ['llm-provider-models', selectedProviderId],
    queryFn: () => api.llmProviders.models(selectedProviderId),
    enabled: (nodeType === 'llm:prompt' || nodeType === 'llm:agent') && !!selectedProviderId,
  })

  useEffect(() => {
    setLabel(nodeLabel)
    setLocalConfig(config)
    setIsLuaEditorOpen(false)
    setHTTPHeadersJSON(stringifyHeaders(config.headers))
  }, [nodeId, nodeLabel, config])

  useEffect(() => {
    onOverlayOpenChange?.(isLuaEditorOpen)

    return () => {
      onOverlayOpenChange?.(false)
    }
  }, [isLuaEditorOpen, onOverlayOpenChange])

  const handleLabelBlur = () => {
    onLabelChange(label)
  }

  const handleConfigChange = (key: string, value: unknown) => {
    const newConfig = { ...localConfig, [key]: value }
    setLocalConfig(newConfig)
    onUpdate(newConfig)
  }

  const resolvedNodeType = typeof nodeType === 'string' ? nodeType : ''
  const nodeDef = resolvedNodeType ? (nodeDefinitionMap[resolvedNodeType] || NODE_TYPE_MAP[resolvedNodeType as NodeType]) : undefined
  const Icon = iconMap[nodeDef?.icon || 'zap']
  const color = nodeDef?.color || getNodeColor(resolvedNodeType)
  const showClusterSelect = proxmoxNodeTypes.has(resolvedNodeType as NodeType)
  const showKubernetesClusterSelect = kubernetesNodeTypes.has(resolvedNodeType as NodeType)
  const showChannelSelect = channelNodeTypes.has(resolvedNodeType as NodeType)
  const templateSuggestions = useMemo<TemplateSuggestion[]>(() => (
    buildTemplateSuggestions(nodeId, nodes, edges, latestExecutionDetail?.node_executions ?? [], nodeDefinitionMap, secrets || [])
  ), [nodeDefinitionMap, nodeId, nodes, edges, latestExecutionDetail, secrets])
  const promptInsertSuggestions = useMemo<TemplateSuggestion[]>(() => (
    buildPromptInsertSuggestions(nodeId, nodes, edges, latestExecutionDetail?.node_executions ?? [], nodeDefinitionMap, secrets || [])
  ), [nodeDefinitionMap, nodeId, nodes, edges, latestExecutionDetail, secrets])
  const agentTemplateSuggestions = useMemo<TemplateSuggestion[]>(() => {
    if (resolvedNodeType !== 'llm:agent' || !Boolean(localConfig.enableSkills)) {
      return promptInsertSuggestions
    }

    return [
      {
        expression: 'skills',
        template: '{{skills}}',
        label: 'Available skills',
        description: 'Current local skills list with names and descriptions.',
        kind: 'template',
      },
      ...promptInsertSuggestions,
    ]
  }, [localConfig.enableSkills, promptInsertSuggestions, resolvedNodeType])
  const switchConditions = useMemo(
    () => normalizeSwitchConditions(localConfig.conditions),
    [localConfig.conditions],
  )
  const modelOptions = useMemo(() => {
    const currentModel = (localConfig.model as string) || ''
    const options = [...(providerModels || [])]
    if (currentModel && !options.some((model) => model.id === currentModel)) {
      options.unshift({ id: currentModel, name: `${currentModel} (current)` })
    }
    return options
  }, [localConfig.model, providerModels])
  const pipelineOptions = useMemo(
    () => pipelines || [],
    [pipelines],
  )
  const runnablePipelines = useMemo(
    () => (pipelines || []).filter((pipeline) => pipeline.id !== pipelineId),
    [pipelineId, pipelines],
  )
  const isToolNode = resolvedNodeType.startsWith('tool:')
  const connectedToolCount = useMemo(
    () => edges.filter((edge) => edge.source === nodeId && edge.sourceHandle === 'tool').length,
    [edges, nodeId],
  )
  const groupColor = useMemo(
    () => normalizeGroupColor(localConfig.color),
    [localConfig.color],
  )
  const pipelineToolArguments = useMemo(
    () => normalizePipelineToolArguments(localConfig.arguments),
    [localConfig.arguments],
  )
  const aggregateIDOverrides = useMemo(
    () => normalizeAggregateIDOverrides(localConfig.idOverrides),
    [localConfig.idOverrides],
  )
  const aggregateInputs = useMemo<AggregateInputConfig[]>(() => {
    if (resolvedNodeType !== 'logic:aggregate') {
      return []
    }

    const sourceMap = new Map<string, AggregateInputConfig>()

    edges.forEach((edge) => {
      if (edge.target !== nodeId || edge.sourceHandle === 'tool') {
        return
      }

      const sourceNode = nodes.find((candidate) => candidate.id === edge.source)
      const sourceData = sourceNode?.data && typeof sourceNode.data === 'object'
        ? sourceNode.data as Record<string, unknown>
        : undefined
      const sourceHandle = typeof edge.sourceHandle === 'string' ? edge.sourceHandle.trim() : ''

      const existing = sourceMap.get(edge.source)
      if (existing) {
        if (sourceHandle && !existing.sourceHandles.includes(sourceHandle)) {
          existing.sourceHandles.push(sourceHandle)
        }
        return
      }

      sourceMap.set(edge.source, {
        nodeId: edge.source,
        label: typeof sourceData?.label === 'string' && sourceData.label.trim()
          ? sourceData.label.trim()
          : edge.source,
        nodeType: typeof sourceData?.type === 'string' ? sourceData.type : '',
        sourceHandles: sourceHandle ? [sourceHandle] : [],
      })
    })

    return Array.from(sourceMap.values())
  }, [edges, nodeId, nodes, resolvedNodeType])
  const aggregateUnusedOverrideKeys = useMemo(
    () => Object.keys(aggregateIDOverrides).filter((sourceId) => !aggregateInputs.some((source) => source.nodeId === sourceId)),
    [aggregateIDOverrides, aggregateInputs],
  )
  const httpHeadersState = useMemo(
    () => parseHeadersJSON(httpHeadersJSON),
    [httpHeadersJSON],
  )
  const showErrorPolicy = useMemo(
    () => supportsNodeErrorPolicy(resolvedNodeType),
    [resolvedNodeType],
  )

  const handleSwitchConditionChange = (conditionId: string, key: keyof SwitchConditionConfig, value: string) => {
    handleConfigChange(
      'conditions',
      switchConditions.map((condition) => (
        condition.id === conditionId
          ? { ...condition, [key]: value }
          : condition
      )),
    )
  }

  const handleSwitchConditionAdd = () => {
    handleConfigChange('conditions', [...switchConditions, createSwitchCondition(switchConditions)])
  }

  const handleSwitchConditionRemove = (conditionId: string) => {
    handleConfigChange(
      'conditions',
      switchConditions.filter((condition) => condition.id !== conditionId),
    )
    onRemoveSourceHandles?.([conditionId])
  }

  const handleHTTPHeadersJSONChange = (value: string) => {
    setHTTPHeadersJSON(value)
    const parsed = parseHeadersJSON(value)
    if (!parsed.error && parsed.value) {
      handleConfigChange('headers', parsed.value)
    }
  }

  const handlePipelineToolArgumentChange = (
    index: number,
    key: keyof PipelineToolArgumentConfig,
    value: string | boolean,
  ) => {
    handleConfigChange(
      'arguments',
      pipelineToolArguments.map((argument, argumentIndex) => (
        argumentIndex === index
          ? { ...argument, [key]: value }
          : argument
      )),
    )
  }

  const handlePipelineToolArgumentAdd = () => {
    handleConfigChange('arguments', [...pipelineToolArguments, createPipelineToolArgument(pipelineToolArguments)])
  }

  const handlePipelineToolArgumentRemove = (index: number) => {
    handleConfigChange(
      'arguments',
      pipelineToolArguments.filter((_, argumentIndex) => argumentIndex !== index),
    )
  }

  const handleAggregateIDOverrideChange = (sourceNodeID: string, value: string) => {
    const nextOverrides = { ...aggregateIDOverrides }
    const alias = value.trim()

    if (alias) {
      nextOverrides[sourceNodeID] = alias
    } else {
      delete nextOverrides[sourceNodeID]
    }

    if (Object.keys(nextOverrides).length === 0) {
      const { idOverrides: _removed, ...rest } = localConfig
      setLocalConfig(rest)
      onUpdate(rest)
      return
    }

    handleConfigChange('idOverrides', nextOverrides)
  }

  return (
    <div className="flex w-[22rem] max-w-[calc(100vw-2rem)] max-h-[calc(100vh-9rem)] min-h-0 flex-col overflow-hidden rounded-xl border border-border bg-bg-elevated shadow-xl">
      <div className="flex items-center justify-between px-4 py-3 border-b border-border">
        <div className="flex items-center gap-3">
          <div className="w-8 h-8 rounded-lg flex items-center justify-center" style={{ backgroundColor: `${color}20` }}>
            <Icon className="w-4 h-4" style={{ color }} />
          </div>
          <div>
            <p className="text-sm font-medium text-text">{nodeLabel}</p>
            <p className="text-xs text-text-dimmed">{nodeType}</p>
          </div>
        </div>
        <button onClick={onClose} className="text-text-dimmed hover:text-text transition-colors">
          <X className="w-4 h-4" />
        </button>
      </div>

      <div className="flex border-b border-border">
        <button
          onClick={() => setActiveTab('general')}
          className={cn(
            'flex-1 px-4 py-2.5 text-sm font-medium transition-colors border-b-2',
            activeTab === 'general'
              ? 'text-accent border-accent'
              : 'text-text-muted border-transparent hover:text-text',
          )}
        >
          General
        </button>
        <button
          onClick={() => setActiveTab('config')}
          className={cn(
            'flex-1 px-4 py-2.5 text-sm font-medium transition-colors border-b-2',
            activeTab === 'config'
              ? 'text-accent border-accent'
              : 'text-text-muted border-transparent hover:text-text',
          )}
        >
          Configuration
        </button>
      </div>

      <div className="flex-1 overflow-y-auto p-4">
        {activeTab === 'general' && (
          <div className="space-y-4">
            <div>
              <Label>Node Label</Label>
              <Input
                value={label}
                onChange={(e) => setLabel(e.target.value)}
                onBlur={handleLabelBlur}
                placeholder="Enter node label"
              />
            </div>
            <div>
              <Label>Node ID</Label>
              <div className="px-3 py-2 bg-bg-input border border-border rounded-lg text-xs text-text-dimmed font-mono">
                {nodeId}
              </div>
            </div>
            <div>
              <Label>Type</Label>
              <div className="px-3 py-2 bg-bg-input border border-border rounded-lg text-sm text-text">
                {getNodeLabel(nodeType)}
              </div>
            </div>
            <div>
              <Label>Description</Label>
              <p className="text-sm text-text-muted">{nodeDef?.description}</p>
            </div>
          </div>
        )}

        {activeTab === 'config' && (
          <div className="space-y-4">
            {showErrorPolicy && (
              <div className="rounded-xl border border-border bg-bg-input/60 p-3">
                <FieldLabel
                  tooltip={(
                    <>
                      <p>
                        <span className="font-medium text-text">Stop pipeline</span>
                        {' '}
                        keeps the current behavior and fails the run immediately.
                      </p>
                      <p>
                        <span className="font-medium text-text">Continue and pass error</span>
                        {' '}
                        records this node as failed but forwards its current input plus
                        {' '}
                        <span className="font-mono text-slate-200">error</span>
                        ,
                        {' '}
                        <span className="font-mono text-slate-200">errorMessage</span>
                        ,
                        {' '}
                        <span className="font-mono text-slate-200">errorNodeId</span>
                        ,
                        {' '}
                        <span className="font-mono text-slate-200">errorNodeType</span>
                        ,
                        and
                        {' '}
                        <span className="font-mono text-slate-200">failed</span>
                        {' '}
                        to the next node.
                      </p>
                    </>
                  )}
                >
                  Error Handling
                </FieldLabel>
                <Select
                  value={String(localConfig.errorPolicy ?? 'stop')}
                  onChange={(e) => handleConfigChange('errorPolicy', e.target.value)}
                >
                  <option value="stop">Stop pipeline on error</option>
                  <option value="continue">Continue and pass error</option>
                </Select>
                <p className="mt-2 text-xs text-text-muted">
                  Use a downstream condition node if you want to branch on
                  {' '}
                  <span className="font-mono text-text">input.failed</span>
                  {' '}
                  or
                  {' '}
                  <span className="font-mono text-text">input.error</span>
                  .
                </p>
              </div>
            )}

            {showClusterSelect && (
              <div>
                <Label>Cluster</Label>
                <Select
                  value={(localConfig.clusterId as string) || ''}
                  onChange={(e) => handleConfigChange('clusterId', e.target.value)}
                >
                  <option value="">Select cluster</option>
                  {clusters?.map((cluster) => (
                    <option key={cluster.id} value={cluster.id}>
                      {cluster.name}
                    </option>
                  ))}
                </Select>
              </div>
            )}

            {showKubernetesClusterSelect && (
              <div>
                <Label>Kubernetes Cluster</Label>
                <Select
                  value={(localConfig.clusterId as string) || ''}
                  onChange={(e) => handleConfigChange('clusterId', e.target.value)}
                >
                  <option value="">Select cluster</option>
                  {kubernetesClusters?.map((cluster) => (
                    <option key={cluster.id} value={cluster.id}>
                      {cluster.name}
                    </option>
                  ))}
                </Select>
              </div>
            )}

            {showChannelSelect && (
              <div>
                <Label>Channel</Label>
                <Select
                  value={(localConfig.channelId as string) || ''}
                  onChange={(e) => handleConfigChange('channelId', e.target.value)}
                >
                  <option value="">Select channel</option>
                  {channels?.map((channel) => (
                    <option key={channel.id} value={channel.id}>
                      {channel.name} ({channel.type})
                    </option>
                  ))}
                </Select>
              </div>
            )}

            {nodeType === 'action:proxmox_list_nodes' || nodeType === 'tool:proxmox_list_nodes' ? (
              <div className="rounded-lg border border-border bg-bg-input px-3 py-2 text-sm text-text-muted">
                {nodeType === 'tool:proxmox_list_nodes'
                  ? 'This tool lists all nodes in the selected cluster when the agent calls it.'
                  : 'This node lists all nodes in the selected cluster.'}
              </div>
            ) : nodeType === 'trigger:channel_message' ? (
              <div className="rounded-lg border border-border bg-bg-input px-3 py-2 text-sm text-text-muted">
                Connected messages from the selected channel will trigger this pipeline when the pipeline is active.
              </div>
            ) : nodeType === 'action:proxmox_list_workloads' || nodeType === 'tool:proxmox_list_workloads' ? (
              <>
                <div>
                  <FieldLabel
                    tooltip={(
                      <>
                        <p>Optional node name to limit workload results to a single Proxmox node.</p>
                        {nodeType === 'tool:proxmox_list_workloads' && (
                          <p>
                            The agent can override this default by passing
                            {' '}
                            <span className="font-mono text-slate-200">node</span>
                            .
                          </p>
                        )}
                      </>
                    )}
                  >
                    {nodeType === 'tool:proxmox_list_workloads' ? 'Default Node Filter' : 'Node Filter'}
                  </FieldLabel>
                  <TemplateInput
                    value={(localConfig.node as string) || ''}
                    onChange={(e) => handleConfigChange('node', e.target.value)}
                    placeholder="Optional node name, e.g., pve1"
                    suggestions={templateSuggestions}
                  />
                </div>
              </>
            ) : nodeType === 'action:vm_start' || nodeType === 'action:vm_stop' || nodeType === 'tool:vm_start' || nodeType === 'tool:vm_stop' ? (
              <>
                <div>
                  <FieldLabel
                    tooltip={isToolNode ? (
                      <>
                        <p>This value acts as the default Proxmox node for the tool.</p>
                        <p>
                          The agent can still override it with
                          {' '}
                          <span className="font-mono text-slate-200">node</span>
                          .
                        </p>
                      </>
                    ) : 'Proxmox node that hosts the target VM.'}
                  >
                    {isToolNode ? 'Default Proxmox Node' : 'Proxmox Node'}
                  </FieldLabel>
                  <TemplateInput
                    value={(localConfig.node as string) || ''}
                    onChange={(e) => handleConfigChange('node', e.target.value)}
                    placeholder="e.g., pve1"
                    suggestions={templateSuggestions}
                  />
                </div>
                <div>
                  <FieldLabel
                    tooltip={isToolNode ? (
                      <>
                        <p>This value acts as the default VM id for the tool.</p>
                        <p>
                          The agent can still override it with
                          {' '}
                          <span className="font-mono text-slate-200">vmid</span>
                          .
                        </p>
                      </>
                    ) : 'Target virtual machine id.'}
                  >
                    {isToolNode ? 'Default VM ID' : 'VM ID'}
                  </FieldLabel>
                  <Input
                    type="number"
                    value={(localConfig.vmid as number) || ''}
                    onChange={(e) => handleConfigChange('vmid', parseInt(e.target.value) || 0)}
                    placeholder="e.g., 100"
                  />
                </div>
              </>
            ) : nodeType === 'action:vm_clone' || nodeType === 'tool:vm_clone' ? (
              <>
                <div>
                  <FieldLabel
                    tooltip={isToolNode ? (
                      <>
                        <p>This value acts as the default source node for the clone tool.</p>
                        <p>
                          The agent can override it with
                          {' '}
                          <span className="font-mono text-slate-200">node</span>
                          .
                        </p>
                      </>
                    ) : 'Node that hosts the source VM.'}
                  >
                    {isToolNode ? 'Default Proxmox Node' : 'Proxmox Node'}
                  </FieldLabel>
                  <TemplateInput
                    value={(localConfig.node as string) || ''}
                    onChange={(e) => handleConfigChange('node', e.target.value)}
                    placeholder="e.g., pve1"
                    suggestions={templateSuggestions}
                  />
                </div>
                <div>
                  <FieldLabel
                    tooltip={isToolNode ? (
                      <>
                        <p>This value acts as the default source VM id for the clone tool.</p>
                        <p>
                          The agent can override it with
                          {' '}
                          <span className="font-mono text-slate-200">vmid</span>
                          .
                        </p>
                      </>
                    ) : 'VM id of the source machine to clone.'}
                  >
                    {isToolNode ? 'Default Source VM ID' : 'Source VM ID'}
                  </FieldLabel>
                  <Input
                    type="number"
                    value={(localConfig.vmid as number) || ''}
                    onChange={(e) => handleConfigChange('vmid', parseInt(e.target.value) || 0)}
                    placeholder="e.g., 100"
                  />
                </div>
                <div>
                  <FieldLabel
                    tooltip={isToolNode ? (
                      <>
                        <p>This value acts as the default name for the new VM clone.</p>
                        <p>
                          The agent can override it with
                          {' '}
                          <span className="font-mono text-slate-200">newName</span>
                          .
                        </p>
                      </>
                    ) : 'Name for the newly created VM clone.'}
                  >
                    {isToolNode ? 'Default New VM Name' : 'New VM Name'}
                  </FieldLabel>
                  <TemplateInput
                    value={(localConfig.newName as string) || ''}
                    onChange={(e) => handleConfigChange('newName', e.target.value)}
                    placeholder="e.g., cloned-vm"
                    suggestions={templateSuggestions}
                  />
                </div>
                <div>
                  <FieldLabel
                    tooltip={isToolNode ? (
                      <>
                        <p>This value acts as the default id for the new VM clone.</p>
                        <p>
                          The agent can override it with
                          {' '}
                          <span className="font-mono text-slate-200">newId</span>
                          .
                        </p>
                      </>
                    ) : 'Numeric id for the newly created VM clone.'}
                  >
                    {isToolNode ? 'Default New VM ID' : 'New VM ID'}
                  </FieldLabel>
                  <Input
                    type="number"
                    value={(localConfig.newId as number) || ''}
                    onChange={(e) => handleConfigChange('newId', parseInt(e.target.value) || 0)}
                    placeholder="e.g., 200"
                  />
                </div>
              </>
            ) : nodeType === 'action:http' || nodeType === 'tool:http' ? (
              <>
                <div>
                  <FieldLabel
                    tooltip={nodeType === 'tool:http' ? (
                      <>
                        <p>Configured request values act as defaults for the tool.</p>
                        <p>
                          The agent can override
                          {' '}
                          <span className="font-mono text-slate-200">url</span>
                          ,
                          {' '}
                          <span className="font-mono text-slate-200">method</span>
                          ,
                          {' '}
                          <span className="font-mono text-slate-200">headers</span>
                          , and
                          {' '}
                          <span className="font-mono text-slate-200">body</span>
                          .
                        </p>
                      </>
                    ) : 'Target URL for this HTTP request.'}
                  >
                    {nodeType === 'tool:http' ? 'Default URL' : 'URL'}
                  </FieldLabel>
                  <TemplateInput
                    value={(localConfig.url as string) || ''}
                    onChange={(e) => handleConfigChange('url', e.target.value)}
                    placeholder="https://api.example.com/endpoint"
                    suggestions={templateSuggestions}
                  />
                </div>
                <div>
                  <Label>{nodeType === 'tool:http' ? 'Default Method' : 'Method'}</Label>
                  <select
                    value={(localConfig.method as string) || 'GET'}
                    onChange={(e) => handleConfigChange('method', e.target.value)}
                    className="w-full px-3 py-2 bg-bg-input border border-border rounded-lg text-text text-sm focus:outline-none focus:ring-2 focus:ring-accent/50 focus:border-accent"
                  >
                    <option value="GET">GET</option>
                    <option value="POST">POST</option>
                    <option value="PUT">PUT</option>
                    <option value="DELETE">DELETE</option>
                    <option value="PATCH">PATCH</option>
                  </select>
                </div>
                <div>
                  <Label>{nodeType === 'tool:http' ? 'Default Body' : 'Body'}</Label>
                  <TemplateTextarea
                    value={(localConfig.body as string) || ''}
                    onChange={(e) => handleConfigChange('body', e.target.value)}
                    placeholder='{"key": "value"}'
                    rows={4}
                    className="font-mono text-xs"
                    suggestions={templateSuggestions}
                  />
                </div>
                <div>
                  <FieldLabel
                    tooltip={(
                      <>
                        <p>Provide headers as a JSON object.</p>
                        <p>
                          Example:
                          {' '}
                          <span className="font-mono text-slate-200">{'{"Authorization":"Bearer ...","X-Trace":"abc"}'}</span>
                        </p>
                      </>
                    )}
                  >
                    {nodeType === 'tool:http' ? 'Default Headers JSON' : 'Headers JSON'}
                  </FieldLabel>
                  <TemplateTextarea
                    value={httpHeadersJSON}
                    onChange={(e) => handleHTTPHeadersJSONChange(e.target.value)}
                    placeholder='{"Authorization":"Bearer ...","X-Trace":"abc"}'
                    rows={4}
                    className="font-mono text-xs"
                    suggestions={templateSuggestions}
                  />
                  {httpHeadersState.error && (
                    <p className="mt-2 text-xs text-red-400">
                      {httpHeadersState.error}
                    </p>
                  )}
                </div>
              </>
            ) : nodeType === 'action:shell_command' || nodeType === 'tool:shell_command' ? (
              <>
                <div>
                  <FieldLabel
                    tooltip={nodeType === 'tool:shell_command' ? (
                      <>
                        <p>Configured values act as defaults for the tool.</p>
                        <p>
                          The agent can override
                          {' '}
                          <span className="font-mono text-slate-200">command</span>
                          ,
                          {' '}
                          <span className="font-mono text-slate-200">workingDirectory</span>
                          , and
                          {' '}
                          <span className="font-mono text-slate-200">timeoutSeconds</span>
                          .
                        </p>
                      </>
                    ) : 'Runs a local command in the workspace and returns stdout, stderr, exit code, and timing information.'}
                  >
                    {nodeType === 'tool:shell_command' ? 'Default Command' : 'Command'}
                  </FieldLabel>
                  <TemplateTextarea
                    value={(localConfig.command as string) || ''}
                    onChange={(e) => handleConfigChange('command', e.target.value)}
                    placeholder="e.g., Get-ChildItem .agents\\skills"
                    rows={4}
                    className="font-mono text-xs"
                    suggestions={templateSuggestions}
                  />
                </div>
                <div>
                  <Label>Working Directory</Label>
                  <TemplateInput
                    value={(localConfig.workingDirectory as string) || ''}
                    onChange={(e) => handleConfigChange('workingDirectory', e.target.value)}
                    placeholder="Optional relative or absolute path"
                    suggestions={templateSuggestions}
                  />
                </div>
                <div>
                  <Label>Timeout Seconds</Label>
                  <Input
                    type="number"
                    min="1"
                    value={(localConfig.timeoutSeconds as number) ?? 60}
                    onChange={(e) => handleConfigChange('timeoutSeconds', parseInt(e.target.value, 10) || 60)}
                  />
                </div>
              </>
            ) : nodeType === 'action:channel_send_message' ? (
              <>
                <div>
                  <Label>Recipient</Label>
                  <TemplateInput
                    value={(localConfig.recipient as string) || ''}
                    onChange={(e) => handleConfigChange('recipient', e.target.value)}
                    placeholder="Optional contact ID or chat ID. Leave empty to reply to the triggering user."
                    suggestions={templateSuggestions}
                  />
                </div>
                <div>
                  <Label>Message</Label>
                  <TemplateTextarea
                    value={(localConfig.message as string) || ''}
                    onChange={(e) => handleConfigChange('message', e.target.value)}
                    placeholder="Write the message to send"
                    rows={6}
                    suggestions={templateSuggestions}
                  />
                </div>
              </>
            ) : nodeType === 'action:channel_edit_message' ? (
              <>
                <div>
                  <FieldLabel
                    tooltip="Optional contact ID or chat ID. Leave empty to edit the current triggering user's chat."
                  >
                    Recipient
                  </FieldLabel>
                  <TemplateInput
                    value={(localConfig.recipient as string) || ''}
                    onChange={(e) => handleConfigChange('recipient', e.target.value)}
                    placeholder="Optional contact ID or chat ID"
                    suggestions={templateSuggestions}
                  />
                </div>
                <div>
                  <FieldLabel
                    tooltip={(
                      <>
                        <p>Message ID to edit.</p>
                        <p>
                          Leave this empty to reuse a prior
                          {' '}
                          <span className="font-mono text-slate-200">message_id</span>
                          {' '}
                          or
                          {' '}
                          <span className="font-mono text-slate-200">response.message_id</span>
                          {' '}
                          from upstream channel nodes.
                        </p>
                      </>
                    )}
                  >
                    Message ID
                  </FieldLabel>
                  <TemplateInput
                    value={(localConfig.messageId as string) || ''}
                    onChange={(e) => handleConfigChange('messageId', e.target.value)}
                    placeholder="Optional explicit message ID"
                    suggestions={templateSuggestions}
                  />
                </div>
                <div>
                  <FieldLabel
                    tooltip="Replacement text for the existing message."
                  >
                    Message
                  </FieldLabel>
                  <TemplateTextarea
                    value={(localConfig.message as string) || ''}
                    onChange={(e) => handleConfigChange('message', e.target.value)}
                    placeholder="Write the updated message text"
                    rows={6}
                    suggestions={templateSuggestions}
                  />
                </div>
              </>
            ) : nodeType === 'action:channel_reply_message' ? (
              <>
                <div>
                  <FieldLabel
                    tooltip="Optional contact ID or chat ID. Leave empty to reuse the current triggering user's chat."
                  >
                    Recipient
                  </FieldLabel>
                  <TemplateInput
                    value={(localConfig.recipient as string) || ''}
                    onChange={(e) => handleConfigChange('recipient', e.target.value)}
                    placeholder="Optional contact ID or chat ID"
                    suggestions={templateSuggestions}
                  />
                </div>
                <div>
                  <FieldLabel
                    tooltip={(
                      <>
                        <p>Message ID to reply to.</p>
                        <p>
                          Leave this empty to reuse the current or upstream
                          {' '}
                          <span className="font-mono text-slate-200">message_id</span>
                          {' '}
                          from a channel trigger or channel action output.
                        </p>
                      </>
                    )}
                  >
                    Reply To Message ID
                  </FieldLabel>
                  <TemplateInput
                    value={(localConfig.replyToMessageId as string) || ''}
                    onChange={(e) => handleConfigChange('replyToMessageId', e.target.value)}
                    placeholder="Optional explicit message ID"
                    suggestions={templateSuggestions}
                  />
                </div>
                <div>
                  <FieldLabel
                    tooltip="Text to send as a new reply message."
                  >
                    Message
                  </FieldLabel>
                  <TemplateTextarea
                    value={(localConfig.message as string) || ''}
                    onChange={(e) => handleConfigChange('message', e.target.value)}
                    placeholder="Write the reply text"
                    rows={6}
                    suggestions={templateSuggestions}
                  />
                </div>
              </>
            ) : nodeType === 'action:channel_send_and_wait' || nodeType === 'tool:channel_send_and_wait' ? (
              <>
                <div>
                  <Label>{nodeType === 'tool:channel_send_and_wait' ? 'Default Recipient' : 'Recipient'}</Label>
                  <TemplateInput
                    value={(localConfig.recipient as string) || ''}
                    onChange={(e) => handleConfigChange('recipient', e.target.value)}
                    placeholder="Optional contact ID or chat ID. Leave empty to reply to the triggering user."
                    suggestions={templateSuggestions}
                  />
                </div>
                <div>
                  <FieldLabel
                    tooltip={nodeType === 'tool:channel_send_and_wait'
                      ? (
                        <>
                          <p>The agent can message a connected user and pause until that user replies.</p>
                          <p>Matching replies are routed back into this tool instead of triggering other channel pipelines.</p>
                        </>
                      )
                      : 'This node sends a message to a connected user, then waits until that same user replies or the timeout is reached.'}
                  >
                    {nodeType === 'tool:channel_send_and_wait' ? 'Default Message' : 'Message'}
                  </FieldLabel>
                  <TemplateTextarea
                    value={(localConfig.message as string) || ''}
                    onChange={(e) => handleConfigChange('message', e.target.value)}
                    placeholder="Write the message to send before waiting"
                    rows={6}
                    suggestions={templateSuggestions}
                  />
                </div>
                <div>
                  <Label>{nodeType === 'tool:channel_send_and_wait' ? 'Default Timeout (seconds)' : 'Timeout (seconds)'}</Label>
                  <Input
                    type="number"
                    min="1"
                    value={(localConfig.timeoutSeconds as number) || 300}
                    onChange={(e) => handleConfigChange('timeoutSeconds', parseInt(e.target.value, 10) || 0)}
                    placeholder="300"
                  />
                </div>
              </>
            ) : kubernetesNodeTypes.has(nodeType) ? (
              <KubernetesNodeConfigSection
                nodeType={nodeType}
                localConfig={localConfig}
                onConfigChange={handleConfigChange}
                suggestions={templateSuggestions}
                isToolNode={isToolNode}
              />
            ) : nodeType === 'action:pipeline_get' ? (
              <>
                <div>
                  <Label>Pipeline</Label>
                  <Select
                    value={(localConfig.pipelineId as string) || ''}
                    onChange={(e) => handleConfigChange('pipelineId', e.target.value)}
                  >
                    <option value="">Select pipeline</option>
                    {pipelineOptions.map((pipeline) => (
                      <option key={pipeline.id} value={pipeline.id}>
                        {pipeline.name}
                      </option>
                    ))}
                  </Select>
                </div>
                <label className="flex items-start gap-3 rounded-lg border border-border bg-bg-input px-3 py-2">
                  <Checkbox
                    checked={Boolean(localConfig.includeDefinition ?? true)}
                    onChange={(e) => handleConfigChange('includeDefinition', e.target.checked)}
                    className="mt-0.5"
                  />
                  <div className="min-w-0">
                    <div className="text-sm font-medium text-text">Include full definition</div>
                    <div className="mt-1 text-xs text-text-muted">
                      Include nodes, edges, and viewport in the returned pipeline data.
                    </div>
                  </div>
                </label>
              </>
            ) : nodeType === 'action:pipeline_run' ? (
              <>
                <div>
                  <Label>Pipeline</Label>
                  <Select
                    value={(localConfig.pipelineId as string) || ''}
                    onChange={(e) => handleConfigChange('pipelineId', e.target.value)}
                  >
                    <option value="">Select pipeline</option>
                    {runnablePipelines.map((pipeline) => (
                      <option key={pipeline.id} value={pipeline.id}>
                        {pipeline.name}
                      </option>
                    ))}
                  </Select>
                </div>
                <div>
                  <FieldLabel
                    tooltip={(
                      <>
                        <p>This field is templated before it is parsed as JSON.</p>
                        <p>After rendering, it must decode to a JSON object.</p>
                        <p>If left empty, the current input object is passed through to the called pipeline.</p>
                        <p>The called pipeline receives these keys at the top level of its input payload.</p>
                      </>
                    )}
                  >
                    Parameters JSON
                  </FieldLabel>
                  <TemplateTextarea
                    value={(localConfig.params as string) || ''}
                    onChange={(e) => handleConfigChange('params', e.target.value)}
                    placeholder='{"message":"hello","target":"ops"}'
                    rows={6}
                    className="font-mono text-xs"
                    suggestions={templateSuggestions}
                  />
                </div>
              </>
            ) : nodeType === 'action:lua' ? (
              <div className="space-y-3">
                <FieldLabel
                  tooltip={(
                    <>
                      <p>The script runs directly, not inside a <span className="font-mono text-slate-200">function(input)</span> wrapper.</p>
                      <p><span className="font-mono text-slate-200">input</span> contains the full current payload, and top-level input keys are also available as globals.</p>
                      <p>Prefer <span className="font-mono text-slate-200">input.field</span> when reading values so the data source stays explicit.</p>
                      <p>Lua arrays are 1-based, so the first item is <span className="font-mono text-slate-200">input.items[1]</span>.</p>
                      <p>Return a table for structured output. Returning a primitive becomes <span className="font-mono text-slate-200">result</span> downstream.</p>
                    </>
                  )}
                >
                  Lua Script
                </FieldLabel>
                <div className="rounded-lg border border-border bg-bg-input px-3 py-3">
                  <p className="text-sm text-text">
                    {((localConfig.script as string) || '').trim()
                      ? 'Open the full-screen editor to update the Lua script.'
                      : 'No Lua script yet. Open the editor to add one.'}
                  </p>
                  <p className="mt-1 text-xs text-text-dimmed">
                    {(((localConfig.script as string) || '').split(/\r?\n/).filter(Boolean).length || 0)} lines saved
                  </p>
                  <div className="mt-3 rounded-md border border-border/70 bg-bg-overlay/70 px-2.5 py-2 text-xs text-text-dimmed">
                    Read runtime data from <span className="font-mono text-text">input</span>, prefer <span className="font-mono text-text">input.field</span> over implicit globals, remember Lua arrays are 1-based, and return a table when you want named output fields.
                  </div>
                </div>
                <div className="flex justify-end">
                  <Button variant="secondary" onClick={() => setIsLuaEditorOpen(true)}>
                    <Code className="w-4 h-4" />
                    Edit code
                  </Button>
                </div>
              </div>
            ) : nodeType === 'tool:pipeline_list' ? (
              <div className="space-y-3">
                <div className="flex items-center gap-2 rounded-lg border border-border bg-bg-input px-3 py-2 text-sm text-text-muted">
                  <span>No additional configuration.</span>
                  <HelpTooltip
                    content={(
                      <>
                        <p>This tool exposes the list of available pipelines to a connected agent node.</p>
                        <p>
                          The agent can optionally pass
                          {' '}
                          <span className="font-mono text-slate-200">pipelineId</span>
                          ,
                          {' '}
                          <span className="font-mono text-slate-200">pipelineName</span>
                          , and
                          {' '}
                          <span className="font-mono text-slate-200">includeDefinition</span>
                          {' '}
                          when it needs full nodes, edges, and viewport for editing.
                        </p>
                      </>
                    )}
                  />
                </div>
              </div>
            ) : nodeType === 'tool:pipeline_get' ? (
              <>
                <div>
                  <Label>Tool Name</Label>
                  <Input
                    value={(localConfig.toolName as string) || ''}
                    onChange={(e) => handleConfigChange('toolName', e.target.value)}
                    placeholder="Optional custom function name"
                  />
                </div>
                <div>
                  <Label>Tool Description</Label>
                  <TemplateTextarea
                    value={(localConfig.toolDescription as string) || ''}
                    onChange={(e) => handleConfigChange('toolDescription', e.target.value)}
                    placeholder="Explain when the model should use this tool."
                    rows={4}
                    suggestions={templateSuggestions}
                  />
                </div>
                <label className="flex items-start gap-3 rounded-lg border border-border bg-bg-input px-3 py-2">
                  <Checkbox
                    checked={Boolean(localConfig.allowModelPipelineId)}
                    onChange={(e) => handleConfigChange('allowModelPipelineId', e.target.checked)}
                    className="mt-0.5"
                  />
                  <div className="min-w-0">
                    <div className="text-sm font-medium text-text">Let model choose pipeline ID</div>
                    <div className="mt-1 text-xs text-text-muted">
                      Expose a <span className="font-mono text-text">pipelineId</span> argument in the tool schema so the model can select the target pipeline dynamically.
                    </div>
                  </div>
                </label>
                <div>
                  <Label>{Boolean(localConfig.allowModelPipelineId) ? 'Default Pipeline' : 'Pipeline'}</Label>
                  <Select
                    value={(localConfig.pipelineId as string) || ''}
                    onChange={(e) => handleConfigChange('pipelineId', e.target.value)}
                  >
                    <option value="">{Boolean(localConfig.allowModelPipelineId) ? 'Optional default pipeline' : 'Select pipeline'}</option>
                    {pipelineOptions.map((pipeline) => (
                      <option key={pipeline.id} value={pipeline.id}>
                        {pipeline.name}
                      </option>
                    ))}
                  </Select>
                </div>
                <label className="flex items-start gap-3 rounded-lg border border-border bg-bg-input px-3 py-2">
                  <Checkbox
                    checked={Boolean(localConfig.includeDefinition ?? true)}
                    onChange={(e) => handleConfigChange('includeDefinition', e.target.checked)}
                    className="mt-0.5"
                  />
                  <div className="min-w-0">
                    <div className="text-sm font-medium text-text">Include full definition</div>
                    <div className="mt-1 text-xs text-text-muted">
                      Return nodes, edges, and viewport together with the pipeline metadata.
                    </div>
                  </div>
                </label>
              </>
            ) : pipelineMutationToolNodeTypes.has(nodeType) ? (
              <>
                <div>
                  <Label>Tool Name</Label>
                  <Input
                    value={(localConfig.toolName as string) || ''}
                    onChange={(e) => handleConfigChange('toolName', e.target.value)}
                    placeholder="Optional custom function name"
                  />
                </div>
                <div>
                  <FieldLabel
                    tooltip={nodeType === 'tool:pipeline_create'
                      ? 'Use this description to tell the model when it should create a new pipeline.'
                      : nodeType === 'tool:pipeline_update'
                      ? 'Use this description to tell the model when it should update an existing pipeline.'
                      : 'Use this description to tell the model when it should delete a pipeline.'}
                  >
                    Tool Description
                  </FieldLabel>
                  <TemplateTextarea
                    value={(localConfig.toolDescription as string) || ''}
                    onChange={(e) => handleConfigChange('toolDescription', e.target.value)}
                    placeholder="Explain when the model should use this tool."
                    rows={4}
                    suggestions={templateSuggestions}
                  />
                </div>
              </>
            ) : nodeType === 'tool:pipeline_run' ? (
              <>
                <div>
                  <Label>Tool Name</Label>
                  <Input
                    value={(localConfig.toolName as string) || ''}
                    onChange={(e) => handleConfigChange('toolName', e.target.value)}
                    placeholder="Optional custom function name"
                  />
                </div>
                <div>
                  <FieldLabel
                    tooltip="Use this description to explain when the model should call this tool. The tool can always pass a params object as manual-run input."
                  >
                    Tool Description
                  </FieldLabel>
                  <TemplateTextarea
                    value={(localConfig.toolDescription as string) || ''}
                    onChange={(e) => handleConfigChange('toolDescription', e.target.value)}
                    placeholder="Explain when the model should use this tool."
                    rows={4}
                    suggestions={templateSuggestions}
                  />
                </div>
                <label className="flex items-start gap-3 rounded-lg border border-border bg-bg-input px-3 py-2">
                  <Checkbox
                    checked={Boolean(localConfig.allowModelPipelineId)}
                    onChange={(e) => handleConfigChange('allowModelPipelineId', e.target.checked)}
                    className="mt-0.5"
                  />
                  <div className="min-w-0">
                    <div className="text-sm font-medium text-text">Let model choose pipeline ID</div>
                    <div className="mt-1 text-xs text-text-muted">
                      Expose a <span className="font-mono text-text">pipelineId</span> argument in the tool schema so the model can select the target pipeline dynamically.
                    </div>
                  </div>
                </label>
                <div>
                  <FieldLabel
                    tooltip={(
                      <>
                        <p>The tool always runs a pipeline manually and can pass a top-level <span className="font-mono text-slate-200">params</span> object as execution input.</p>
                        <p>If the called pipeline reaches a Return node, that returned value is sent back to the agent.</p>
                      </>
                    )}
                  >
                    {Boolean(localConfig.allowModelPipelineId) ? 'Default Pipeline' : 'Pipeline'}
                  </FieldLabel>
                  <Select
                    value={(localConfig.pipelineId as string) || ''}
                    onChange={(e) => handleConfigChange('pipelineId', e.target.value)}
                  >
                    <option value="">{Boolean(localConfig.allowModelPipelineId) ? 'Optional default pipeline' : 'Select pipeline'}</option>
                    {runnablePipelines.map((pipeline) => (
                      <option key={pipeline.id} value={pipeline.id}>
                        {pipeline.name}
                      </option>
                    ))}
                  </Select>
                </div>
                <div className="space-y-3 rounded-xl border border-border bg-bg-input/70 p-3">
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <div className="flex items-center gap-2">
                        <Label className="mb-0">Additional Arguments</Label>
                        <HelpTooltip
                          content={(
                            <>
                              <p>Each argument becomes a tool parameter the model can send.</p>
                              <p>
                                Values are passed into the called pipeline as
                                {' '}
                                <span className="font-mono text-slate-200">{'{{arguments.name}}'}</span>
                                .
                              </p>
                            </>
                          )}
                        />
                      </div>
                    </div>
                    <Button variant="secondary" size="sm" onClick={handlePipelineToolArgumentAdd}>
                      <Plus className="w-4 h-4" />
                      Add parameter
                    </Button>
                  </div>
                  {pipelineToolArguments.length > 0 ? (
                    pipelineToolArguments.map((argument, index) => (
                      <div key={`${argument.name}-${index}`} className="space-y-3 rounded-xl border border-border bg-bg-overlay/70 p-3">
                        <div className="flex items-start gap-2">
                          <div className="flex-1">
                            <Label>Parameter Name</Label>
                            <Input
                              value={argument.name}
                              onChange={(e) => handlePipelineToolArgumentChange(index, 'name', e.target.value)}
                              placeholder="argument_name"
                            />
                          </div>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="mt-6"
                            onClick={() => handlePipelineToolArgumentRemove(index)}
                            title="Remove parameter"
                          >
                            <Trash2 className="w-4 h-4 text-red-400" />
                          </Button>
                        </div>
                        <div>
                          <Label>Description</Label>
                          <Input
                            value={argument.description}
                            onChange={(e) => handlePipelineToolArgumentChange(index, 'description', e.target.value)}
                            placeholder="Explain what the model should pass here."
                          />
                        </div>
                        <label className="flex items-start gap-3 rounded-lg border border-border bg-bg-input px-3 py-2">
                          <Checkbox
                            checked={argument.required}
                            onChange={(e) => handlePipelineToolArgumentChange(index, 'required', e.target.checked)}
                            className="mt-0.5"
                          />
                          <div className="min-w-0">
                            <div className="text-sm font-medium text-text">Required parameter</div>
                            <div className="mt-1 text-xs text-text-muted">
                              The tool call must include this parameter.
                            </div>
                          </div>
                        </label>
                      </div>
                    ))
                  ) : (
                    <div className="rounded-lg border border-dashed border-border bg-bg-overlay/70 px-3 py-2 text-sm text-text-muted">
                      No additional parameters yet. Use the button above to expose named arguments to the model.
                    </div>
                  )}
                </div>
              </>
            ) : nodeType === 'visual:group' ? (
              <div className="space-y-3">
                <div className="rounded-lg border border-border bg-bg-input px-3 py-2 text-sm text-text-muted">
                  Groups are visual only. Set a title in the General tab, and choose the box color here.
                </div>
                <div>
                  <Label>Group Color</Label>
                  <div className="flex items-center gap-3">
                    <input
                      type="color"
                      value={groupColor}
                      onChange={(e) => handleConfigChange('color', e.target.value)}
                      className="h-11 w-14 cursor-pointer rounded-lg border border-border bg-bg-input p-1"
                    />
                    <Input
                      value={groupColor}
                      onChange={(e) => handleConfigChange('color', normalizeGroupColor(e.target.value))}
                      placeholder={DEFAULT_GROUP_COLOR}
                      className="font-mono"
                    />
                  </div>
                </div>
              </div>
            ) : nodeType === 'logic:condition' ? (
              <div>
                <ExpressionLabel />
                <TemplateTextarea
                  value={(localConfig.expression as string) || ''}
                  onChange={(e) => handleConfigChange('expression', e.target.value)}
                  placeholder="e.g., input.status == 'running'"
                  rows={3}
                  className="font-mono text-xs"
                  suggestions={templateSuggestions}
                />
              </div>
            ) : nodeType === 'logic:switch' ? (
              <div className="space-y-3">
                <div className="rounded-lg border border-border bg-bg-input px-3 py-2 text-sm text-text-muted">
                  Every condition gets its own output pin. All branches that evaluate to <span className="font-medium text-text">true</span> will fire, and the <span className="font-medium text-text">Else</span> pin runs when none match.
                </div>
                {switchConditions.map((condition) => (
                  <div key={condition.id} className="space-y-3 rounded-xl border border-border bg-bg-input/80 p-3">
                    <div className="flex items-start gap-2">
                      <div className="flex-1">
                        <Label>Branch Label</Label>
                        <Input
                          value={condition.label}
                          onChange={(e) => handleSwitchConditionChange(condition.id, 'label', e.target.value)}
                          placeholder="e.g., Healthy"
                        />
                      </div>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="mt-6"
                        onClick={() => handleSwitchConditionRemove(condition.id)}
                        title="Remove condition"
                      >
                        <Trash2 className="w-4 h-4 text-red-400" />
                      </Button>
                    </div>
                    <div>
                      <ExpressionLabel />
                      <TemplateTextarea
                        value={condition.expression}
                        onChange={(e) => handleSwitchConditionChange(condition.id, 'expression', e.target.value)}
                        placeholder="e.g., input.status_code == 200"
                        rows={3}
                        className="font-mono text-xs"
                        suggestions={templateSuggestions}
                      />
                    </div>
                    <div className="rounded-md border border-border/70 bg-bg-overlay/70 px-2.5 py-2 text-xs text-text-dimmed">
                      Handle ID: <span className="font-mono text-text">{condition.id}</span>
                    </div>
                  </div>
                ))}
                <Button variant="secondary" onClick={handleSwitchConditionAdd}>
                  <Plus className="w-4 h-4" />
                  Add condition
                </Button>
              </div>
            ) : nodeType === 'logic:merge' ? (
              <>
                <div>
                  <FieldLabel
                    tooltip={(
                      <>
                        <p>Merge waits for every connected upstream branch, then combines object outputs into one payload.</p>
                        <p>
                          Downstream nodes get the merged fields directly, plus
                          {' '}
                          <span className="font-mono text-slate-200">merged</span>
                          {' '}
                          and
                          {' '}
                          <span className="font-mono text-slate-200">entries</span>
                          .
                        </p>
                      </>
                    )}
                  >
                    Mode
                  </FieldLabel>
                  <Select
                    value={(localConfig.mode as string) || 'shallow'}
                    onChange={(e) => handleConfigChange('mode', e.target.value)}
                  >
                    <option value="shallow">Shallow merge</option>
                    <option value="deep">Deep merge</option>
                  </Select>
                </div>
              </>
            ) : nodeType === 'logic:aggregate' ? (
              <div className="space-y-3">
                <div className="flex items-center gap-2 rounded-lg border border-border bg-bg-input px-3 py-2 text-sm text-text-muted">
                  <span>Aggregate waits for every connected upstream branch.</span>
                  <HelpTooltip
                    content={(
                      <>
                        <p><span className="font-mono text-slate-200">items</span>: ordered list of upstream outputs.</p>
                        <p><span className="font-mono text-slate-200">entries</span>: upstream outputs with node ids, labels, and types.</p>
                        <p><span className="font-mono text-slate-200">byNodeId</span>: upstream outputs keyed by the resolved output id.</p>
                        <p>
                          <span className="font-mono text-slate-200">idOverrides</span>
                          {' '}
                          lets you rename those source ids in the aggregate output without changing the actual graph node ids.
                        </p>
                      </>
                    )}
                  />
                </div>
                {aggregateInputs.length === 0 ? (
                  <div className="rounded-lg border border-dashed border-border bg-bg-overlay/70 px-3 py-2 text-sm text-text-muted">
                    Connect upstream nodes to optionally rewrite their ids in
                    {' '}
                    <span className="font-mono text-text">entries</span>
                    {' '}
                    and
                    {' '}
                    <span className="font-mono text-text">byNodeId</span>
                    .
                  </div>
                ) : (
                  <div className="space-y-3">
                    <FieldLabel
                      tooltip={(
                        <>
                          <p>These overrides only affect the aggregate output payload.</p>
                          <p>
                            The actual connected node ids stay the same, and overridden entries keep their real id in
                            {' '}
                            <span className="font-mono text-slate-200">originalNodeId</span>
                            .
                          </p>
                        </>
                      )}
                    >
                      Output ID Overrides
                    </FieldLabel>
                    {aggregateInputs.map((source) => (
                      <div key={source.nodeId} className="space-y-3 rounded-xl border border-border bg-bg-input/80 p-3">
                        <div className="flex items-start justify-between gap-3">
                          <div className="min-w-0">
                            <div className="truncate text-sm font-medium text-text">{source.label}</div>
                            <div className="mt-1 text-xs text-text-dimmed">
                              <span className="font-mono text-text">{source.nodeId}</span>
                              {source.nodeType && (
                                <>
                                  {' '}
                                  · {source.nodeType}
                                </>
                              )}
                              {source.sourceHandles.length > 0 && (
                                <>
                                  {' '}
                                  · handles: {source.sourceHandles.join(', ')}
                                </>
                              )}
                            </div>
                          </div>
                        </div>
                        <div>
                          <Label>Output ID</Label>
                          <Input
                            value={aggregateIDOverrides[source.nodeId] || ''}
                            onChange={(e) => handleAggregateIDOverrideChange(source.nodeId, e.target.value)}
                            placeholder={source.nodeId}
                            className="font-mono"
                          />
                          <p className="mt-1 text-xs text-text-dimmed">
                            Leave empty to keep
                            {' '}
                            <span className="font-mono text-text">{source.nodeId}</span>
                            .
                          </p>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
                {aggregateUnusedOverrideKeys.length > 0 && (
                  <div className="rounded-lg border border-border bg-bg-input px-3 py-2 text-xs text-text-dimmed">
                    Unused overrides are being kept for disconnected inputs:
                    {' '}
                    <span className="font-mono text-text">{aggregateUnusedOverrideKeys.join(', ')}</span>
                  </div>
                )}
              </div>
            ) : nodeType === 'logic:return' ? (
              <>
                <div>
                  <FieldLabel tooltip="This node stops the pipeline and returns data to the caller. Leave the value empty to return the full current input object.">
                    Return Value
                  </FieldLabel>
                  <TemplateTextarea
                    value={(localConfig.value as string) || ''}
                    onChange={(e) => handleConfigChange('value', e.target.value)}
                    placeholder='{{input}} or {"status":"ok","message":{{input.message}}}'
                    rows={5}
                    className="font-mono text-xs"
                    suggestions={templateSuggestions}
                  />
                </div>
              </>
            ) : nodeType === 'llm:prompt' ? (
              <>
                <div>
                  <Label>Provider</Label>
                  <Select
                    value={(localConfig.providerId as string) || ''}
                    onChange={(e) => handleConfigChange('providerId', e.target.value)}
                  >
                    <option value="">Default provider{defaultProvider ? ` (${defaultProvider.name})` : ''}</option>
                    {llmProviders?.map((provider) => (
                      <option key={provider.id} value={provider.id}>
                        {provider.name} ({provider.provider_type})
                      </option>
                    ))}
                  </Select>
                </div>
                <div>
                  <Label>Prompt</Label>
                  <TemplateTextarea
                    value={(localConfig.prompt as string) || ''}
                    onChange={(e) => handleConfigChange('prompt', e.target.value)}
                    placeholder="Enter your prompt template..."
                    rows={6}
                    suggestions={promptInsertSuggestions}
                  />
                </div>
                <div>
                  <Label>Model</Label>
                  {modelOptions.length > 0 ? (
                    <Select
                      value={(localConfig.model as string) || ''}
                      onChange={(e) => handleConfigChange('model', e.target.value)}
                    >
                      <option value="">Use provider default model</option>
                      {modelOptions.map((model) => (
                        <option key={model.id} value={model.id}>
                          {model.name || model.id}
                        </option>
                      ))}
                    </Select>
                  ) : (
                    <TemplateInput
                      value={(localConfig.model as string) || ''}
                      onChange={(e) => handleConfigChange('model', e.target.value)}
                      placeholder="e.g., openai/gpt-4o-mini"
                      suggestions={templateSuggestions}
                    />
                  )}
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <Label>Temperature</Label>
                    <Input
                      type="number"
                      step="0.1"
                      min="0"
                      max="2"
                      value={(localConfig.temperature as number) ?? 0.7}
                      onChange={(e) => handleConfigChange('temperature', parseFloat(e.target.value))}
                    />
                  </div>
                  <div>
                    <Label>Max Tokens</Label>
                    <Input
                      type="number"
                      value={(localConfig.max_tokens as number) ?? 1024}
                      onChange={(e) => handleConfigChange('max_tokens', parseInt(e.target.value))}
                    />
                  </div>
                </div>
              </>
            ) : nodeType === 'llm:agent' ? (
              <>
                <div>
                  <Label>Provider</Label>
                  <Select
                    value={(localConfig.providerId as string) || ''}
                    onChange={(e) => handleConfigChange('providerId', e.target.value)}
                  >
                    <option value="">Default provider{defaultProvider ? ` (${defaultProvider.name})` : ''}</option>
                    {llmProviders?.map((provider) => (
                      <option key={provider.id} value={provider.id}>
                        {provider.name} ({provider.provider_type})
                      </option>
                    ))}
                  </Select>
                </div>
                <div className="flex items-center justify-between rounded-lg border border-border bg-bg-input px-3 py-2">
                  <div className="flex items-center gap-2 text-sm text-text-muted">
                    <span>Connected tools</span>
                    <HelpTooltip content="Connect tool nodes from the blue pin on the bottom of the agent node to the top pin on each tool node to make them available during multi-turn execution." />
                  </div>
                  <span className="text-sm font-medium text-text">{connectedToolCount}</span>
                </div>
                <label className="flex items-start gap-3 rounded-lg border border-border bg-bg-input px-3 py-2">
                  <Checkbox
                    checked={Boolean(localConfig.enableSkills)}
                    onChange={(e) => handleConfigChange('enableSkills', e.target.checked)}
                    className="mt-0.5"
                  />
                  <div className="min-w-0">
                    <div className="flex items-center gap-2 text-sm font-medium text-text">
                      <span>Enable local skills</span>
                      <HelpTooltip
                        content={(
                          <>
                            <p>Adds the <span className="font-mono text-slate-200">get_skill</span> tool to this agent.</p>
                            <p>
                              It also lets you insert
                              {' '}
                              <span className="font-mono text-slate-200">{'{{skills}}'}</span>
                              {' '}
                              into the prompt.
                            </p>
                          </>
                        )}
                      />
                    </div>
                  </div>
                </label>
                <div>
                  <Label>Instructions</Label>
                  <TemplateTextarea
                    value={(localConfig.prompt as string) || ''}
                    onChange={(e) => handleConfigChange('prompt', e.target.value)}
                    placeholder="Describe the agent's role, goals, and how it should use connected tools."
                    rows={7}
                    suggestions={agentTemplateSuggestions}
                  />
                </div>
                <div>
                  <Label>Model</Label>
                  {modelOptions.length > 0 ? (
                    <Select
                      value={(localConfig.model as string) || ''}
                      onChange={(e) => handleConfigChange('model', e.target.value)}
                    >
                      <option value="">Use provider default model</option>
                      {modelOptions.map((model) => (
                        <option key={model.id} value={model.id}>
                          {model.name || model.id}
                        </option>
                      ))}
                    </Select>
                  ) : (
                    <TemplateInput
                      value={(localConfig.model as string) || ''}
                      onChange={(e) => handleConfigChange('model', e.target.value)}
                      placeholder="e.g., openai/gpt-4o-mini"
                      suggestions={agentTemplateSuggestions}
                    />
                  )}
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <Label>Temperature</Label>
                    <Input
                      type="number"
                      step="0.1"
                      min="0"
                      max="2"
                      value={(localConfig.temperature as number) ?? 0.7}
                      onChange={(e) => handleConfigChange('temperature', parseFloat(e.target.value))}
                    />
                  </div>
                  <div>
                    <Label>Max Tokens</Label>
                    <Input
                      type="number"
                      value={(localConfig.max_tokens as number) ?? 1024}
                      onChange={(e) => handleConfigChange('max_tokens', parseInt(e.target.value))}
                    />
                  </div>
                </div>
              </>
            ) : resolvedNodeType.startsWith('action:plugin/') || resolvedNodeType.startsWith('tool:plugin/') ? (
              nodeDef ? (
                <div className="space-y-4">
                  {nodeDef.pluginName && (
                    <div className="rounded-lg border border-border bg-bg-input px-3 py-2 text-xs text-text-dimmed">
                      Plugin: <span className="font-medium text-text">{nodeDef.pluginName}</span>
                    </div>
                  )}
                  {(nodeDef.fields || []).length === 0 ? (
                    <div className="rounded-lg border border-border bg-bg-input px-3 py-2 text-sm text-text-muted">
                      This plugin node does not expose configurable fields.
                    </div>
                  ) : (
                    nodeDef.fields?.map((field) => {
                      const value = pluginFieldValue(field, localConfig)
                      const templateEnabled = field.template_supported !== false
                      const stringValue = typeof value === 'string' ? value : value == null ? '' : String(value)

                      if (field.type === 'boolean') {
                        return (
                          <label key={field.name} className="flex items-start gap-3 rounded-lg border border-border bg-bg-input px-3 py-2">
                            <Checkbox
                              checked={Boolean(value)}
                              onChange={(event) => handleConfigChange(field.name, event.target.checked)}
                              className="mt-0.5"
                            />
                            <div className="min-w-0">
                              <div className="text-sm font-medium text-text">{field.label}</div>
                              {field.description && <div className="mt-1 text-xs text-text-dimmed">{field.description}</div>}
                            </div>
                          </label>
                        )
                      }

                      return (
                        <div key={field.name}>
                          <FieldLabel tooltip={field.description}>{field.label}</FieldLabel>
                          {field.type === 'textarea' ? (
                            <TemplateTextarea
                              value={stringValue}
                              onChange={(event) => handleConfigChange(field.name, event.target.value)}
                              placeholder={field.placeholder}
                              rows={5}
                              suggestions={templateEnabled ? templateSuggestions : []}
                            />
                          ) : field.type === 'number' ? (
                            <Input
                              type="number"
                              value={typeof value === 'number' ? value : Number(value) || 0}
                              onChange={(event) => handleConfigChange(field.name, Number(event.target.value))}
                            />
                          ) : field.type === 'select' ? (
                            <Select
                              value={stringValue}
                              onChange={(event) => handleConfigChange(field.name, event.target.value)}
                            >
                              {(field.options || []).map((option) => (
                                <option key={option.value} value={option.value}>
                                  {option.label}
                                </option>
                              ))}
                            </Select>
                          ) : field.type === 'json' ? (
                            <TemplateTextarea
                              value={typeof value === 'string' ? value : JSON.stringify(value ?? {}, null, 2)}
                              onChange={(event) => {
                                const nextValue = event.target.value
                                try {
                                  handleConfigChange(field.name, JSON.parse(nextValue))
                                } catch {
                                  handleConfigChange(field.name, nextValue)
                                }
                              }}
                              placeholder={field.placeholder || '{\n  "key": "value"\n}'}
                              rows={6}
                              className="font-mono text-xs"
                              suggestions={templateEnabled ? templateSuggestions : []}
                            />
                          ) : templateEnabled ? (
                            <TemplateInput
                              value={stringValue}
                              onChange={(event) => handleConfigChange(field.name, event.target.value)}
                              placeholder={field.placeholder}
                              suggestions={templateSuggestions}
                            />
                          ) : (
                            <Input
                              value={stringValue}
                              onChange={(event) => handleConfigChange(field.name, event.target.value)}
                              placeholder={field.placeholder}
                            />
                          )}
                        </div>
                      )
                    })
                  )}
                </div>
              ) : (
                <div className="space-y-3">
                  <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-200">
                    This plugin node is currently unavailable. You can keep editing the pipeline as a draft, but activation and execution will fail until the plugin is installed again.
                  </div>
                  <div className="rounded-lg border border-border bg-bg-input px-3 py-2 text-xs text-text-dimmed">
                    Node type: <span className="font-mono text-text">{resolvedNodeType}</span>
                  </div>
                </div>
              )
            ) : nodeType === 'trigger:cron' ? (
              <>
                <div>
                  <Label>Cron Expression</Label>
                  <TemplateInput
                    value={(localConfig.schedule as string) || ''}
                    onChange={(e) => handleConfigChange('schedule', e.target.value)}
                    placeholder="e.g., 0 * * * *"
                    suggestions={templateSuggestions}
                  />
                </div>
                <div>
                  <Label>Timezone</Label>
                  <TemplateInput
                    value={(localConfig.timezone as string) || 'UTC'}
                    onChange={(e) => handleConfigChange('timezone', e.target.value)}
                    placeholder="e.g., UTC, America/New_York"
                    suggestions={templateSuggestions}
                  />
                </div>
              </>
            ) : nodeType === 'trigger:webhook' ? (
              <>
                <div>
                  <Label>Path</Label>
                  <TemplateInput
                    value={(localConfig.path as string) || ''}
                    onChange={(e) => handleConfigChange('path', e.target.value)}
                    placeholder="/webhook/my-endpoint"
                    suggestions={templateSuggestions}
                  />
                </div>
                <div>
                  <Label>Method</Label>
                  <select
                    value={(localConfig.method as string) || 'POST'}
                    onChange={(e) => handleConfigChange('method', e.target.value)}
                    className="w-full px-3 py-2 bg-bg-input border border-border rounded-lg text-text text-sm focus:outline-none focus:ring-2 focus:ring-accent/50 focus:border-accent"
                  >
                    <option value="POST">POST</option>
                    <option value="GET">GET</option>
                  </select>
                </div>
              </>
            ) : (
              <div className="text-center py-8">
                <Settings className="w-8 h-8 text-text-dimmed mx-auto mb-2" />
                <p className="text-sm text-text-muted">No configuration options</p>
              </div>
            )}
          </div>
        )}
      </div>

      {nodeType === 'action:lua' && isLuaEditorOpen && (
        <LuaEditorModal
          value={(localConfig.script as string) || ''}
          suggestions={templateSuggestions}
          onSave={(value) => handleConfigChange('script', value)}
          onClose={() => setIsLuaEditorOpen(false)}
        />
      )}
    </div>
  )
}
