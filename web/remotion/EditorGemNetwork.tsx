import * as React from 'react'
import type { ComponentType, SVGProps } from 'react'
import { Bot, Code, Gem, Globe, MessageSquare, Shield, Workflow } from 'lucide-react'

const COLORS = {
  canvas: '#0a0e14',
  panel: '#131b24',
  panelOverlay: '#1a2332',
  border: '#1e2d3d',
  text: '#e6edf3',
  textMuted: '#8b949e',
  accent: '#10b981',
  accentHover: '#34d399',
}

const NODE_WIDTH = 360
const NODE_HEIGHT = 144
const HUB_RADIUS = 98
const GRID_GAP = 20
const NODE_ICON_BOX_SIZE = 56
const NODE_ICON_SIZE = 32

export const EDITOR_GEM_ILLUSTRATION_WIDTH = 1600
export const EDITOR_GEM_ILLUSTRATION_HEIGHT = 900

export type EditorGemNetworkProps = {
  transparent?: boolean
}

type LucideIcon = ComponentType<SVGProps<SVGSVGElement>>

type IllustrationNode = {
  eyebrow: string
  titleLines: string[]
  bodyLines: string[]
  color: string
  icon: LucideIcon
  side: 'left' | 'right'
  centerY: number
}

const NODES: IllustrationNode[] = [
  {
    eyebrow: 'Integrations',
    titleLines: ['Connect APIs'],
    bodyLines: ['HTTP, webhooks, and services'],
    color: '#38bdf8',
    icon: Globe,
    side: 'left',
    centerY: 230,
  },
  {
    eyebrow: 'Automation',
    titleLines: ['Run commands'],
    bodyLines: ['Scripts and background jobs'],
    color: '#38bdf8',
    icon: Code,
    side: 'left',
    centerY: 450,
  },
  {
    eyebrow: 'Reuse',
    titleLines: ['Reuse flows'],
    bodyLines: ['Templates and schedules'],
    color: '#10b981',
    icon: Workflow,
    side: 'left',
    centerY: 670,
  },
  {
    eyebrow: 'Agents',
    titleLines: ['AI with tools'],
    bodyLines: ['Reasoning with connected tools'],
    color: '#ec4899',
    icon: Bot,
    side: 'right',
    centerY: 230,
  },
  {
    eyebrow: 'Human Loop',
    titleLines: ['Human loop'],
    bodyLines: ['Ask, approve, and resume'],
    color: '#10b981',
    icon: MessageSquare,
    side: 'right',
    centerY: 450,
  },
  {
    eyebrow: 'Operations',
    titleLines: ['Ready for ops'],
    bodyLines: ['Logs, retries, visibility'],
    color: '#8b5cf6',
    icon: Shield,
    side: 'right',
    centerY: 670,
  },
]

export const editorGemNetworkDefaults: Required<EditorGemNetworkProps> = {
  transparent: false,
}

export function EditorGemNetwork({
  transparent = editorGemNetworkDefaults.transparent,
}: EditorGemNetworkProps) {
  const centerX = EDITOR_GEM_ILLUSTRATION_WIDTH / 2
  const centerY = EDITOR_GEM_ILLUSTRATION_HEIGHT / 2
  const nodeLayouts = NODES.map((node) => {
    const nodeX = node.side === 'left' ? 110 : EDITOR_GEM_ILLUSTRATION_WIDTH - 110 - NODE_WIDTH
    const nodeY = node.centerY - (NODE_HEIGHT / 2)
    const start = {
      x: node.side === 'left' ? nodeX + NODE_WIDTH : nodeX,
      y: node.centerY,
    }
    const end = getHubAnchor(start, centerX, centerY, HUB_RADIUS)
    const path = buildConnectionPath(start, end, node.side)

    return {
      node,
      nodeX,
      nodeY,
      start,
      end,
      path,
    }
  })

  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width={EDITOR_GEM_ILLUSTRATION_WIDTH}
      height={EDITOR_GEM_ILLUSTRATION_HEIGHT}
      viewBox={`0 0 ${EDITOR_GEM_ILLUSTRATION_WIDTH} ${EDITOR_GEM_ILLUSTRATION_HEIGHT}`}
      fill="none"
    >
      <defs>
        <filter id="node-shadow" x="-25%" y="-30%" width="150%" height="170%">
          <feDropShadow dx="0" dy="18" stdDeviation="18" floodColor="#020617" floodOpacity="0.30" />
        </filter>
        <filter id="hub-shadow" x="-40%" y="-40%" width="180%" height="180%">
          <feDropShadow dx="0" dy="18" stdDeviation="26" floodColor="#020617" floodOpacity="0.32" />
        </filter>
        <radialGradient id="hub-halo" cx="50%" cy="50%" r="50%">
          <stop offset="0%" stopColor="#10b981" stopOpacity="0.26" />
          <stop offset="68%" stopColor="#10b981" stopOpacity="0.10" />
          <stop offset="100%" stopColor="#10b981" stopOpacity="0" />
        </radialGradient>
        <pattern id="canvas-dots" x="0" y="0" width={GRID_GAP} height={GRID_GAP} patternUnits="userSpaceOnUse">
          <circle cx="1" cy="1" r="1" fill={COLORS.border} opacity="0.9" />
        </pattern>
      </defs>

      {!transparent && (
        <>
          <rect
            x="0"
            y="0"
            width={EDITOR_GEM_ILLUSTRATION_WIDTH}
            height={EDITOR_GEM_ILLUSTRATION_HEIGHT}
            fill={COLORS.canvas}
          />
          <DotBackground />
        </>
      )}

      <circle cx={centerX} cy={centerY} r="150" fill="url(#hub-halo)" />

      {nodeLayouts.map(({ node, nodeX, nodeY, start, path }) => (
        <g key={`${node.side}-${node.eyebrow}`}>
          <path
            d={path}
            stroke={COLORS.border}
            strokeWidth="4"
            strokeLinecap="round"
            opacity="0.95"
          />
          <path
            d={path}
            stroke={COLORS.accentHover}
            strokeWidth="1.5"
            strokeLinecap="round"
            opacity="0.18"
          />
          <NodeCard node={node} x={nodeX} y={nodeY} />
          <HandleDot cx={start.x} cy={start.y} />
        </g>
      ))}

      <g filter="url(#hub-shadow)">
        <circle cx={centerX} cy={centerY} r="126" fill="#10b981" opacity="0.08" />
        <circle cx={centerX} cy={centerY} r="98" fill={COLORS.panel} stroke={COLORS.border} strokeWidth="2.5" />
        <circle cx={centerX} cy={centerY} r="80" fill={COLORS.panelOverlay} stroke={COLORS.accent} strokeOpacity="0.30" />
        <Gem
          x={centerX - 58}
          y={centerY - 58}
          width="116"
          height="116"
          color={COLORS.accent}
          strokeWidth="1.45"
        />
      </g>

      {nodeLayouts.map(({ node, end }) => (
        <g key={`hub-pin-${node.side}-${node.eyebrow}`}>
          <HandleDot cx={end.x} cy={end.y} />
        </g>
      ))}
    </svg>
  )
}

