import { memo } from 'react'
import { Handle, NodeResizeControl, Position, ResizeControlVariant } from '@xyflow/react'
import { getNodeColor, getNodeLabel, getNodeIcon } from '../nodeTypes'
import { getNodeBorderTint, getNodeStatusColor, resolveGroupColor, withAlpha } from '../nodeAppearance'
import type { NodeDefinitionOutputHandle, NodeExecutionLogData, NodeType } from '../../../types'
import { FileText } from 'lucide-react'
import { cn } from '../../../lib/utils'
import LucideIcon from '../../ui/LucideIcon'

interface AutomatorNodeData {
  label?: string
  type: NodeType
  config?: Record<string, unknown>
  color?: string
  icon?: string
  outputHandles?: NodeDefinitionOutputHandle[]
  isUnavailable?: boolean
  status?: 'pending' | 'running' | 'success' | 'error'
  enabled?: boolean
  isHighlight?: boolean
  executionLog?: NodeExecutionLogData
  canViewLog?: boolean
  onViewLog?: () => void
  onResizeStart?: () => void
  onResizeEnd?: () => void
}

type LogicOutlet = {
  id: string
  label: string
  color: string
}

const HANDLE_CLASS = '!h-3 !w-3 !bg-bg-overlay !border-2'
const LOGIC_HANDLE_OUTSET = -16

function getSwitchOutlets(config?: Record<string, unknown>): LogicOutlet[] {
  const conditions = Array.isArray(config?.conditions) ? config.conditions : []
  const outlets = conditions.map((condition, index) => {
    const value = typeof condition === 'object' && condition !== null
      ? condition as Record<string, unknown>
      : {}

    const id = typeof value.id === 'string' && value.id.trim()
      ? value.id.trim()
      : `condition-${index + 1}`
    const label = typeof value.label === 'string' && value.label.trim()
      ? value.label.trim()
      : `Condition ${index + 1}`

    return {
      id,
      label,
      color: '#a78bfa',
    }
  })

  outlets.push({
    id: 'default',
    label: 'Else',
    color: '#94a3b8',
  })

  return outlets
}

