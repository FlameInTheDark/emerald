import type { ComponentProps } from 'react'
import { Circle } from 'lucide-react'
import { DynamicIcon, dynamicIconImports, type IconName } from 'lucide-react/dynamic'

const availableIconNames = new Set<string>(Object.keys(dynamicIconImports))

function normalizeIconToken(value: string): string {
  return value
    .trim()
    .replace(/([A-Z]+)([A-Z][a-z])/g, '$1-$2')
    .replace(/([a-z0-9])([A-Z])/g, '$1-$2')
    .replace(/([a-zA-Z])(\d)/g, '$1-$2')
    .replace(/(\d)([a-zA-Z])/g, '$1-$2')
    .replace(/[\s_]+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '')
    .toLowerCase()
}

function iconCandidates(value?: string | null): string[] {
  const trimmed = value?.trim()
  if (!trimmed) {
    return []
  }

  const normalized = normalizeIconToken(trimmed)
  const candidates = [
    trimmed,
    trimmed.toLowerCase(),
    normalized,
  ]

  if (normalized.endsWith('-icon')) {
    candidates.push(normalized.slice(0, -5))
  }

  return candidates.filter((candidate, index, list) => candidate && list.indexOf(candidate) === index)
}

export function resolveLucideIconName(name?: string | null, fallbackName: IconName = 'circle'): IconName {
  for (const candidate of iconCandidates(name)) {
    if (availableIconNames.has(candidate)) {
      return candidate as IconName
    }
  }

  return fallbackName
}

type LucideIconProps = Omit<ComponentProps<typeof DynamicIcon>, 'name' | 'fallback'> & {
  name?: string | null
  fallbackName?: IconName
}

export default function LucideIcon({
  name,
  fallbackName = 'circle',
  ...props
}: LucideIconProps) {
  const resolvedName = resolveLucideIconName(name, fallbackName)

  return (
    <DynamicIcon
      {...props}
      name={resolvedName}
      fallback={() => <Circle {...props} />}
    />
  )
}