function DotBackground() {
  return (
    <rect
      x="0"
      y="0"
      width={EDITOR_GEM_ILLUSTRATION_WIDTH}
      height={EDITOR_GEM_ILLUSTRATION_HEIGHT}
      fill="url(#canvas-dots)"
    />
  )
}

function NodeCard({
  node,
  x,
  y,
}: {
  node: IllustrationNode
  x: number
  y: number
}) {
  const iconBoxX = x + 20
  const iconBoxY = y + 20
  const iconX = iconBoxX + ((NODE_ICON_BOX_SIZE - NODE_ICON_SIZE) / 2)
  const iconY = iconBoxY + ((NODE_ICON_BOX_SIZE - NODE_ICON_SIZE) / 2)
  const eyebrowX = x + 104
  const titleX = x + 104
  const bodyX = x + 26
  const titleStartY = y + 60
  const titleLineHeight = 24
  const bodyStartY = y + 106
  const bodyLineHeight = 20

  return (
    <g filter="url(#node-shadow)">
      <rect
        x={x}
        y={y}
        width={NODE_WIDTH}
        height={NODE_HEIGHT}
        rx="22"
        fill={COLORS.panel}
        stroke={COLORS.border}
        strokeWidth="2"
      />
      <rect
        x={iconBoxX}
        y={iconBoxY}
        width={NODE_ICON_BOX_SIZE}
        height={NODE_ICON_BOX_SIZE}
        rx="14"
        fill={withAlpha(node.color, 0.16)}
      />
      <node.icon
        x={iconX}
        y={iconY}
        width={NODE_ICON_SIZE}
        height={NODE_ICON_SIZE}
        color={node.color}
        strokeWidth="2.1"
      />
      <text
        x={titleX}
        y={titleStartY}
        fill={COLORS.text}
        fontFamily="Inter, system-ui, sans-serif"
        fontSize="24"
        fontWeight="600"
      >
        {node.titleLines.map((line, index) => (
          <tspan key={line} x={titleX} dy={index === 0 ? 0 : titleLineHeight}>
            {line}
          </tspan>
        ))}
      </text>
      <text
        x={bodyX}
        y={bodyStartY}
        fill={COLORS.textMuted}
        fontFamily="Inter, system-ui, sans-serif"
        fontSize="18"
        fontWeight="500"
      >
        {node.bodyLines.map((line, index) => (
          <tspan key={line} x={bodyX} dy={index === 0 ? 0 : bodyLineHeight}>
            {line}
          </tspan>
        ))}
      </text>
    </g>
  )
}

function HandleDot({
  cx,
  cy,
}: {
  cx: number
  cy: number
}) {
  return (
    <g>
      <circle cx={cx} cy={cy} r="7" fill={COLORS.panelOverlay} stroke={COLORS.accentHover} strokeWidth="2.5" />
      <circle cx={cx} cy={cy} r="2.5" fill={COLORS.accentHover} />
    </g>
  )
}

function getHubAnchor(
  point: { x: number; y: number },
  centerX: number,
  centerY: number,
  radius: number,
) {
  const deltaX = point.x - centerX
  const deltaY = point.y - centerY
  const length = Math.hypot(deltaX, deltaY) || 1

  return {
    x: centerX + ((deltaX / length) * radius),
    y: centerY + ((deltaY / length) * radius),
  }
}

function buildConnectionPath(
  start: { x: number; y: number },
  end: { x: number; y: number },
  side: 'left' | 'right',
) {
  const direction = side === 'left' ? 1 : -1
  const controlOffset = 110

  return [
    `M ${start.x} ${start.y}`,
    `C ${start.x + (direction * controlOffset)} ${start.y}`,
    `${end.x - (direction * (controlOffset * 0.55))} ${end.y}`,
    `${end.x} ${end.y}`,
  ].join(' ')
}

function withAlpha(hexColor: string, alpha: number) {
  const normalized = hexColor.replace('#', '')
  const value = normalized.length === 3
    ? normalized.split('').map((char) => `${char}${char}`).join('')
    : normalized

  const red = Number.parseInt(value.slice(0, 2), 16)
  const green = Number.parseInt(value.slice(2, 4), 16)
  const blue = Number.parseInt(value.slice(4, 6), 16)

  return `rgba(${red}, ${green}, ${blue}, ${alpha})`
}
