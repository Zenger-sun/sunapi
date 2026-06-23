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
import { api } from '@/lib/api'
import { API_ENDPOINTS } from './constants'
import type {
  ChatCompletionRequest,
  ChatCompletionResponse,
  ModelOption,
  GroupOption,
  ImageGenerationItem,
  ImageGenerationParams,
  MessageAttachment,
  PlaygroundAttachment,
  PlaygroundSession,
  PlaygroundSessionPayload,
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
    ...(payload.resolution !== 'auto' ? { resolution: payload.resolution } : {}),
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
  status?: string
  url?: string
  preview_url?: string
  result_url?: string
  error?: { message?: string }
}

function normalizeVideoTaskResponse(payload: unknown): VideoTaskResponse {
  const raw = payload as Record<string, unknown>
  const data = raw?.data as Record<string, unknown> | undefined
  const source = data && typeof data === 'object' ? data : raw
  const taskId = source.id ?? source.task_id
  const resultUrl = source.preview_url ?? source.url ?? source.result_url
  const failReason = source.fail_reason

  return {
    id: typeof taskId === 'string' ? taskId : undefined,
    task_id: typeof source.task_id === 'string' ? source.task_id : undefined,
    status: typeof source.status === 'string' ? source.status : undefined,
    preview_url: typeof resultUrl === 'string' ? resultUrl : undefined,
    url: typeof resultUrl === 'string' ? resultUrl : undefined,
    result_url:
      typeof source.result_url === 'string' ? source.result_url : undefined,
    error:
      typeof failReason === 'string' && failReason
        ? { message: failReason }
        : (source.error as VideoTaskResponse['error']),
  }
}

export async function submitVideoTask(payload: {
  model: string
  group: string
  prompt: string
  duration_sec: number
  aspect_ratio: VideoGenerationParams['aspectRatio']
  seed: number | null
}): Promise<VideoTaskResponse> {
  const res = await api.post(
    API_ENDPOINTS.PLAYGROUND_VIDEOS,
    {
      model: payload.model,
      group: payload.group,
      prompt: payload.prompt,
      duration: payload.duration_sec,
      seed: payload.seed ?? undefined,
      n: 1,
      response_format: 'url',
      metadata: {
        aspect_ratio: payload.aspect_ratio,
      },
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
