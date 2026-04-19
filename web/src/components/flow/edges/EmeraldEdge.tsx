import { memo } from 'react'
import { BaseEdge, getBezierPath, type EdgeProps } from '@xyflow/react'

type EmeraldEdgeData = {
  useGradient?: boolean
  gradientStartColor?: string
  gradientEndColor?: string
}

function sanitizeId(value: string): string {
  return value.replace(/[^a-zA-Z0-9_-]/g, '-')
}

function EmeraldEdge({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  markerEnd,
  markerStart,
  style,
  data,
  interactionWidth,
}: EdgeProps) {
  const [edgePath] = getBezierPath({
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
  })

  const edgeData = (data as EmeraldEdgeData | undefined) ?? {}
  const useGradient = Boolean(
    edgeData.useGradient
      && edgeData.gradientStartColor
      && edgeData.gradientEndColor,
  )
  const gradientId = `emerald-edge-gradient-${sanitizeId(id)}`

  return (
    <>
      {useGradient && (
        <defs>
          <linearGradient
            id={gradientId}
            gradientUnits="userSpaceOnUse"
            x1={sourceX}
            y1={sourceY}
            x2={targetX}
            y2={targetY}
          >
            <stop offset="0%" stopColor={edgeData.gradientStartColor} />
            <stop offset="100%" stopColor={edgeData.gradientEndColor} />
          </linearGradient>
        </defs>
      )}
      <BaseEdge
        path={edgePath}
        markerEnd={markerEnd}
        markerStart={markerStart}
        interactionWidth={interactionWidth}
        style={{
          ...style,
          stroke: useGradient ? `url(#${gradientId})` : style?.stroke,
        }}
      />
    </>
  )
}

export default memo(EmeraldEdge)
