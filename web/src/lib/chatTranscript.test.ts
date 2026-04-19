import { describe, expect, it } from 'vitest'

import { buildAssistantTranscript } from './chatTranscript'

describe('buildAssistantTranscript', () => {
  it('reconstructs structured tool display metadata from stored context messages', () => {
    const transcript = buildAssistantTranscript({
      id: 'msg-1',
      role: 'assistant',
      content: '',
      created_at: '2026-04-18T10:00:00Z',
      context_messages: [
        {
          role: 'assistant',
          content: 'Looking through the file.',
          tool_calls: [
            {
              id: 'tool-1',
              type: 'function',
              function: {
                name: 'read_file',
                arguments: '{"path":"internal/llm/tools.go"}',
              },
            },
          ],
        },
        {
          role: 'tool',
          name: 'read_file',
          tool_call_id: 'tool-1',
          content: JSON.stringify({
            result: {
              path: 'internal/llm/tools.go',
              content: '1: package llm',
            },
            display: {
              kind: 'read',
              title: 'internal/llm/tools.go',
              path: 'internal/llm/tools.go',
              summary: 'Lines 1-1 of 40',
              preview: '1: package llm',
            },
          }),
        },
      ],
    })

    const toolPart = transcript.find((part) => part.kind === 'tool')
    expect(toolPart).toBeDefined()
    if (!toolPart || toolPart.kind !== 'tool') {
      throw new Error('tool part missing')
    }

    expect(toolPart.label).toBe('internal/llm/tools.go')
    expect(toolPart.toolResult.display?.kind).toBe('read')
    expect(toolPart.toolResult.display?.preview).toContain('package llm')
  })
})
