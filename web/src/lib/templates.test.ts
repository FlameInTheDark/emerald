import { describe, expect, it } from 'vitest'

import { buildPromptInsertSuggestions, buildTemplateSuggestions } from './templates'

describe('buildPromptInsertSuggestions', () => {
  const nodes = [
    {
      id: 'source',
      position: { x: 0, y: 0 },
      data: { label: 'Source HTTP', type: 'action:http' },
    },
    {
      id: 'prompt',
      position: { x: 240, y: 0 },
      data: { label: 'Prompt Node', type: 'llm:prompt' },
    },
  ]

  const edges = [
    {
      id: 'edge-source-prompt',
      source: 'source',
      target: 'prompt',
    },
  ]

  it('prefers the latest recorded input for the selected node', () => {
    const suggestions = buildPromptInsertSuggestions('prompt', nodes, edges, [
      {
        id: 'exec-source',
        execution_id: 'exec-1',
        node_id: 'source',
        node_type: 'action:http',
        status: 'completed',
        output: JSON.stringify({
          status_code: 200,
          response: { ignored: true },
        }),
      },
      {
        id: 'exec-prompt',
        execution_id: 'exec-1',
        node_id: 'prompt',
        node_type: 'llm:prompt',
        status: 'completed',
        input: JSON.stringify({
          user: 'Ada',
          payload: {
            id: 42,
            tags: ['one', 'two', 'three'],
          },
        }),
      },
    ])

    const sample = suggestions.find((suggestion) => suggestion.kind === 'sample')

    expect(sample).toMatchObject({
      label: 'Latest input sample',
      expression: 'sample.current_input',
      kind: 'sample',
    })
    expect(sample?.template).toContain('Example runtime input for this node from the latest execution:')
    expect(sample?.template).toContain('```json')
    expect(sample?.template).toContain('"user": "Ada"')
    expect(sample?.template).toContain('"id": 42')
  })

  it('falls back to merged upstream outputs when the selected node has no recorded input yet', () => {
    const suggestions = buildPromptInsertSuggestions('prompt', nodes, edges, [
      {
        id: 'exec-source',
        execution_id: 'exec-1',
        node_id: 'source',
        node_type: 'action:http',
        status: 'completed',
        output: JSON.stringify({
          status_code: 200,
          response: {
            message: 'ok',
          },
        }),
      },
    ])

    const sample = suggestions.find((suggestion) => suggestion.kind === 'sample')

    expect(sample).toMatchObject({
      label: 'Merged input sample',
      expression: 'sample.merged_input',
      kind: 'sample',
    })
    expect(sample?.template).toContain('Example merged input built from the latest upstream execution data:')
    expect(sample?.template).toContain('"status_code": 200')
    expect(sample?.template).toContain('"message": "ok"')
  })

  it('includes runtime output hints and secret suggestions in template autocomplete', () => {
    const suggestions = buildTemplateSuggestions(
      'prompt',
      nodes,
      edges,
      [],
      {
        'action:http': {
          type: 'action:http',
          label: 'HTTP Request',
          description: 'Make an HTTP request',
          icon: 'globe',
          category: 'action',
          color: '#10b981',
          defaultConfig: {},
          outputHints: [
            {
              expression: 'input.response.body',
              label: 'HTTP body',
              description: 'Runtime response body.',
            },
          ],
        },
      },
      [
        {
          id: 'secret-1',
          name: 'db_password',
          created_at: '2026-04-10T00:00:00Z',
          updated_at: '2026-04-10T00:00:00Z',
        },
      ],
    )

    expect(suggestions).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          expression: 'input.response.body',
          template: '{{input.response.body}}',
          label: 'HTTP body',
        }),
        expect.objectContaining({
          expression: 'secret.db_password',
          template: '{{secret.db_password}}',
          label: 'Secret: db_password',
        }),
      ]),
    )
  })
})
