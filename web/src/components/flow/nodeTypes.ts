import type { NodeDefinition, NodeType, NodeTypeDefinition, NodeCategory } from '../../types'

export interface NodeMenuGroup {
  id: string
  label: string
  path: string[]
  groups: NodeMenuGroup[]
  types: NodeTypeDefinition[]
  totalCount: number
}

export interface NodeMenuCategory {
  id: string
  label: string
  color: string
  groups: NodeMenuGroup[]
  types: NodeTypeDefinition[]
  totalCount: number
}

const KUBERNETES_ACTION_TYPES: NodeTypeDefinition[] = [
  {
    type: 'action:kubernetes_api_resources',
    label: 'K8s API Resources',
    description: 'Read the API resources available on a Kubernetes cluster',
    icon: 'globe',
    category: 'action',
    color: '#10b981',
    defaultConfig: { clusterId: '' },
  },
  {
    type: 'action:kubernetes_list_resources',
    label: 'K8s List Resources',
    description: 'List Kubernetes resources by apiVersion and kind or resource name',
    icon: 'list',
    category: 'action',
    color: '#10b981',
    defaultConfig: { clusterId: '', namespace: '', apiVersion: 'v1', kind: '', resource: '', labelSelector: '', fieldSelector: '', allNamespaces: false, limit: 0 },
  },
  {
    type: 'action:kubernetes_get_resource',
    label: 'K8s Get Resource',
    description: 'Fetch a single Kubernetes resource',
    icon: 'workflow',
    category: 'action',
    color: '#10b981',
    defaultConfig: { clusterId: '', namespace: '', apiVersion: 'v1', kind: '', resource: '', name: '' },
  },
  {
    type: 'action:kubernetes_apply_manifest',
    label: 'K8s Apply Manifest',
    description: 'Apply Kubernetes manifests with server-side apply',
    icon: 'copy',
    category: 'action',
    color: '#10b981',
        defaultConfig: { clusterId: '', namespace: '', manifest: '', fieldManager: 'emerald', force: false },
  },
  {
    type: 'action:kubernetes_patch_resource',
    label: 'K8s Patch Resource',
    description: 'Patch a Kubernetes resource',
    icon: 'wrench',
    category: 'action',
    color: '#10b981',
    defaultConfig: { clusterId: '', namespace: '', apiVersion: 'apps/v1', kind: '', resource: '', name: '', patchType: 'merge', patch: '' },
  },
  {
    type: 'action:kubernetes_delete_resource',
    label: 'K8s Delete Resource',
    description: 'Delete a Kubernetes resource or matching collection',
    icon: 'trash-2',
    category: 'action',
    color: '#10b981',
    defaultConfig: { clusterId: '', namespace: '', apiVersion: 'apps/v1', kind: '', resource: '', name: '', labelSelector: '', fieldSelector: '', propagationPolicy: 'background' },
  },
  {
    type: 'action:kubernetes_scale_resource',
    label: 'K8s Scale Resource',
    description: 'Update workload replica count',
    icon: 'workflow',
    category: 'action',
    color: '#10b981',
    defaultConfig: { clusterId: '', namespace: '', apiVersion: 'apps/v1', kind: 'Deployment', resource: '', name: '', replicas: 1 },
  },
  {
    type: 'action:kubernetes_rollout_restart',
    label: 'K8s Rollout Restart',
    description: 'Restart a rollout-capable workload',
    icon: 'refresh-cw',
    category: 'action',
    color: '#10b981',
    defaultConfig: { clusterId: '', namespace: '', apiVersion: 'apps/v1', kind: 'Deployment', resource: '', name: '' },
  },
  {
    type: 'action:kubernetes_rollout_status',
    label: 'K8s Rollout Status',
    description: 'Wait for a workload rollout to become ready',
    icon: 'refresh-cw',
    category: 'action',
    color: '#10b981',
    defaultConfig: { clusterId: '', namespace: '', apiVersion: 'apps/v1', kind: 'Deployment', resource: '', name: '', timeoutSeconds: 300 },
  },
  {
    type: 'action:kubernetes_pod_logs',
    label: 'K8s Pod Logs',
    description: 'Read logs from a Kubernetes pod',
    icon: 'list',
    category: 'action',
    color: '#10b981',
    defaultConfig: { clusterId: '', namespace: '', name: '', container: '', tailLines: 0, sinceSeconds: 0, timestamps: false, previous: false },
  },
  {
    type: 'action:kubernetes_pod_exec',
    label: 'K8s Pod Exec',
    description: 'Run a command in a Kubernetes pod',
    icon: 'code',
    category: 'action',
    color: '#10b981',
    defaultConfig: { clusterId: '', namespace: '', name: '', container: '', command: ['sh', '-c', 'echo hello'] },
  },
  {
    type: 'action:kubernetes_events',
    label: 'K8s Events',
    description: 'List recent Kubernetes events',
    icon: 'message-square',
    category: 'action',
    color: '#10b981',
    defaultConfig: { clusterId: '', namespace: '', limit: 50, fieldSelector: '', involvedObjectName: '', involvedObjectKind: '', involvedObjectUID: '' },
  },
]

