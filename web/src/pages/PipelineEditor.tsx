import { useCallback, useRef, useState, useEffect, type MouseEvent as ReactMouseEvent } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  useNodesState,
  useEdgesState,
  applyNodeChanges,
  addEdge,
  type OnConnect,
  type Node,
  type Edge,
  type OnNodesChange,
  type OnEdgesChange,
  MarkerType,
  Panel,
  useReactFlow,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { Save, Play, ArrowLeft, Loader2, Trash2, Copy, ListChecks, Scissors, Edit2, GitBranch, Zap, Clock, Webhook, Square, Globe, Code, Split, Brain, Link, MessageSquare, Send, Power, Bot, Workflow, List, Wrench, CornerDownLeft, RefreshCw, Server, Shield, Download, FileJson, ChevronDown, ChevronUp, Plus, Minus, ScanSearch, Lock, LockOpen } from 'lucide-react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'

import NodePalette from '../components/flow/NodePalette'
import NodeConfigPanel from '../components/flow/NodeConfigPanel'
import ExecutionLog from '../components/flow/ExecutionLog'
import NodeExecutionModal from '../components/flow/NodeExecutionModal'
import EditorAssistantDock from '../components/flow/EditorAssistantDock'
import AutomatorNode from '../components/flow/nodes/AutomatorNode'
import AutomatorEdge from '../components/flow/edges/AutomatorEdge'
import { NODE_CATEGORIES } from '../components/flow/nodeTypes'
import { DEFAULT_NODE_BORDER_COLOR, getNodeBorderTint } from '../components/flow/nodeAppearance'
import { api } from '../api/client'
import { useUIStore } from '../store/ui'
import Button from '../components/ui/Button'
import Badge from '../components/ui/Badge'
import ContextMenu, { type ContextMenuItem } from '../components/ui/ContextMenu'
import Input from '../components/ui/Input'
import { Textarea } from '../components/ui/Form'
import Modal from '../components/ui/Modal'
import { buildPipelineDocument, extractSingleDefinitionDocument } from '../lib/documents'
import { downloadJSON, sanitizeFilename } from '../lib/download'
import { applyLivePipelineOperations } from '../lib/editorAssistant'
import { cn } from '../lib/utils'
import type { EditorAssistantExecutionLogAttachment, ExecutionDetail, FlowDefinitionDocument, LLMProvider, LivePipelineOperation, NodeExecutionLogData, Pipeline, PipelineRunResponse, NodeType, TemplateSummary } from '../types'

const nodeTypes = {
  automator: AutomatorNode,
}

const edgeTypes = {
  automator: AutomatorEdge,
}

const defaultEdgeOptions = {
  type: 'automator',
  markerEnd: {
    type: MarkerType.ArrowClosed,
    color: '#1e2d3d',
  },
  style: {
    stroke: '#1e2d3d',
    strokeWidth: 2,
  },
}

const toolEdgeOptions = {
  type: 'automator',
  markerEnd: {
    type: MarkerType.ArrowClosed,
    color: '#38bdf8',
  },
  style: {
    stroke: '#38bdf8',
    strokeWidth: 2,
    strokeDasharray: '8 4',
  },
}

const nodeMenuIconMap: Record<string, React.ElementType> = {
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

const categoryMenuIconMap: Record<string, React.ElementType> = {
  trigger: Zap,
  action: Play,
  tool: Wrench,
  logic: GitBranch,
  llm: Brain,
}

function isProxmoxNodeType(type: NodeType): boolean {
  return [
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
  ].includes(type)
}

function isKubernetesNodeType(type: NodeType): boolean {
  return type.includes(':kubernetes_')
}

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) {
    return false
  }

  if (target.isContentEditable) {
    return true
  }

  return Boolean(target.closest('input, textarea, select, [contenteditable="true"]'))
}

type EditorContextMenuState = {
  x: number
  y: number
  items: ContextMenuItem[]
  searchable?: boolean
  searchPlaceholder?: string
  emptyMessage?: string
}

const VISUAL_GROUP_TYPE: NodeType = 'visual:group'
const DEFAULT_GROUP_COLOR = '#64748b'
const GROUP_PADDING_X = 32
const GROUP_PADDING_TOP = 56
const GROUP_PADDING_BOTTOM = 28
const MIN_GROUP_WIDTH = 280
const MIN_GROUP_HEIGHT = 180
const DEFAULT_NODE_WIDTH = 220
const DEFAULT_NODE_HEIGHT = 120
const DUPLICATE_SELECTION_OFFSET = { x: 40, y: 40 }

type NodeBounds = {
  x: number
  y: number
  width: number
  height: number
}

function isVisualGroupNodeType(type: unknown): type is NodeType {
  return type === VISUAL_GROUP_TYPE
}

function isGroupNode(node: Node | null | undefined): boolean {
  return isVisualGroupNodeType(node?.data?.type)
}

function getNodeWidth(node: Node): number {
  if (typeof node.measured?.width === 'number') return node.measured.width
  if (typeof node.width === 'number') return node.width
  if (typeof node.initialWidth === 'number') return node.initialWidth

  const styleWidth = node.style?.width
  if (typeof styleWidth === 'number') return styleWidth

  return DEFAULT_NODE_WIDTH
}

function getNodeHeight(node: Node): number {
  if (typeof node.measured?.height === 'number') return node.measured.height
  if (typeof node.height === 'number') return node.height
  if (typeof node.initialHeight === 'number') return node.initialHeight

  const styleHeight = node.style?.height
  if (typeof styleHeight === 'number') return styleHeight

  return DEFAULT_NODE_HEIGHT
}

function getAbsoluteNodePosition(node: Node, nodesById: Map<string, Node>): { x: number; y: number } {
  let x = node.position.x
  let y = node.position.y
  let parentId = node.parentId
  const visited = new Set<string>()

  while (parentId) {
    if (visited.has(parentId)) {
      break
    }

    visited.add(parentId)

    const parent = nodesById.get(parentId)
    if (!parent) {
      break
    }

    x += parent.position.x
    y += parent.position.y
    parentId = parent.parentId
  }

  return { x, y }
}

function getNodeBounds(node: Node, nodesById: Map<string, Node>): NodeBounds {
  const absolutePosition = getAbsoluteNodePosition(node, nodesById)

  return {
    ...absolutePosition,
    width: getNodeWidth(node),
    height: getNodeHeight(node),
  }
}

function isPointInsideBounds(
  point: { x: number; y: number },
  bounds: NodeBounds,
  padding = 0,
): boolean {
  return (
    point.x >= bounds.x + padding
    && point.x <= bounds.x + bounds.width - padding
    && point.y >= bounds.y + padding
    && point.y <= bounds.y + bounds.height - padding
  )
}

function findContainingGroup(node: Node, nodes: Node[]): Node | null {
  const nodesById = new Map(nodes.map((candidate) => [candidate.id, candidate]))
  const nodeBounds = getNodeBounds(node, nodesById)
  const center = {
    x: nodeBounds.x + (nodeBounds.width / 2),
    y: nodeBounds.y + (nodeBounds.height / 2),
  }

  const containingGroups = nodes
    .filter((candidate) => candidate.id !== node.id && isGroupNode(candidate))
    .map((group) => ({
      group,
      bounds: getNodeBounds(group, nodesById),
    }))
    .filter(({ bounds }) => isPointInsideBounds(center, bounds, 8))
    .sort((left, right) => (
      (left.bounds.width * left.bounds.height) - (right.bounds.width * right.bounds.height)
    ))

  return containingGroups[0]?.group ?? null
}

function getMinimumGroupDimensions(group: Node, nodes: Node[]): { width: number; height: number } {
  const children = nodes.filter((node) => node.parentId === group.id)

  if (children.length === 0) {
    return {
      width: MIN_GROUP_WIDTH,
      height: MIN_GROUP_HEIGHT,
    }
  }

  const maxChildRight = Math.max(...children.map((node) => node.position.x + getNodeWidth(node)))
  const maxChildBottom = Math.max(...children.map((node) => node.position.y + getNodeHeight(node)))

  return {
    width: Math.max(maxChildRight + GROUP_PADDING_X, MIN_GROUP_WIDTH),
    height: Math.max(maxChildBottom + GROUP_PADDING_BOTTOM, MIN_GROUP_HEIGHT),
  }
}

function getNextGroupLabel(nodes: Node[]): string {
  const count = nodes.filter((node) => isGroupNode(node)).length
  return `Group ${count + 1}`
}

function canGroupNodes(nodes: Node[]): boolean {
  return nodes.length > 1 && nodes.every((node) => !isGroupNode(node) && !node.parentId)
}

function normalizeNodesForSubflows(nodes: Node[]): Node[] {
  const nodesById = new Map(nodes.map((node) => [node.id, node]))
  const childrenByParentId = new Map<string, Node[]>()

  nodes.forEach((node) => {
    if (!node.parentId || !nodesById.has(node.parentId)) {
      return
    }

    const siblings = childrenByParentId.get(node.parentId) ?? []
    siblings.push(node)
    childrenByParentId.set(node.parentId, siblings)
  })

  const ordered: Node[] = []
  const visited = new Set<string>()

  const visit = (node: Node) => {
    if (visited.has(node.id)) {
      return
    }

    visited.add(node.id)
    ordered.push(node)

    const children = childrenByParentId.get(node.id) ?? []
    children.forEach(visit)
  }

  nodes.forEach((node) => {
    if (node.parentId && nodesById.has(node.parentId)) {
      return
    }

    visit(node)
  })

  nodes.forEach(visit)

  return ordered
}

function hydratePersistedNode(rawNode: any): Node {
  return {
    ...rawNode,
    type: 'automator',
    data: {
      ...rawNode?.data,
      type: rawNode?.data?.type || 'trigger:manual',
      config: rawNode?.data?.config || {},
      label: rawNode?.data?.label || 'Node',
    },
  }
}

function hydratePersistedEdge(rawEdge: any): Edge {
  return {
    ...rawEdge,
    ...(rawEdge?.sourceHandle === 'tool' ? toolEdgeOptions : defaultEdgeOptions),
  }
}

function serializeFlowNodes(nodes: Node[]) {
  return normalizeNodesForSubflows(nodes).map(({ type: _type, ...rest }) => rest)
}

function serializeFlowEdges(edges: Edge[]) {
  return edges.map(({ ...rest }) => rest)
}

function buildFlowDefinitionDocument(
  nodes: Node[],
  edges: Edge[],
  viewport: { x: number; y: number; zoom: number },
): FlowDefinitionDocument {
  return {
    nodes: serializeFlowNodes(nodes) as unknown[],
    edges: serializeFlowEdges(edges) as unknown[],
    viewport: JSON.parse(JSON.stringify(viewport)) as Record<string, unknown>,
  }
}

function buildImportedFlow(definition: FlowDefinitionDocument): { nodes: Node[]; edges: Edge[] } {
  return {
    nodes: (definition.nodes || []).map((node) => hydratePersistedNode(node)),
    edges: (definition.edges || []).map((edge) => hydratePersistedEdge(edge)),
  }
}

function buildGeneratedNodeId(node: Node) {
  const prefix = String(node.data?.type || 'node').replace(/[^a-zA-Z0-9:_-]+/g, '-')
  return `${prefix}-${crypto.randomUUID()}`
}

type SelectionDuplicationIssue = {
  title: string
  message: string
}

type FlowPoint = {
  x: number
  y: number
}

type InsertedSelection = {
  nodes: Node[]
  edges: Edge[]
  selectedNodeIds: string[]
}

