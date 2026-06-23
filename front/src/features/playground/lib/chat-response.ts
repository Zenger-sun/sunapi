/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import { ERROR_MESSAGES } from '../constants'

type AnyRecord = Record<string, unknown>

export interface AssistantReply {
  content: string
  reasoning?: string
}

export interface StreamUpdate {
  done?: boolean
  content?: string
  reasoning?: string
  error?: string
  errorCode?: string
}

export interface ParsedError {
  message: string
  code?: string
}

function isRecord(value: unknown): value is AnyRecord {
  return typeof value === 'object' && value !== null
}

function asString(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function unwrapData(payload: unknown): unknown {
  if (!isRecord(payload)) return payload
  if (payload.success === true && 'data' in payload) {
    return payload.data
  }
  return payload
}

function collectText(value: unknown): string {
  if (typeof value === 'string') return value
  if (Array.isArray(value)) {
    return value
      .map((item) => {
        if (typeof item === 'string') return item
        if (!isRecord(item)) return ''
        return (
          asString(item.text) ||
          asString(item.output_text) ||
          asString(item.content)
        )
      })
      .join('')
  }
  return ''
}

function extractOutputText(payload: unknown): string {
  if (!isRecord(payload)) return ''

  const direct =
    asString(payload.output_text) ||
    asString(payload.response) ||
    asString(payload.text) ||
    asString(payload.content)
  if (direct) return direct

  const contentText = collectText(payload.content)
  if (contentText) return contentText

  const output = payload.output
  if (!Array.isArray(output)) return ''

  return output
    .map((item) => {
      if (!isRecord(item)) return ''
      const content = item.content
      if (Array.isArray(content)) {
        return content
          .map((part) => {
            if (!isRecord(part)) return ''
            return (
              asString(part.text) ||
              asString(part.output_text) ||
              asString(part.content)
            )
          })
          .join('')
      }
      return collectText(content)
    })
    .join('')
}

function extractReasoningText(payload: unknown): string {
  if (!isRecord(payload)) return ''

  const direct =
    asString(payload.reasoning_content) ||
    asString(payload.reasoning_text) ||
    asString(payload.reasoning)
  if (direct) return direct

  const output = payload.output
  if (!Array.isArray(output)) return ''

  return output
    .map((item) => {
      if (!isRecord(item)) return ''
      const type = asString(item.type)
      if (type.includes('reasoning')) {
        return collectText(item.content ?? item.summary ?? item.text)
      }
      return ''
    })
    .join('')
}

export function extractAssistantReply(payload: unknown): AssistantReply | null {
  const response = unwrapData(payload)
  if (!isRecord(response)) return null

  const choice = Array.isArray(response.choices)
    ? response.choices.find(isRecord)
    : undefined
  if (choice) {
    const message = isRecord(choice.message) ? choice.message : undefined
    const content =
      collectText(message?.content) ||
      collectText(choice.text) ||
      extractOutputText(response)
    const reasoning =
      asString(message?.reasoning_content) ||
      extractReasoningText(message) ||
      extractReasoningText(response)
    if (content || reasoning) {
      return { content, reasoning: reasoning || undefined }
    }
  }

  const content = extractOutputText(response)
  const reasoning = extractReasoningText(response)
  if (!content && !reasoning) return null
  return { content, reasoning: reasoning || undefined }
}

export function parseErrorPayload(payload: unknown): ParsedError | null {
  const source = unwrapData(payload)
  if (!isRecord(source)) {
    const message = asString(source).trim()
    return message ? { message } : null
  }

  if (source.type === 'error') {
    const message = asString(source.message).trim()
    const code = asString(source.code).trim()
    return message ? { message, code: code || undefined } : null
  }

  const error = source.error
  if (typeof error === 'string' && error.trim()) {
    return { message: error.trim() }
  }
  if (isRecord(error)) {
    const message = asString(error.message).trim()
    const code = asString(error.code).trim()
    if (message) return { message, code: code || undefined }
  }

  const message = asString(source.message).trim()
  return message ? { message } : null
}

export function parseStreamPayload(
  payload: unknown,
  eventType?: string
): StreamUpdate {
  if (payload === '[DONE]') return { done: true }

  const parsedError = parseErrorPayload(payload)
  if (
    parsedError &&
    isRecord(payload) &&
    (payload.type === 'error' || payload.error)
  ) {
    return { error: parsedError.message, errorCode: parsedError.code }
  }

  const source = unwrapData(payload)
  if (!isRecord(source)) return {}

  const type = asString(source.type) || eventType || ''
  if (
    type === 'done' ||
    type.endsWith('.done') ||
    type.endsWith('.completed')
  ) {
    const completed = isRecord(source.response) ? source.response : source
    const content = extractOutputText(completed)
    const reasoning = extractReasoningText(completed)
    return {
      done: true,
      content: content || undefined,
      reasoning: reasoning || undefined,
    }
  }

  const choice = Array.isArray(source.choices)
    ? source.choices.find(isRecord)
    : undefined
  if (choice) {
    const delta = isRecord(choice.delta) ? choice.delta : undefined
    const content = collectText(delta?.content) || collectText(choice.text)
    const reasoning = asString(delta?.reasoning_content)
    return {
      content: content || undefined,
      reasoning: reasoning || undefined,
    }
  }

  if (
    type.includes('reasoning') ||
    type.includes('summary_text') ||
    type.includes('reasoning_text')
  ) {
    const reasoning =
      asString(source.delta) ||
      asString(source.text) ||
      asString(source.content)
    return { reasoning: reasoning || undefined }
  }

  if (
    type.includes('output_text') ||
    type.includes('content_part') ||
    type.includes('text.delta')
  ) {
    const content =
      asString(source.delta) ||
      asString(source.text) ||
      asString(source.content)
    return { content: content || undefined }
  }

  const content = extractOutputText(source)
  const reasoning = extractReasoningText(source)
  return {
    content: content || undefined,
    reasoning: reasoning || undefined,
  }
}

export function parseUnknownError(error: unknown): ParsedError {
  if (isRecord(error)) {
    const response = isRecord(error.response) ? error.response : undefined
    const data = response?.data
    const parsed = parseErrorPayload(data)
    if (parsed) return parsed
    const message = asString(error.message).trim()
    if (message) return { message }
  }
  return { message: ERROR_MESSAGES.API_REQUEST_ERROR }
}