const KUBERNETES_TOOL_TYPES: NodeTypeDefinition[] = KUBERNETES_ACTION_TYPES.map((typeDef) => ({
  ...typeDef,
  type: typeDef.type.replace('action:', 'tool:') as NodeType,
  label: `${typeDef.label} Tool`,
  category: 'tool',
  color: '#38bdf8',
}))

export const NODE_CATEGORIES: NodeCategory[] = [
  {
    id: 'trigger',
    label: 'Triggers',
    color: '#f59e0b',
    types: [
      {
        type: 'trigger:manual',
        label: 'Manual Trigger',
        description: 'Start pipeline manually',
        icon: 'zap',
        category: 'trigger',
        color: '#f59e0b',
        defaultConfig: {},
      },
      {
        type: 'trigger:cron',
        label: 'Cron Trigger',
        description: 'Schedule pipeline with cron expression',
        icon: 'clock',
        category: 'trigger',
        color: '#f59e0b',
        defaultConfig: { schedule: '0 * * * *', timezone: 'UTC' },
      },
      {
        type: 'trigger:webhook',
        label: 'Webhook Trigger',
        description: 'Trigger pipeline via HTTP webhook',
        icon: 'webhook',
        category: 'trigger',
        color: '#f59e0b',
        defaultConfig: { path: '', method: 'POST' },
      },
      {
        type: 'trigger:channel_message',
        label: 'Channel Message',
        description: 'Trigger when a connected channel user sends a message',
        icon: 'message-square',
        category: 'trigger',
        color: '#f59e0b',
        defaultConfig: { channelId: '' },
      },
    ],
  },
  {
    id: 'action',
    label: 'Actions',
    color: '#10b981',
    types: [
      {
        type: 'action:proxmox_list_nodes',
        label: 'List Nodes',
        description: 'List nodes in a selected Proxmox cluster',
        icon: 'globe',
        category: 'action',
        color: '#10b981',
        defaultConfig: { clusterId: '' },
      },
      {
        type: 'action:proxmox_list_workloads',
        label: 'List VMs/CTs',
        description: 'List virtual machines and containers in a cluster',
        icon: 'link',
        category: 'action',
        color: '#10b981',
        defaultConfig: { clusterId: '', node: '' },
      },
      {
        type: 'action:vm_start',
        label: 'Start VM',
        description: 'Start a Proxmox VM',
        icon: 'play',
        category: 'action',
        color: '#10b981',
        defaultConfig: { clusterId: '', node: '', vmid: 0 },
      },
      {
        type: 'action:vm_stop',
        label: 'Stop VM',
        description: 'Stop a Proxmox VM immediately',
        icon: 'square',
        category: 'action',
        color: '#10b981',
        defaultConfig: { clusterId: '', node: '', vmid: 0 },
      },
      {
        type: 'action:vm_clone',
        label: 'Clone VM',
        description: 'Clone a Proxmox VM',
        icon: 'copy',
        category: 'action',
        color: '#10b981',
        defaultConfig: { clusterId: '', node: '', vmid: 0, newName: '', newId: 0 },
      },
      {
        type: 'action:http',
        label: 'HTTP Request',
        description: 'Make an HTTP request',
        icon: 'globe',
        category: 'action',
        color: '#10b981',
        defaultConfig: { url: '', method: 'GET', headers: {}, body: '' },
      },
      {
        type: 'action:shell_command',
        label: 'Shell Command',
        description: 'Run a local shell command in the workspace',
        icon: 'code',
        category: 'action',
        color: '#10b981',
        defaultConfig: { command: '', workingDirectory: '', timeoutSeconds: 60 },
      },
      {
        type: 'action:lua',
        label: 'Lua Script',
        description: 'Execute custom Lua code',
        icon: 'code',
        category: 'action',
        color: '#10b981',
        defaultConfig: { script: '-- Write your Lua code here\nreturn { status = "ok" }' },
      },
      {
        type: 'action:channel_send_message',
        label: 'Send Channel Message',
        description: 'Send a reply or outbound message through an active channel',
        icon: 'send',
        category: 'action',
        color: '#10b981',
        defaultConfig: { channelId: '', recipient: '', message: '' },
      },
      {
        type: 'action:channel_reply_message',
        label: 'Reply To Message',
        description: 'Send a new message as a reply to an existing channel message',
        icon: 'corner-down-left',
        category: 'action',
        color: '#10b981',
        defaultConfig: { channelId: '', recipient: '', replyToMessageId: '', message: '' },
      },
      {
        type: 'action:channel_edit_message',
        label: 'Edit Channel Message',
        description: 'Edit a previously sent channel message by message ID',
        icon: 'wrench',
        category: 'action',
        color: '#10b981',
        defaultConfig: { channelId: '', recipient: '', messageId: '', message: '' },
      },
      {
        type: 'action:channel_send_and_wait',
        label: 'Send And Wait',
        description: 'Send a channel message and wait for the user reply',
        icon: 'message-square',
        category: 'action',
        color: '#10b981',
        defaultConfig: { channelId: '', recipient: '', message: '', timeoutSeconds: 300 },
      },
      {
        type: 'action:pipeline_get',
        label: 'Get Pipeline',
        description: 'Load pipeline data for a selected pipeline',
        icon: 'workflow',
        category: 'action',
        color: '#10b981',
        defaultConfig: { pipelineId: '', includeDefinition: true },
      },
      {
        type: 'action:pipeline_run',
        label: 'Run Pipeline',
        description: 'Run another pipeline manually and pass JSON parameters',
        icon: 'workflow',
        category: 'action',
        color: '#10b981',
        defaultConfig: { pipelineId: '', params: '' },
      },
      ...KUBERNETES_ACTION_TYPES,
    ],
  },
  {
    id: 'tool',
    label: 'Tools',
    color: '#38bdf8',
    types: [
      {
        type: 'tool:proxmox_list_nodes',
        label: 'List Nodes Tool',
        description: 'Expose a tool that lists nodes in a selected Proxmox cluster',
        icon: 'globe',
        category: 'tool',
        color: '#38bdf8',
        defaultConfig: { clusterId: '' },
      },
      {
        type: 'tool:proxmox_list_workloads',
        label: 'List VMs/CTs Tool',
        description: 'Expose a tool that lists workloads in a selected Proxmox cluster',
        icon: 'link',
        category: 'tool',
        color: '#38bdf8',
        defaultConfig: { clusterId: '', node: '' },
      },
      {
        type: 'tool:vm_start',
        label: 'Start VM Tool',
        description: 'Expose a tool that starts a VM in a selected Proxmox cluster',
        icon: 'play',
        category: 'tool',
        color: '#38bdf8',
        defaultConfig: { clusterId: '', node: '', vmid: 0 },
      },
      {
        type: 'tool:vm_stop',
        label: 'Stop VM Tool',
        description: 'Expose a tool that stops a VM in a selected Proxmox cluster',
        icon: 'square',
        category: 'tool',
        color: '#38bdf8',
        defaultConfig: { clusterId: '', node: '', vmid: 0 },
      },
      {
        type: 'tool:vm_clone',
        label: 'Clone VM Tool',
        description: 'Expose a tool that clones a VM in a selected Proxmox cluster',
        icon: 'copy',
        category: 'tool',
        color: '#38bdf8',
        defaultConfig: { clusterId: '', node: '', vmid: 0, newName: '', newId: 0 },
      },
      {
        type: 'tool:http',
        label: 'HTTP Request Tool',
        description: 'Expose a tool that makes HTTP requests',
        icon: 'globe',
        category: 'tool',
        color: '#38bdf8',
        defaultConfig: { url: '', method: 'GET', headers: {}, body: '' },
      },
      {
        type: 'tool:shell_command',
        label: 'Shell Command Tool',
        description: 'Expose a tool that runs local shell commands in the workspace',
        icon: 'code',
        category: 'tool',
        color: '#38bdf8',
        defaultConfig: { command: '', workingDirectory: '', timeoutSeconds: 60 },
      },
      {
        type: 'tool:pipeline_list',
        label: 'List Pipelines Tool',
        description: 'Expose a tool that lists available pipelines to an agent',
        icon: 'list',
        category: 'tool',
        color: '#38bdf8',
        defaultConfig: {},
      },
      {
        type: 'tool:pipeline_get',
        label: 'Get Pipeline Tool',
        description: 'Expose a tool that returns data for a selected pipeline',
        icon: 'workflow',
        category: 'tool',
        color: '#38bdf8',
        defaultConfig: { pipelineId: '', includeDefinition: true, toolName: '', toolDescription: '', allowModelPipelineId: false },
      },
      {
        type: 'tool:pipeline_create',
        label: 'Create Pipeline Tool',
        description: 'Expose a tool that creates new pipelines',
        icon: 'workflow',
        category: 'tool',
        color: '#38bdf8',
        defaultConfig: { toolName: '', toolDescription: '' },
      },
      {
        type: 'tool:pipeline_update',
        label: 'Update Pipeline Tool',
        description: 'Expose a tool that updates existing pipelines',
        icon: 'wrench',
        category: 'tool',
        color: '#38bdf8',
        defaultConfig: { toolName: '', toolDescription: '' },
      },
      {
        type: 'tool:pipeline_delete',
        label: 'Delete Pipeline Tool',
        description: 'Expose a tool that deletes pipelines',
        icon: 'trash-2',
        category: 'tool',
        color: '#38bdf8',
        defaultConfig: { toolName: '', toolDescription: '' },
      },
      {
        type: 'tool:pipeline_run',
        label: 'Run Pipeline Tool',
        description: 'Expose a tool that runs a selected pipeline and returns its output',
        icon: 'wrench',
        category: 'tool',
        color: '#38bdf8',
        defaultConfig: { pipelineId: '', toolName: '', toolDescription: '', allowModelPipelineId: false, arguments: [] },
      },
      {
        type: 'tool:channel_send_and_wait',
        label: 'Send And Wait Tool',
        description: 'Expose a tool that messages a channel user and waits for their reply',
        icon: 'message-square',
        category: 'tool',
        color: '#38bdf8',
        defaultConfig: { channelId: '', recipient: '', message: '', timeoutSeconds: 300 },
      },
      ...KUBERNETES_TOOL_TYPES,
    ],
  },
  {
    id: 'logic',
    label: 'Logic',
    color: '#8b5cf6',
    types: [
      {
        type: 'logic:condition',
        label: 'Condition',
        description: 'Evaluate a condition (if/else)',
        icon: 'git-branch',
        category: 'logic',
        color: '#8b5cf6',
        defaultConfig: { expression: '' },
      },
      {
        type: 'logic:switch',
        label: 'Switch',
        description: 'Evaluate multiple conditions and fan out to every matching branch',
        icon: 'split',
        category: 'logic',
        color: '#8b5cf6',
        defaultConfig: {
          conditions: [
            {
              id: 'condition-1',
              label: 'Condition 1',
              expression: '',
            },
          ],
        },
      },
      {
        type: 'logic:merge',
        label: 'Merge',
        description: 'Merge multiple upstream object outputs into one payload',
        icon: 'workflow',
        category: 'logic',
        color: '#8b5cf6',
        defaultConfig: { mode: 'shallow' },
      },
      {
        type: 'logic:aggregate',
        label: 'Aggregate',
        description: 'Collect multiple upstream outputs into ordered arrays with source metadata and optional output id overrides',
        icon: 'list',
        category: 'logic',
        color: '#8b5cf6',
        defaultConfig: { idOverrides: {} },
      },
      {
        type: 'logic:return',
        label: 'Return',
        description: 'Return data from this pipeline to the caller and stop execution',
        icon: 'corner-down-left',
        category: 'logic',
        color: '#8b5cf6',
        defaultConfig: { value: '' },
      },
    ],
  },
  {
    id: 'llm',
    label: 'LLM',
    color: '#ec4899',
    types: [
      {
        type: 'llm:prompt',
        label: 'LLM Prompt',
        description: 'Send a prompt to an LLM provider',
        icon: 'brain',
        category: 'llm',
        color: '#ec4899',
        defaultConfig: { providerId: '', prompt: '', model: '', temperature: 0.7, max_tokens: 1024 },
      },
      {
        type: 'llm:agent',
        label: 'LLM Agent',
        description: 'Run a multi-turn LLM agent with connected tool nodes',
        icon: 'bot',
        category: 'llm',
        color: '#ec4899',
        defaultConfig: { providerId: '', prompt: '', model: '', temperature: 0.7, max_tokens: 1024, enableSkills: false },
      },
    ],
  },
]

