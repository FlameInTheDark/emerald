import type { LLMContextMessage, LLMConversationMessage, LLMToolCall, LLMToolResult } from '../types'

export type ToolStepStatus = 'running' | 'completed' | 'failed'

export type AssistantTranscriptPart =
  | {
      id: string
      kind: 'reasoning'
      reasoning: string
    }
  | {
      id: string
      kind: 'assistant'
      content: string
    }
  | {
      id: string
      kind: 'tool'
      toolCall?: LLMToolCall
      label: string
      status: ToolStepStatus
      toolResult: LLMToolResult
    }

export function buildAssistantTranscript(message: LLMConversationMessage): AssistantTranscriptPart[] {
  if (message.role !== 'assistant') {
    return []
  }

  const contextMessages = message.context_messages ?? []
  if (contextMessages.length > 0) {
    const parts = buildTranscriptFromContextMessages(contextMessages)
    if (parts.length > 0) {
      return parts
    }
  }

  return buildTranscriptFromAssistantMessage(message)
}

export function formatToolArguments(rawArguments: string): string {
  try {
    return JSON.stringify(JSON.parse(rawArguments), null, 2)
  } catch {
    return rawArguments
  }
}

function buildTranscriptFromContextMessages(messages: LLMContextMessage[]): AssistantTranscriptPart[] {
  const parts: AssistantTranscriptPart[] = []
  const toolPartIndexByID = new Map<string, number>()

  messages.forEach((message, index) => {
    if (message.role === 'assistant') {
      if (message.reasoning?.trim()) {
        parts.push({
          id: `reasoning-${index}`,
          kind: 'reasoning',
          reasoning: message.reasoning,
        })
      }
      if (message.content?.trim()) {
        parts.push({
          id: `assistant-${index}`,
          kind: 'assistant',
          content: message.content,
        })
      }
      for (const toolCall of message.tool_calls ?? []) {
        toolPartIndexByID.set(toolCall.id, parts.length)
        parts.push({
          id: `tool-${toolCall.id}`,
          kind: 'tool',
          toolCall,
          label: toolCall.function.name,
          status: 'running',
          toolResult: {
            tool: toolCall.function.name,
            arguments: parseToolArguments(toolCall.function.arguments),
          },
        })
      }
      return
    }

    if (message.role !== 'tool') {
      return
    }

    const toolCallID = message.tool_call_id?.trim()
    const existingIndex = toolCallID ? toolPartIndexByID.get(toolCallID) : undefined
    if (existingIndex !== undefined) {
      const existingPart = parts[existingIndex]
      if (existingPart && existingPart.kind === 'tool') {
        const toolResult = buildToolResultFromContextMessage(message, existingPart.toolCall)
        parts[existingIndex] = {
          ...existingPart,
          label: toolResult.display?.title || existingPart.toolCall?.function.name || toolResult.tool,
          status: toolResult.error ? 'failed' : 'completed',
          toolResult,
        }
        return
      }
    }

    const toolResult = buildToolResultFromContextMessage(message)
    parts.push({
      id: `tool-${toolCallID ?? index}`,
      kind: 'tool',
      label: toolResult.display?.title || toolResult.tool,
      status: toolResult.error ? 'failed' : 'completed',
      toolResult,
    })
  })

  return parts
}

function buildTranscriptFromAssistantMessage(message: LLMConversationMessage): AssistantTranscriptPart[] {
  const parts: AssistantTranscriptPart[] = []

  if (message.reasoning?.trim()) {
    parts.push({
      id: `${message.id}-reasoning`,
      kind: 'reasoning',
      reasoning: message.reasoning,
    })
  }
  if (message.content.trim()) {
    parts.push({
      id: `${message.id}-content`,
      kind: 'assistant',
      content: message.content,
    })
  }

  const toolCalls = message.tool_calls ?? []
  const toolResults = message.tool_results ?? []
  toolCalls.forEach((toolCall, index) => {
    const matchingResult = toolResults[index]
    parts.push({
      id: `${message.id}-tool-${toolCall.id}`,
      kind: 'tool',
      toolCall,
      label: matchingResult?.display?.title || toolCall.function.name,
      status: matchingResult ? (matchingResult.error ? 'failed' : 'completed') : 'running',
      toolResult: matchingResult ?? {
        tool: toolCall.function.name,
        arguments: parseToolArguments(toolCall.function.arguments),
      },
    })
  })
  toolResults.slice(toolCalls.length).forEach((toolResult, index) => {
    parts.push({
      id: `${message.id}-tool-extra-${index}`,
      kind: 'tool',
      label: toolResult.display?.title || toolResult.tool,
      status: toolResult.error ? 'failed' : 'completed',
      toolResult,
    })
  })

  return parts
}

function buildToolResultFromContextMessage(message: LLMContextMessage, toolCall?: LLMToolCall): LLMToolResult {
  const parsedPayload = parseToolPayload(message.content)
  return {
    tool: message.name ?? toolCall?.function.name ?? 'Tool',
    arguments: toolCall ? parseToolArguments(toolCall.function.arguments) : undefined,
    result: parsedPayload.result,
    error: parsedPayload.error,
    display: parsedPayload.display,
  }
}

function parseToolPayload(content?: string): Pick<LLMToolResult, 'result' | 'error' | 'display'> {
  if (!content?.trim()) {
    return {}
  }

  try {
    const parsed = JSON.parse(content) as Record<string, unknown>
    if (typeof parsed.error === 'string') {
      return {
        error: parsed.error,
        display: isToolDisplay(parsed.display) ? parsed.display : undefined,
      }
    }
    if (Object.prototype.hasOwnProperty.call(parsed, 'result')) {
      return {
        result: parsed.result,
        display: isToolDisplay(parsed.display) ? parsed.display : undefined,
      }
    }
    return { result: parsed }
  } catch {
    return { result: content }
  }
}

function parseToolArguments(rawArguments: string): unknown {
  try {
    return JSON.parse(rawArguments)
  } catch {
    return rawArguments
  }
}

function isToolDisplay(value: unknown): value is NonNullable<LLMToolResult['display']> {
  return typeof value === 'object' && value !== null && 'kind' in value
}
