import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'

import { api } from '../api/client'
import { buildNodeCatalog } from '../components/flow/nodeTypes'
import type { NodeDefinition, NodeTypeDefinition, PluginBundleStatus } from '../types'

export function useNodeDefinitions() {
  const query = useQuery({
    queryKey: ['node-definitions'],
    queryFn: () => api.nodeDefinitions.list(),
  })

  const definitions = query.data?.definitions || []
  const plugins = (query.data?.plugins || []) as PluginBundleStatus[]
  const catalog = useMemo(() => buildNodeCatalog(definitions), [definitions])

  return {
    ...query,
    definitions,
    plugins,
    categories: catalog.categories,
    map: catalog.map as Record<string, NodeTypeDefinition>,
  }
}

export function definitionOutputHandles(definition?: NodeDefinition | NodeTypeDefinition) {
  if (!definition || !Array.isArray(definition.outputs)) {
    return []
  }
  return definition.outputs
}