const EXTRA_NODE_TYPES: NodeTypeDefinition[] = [
  {
    type: 'visual:group',
    label: 'Group',
    description: 'Visual canvas group that keeps related nodes together without affecting execution.',
    icon: 'square',
    category: 'visual',
    color: '#64748b',
    defaultConfig: { color: '#64748b' },
  },
]

export const NODE_TYPE_MAP: Record<NodeType, NodeTypeDefinition> = [...NODE_CATEGORIES.flatMap((category) => category.types), ...EXTRA_NODE_TYPES].reduce(
  (acc, typeDef) => {
    acc[typeDef.type] = withResolvedMenuPath(typeDef)
    return acc
  },
  {} as Record<NodeType, NodeTypeDefinition>,
)

function toNodeTypeDefinition(definition: NodeDefinition): NodeTypeDefinition {
  return withResolvedMenuPath({
    type: definition.type,
    source: definition.source,
    pluginId: definition.plugin_id,
    pluginName: definition.plugin_name,
    label: definition.label,
    description: definition.description,
    icon: definition.icon,
    category: definition.category,
    color: definition.color,
    menuPath: definition.menu_path,
    defaultConfig: definition.default_config || {},
    fields: definition.fields || [],
    outputs: definition.outputs || [],
    outputHints: definition.output_hints || [],
  })
}

