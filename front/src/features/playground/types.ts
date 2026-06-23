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

For commercial licensing, please contact support@quantumnous.com
*/
// Message types
export type MessageRole = 'user' | 'assistant' | 'system'

export type MessageStatus = 'loading' | 'streaming' | 'complete' | 'error'

export interface MessageVersion {
  id: string
  content: string
}

export interface MessageAttachment {
  id: string
  file_id?: string
  type: 'image'
  url: string
  mediaType?: string
  filename?: string
  omitted?: boolean
  size?: number
}

export interface Message {
  key: string
  from: MessageRole
  versions: MessageVersion[]
  model?: string
  durationMs?: number
  tokens?: {
    prompt?: number
    completion?: number
    total?: number
  }
  attachments?: MessageAttachment[]
  sources?: { href: string; title: string }[]
  reasoning?: {
    content: string
    duration: number
  }
  isReasoningStreaming?: boolean
  isReasoningComplete?: boolean
  isContentComplete?: boolean
  status?: MessageStatus
  errorCode?: string | null
}

// API payload types
export interface ChatCompletionMessage {
  role: MessageRole
  content: string | ContentPart[]
}

export interface ContentPart {
  type: 'text' | 'image_url'
  text?: string
  image_url?: {
    url: string
    file_id?: string
  }
}

export interface ChatCompletionRequest {
  model: string
  group?: string
  messages: ChatCompletionMessage[]
  stream: boolean
  temperature?: number
  top_p?: number
  max_tokens?: number
  frequency_penalty?: number
  presence_penalty?: number
  seed?: number
}

export interface ChatCompletionChunk {
  id: string
  object: string
  created: number
  model: string
  choices: Array<{
    index: number
    delta: {
      role?: MessageRole
      content?: string
      reasoning_content?: string
    }
    finish_reason: string | null
  }>
}

export interface ChatCompletionResponse {
  id: string
  object: string
  created: number
  model: string
  choices: Array<{
    index: number
    message: {
      role: MessageRole
      content: string
      reasoning_content?: string
    }
    finish_reason: string
  }>
  usage?: {
    prompt_tokens: number
    completion_tokens: number
    total_tokens: number
  }
}

export interface PlaygroundSession {
  id: number
  user_id: number
  title: string
  model: string
  group: string
  pinned?: boolean
  summary?: string
  messages?: Message[]
  message_count?: number
  config: Partial<PlaygroundConfig>
  created_time: number
  updated_time: number
}

export interface PlaygroundSessionPayload {
  id?: number | null
  title: string
  model: string
  group: string
  summary?: string
  messages: Message[]
  config: PlaygroundConfig
}

export interface PlaygroundAttachment {
  id: string
  file_id: string
  type: 'image'
  url: string
  media_type: string
  filename: string
  size: number
  created_time: number
}

// Configuration types
export interface PlaygroundConfig {
  model: string
  group: string
  temperature: number
  top_p: number
  max_tokens: number
  frequency_penalty: number
  presence_penalty: number
  seed: number | null
  stream: boolean
}

export interface ParameterEnabled {
  temperature: boolean
  top_p: boolean
  max_tokens: boolean
  frequency_penalty: boolean
  presence_penalty: boolean
  seed: boolean
}

// Model and group options
export interface ModelOption {
  label: string
  value: string
  category?: string
  contextWindow?: number
  description?: string
  pricePerKToken?: number
  supportsImage?: boolean
  supportsVideo?: boolean
}

export interface GroupOption {
  label: string
  value: string
  ratio: number
  desc?: string
  description?: string
}

export type ImageResolutionPreset = 'auto' | '1k' | '2k' | '4k'

export type ImageAspectRatio =
  | 'auto'
  | '1:1'
  | '16:9'
  | '9:16'
  | '4:3'
  | '3:4'
  | '3:2'
  | '2:3'

export interface ImageGenerationParams {
  resolution: ImageResolutionPreset
  aspectRatio: ImageAspectRatio
  quality: 'auto' | 'standard' | 'hd'
  style: 'auto' | 'vivid' | 'natural'
  n: number
  seed: number | null
  negativePrompt: string
}

export interface ImageGenerationItem {
  id: string
  prompt: string
  negativePrompt?: string
  params: ImageGenerationParams
  model: string
  group: string
  referenceImages?: MessageAttachment[]
  urls: string[]
  status: 'running' | 'succeeded' | 'failed'
  errorMessage?: string
  durationMs?: number
  createdAt: number
  updatedAt?: number
}

export interface VideoGenerationParams {
  aspectRatio: '16:9' | '9:16' | '1:1'
  durationSec: number
  seed: number | null
}

export interface VideoGenerationItem {
  id: string
  prompt: string
  params: VideoGenerationParams
  model: string
  group: string
  status: 'pending' | 'running' | 'succeeded' | 'failed'
  previewUrl?: string
  errorMessage?: string
  durationMs?: number
  createdAt: number
}
