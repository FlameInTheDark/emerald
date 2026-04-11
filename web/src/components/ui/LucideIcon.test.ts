import { describe, expect, it } from 'vitest'

import { resolveLucideIconName } from './LucideIcon'

describe('resolveLucideIconName', () => {
  it('accepts standard lucide kebab-case names', () => {
    expect(resolveLucideIconName('globe')).toBe('globe')
  })

  it('normalizes common plugin icon name variants', () => {
    expect(resolveLucideIconName('Globe')).toBe('globe')
    expect(resolveLucideIconName('message_square')).toBe('message-square')
    expect(resolveLucideIconName('RefreshCw')).toBe('refresh-cw')
  })

  it('falls back safely when the icon is unknown', () => {
    expect(resolveLucideIconName('definitely-not-a-real-icon', 'zap')).toBe('zap')
  })
})
