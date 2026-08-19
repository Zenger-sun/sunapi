/*
Copyright (C) 2026 SunAPI Project Owner

This file is part of the SunAPI Creator Studio Components.

Licensed under the SunAPI Creator Studio Source-Available Commercial License
v1.0. See LICENSE-CREATOR-STUDIO at the repository root.

This source is available for evaluation, but modification, redistribution,
hosting, sale, resale, sublicensing, or commercial use requires prior written
permission from the SunAPI project owner.
*/
import type {
  ChatCompletionRequest,
  Message,
  PlaygroundConfig,
  ParameterEnabled,
} from '../types'
import {
  formatMessageForAPI,
  formatMessageForAPIWithoutImages,
  isValidMessage,
} from './message-utils'

/**
 * Build API request payload from messages and config
 */
export function buildChatCompletionPayload(
  messages: Message[],
  config: PlaygroundConfig,
  parameterEnabled: ParameterEnabled,
  conversationId?: number | null,
  knowledge?: {
    knowledgeBaseIds?: string[]
    knowledgeDisabled?: boolean
    knowledgeManual?: boolean
  }
): ChatCompletionRequest {
  // Filter and format valid messages
  const validMessages = messages.filter(isValidMessage)
  const lastUserMessage = [...validMessages]
    .reverse()
    .find((message) => message.from === 'user')
  const lastImageUserMessage = [...validMessages].reverse().find(
    (message) =>
      message.from === 'user' &&
      message.attachments?.some(
        (attachment) =>
          attachment.type === 'image' &&
          !attachment.omitted &&
          (attachment.file_id || attachment.url.trim() !== '')
      )
  )
  const processedMessages = validMessages.map((message) =>
    message === lastUserMessage || message === lastImageUserMessage
      ? formatMessageForAPI(message)
      : formatMessageForAPIWithoutImages(message)
  )

  const payload: ChatCompletionRequest = {
    model: config.model,
    group: config.group,
    messages: processedMessages,
    stream: config.stream,
  }

  if (conversationId && conversationId > 0) {
    payload.knowledge_conversation_id = conversationId
  }
  if (knowledge?.knowledgeBaseIds?.length) {
    payload.knowledge_base_ids = knowledge.knowledgeBaseIds
  }
  if (
    knowledge?.knowledgeDisabled ||
    (knowledge?.knowledgeManual && !knowledge.knowledgeBaseIds?.length)
  ) {
    payload.knowledge_disabled = true
  }

  // Add enabled parameters
  const parameterKeys: Array<keyof ParameterEnabled> = [
    'temperature',
    'top_p',
    'max_tokens',
    'frequency_penalty',
    'presence_penalty',
    'seed',
  ]

  parameterKeys.forEach((key) => {
    if (parameterEnabled[key]) {
      const value = config[key as keyof PlaygroundConfig]
      if (value !== undefined && value !== null) {
        ;(payload as unknown as Record<string, unknown>)[key] = value
      }
    }
  })

  return payload
}