type CopiedSelectionNodeSnapshot = {
  node: Node
  absolutePosition: FlowPoint
}

type CopiedSelectionSnapshot = {
  nodes: CopiedSelectionNodeSnapshot[]
  edges: Edge[]
  center: FlowPoint
}

function cloneSerializableValue<T>(value: T): T {
  if (typeof globalThis.structuredClone === 'function') {
    return globalThis.structuredClone(value)
  }

  return JSON.parse(JSON.stringify(value)) as T
}

function getSelectionDuplicationIssue(nodes: Node[]): SelectionDuplicationIssue | null {
  if (nodes.length === 0) {
    return null
  }

  if (nodes.some((node) => node.data?.type === 'logic:return')) {
    return {
      title: 'Return node already exists',
      message: 'Each pipeline can only contain one Return node.',
    }
  }

  if (nodes.length === 1 && isGroupNode(nodes[0])) {
    return {
      title: 'Duplicate is not supported for groups',
      message: 'Create a new group from a node selection instead.',
    }
  }

  return null
}

function buildDuplicatedSelection(
  nodes: Node[],
  edges: Edge[],
  nodeIds: string[],
): InsertedSelection | null {
  const selectedIdSet = new Set(nodeIds)
  const selectedNodes = normalizeNodesForSubflows(nodes.filter((node) => selectedIdSet.has(node.id)))

  if (selectedNodes.length === 0) {
    return null
  }

  const nodesById = new Map(nodes.map((node) => [node.id, node]))
  const nodeIdMap = new Map<string, string>()

  selectedNodes.forEach((node) => {
    nodeIdMap.set(node.id, buildGeneratedNodeId(node))
  })

  const duplicatedNodes = selectedNodes.map((node) => {
    const duplicatedNode = cloneSerializableValue(node)
    const parentSelected = Boolean(node.parentId && selectedIdSet.has(node.parentId))
    const parentExists = Boolean(node.parentId && nodesById.has(node.parentId))
    const duplicateParentId = parentSelected && node.parentId
      ? nodeIdMap.get(node.parentId)
      : parentExists
      ? node.parentId
      : undefined

    let nextPosition = { ...node.position }

    if (parentSelected) {
      nextPosition = { ...node.position }
    } else if (parentExists) {
      nextPosition = {
        x: node.position.x + DUPLICATE_SELECTION_OFFSET.x,
        y: node.position.y + DUPLICATE_SELECTION_OFFSET.y,
      }
    } else {
      const absolutePosition = getAbsoluteNodePosition(node, nodesById)
      nextPosition = {
        x: absolutePosition.x + DUPLICATE_SELECTION_OFFSET.x,
        y: absolutePosition.y + DUPLICATE_SELECTION_OFFSET.y,
      }
    }

    const nextNode: Node = {
      ...duplicatedNode,
      id: nodeIdMap.get(node.id)!,
      parentId: duplicateParentId,
      position: nextPosition,
      selected: true,
      dragging: false,
    }

    if (!duplicateParentId) {
      nextNode.extent = undefined
    }

    return nextNode
  })

  const duplicatedEdges = edges.flatMap((edge) => {
    const source = nodeIdMap.get(edge.source)
    const target = nodeIdMap.get(edge.target)

    if (!source || !target) {
      return []
    }

    const nextEdge: Edge = {
      ...cloneSerializableValue(edge),
      id: `edge-${crypto.randomUUID()}`,
      source,
      target,
      selected: false,
    }

    return [nextEdge]
  })

  return {
    nodes: duplicatedNodes,
    edges: duplicatedEdges,
    selectedNodeIds: duplicatedNodes.map((node) => node.id),
  }
}

function buildCopiedSelectionSnapshot(
  nodes: Node[],
  edges: Edge[],
  nodeIds: string[],
): CopiedSelectionSnapshot | null {
  const selectedIdSet = new Set(nodeIds)
  const selectedNodes = normalizeNodesForSubflows(nodes.filter((node) => selectedIdSet.has(node.id)))

  if (selectedNodes.length === 0) {
    return null
  }

  const nodesById = new Map(nodes.map((node) => [node.id, node]))
  const copiedNodes = selectedNodes.map((node) => ({
    node: cloneSerializableValue(node),
    absolutePosition: getAbsoluteNodePosition(node, nodesById),
  }))

  const minX = Math.min(...copiedNodes.map(({ absolutePosition }) => absolutePosition.x))
  const minY = Math.min(...copiedNodes.map(({ absolutePosition }) => absolutePosition.y))
  const maxX = Math.max(...copiedNodes.map(({ node, absolutePosition }) => absolutePosition.x + getNodeWidth(node)))
  const maxY = Math.max(...copiedNodes.map(({ node, absolutePosition }) => absolutePosition.y + getNodeHeight(node)))

  return {
    nodes: copiedNodes,
    edges: edges
      .filter((edge) => selectedIdSet.has(edge.source) && selectedIdSet.has(edge.target))
      .map((edge) => cloneSerializableValue(edge)),
    center: {
      x: (minX + maxX) / 2,
      y: (minY + maxY) / 2,
    },
  }
}

function getPasteSelectionIssue(
  nodes: Node[],
  copiedSelection: CopiedSelectionSnapshot | null,
): SelectionDuplicationIssue | null {
  if (!copiedSelection || copiedSelection.nodes.length === 0) {
    return {
      title: 'Nothing copied yet',
      message: 'Select one or more nodes on the canvas and press Ctrl+C first.',
    }
  }

  const includesReturnNode = copiedSelection.nodes.some(({ node }) => node.data?.type === 'logic:return')
  if (includesReturnNode && hasReturnNode(nodes)) {
    return {
      title: 'Return node already exists',
      message: 'Each pipeline can only contain one Return node.',
    }
  }

  return null
}

function buildPastedSelection(
  copiedSelection: CopiedSelectionSnapshot,
  pastePosition: FlowPoint,
): InsertedSelection | null {
  const copiedNodes = copiedSelection.nodes.map(({ node }) => node)

  if (copiedNodes.length === 0) {
    return null
  }

  const selectedIdSet = new Set(copiedNodes.map((node) => node.id))
  const nodeIdMap = new Map<string, string>()
  const delta = {
    x: pastePosition.x - copiedSelection.center.x,
    y: pastePosition.y - copiedSelection.center.y,
  }

  copiedNodes.forEach((node) => {
    nodeIdMap.set(node.id, buildGeneratedNodeId(node))
  })

  const pastedNodes = copiedSelection.nodes.map(({ node, absolutePosition }) => {
    const pastedNode = cloneSerializableValue(node)
    const parentSelected = Boolean(node.parentId && selectedIdSet.has(node.parentId))
    const nextParentId = parentSelected && node.parentId
      ? nodeIdMap.get(node.parentId)
      : undefined

    const nextNode: Node = {
      ...pastedNode,
      id: nodeIdMap.get(node.id)!,
      parentId: nextParentId,
      position: nextParentId
        ? { ...node.position }
        : {
          x: absolutePosition.x + delta.x,
          y: absolutePosition.y + delta.y,
        },
      selected: true,
      dragging: false,
    }

    if (!nextParentId) {
      nextNode.extent = undefined
    }

    return nextNode
  })

  const pastedEdges = copiedSelection.edges.flatMap((edge) => {
    const source = nodeIdMap.get(edge.source)
    const target = nodeIdMap.get(edge.target)

    if (!source || !target) {
      return []
    }

    return [{
      ...cloneSerializableValue(edge),
      id: `edge-${crypto.randomUUID()}`,
      source,
      target,
      selected: false,
    }]
  })

  return {
    nodes: pastedNodes,
    edges: pastedEdges,
    selectedNodeIds: pastedNodes.map((node) => node.id),
  }
}

function getVisualExecutionStatus(status?: string): 'pending' | 'running' | 'success' | 'error' | undefined {
  if (!status) return undefined

  switch (status) {
    case 'completed':
      return 'success'
    case 'failed':
      return 'error'
    case 'running':
      return 'running'
    case 'pending':
      return 'pending'
    default:
      return undefined
  }
}

const EXECUTION_LOG_STRING_LIMIT = 1200
const EXECUTION_LOG_ARRAY_LIMIT = 6
const EXECUTION_LOG_OBJECT_LIMIT = 12
const EXECUTION_LOG_DEPTH_LIMIT = 4

function parseExecutionLogValue(value?: string): unknown {
  if (!value) {
    return undefined
  }

  try {
    return JSON.parse(value)
  } catch {
    return value
  }
}

function compactExecutionLogValue(value: unknown, depth = 0): unknown {
  if (typeof value === 'string') {
    return value.length <= EXECUTION_LOG_STRING_LIMIT
      ? value
      : `${value.slice(0, EXECUTION_LOG_STRING_LIMIT)}... [truncated ${value.length - EXECUTION_LOG_STRING_LIMIT} chars]`
  }

  if (typeof value === 'number' || typeof value === 'boolean' || value === null || value === undefined) {
    return value
  }

  if (depth >= EXECUTION_LOG_DEPTH_LIMIT) {
    if (Array.isArray(value)) {
      return `[array truncated, ${value.length} item(s)]`
    }

    if (typeof value === 'object') {
      return '[object truncated]'
    }
  }

  if (Array.isArray(value)) {
    const next = value
      .slice(0, EXECUTION_LOG_ARRAY_LIMIT)
      .map((entry) => compactExecutionLogValue(entry, depth + 1))

    if (value.length > EXECUTION_LOG_ARRAY_LIMIT) {
      next.push(`... ${value.length - EXECUTION_LOG_ARRAY_LIMIT} more item(s) omitted`)
    }

    return next
  }

  if (!value || typeof value !== 'object') {
    return String(value)
  }

  const entries = Object.entries(value)
  const next = entries
    .slice(0, EXECUTION_LOG_OBJECT_LIMIT)
    .reduce<Record<string, unknown>>((acc, [key, child]) => {
      acc[key] = compactExecutionLogValue(child, depth + 1)
      return acc
    }, {})

  if (entries.length > EXECUTION_LOG_OBJECT_LIMIT) {
    next._truncated = `${entries.length - EXECUTION_LOG_OBJECT_LIMIT} more field(s) omitted`
  }

  return next
}

function buildAssistantLogAttachment(detail: ExecutionDetail): EditorAssistantExecutionLogAttachment {
  const nodes = detail.node_executions.map((nodeExecution) => ({
    node_id: nodeExecution.node_id,
    node_type: nodeExecution.node_type,
    status: nodeExecution.status,
    input: compactExecutionLogValue(parseExecutionLogValue(nodeExecution.input)),
    output: compactExecutionLogValue(parseExecutionLogValue(nodeExecution.output)),
    error: nodeExecution.error || undefined,
  }))

  const payload = {
    execution: {
      id: detail.execution.id,
      trigger_type: detail.execution.trigger_type,
      status: detail.execution.status,
      started_at: detail.execution.started_at,
      completed_at: detail.execution.completed_at,
      error: detail.execution.error || undefined,
    },
    nodes,
  }

  return {
    id: `${detail.execution.id}-${Date.now()}`,
    ...payload,
  }
}

function isToolNodeType(type: unknown): type is NodeType {
  return typeof type === 'string' && type.startsWith('tool:')
}

function hasReturnNode(nodes: Node[]): boolean {
  return nodes.some((node) => node.data?.type === 'logic:return')
}

