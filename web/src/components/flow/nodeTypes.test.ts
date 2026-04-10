import { describe, expect, it } from 'vitest'

import { buildNodeCatalog, getNodeColor, getNodeIcon, getNodeLabel } from './nodeTypes'

describe('nodeTypes helpers', () => {
  it('returns safe fallbacks for missing node types', () => {
    expect(getNodeColor(undefined)).toBe('#6b7280')
    expect(getNodeLabel(undefined)).toBe('Unknown node type')
    expect(getNodeIcon(undefined)).toBe('circle')
  })

  it('merges runtime plugin definitions into the node catalog', () => {
    const catalog = buildNodeCatalog([
      {
        type: 'action:plugin/acme/request',
        category: 'action',
        source: 'plugin',
        plugin_id: 'acme',
        plugin_name: 'Acme Toolkit',
        label: 'Acme Request',
        description: 'Call the Acme API',
        icon: 'globe',
        color: '#f97316',
        default_config: { endpoint: '/status' },
        fields: [],
        outputs: [{ id: 'success', label: 'Success' }],
        output_hints: [{ expression: 'input.result', label: 'Result' }],
      },
    ])

    const definition = catalog.map['action:plugin/acme/request']
    expect(definition).toMatchObject({
      type: 'action:plugin/acme/request',
      source: 'plugin',
      pluginId: 'acme',
      pluginName: 'Acme Toolkit',
      label: 'Acme Request',
      color: '#f97316',
      defaultConfig: { endpoint: '/status' },
    })

    const actionCategory = catalog.categories.find((category) => category.id === 'action')
    expect(actionCategory?.types.some((type) => type.type === 'action:plugin/acme/request')).toBe(true)
  })
})
