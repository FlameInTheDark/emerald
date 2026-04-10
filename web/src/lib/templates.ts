import type { Edge, Node } from '@xyflow/react'
import type { NodeExecution, NodeType, NodeTypeDefinition, SecretMetadata, TemplateSuggestion } from '../types'

type NodeOutputHint = {
  expression: string
  label: string
  description?: string
}

const SAMPLE_STRING_LIMIT = 1200
const SAMPLE_PREVIEW_LIMIT = 220
const SAMPLE_ARRAY_LIMIT = 6
const SAMPLE_OBJECT_LIMIT = 12
const SAMPLE_DEPTH_LIMIT = 4

const NODE_OUTPUT_HINTS: Partial<Record<NodeType, NodeOutputHint[]>> = {
  'action:proxmox_list_nodes': [
    { expression: 'input.clusterId', label: 'Cluster ID' },
    { expression: 'input.clusterName', label: 'Cluster name' },
    { expression: 'input.count', label: 'Node count' },
    { expression: 'input.nodes', label: 'Nodes list (JSON)' },
    { expression: 'input.nodes[0].node', label: 'First node name' },
    { expression: 'input.nodes[0].status', label: 'First node status' },
  ],
  'action:proxmox_list_workloads': [
    { expression: 'input.clusterName', label: 'Cluster name' },
    { expression: 'input.workloads', label: 'Workloads list (JSON)' },
    { expression: 'input.vms', label: 'VM list (JSON)' },
    { expression: 'input.containers', label: 'Container list (JSON)' },
    { expression: 'input.workloads[0].vmid', label: 'First workload VMID' },
    { expression: 'input.workloads[0].status', label: 'First workload status' },
    { expression: 'input.workloads[0].node', label: 'First workload node' },
  ],
  'action:vm_start': [
    { expression: 'input.clusterName', label: 'Cluster name' },
    { expression: 'input.node', label: 'Node name' },
    { expression: 'input.vmid', label: 'VM ID' },
    { expression: 'input.status', label: 'Action status' },
  ],
  'action:vm_stop': [
    { expression: 'input.clusterName', label: 'Cluster name' },
    { expression: 'input.node', label: 'Node name' },
    { expression: 'input.vmid', label: 'VM ID' },
    { expression: 'input.status', label: 'Action status' },
  ],
  'action:vm_clone': [
    { expression: 'input.clusterName', label: 'Cluster name' },
    { expression: 'input.node', label: 'Node name' },
    { expression: 'input.vmid', label: 'Source VM ID' },
    { expression: 'input.newId', label: 'New VM ID' },
    { expression: 'input.newName', label: 'New VM name' },
    { expression: 'input.status', label: 'Action status' },
  ],
  'action:http': [
    { expression: 'input.status_code', label: 'HTTP status code' },
    { expression: 'input.response', label: 'Response body (JSON)' },
  ],
  'action:kubernetes_api_resources': [
    { expression: 'input.resources', label: 'API resources list (JSON)' },
    { expression: 'input.count', label: 'Resource type count' },
  ],
  'action:kubernetes_list_resources': [
    { expression: 'input.clusterName', label: 'Cluster name' },
    { expression: 'input.namespace', label: 'Namespace' },
    { expression: 'input.resourceRef.kind', label: 'Resolved kind' },
    { expression: 'input.items', label: 'Resource list (JSON)' },
    { expression: 'input.count', label: 'Item count' },
  ],
  'action:kubernetes_get_resource': [
    { expression: 'input.clusterName', label: 'Cluster name' },
    { expression: 'input.namespace', label: 'Namespace' },
    { expression: 'input.resourceRef.name', label: 'Resource name' },
    { expression: 'input.item', label: 'Fetched resource (JSON)' },
  ],
  'action:kubernetes_apply_manifest': [
    { expression: 'input.clusterName', label: 'Cluster name' },
    { expression: 'input.namespace', label: 'Namespace' },
    { expression: 'input.refs', label: 'Applied references (JSON)' },
    { expression: 'input.items', label: 'Applied resources (JSON)' },
    { expression: 'input.count', label: 'Applied resource count' },
  ],
  'action:kubernetes_patch_resource': [
    { expression: 'input.resourceRef.name', label: 'Patched resource name' },
    { expression: 'input.item', label: 'Patched resource (JSON)' },
  ],
  'action:kubernetes_delete_resource': [
    { expression: 'input.resourceRef.kind', label: 'Deleted resource kind' },
    { expression: 'input.deleted', label: 'Deleted item count' },
    { expression: 'input.mode', label: 'Delete mode' },
  ],
  'action:kubernetes_scale_resource': [
    { expression: 'input.resourceRef.name', label: 'Scaled workload name' },
    { expression: 'input.item.spec.replicas', label: 'Replica count' },
    { expression: 'input.item.status.readyReplicas', label: 'Ready replicas' },
  ],
  'action:kubernetes_rollout_restart': [
    { expression: 'input.resourceRef.name', label: 'Restarted workload name' },
    { expression: 'input.item.metadata.annotations', label: 'Updated annotations (JSON)' },
  ],
  'action:kubernetes_rollout_status': [
    { expression: 'input.resourceRef.name', label: 'Rollout name' },
    { expression: 'input.status.ready', label: 'Rollout ready' },
    { expression: 'input.status.message', label: 'Rollout status message' },
    { expression: 'input.status.readyReplicas', label: 'Ready replicas' },
  ],
  'action:kubernetes_pod_logs': [
    { expression: 'input.resourceRef.name', label: 'Pod name' },
    { expression: 'input.logs', label: 'Pod logs' },
  ],
  'action:kubernetes_pod_exec': [
    { expression: 'input.resourceRef.name', label: 'Pod name' },
    { expression: 'input.result.stdout', label: 'Command stdout' },
    { expression: 'input.result.stderr', label: 'Command stderr' },
  ],
  'action:kubernetes_events': [
    { expression: 'input.namespace', label: 'Namespace' },
    { expression: 'input.items', label: 'Events list (JSON)' },
    { expression: 'input.count', label: 'Event count' },
    { expression: 'input.items[0].reason', label: 'First event reason' },
  ],
  'logic:condition': [
    { expression: 'input.result', label: 'Condition result' },
    { expression: 'input.condition', label: 'Condition expression' },
    { expression: 'input.error', label: 'Evaluation error' },
  ],
  'logic:switch': [
    { expression: 'input.conditions', label: 'Condition evaluations (JSON)' },
    { expression: 'input.matches', label: 'Branch match map (JSON)' },
    { expression: 'input.matched', label: 'Matched conditions (JSON)' },
    { expression: 'input.hasMatch', label: 'Any condition matched' },
    { expression: 'input.defaultMatched', label: 'Else branch matched' },
  ],
  'logic:aggregate': [
    { expression: 'input.count', label: 'Aggregated input count' },
    { expression: 'input.items', label: 'Ordered upstream outputs (JSON)' },
    { expression: 'input.entries', label: 'Upstream entries with source metadata (JSON)' },
    { expression: 'input.entries[0].nodeId', label: 'First resolved node ID' },
    { expression: 'input.entries[0].originalNodeId', label: 'First original node ID' },
    { expression: 'input.byNodeId', label: 'Upstream outputs keyed by resolved node ID (JSON)' },
  ],
  'llm:prompt': [
    { expression: 'input.prompt', label: 'Rendered prompt' },
    { expression: 'input.content', label: 'LLM response text' },
    { expression: 'input.usage', label: 'Token usage (JSON)' },
    { expression: 'input.usage.total_tokens', label: 'Total tokens' },
  ],
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export function parseExecutionJSON(value?: string): unknown {
  if (!value) {
    return undefined
  }

  try {
    return JSON.parse(value)
  } catch {
    return value
  }
}

function collectPaths(
  value: unknown,
  path: string,
  results: TemplateSuggestion[],
  seen: Set<string>,
  depth: number,
): void {
  if (depth > 3 || !path) {
    return
  }

  addSuggestion(results, seen, path, path, describeValue(value))

  if (Array.isArray(value)) {
    if (value.length === 0) {
      return
    }
    collectPaths(value[0], `${path}[0]`, results, seen, depth + 1)
    return
  }

  if (!isObject(value)) {
    return
  }

  Object.entries(value).forEach(([key, child]) => {
    collectPaths(child, `${path}.${key}`, results, seen, depth + 1)
  })
}

function describeValue(value: unknown): string | undefined {
  if (Array.isArray(value)) {
    return 'Array value, inserted as JSON.'
  }
  if (isObject(value)) {
    return 'Object value, inserted as JSON.'
  }
  if (typeof value === 'boolean' || typeof value === 'number') {
    return 'Scalar value.'
  }
  if (typeof value === 'string') {
    return 'String value.'
  }
  return undefined
}

function addSuggestion(
  results: TemplateSuggestion[],
  seen: Set<string>,
  expression: string,
  label: string,
  description?: string,
): void {
  if (!expression || seen.has(expression)) {
    return
  }

  seen.add(expression)
  results.push({
    expression,
    template: `{{${expression}}}`,
    label,
    description,
    kind: 'template',
  })
}

function buildSchemaSuggestions(
  nodeType: NodeType | undefined,
  nodeDefinitions?: Record<string, NodeTypeDefinition>,
  sourceLabel?: string,
): TemplateSuggestion[] {
  const runtimeHints = nodeType ? nodeDefinitions?.[nodeType]?.outputHints : undefined
  const hints = runtimeHints && runtimeHints.length > 0
    ? runtimeHints
    : nodeType
    ? NODE_OUTPUT_HINTS[nodeType]
    : undefined
  if (!hints) {
    return []
  }

  return hints.map((hint) => ({
    expression: hint.expression,
    template: `{{${hint.expression}}}`,
    label: hint.label,
    description: sourceLabel ? `${hint.description ?? 'Suggested field.'} Source: ${sourceLabel}.` : hint.description,
  }))
}

function buildSecretSuggestions(secrets: SecretMetadata[] = []): TemplateSuggestion[] {
  return secrets.map((secret) => ({
    expression: `secret.${secret.name}`,
    template: `{{secret.${secret.name}}}`,
    label: `Secret: ${secret.name}`,
    description: 'Global secret value resolved at runtime.',
    kind: 'template',
    badge: 'Secret',
  }))
}

function mergeSourceOutputs(outputs: unknown[]): Record<string, unknown> {
  return outputs.reduce<Record<string, unknown>>((acc, current) => {
    if (isObject(current)) {
      Object.assign(acc, current)
    }
    return acc
  }, {})
}

function truncatePlainText(value: string, limit: number): string {
  if (value.length <= limit) {
    return value
  }

  return `${value.slice(0, limit)}... [truncated ${value.length - limit} chars]`
}

function compactSampleValue(value: unknown, depth = 0): unknown {
  if (typeof value === 'string') {
    return truncatePlainText(value, SAMPLE_STRING_LIMIT)
  }

  if (typeof value === 'number' || typeof value === 'boolean' || value === null || value === undefined) {
    return value
  }

  if (depth >= SAMPLE_DEPTH_LIMIT) {
    if (Array.isArray(value)) {
      return `[array truncated, ${value.length} item(s)]`
    }

    if (isObject(value)) {
      return '[object truncated]'
    }
  }

  if (Array.isArray(value)) {
    const next = value
      .slice(0, SAMPLE_ARRAY_LIMIT)
      .map((item) => compactSampleValue(item, depth + 1))

    if (value.length > SAMPLE_ARRAY_LIMIT) {
      next.push(`... ${value.length - SAMPLE_ARRAY_LIMIT} more item(s) omitted`)
    }

    return next
  }

  if (!isObject(value)) {
    return String(value)
  }

  const entries = Object.entries(value)
  const next = entries
    .slice(0, SAMPLE_OBJECT_LIMIT)
    .reduce<Record<string, unknown>>((acc, [key, child]) => {
      acc[key] = compactSampleValue(child, depth + 1)
      return acc
    }, {})

  if (entries.length > SAMPLE_OBJECT_LIMIT) {
    next._truncated = `${entries.length - SAMPLE_OBJECT_LIMIT} more field(s) omitted`
  }

  return next
}

function renderSampleBlock(value: unknown, heading: string): { template: string; preview: string } | null {
  if (value === undefined) {
    return null
  }

  const compactValue = compactSampleValue(value)
  const isStructured = Array.isArray(compactValue) || isObject(compactValue) || typeof compactValue === 'number' || typeof compactValue === 'boolean' || compactValue === null
  const language = isStructured ? 'json' : 'text'
  const renderedValue = isStructured
    ? JSON.stringify(compactValue, null, 2)
    : truncatePlainText(String(compactValue), SAMPLE_STRING_LIMIT)

  if (!renderedValue) {
    return null
  }

  const fence = renderedValue.includes('```') ? '~~~' : '```'

  return {
    template: `${heading}\n${fence}${language}\n${renderedValue}\n${fence}\n`,
    preview: truncatePlainText(renderedValue, SAMPLE_PREVIEW_LIMIT),
  }
}

function buildPromptSampleSuggestions(
  selectedNodeId: string,
  latestNodeExecutions: NodeExecution[],
  mergedOutput: Record<string, unknown>,
): TemplateSuggestion[] {
  const currentExecution = latestNodeExecutions.find((execution) => execution.node_id === selectedNodeId)
  const currentInput = parseExecutionJSON(currentExecution?.input)
  const currentSample = renderSampleBlock(
    currentInput,
    'Example runtime input for this node from the latest execution:',
  )

  if (currentSample) {
    return [{
      expression: 'sample.current_input',
      template: currentSample.template,
      label: 'Latest input sample',
      description: 'Insert a compact snapshot of the latest recorded input payload for this node.',
      kind: 'sample',
      preview: currentSample.preview,
    }]
  }

  if (Object.keys(mergedOutput).length === 0) {
    return []
  }

  const mergedSample = renderSampleBlock(
    mergedOutput,
    'Example merged input built from the latest upstream execution data:',
  )

  if (!mergedSample) {
    return []
  }

  return [{
    expression: 'sample.merged_input',
    template: mergedSample.template,
    label: 'Merged input sample',
    description: 'Insert an approximate input example assembled from the latest upstream node outputs.',
    kind: 'sample',
    preview: mergedSample.preview,
  }]
}

export function buildTemplateSuggestions(
  selectedNodeId: string,
  nodes: Node[],
  edges: Edge[],
  latestNodeExecutions: NodeExecution[] = [],
  nodeDefinitions?: Record<string, NodeTypeDefinition>,
  secrets: SecretMetadata[] = [],
): TemplateSuggestion[] {
  const results: TemplateSuggestion[] = []
  const seen = new Set<string>()

  const incomingEdges = edges.filter((edge) => edge.target === selectedNodeId)
  if (incomingEdges.length === 0) {
    return results
  }

  addSuggestion(results, seen, 'input', 'Entire merged input', 'All incoming data, inserted as JSON.')

  const sourceNodes = incomingEdges
    .map((edge) => nodes.find((node) => node.id === edge.source))
    .filter((node): node is Node => !!node)

  const latestOutputs = sourceNodes
    .map((node) => latestNodeExecutions.find((execution) => execution.node_id === node.id))
    .map((execution) => parseExecutionJSON(execution?.output))
    .filter((value): value is Record<string, unknown> => isObject(value))

  const mergedOutput = mergeSourceOutputs(latestOutputs)
  if (Object.keys(mergedOutput).length > 0) {
    collectPaths(mergedOutput, 'input', results, seen, 0)
  }

  sourceNodes.forEach((node) => {
    const sourceType = node.data?.type as NodeType | undefined
    const sourceLabel = (node.data?.label as string | undefined) || node.id
    buildSchemaSuggestions(sourceType, nodeDefinitions, sourceLabel).forEach((suggestion) => {
      addSuggestion(results, seen, suggestion.expression, suggestion.label, suggestion.description)
    })
  })

  buildSecretSuggestions(secrets).forEach((suggestion) => {
    addSuggestion(results, seen, suggestion.expression, suggestion.label, suggestion.description)
  })

  return results
}

export function buildPromptInsertSuggestions(
  selectedNodeId: string,
  nodes: Node[],
  edges: Edge[],
  latestNodeExecutions: NodeExecution[] = [],
  nodeDefinitions?: Record<string, NodeTypeDefinition>,
  secrets: SecretMetadata[] = [],
): TemplateSuggestion[] {
  const templateSuggestions = buildTemplateSuggestions(selectedNodeId, nodes, edges, latestNodeExecutions, nodeDefinitions, secrets)
  const incomingEdges = edges.filter((edge) => edge.target === selectedNodeId)
  const sourceNodes = incomingEdges
    .map((edge) => nodes.find((node) => node.id === edge.source))
    .filter((node): node is Node => !!node)

  const latestOutputs = sourceNodes
    .map((node) => latestNodeExecutions.find((execution) => execution.node_id === node.id))
    .map((execution) => parseExecutionJSON(execution?.output))
    .filter((value): value is Record<string, unknown> => isObject(value))

  const mergedOutput = mergeSourceOutputs(latestOutputs)

  return [
    ...templateSuggestions,
    ...buildPromptSampleSuggestions(selectedNodeId, latestNodeExecutions, mergedOutput),
  ]
}
