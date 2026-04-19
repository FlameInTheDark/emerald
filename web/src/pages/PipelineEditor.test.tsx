import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import PipelineEditor from './PipelineEditor'
import { useUIStore } from '../store/ui'
import type { Pipeline } from '../types'

const {
  blockerStore,
  executionLogState,
  mockApi,
  reactFlowApi,
  reactFlowState,
} = vi.hoisted(() => {
  const listeners = new Set<() => void>()

  return {
    blockerStore: {
      current: {
        state: 'unblocked' as 'unblocked' | 'blocked',
        proceed: vi.fn(),
        reset: vi.fn(),
      },
      set(next: { state: 'unblocked' | 'blocked'; proceed: ReturnType<typeof vi.fn>; reset: ReturnType<typeof vi.fn> }) {
        this.current = next
        listeners.forEach((listener) => listener())
      },
      subscribe(listener: () => void) {
        listeners.add(listener)
        return () => listeners.delete(listener)
      },
    },
    executionLogState: {
      onRealtimeStatusChange: null as null | ((isReady: boolean) => void),
    },
    reactFlowApi: {
      screenToFlowPosition: vi.fn((point: { x: number; y: number }) => point),
      getViewport: vi.fn(() => ({ x: 0, y: 0, zoom: 1 })),
      setViewport: vi.fn(),
      zoomIn: vi.fn(),
      zoomOut: vi.fn(),
      fitView: vi.fn(),
    },
    reactFlowState: {
      latestNodes: [] as Array<Record<string, any>>,
      latestOnNodesChange: null as null | ((changes: Array<Record<string, unknown>>) => void),
    },
    mockApi: {
      pipelines: {
        get: vi.fn(),
        update: vi.fn(),
        run: vi.fn(),
      },
      templates: {
        list: vi.fn(),
        create: vi.fn(),
        get: vi.fn(),
      },
      llmProviders: {
        list: vi.fn(),
      },
      llm: {
        editorAssistantStream: vi.fn(),
      },
    },
  }
})

vi.mock('react-router-dom', async () => {
  const React = await vi.importActual<typeof import('react')>('react')
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')

  return {
    ...actual,
    useBlocker: vi.fn(() => React.useSyncExternalStore(
      (listener) => blockerStore.subscribe(listener),
      () => blockerStore.current,
    )),
  }
})

vi.mock('@xyflow/react', async () => {
  function applyNodeChanges(changes: Array<Record<string, unknown>>, nodes: Array<Record<string, any>>) {
    let nextNodes = nodes

    for (const change of changes) {
      if (change.type === 'remove') {
        nextNodes = nextNodes.filter((node) => node.id !== change.id)
        continue
      }

      if (change.type === 'select') {
        nextNodes = nextNodes.map((node) => (
          node.id === change.id
            ? { ...node, selected: change.selected }
            : node
        ))
        continue
      }

      if (change.type === 'position') {
        nextNodes = nextNodes.map((node) => (
          node.id === change.id
            ? { ...node, position: change.position ?? node.position, dragging: change.dragging }
            : node
        ))
        continue
      }

      if (change.type === 'dimensions') {
        nextNodes = nextNodes.map((node) => (
          node.id === change.id
            ? {
                ...node,
                width: change.dimensions?.width ?? node.width,
                height: change.dimensions?.height ?? node.height,
                resizing: change.resizing,
              }
            : node
        ))
      }
    }

    return nextNodes
  }

  function applyEdgeChanges(changes: Array<Record<string, unknown>>, edges: Array<Record<string, any>>) {
    let nextEdges = edges

    for (const change of changes) {
      if (change.type === 'remove') {
        nextEdges = nextEdges.filter((edge) => edge.id !== change.id)
        continue
      }

      if (change.type === 'select') {
        nextEdges = nextEdges.map((edge) => (
          edge.id === change.id
            ? { ...edge, selected: change.selected }
            : edge
        ))
      }
    }

    return nextEdges
  }

  return {
    ReactFlowProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
    ReactFlow: ({
      nodes,
      onNodesChange,
      onNodeClick,
      onPaneClick,
      children,
    }: {
      nodes: Array<Record<string, any>>
      onNodesChange?: (changes: Array<Record<string, unknown>>) => void
      onNodeClick?: (event: { preventDefault: () => void; clientX: number; clientY: number }, node: Record<string, any>) => void
      onPaneClick?: (event: { preventDefault: () => void; clientX: number; clientY: number }) => void
      children?: React.ReactNode
    }) => {
      reactFlowState.latestNodes = nodes
      reactFlowState.latestOnNodesChange = onNodesChange ?? null

      return (
        <div data-testid="react-flow">
          <button
            type="button"
            onClick={() => onPaneClick?.({ preventDefault() {}, clientX: 0, clientY: 0 })}
          >
            Canvas pane
          </button>
          {nodes.map((node) => {
            const label = typeof node.data?.label === 'string' ? node.data.label : node.id

            return (
              <button
                key={node.id}
                type="button"
                aria-label={`Node ${label}`}
                onClick={() => onNodeClick?.({ preventDefault() {}, clientX: 24, clientY: 24 }, node)}
              >
                {label}
              </button>
            )
          })}
          {children}
        </div>
      )
    },
    Background: () => null,
    Panel: ({ children, className }: { children: React.ReactNode; className?: string }) => (
      <div className={className}>{children}</div>
    ),
    useReactFlow: () => reactFlowApi,
    applyNodeChanges,
    applyEdgeChanges,
    addEdge: (connection: Record<string, unknown>, edges: Array<Record<string, unknown>>) => (
      edges.concat({
        id: String(connection.id ?? `edge-${edges.length + 1}`),
        ...connection,
      })
    ),
    MarkerType: {
      ArrowClosed: 'arrow-closed',
    },
  }
})

