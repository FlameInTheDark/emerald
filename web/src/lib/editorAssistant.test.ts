import { describe, expect, it } from 'vitest'

import { applyLivePipelineOperations } from './editorAssistant'

describe('applyLivePipelineOperations', () => {
  it('removes connected edges when deleting nodes', () => {
    const result = applyLivePipelineOperations({
      nodes: [
        { id: 'a', position: { x: 0, y: 0 }, data: {} },
        { id: 'b', position: { x: 10, y: 0 }, data: {} },
      ],
      edges: [
        { id: 'edge-a-b', source: 'a', target: 'b' },
      ],
      viewport: { x: 0, y: 0, zoom: 1 },
      operations: [
        { type: 'delete_nodes', node_ids: ['b'] },
      ],
    })

    expect(result.nodes.map((node) => node.id)).toEqual(['a'])
    expect(result.edges).toEqual([])
  })

  it('updates nodes, edges, and viewport in sequence', () => {
    const result = applyLivePipelineOperations({
      nodes: [
        { id: 'a', position: { x: 0, y: 0 }, data: { label: 'Old' } },
      ],
      edges: [
        { id: 'edge-a-a', source: 'a', target: 'a' },
      ],
      viewport: { x: 0, y: 0, zoom: 1 },
      operations: [
        {
          type: 'update_nodes',
          nodes: [{ id: 'a', position: { x: 25, y: 15 }, data: { label: 'New' } }],
        },
        {
          type: 'update_edges',
          edges: [{ id: 'edge-a-a', source: 'a', target: 'a', sourceHandle: 'tool' }],
        },
        {
          type: 'set_viewport',
          viewport: { x: 50, y: 60, zoom: 1.5 },
        },
      ],
    })

    expect(result.nodes[0].position).toEqual({ x: 25, y: 15 })
    expect(result.nodes[0].data).toEqual({ label: 'New' })
    expect(result.edges[0].sourceHandle).toBe('tool')
    expect(result.viewport).toEqual({ x: 50, y: 60, zoom: 1.5 })
  })
})
