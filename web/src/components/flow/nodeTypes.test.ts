import { describe, expect, it } from 'vitest'

import { buildNodeCatalog, buildNodeMenuCategories, getNodeColor, getNodeIcon, getNodeLabel } from './nodeTypes'

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
        menu_path: ['Acme', 'Requests'],
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
      menuPath: ['Acme', 'Requests'],
      defaultConfig: { endpoint: '/status' },
    })

    const actionCategory = catalog.categories.find((category) => category.id === 'action')
    expect(actionCategory?.types.some((type) => type.type === 'action:plugin/acme/request')).toBe(true)
  })

  it('builds nested menu categories from menu paths', () => {
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
        menu_path: ['Acme', 'Requests'],
        default_config: {},
      },
      {
        type: 'action:plugin/acme/status',
        category: 'action',
        source: 'plugin',
        plugin_id: 'acme',
        plugin_name: 'Acme Toolkit',
        label: 'Acme Status',
        description: 'Check status',
        icon: 'server',
        color: '#f97316',
        menu_path: ['Acme'],
        default_config: {},
      },
    ])

    const actionCategory = buildNodeMenuCategories(catalog.categories).find((category) => category.id === 'action')
    const acmeGroup = actionCategory?.groups.find((group) => group.label === 'Acme')
    const requestsGroup = acmeGroup?.groups.find((group) => group.label === 'Requests')

    expect(actionCategory).toBeDefined()
    expect(acmeGroup?.types.map((type) => type.label)).toContain('Acme Status')
    expect(requestsGroup?.types.map((type) => type.label)).toContain('Acme Request')
  })
})