vi.mock('../api/client', () => ({
  api: mockApi,
}))

vi.mock('../components/flow/NodePalette', () => ({
  default: () => <div data-testid="node-palette" />,
}))

vi.mock('../components/flow/NodeConfigPanel', () => ({
  default: ({
    nodeLabel,
    onLabelChange,
  }: {
    nodeLabel: string
    onLabelChange: (label: string) => void
  }) => (
    <div data-testid="node-config-panel">
      <div>Node config for {nodeLabel}</div>
      <input aria-label="Node config input" value={nodeLabel} readOnly />
      <button type="button" onClick={() => onLabelChange('Renamed node')}>
        Rename node
      </button>
    </div>
  ),
}))

vi.mock('../components/flow/ExecutionLog', () => ({
  default: ({
    onRealtimeStatusChange,
  }: {
    onRealtimeStatusChange?: (isReady: boolean) => void
  }) => {
    executionLogState.onRealtimeStatusChange = onRealtimeStatusChange ?? null

    return (
      <div data-testid="execution-log">
        <button type="button" onClick={() => onRealtimeStatusChange?.(true)}>
          Execution log ready
        </button>
      </div>
    )
  },
}))

vi.mock('../components/flow/NodeExecutionModal', () => ({
  default: () => null,
}))

vi.mock('../components/flow/nodes/EmeraldNode', () => ({
  default: () => null,
}))

vi.mock('../components/flow/edges/EmeraldEdge', () => ({
  default: () => null,
}))

