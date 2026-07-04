/*
Copyright (C) 2026 SunAPI Project Owner

This file is part of the SunAPI Creator Studio Components.

Licensed under the SunAPI Creator Studio Source-Available Commercial License
v1.0. See LICENSE-CREATOR-STUDIO at the repository root.

This source is available for evaluation, but modification, redistribution,
hosting, sale, resale, sublicensing, or commercial use requires prior written
permission from the SunAPI project owner.
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

export type AgentFileUploadPurpose = 'report-template' | 'web-template'

export interface AgentFileUpload {
  filename: string
  original_name?: string
  directory: string
  path: string
  media_type: string
  size: number
  hash?: string
  content_preview?: string
  output_directory?: string
  reused?: boolean
  replaced?: boolean
  created_time: number
}

export interface AgentReportOutputFile {
  label: string
  extension: string
  content: string
  template_path?: string
  template_original_name?: string
}

export interface AgentReportWrittenFile {
  label: string
  path: string
  extension: string
  size: number
}

export interface AgentReportWriteResult {
  output_directory: string
  files: AgentReportWrittenFile[]
  markdown_path?: string
  csv_path?: string
  created_time: number
}

export interface AgentBrowserCaptureRequest {
  url: string
  login_state?: string
  wait_seconds?: number
}

export interface AgentBrowserCaptureResult {
  url: string
  final_url: string
  title: string
  text: string
  captured_at: number
  elapsed_ms: number
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

export type VideoAspectRatio =
  | '16:9'
  | '9:16'
  | '1:1'
  | '4:3'
  | '3:4'
  | '3:2'
  | '2:3'

export type VideoResolutionPreset = '480p' | '720p'

export interface VideoGenerationParams {
  aspectRatio: VideoAspectRatio
  resolution: VideoResolutionPreset
  durationSec: number
  seed: number | null
}

export interface VideoGenerationItem {
  id: string
  taskId?: string
  prompt: string
  params: VideoGenerationParams
  model: string
  group: string
  referenceImages?: MessageAttachment[]
  status: 'queued' | 'running' | 'succeeded' | 'failed'
  progress?: number
  previewUrl?: string
  errorMessage?: string
  durationMs?: number
  createdAt: number
  updatedAt?: number
}