function getRenderedNodeBorderColor(node: Node | undefined, selectedNodeIds: ReadonlySet<string>): string {
  if (!node || typeof node.data?.type !== 'string') {
    return DEFAULT_NODE_BORDER_COLOR
  }

  const nodeType = node.data.type

  const config = typeof node.data?.config === 'object' && node.data?.config !== null
    ? node.data.config as Record<string, unknown>
    : undefined

  return getNodeBorderTint({
    nodeType: nodeType as NodeType,
    selected: selectedNodeIds.has(node.id),
    isHighlight: node.data?.isHighlight === true,
    status: node.data?.status as 'pending' | 'running' | 'success' | 'error' | undefined,
    config,
  })
}

function PipelineEditor() {
  const { id } = useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const reactFlowWrapper = useRef<HTMLDivElement>(null)
  const addJSONTemplateInputRef = useRef<HTMLInputElement>(null)
  const copiedSelectionRef = useRef<CopiedSelectionSnapshot | null>(null)
  const lastCanvasPointerClientPositionRef = useRef<FlowPoint | null>(null)
  const isCanvasPointerActiveRef = useRef(false)
  const {
    screenToFlowPosition,
    getViewport,
    setViewport: setCanvasViewport,
    zoomIn,
    zoomOut,
    fitView: fitCanvasView,
  } = useReactFlow()
  const { selectedNodeId, setSelectedNodeId, addToast } = useUIStore()

  const [nodes, setNodes] = useNodesState([])
  const [edges, setEdges, onEdgesChange] = useEdgesState([])
  const [isSaving, setIsSaving] = useState(false)
  const [isRunning, setIsRunning] = useState(false)
  const [pipelineName, setPipelineName] = useState('')
  const [pipelineDescription, setPipelineDescription] = useState('')
  const [pipelineStatus, setPipelineStatus] = useState<Pipeline['status']>('draft')
  const [editingDetails, setEditingDetails] = useState(false)
  const [showExecutionLog, setShowExecutionLog] = useState(false)
  const [isFlowInteractive, setIsFlowInteractive] = useState(true)
  const [isNodePaletteOpen, setIsNodePaletteOpen] = useState(false)
  const [highlightedNodes, setHighlightedNodes] = useState<Set<string>>(new Set())
  const [nodeStatuses, setNodeStatuses] = useState<Record<string, string>>({})
  const [nodeLogs, setNodeLogs] = useState<Record<string, NodeExecutionLogData>>({})
  const [activeNodeLogId, setActiveNodeLogId] = useState<string | null>(null)
  const [contextMenu, setContextMenu] = useState<EditorContextMenuState | null>(null)
  const [templateMenuPosition, setTemplateMenuPosition] = useState<{ x: number; y: number } | null>(null)
  const [isBlockingOverlayOpen, setIsBlockingOverlayOpen] = useState(false)
  const [assistantEditLockActive, setAssistantEditLockActive] = useState(false)
  const [assistantLogAttachment, setAssistantLogAttachment] = useState<EditorAssistantExecutionLogAttachment | null>(null)
  const [showSaveTemplateModal, setShowSaveTemplateModal] = useState(false)
  const [showTemplateLibraryModal, setShowTemplateLibraryModal] = useState(false)
  const [templateDraftName, setTemplateDraftName] = useState('')
  const [templateDraftDescription, setTemplateDraftDescription] = useState('')
  const [activeTemplateId, setActiveTemplateId] = useState<string | null>(null)
  const nameInputRef = useRef<HTMLInputElement>(null)
  const selectedNodes = nodes.filter((node) => node.selected)
  const selectedNodeIds = selectedNodes.map((node) => node.id)
  const activeSelectionIds = selectedNodeIds.length > 0
    ? selectedNodeIds
    : selectedNodeId
    ? [selectedNodeId]
    : []
  const activeSelectionIdSet = new Set(activeSelectionIds)
  const activeSelectionNodes = nodes.filter((node) => activeSelectionIdSet.has(node.id))
  const activeSelectionDuplicationIssue = getSelectionDuplicationIssue(activeSelectionNodes)
  const isCanvasInteractionBlocked = isBlockingOverlayOpen || showSaveTemplateModal || showTemplateLibraryModal || templateMenuPosition !== null || assistantEditLockActive
  const isCanvasInteractionDisabled = isCanvasInteractionBlocked || !isFlowInteractive

  const { data: pipeline, isLoading } = useQuery<Pipeline>({
    queryKey: ['pipeline', id],
    queryFn: () => api.pipelines.get(id!),
    enabled: !!id,
  })

  const { data: templates = [], isLoading: areTemplatesLoading } = useQuery<TemplateSummary[]>({
    queryKey: ['templates'],
    queryFn: () => api.templates.list(),
  })

  const { data: llmProviders = [] } = useQuery<LLMProvider[]>({
    queryKey: ['llm-providers'],
    queryFn: () => api.llmProviders.list(),
  })

  const buildCurrentDefinition = useCallback((): FlowDefinitionDocument => (
    buildFlowDefinitionDocument(nodes, edges, getViewport())
  ), [edges, getViewport, nodes])

  const updateCanvasPointerPosition = useCallback((clientPosition: FlowPoint) => {
    isCanvasPointerActiveRef.current = true
    lastCanvasPointerClientPositionRef.current = clientPosition
  }, [])

  const handleCanvasMouseEnter = useCallback((event: ReactMouseEvent<HTMLDivElement>) => {
    updateCanvasPointerPosition({ x: event.clientX, y: event.clientY })
  }, [updateCanvasPointerPosition])

  const handleCanvasMouseMove = useCallback((event: ReactMouseEvent<HTMLDivElement>) => {
    updateCanvasPointerPosition({ x: event.clientX, y: event.clientY })
  }, [updateCanvasPointerPosition])

  const handleCanvasMouseLeave = useCallback(() => {
    isCanvasPointerActiveRef.current = false
  }, [])

  const getActiveCanvasPastePosition = useCallback((): FlowPoint | null => {
    if (!isCanvasPointerActiveRef.current) {
      return null
    }

    const clientPosition = lastCanvasPointerClientPositionRef.current
    if (clientPosition) {
      return screenToFlowPosition(clientPosition)
    }

    const wrapperBounds = reactFlowWrapper.current?.getBoundingClientRect()
    if (!wrapperBounds) {
      return null
    }

    return screenToFlowPosition({
      x: wrapperBounds.left + (wrapperBounds.width / 2),
      y: wrapperBounds.top + (wrapperBounds.height / 2),
    })
  }, [screenToFlowPosition])

  useEffect(() => {
    if (pipeline) {
      try {
        setPipelineName(pipeline.name)
        setPipelineDescription(pipeline.description || '')
        setPipelineStatus((pipeline.status as Pipeline['status']) || 'draft')
        const parsedNodes = JSON.parse(pipeline.nodes || '[]')
        const parsedEdges = JSON.parse(pipeline.edges || '[]')
        const flow = buildImportedFlow({
          nodes: parsedNodes,
          edges: parsedEdges,
        })
        setNodes(normalizeNodesForSubflows(flow.nodes))
        setEdges(flow.edges)
      } catch (err) {
        console.error('Failed to parse pipeline data:', err)
      }
    }
  }, [pipeline])

  useEffect(() => {
    if (editingDetails && nameInputRef.current) {
      nameInputRef.current.focus()
      nameInputRef.current.select()
    }
  }, [editingDetails])

  const saveMutation = useMutation({
    mutationFn: async (nextStatus?: Pipeline['status']) => {
      if (!id) return
      const definition = buildCurrentDefinition()

      return api.pipelines.update(id, {
        name: pipelineName,
        description: pipelineDescription,
        nodes: JSON.stringify(definition.nodes),
        edges: JSON.stringify(definition.edges),
        viewport: JSON.stringify(definition.viewport),
        status: nextStatus || pipelineStatus,
      })
    },
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ['pipeline', id] })
      queryClient.invalidateQueries({ queryKey: ['pipelines'] })
      if (result?.status) {
        setPipelineStatus(result.status as Pipeline['status'])
      }
      setEditingDetails(false)
      addToast({ type: 'success', title: 'Pipeline saved' })
      setIsSaving(false)
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to save', message: err.message })
      setIsSaving(false)
    },
  })

  const appendDefinitionToCanvas = useCallback((definition: FlowDefinitionDocument, sourceName: string) => {
    const importedFlow = buildImportedFlow(definition)

    if (importedFlow.nodes.length === 0) {
      addToast({
        type: 'warning',
        title: 'Nothing to add',
        message: 'The selected template does not contain any nodes.',
      })
      return
    }

    const importedNodesById = new Map(importedFlow.nodes.map((node) => [node.id, node]))
    const importedBounds = importedFlow.nodes.map((node) => getNodeBounds(node, importedNodesById))
    const minX = Math.min(...importedBounds.map((bounds) => bounds.x))
    const minY = Math.min(...importedBounds.map((bounds) => bounds.y))
    const maxX = Math.max(...importedBounds.map((bounds) => bounds.x + bounds.width))
    const maxY = Math.max(...importedBounds.map((bounds) => bounds.y + bounds.height))

    const wrapperBounds = reactFlowWrapper.current?.getBoundingClientRect()
    const viewportCenter = wrapperBounds
      ? screenToFlowPosition({
        x: wrapperBounds.left + (wrapperBounds.width / 2),
        y: wrapperBounds.top + (wrapperBounds.height / 2),
      })
      : { x: minX, y: minY }

    const delta = {
      x: viewportCenter.x - ((minX + maxX) / 2) + 40,
      y: viewportCenter.y - ((minY + maxY) / 2) + 32,
    }

    const nodeIdMap = new Map<string, string>()
    importedFlow.nodes.forEach((node) => {
      nodeIdMap.set(node.id, buildGeneratedNodeId(node))
    })

    const firstInsertedNodeId = nodeIdMap.get(importedFlow.nodes[0].id) ?? null

    const insertedNodes = importedFlow.nodes.map((node) => {
      const absolutePosition = getAbsoluteNodePosition(node, importedNodesById)
      const hasParent = Boolean(node.parentId && importedNodesById.has(node.parentId))
      const parentId = hasParent && node.parentId ? nodeIdMap.get(node.parentId) : undefined
      const nextNode: Node = {
        ...node,
        id: nodeIdMap.get(node.id)!,
        parentId,
        position: hasParent
          ? { x: node.position.x, y: node.position.y }
          : {
            x: absolutePosition.x + delta.x,
            y: absolutePosition.y + delta.y,
          },
        selected: nodeIdMap.get(node.id) === firstInsertedNodeId,
        dragging: false,
        data: {
          ...node.data,
        },
      }

      if (!parentId) {
        nextNode.extent = undefined
      }

      return nextNode
    })

    let droppedEdgeCount = 0
    const insertedEdges = importedFlow.edges.flatMap((edge) => {
      const source = nodeIdMap.get(edge.source)
      const target = nodeIdMap.get(edge.target)

      if (!source || !target) {
        droppedEdgeCount += 1
        return []
      }

      return [{
        ...edge,
        id: `edge-${crypto.randomUUID()}`,
        source,
        target,
        selected: false,
      }]
    })

    setNodes((currentNodes) => normalizeNodesForSubflows([
      ...currentNodes.map((node) => ({ ...node, selected: false })),
      ...insertedNodes,
    ]))
    setEdges((currentEdges) => currentEdges.concat(insertedEdges))
    setSelectedNodeId(firstInsertedNodeId)
    setContextMenu(null)

    addToast({
      type: 'success',
      title: 'Added to current pipeline',
      message: `${sourceName || 'Template'} was appended to the canvas.`,
    })

    if (hasReturnNode(nodes) && hasReturnNode(importedFlow.nodes)) {
      addToast({
        type: 'warning',
        title: 'Pipeline needs cleanup before save',
        message: 'The inserted definition adds another Return node. Keep only one Return node before saving.',
        duration: 7000,
      })
    }

    if (droppedEdgeCount > 0) {
      addToast({
        type: 'warning',
        title: 'Some connections were skipped',
        message: `${droppedEdgeCount} imported connection${droppedEdgeCount === 1 ? '' : 's'} could not be mapped to inserted nodes.`,
        duration: 6500,
      })
    }
  }, [addToast, nodes, screenToFlowPosition, setEdges, setNodes, setSelectedNodeId])

  const handleApplyAssistantOperations = useCallback(async (operations: LivePipelineOperation[]) => {
    if (operations.length === 0) {
      return
    }

    const nextState = applyLivePipelineOperations({
      nodes,
      edges,
      viewport: getViewport(),
      operations,
    })

    setNodes(normalizeNodesForSubflows(nextState.nodes))
    setEdges(nextState.edges)
    void setCanvasViewport(nextState.viewport)

    if (selectedNodeId && !nextState.nodes.some((node) => node.id === selectedNodeId)) {
      setSelectedNodeId(null)
    }
  }, [edges, getViewport, nodes, selectedNodeId, setCanvasViewport, setEdges, setNodes, setSelectedNodeId])

  const saveTemplateMutation = useMutation({
    mutationFn: (payload: { name: string; description?: string }) => api.templates.create({
      name: payload.name,
      description: payload.description || undefined,
      definition: buildCurrentDefinition(),
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['templates'] })
      setShowSaveTemplateModal(false)
      setTemplateDraftName('')
      setTemplateDraftDescription('')
      addToast({ type: 'success', title: 'Template saved from current pipeline' })
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to save template', message: err.message })
    },
  })

  const appendTemplateMutation = useMutation({
    mutationFn: (templateId: string) => api.templates.get(templateId),
    onSuccess: (template) => {
      appendDefinitionToCanvas(template.definition, template.name)
      setShowTemplateLibraryModal(false)
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to load template', message: err.message })
    },
    onSettled: () => {
      setActiveTemplateId(null)
    },
  })

  const runMutation = useMutation({
    mutationFn: () => api.pipelines.run(id!),
    onSuccess: (result: PipelineRunResponse) => {
      if (result.status === 'completed') {
        addToast({ type: 'success', title: 'Pipeline completed' })
      } else if (result.status === 'cancelled') {
        addToast({
          type: 'warning',
          title: 'Pipeline stopped',
          message: result.error || 'The execution was cancelled.',
        })
      } else {
        addToast({
          type: 'error',
          title: 'Pipeline failed',
          message: result.error || 'The execution finished with an error.',
        })
      }
      setIsRunning(false)
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Pipeline failed', message: err.message })
      setIsRunning(false)
    },
  })

  const handleSave = useCallback(() => {
    setIsSaving(true)
    saveMutation.mutate(undefined)
  }, [saveMutation])

  const handleToggleStatus = useCallback((nextStatus: Pipeline['status']) => {
    setIsSaving(true)
    saveMutation.mutate(nextStatus)
  }, [saveMutation])

  const handleRun = useCallback(() => {
    setShowExecutionLog(true)
    setIsRunning(true)
    runMutation.mutate()
  }, [runMutation])

  const handleCancelDetailsEdit = useCallback(() => {
    setPipelineName(pipeline?.name || '')
    setPipelineDescription(pipeline?.description || '')
    setEditingDetails(false)
  }, [pipeline])

  const handleOpenSaveTemplateModal = useCallback(() => {
    setTemplateMenuPosition(null)
    setTemplateDraftName(
      pipelineName.trim() ? `${pipelineName.trim()} Template` : 'Untitled Template',
    )
    setTemplateDraftDescription(pipelineDescription || '')
    setShowSaveTemplateModal(true)
  }, [pipelineDescription, pipelineName])

  const handleSubmitSaveTemplate = useCallback(() => {
    const name = templateDraftName.trim()

    if (!name) {
      addToast({
        type: 'warning',
        title: 'Template name is required',
        message: 'Give the template a name before saving it.',
      })
      return
    }

    saveTemplateMutation.mutate({
      name,
      description: templateDraftDescription.trim() || undefined,
    })
  }, [addToast, saveTemplateMutation, templateDraftDescription, templateDraftName])

  const handleExportJSON = useCallback(() => {
    const document = buildPipelineDocument({
      name: pipelineName.trim() || 'Untitled Pipeline',
      description: pipelineDescription,
      status: pipelineStatus,
      definition: buildCurrentDefinition(),
    })

    downloadJSON(
      sanitizeFilename(pipelineName.trim() || 'pipeline', 'pipeline'),
      document,
    )

    addToast({ type: 'success', title: 'Pipeline exported as JSON' })
  }, [addToast, buildCurrentDefinition, pipelineDescription, pipelineName, pipelineStatus])

  const handleAddJSONTemplate = useCallback(async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''

    if (!file) {
      return
    }

    try {
      const raw = await file.text()
      const parsed = JSON.parse(raw)
      const document = extractSingleDefinitionDocument(parsed)
      appendDefinitionToCanvas(document.definition, document.name || file.name)
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Failed to add JSON template',
        message: err instanceof Error ? err.message : 'Unknown error',
      })
    }
  }, [addToast, appendDefinitionToCanvas])

  const handleExecutionHighlight = useCallback((data: {
    nodeIds: string[]
    nodeStatuses: Record<string, string>
    nodeErrors: Record<string, string | undefined>
    nodeLogs: Record<string, NodeExecutionLogData>
  } | null) => {
    if (!data) {
      setHighlightedNodes(new Set())
      setNodeStatuses({})
      setNodeLogs({})
      setActiveNodeLogId(null)
      return
    }
    setHighlightedNodes(new Set(data.nodeIds))
    setNodeStatuses(data.nodeStatuses)
    setNodeLogs(data.nodeLogs)
  }, [])

  const handleCloseExecutionLog = useCallback(() => {
    setShowExecutionLog(false)
    setHighlightedNodes(new Set())
    setNodeStatuses({})
    setNodeLogs({})
    setActiveNodeLogId(null)
  }, [])

  const handleAddExecutionLogToAssistant = useCallback((detail: ExecutionDetail) => {
    setAssistantLogAttachment(buildAssistantLogAttachment(detail))
    addToast({
      type: 'success',
      title: 'Execution log attached',
      message: 'The selected run is now available below the live assistant composer.',
    })
  }, [addToast])

  useEffect(() => {
    if (activeNodeLogId && !nodeLogs[activeNodeLogId]) {
      setActiveNodeLogId(null)
    }
  }, [activeNodeLogId, nodeLogs])

  const onConnect: OnConnect = useCallback(
    (params) => {
      const sourceNode = nodes.find((node) => node.id === params.source)
      const targetNode = nodes.find((node) => node.id === params.target)
      const sourceType = sourceNode?.data?.type
      const targetType = targetNode?.data?.type
      const isToolConnection = params.sourceHandle === 'tool'
      const sourceIsTool = isToolNodeType(sourceType)
      const targetIsTool = isToolNodeType(targetType)
      const sourceIsReturn = sourceType === 'logic:return'
      const targetIsTrigger = typeof targetType === 'string' && targetType.startsWith('trigger:')
      const sourceIsGroup = isVisualGroupNodeType(sourceType)
      const targetIsGroup = isVisualGroupNodeType(targetType)

      if (sourceIsGroup || targetIsGroup) {
        addToast({
          type: 'warning',
          title: 'Groups are visual only',
          message: 'Visual groups cannot have incoming or outgoing connections.',
        })
        return
      }

      if (isToolConnection) {
        if (sourceType !== 'llm:agent' || !targetIsTool) {
          addToast({
            type: 'warning',
            title: 'Invalid tool connection',
            message: 'Only an LLM Agent tool pin can connect to tool nodes.',
          })
          return
        }
      } else if (sourceIsReturn) {
        addToast({
          type: 'warning',
          title: 'Return nodes end the pipeline',
          message: 'Return nodes cannot connect to downstream nodes.',
        })
        return
      } else if (targetIsTrigger) {
        addToast({
          type: 'warning',
          title: 'Triggers cannot accept input',
          message: 'Trigger nodes can only start a pipeline and cannot have incoming connections.',
        })
        return
      } else if (sourceIsTool || targetIsTool) {
        addToast({
          type: 'warning',
          title: 'Tool nodes only connect to agents',
          message: 'Use the blue tool pin on an LLM Agent to connect tool nodes.',
        })
        return
      }

      setEdges((eds) => addEdge({
        ...params,
        ...(isToolConnection ? toolEdgeOptions : defaultEdgeOptions),
      }, eds))
    },
    [addToast, nodes, setEdges],
  )

  const createNodeAtPosition = useCallback((
    type: NodeType,
    label: string,
    config: Record<string, unknown>,
    clientPosition: { x: number; y: number },
  ) => {
    if (type === 'logic:return' && hasReturnNode(nodes)) {
      addToast({
        type: 'warning',
        title: 'Return node already exists',
        message: 'Each pipeline can only contain one Return node.',
      })
      return
    }

    const position = reactFlowWrapper.current
      ? screenToFlowPosition(clientPosition)
      : clientPosition

    const newNode: Node = {
      id: `${type}-${Date.now()}`,
      type: 'automator',
      position,
      data: {
        label,
        type,
        config,
        enabled: true,
      },
    }

    setNodes((nds) => nds.concat(newNode))
    setSelectedNodeId(newNode.id)
  }, [addToast, nodes, screenToFlowPosition, setNodes, setSelectedNodeId])

  const onDragOver = useCallback((event: React.DragEvent) => {
    if (!isFlowInteractive) {
      return
    }
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
  }, [isFlowInteractive])

  const onDrop = useCallback(
    (event: React.DragEvent) => {
      if (!isFlowInteractive) {
        return
      }
      event.preventDefault()

      const type = event.dataTransfer.getData('application/reactflow/type')
      const label = event.dataTransfer.getData('application/reactflow/label')
      const config = event.dataTransfer.getData('application/reactflow/config')

      if (!type || !reactFlowWrapper.current) return

      createNodeAtPosition(
        type as NodeType,
        label || 'Node',
        JSON.parse(config || '{}'),
        {
          x: event.clientX,
          y: event.clientY,
        },
      )
    },
    [createNodeAtPosition, isFlowInteractive],
  )

  const onNodesChangeHandler: OnNodesChange = useCallback(
    (changes) => {
      setNodes((currentNodes) => {
        const nextNodes = applyNodeChanges(changes, currentNodes).map((node) => {
          if (!isGroupNode(node)) {
            return node
          }

          const dimensionChange = changes.find((change) => change.type === 'dimensions' && change.id === node.id)
          if (!dimensionChange || !dimensionChange.dimensions) {
            return node
          }

          const minDimensions = getMinimumGroupDimensions(node, currentNodes)
          const width = Math.max(dimensionChange.dimensions.width, minDimensions.width)
          const height = Math.max(dimensionChange.dimensions.height, minDimensions.height)

          return {
            ...node,
            width,
            height,
            style: {
              ...node.style,
              width,
              height,
            },
          }
        })
        const nextSelectedNodes = nextNodes.filter((node) => node.selected)

        if (nextSelectedNodes.length === 1) {
          setSelectedNodeId(nextSelectedNodes[0].id)
        } else {
          setSelectedNodeId(null)
        }

        return nextNodes
      })
      setContextMenu(null)
    },
    [setNodes, setSelectedNodeId],
  )

  const handleNodeDragStop = useCallback((_: React.MouseEvent, _draggedNode: Node, draggedNodes: Node[]) => {
    setNodes((currentNodes) => {
      const draggedNodesById = new Map(draggedNodes.map((node) => [node.id, node]))
      let nextNodes = currentNodes.map((node) => {
        const draggedSnapshot = draggedNodesById.get(node.id)
        if (!draggedSnapshot) {
          return node
        }

        return {
          ...node,
          position: { ...draggedSnapshot.position },
        }
      })

      draggedNodes.forEach((draggedSnapshot) => {
        const candidateNode = nextNodes.find((node) => node.id === draggedSnapshot.id)
        if (!candidateNode || isGroupNode(candidateNode)) {
          return
        }

        const nodesById = new Map(nextNodes.map((node) => [node.id, node]))
        const absolutePosition = getAbsoluteNodePosition(candidateNode, nodesById)
        const containingGroup = findContainingGroup(candidateNode, nextNodes)

        if (containingGroup) {
          if (candidateNode.parentId === containingGroup.id) {
            return
          }

          const groupAbsolutePosition = getAbsoluteNodePosition(containingGroup, nodesById)

          nextNodes = nextNodes.map((node) => {
            if (node.id !== candidateNode.id) {
              return node
            }

            return {
              ...node,
              position: {
                x: absolutePosition.x - groupAbsolutePosition.x,
                y: absolutePosition.y - groupAbsolutePosition.y,
              },
              parentId: containingGroup.id,
              extent: undefined,
            }
          })

          return
        }

        if (!candidateNode.parentId) {
          return
        }

        nextNodes = nextNodes.map((node) => {
          if (node.id !== candidateNode.id) {
            return node
          }

          return {
            ...node,
            position: absolutePosition,
            parentId: undefined,
            extent: undefined,
          }
        })
      })

      return normalizeNodesForSubflows(nextNodes)
    })
  }, [setNodes])

  const onDragStart = useCallback((event: React.DragEvent, nodeType: string, label: string, config: Record<string, unknown>) => {
    event.dataTransfer.setData('application/reactflow/type', nodeType)
    event.dataTransfer.setData('application/reactflow/label', label)
    event.dataTransfer.setData('application/reactflow/config', JSON.stringify(config))
    event.dataTransfer.effectAllowed = 'move'
  }, [])

  const selectedNode = nodes.find((n) => n.id === selectedNodeId)
  const activeNodeLog = activeNodeLogId ? nodeLogs[activeNodeLogId] : null
  const activeNodeLogNode = activeNodeLogId ? nodes.find((node) => node.id === activeNodeLogId) : undefined
  const templateMenuItems: ContextMenuItem[] = [
    {
      label: 'Save as Template',
      icon: <Copy className="w-3.5 h-3.5" />,
      onClick: handleOpenSaveTemplateModal,
    },
    {
      label: 'Add Template',
      icon: <Workflow className="w-3.5 h-3.5" />,
      onClick: () => setShowTemplateLibraryModal(true),
    },
    {
      label: 'Add JSON Template',
      icon: <FileJson className="w-3.5 h-3.5" />,
      onClick: () => addJSONTemplateInputRef.current?.click(),
    },
    {
      label: 'Export JSON',
      icon: <Download className="w-3.5 h-3.5" />,
      onClick: handleExportJSON,
    },
  ]
  const handleTemplateMenuToggle = useCallback((event: ReactMouseEvent<HTMLButtonElement>) => {
    if (templateMenuPosition) {
      setTemplateMenuPosition(null)
      return
    }

    const buttonRect = event.currentTarget.getBoundingClientRect()
    setContextMenu(null)
    setTemplateMenuPosition({
      x: buttonRect.left,
      y: buttonRect.bottom + 8,
    })
  }, [templateMenuPosition])
  const handleZoomIn = useCallback(() => {
    void zoomIn({ duration: 180 })
  }, [zoomIn])
  const handleZoomOut = useCallback(() => {
    void zoomOut({ duration: 180 })
  }, [zoomOut])
  const handleFitCanvas = useCallback(() => {
    void fitCanvasView({ padding: 0.2, duration: 220 })
  }, [fitCanvasView])
  const canvasControlButtonClass = 'h-8 min-w-8 rounded-xl px-1.5 text-text-muted hover:text-text'

  const updateNodeConfig = useCallback((config: Record<string, unknown>) => {
    if (!selectedNodeId) return
    setNodes((nds) =>
      nds.map((node) => {
        if (node.id === selectedNodeId) {
          return {
            ...node,
            data: {
              ...node.data,
              config,
            },
          }
        }
        return node
      })
    )
  }, [selectedNodeId, setNodes])

  const updateNodeLabel = useCallback((label: string) => {
    if (!selectedNodeId) return
    setNodes((nds) =>
      nds.map((node) => {
        if (node.id === selectedNodeId) {
          return {
            ...node,
            data: {
              ...node.data,
              label,
            },
          }
        }
        return node
      })
    )
  }, [selectedNodeId, setNodes])

  const removeSelectedNodeSourceHandles = useCallback((handleIds: string[]) => {
    if (!selectedNodeId || handleIds.length === 0) return

    const removedHandles = new Set(handleIds)
    setEdges((eds) => eds.filter((edge) => (
      edge.source !== selectedNodeId
        || !edge.sourceHandle
        || !removedHandles.has(edge.sourceHandle)
    )))
  }, [selectedNodeId, setEdges])

  const removeNodesByIds = useCallback((nodeIds: string[]) => {
    if (nodeIds.length === 0) return

    const idsToRemove = new Set(nodeIds)

    setNodes((currentNodes) => {
      const removedGroups = new Map(
        currentNodes
          .filter((node) => idsToRemove.has(node.id) && isGroupNode(node))
          .map((node) => [node.id, { x: node.position.x, y: node.position.y }]),
      )

      return normalizeNodesForSubflows(currentNodes.flatMap((node) => {
        if (idsToRemove.has(node.id)) {
          return []
        }

        if (node.parentId && removedGroups.has(node.parentId)) {
          const parentPosition = removedGroups.get(node.parentId)!

          return [{
            ...node,
            position: {
              x: parentPosition.x + node.position.x,
              y: parentPosition.y + node.position.y,
            },
            parentId: undefined,
            extent: undefined,
            selected: false,
          }]
        }

        return [node]
      }))
    })
    setEdges((currentEdges) => currentEdges.filter((edge) => !idsToRemove.has(edge.source) && !idsToRemove.has(edge.target)))
    setSelectedNodeId(null)
    setContextMenu(null)
  }, [setEdges, setNodes, setSelectedNodeId])

  const ungroupNode = useCallback((groupId: string) => {
    setNodes((currentNodes) => {
      const groupNode = currentNodes.find((node) => node.id === groupId)
      if (!groupNode || !isGroupNode(groupNode)) {
        return currentNodes
      }

      return normalizeNodesForSubflows(currentNodes.flatMap((node) => {
        if (node.id === groupId) {
          return []
        }

        if (node.parentId === groupId) {
          return [{
            ...node,
            position: {
              x: groupNode.position.x + node.position.x,
              y: groupNode.position.y + node.position.y,
            },
            parentId: undefined,
            extent: undefined,
            selected: false,
          }]
        }

        return [node]
      }))
    })
    setSelectedNodeId(null)
    setContextMenu(null)
  }, [setNodes, setSelectedNodeId])

  const createGroupFromSelection = useCallback(() => {
    if (!canGroupNodes(selectedNodes)) {
      addToast({
        type: 'warning',
        title: 'Cannot group this selection',
        message: 'Groups can only be created from two or more top-level non-group nodes.',
      })
      return
    }

    const minX = Math.min(...selectedNodes.map((node) => node.position.x))
    const minY = Math.min(...selectedNodes.map((node) => node.position.y))
    const maxX = Math.max(...selectedNodes.map((node) => node.position.x + getNodeWidth(node)))
    const maxY = Math.max(...selectedNodes.map((node) => node.position.y + getNodeHeight(node)))

    const groupPosition = {
      x: minX - GROUP_PADDING_X,
      y: minY - GROUP_PADDING_TOP,
    }
    const groupWidth = Math.max((maxX - minX) + GROUP_PADDING_X * 2, MIN_GROUP_WIDTH)
    const groupHeight = Math.max((maxY - minY) + GROUP_PADDING_TOP + GROUP_PADDING_BOTTOM, MIN_GROUP_HEIGHT)
    const groupId = `visual:group-${Date.now()}`
    const selectedIds = new Set(selectedNodes.map((node) => node.id))
    const groupLabel = getNextGroupLabel(nodes)

    setNodes((currentNodes) => normalizeNodesForSubflows([
      {
        id: groupId,
        type: 'automator',
        position: groupPosition,
        style: {
          width: groupWidth,
          height: groupHeight,
        },
        width: groupWidth,
        height: groupHeight,
        selected: true,
        data: {
          label: groupLabel,
          type: VISUAL_GROUP_TYPE,
          config: { color: DEFAULT_GROUP_COLOR },
          enabled: true,
        },
      },
      ...currentNodes.map((node) => {
        if (!selectedIds.has(node.id)) {
          return { ...node, selected: false }
        }

        return {
          ...node,
          position: {
            x: node.position.x - groupPosition.x,
            y: node.position.y - groupPosition.y,
          },
          parentId: groupId,
          extent: undefined,
          selected: false,
        }
      }),
    ]))

    setSelectedNodeId(groupId)
    setContextMenu(null)
  }, [addToast, nodes, selectedNodes, setNodes, setSelectedNodeId])

  const duplicateNodesByIds = useCallback((nodeIds: string[]) => {
    const selection = nodes.filter((node) => nodeIds.includes(node.id))
    const issue = getSelectionDuplicationIssue(selection)

    if (issue) {
      addToast({
        type: 'warning',
        title: issue.title,
        message: issue.message,
      })
      return
    }

    const duplicatedSelection = buildDuplicatedSelection(nodes, edges, nodeIds)
    if (!duplicatedSelection) {
      return
    }

    setNodes((currentNodes) => normalizeNodesForSubflows([
      ...currentNodes.map((node) => ({ ...node, selected: false })),
      ...duplicatedSelection.nodes,
    ]))
    setEdges((currentEdges) => currentEdges.concat(duplicatedSelection.edges))
    setSelectedNodeId(duplicatedSelection.selectedNodeIds.length === 1 ? duplicatedSelection.selectedNodeIds[0] : null)
    setContextMenu(null)
  }, [addToast, edges, nodes, setEdges, setNodes, setSelectedNodeId])

  const copyNodesByIds = useCallback((nodeIds: string[]) => {
    const copiedSelection = buildCopiedSelectionSnapshot(nodes, edges, nodeIds)
    if (!copiedSelection) {
      return
    }

    copiedSelectionRef.current = copiedSelection
    setContextMenu(null)
  }, [edges, nodes])

  const pasteCopiedSelectionAtPosition = useCallback((pastePosition: FlowPoint | null) => {
    if (!pastePosition) {
      return
    }

    const issue = getPasteSelectionIssue(nodes, copiedSelectionRef.current)
    if (issue) {
      addToast({
        type: 'warning',
        title: issue.title,
        message: issue.message,
      })
      return
    }

    const pastedSelection = buildPastedSelection(copiedSelectionRef.current!, pastePosition)
    if (!pastedSelection) {
      return
    }

    setNodes((currentNodes) => normalizeNodesForSubflows([
      ...currentNodes.map((node) => ({ ...node, selected: false })),
      ...pastedSelection.nodes,
    ]))
    setEdges((currentEdges) => currentEdges.concat(pastedSelection.edges))
    setSelectedNodeId(pastedSelection.selectedNodeIds.length === 1 ? pastedSelection.selectedNodeIds[0] : null)
    setContextMenu(null)
  }, [addToast, nodes, setEdges, setNodes, setSelectedNodeId])

  const pasteCopiedSelectionAtClientPosition = useCallback((clientPosition: FlowPoint) => {
    pasteCopiedSelectionAtPosition(screenToFlowPosition(clientPosition))
  }, [pasteCopiedSelectionAtPosition, screenToFlowPosition])

  const pasteCopiedSelectionAtCursor = useCallback(() => {
    pasteCopiedSelectionAtPosition(getActiveCanvasPastePosition())
  }, [getActiveCanvasPastePosition, pasteCopiedSelectionAtPosition])

  const buildPaneContextMenuItems = useCallback((clientX: number, clientY: number): ContextMenuItem[] => {
    const buildNodeItem = (
      nodeType: typeof NODE_CATEGORIES[number]['types'][number],
      contextLabel: string,
    ): ContextMenuItem => {
      const Icon = nodeMenuIconMap[nodeType.icon] || Zap

      return {
        label: nodeType.label,
        icon: <Icon className="w-3.5 h-3.5" style={{ color: nodeType.color }} />,
        searchText: `${nodeType.label} ${nodeType.description} ${contextLabel}`,
        onClick: () => createNodeAtPosition(
          nodeType.type,
          nodeType.label,
          { ...nodeType.defaultConfig },
          { x: clientX, y: clientY },
        ),
      }
    }

    const buildProviderGroup = (
      label: string,
      Icon: React.ElementType,
      color: string,
      nodeTypes: typeof NODE_CATEGORIES[number]['types'],
      categoryLabel: string,
    ): ContextMenuItem | null => {
      if (nodeTypes.length === 0) {
        return null
      }

      return {
        label,
        icon: <Icon className="w-3.5 h-3.5" style={{ color }} />,
        searchText: `${label} ${categoryLabel} add node`,
        children: nodeTypes.map((nodeType) => buildNodeItem(nodeType, `${categoryLabel} ${label}`)),
      }
    }

    const addNodeItems = NODE_CATEGORIES.map((category) => {
      const CategoryIcon = categoryMenuIconMap[category.id] || Zap

      if (category.id === 'action' || category.id === 'tool') {
        const generalTypes = category.types.filter((nodeType) => (
          !isProxmoxNodeType(nodeType.type) && !isKubernetesNodeType(nodeType.type)
        ))
        const proxmoxTypes = category.types.filter((nodeType) => isProxmoxNodeType(nodeType.type))
        const kubernetesTypes = category.types.filter((nodeType) => isKubernetesNodeType(nodeType.type))
        const providerGroups = [
          buildProviderGroup('General', Workflow, category.color, generalTypes, category.label),
          buildProviderGroup('Proxmox', Server, category.color, proxmoxTypes, category.label),
          buildProviderGroup('Kubernetes', Shield, category.color, kubernetesTypes, category.label),
        ].filter((item): item is ContextMenuItem => item !== null)

        return {
          label: category.label,
          icon: <CategoryIcon className="w-3.5 h-3.5" style={{ color: category.color }} />,
          searchText: `${category.label} add node`,
          children: providerGroups,
        }
      }

      return {
        label: category.label,
        icon: <CategoryIcon className="w-3.5 h-3.5" style={{ color: category.color }} />,
        searchText: `${category.label} add node`,
        children: category.types.map((nodeType) => buildNodeItem(nodeType, category.label)),
      }
    })

    const pasteItem = copiedSelectionRef.current
      ? [{
        label: 'Paste',
        icon: <Copy className="w-3.5 h-3.5" />,
        shortcut: 'Ctrl+V',
        searchText: 'paste copied nodes selection',
        onClick: () => pasteCopiedSelectionAtClientPosition({ x: clientX, y: clientY }),
      }]
      : []

    return [
      ...pasteItem,
      ...(pasteItem.length > 0 ? [{ divider: true, label: '' } satisfies ContextMenuItem] : []),
      ...addNodeItems,
      { divider: true, label: '' },
      {
        label: 'Save pipeline',
        icon: <Save className="w-3.5 h-3.5" />,
        shortcut: 'Ctrl+S',
        onClick: handleSave,
      },
      {
        label: 'Run pipeline',
        icon: <Play className="w-3.5 h-3.5" />,
        onClick: handleRun,
      },
    ]
  }, [createNodeAtPosition, handleRun, handleSave, pasteCopiedSelectionAtClientPosition])

  const duplicateActiveSelection = useCallback(() => {
    if (activeSelectionIds.length === 0) {
      return
    }

    duplicateNodesByIds(activeSelectionIds)
  }, [activeSelectionIds, duplicateNodesByIds])

  const buildSelectionContextMenuItems = useCallback((selection: Node[]): ContextMenuItem[] => {
    const groupable = canGroupNodes(selection)
    const duplicationIssue = getSelectionDuplicationIssue(selection)

    return [
      {
        label: 'Copy selection',
        icon: <Copy className="w-3.5 h-3.5" />,
        shortcut: 'Ctrl+C',
        onClick: () => copyNodesByIds(selection.map((node) => node.id)),
      },
      {
        label: 'Duplicate selection',
        icon: <Copy className="w-3.5 h-3.5" />,
        shortcut: 'Ctrl+D',
        disabled: Boolean(duplicationIssue),
        onClick: () => duplicateNodesByIds(selection.map((node) => node.id)),
      },
      {
        label: 'Make group',
        icon: <Workflow className="w-3.5 h-3.5" />,
        disabled: !groupable,
        onClick: createGroupFromSelection,
      },
      {
        divider: true,
        label: '',
      },
      {
        label: 'Delete selected nodes',
        icon: <Trash2 className="w-3.5 h-3.5" />,
        shortcut: 'Del',
        danger: true,
        onClick: () => removeNodesByIds(selection.map((node) => node.id)),
      },
    ]
  }, [copyNodesByIds, createGroupFromSelection, duplicateNodesByIds, removeNodesByIds])

  const disconnectNode = useCallback(() => {
    if (!selectedNodeId) return
    setEdges((eds) => eds.filter((e) => e.source !== selectedNodeId && e.target !== selectedNodeId))
  }, [selectedNodeId, setEdges])

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (isCanvasInteractionDisabled) {
        return
      }

      const isEditingField = isEditableTarget(e.target)
      if (isEditingField) {
        if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
          e.preventDefault()
          handleSave()
        }
        return
      }

      const normalizedKey = e.key.toLowerCase()
      const isCanvasShortcutActive = isCanvasPointerActiveRef.current

      if ((e.key === 'Delete' || e.key === 'Backspace') && activeSelectionIds.length > 0) {
        e.preventDefault()
        removeNodesByIds(activeSelectionIds)
      }
      if ((e.ctrlKey || e.metaKey) && normalizedKey === 'c' && isCanvasShortcutActive && activeSelectionIds.length > 0) {
        e.preventDefault()
        copyNodesByIds(activeSelectionIds)
      }
      if ((e.ctrlKey || e.metaKey) && normalizedKey === 'v' && isCanvasShortcutActive) {
        e.preventDefault()
        pasteCopiedSelectionAtCursor()
      }
      if ((e.ctrlKey || e.metaKey) && normalizedKey === 'd' && activeSelectionIds.length > 0) {
        e.preventDefault()
        duplicateNodesByIds(activeSelectionIds)
      }
      if ((e.ctrlKey || e.metaKey) && normalizedKey === 'k' && activeSelectionIds.length === 1 && selectedNodeId) {
        e.preventDefault()
        disconnectNode()
      }
      if ((e.ctrlKey || e.metaKey) && normalizedKey === 's') {
        e.preventDefault()
        handleSave()
      }
    }

    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [activeSelectionIds, copyNodesByIds, disconnectNode, duplicateNodesByIds, handleSave, isCanvasInteractionDisabled, pasteCopiedSelectionAtCursor, removeNodesByIds, selectedNodeId])

  const handleNodeContextMenu = useCallback((event: React.MouseEvent, node: Node) => {
    event.preventDefault()
    setTemplateMenuPosition(null)
    if (!isFlowInteractive) {
      setContextMenu(null)
      return
    }
    if (node.selected && selectedNodes.length > 1) {
      setSelectedNodeId(null)
      setContextMenu({
        x: event.clientX,
        y: event.clientY,
        items: buildSelectionContextMenuItems(selectedNodes),
      })
      return
    }

    if (isGroupNode(node)) {
      setSelectedNodeId(node.id)
      setContextMenu({
        x: event.clientX,
        y: event.clientY,
        items: [
          {
            label: 'Edit title',
            icon: <Edit2 className="w-3.5 h-3.5" />,
            onClick: () => setSelectedNodeId(node.id),
          },
          {
            label: 'Ungroup',
            icon: <Workflow className="w-3.5 h-3.5" />,
            onClick: () => ungroupNode(node.id),
          },
        ],
      })
      return
    }

    setSelectedNodeId(node.id)
    const items: ContextMenuItem[] = [
      {
        label: 'Edit',
        icon: <Edit2 className="w-3.5 h-3.5" />,
        onClick: () => setSelectedNodeId(node.id),
      },
      {
        label: 'Copy',
        icon: <Copy className="w-3.5 h-3.5" />,
        shortcut: 'Ctrl+C',
        onClick: () => copyNodesByIds([node.id]),
      },
      {
        label: 'Duplicate',
        icon: <Copy className="w-3.5 h-3.5" />,
        shortcut: 'Ctrl+D',
        onClick: () => duplicateNodesByIds([node.id]),
      },
      {
        label: 'Disconnect',
        icon: <Scissors className="w-3.5 h-3.5" />,
        shortcut: 'Ctrl+K',
        onClick: () => {
          setEdges((eds) => eds.filter((e) => e.source !== node.id && e.target !== node.id))
        },
      },
      {
        divider: true,
        label: '',
        onClick: () => {},
      },
      {
        label: 'Delete',
        icon: <Trash2 className="w-3.5 h-3.5" />,
        shortcut: 'Del',
        danger: true,
        onClick: () => removeNodesByIds([node.id]),
      },
    ]
    setContextMenu({ x: event.clientX, y: event.clientY, items })
  }, [buildSelectionContextMenuItems, copyNodesByIds, duplicateNodesByIds, isFlowInteractive, removeNodesByIds, selectedNodes, setSelectedNodeId, setEdges, ungroupNode])

  const handleEdgeContextMenu = useCallback((event: React.MouseEvent, edge: Edge) => {
    event.preventDefault()
    setTemplateMenuPosition(null)
    if (!isFlowInteractive) {
      setContextMenu(null)
      return
    }
    const items: ContextMenuItem[] = [
      {
        label: 'Delete connection',
        icon: <Scissors className="w-3.5 h-3.5" />,
        danger: true,
        onClick: () => {
          setEdges((eds) => eds.filter((e) => e.id !== edge.id))
        },
      },
    ]
    setContextMenu({ x: event.clientX, y: event.clientY, items })
  }, [isFlowInteractive, setEdges])

  const handleSelectionContextMenu = useCallback((event: MouseEvent, selection: Node[]) => {
    event.preventDefault()
    setTemplateMenuPosition(null)
    if (!isFlowInteractive) {
      setContextMenu(null)
      return
    }
    setSelectedNodeId(null)
    setContextMenu({
      x: event.clientX,
      y: event.clientY,
      items: buildSelectionContextMenuItems(selection),
    })
  }, [buildSelectionContextMenuItems, isFlowInteractive, setSelectedNodeId])

  const handlePaneContextMenu = useCallback((event: React.MouseEvent) => {
    event.preventDefault()

    setTemplateMenuPosition(null)
    if (!isFlowInteractive) {
      setContextMenu(null)
      return
    }
    setSelectedNodeId(null)
    setContextMenu({
      x: event.clientX,
      y: event.clientY,
      items: buildPaneContextMenuItems(event.clientX, event.clientY),
      searchable: true,
      searchPlaceholder: 'Search nodes by name...',
      emptyMessage: 'No nodes match your search.',
    })
  }, [buildPaneContextMenuItems, isFlowInteractive, setSelectedNodeId])

  const renderedNodes = nodes.map((node) => {
    const isHighlightMode = highlightedNodes.size > 0
    const isHighlighted = highlightedNodes.has(node.id)
    const execStatus = nodeStatuses[node.id]
    const visualStatus = getVisualExecutionStatus(execStatus)

    if (!isHighlightMode) {
      return node
    }

    return {
      ...node,
      data: {
        ...node.data,
        status: visualStatus,
        enabled: isHighlighted,
        isHighlight: isHighlighted,
        executionLog: nodeLogs[node.id],
        canViewLog: isHighlighted && !!nodeLogs[node.id],
        onViewLog: () => setActiveNodeLogId(node.id),
      },
    }
  })

  const renderedNodeById = new Map(renderedNodes.map((node) => [node.id, node]))

  const renderedEdges = edges.map((edge) => {
    const isHighlightMode = highlightedNodes.size > 0
    const isHighlighted = highlightedNodes.has(edge.source) && highlightedNodes.has(edge.target)
    const baseEdgeOptions = edge.sourceHandle === 'tool' ? toolEdgeOptions : defaultEdgeOptions

    if (isHighlightMode) {
      return {
        ...edge,
        style: {
          ...baseEdgeOptions.style,
          strokeDasharray: isHighlighted ? 'none' : '6 4',
          stroke: isHighlighted ? '#f59e0b' : baseEdgeOptions.style.stroke,
          strokeWidth: isHighlighted ? 2.5 : 1.5,
          opacity: isHighlighted ? 1 : 0.3,
        },
        animated: isHighlighted,
        markerEnd: {
          ...baseEdgeOptions.markerEnd,
          color: isHighlighted ? '#f59e0b' : baseEdgeOptions.markerEnd.color,
        },
        data: {
          ...(typeof edge.data === 'object' && edge.data !== null ? edge.data : {}),
          useGradient: false,
        },
      }
    }

    const isConnectedToSelection = activeSelectionIdSet.has(edge.source) || activeSelectionIdSet.has(edge.target)
    const connectedSelectionCount = Number(activeSelectionIdSet.has(edge.source)) + Number(activeSelectionIdSet.has(edge.target))
    const edgeStrokeWidth = isConnectedToSelection
      ? connectedSelectionCount === 2 ? 2.9 : 2.6
      : (edge.style?.strokeWidth ?? baseEdgeOptions.style.strokeWidth)
    const sourceNode = renderedNodeById.get(edge.source)
    const targetNode = renderedNodeById.get(edge.target)
    const sourceBorderColor = getRenderedNodeBorderColor(sourceNode, activeSelectionIdSet)
    const targetBorderColor = getRenderedNodeBorderColor(targetNode, activeSelectionIdSet)

    return {
      ...edge,
      style: {
        ...baseEdgeOptions.style,
        ...edge.style,
        strokeWidth: edgeStrokeWidth,
      },
      markerEnd: {
        ...baseEdgeOptions.markerEnd,
        color: isConnectedToSelection ? targetBorderColor : baseEdgeOptions.markerEnd.color,
      },
      data: {
        ...(typeof edge.data === 'object' && edge.data !== null ? edge.data : {}),
        useGradient: isConnectedToSelection,
        gradientStartColor: sourceBorderColor,
        gradientEndColor: targetBorderColor,
      },
    }
  })

  if (isLoading) {
    return (
      <div className="h-screen flex items-center justify-center bg-bg">
        <Loader2 className="w-8 h-8 text-accent animate-spin" />
      </div>
    )
  }

  return (
    <div className="flex h-screen bg-bg">
      <div className="flex min-w-0 flex-1 flex-col">
        <input
          ref={addJSONTemplateInputRef}
          type="file"
          accept="application/json,.json"
          className="hidden"
          onChange={handleAddJSONTemplate}
        />

        {/* Canvas */}
        <div
          ref={reactFlowWrapper}
          className={cn(
            'relative flex-1 min-h-0',
            assistantEditLockActive && '[&_.react-flow__background]:opacity-45 [&_.react-flow__viewport]:opacity-70 [&_.react-flow__viewport]:saturate-50',
          )}
          onMouseEnter={handleCanvasMouseEnter}
          onMouseMove={handleCanvasMouseMove}
          onMouseLeave={handleCanvasMouseLeave}
        >
          <ReactFlow
            nodes={renderedNodes}
            edges={renderedEdges}
            onNodesChange={onNodesChangeHandler}
            onEdgesChange={onEdgesChange}
            onConnect={onConnect}
            onDrop={onDrop}
            onDragOver={onDragOver}
            onNodeDragStop={handleNodeDragStop}
            onNodeClick={(_, node) => {
              setTemplateMenuPosition(null)
              if (!isFlowInteractive) {
                setContextMenu(null)
                return
              }
              setSelectedNodeId(node.id)
              setContextMenu(null)
            }}
            onNodeContextMenu={handleNodeContextMenu}
            onSelectionContextMenu={handleSelectionContextMenu}
            onEdgeContextMenu={handleEdgeContextMenu}
            onPaneContextMenu={handlePaneContextMenu}
            onPaneClick={() => {
              setTemplateMenuPosition(null)
              setSelectedNodeId(null)
              setContextMenu(null)
            }}
            nodeTypes={nodeTypes}
            edgeTypes={edgeTypes}
            defaultEdgeOptions={defaultEdgeOptions}
            nodesDraggable={isFlowInteractive && !isCanvasInteractionBlocked}
            nodesConnectable={isFlowInteractive && !isCanvasInteractionBlocked}
            elementsSelectable={isFlowInteractive && !isCanvasInteractionBlocked}
            panOnDrag={isFlowInteractive && !isCanvasInteractionBlocked}
            zoomOnScroll={isFlowInteractive && !isCanvasInteractionBlocked}
            zoomOnPinch={isFlowInteractive && !isCanvasInteractionBlocked}
            zoomOnDoubleClick={isFlowInteractive && !isCanvasInteractionBlocked}
            panActivationKeyCode={isCanvasInteractionDisabled ? null : 'Space'}
            deleteKeyCode={isCanvasInteractionDisabled ? null : 'Backspace'}
            selectionKeyCode={isCanvasInteractionDisabled ? null : 'Shift'}
            multiSelectionKeyCode={isCanvasInteractionDisabled ? null : undefined}
            zoomActivationKeyCode={isCanvasInteractionDisabled ? null : undefined}
            disableKeyboardA11y={isCanvasInteractionDisabled}
            fitView
            fitViewOptions={{ padding: 0.2 }}
            minZoom={0.1}
            maxZoom={4}
            snapToGrid
            snapGrid={[15, 15]}
          >
            <Background color="#1e2d3d" gap={20} size={1} />

            <Panel
              position="top-left"
              className={cn(
                '!m-4 flex max-w-[min(32rem,calc(100vw-2rem))] pointer-events-none flex-col gap-1',
                isNodePaletteOpen && 'h-[calc(100vh-6.5rem)] max-h-[calc(100vh-6.5rem)]',
              )}
            >
              <FloatingEditorPanel className="w-full max-w-[min(32rem,calc(100vw-2rem))] overflow-hidden">
                <div className="space-y-3 px-4 py-3">
                  {editingDetails ? (
                    <div className="space-y-3">
                      <div className="flex flex-wrap items-center gap-2">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => navigate('/pipelines')}
                          className="rounded-lg"
                        >
                          <ArrowLeft className="w-4 h-4" />
                        </Button>
                        <span className="text-[11px] font-semibold uppercase tracking-[0.18em] text-text-dimmed">
                          Pipeline
                        </span>
                        <Badge variant={pipelineStatus === 'active' ? 'success' : pipelineStatus === 'draft' ? 'warning' : 'default'}>
                          {pipelineStatus}
                        </Badge>
                      </div>
                      <Input
                        ref={nameInputRef}
                        value={pipelineName}
                        onChange={(e) => setPipelineName(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === 'Escape') {
                            handleCancelDetailsEdit()
                          }
                        }}
                        placeholder="Pipeline name"
                        className="rounded-lg bg-bg-input text-base font-semibold"
                      />
                      <Textarea
                        value={pipelineDescription}
                        onChange={(e) => setPipelineDescription(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === 'Escape') {
                            handleCancelDetailsEdit()
                          }
                        }}
                        placeholder="Add a short description"
                        rows={3}
                        className="min-h-[92px] rounded-lg bg-bg-input text-sm resize-none"
                      />
                      <div className="flex flex-wrap items-center gap-2">
                        <Button variant="secondary" size="sm" onClick={() => setEditingDetails(false)} className="rounded-xl">
                          Done
                        </Button>
                        <Button variant="ghost" size="sm" onClick={handleCancelDetailsEdit} className="rounded-xl">
                          Cancel
                        </Button>
                      </div>
                    </div>
                  ) : (
                    <div className="flex items-start gap-3">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => navigate('/pipelines')}
                        className="mt-0.5 rounded-lg"
                      >
                        <ArrowLeft className="w-4 h-4" />
                      </Button>
                      <button
                        type="button"
                        onClick={() => setEditingDetails(true)}
                        className="block min-w-0 flex-1 rounded-lg border border-transparent bg-transparent p-0 text-left transition-colors hover:border-border hover:bg-bg-input/40"
                        title="Edit pipeline details"
                      >
                        <div className="flex items-start justify-between gap-3 px-2 py-1.5">
                          <div className="min-w-0 flex-1">
                            <div className="flex flex-wrap items-center gap-2">
                              <h1 className="truncate text-lg font-semibold text-text">
                                {pipelineName || 'Untitled'}
                              </h1>
                              <Badge variant={pipelineStatus === 'active' ? 'success' : pipelineStatus === 'draft' ? 'warning' : 'default'}>
                                {pipelineStatus}
                              </Badge>
                            </div>
                            <p className="mt-1 line-clamp-1 text-xs text-text-muted">
                              {pipelineDescription.trim() || 'Add a pipeline description'}
                            </p>
                          </div>
                          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-border bg-bg-input text-text-dimmed">
                            <Edit2 className="w-4 h-4" />
                          </span>
                        </div>
                      </button>
                    </div>
                  )}
                </div>
              </FloatingEditorPanel>

              <div className="pointer-events-auto flex min-h-0 flex-1 flex-col gap-1">
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => setIsNodePaletteOpen((current) => !current)}
                  className="w-fit rounded-xl shadow-lg"
                >
                  <List className="w-4 h-4" />
                  Nodes
                  {isNodePaletteOpen ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
                </Button>

                {isNodePaletteOpen && (
                  <NodePalette
                    onDragStart={onDragStart}
                    className="min-h-0 flex-1 max-h-full"
                  />
                )}
              </div>
            </Panel>

            <Panel position="top-right" className="!m-4 flex max-w-[min(42rem,calc(100vw-2rem))] justify-end pointer-events-none">
              <FloatingEditorPanel className="max-w-full px-2 py-2">
                <div className="flex flex-wrap items-center justify-end gap-2">
                  <Button
                    variant="secondary"
                    size="sm"
                    loading={isSaving}
                    onClick={() => handleToggleStatus(pipelineStatus === 'active' ? 'draft' : 'active')}
                    className="rounded-xl"
                  >
                    <Power className="w-4 h-4" />
                    {pipelineStatus === 'active' ? 'Deactivate' : 'Activate'}
                  </Button>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => {
                      setTemplateMenuPosition(null)
                      if (showExecutionLog) {
                        handleCloseExecutionLog()
                        return
                      }
                      setShowExecutionLog(true)
                    }}
                    className={cn(
                      'rounded-xl',
                      showExecutionLog && 'border-accent/50 bg-accent/10 text-accent',
                    )}
                  >
                    <ListChecks className="w-4 h-4" />
                    Log
                  </Button>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={handleTemplateMenuToggle}
                    className={cn(
                      'rounded-xl',
                      templateMenuPosition && 'border-accent/50 bg-accent/10 text-accent',
                    )}
                  >
                    <Workflow className="w-4 h-4" />
                    Template
                    {templateMenuPosition ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
                  </Button>
                  <Button
                    variant="secondary"
                    size="sm"
                    loading={isSaving}
                    onClick={handleSave}
                    className="rounded-xl"
                  >
                    <Save className="w-4 h-4" />
                    Save
                  </Button>
                  <Button
                    size="sm"
                    loading={isRunning}
                    onClick={handleRun}
                    className="rounded-xl"
                  >
                    <Play className="w-4 h-4" />
                    Run
                  </Button>
                </div>
              </FloatingEditorPanel>
            </Panel>

            {activeSelectionIds.length > 0 && (
              <Panel position="bottom-right" className="!m-4">
                <div className="bg-bg-elevated border border-border rounded-lg shadow-lg p-2 flex gap-1">
                  {!activeSelectionDuplicationIssue && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={duplicateActiveSelection}
                      title={activeSelectionIds.length > 1 ? 'Duplicate selection' : 'Duplicate'}
                    >
                      <Copy className="w-4 h-4" />
                    </Button>
                  )}
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => removeNodesByIds(activeSelectionIds)}
                    title={activeSelectionIds.length > 1 ? 'Delete selection' : 'Delete'}
                  >
                    <Trash2 className="w-4 h-4 text-red-400" />
                  </Button>
                </div>
              </Panel>
            )}

            <EditorAssistantDock
              triggerControlsLeft={(
                <>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={handleZoomIn}
                    disabled={isCanvasInteractionBlocked}
                    className={canvasControlButtonClass}
                    title="Zoom in"
                    aria-label="Zoom in"
                  >
                    <Plus className="w-3.5 h-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={handleZoomOut}
                    disabled={isCanvasInteractionBlocked}
                    className={canvasControlButtonClass}
                    title="Zoom out"
                    aria-label="Zoom out"
                  >
                    <Minus className="w-3.5 h-3.5" />
                  </Button>
                </>
              )}
              triggerControlsRight={(
                <>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={handleFitCanvas}
                    disabled={isCanvasInteractionBlocked}
                    className={canvasControlButtonClass}
                    title="Fit view"
                    aria-label="Fit view"
                  >
                    <ScanSearch className="w-3.5 h-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setIsFlowInteractive((current) => !current)}
                    disabled={isCanvasInteractionBlocked}
                    className={cn(
                      'h-8 min-w-8 rounded-xl px-1.5',
                      isFlowInteractive
                        ? 'border border-accent/30 bg-accent/10 text-accent hover:bg-accent/15 hover:text-accent-hover'
                        : 'border border-amber-500/30 bg-amber-500/10 text-amber-300 hover:bg-amber-500/15 hover:text-amber-200',
                    )}
                    title={isFlowInteractive ? 'Lock interactivity' : 'Unlock interactivity'}
                    aria-label={isFlowInteractive ? 'Lock interactivity' : 'Unlock interactivity'}
                  >
                    {isFlowInteractive ? <LockOpen className="w-3.5 h-3.5" /> : <Lock className="w-3.5 h-3.5" />}
                  </Button>
                </>
              )}
              pipelineKey={id ?? 'new'}
              pipeline={{
                name: pipelineName,
                description: pipelineDescription,
                status: pipelineStatus,
                nodes,
                edges,
                viewport: getViewport(),
              }}
              selection={{
                selected_node_id: selectedNodeId ?? undefined,
                selected_node_ids: selectedNodeIds,
              }}
              providers={llmProviders}
              injectedLogAttachment={assistantLogAttachment}
              onApplyOperations={handleApplyAssistantOperations}
              onEditLockChange={setAssistantEditLockActive}
            />
            
            {selectedNode && !showExecutionLog && (
              <Panel position="top-right" className="!m-4 !mt-28 flex max-w-[calc(100vw-2rem)] sm:!mt-20">
                <NodeConfigPanel
                  pipelineId={id!}
                  nodes={nodes}
                  edges={edges}
                  nodeId={selectedNode.id}
                  nodeType={selectedNode.data.type as NodeType}
                  nodeLabel={selectedNode.data.label || 'Node'}
                  config={selectedNode.data.config || {}}
                  onUpdate={updateNodeConfig}
                  onLabelChange={updateNodeLabel}
                  onRemoveSourceHandles={removeSelectedNodeSourceHandles}
                  onOverlayOpenChange={setIsBlockingOverlayOpen}
                  onClose={() => setSelectedNodeId(null)}
                />
              </Panel>
            )}

            {showExecutionLog && (
              <Panel position="top-right" className="!m-4 !mt-28 flex max-w-[calc(100vw-2rem)] sm:!mt-20">
                <ExecutionLog
                  pipelineId={id!}
                  isOpen={showExecutionLog}
                  onClose={handleCloseExecutionLog}
                  onExecutionSelect={handleExecutionHighlight}
                  onAddToAssistant={handleAddExecutionLogToAssistant}
                />
              </Panel>
            )}
            
          </ReactFlow>

          {assistantEditLockActive && (
            <div className="pointer-events-none absolute left-4 top-4 z-20">
              <div className="rounded-full border border-border bg-bg-elevated/95 px-3 py-2 shadow-lg backdrop-blur">
                <div className="flex items-center gap-2 text-xs">
                  <Brain className="h-3.5 w-3.5 animate-pulse text-accent" />
                  <span className="font-medium text-text">Applying live edits</span>
                  <span className="text-text-dimmed">Canvas locked</span>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>

      {activeNodeLog && activeNodeLogNode && (
        <NodeExecutionModal
          nodeId={activeNodeLogId!}
          nodeLabel={activeNodeLogNode.data.label || activeNodeLogNode.id}
          nodeType={activeNodeLogNode.data.type as NodeType}
          log={activeNodeLog}
          onClose={() => setActiveNodeLogId(null)}
        />
      )}

      {/* Context Menu */}
      {contextMenu && (
        <ContextMenu
          x={contextMenu.x}
          y={contextMenu.y}
          items={contextMenu.items}
          searchable={contextMenu.searchable}
          searchPlaceholder={contextMenu.searchPlaceholder}
          emptyMessage={contextMenu.emptyMessage}
          onClose={() => setContextMenu(null)}
        />
      )}

      {templateMenuPosition && (
        <ContextMenu
          x={templateMenuPosition.x}
          y={templateMenuPosition.y}
          items={templateMenuItems}
          onClose={() => setTemplateMenuPosition(null)}
        />
      )}

      <Modal
        open={showSaveTemplateModal}
        title="Save Pipeline as Template"
        description="Save the current in-memory canvas, including unsaved edits, as a reusable template."
        onClose={() => {
          if (!saveTemplateMutation.isPending) {
            setShowSaveTemplateModal(false)
          }
        }}
      >
        <div className="space-y-4">
          <div className="space-y-2">
            <label className="block text-sm font-medium text-text" htmlFor="template-name">
              Template name
            </label>
            <Input
              id="template-name"
              value={templateDraftName}
              onChange={(event) => setTemplateDraftName(event.target.value)}
              placeholder="My reusable template"
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault()
                  handleSubmitSaveTemplate()
                }
              }}
            />
          </div>
          <div className="space-y-2">
            <label className="block text-sm font-medium text-text" htmlFor="template-description">
              Description
            </label>
            <Textarea
              id="template-description"
              value={templateDraftDescription}
              onChange={(event) => setTemplateDraftDescription(event.target.value)}
              placeholder="Explain when this template is useful"
              rows={3}
            />
          </div>
          <div className="flex justify-end gap-2">
            <Button
              variant="ghost"
              onClick={() => setShowSaveTemplateModal(false)}
              disabled={saveTemplateMutation.isPending}
            >
              Cancel
            </Button>
            <Button onClick={handleSubmitSaveTemplate} loading={saveTemplateMutation.isPending}>
              <Save className="w-4 h-4" />
              Save Template
            </Button>
          </div>
        </div>
      </Modal>

      <Modal
        open={showTemplateLibraryModal}
        title="Add Saved Template"
        description="Append a saved template to the current pipeline. Node and edge IDs will be remapped automatically."
        onClose={() => {
          if (!appendTemplateMutation.isPending) {
            setShowTemplateLibraryModal(false)
          }
        }}
      >
        {areTemplatesLoading ? (
          <div className="py-8 text-sm text-text-muted">Loading templates...</div>
        ) : templates.length === 0 ? (
          <div className="space-y-3 py-4">
            <p className="text-sm text-text-muted">
              No saved templates yet. Save this pipeline as a template first, or import JSON on the Templates page.
            </p>
            <div className="flex justify-end">
              <Button variant="ghost" onClick={() => setShowTemplateLibraryModal(false)}>
                Close
              </Button>
            </div>
          </div>
        ) : (
          <div className="space-y-3">
            {templates.map((template) => {
              const isLoadingTemplate = activeTemplateId === template.id && appendTemplateMutation.isPending

              return (
                <div
                  key={template.id}
                  className="rounded-xl border border-border bg-bg px-4 py-3"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <h3 className="truncate text-sm font-semibold text-text">{template.name}</h3>
                      <p className="mt-1 text-xs text-text-muted">
                        {template.description?.trim() || 'No description'}
                      </p>
                    </div>
                    <Badge>{template.category}</Badge>
                  </div>
                  <div className="mt-3 flex justify-end">
                    <Button
                      size="sm"
                      onClick={() => {
                        setActiveTemplateId(template.id)
                        appendTemplateMutation.mutate(template.id)
                      }}
                      loading={isLoadingTemplate}
                    >
                      <Workflow className="w-4 h-4" />
                      Add to Current Pipeline
                    </Button>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </Modal>
    </div>
  )
}

function PipelineEditorWithProvider() {
  return (
    <ReactFlowProvider>
      <PipelineEditor />
    </ReactFlowProvider>
  )
}

export default PipelineEditorWithProvider

function FloatingEditorPanel({
  className,
  children,
}: {
  className?: string
  children: React.ReactNode
}) {
  return (
    <div
      className={cn(
        'pointer-events-auto rounded-xl border border-border bg-bg-elevated shadow-xl',
        className,
      )}
    >
      {children}
    </div>
  )
}
