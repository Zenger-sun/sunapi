/*
Copyright (C) 2026 SunAPI Project Owner

This file is part of the SunAPI Creator Studio Components.

Licensed under the SunAPI Creator Studio Source-Available Commercial License
v1.0. See LICENSE-CREATOR-STUDIO at the repository root.

This source is available for evaluation, but modification, redistribution,
hosting, sale, resale, sublicensing, or commercial use requires prior written
permission from the SunAPI project owner.
*/
import { api } from '@/lib/api'
import { API_ENDPOINTS } from './constants'
import type {
  ChatCompletionRequest,
  ChatCompletionResponse,
  ModelOption,
  AgentBrowserCaptureRequest,
  AgentBrowserCaptureResult,
  AgentSkillFormData,
  AgentSkillItem,
  AgentSkillMarketplaceItem,
  AgentComponentFormData,
  AgentComponentItem,
  AgentFileUpload,
  AgentFileUploadPurpose,
  AgentHTTPRequestPayload,
  AgentHTTPResponse,
  AgentReportOutputFile,
  AgentReportWriteResult,
  GroupOption,
  ImageGenerationItem,
  ImageGenerationParams,
  MessageAttachment,
  PlaygroundAttachment,
  PlaygroundSession,
  PlaygroundSessionPayload,
  VideoGenerationItem,
  VideoGenerationParams,
} from './types'

interface PageResponse<T> {
  items?: T[]
  total?: number
}

function normalizeModelOption(
  model: string | Record<string, unknown>
): ModelOption | null {
  if (typeof model === 'string') {
    return {
      label: model,
      value: model,
    }
  }
  const value =
    typeof model.value === 'string'
      ? model.value
      : typeof model.id === 'string'
        ? model.id
        : ''
  if (!value) return null
  return {
    label: typeof model.label === 'string' ? model.label : value,
    value,
    category: typeof model.category === 'string' ? model.category : undefined,
    contextWindow:
      typeof model.context_window === 'number'
        ? model.context_window
        : typeof model.contextWindow === 'number'
          ? model.contextWindow
          : undefined,
    description:
      typeof model.description === 'string' ? model.description : undefined,
    pricePerKToken:
      typeof model.price_per_k_token === 'number'
        ? model.price_per_k_token
        : typeof model.pricePerKToken === 'number'
          ? model.pricePerKToken
          : undefined,
    supportsImage:
      typeof model.supports_image === 'boolean'
        ? model.supports_image
        : typeof model.supportsImage === 'boolean'
          ? model.supportsImage
          : false,
    supportsVideo:
      typeof model.supports_video === 'boolean'
        ? model.supports_video
        : typeof model.supportsVideo === 'boolean'
          ? model.supportsVideo
          : false,
  }
}

/**
 * Send chat completion request (non-streaming)
 */