export function buildNodeCatalog(definitions: NodeDefinition[] = []): { categories: NodeCategory[]; map: Record<string, NodeTypeDefinition> } {
  const mergedMap: Record<string, NodeTypeDefinition> = {
    ...NODE_TYPE_MAP,
  }

  definitions.forEach((definition) => {
    mergedMap[definition.type] = toNodeTypeDefinition(definition)
  })

  const grouped = new Map<string, NodeCategory>()

  Object.values(mergedMap).forEach((definition) => {
    if (definition.category === 'visual') {
      return
    }

    const existing = grouped.get(definition.category)
    if (existing) {
      existing.types.push(definition)
      return
    }

    const staticCategory = NODE_CATEGORIES.find((category) => category.id === definition.category)
    grouped.set(definition.category, {
      id: definition.category,
      label: staticCategory?.label || definition.category,
      color: staticCategory?.color || definition.color || '#6b7280',
      types: [definition],
    })
  })

  const categories = Array.from(grouped.values())
    .map((category) => ({
      ...category,
      types: [...category.types].sort((left, right) => left.label.localeCompare(right.label)),
    }))
    .sort((left, right) => {
      const leftIndex = NODE_CATEGORIES.findIndex((category) => category.id === left.id)
      const rightIndex = NODE_CATEGORIES.findIndex((category) => category.id === right.id)
      const normalizedLeft = leftIndex === -1 ? Number.MAX_SAFE_INTEGER : leftIndex
      const normalizedRight = rightIndex === -1 ? Number.MAX_SAFE_INTEGER : rightIndex
      if (normalizedLeft === normalizedRight) {
        return left.label.localeCompare(right.label)
      }
      return normalizedLeft - normalizedRight
    })

  return { categories, map: mergedMap }
}

