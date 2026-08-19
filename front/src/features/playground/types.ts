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
  knowledge_disabled?: boolean
  knowledge_base_ids?: string[]
  knowledge_conversation_id?: number
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

export type AgentFileUploadPurpose =
  | 'report-template'
  | 'web-template'
  | 'component-input'

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

export interface AgentSkillItem {
  id: string
  name: string
  description: string
  version: string
  source: 'builtin' | 'marketplace' | 'github' | 'custom'
  source_url?: string
  enabled: boolean
  config?: Record<string, unknown>
  permissions?: string[]
  created_time: number
  updated_time: number
}

export interface AgentSkillMarketplaceItem {
  id: string
  name: string
  description: string
  version: string
  source_url?: string
  tags?: string[]
  capabilities?: string[]
  permissions?: string[]
  config?: Record<string, unknown>
  installed: boolean
}

export interface AgentSkillFormData {
  name: string
  description: string
  source_type: 'marketplace' | 'github' | 'custom'
  source_url?: string
  marketplace_id?: string
  model_group?: string
  model_name?: string
  tags?: string[]
  config?: Record<string, unknown>
}

export interface KnowledgeBaseItem {
  id: string
  user_id: number
  name: string
  description?: string
  enabled: boolean
  document_count: number
  chunk_count: number
  created_time: number
  updated_time: number
  bindings?: KnowledgeBinding[]
}

export type KnowledgeBindingTargetType =
  | 'module'
  | 'agent'
  | 'blueprint'
  | 'conversation'

export interface KnowledgeBinding {
  id: number
  knowledge_base_id: string
  user_id: number
  target_type: KnowledgeBindingTargetType
  target_id: string
  created_time: number
}

export interface KnowledgeBindingFormData {
  target_type: KnowledgeBindingTargetType
  target_id: string
}

export interface KnowledgeAgentOption {
  id: string
  name: string
  blueprint_count?: number
}

export interface KnowledgeBlueprintOption {
  id: string
  agent_id: string
  name: string
  status?: string
}

export interface KnowledgeBaseFormData {
  name: string
  description?: string
}

export interface KnowledgeDocumentItem {
  id: string
  knowledge_base_id: string
  user_id: number
  filename: string
  media_type?: string
  size: number
  content_hash: string
  chunk_count: number
  created_time: number
  updated_time: number
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
  screenshot_path?: string
  screenshot_error?: string
  browser_target_id?: string
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

export type AgentComponentSource = 'builtin' | 'custom'

export interface AgentComponentConfig {
  tags?: string[]
  capabilities?: string[]
  inputs?: string[]
  outputs?: string[]
  settings?: Record<string, string>
  model_group?: string
  model_name?: string
  skill?: string
  role?: string
  icon?: string
  accent?: string
}

export interface AgentComponentItem {
  id: string
  user_id: number
  name: string
  description: string
  kind: string
  group: string
  source: AgentComponentSource
  enabled: boolean
  config?: AgentComponentConfig
  created_time: number
  updated_time: number
}

export interface AgentComponentFormData {
  name: string
  description: string
  group: string
  model_group?: string
  model_name?: string
  skill?: string
  inputs?: string[]
  outputs?: string[]
  settings?: Record<string, string>
  tags?: string[]
}

export interface AgentHTTPRequestPayload {
  url: string
  method?: string
  headers?: Record<string, string>
  body?: string
  timeout_seconds?: number
}

export interface AgentHTTPResponse {
  url: string
  final_url: string
  method: string
  status_code: number
  headers: Record<string, string>
  body: string
  body_bytes: number
  truncated: boolean
  content_type?: string
  duration_ms: number
  executed_at: number
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
  knowledgeBaseIds?: string[]
  knowledgeDisabled?: boolean
  knowledgeManual?: boolean
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
  knowledgeBaseIds?: string[]
  knowledgeDisabled?: boolean
  knowledgeManual?: boolean
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