describe('PipelineEditor', () => {
  let currentPipeline: Pipeline

  afterEach(() => {
    cleanup()
  })

  beforeEach(() => {
    vi.clearAllMocks()

    currentPipeline = createPipeline('Start')
    blockerStore.set({
      state: 'unblocked',
      proceed: vi.fn(),
      reset: vi.fn(),
    })

    reactFlowApi.screenToFlowPosition.mockImplementation((point: { x: number; y: number }) => point)
    reactFlowApi.getViewport.mockReturnValue({ x: 0, y: 0, zoom: 1 })
    reactFlowApi.setViewport.mockResolvedValue(undefined)
    reactFlowApi.zoomIn.mockResolvedValue(undefined)
    reactFlowApi.zoomOut.mockResolvedValue(undefined)
    reactFlowApi.fitView.mockResolvedValue(undefined)
    executionLogState.onRealtimeStatusChange = null
    reactFlowState.latestNodes = []
    reactFlowState.latestOnNodesChange = null

    mockApi.pipelines.get.mockImplementation(async () => structuredClone(currentPipeline))
    mockApi.pipelines.update.mockImplementation(async (_id: string, payload: Record<string, any>) => {
      currentPipeline = {
        ...currentPipeline,
        name: payload.name,
        description: payload.description,
        nodes: payload.nodes,
        edges: payload.edges,
        viewport: payload.viewport,
        status: payload.status,
        updated_at: '2026-04-10T15:30:00Z',
      }

      return structuredClone(currentPipeline)
    })
    mockApi.pipelines.run.mockResolvedValue({
      execution_id: 'exec-1',
      status: 'completed',
      duration: '1s',
      nodes_run: 1,
    })
    mockApi.templates.list.mockResolvedValue([])
    mockApi.templates.create.mockResolvedValue({
      id: 'template-1',
      name: 'Template',
      description: 'Saved template',
      category: 'custom',
      created_at: '2026-04-10T15:00:00Z',
      definition: {
        nodes: [],
        edges: [],
        viewport: { x: 0, y: 0, zoom: 1 },
      },
    })
    mockApi.templates.get.mockResolvedValue({
      id: 'template-1',
      name: 'Template',
      description: 'Saved template',
      category: 'custom',
      created_at: '2026-04-10T15:00:00Z',
      definition: {
        nodes: [],
        edges: [],
        viewport: { x: 0, y: 0, zoom: 1 },
      },
    })
    mockApi.llmProviders.list.mockResolvedValue([])
    mockApi.llm.editorAssistantStream.mockResolvedValue({
      content: 'Noop',
      operations: [],
    })

    useUIStore.setState({
      sidebarCollapsed: true,
      selectedNodeId: null,
      toasts: [],
      showExecutionLog: false,
      activeLeaveConfirmation: null,
    })
  })

  it('shows dirty state, keeps dock groups separated, and supports canvas-scoped undo/redo', async () => {
    const user = userEvent.setup()
    renderEditor()

    expect(await screen.findByRole('button', { name: 'Node Start' })).toBeInTheDocument()
    expect(screen.queryByLabelText('Unsaved changes')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Undo' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Redo' })).not.toBeInTheDocument()
    const zoomInButton = screen.getByRole('button', { name: 'Zoom in' })

    await user.click(screen.getByRole('button', { name: 'Node Start' }))
    expect(await screen.findByTestId('node-config-panel')).toBeInTheDocument()

    const fitViewButton = screen.getByRole('button', { name: 'Fit view' })
    const deleteButton = screen.getByRole('button', { name: 'Delete' })
    expect(deleteButton.parentElement).not.toBe(fitViewButton.parentElement)
    expect(
      fitViewButton.parentElement?.compareDocumentPosition(deleteButton.parentElement as Node) ?? 0,
    ).toBeGreaterThan(0)

    await user.click(screen.getByRole('button', { name: 'Rename node' }))

    expect(await screen.findByRole('button', { name: 'Node Renamed node' })).toBeInTheDocument()
    expect(screen.getByLabelText('Unsaved changes')).toBeInTheDocument()
    const undoButton = screen.getByRole('button', { name: 'Undo' })
    const zoomInButtonAfterEdit = screen.getByRole('button', { name: 'Zoom in' })
    expect(screen.queryByRole('button', { name: 'Redo' })).not.toBeInTheDocument()
    expect(undoButton.parentElement).not.toBe(zoomInButtonAfterEdit.parentElement)

    const configInput = screen.getByRole('textbox', { name: 'Node config input' })
    fireEvent.keyDown(configInput, { key: 'z', ctrlKey: true })
    expect(screen.getByRole('button', { name: 'Node Renamed node' })).toBeInTheDocument()

    const canvasWrapper = screen.getByTestId('react-flow').parentElement as HTMLElement
    fireEvent.mouseEnter(canvasWrapper, { clientX: 48, clientY: 48 })
    fireEvent.keyDown(document, { key: 'z', ctrlKey: true })
    expect(await screen.findByRole('button', { name: 'Node Start' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Undo' })).not.toBeInTheDocument()
    const redoButton = screen.getByRole('button', { name: 'Redo' })
    const zoomInButtonAfterUndo = screen.getByRole('button', { name: 'Zoom in' })
    expect(redoButton.parentElement).not.toBe(zoomInButtonAfterUndo.parentElement)

    fireEvent.keyDown(document, { key: 'y', ctrlKey: true })
    expect(await screen.findByRole('button', { name: 'Node Renamed node' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Undo' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Redo' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /^Save$/i }))

    await waitFor(() => expect(mockApi.pipelines.update).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(screen.queryByLabelText('Unsaved changes')).not.toBeInTheDocument())
    expect(currentPipeline.nodes).toContain('Renamed node')
  })

  it('undoes pasted nodes after copy and paste', async () => {
    const user = userEvent.setup()
    renderEditor()

    expect(await screen.findByRole('button', { name: 'Node Start' })).toBeInTheDocument()

    const canvasWrapper = screen.getByTestId('react-flow').parentElement as HTMLElement
    fireEvent.mouseEnter(canvasWrapper, { clientX: 120, clientY: 120 })
    fireEvent.mouseMove(canvasWrapper, { clientX: 120, clientY: 120 })

    await user.click(screen.getByRole('button', { name: 'Node Start' }))

    reactFlowApi.screenToFlowPosition.mockClear()
    fireEvent.keyDown(document, { key: 'c', ctrlKey: true })
    fireEvent.keyDown(document, { key: 'v', ctrlKey: true })

    expect(reactFlowApi.screenToFlowPosition).toHaveBeenCalledTimes(1)

    await waitFor(() => {
      expect(screen.getAllByRole('button', { name: 'Node Start' })).toHaveLength(2)
    })

    await emitRuntimeMeasurementForNewestNode()

    await user.click(screen.getByRole('button', { name: 'Undo' }))

    await waitFor(() => {
      expect(screen.getAllByRole('button', { name: 'Node Start' })).toHaveLength(1)
    })
  })

  it('undoes duplicated nodes after runtime dimension updates', async () => {
    const user = userEvent.setup()
    renderEditor()

    expect(await screen.findByRole('button', { name: 'Node Start' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Node Start' }))
    await user.click(screen.getByRole('button', { name: 'Duplicate' }))

    await waitFor(() => {
      expect(screen.getAllByRole('button', { name: 'Node Start' })).toHaveLength(2)
    })

    await emitRuntimeMeasurementForNewestNode()

    await user.click(screen.getByRole('button', { name: 'Undo' }))

    await waitFor(() => {
      expect(screen.getAllByRole('button', { name: 'Node Start' })).toHaveLength(1)
    })
  })

  it('shows a leave confirmation modal for blocked navigation and handles cancel or discard', async () => {
    const user = userEvent.setup()
    renderEditor()

    await makeEditorDirty(user)

    const firstBlocker = {
      state: 'blocked' as const,
      proceed: vi.fn(),
      reset: vi.fn(),
    }
    blockerStore.set(firstBlocker)

    expect(await screen.findByRole('dialog', { name: 'Unsaved changes' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(firstBlocker.reset).toHaveBeenCalledTimes(1))

    const secondBlocker = {
      state: 'blocked' as const,
      proceed: vi.fn(),
      reset: vi.fn(),
    }
    blockerStore.set(secondBlocker)

    expect(await screen.findByRole('dialog', { name: 'Unsaved changes' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Leave without saving' }))
    await waitFor(() => expect(secondBlocker.proceed).toHaveBeenCalledTimes(1))
  })

  it('waits for save success before allowing blocked navigation to continue', async () => {
    const user = userEvent.setup()
    let resolveUpdate: ((value: Pipeline) => void) | null = null

    mockApi.pipelines.update.mockImplementationOnce(() => new Promise<Pipeline>((resolve) => {
      resolveUpdate = resolve
    }))

    renderEditor()
    await makeEditorDirty(user)

    const blocker = {
      state: 'blocked' as const,
      proceed: vi.fn(),
      reset: vi.fn(),
    }
    blockerStore.set(blocker)

    expect(await screen.findByRole('dialog', { name: 'Unsaved changes' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Save and continue' }))

    await waitFor(() => expect(mockApi.pipelines.update).toHaveBeenCalledTimes(1))
    expect(blocker.proceed).not.toHaveBeenCalled()

    currentPipeline = {
      ...currentPipeline,
      name: 'Example pipeline',
      description: 'Testing pipeline editor',
      nodes: JSON.stringify([
        createPersistedNode('Renamed node'),
      ]),
      edges: '[]',
      viewport: JSON.stringify({ x: 0, y: 0, zoom: 1 }),
      status: 'draft',
      updated_at: '2026-04-10T15:40:00Z',
    }

    resolveUpdate?.(structuredClone(currentPipeline))

    await waitFor(() => expect(blocker.proceed).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Unsaved changes' })).not.toBeInTheDocument())
  })

  it('keeps the leave confirmation open when save and continue fails', async () => {
    const user = userEvent.setup()

    mockApi.pipelines.update.mockRejectedValueOnce(new Error('Save failed'))

    renderEditor()
    await makeEditorDirty(user)

    const blocker = {
      state: 'blocked' as const,
      proceed: vi.fn(),
      reset: vi.fn(),
    }
    blockerStore.set(blocker)

    expect(await screen.findByRole('dialog', { name: 'Unsaved changes' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Save and continue' }))

    await waitFor(() => expect(mockApi.pipelines.update).toHaveBeenCalledTimes(1))
    expect(blocker.proceed).not.toHaveBeenCalled()
    expect(screen.getByRole('dialog', { name: 'Unsaved changes' })).toBeInTheDocument()
  })

  it('registers beforeunload protection while the editor is dirty', async () => {
    const user = userEvent.setup()
    renderEditor()

    await makeEditorDirty(user)

    const event = new Event('beforeunload', { cancelable: true }) as BeforeUnloadEvent
    const preventDefault = vi.spyOn(event, 'preventDefault')
    Object.defineProperty(event, 'returnValue', {
      configurable: true,
      writable: true,
      value: undefined,
    })

    window.dispatchEvent(event)

    expect(preventDefault).toHaveBeenCalled()
    expect(event.returnValue).toBe('')
  })

  it('waits for execution log realtime readiness before starting a run', async () => {
    const user = userEvent.setup()
    renderEditor()

    expect(await screen.findByRole('button', { name: 'Node Start' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Run' }))

    expect(await screen.findByTestId('execution-log')).toBeInTheDocument()
    expect(mockApi.pipelines.run).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Execution log ready' }))

    await waitFor(() => expect(mockApi.pipelines.run).toHaveBeenCalledTimes(1))
  })
})

function renderEditor() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: Infinity,
      },
      mutations: {
        retry: false,
      },
    },
  })

  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/pipelines/pipeline-1']}>
        <Routes>
          <Route path="/pipelines/:id" element={<PipelineEditor />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

async function makeEditorDirty(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole('button', { name: 'Node Start' }))
  await user.click(screen.getByRole('button', { name: 'Rename node' }))
  await screen.findByRole('button', { name: 'Node Renamed node' })
}

function createPersistedNode(label: string) {
  return {
    id: 'node-1',
    type: 'emerald',
    position: { x: 0, y: 0 },
    data: {
      type: 'trigger:manual',
      label,
      config: {},
    },
  }
}

async function emitRuntimeMeasurementForNewestNode() {
  const newestNode = reactFlowState.latestNodes[reactFlowState.latestNodes.length - 1]
  expect(newestNode).toBeDefined()
  expect(reactFlowState.latestOnNodesChange).toBeTruthy()

  await act(async () => {
    reactFlowState.latestOnNodesChange?.([{
      id: newestNode.id,
      type: 'dimensions',
      dimensions: { width: 220, height: 72 },
      resizing: false,
    }])
  })
}

function createPipeline(label: string): Pipeline {
  return {
    id: 'pipeline-1',
    name: 'Example pipeline',
    description: 'Testing pipeline editor',
    nodes: JSON.stringify([
      createPersistedNode(label),
    ]),
    edges: '[]',
    viewport: JSON.stringify({ x: 0, y: 0, zoom: 1 }),
    status: 'draft',
    created_at: '2026-04-10T15:00:00Z',
    updated_at: '2026-04-10T15:00:00Z',
  }
}
