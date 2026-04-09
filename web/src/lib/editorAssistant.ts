import type { Edge, Node, Viewport } from '@xyflow/react'

import type { LivePipelineOperation } from '../types'

export function applyLivePipelineOperations(input: {
  nodes: Node[]
  edges: Edge[]
  viewport: Viewport
  operations: LivePipelineOperation[]
}): {
  nodes: Node[]
  edges: Edge[]
  viewport: Viewport
} {
  let nodes = [...input.nodes]
  let edges = [...input.edges]
  let viewport = input.viewport

  for (const operation of input.operations) {
    switch (operation.type) {
      case 'add_nodes':
        nodes = nodes.concat(operation.nodes ?? [])
        break
      case 'update_nodes': {
        const updates = new Map((operation.nodes ?? []).map((node) => [node.id, node]))
        nodes = nodes.map((node) => updates.get(node.id) ?? node)
        break
      }
      case 'delete_nodes': {
        const toDelete = new Set(operation.node_ids ?? [])
        nodes = nodes.filter((node) => !toDelete.has(node.id))
        edges = edges.filter((edge) => !toDelete.has(edge.source) && !toDelete.has(edge.target))
        break
      }
      case 'add_edges':
        edges = edges.concat(operation.edges ?? [])
        break
      case 'update_edges': {
        const updates = new Map((operation.edges ?? []).map((edge) => [edge.id, edge]))
        edges = edges.map((edge) => updates.get(edge.id) ?? edge)
        break
      }
      case 'delete_edges': {
        const toDelete = new Set(operation.edge_ids ?? [])
        edges = edges.filter((edge) => !toDelete.has(edge.id))
        break
      }
      case 'set_viewport':
        if (operation.viewport) {
          viewport = operation.viewport
        }
        break
    }
  }

  return { nodes, edges, viewport }
}