function withResolvedMenuPath(definition: NodeTypeDefinition): NodeTypeDefinition {
  return {
    ...definition,
    menuPath: resolveMenuPath(definition),
  }
}

function resolveMenuPath(definition: Pick<NodeTypeDefinition, 'type' | 'category' | 'menuPath'>): string[] {
  if (definition.menuPath !== undefined) {
    return normalizeMenuPath(definition.menuPath)
  }

  if (definition.category !== 'action' && definition.category !== 'tool') {
    return []
  }

  if (isProxmoxNodeType(definition.type)) {
    return ['Proxmox']
  }
  if (isKubernetesNodeType(definition.type)) {
    return ['Kubernetes']
  }

  return ['General']
}

function normalizeMenuPath(menuPath?: string[]): string[] {
  if (!Array.isArray(menuPath)) {
    return []
  }

  return menuPath
    .map((segment) => segment.trim())
    .filter(Boolean)
}

function isProxmoxNodeType(type: string): boolean {
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

function isKubernetesNodeType(type: string): boolean {
  return type.includes(':kubernetes_')
}

function sortNodeTypes(types: NodeTypeDefinition[]): NodeTypeDefinition[] {
  return [...types].sort((left, right) => left.label.localeCompare(right.label))
}

function sortNodeMenuGroups(groups: NodeMenuGroup[]): NodeMenuGroup[] {
  const priority = (label: string): number => {
    switch (label) {
      case 'General':
        return 0
      case 'Proxmox':
        return 1
      case 'Kubernetes':
        return 2
      default:
        return 100
    }
  }

  return [...groups]
    .map((group) => ({
      ...group,
      groups: sortNodeMenuGroups(group.groups),
      types: sortNodeTypes(group.types),
    }))
    .sort((left, right) => {
      const leftPriority = priority(left.label)
      const rightPriority = priority(right.label)
      if (leftPriority !== rightPriority) {
        return leftPriority - rightPriority
      }
      return left.label.localeCompare(right.label)
    })
}

function countNodeMenuEntries(types: NodeTypeDefinition[], groups: NodeMenuGroup[]): number {
  return types.length + groups.reduce((total, group) => total + group.totalCount, 0)
}

export function buildNodeMenuCategories(categories: NodeCategory[]): NodeMenuCategory[] {
  return categories.map((category) => {
    const rootGroups: NodeMenuGroup[] = []
    const rootTypes: NodeTypeDefinition[] = []

    category.types.forEach((type) => {
      const menuPath = resolveMenuPath(type)
      if (menuPath.length === 0) {
        rootTypes.push(type)
        return
      }

      let currentGroups = rootGroups
      let currentGroup: NodeMenuGroup | null = null

      menuPath.forEach((segment, index) => {
        const nextPath = menuPath.slice(0, index + 1)
        const groupID = nextPath.join(' / ')
        const existing = currentGroups.find((group) => group.id === groupID)
        if (existing) {
          currentGroup = existing
        } else {
          currentGroup = {
            id: groupID,
            label: segment,
            path: nextPath,
            groups: [],
            types: [],
            totalCount: 0,
          }
          currentGroups.push(currentGroup)
        }

        if (index < menuPath.length - 1 && currentGroup) {
          currentGroups = currentGroup.groups
        }
      })

      currentGroup?.types.push(type)
    })

    const materializeGroups = (groups: NodeMenuGroup[]): NodeMenuGroup[] => {
      return sortNodeMenuGroups(groups.map((group) => {
        const nestedGroups = materializeGroups(group.groups)
        const directTypes = sortNodeTypes(group.types)
        return {
          ...group,
          groups: nestedGroups,
          types: directTypes,
          totalCount: countNodeMenuEntries(directTypes, nestedGroups),
        }
      }))
    }

    const groups = materializeGroups(rootGroups)
    const types = sortNodeTypes(rootTypes)

    return {
      id: category.id,
      label: category.label,
      color: category.color,
      groups,
      types,
      totalCount: countNodeMenuEntries(types, groups),
    }
  }).filter((category) => category.totalCount > 0)
}

export function getNodeColor(type?: string): string {
  if (typeof type !== 'string' || !type) {
    return '#6b7280'
  }

  return NODE_TYPE_MAP[type as NodeType]?.color || '#6b7280'
}

export function getNodeLabel(type?: string): string {
  if (typeof type !== 'string' || !type) {
    return 'Unknown node type'
  }

  return NODE_TYPE_MAP[type as NodeType]?.label || type
}

export function getNodeIcon(type?: string): string {
  if (typeof type !== 'string' || !type) {
    return 'circle'
  }

  return NODE_TYPE_MAP[type as NodeType]?.icon || 'circle'
}
