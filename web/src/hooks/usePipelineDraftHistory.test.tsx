import { act, renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { Edge, Node } from '@xyflow/react'

import { usePipelineDraftHistory, type PipelineDraftState } from './usePipelineDraftHistory'

function createNode(label: string, selected = false): Node {
  return {
    id: 'node-1',
    type: 'emerald',
    position: { x: 0, y: 0 },
    selected,
    data: {
      type: 'trigger:manual',
      label,
      config: {},
    },
  }
}

function createDraft(overrides: Partial<PipelineDraftState> = {}): PipelineDraftState {
  return {
    nodes: [createNode('Start')],
    edges: [] as Edge[],
    pipelineName: 'Example pipeline',
    pipelineDescription: 'Testing history',
    ...overrides,
  }
}

function sanitizeDraft(draft: PipelineDraftState): PipelineDraftState {
  return {
    ...draft,
    nodes: draft.nodes.map((node) => ({
      ...node,
      selected: false,
    })),
    edges: draft.edges.map((edge) => ({
      ...edge,
      selected: false,
    })),
  }
}

describe('usePipelineDraftHistory', () => {
  it('starts clean with no undo or redo history', () => {
    const { result } = renderHook(() => usePipelineDraftHistory({
      initialDraft: createDraft(),
      sanitizeDraft,
      serializeDraft: JSON.stringify,
    }))

    expect(result.current.isDirty).toBe(false)
    expect(result.current.canUndo).toBe(false)
    expect(result.current.canRedo).toBe(false)
  })

  it('creates a history frame for structural edits and supports undo/redo', () => {
    const { result } = renderHook(() => usePipelineDraftHistory({
      initialDraft: createDraft(),
      sanitizeDraft,
      serializeDraft: JSON.stringify,
    }))

    act(() => {
      result.current.commitDraft((draft) => ({
        ...draft,
        pipelineName: 'Renamed pipeline',
      }))
    })

    expect(result.current.draft.pipelineName).toBe('Renamed pipeline')
    expect(result.current.isDirty).toBe(true)
    expect(result.current.canUndo).toBe(true)
    expect(result.current.canRedo).toBe(false)

    act(() => {
      result.current.undo()
    })

    expect(result.current.draft.pipelineName).toBe('Example pipeline')
    expect(result.current.isDirty).toBe(false)
    expect(result.current.canUndo).toBe(false)
    expect(result.current.canRedo).toBe(true)

    act(() => {
      result.current.redo()
    })

    expect(result.current.draft.pipelineName).toBe('Renamed pipeline')
    expect(result.current.isDirty).toBe(true)
    expect(result.current.canUndo).toBe(true)
    expect(result.current.canRedo).toBe(false)
  })

  it('updates the saved baseline without clearing undo history', () => {
    const { result } = renderHook(() => usePipelineDraftHistory({
      initialDraft: createDraft(),
      sanitizeDraft,
      serializeDraft: JSON.stringify,
    }))

    act(() => {
      result.current.commitDraft((draft) => ({
        ...draft,
        pipelineDescription: 'Saved change',
      }))
    })

    expect(result.current.isDirty).toBe(true)
    expect(result.current.canUndo).toBe(true)

    act(() => {
      result.current.markSaved()
    })

    expect(result.current.isDirty).toBe(false)
    expect(result.current.canUndo).toBe(true)

    act(() => {
      result.current.undo()
    })

    expect(result.current.draft.pipelineDescription).toBe('Testing history')
    expect(result.current.isDirty).toBe(true)
  })

  it('ignores selection-only live updates for dirty checks', () => {
    const { result } = renderHook(() => usePipelineDraftHistory({
      initialDraft: createDraft(),
      sanitizeDraft,
      serializeDraft: JSON.stringify,
    }))

    act(() => {
      result.current.updateDraftLive((draft) => ({
        ...draft,
        nodes: draft.nodes.map((node) => ({
          ...node,
          selected: true,
        })),
      }))
    })

    expect(result.current.isDirty).toBe(false)
    expect(result.current.canUndo).toBe(false)
    expect(result.current.draft.nodes[0].selected).toBe(true)
  })

  it('commits drag-like interactions as a single undo step', () => {
    const { result } = renderHook(() => usePipelineDraftHistory({
      initialDraft: createDraft(),
      sanitizeDraft,
      serializeDraft: JSON.stringify,
    }))

    act(() => {
      result.current.beginInteraction()
      result.current.updateDraftLive((draft) => ({
        ...draft,
        nodes: draft.nodes.map((node) => ({
          ...node,
          position: { x: 80, y: 40 },
        })),
      }))
      result.current.commitInteraction()
    })

    expect(result.current.draft.nodes[0].position).toEqual({ x: 80, y: 40 })
    expect(result.current.canUndo).toBe(true)

    act(() => {
      result.current.undo()
    })

    expect(result.current.draft.nodes[0].position).toEqual({ x: 0, y: 0 })
  })
})