function AutomatorNode({ data, selected }: { data: AutomatorNodeData; selected: boolean }) {
  const nodeType = data.type
  const color = data.color || getNodeColor(nodeType)
  const label = data.label || getNodeLabel(nodeType)
  const iconName = data.icon || getNodeIcon(nodeType)
  const isEnabled = data.enabled !== false
  const isHighlight = data.isHighlight === true

  const isCondition = nodeType === 'logic:condition'
  const isSwitch = nodeType === 'logic:switch'
  const isTrigger = nodeType.startsWith('trigger:')
  const isReturn = nodeType === 'logic:return'
  const isAgent = nodeType === 'llm:agent'
  const isTool = nodeType.startsWith('tool:')
  const isGroup = nodeType === 'visual:group'
  const groupColor = resolveGroupColor(data.config)
  const isLogic = isCondition || isSwitch
  const hasCustomOutputs = !isTool && !isReturn && !isLogic && Array.isArray(data.outputHandles) && data.outputHandles.length > 0
  const logicOutlets: LogicOutlet[] = isCondition
    ? [
        { id: 'true', label: 'True', color: '#22c55e' },
        { id: 'false', label: 'False', color: '#ef4444' },
      ]
    : isSwitch
    ? getSwitchOutlets(data.config)
    : []

  const statusColor = getNodeStatusColor(data.status)
  const highlightColor = statusColor || '#f59e0b'
  const borderTint = getNodeBorderTint({
    nodeType,
    selected,
    isHighlight,
    status: data.status,
    config: data.config,
  })
  const shouldGlow = selected || isHighlight || statusColor !== null
  const boxShadow = shouldGlow
    ? `0 0 0 1px ${borderTint}55, 0 0 18px ${borderTint}33, 0 16px 36px ${borderTint}22`
    : '0 12px 28px rgba(2, 6, 23, 0.28)'

  if (isGroup) {
    return (
      <div
        className="relative h-full w-full overflow-visible rounded-[22px] border-2 border-dashed transition-all"
        style={{
          borderColor: borderTint,
          backgroundColor: withAlpha(groupColor, '18'),
          boxShadow,
        }}
      >
        <div
          className="absolute left-3 top-3 inline-flex max-w-[calc(100%-24px)] items-center gap-2 rounded-full border px-3 py-1.5 shadow-lg"
          style={{
            borderColor: withAlpha(groupColor, '88'),
            backgroundColor: withAlpha(groupColor, '24'),
          }}
        >
          <LucideIcon name={iconName} fallbackName="circle" className="h-3.5 w-3.5 flex-shrink-0" style={{ color: groupColor }} />
          <span className="truncate text-xs font-semibold uppercase tracking-[0.08em] text-text">
            {label}
          </span>
        </div>

        {selected && (
          <NodeResizeControl
            position="bottom-right"
            variant={ResizeControlVariant.Handle}
            minWidth={280}
            minHeight={180}
            color={groupColor}
            className="nodrag nopan"
            onResizeStart={() => data.onResizeStart?.()}
            onResizeEnd={() => data.onResizeEnd?.()}
          >
            <div
              className="h-3.5 w-3.5 rounded-sm border-2 bg-bg-elevated shadow-lg"
              style={{ borderColor: groupColor }}
            />
          </NodeResizeControl>
        )}
      </div>
    )
  }

  return (
    <div className={cn(
      'relative min-w-[180px] overflow-visible rounded-xl border-2 bg-bg-elevated shadow-lg transition-all',
      !isEnabled && 'opacity-30 grayscale',
      isHighlight && 'highlight-node',
    )} style={{
      borderColor: borderTint,
      borderStyle: 'solid',
      boxShadow,
    }}>
      {!isTrigger && !isTool && (
        <Handle
          type="target"
          position={Position.Left}
          className={HANDLE_CLASS}
          style={{ borderColor: borderTint }}
        />
      )}

      {isTool && (
        <Handle
          type="target"
          position={Position.Top}
          className={HANDLE_CLASS}
          style={{ borderColor: borderTint }}
        />
      )}

      {isHighlight && data.canViewLog && data.onViewLog && (
        <button
          type="button"
          onClick={(event) => {
            event.stopPropagation()
            data.onViewLog?.()
          }}
          className="absolute top-0 right-3 z-10 inline-flex -translate-y-1/2 items-center gap-1 rounded-full border border-border bg-bg-overlay px-2 py-1 text-[11px] font-medium text-text shadow-lg transition-colors hover:border-accent/50 hover:text-accent nodrag nopan"
        >
          <FileText className="w-3 h-3" />
          Log
        </button>
      )}

      <div className="flex items-center gap-3 px-4 py-3">
        <div className={cn(
          'flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg',
          isHighlight && 'animate-pulse',
        )} style={{ backgroundColor: isHighlight ? `${highlightColor}20` : `${color}20` }}>
          <LucideIcon
            name={iconName}
            fallbackName="circle"
            className="w-4 h-4"
            style={{ color: isHighlight ? highlightColor : color }}
          />
        </div>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium text-text">{label}</p>
          <p className="truncate text-xs text-text-dimmed">{nodeType}</p>
        </div>
        {data.status && (
          <div className={cn(
            'h-2.5 w-2.5 flex-shrink-0 rounded-full',
            {
              'bg-text-dimmed': data.status === 'pending',
              'bg-amber-400 animate-pulse': data.status === 'running',
              'bg-green-400': data.status === 'success',
              'bg-red-400': data.status === 'error',
            },
          )} />
        )}
      </div>

      {data.isUnavailable && (
        <div className="px-4 -mt-1 pb-2">
          <div className="rounded-md border border-amber-500/30 bg-amber-500/10 px-2 py-1 text-[10px] font-semibold uppercase tracking-[0.08em] text-amber-200">
            Plugin unavailable
          </div>
        </div>
      )}

      {isTool && (
        <div className="px-4 -mt-1 pb-2">
          <div className="rounded-md border border-border/70 bg-bg-overlay/60 px-2 py-1 text-[10px] font-semibold uppercase tracking-[0.08em] text-sky-300">
            Tool Node
          </div>
        </div>
      )}

      {isLogic ? (
        <div className="px-4 pb-3 pt-1">
          <div className="space-y-1.5">
            {logicOutlets.map((outlet) => (
              <div
                key={outlet.id}
                className="relative rounded-lg border border-border/80 bg-bg-overlay/60 px-2.5 py-1.5 pr-7"
              >
                <span
                  className="block truncate text-[10px] font-semibold uppercase tracking-[0.08em]"
                  style={{ color: outlet.color }}
                >
                  {outlet.label}
                </span>
                <Handle
                  id={outlet.id}
                  type="source"
                  position={Position.Right}
                  className={HANDLE_CLASS}
                  style={{
                    right: LOGIC_HANDLE_OUTSET,
                    borderColor: outlet.color,
                  }}
                />
              </div>
            ))}
          </div>
        </div>
      ) : hasCustomOutputs ? (
        <div className="px-4 pb-3 pt-1">
          <div className="space-y-1.5">
            {data.outputHandles?.map((outlet) => (
              <div
                key={outlet.id}
                className="relative rounded-lg border border-border/80 bg-bg-overlay/60 px-2.5 py-1.5 pr-7"
              >
                <span
                  className="block truncate text-[10px] font-semibold uppercase tracking-[0.08em]"
                  style={{ color: outlet.color || color }}
                >
                  {outlet.label || outlet.id}
                </span>
                <Handle
                  id={outlet.id}
                  type="source"
                  position={Position.Right}
                  className={HANDLE_CLASS}
                  style={{
                    right: LOGIC_HANDLE_OUTSET,
                    borderColor: outlet.color || color,
                  }}
                />
              </div>
            ))}
          </div>
        </div>
      ) : (
        <>
          {!isTool && !isReturn && (
            <Handle
              type="source"
              position={Position.Right}
              className={HANDLE_CLASS}
              style={{ borderColor: borderTint }}
            />
          )}
          {isAgent && (
            <>
              <div className="px-4 pb-3 -mt-1">
                <div className="rounded-md border border-border/70 bg-bg-overlay/60 px-2 py-1 text-[10px] font-semibold uppercase tracking-[0.08em] text-sky-300 text-center">
                  Tools
                </div>
              </div>
              <Handle
                id="tool"
                type="source"
                position={Position.Bottom}
                className={HANDLE_CLASS}
                style={{ borderColor: borderTint }}
              />
            </>
          )}
        </>
      )}
    </div>
  )
}

export default memo(AutomatorNode)
