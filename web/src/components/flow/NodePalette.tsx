import { useMemo, useState, type ReactNode } from 'react'
import { cn } from '../../lib/utils'
import Input from '../ui/Input'
import { useNodeDefinitions } from '../../hooks/useNodeDefinitions'
import { ChevronDown, ChevronRight, Search } from 'lucide-react'
import LucideIcon from '../ui/LucideIcon'
import { buildNodeMenuCategories, type NodeMenuGroup } from './nodeTypes'
import type { NodeTypeDefinition } from '../../types'

interface NodePaletteProps {
  onDragStart: (event: React.DragEvent, nodeType: string, label: string, config: Record<string, unknown>) => void
  className?: string
}

export default function NodePalette({ onDragStart, className }: NodePaletteProps) {
  const { categories } = useNodeDefinitions()
  const [expandedCategories, setExpandedCategories] = useState<Record<string, boolean>>({
    trigger: true,
    action: true,
  })
  const [expandedGroups, setExpandedGroups] = useState<Record<string, boolean>>({})
  const [searchQuery, setSearchQuery] = useState('')

  const normalizedQuery = searchQuery.trim().toLowerCase()
  const filteredCategories = useMemo(
    () => categories
      .map((category) => ({
        ...category,
        types: category.types.filter((nodeType) => (
          `${category.label} ${nodeType.label} ${nodeType.description} ${nodeType.type} ${(nodeType.menuPath || []).join(' ')}`
            .toLowerCase()
            .includes(normalizedQuery)
        )),
      }))
      .filter((category) => category.types.length > 0),
    [categories, normalizedQuery],
  )
  const menuCategories = useMemo(
    () => buildNodeMenuCategories(filteredCategories),
    [filteredCategories],
  )

  const toggleCategory = (id: string) => {
    setExpandedCategories((prev) => ({ ...prev, [id]: !prev[id] }))
  }

  const toggleGroup = (id: string) => {
    setExpandedGroups((prev) => ({ ...prev, [id]: !(prev[id] ?? true) }))
  }

  const renderNodeItem = (nodeType: NodeTypeDefinition, depth = 0) => (
    <div
      key={nodeType.type}
      draggable
      onDragStart={(e) => onDragStart(e, nodeType.type, nodeType.label, nodeType.defaultConfig)}
      className={cn(
        'flex items-center gap-3 px-3 py-2.5 rounded-lg cursor-grab active:cursor-grabbing',
        'hover:bg-bg-overlay transition-colors group'
      )}
      style={{ marginLeft: depth > 0 ? `${depth * 12}px` : undefined }}
    >
      <div
        className="w-7 h-7 rounded-md flex items-center justify-center flex-shrink-0"
        style={{ backgroundColor: nodeType.color + '20' }}
      >
        <LucideIcon
          name={nodeType.icon}
          fallbackName="zap"
          className="w-3.5 h-3.5"
          style={{ color: nodeType.color }}
        />
      </div>
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium text-text truncate">{nodeType.label}</p>
        <p className="text-xs text-text-dimmed truncate">{nodeType.description}</p>
      </div>
      <div
        className="w-1.5 h-1.5 rounded-full opacity-0 group-hover:opacity-100 transition-opacity"
        style={{ backgroundColor: nodeType.color }}
      />
    </div>
  )

  const renderGroup = (group: NodeMenuGroup, depth = 0): ReactNode => {
    const isExpanded = normalizedQuery ? true : (expandedGroups[group.id] ?? true)

    return (
      <div key={group.id} className="space-y-1">
        <button
          onClick={() => toggleGroup(group.id)}
          className="flex items-center gap-2 w-full px-3 py-2 text-xs font-semibold text-text-muted uppercase tracking-wider hover:text-text transition-colors"
          style={{ paddingLeft: `${12 + (depth * 14)}px` }}
        >
          {isExpanded ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
          <span className="truncate">{group.label}</span>
          <span className="ml-auto text-text-dimmed">{group.totalCount}</span>
        </button>

        {isExpanded && (
          <div className="space-y-1">
            {group.groups.map((child) => renderGroup(child, depth + 1))}
            {group.types.map((nodeType) => renderNodeItem(nodeType, depth + 1))}
          </div>
        )}
      </div>
    )
  }

  return (
    <div className={cn('flex w-72 max-w-[calc(100vw-2rem)] min-h-0 flex-col overflow-hidden rounded-xl border border-border bg-bg-elevated shadow-xl', className)}>
      <div className="border-b border-border px-4 py-3">
        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-dimmed" />
          <Input
            value={searchQuery}
            onChange={(event) => setSearchQuery(event.target.value)}
            placeholder="Search nodes..."
            className="pl-9"
          />
        </div>
      </div>

      <div className="flex-1 overflow-y-auto py-2">
        {menuCategories.length === 0 ? (
          <div className="px-4 py-6 text-sm text-text-dimmed">No nodes match your search.</div>
        ) : menuCategories.map((category) => {
          const isExpanded = normalizedQuery ? true : expandedCategories[category.id]
          return (
            <div key={category.id} className="mb-1">
              <button
                onClick={() => toggleCategory(category.id)}
                className="flex items-center gap-2 w-full px-4 py-2 text-xs font-semibold text-text-muted uppercase tracking-wider hover:text-text transition-colors"
              >
                {isExpanded ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
                <span>{category.label}</span>
                <span className="ml-auto text-text-dimmed">{category.totalCount}</span>
              </button>

              {isExpanded && (
                <div className="px-2 space-y-1">
                  {category.groups.map((group) => renderGroup(group))}
                  {category.types.map((nodeType) => renderNodeItem(nodeType))}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
