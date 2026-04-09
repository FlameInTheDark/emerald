import { afterEach, describe, expect, it, vi } from 'vitest'

import { APIError, api } from './client'

describe('api.llm.chatStream', () => {
  const originalFetch = global.fetch

  afterEach(() => {
    global.fetch = originalFetch
    vi.restoreAllMocks()
  })

  it('parses SSE events separated by CRLF blank lines', async () => {
    const encoder = new TextEncoder()
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(
          encoder.encode(
            'data: {"type":"assistant_delta","delta":"Hello"}\r\n\r\n' +
              'data: {"type":"assistant_delta","delta":" world"}\r\n\r\n' +
              'data: {"type":"done","response":{"conversation_id":"conv-1","conversation":{"id":"conv-1","title":"Chat","proxmox_enabled":false,"kubernetes_enabled":false,"compaction_count":0,"context_window":128000,"context_token_count":0,"last_prompt_tokens":0,"last_completion_tokens":0,"last_total_tokens":0,"last_message_at":"2026-04-09T00:00:00Z","created_at":"2026-04-09T00:00:00Z","updated_at":"2026-04-09T00:00:00Z"},"content":"Hello world","usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}}\r\n\r\n',
          ),
        )
        controller.close()
      },
    })

    global.fetch = vi.fn().mockResolvedValue(
      new Response(body, {
        status: 200,
        headers: {
          'Content-Type': 'text/event-stream',
        },
      }),
    ) as typeof fetch

    const events: string[] = []
    const response = await api.llm.chatStream(
      { message: 'hi' },
      {
        onEvent: (event) => {
          if (event.type === 'assistant_delta') {
            events.push(event.delta)
          }
        },
      },
    )

    expect(events).toEqual(['Hello', ' world'])
    expect(response.content).toBe('Hello world')
    expect(response.conversation_id).toBe('conv-1')
  })

  it('preserves stream error status codes', async () => {
    const encoder = new TextEncoder()
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(
          encoder.encode('data: {"type":"error","error":"Too Many Requests","status":429}\r\n\r\n'),
        )
        controller.close()
      },
    })

    global.fetch = vi.fn().mockResolvedValue(
      new Response(body, {
        status: 200,
        headers: {
          'Content-Type': 'text/event-stream',
        },
      }),
    ) as typeof fetch

    await expect(api.llm.chatStream({ message: 'hi' })).rejects.toMatchObject<Partial<APIError>>({
      status: 429,
      message: 'Too Many Requests',
    })
  })
})