export async function sendChatCompletion(
  payload: ChatCompletionRequest
): Promise<ChatCompletionResponse> {
  const res = await api.post(API_ENDPOINTS.CHAT_COMPLETIONS, payload, {
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Get user available models
 */
export async function getUserModels(group?: string): Promise<ModelOption[]> {
  const res = await api.get(API_ENDPOINTS.USER_MODELS, {
    params: { ...(group ? { group } : {}), details: 1 },
  })
  const { data } = res

  if (!data.success || !Array.isArray(data.data)) {
    return []
  }

  const rawModels = data.data as Array<string | Record<string, unknown>>
  const models: ModelOption[] = []

  for (const rawModel of rawModels) {
    const normalized = normalizeModelOption(rawModel)
    if (normalized) models.push(normalized)
  }

  return models
}

/**
 * Get user groups
 */
export async function getUserGroups(): Promise<GroupOption[]> {
  const res = await api.get(API_ENDPOINTS.USER_GROUPS)
  const { data } = res

  if (!data.success || !data.data) {
    return []
  }

  const groupData = data.data as Record<string, { desc: string; ratio: number }>

  // label is for button display (name only); desc is for dropdown content
  return Object.entries(groupData).map(([group, info]) => ({
    label: group,
    value: group,
    ratio: info.ratio,
    desc: info.desc,
  }))
}

export async function getPlaygroundSessions(): Promise<PlaygroundSession[]> {
  const res = await api.get(API_ENDPOINTS.PLAYGROUND_SESSIONS, {
    params: { p: 1, page_size: 50 },
    skipErrorHandler: true,
  })
  const { data } = res

  if (!data.success) {
    throw new Error(data.message || 'Failed to load conversations')
  }

  const page = data.data as PageResponse<PlaygroundSession>
  return Array.isArray(page?.items) ? page.items : []
}

export async function getPlaygroundSession(
  id: number
): Promise<PlaygroundSession> {
  const res = await api.get(`${API_ENDPOINTS.PLAYGROUND_SESSIONS}/${id}`, {
    skipErrorHandler: true,
  })
  const { data } = res

  if (!data.success || !data.data) {
    throw new Error(data.message || 'Failed to load conversation')
  }

  return data.data as PlaygroundSession
}

export async function savePlaygroundSession(
  payload: PlaygroundSessionPayload
): Promise<PlaygroundSession> {
  const id = payload.id
  const endpoint = id
    ? `${API_ENDPOINTS.PLAYGROUND_SESSIONS}/${id}`
    : API_ENDPOINTS.PLAYGROUND_SESSIONS
  const res = id
    ? await api.put(endpoint, payload, {
        skipErrorHandler: true,
      })
    : await api.post(endpoint, payload, {
        skipErrorHandler: true,
      })
  const { data } = res

  if (!data.success || !data.data) {
    throw new Error(data.message || 'Failed to save conversation')
  }

  return data.data as PlaygroundSession
}

export async function updatePlaygroundSessionMeta(
  id: number,
  payload: { title?: string; pinned?: boolean }
): Promise<PlaygroundSession> {
  const res = await api.patch(
    `${API_ENDPOINTS.PLAYGROUND_SESSIONS}/${id}`,
    payload,
    {
      skipErrorHandler: true,
    }
  )
  const { data } = res

  if (!data.success || !data.data) {
    throw new Error(data.message || 'Failed to update conversation')
  }

  return data.data as PlaygroundSession
}

export async function deletePlaygroundSession(id: number): Promise<void> {
  const res = await api.delete(`${API_ENDPOINTS.PLAYGROUND_SESSIONS}/${id}`, {
    skipErrorHandler: true,
  })
  const { data } = res

  if (!data.success) {
    throw new Error(data.message || 'Failed to delete conversation')
  }
}

export async function deleteAllPlaygroundSessions(): Promise<void> {
  const res = await api.delete(API_ENDPOINTS.PLAYGROUND_SESSIONS, {
    skipErrorHandler: true,
  })
  const { data } = res

  if (!data.success) {
    throw new Error(data.message || 'Failed to clear conversations')
  }
}

export async function uploadPlaygroundAttachment(
  file: File
): Promise<PlaygroundAttachment> {
  const formData = new FormData()
  formData.append('file', file)

  const res = await api.post(API_ENDPOINTS.PLAYGROUND_ATTACHMENTS, formData, {
    skipBusinessError: true,
    skipErrorHandler: true,
  })
  const { data } = res

  if (!data.success || !data.data) {
    throw new Error(data.message || 'Failed to upload attachment')
  }

  return data.data as PlaygroundAttachment
}

export async function uploadAgentFile(
  file: File,
  purpose: AgentFileUploadPurpose = 'report-template'
): Promise<AgentFileUpload> {
  const formData = new FormData()
  formData.append('file', file)
  formData.append('purpose', purpose)

  const res = await api.post(API_ENDPOINTS.AGENT_FILES, formData, {
    skipBusinessError: true,
    skipErrorHandler: true,
  })
  const { data } = res

  if (!data.success || !data.data) {
    throw new Error(data.message || 'Failed to upload file')
  }

  return data.data as AgentFileUpload
}

export async function listAgentFiles(
  purpose: AgentFileUploadPurpose = 'report-template'
): Promise<AgentFileUpload[]> {
  const res = await api.get(API_ENDPOINTS.AGENT_FILES, {
    params: { purpose },
    skipErrorHandler: true,
  })
  const { data } = res

  if (!data.success || !Array.isArray(data.data)) {
    throw new Error(data.message || 'Failed to list files')
  }

  return data.data as AgentFileUpload[]
}

export async function writeAgentReport(payload: {
  title: string
  output_directory: string
  files: AgentReportOutputFile[]
}): Promise<AgentReportWriteResult> {
  const res = await api.post(API_ENDPOINTS.AGENT_REPORTS, payload, {
    skipBusinessError: true,
    skipErrorHandler: true,
  })
  const { data } = res

  if (!data.success || !data.data) {
    throw new Error(data.message || 'Failed to write report')
  }

  return data.data as AgentReportWriteResult
}

export async function captureAgentBrowserPage(
  payload: AgentBrowserCaptureRequest
): Promise<AgentBrowserCaptureResult> {
  const res = await api.post(API_ENDPOINTS.AGENT_BROWSER_CAPTURE, payload, {
    skipBusinessError: true,
    skipErrorHandler: true,
  })
  const { data } = res

  if (!data.success || !data.data) {
    throw new Error(data.message || 'Failed to capture browser page')
  }

  return data.data as AgentBrowserCaptureResult
}

export async function executeAgentHTTPRequest(
  payload: AgentHTTPRequestPayload
): Promise<AgentHTTPResponse> {
  const res = await api.post(API_ENDPOINTS.AGENT_HTTP_REQUEST, payload, {
    skipBusinessError: true,
    skipErrorHandler: true,
  })
  const { data } = res

  if (!data.success || !data.data) {
    throw new Error(data.message || 'Failed to execute HTTP request')
  }

  return data.data as AgentHTTPResponse
}

export async function listAgentComponents(): Promise<AgentComponentItem[]> {
  const res = await api.get(API_ENDPOINTS.AGENT_COMPONENTS, {
    skipErrorHandler: true,
  })
  const { data } = res

  if (!data.success || !Array.isArray(data.data)) {
    throw new Error(data.message || 'Failed to list components')
  }

  return data.data as AgentComponentItem[]
}

export async function createAgentComponent(
  payload: AgentComponentFormData
): Promise<AgentComponentItem> {
  const res = await api.post(API_ENDPOINTS.AGENT_COMPONENTS, payload, {
    skipBusinessError: true,
    skipErrorHandler: true,
  })
  const { data } = res

  if (!data.success || !data.data) {
    throw new Error(data.message || 'Failed to create component')
  }

  return data.data as AgentComponentItem
}

export async function deleteAgentComponent(id: string): Promise<void> {
  const res = await api.delete(`${API_ENDPOINTS.AGENT_COMPONENTS}/${id}`, {
    skipErrorHandler: true,
  })
  const { data } = res

  if (!data.success) {
    throw new Error(data.message || 'Failed to delete component')
  }
}

export async function listAgentSkills(): Promise<AgentSkillItem[]> {
  const res = await api.get('/api/agent-skills', {
    skipErrorHandler: true,
  })
  const { data } = res

  if (!data.success || !Array.isArray(data.data)) {
    throw new Error(data.message || 'Failed to list skills')
  }

  return data.data as AgentSkillItem[]
}

export async function listAgentSkillMarketplace(): Promise<
  AgentSkillMarketplaceItem[]
> {
  const res = await api.get('/api/agent-skills/marketplace', {
    skipErrorHandler: true,
  })
  const { data } = res

  if (!data.success || !Array.isArray(data.data)) {
    throw new Error(data.message || 'Failed to list skill marketplace')
  }

  return data.data as AgentSkillMarketplaceItem[]
}

export async function createAgentSkill(
  payload: AgentSkillFormData
): Promise<AgentSkillItem> {
  const res = await api.post('/api/agent-skills', payload, {
    skipBusinessError: true,
    skipErrorHandler: true,
  })
  const { data } = res

  if (!data.success || !data.data) {
    throw new Error(data.message || 'Failed to create skill')
  }

  return data.data as AgentSkillItem
}

export async function deleteAgentSkill(id: string): Promise<void> {
  const res = await api.delete(`/api/agent-skills/${id}`, {
    skipErrorHandler: true,
  })
  const { data } = res

  if (!data.success) {
    throw new Error(data.message || 'Failed to delete skill')
  }
}

export async function getImageGenerationHistory(): Promise<
  ImageGenerationItem[]
> {
  const res = await api.get(API_ENDPOINTS.PLAYGROUND_IMAGE_HISTORY, {
    params: { page_size: 50 },
    skipErrorHandler: true,
  })
  const { data } = res

  if (!data.success) {
    throw new Error(data.message || 'Failed to load image history')
  }

  const page = data.data as PageResponse<ImageGenerationItem>
  return Array.isArray(page?.items) ? page.items : []
}

export async function saveImageGenerationHistory(
  item: ImageGenerationItem
): Promise<ImageGenerationItem> {
  const res = await api.post(API_ENDPOINTS.PLAYGROUND_IMAGE_HISTORY, item, {
    skipErrorHandler: true,
  })
  const { data } = res

  if (!data.success || !data.data) {
    throw new Error(data.message || 'Failed to save image history')
  }

  return data.data as ImageGenerationItem
}

export async function deleteImageGenerationHistory(id: string): Promise<void> {
  const res = await api.delete(
    `${API_ENDPOINTS.PLAYGROUND_IMAGE_HISTORY}/${id}`,
    {
      skipErrorHandler: true,
    }
  )
  const { data } = res

  if (!data.success) {
    throw new Error(data.message || 'Failed to delete image history')
  }
}

export type ImageGenerationDataItem = {
  url?: string
  b64_json?: string
  revised_prompt?: string
}

export type ImageGenerationResponse = {
  created?: number
  data?: ImageGenerationDataItem[]
}

function resolveImageSize(
  resolution: ImageGenerationParams['resolution'],
  aspectRatio: ImageGenerationParams['aspectRatio']
) {
  if (resolution === 'auto' || aspectRatio === 'auto') {
    return undefined
  }
  const sizes = {
    '1k': {
      '1:1': '1024x1024',
      '16:9': '1792x1024',
      '9:16': '1024x1792',
      '4:3': '1344x1024',
      '3:4': '1024x1344',
      '3:2': '1536x1024',
      '2:3': '1024x1536',
    },
    '2k': {
      '1:1': '2048x2048',
      '16:9': '2048x1152',
      '9:16': '1152x2048',
      '4:3': '2048x1536',
      '3:4': '1536x2048',
      '3:2': '2048x1365',
      '2:3': '1365x2048',
    },
    '4k': {
      '1:1': '4096x4096',
      '16:9': '4096x2304',
      '9:16': '2304x4096',
      '4:3': '4096x3072',
      '3:4': '3072x4096',
      '3:2': '4096x2731',
      '2:3': '2731x4096',
    },
  } satisfies Record<
    Exclude<ImageGenerationParams['resolution'], 'auto'>,
    Record<Exclude<ImageGenerationParams['aspectRatio'], 'auto'>, string>
  >

  return sizes[resolution]?.[aspectRatio]
}

type ImageGenerationRequestPayload = {
  model: string
  group: string
  prompt: string
  negative_prompt?: string
  resolution?: ImageGenerationParams['resolution']
  aspect_ratio?: ImageGenerationParams['aspectRatio']
  size?: string
  quality: ImageGenerationParams['quality']
  style: ImageGenerationParams['style']
  n: number
  seed: number | null
  images?: Array<{ image_url: string }>
  response_format: 'url'
}

function buildImageGenerationRequest(payload: {
  model: string
  group: string
  prompt: string
  negative_prompt?: string
  resolution: ImageGenerationParams['resolution']
  aspect_ratio: ImageGenerationParams['aspectRatio']
  quality: ImageGenerationParams['quality']
  style: ImageGenerationParams['style']
  n: number
  seed: number | null
  reference_images?: MessageAttachment[]
}): ImageGenerationRequestPayload {
  const referenceImages =
    payload.reference_images
      ?.map((image) => ({
        image_url: image.file_id
          ? `playground-attachment://${image.file_id}`
          : image.url,
      }))
      .filter((image) => image.image_url.trim() !== '') ?? []
  const size = resolveImageSize(payload.resolution, payload.aspect_ratio)
  const requestPayload = {
    model: payload.model,
    group: payload.group,
    prompt: payload.prompt,
    negative_prompt: payload.negative_prompt,
    ...(payload.resolution !== 'auto'
      ? { resolution: payload.resolution }
      : {}),
    ...(payload.aspect_ratio !== 'auto'
      ? { aspect_ratio: payload.aspect_ratio }
      : {}),
    ...(size ? { size } : {}),
    quality: payload.quality,
    style: payload.style,
    n: payload.n,
    seed: payload.seed,
  }
  return {
    ...requestPayload,
    ...(referenceImages.length > 0 ? { images: referenceImages } : {}),
    response_format: 'url',
  }
}

export async function generateImage(payload: {
  model: string
  group: string
  prompt: string
  negative_prompt?: string
  resolution: ImageGenerationParams['resolution']
  aspect_ratio: ImageGenerationParams['aspectRatio']
  quality: ImageGenerationParams['quality']
  style: ImageGenerationParams['style']
  n: number
  seed: number | null
  reference_images?: MessageAttachment[]
}): Promise<ImageGenerationResponse> {
  const requestPayload = buildImageGenerationRequest(payload)
  const endpoint =
    requestPayload.images && requestPayload.images.length > 0
      ? API_ENDPOINTS.PLAYGROUND_IMAGE_EDITS
      : API_ENDPOINTS.PLAYGROUND_IMAGE_GENERATIONS

  const res = await api.post(endpoint, requestPayload, {
    skipErrorHandler: true,
  })
  return res.data as ImageGenerationResponse
}

export async function startImageGenerationTask(
  item: ImageGenerationItem
): Promise<ImageGenerationItem> {
  const request = buildImageGenerationRequest({
    model: item.model,
    group: item.group,
    prompt: item.prompt,
    negative_prompt: item.negativePrompt || undefined,
    resolution: item.params.resolution,
    aspect_ratio: item.params.aspectRatio,
    quality: item.params.quality,
    style: item.params.style,
    n: item.params.n,
    seed: item.params.seed,
    reference_images: item.referenceImages,
  })
  const res = await api.post(
    API_ENDPOINTS.PLAYGROUND_IMAGE_TASKS,
    {
      ...item,
      status: 'running',
      urls: [],
      errorMessage: undefined,
      request,
    },
    {
      skipErrorHandler: true,
    }
  )
  const { data } = res

  if (!data.success || !data.data) {
    throw new Error(data.message || 'Failed to start image task')
  }

  return data.data as ImageGenerationItem
}

export type VideoTaskResponse = {
  id?: string
  task_id?: string
  request_id?: string
  status?: string
  progress?: number
  url?: string
  preview_url?: string
  result_url?: string
  content_url?: string
  video?: { url?: string; duration?: number }
  error?: { message?: string }
}

function normalizeVideoTaskResponse(payload: unknown): VideoTaskResponse {
  const raw = (payload ?? {}) as Record<string, unknown>
  const data = raw?.data as Record<string, unknown> | undefined
  const source = data && typeof data === 'object' ? data : raw
  const video = source.video as Record<string, unknown> | undefined
  const taskId = source.id ?? source.task_id ?? source.request_id
  const resultUrl =
    source.preview_url ??
    source.url ??
    source.result_url ??
    source.content_url ??
    video?.url
  const failReason = source.fail_reason
  const progress = source.progress

  return {
    id: typeof taskId === 'string' ? taskId : undefined,
    task_id: typeof source.task_id === 'string' ? source.task_id : undefined,
    request_id:
      typeof source.request_id === 'string' ? source.request_id : undefined,
    status: typeof source.status === 'string' ? source.status : undefined,
    progress: typeof progress === 'number' ? progress : undefined,
    preview_url: typeof resultUrl === 'string' ? resultUrl : undefined,
    url: typeof resultUrl === 'string' ? resultUrl : undefined,
    result_url:
      typeof source.result_url === 'string' ? source.result_url : undefined,
    content_url:
      typeof source.content_url === 'string' ? source.content_url : undefined,
    video:
      video && typeof video === 'object'
        ? {
            url: typeof video.url === 'string' ? video.url : undefined,
            duration:
              typeof video.duration === 'number' ? video.duration : undefined,
          }
        : undefined,
    error:
      typeof failReason === 'string' && failReason
        ? { message: failReason }
        : typeof source.error === 'string' && source.error
          ? { message: source.error }
          : (source.error as VideoTaskResponse['error']),
  }
}

function videoAttachmentURL(image: MessageAttachment) {
  return image.file_id ? `playground-attachment://${image.file_id}` : image.url
}

export async function submitVideoTask(payload: {
  model: string
  group: string
  prompt: string
  duration_sec: number
  aspect_ratio: VideoGenerationParams['aspectRatio']
  resolution: VideoGenerationParams['resolution']
  seed: number | null
  reference_images?: MessageAttachment[]
}): Promise<VideoTaskResponse> {
  const referenceImages =
    payload.reference_images
      ?.map((image) => ({ url: videoAttachmentURL(image) }))
      .filter((image) => image.url.trim() !== '') ?? []
  const inputImage = referenceImages.length === 1 ? referenceImages[0] : null

  const res = await api.post(
    API_ENDPOINTS.PLAYGROUND_VIDEOS,
    {
      model: payload.model,
      group: payload.group,
      prompt: payload.prompt,
      duration: payload.duration_sec,
      seconds: String(payload.duration_sec),
      aspect_ratio: payload.aspect_ratio,
      resolution: payload.resolution,
      seed: payload.seed ?? undefined,
      ...(inputImage ? { image: inputImage } : {}),
      ...(referenceImages.length > 1
        ? { reference_images: referenceImages.slice(0, 7) }
        : {}),
    },
    {
      skipErrorHandler: true,
    }
  )
  return normalizeVideoTaskResponse(res.data)
}

export async function fetchVideoTask(
  taskId: string
): Promise<VideoTaskResponse> {
  const res = await api.get(`${API_ENDPOINTS.PLAYGROUND_VIDEOS}/${taskId}`, {
    skipErrorHandler: true,
  })
  return normalizeVideoTaskResponse(res.data)
}

export async function saveVideoHistoryItem(
  item: VideoGenerationItem
): Promise<VideoGenerationItem> {
  const res = await api.post(
    API_ENDPOINTS.PLAYGROUND_VIDEO_HISTORY,
    {
      id: item.id,
      taskId: item.taskId,
      prompt: item.prompt,
      params: item.params,
      model: item.model,
      group: item.group,
      referenceImages: item.referenceImages ?? [],
      status: item.status,
      progress: item.progress ?? 0,
      previewUrl: item.previewUrl,
      errorMessage: item.errorMessage,
      durationMs: item.durationMs,
      createdAt: item.createdAt,
    },
    { skipErrorHandler: true }
  )
  return res.data.data as VideoGenerationItem
}
