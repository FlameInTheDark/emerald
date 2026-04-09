import { useMemo, useState } from 'react'
import { NODE_CATEGORIES } from './nodeTypes'
import { cn } from '../../lib/utils'
import Input from '../ui/Input'
import { 
  Zap, Clock, Webhook, Play, Square, Copy, Globe, Code,
  GitBranch, Split, Brain, ChevronDown, ChevronRight, Link, MessageSquare, Send, Trash2,
  Bot, Workflow, List, Wrench, CornerDownLeft, RefreshCw, Search,
} from 'lucide-react'

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

interface NodePaletteProps {
  onDragStart: (event: React.DragEvent, nodeType: string, label: string, config: Record<string, unknown>) => void
  className?: string
}

export default function NodePalette({ onDragStart, className }: NodePaletteProps) {
  const [expandedCategories, setExpandedCategories] = useState<Record<string, boolean>>({
    trigger: true,
    action: true,
  })
  const [searchQuery, setSearchQuery] = useState('')

  const normalizedQuery = searchQuery.trim().toLowerCase()
  const filteredCategories = useMemo(
    () => NODE_CATEGORIES
      .map((category) => ({
        ...category,
        types: category.types.filter((nodeType) => (
          `${category.label} ${nodeType.label} ${nodeType.description} ${nodeType.type}`
            .toLowerCase()
            .includes(normalizedQuery)
        )),
      }))
      .filter((category) => category.types.length > 0),
    [normalizedQuery],
  )

  const toggleCategory = (id: string) => {
    setExpandedCategories((prev) => ({ ...prev, [id]: !prev[id] }))
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
        {filteredCategories.length === 0 ? (
          <div className="px-4 py-6 text-sm text-text-dimmed">No nodes match your search.</div>
        ) : filteredCategories.map((category) => {
          const isExpanded = normalizedQuery ? true : expandedCategories[category.id]
          return (
            <div key={category.id} className="mb-1">
              <button
                onClick={() => toggleCategory(category.id)}
                className="flex items-center gap-2 w-full px-4 py-2 text-xs font-semibold text-text-muted uppercase tracking-wider hover:text-text transition-colors"
              >
                {isExpanded ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
                <span>{category.label}</span>
                <span className="ml-auto text-text-dimmed">{category.types.length}</span>
              </button>

              {isExpanded && (
                <div className="px-2 space-y-1">
                  {category.types.map((nodeType) => {
                    const Icon = iconMap[nodeType.icon] || Zap
                    return (
                      <div
                        key={nodeType.type}
                        draggable
                        onDragStart={(e) => onDragStart(e, nodeType.type, nodeType.label, nodeType.defaultConfig)}
                        className={cn(
                          'flex items-center gap-3 px-3 py-2.5 rounded-lg cursor-grab active:cursor-grabbing',
                          'hover:bg-bg-overlay transition-colors group'
                        )}
                      >
                        <div
                          className="w-7 h-7 rounded-md flex items-center justify-center flex-shrink-0"
                          style={{ backgroundColor: nodeType.color + '20' }}
                        >
                          <Icon className="w-3.5 h-3.5" style={{ color: nodeType.color }} />
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
                  })}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
