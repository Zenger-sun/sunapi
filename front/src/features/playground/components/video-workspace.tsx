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
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertCircleIcon,
  ChevronDownIcon,
  ClockIcon,
  CopyIcon,
  DownloadIcon,
  FilmIcon,
  ImagePlusIcon,
  MoreHorizontalIcon,
  PaperclipIcon,
  PlayCircleIcon,
  RotateCcwIcon,
  SendIcon,
  Settings2Icon,
  SparklesIcon,
  Trash2Icon,
  WandSparklesIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { fetchAuthenticatedBlob } from '@/lib/authenticated-media'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  PromptInput,
  PromptInputAttachment,
  PromptInputButton,
  PromptInputFooter,
  PromptInputHeader,
  PromptInputTextarea,
  type PromptInputMessage,
  usePromptInputAttachments,
} from '@/components/ai-elements/prompt-input'
import {
  fetchVideoTask,
  submitVideoTask,
  uploadPlaygroundAttachment,
} from '../api'
import type {
  GroupOption,
  MessageAttachment,
  ModelOption,
  VideoAspectRatio,
  VideoGenerationItem,
  VideoGenerationParams,
  VideoResolutionPreset,
} from '../types'
import { ModelCapsule } from './model-capsule'

interface VideoWorkspaceProps {
  params: VideoGenerationParams
  model: string
  group: string
  models: ModelOption[]
  groups: GroupOption[]
  isLoadingModels?: boolean
  onModelChange: (value: string) => void
  onGroupChange: (value: string) => void
  onGeneratingChange?: (generating: boolean) => void
}

const MAX_CONCURRENT_VIDEO_TASKS = 3
const VIDEO_POLL_INTERVAL_MS = 3000
const VIDEO_RESULT_STATUS = new Set([
  'completed',
  'done',
  'succeeded',
  'success',
])
const VIDEO_FAILED_STATUS = new Set([
  'failed',
  'error',
  'expired',
  'cancelled',
  'canceled',
])
const VIDEO_RUNNING_STATUS = new Set(['in_progress', 'processing', 'running'])
const VIDEO_QUEUED_STATUS = new Set(['queued', 'pending'])

const ASPECT_RATIOS: {
  value: VideoAspectRatio
  label: string
  hintKey: string
}[] = [
  { value: '16:9', label: '16:9', hintKey: 'Landscape / desktop / YouTube' },
  { value: '9:16', label: '9:16', hintKey: 'Portrait / short video / mobile' },
  { value: '1:1', label: '1:1', hintKey: 'Square social clip' },
  { value: '4:3', label: '4:3', hintKey: 'Classic presentation frame' },
  { value: '3:4', label: '3:4', hintKey: 'Portrait product shot' },
  { value: '3:2', label: '3:2', hintKey: 'Photo-style landscape' },
  { value: '2:3', label: '2:3', hintKey: 'Poster portrait' },
]

const RESOLUTION_OPTIONS: {
  value: VideoResolutionPreset
  label: string
  hintKey: string
}[] = [
  { value: '480p', label: '480p', hintKey: 'Faster preview render' },
  { value: '720p', label: '720p', hintKey: 'Sharper default render' },
]

const DURATIONS = [3, 4, 5, 6, 8, 10, 15]

function generateId() {
  return Math.random().toString(36).slice(2, 10) + Date.now().toString(36)
}

function statusFromTask(status?: string): VideoGenerationItem['status'] {
  const normalized = (status || '').toLowerCase()
  if (VIDEO_RESULT_STATUS.has(normalized)) return 'succeeded'
  if (VIDEO_FAILED_STATUS.has(normalized)) return 'failed'
  if (VIDEO_RUNNING_STATUS.has(normalized)) return 'running'
  if (VIDEO_QUEUED_STATUS.has(normalized)) return 'queued'
  return 'running'
}

function generationDurationMs(item: VideoGenerationItem, now: number) {
  if (typeof item.durationMs === 'number' && item.durationMs >= 0) {
    return item.durationMs
  }
  if (item.status === 'running' || item.status === 'queued') {
    return Math.max(0, now - item.createdAt)
  }
  if (item.updatedAt && item.createdAt) {
    return Math.max(0, item.updatedAt - item.createdAt)
  }
  return null
}

function formatGenerationDuration(ms: number) {
  const totalSeconds = Math.max(0, Math.floor(ms / 1000))
  if (totalSeconds < 60) return `${totalSeconds}s`
  const totalMinutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  if (totalMinutes < 60) {
    return `${totalMinutes}m ${seconds.toString().padStart(2, '0')}s`
  }
  const hours = Math.floor(totalMinutes / 60)
  const minutes = totalMinutes % 60
  return `${hours}h ${minutes.toString().padStart(2, '0')}m`
}

function clampProgress(value?: number) {
  if (typeof value !== 'number' || Number.isNaN(value)) return undefined
  return Math.max(0, Math.min(100, value))
}

function getVideoResultUrl(item: VideoGenerationItem) {
  if (item.previewUrl) return item.previewUrl
  if (item.taskId) return `/openai/v1/videos/${item.taskId}/content`
  return ''
}

function getVideoErrorMessage(error: unknown, fallback: string) {
  const responseData = (
    error as {
      response?: {
        data?: {
          error?: { message?: unknown } | string
          message?: unknown
        }
      }
    }
  )?.response?.data
  const errorPayload = responseData?.error
  if (
    errorPayload &&
    typeof errorPayload === 'object' &&
    typeof errorPayload.message === 'string' &&
    errorPayload.message.trim()
  ) {
    return errorPayload.message.trim()
  }
  if (typeof errorPayload === 'string' && errorPayload.trim()) {
    return errorPayload.trim()
  }
  if (
    typeof responseData?.message === 'string' &&
    responseData.message.trim()
  ) {
    return responseData.message.trim()
  }
  return error instanceof Error && error.message.trim()
    ? error.message
    : fallback
}

export function VideoWorkspace({
  params,
  model,
  group,
  models,
  groups,
  isLoadingModels = false,
  onModelChange,
  onGroupChange,
  onGeneratingChange,
}: VideoWorkspaceProps) {
  const { t } = useTranslation()
  const [items, setItems] = useState<VideoGenerationItem[]>([])
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [prompt, setPrompt] = useState('')
  const [submittingCount, setSubmittingCount] = useState(0)
  const composerRef = useRef<HTMLTextAreaElement | null>(null)
  const activeTaskCountRef = useRef(0)
  const runningTasks = useMemo(
    () =>
      items.filter(
        (item) => item.status === 'running' || item.status === 'queued'
      ),
    [items]
  )
  const activeTaskCount = runningTasks.length + submittingCount
  const isAtConcurrencyLimit = activeTaskCount >= MAX_CONCURRENT_VIDEO_TASKS
  const selectedItem = useMemo(
    () => items.find((item) => item.id === selectedId) ?? items[0] ?? null,
    [items, selectedId]
  )
  activeTaskCountRef.current = activeTaskCount

  const reserveVideoTaskSlot = () => {
    if (activeTaskCountRef.current >= MAX_CONCURRENT_VIDEO_TASKS) {
      return false
    }
    activeTaskCountRef.current += 1
    setSubmittingCount((count) => count + 1)
    return true
  }

  useEffect(() => {
    onGeneratingChange?.(activeTaskCount > 0)
  }, [activeTaskCount, onGeneratingChange])

  const pollTask = useCallback(
    async (localId: string, taskId: string, startedAt: number) => {
      try {
        const result = await fetchVideoTask(taskId)
        const nextStatus = statusFromTask(result.status)
        const progress = clampProgress(result.progress)
        const previewUrl =
          result.preview_url ||
          result.url ||
          result.result_url ||
          result.content_url ||
          result.video?.url ||
          `/openai/v1/videos/${taskId}/content`

        setItems((prev) =>
          prev.map((item) =>
            item.id === localId
              ? {
                  ...item,
                  status: nextStatus,
                  progress:
                    nextStatus === 'succeeded'
                      ? 100
                      : (progress ?? item.progress),
                  previewUrl:
                    nextStatus === 'succeeded'
                      ? previewUrl
                      : previewUrl || item.previewUrl,
                  errorMessage:
                    nextStatus === 'failed'
                      ? result.error?.message || t('Video generation failed')
                      : item.errorMessage,
                  durationMs:
                    nextStatus === 'succeeded' || nextStatus === 'failed'
                      ? Math.max(0, Date.now() - startedAt)
                      : item.durationMs,
                  updatedAt: Date.now(),
                }
              : item
          )
        )
      } catch {
        // Keep polling. Short network hiccups should not mark a long render failed.
      }
    },
    [t]
  )

  useEffect(() => {
    if (runningTasks.length === 0) return
    const timer = window.setInterval(() => {
      for (const item of runningTasks) {
        if (!item.taskId) continue
        void pollTask(item.id, item.taskId, item.createdAt)
      }
    }, VIDEO_POLL_INTERVAL_MS)
    return () => window.clearInterval(timer)
  }, [pollTask, runningTasks])

  const runGeneration = async ({
    itemId,
    createdAt,
    text,
    generationParams,
    referenceImages,
    targetModel,
    targetGroup,
  }: {
    itemId: string
    createdAt: number
    text: string
    generationParams: VideoGenerationParams
    referenceImages: MessageAttachment[]
    targetModel: string
    targetGroup: string
  }) => {
    try {
      const response = await submitVideoTask({
        model: targetModel,
        group: targetGroup,
        prompt: text,
        duration_sec: generationParams.durationSec,
        aspect_ratio: generationParams.aspectRatio,
        resolution: generationParams.resolution,
        seed: generationParams.seed,
        reference_images: referenceImages,
      })
      const taskId = response.id || response.task_id || response.request_id
      if (!taskId) {
        throw new Error(t('Backend did not return a task id'))
      }
      const nextStatus = statusFromTask(response.status)
      const previewUrl =
        response.preview_url ||
        response.url ||
        response.result_url ||
        response.content_url ||
        response.video?.url

      setItems((prev) =>
        prev.map((item) =>
          item.id === itemId
            ? {
                ...item,
                taskId,
                status: nextStatus,
                progress:
                  nextStatus === 'succeeded'
                    ? 100
                    : (clampProgress(response.progress) ?? item.progress),
                previewUrl:
                  nextStatus === 'succeeded'
                    ? previewUrl || `/openai/v1/videos/${taskId}/content`
                    : previewUrl || item.previewUrl,
                durationMs:
                  nextStatus === 'succeeded' || nextStatus === 'failed'
                    ? Math.max(0, Date.now() - createdAt)
                    : item.durationMs,
                updatedAt: Date.now(),
              }
            : item
        )
      )

      if (nextStatus !== 'succeeded' && nextStatus !== 'failed') {
        void pollTask(itemId, taskId, createdAt)
      }
    } catch (error) {
      const message = getVideoErrorMessage(error, t('Video generation failed'))
      setItems((prev) =>
        prev.map((item) =>
          item.id === itemId
            ? {
                ...item,
                status: 'failed',
                progress: 0,
                errorMessage: message,
                durationMs: Math.max(0, Date.now() - createdAt),
                updatedAt: Date.now(),
              }
            : item
        )
      )
      toast.error(message)
    }
  }

  const handleSubmit = async (message: PromptInputMessage) => {
    const text = (message.text ?? prompt).trim()
    if (!text) {
      toast.info(t('Please describe the video you want to create'))
      composerRef.current?.focus()
      return
    }
    if (isAtConcurrencyLimit) {
      toast.info(
        t('最多同时运行 {{count}} 个视频任务', {
          count: MAX_CONCURRENT_VIDEO_TASKS,
        })
      )
      return
    }
    if (!reserveVideoTaskSlot()) {
      toast.info(
        t('最多同时运行 {{count}} 个视频任务', {
          count: MAX_CONCURRENT_VIDEO_TASKS,
        })
      )
      return
    }

    let submittingReleased = false
    let slotTransferredToTask = false
    const releaseSubmittingSlot = () => {
      if (submittingReleased) return
      submittingReleased = true
      if (!slotTransferredToTask) {
        activeTaskCountRef.current = Math.max(0, activeTaskCountRef.current - 1)
      }
      setSubmittingCount((count) => Math.max(0, count - 1))
    }

    try {
      const imageFiles = (message.files ?? []).filter(
        (file) => file.file && file.mediaType?.startsWith('image/')
      )
      const referenceImages: MessageAttachment[] = await Promise.all(
        imageFiles.map(async (inputFile) => {
          const file = inputFile.file
          if (!file) throw new Error(t('参考图读取失败'))
          const uploaded = await uploadPlaygroundAttachment(file)
          return {
            id: uploaded.id,
            file_id: uploaded.file_id || uploaded.id,
            type: 'image',
            url: uploaded.url,
            mediaType: uploaded.media_type || inputFile.mediaType,
            filename: uploaded.filename || inputFile.filename,
            size: uploaded.size,
          }
        })
      )

      const itemId = generateId()
      const createdAt = Date.now()
      const newItem: VideoGenerationItem = {
        id: itemId,
        prompt: text,
        params,
        model,
        group,
        referenceImages,
        status: 'queued',
        progress: 0,
        createdAt,
        updatedAt: createdAt,
      }
      setItems((prev) => [newItem, ...prev])
      setSelectedId(itemId)
      slotTransferredToTask = true
      setPrompt('')
      const generationPromise = runGeneration({
        itemId,
        createdAt,
        text,
        generationParams: params,
        referenceImages,
        targetModel: model,
        targetGroup: group,
      })
      releaseSubmittingSlot()
      await generationPromise
    } catch (error) {
      toast.error(getVideoErrorMessage(error, t('Video generation failed')))
    } finally {
      releaseSubmittingSlot()
    }
  }

  const handleRetry = async (item: VideoGenerationItem) => {
    if (isAtConcurrencyLimit) {
      toast.info(
        t('最多同时运行 {{count}} 个视频任务', {
          count: MAX_CONCURRENT_VIDEO_TASKS,
        })
      )
      return
    }
    if (!reserveVideoTaskSlot()) {
      return
    }
    let submittingReleased = false
    let slotTransferredToTask = false
    const releaseSubmittingSlot = () => {
      if (submittingReleased) return
      submittingReleased = true
      if (!slotTransferredToTask) {
        activeTaskCountRef.current = Math.max(0, activeTaskCountRef.current - 1)
      }
      setSubmittingCount((count) => Math.max(0, count - 1))
    }

    try {
      setItems((prev) =>
        prev.map((candidate) =>
          candidate.id === item.id
            ? {
                ...candidate,
                taskId: undefined,
                createdAt: Date.now(),
                status: 'queued',
                progress: 0,
                errorMessage: undefined,
                previewUrl: undefined,
                durationMs: undefined,
                updatedAt: Date.now(),
              }
            : candidate
        )
      )
      slotTransferredToTask = true
      releaseSubmittingSlot()
      await runGeneration({
        itemId: item.id,
        createdAt: Date.now(),
        text: item.prompt,
        generationParams: item.params,
        referenceImages: item.referenceImages ?? [],
        targetModel: item.model || model,
        targetGroup: item.group || group,
      })
    } finally {
      releaseSubmittingSlot()
    }
  }

  const handleDelete = (item: VideoGenerationItem) => {
    setItems((prev) => prev.filter((candidate) => candidate.id !== item.id))
    setSelectedId((current) => (current === item.id ? null : current))
  }

  const handleCopyPrompt = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text)
      toast.success(t('提示词已复制'))
    } catch {
      toast.error(t('复制失败'))
    }
  }

  const handleDownload = async (item: VideoGenerationItem) => {
    const url = getVideoResultUrl(item)
    if (!url) return
    try {
      const blob = await fetchAuthenticatedBlob(url)
      const objectUrl = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = objectUrl
      anchor.download = `sunapi-video-${item.taskId || item.id}.mp4`
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
      URL.revokeObjectURL(objectUrl)
    } catch {
      window.open(url, '_blank')
    }
  }

  return (
    <div className='bg-background flex size-full min-h-0 flex-col overflow-hidden'>
      <main className='flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden'>
        <VideoStage
          selectedItem={selectedItem}
          items={items}
          selectedId={selectedItem?.id ?? null}
          onSelect={setSelectedId}
          onRetry={handleRetry}
          onDelete={handleDelete}
          onCopyPrompt={handleCopyPrompt}
          onDownload={handleDownload}
        />
      </main>

      <div className='bg-background shrink-0 px-3 pt-2 pb-3 md:px-6'>
        <div className='mx-auto w-full max-w-3xl'>
          <VideoComposer
            ref={composerRef}
            value={prompt}
            onChange={setPrompt}
            onSubmit={handleSubmit}
            isGenerating={isAtConcurrencyLimit}
            activeTaskCount={activeTaskCount}
            maxConcurrentTasks={MAX_CONCURRENT_VIDEO_TASKS}
            model={model}
            models={models}
            group={group}
            groups={groups}
            isModelLoading={isLoadingModels}
            onModelChange={onModelChange}
            onGroupChange={onGroupChange}
          />
        </div>
      </div>
    </div>
  )
}

export function VideoNotice() {
  const { t } = useTranslation()
  return (
    <div className='flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/5 p-3 text-[11px] leading-relaxed text-amber-700 dark:text-amber-300'>
      <FilmIcon className='mt-0.5 size-3.5 shrink-0' />
      <div>
        <div className='mb-0.5 font-medium'>{t('Video generation queue')}</div>
        <div>
          {t(
            'Submit a prompt to create a queued video task. The result will refresh automatically and remain available while the page is open.'
          )}
        </div>
      </div>
    </div>
  )
}

interface VideoParamsPanelProps {
  value: VideoGenerationParams
  onChange: (next: VideoGenerationParams) => void
  disabled?: boolean
  compact?: boolean
}

export function VideoParamsPanel({
  value,
  onChange,
  disabled,
  compact = false,
}: VideoParamsPanelProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(true)

  const update = <K extends keyof VideoGenerationParams>(
    key: K,
    next: VideoGenerationParams[K]
  ) => onChange({ ...value, [key]: next })

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className='border-border/60 bg-card/40 rounded-lg border'
    >
      <CollapsibleTrigger
        render={
          <button
            type='button'
            disabled={disabled}
            className='hover:bg-accent/40 flex w-full items-center justify-between gap-2 rounded-t-lg px-3 py-2 text-left transition-colors'
          />
        }
      >
        <span className='flex items-center gap-2 text-xs font-medium'>
          <Settings2Icon className='text-muted-foreground size-3.5' />
          {t('Video parameters')}
        </span>
        <ChevronDownIcon
          className={cn(
            'text-muted-foreground size-3.5 transition-transform',
            open && 'rotate-180'
          )}
        />
      </CollapsibleTrigger>
      <CollapsibleContent className='border-border/60 space-y-4 border-t p-3'>
        <div
          className={cn('grid gap-3', compact ? 'grid-cols-1' : 'grid-cols-2')}
        >
          <Field label={t('Aspect ratio')}>
            <Select
              value={value.aspectRatio}
              onValueChange={(v) =>
                update('aspectRatio', v as VideoAspectRatio)
              }
              disabled={disabled}
            >
              <SelectTrigger className='h-8 w-full text-xs'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {ASPECT_RATIOS.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    <span className='flex flex-col'>
                      <span className='text-xs'>{opt.label}</span>
                      <span className='text-muted-foreground text-[10px]'>
                        {t(opt.hintKey)}
                      </span>
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>

          <Field label={t('Resolution')}>
            <Select
              value={value.resolution}
              onValueChange={(v) =>
                update('resolution', v as VideoResolutionPreset)
              }
              disabled={disabled}
            >
              <SelectTrigger className='h-8 w-full text-xs'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {RESOLUTION_OPTIONS.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    <span className='flex flex-col'>
                      <span className='text-xs'>{opt.label}</span>
                      <span className='text-muted-foreground text-[10px]'>
                        {t(opt.hintKey)}
                      </span>
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>

          <Field label={t('Duration (sec)')}>
            <Select
              value={String(value.durationSec)}
              onValueChange={(v) => update('durationSec', Number(v))}
              disabled={disabled}
            >
              <SelectTrigger className='h-8 w-full text-xs'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {DURATIONS.map((d) => (
                  <SelectItem key={d} value={String(d)}>
                    {d}s
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>

          <Field label={t('Seed (optional)')}>
            <div
              className={cn(
                'flex gap-2',
                compact ? 'flex-col' : 'items-center'
              )}
            >
              <Input
                type='number'
                value={value.seed ?? ''}
                onChange={(event) => {
                  const next = event.target.value
                  update('seed', next === '' ? null : Number(next))
                }}
                placeholder={t('Random')}
                className='h-8 text-xs'
                disabled={disabled}
              />
              <Button
                type='button'
                variant='outline'
                size='sm'
                className={cn('gap-1.5 text-xs', compact && 'w-full')}
                onClick={() => update('seed', Math.floor(Math.random() * 1e9))}
                disabled={disabled}
              >
                <SparklesIcon className='size-3' />
                {t('Random')}
              </Button>
            </div>
          </Field>
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

export function VideoPromptTips() {
  const { t } = useTranslation()
  return (
    <div className='border-border/60 bg-card/40 text-muted-foreground rounded-lg border p-3 text-[11px] leading-relaxed'>
      <div className='text-foreground mb-1.5 flex items-center gap-1.5 text-xs font-medium'>
        <WandSparklesIcon className='size-3.5' />
        {t('Prompt tips')}
      </div>
      <ul className='list-disc space-y-1 pl-4'>
        <li>
          {t(
            'Describe camera motion, subject action, setting, mood, and lighting.'
          )}
        </li>
        <li>
          {t(
            'Use a reference image when you need the first frame or character to stay consistent.'
          )}
        </li>
        <li>
          {t('Choose 480p for quick tests, then 720p for final previews.')}
        </li>
        <li>
          {t(
            'Keep prompts concrete; short videos benefit from one clear action.'
          )}
        </li>
      </ul>
    </div>
  )
}

export function useVideoDefaultParams(): VideoGenerationParams {
  return {
    durationSec: 5,
    aspectRatio: '16:9',
    resolution: '720p',
    seed: null,
  }
}

interface VideoComposerProps {
  value: string
  onChange: (value: string) => void
  onSubmit: (message: PromptInputMessage) => void | Promise<void>
  isGenerating: boolean
  activeTaskCount: number
  maxConcurrentTasks: number
  model: string
  models: ModelOption[]
  group: string
  groups: GroupOption[]
  isModelLoading?: boolean
  onModelChange: (value: string) => void
  onGroupChange: (value: string) => void
}

const VideoComposer = ({
  value,
  onChange,
  onSubmit,
  isGenerating,
  activeTaskCount,
  maxConcurrentTasks,
  model,
  models,
  group,
  groups,
  isModelLoading = false,
  onModelChange,
  onGroupChange,
  ref,
}: VideoComposerProps & { ref?: React.Ref<HTMLTextAreaElement> }) => {
  const { t } = useTranslation()
  const hasVideoModel = models.length > 0

  const handleSubmit = (message: PromptInputMessage) => {
    if (!message.text?.trim() || isGenerating || !hasVideoModel) return
    return onSubmit(message)
  }

  return (
    <PromptInput
      accept='image/*'
      globalDrop
      groupClassName='rounded-xl'
      maxFileSize={12 * 1024 * 1024}
      maxFiles={7}
      multiple
      onError={(error) => {
        const message =
          error.code === 'max_files'
            ? t('最多只能添加 7 张视频参考图')
            : error.code === 'max_file_size'
              ? t('参考图不能超过 12MB')
              : t('只能添加图片作为视频参考图')
        toast.error(message)
      }}
      onSubmit={handleSubmit}
    >
      <VideoReferencePreview />

      <PromptInputTextarea
        ref={ref}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={t(
          'Describe the video scene, camera motion, and action...'
        )}
        disabled={isGenerating}
        className='px-5 md:text-base'
      />

      <PromptInputFooter className='p-2.5'>
        <div className='text-muted-foreground inline-flex items-center gap-1.5 px-1.5 text-xs'>
          <FilmIcon className='size-3.5' />
          {hasVideoModel ? t('Video generation') : t('当前分组无视频模型')}
          {hasVideoModel && activeTaskCount > 0 && (
            <span className='bg-muted text-muted-foreground rounded px-1 py-0.5 text-[10px]'>
              {activeTaskCount}/{maxConcurrentTasks}
            </span>
          )}
        </div>
        <div className='flex min-w-0 flex-wrap items-center justify-end gap-1.5'>
          <ModelCapsule
            model={model}
            models={models}
            group={group}
            groups={groups}
            onModelChange={onModelChange}
            onGroupChange={onGroupChange}
            disabled={isGenerating}
            isLoading={isModelLoading}
          />
          <VideoReferenceButton disabled={isGenerating || !hasVideoModel} />
          <PromptInputButton
            type='submit'
            disabled={!value.trim() || isGenerating || !hasVideoModel}
            variant='secondary'
            className='text-foreground font-medium'
          >
            <SendIcon size={16} />
            <span className='hidden sm:inline'>{t('生成')}</span>
            <span className='sr-only sm:hidden'>{t('生成')}</span>
          </PromptInputButton>
        </div>
      </PromptInputFooter>
    </PromptInput>
  )
}

function VideoReferencePreview() {
  const attachments = usePromptInputAttachments()
  const imageAttachments = attachments.files.filter((file) =>
    file.mediaType?.startsWith('image/')
  )

  if (imageAttachments.length === 0) return null

  return (
    <PromptInputHeader className='px-3 pt-3 pb-0'>
      {imageAttachments.map((attachment) => (
        <PromptInputAttachment
          className='max-w-36'
          data={attachment}
          key={attachment.id}
        />
      ))}
    </PromptInputHeader>
  )
}

function VideoReferenceButton({ disabled }: { disabled?: boolean }) {
  const { t } = useTranslation()
  const attachments = usePromptInputAttachments()

  return (
    <Button
      type='button'
      variant='outline'
      size='sm'
      onClick={() => attachments.openFileDialog()}
      disabled={disabled}
      className='h-8 gap-1.5 text-xs shadow-none'
      title={t('参考图')}
    >
      <PaperclipIcon className='size-3.5' />
      <span className='hidden sm:inline'>{t('参考图')}</span>
    </Button>
  )
}

interface VideoStageProps {
  selectedItem: VideoGenerationItem | null
  items: VideoGenerationItem[]
  selectedId: string | null
  onSelect: (id: string) => void
  onRetry: (item: VideoGenerationItem) => void
  onDelete: (item: VideoGenerationItem) => void
  onCopyPrompt: (text: string) => void
  onDownload: (item: VideoGenerationItem) => void
}

function VideoStage({
  selectedItem,
  items,
  selectedId,
  onSelect,
  onRetry,
  onDelete,
  onCopyPrompt,
  onDownload,
}: VideoStageProps) {
  const { t } = useTranslation()

  if (!selectedItem) {
    return (
      <div className='grid size-full place-items-center px-4 py-12'>
        <div className='text-muted-foreground flex max-w-md flex-col items-center gap-3 text-center'>
          <div className='border-border/70 bg-muted/30 flex aspect-video w-full max-w-sm items-center justify-center rounded-xl border-2 border-dashed'>
            <div className='flex flex-col items-center gap-2'>
              <ImagePlusIcon className='size-6' />
              <span className='text-xs'>{t('暂无视频任务')}</span>
            </div>
          </div>
          <div className='text-xs leading-relaxed'>
            {t('在下方输入提示词，生成任务会进入队列并在这里展示进度。')}
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className='flex size-full min-h-0 flex-col overflow-y-auto px-3 py-4 md:px-6 md:py-6'>
      <div className='mx-auto flex w-full max-w-7xl flex-1 flex-col gap-5 pb-28 lg:grid lg:grid-cols-[minmax(0,1fr)_22rem]'>
        <PreviewPanel
          item={selectedItem}
          onRetry={onRetry}
          onCopyPrompt={onCopyPrompt}
          onDownload={onDownload}
        />
        <aside className='min-h-0 space-y-3'>
          <div className='text-muted-foreground flex items-center justify-between text-xs'>
            <span>{t('Task queue')}</span>
            <span>{t('{{count}} item(s)', { count: items.length })}</span>
          </div>
          <div className='grid gap-3'>
            {items.map((item) => (
              <VideoQueueCard
                item={item}
                key={item.id}
                isActive={item.id === selectedId}
                onSelect={onSelect}
                onRetry={onRetry}
                onDelete={onDelete}
                onCopyPrompt={onCopyPrompt}
                onDownload={onDownload}
              />
            ))}
          </div>
        </aside>
      </div>
    </div>
  )
}

function PreviewPanel({
  item,
  onRetry,
  onCopyPrompt,
  onDownload,
}: {
  item: VideoGenerationItem
  onRetry: (item: VideoGenerationItem) => void
  onCopyPrompt: (text: string) => void
  onDownload: (item: VideoGenerationItem) => void
}) {
  const { t } = useTranslation()
  const url = getVideoResultUrl(item)

  return (
    <div className='border-border/60 bg-card/60 flex min-h-[28rem] min-w-0 flex-col overflow-hidden rounded-xl border shadow-sm'>
      <div className='bg-muted/25 relative flex min-h-0 flex-1 items-center justify-center overflow-hidden'>
        {item.status === 'succeeded' && url ? (
          <video
            controls
            src={url}
            className='max-h-full max-w-full rounded-md'
          />
        ) : item.status === 'failed' ? (
          <div className='text-muted-foreground flex max-w-md flex-col items-center gap-3 px-6 text-center'>
            <AlertCircleIcon className='text-destructive size-8' />
            <div className='text-sm font-medium'>
              {t('Video generation failed')}
            </div>
            <div className='line-clamp-4 text-xs leading-relaxed'>
              {item.errorMessage || t('Generation failed')}
            </div>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => onRetry(item)}
              className='gap-1.5'
            >
              <RotateCcwIcon className='size-3.5' />
              {t('Retry')}
            </Button>
          </div>
        ) : (
          <div className='flex w-full max-w-md flex-col items-center gap-4 px-8 text-center'>
            <Skeleton className='aspect-video w-full rounded-xl' />
            <div className='text-muted-foreground flex flex-col items-center gap-2 text-xs'>
              <ClockIcon className='size-4' />
              <span>
                {item.status === 'queued' ? t('Queued') : t('Rendering...')}
              </span>
              <Progress value={item.progress ?? 12} className='w-56' />
            </div>
          </div>
        )}
      </div>
      <div className='border-border/60 border-t px-4 py-3'>
        <div className='flex min-w-0 items-start justify-between gap-3'>
          <div className='min-w-0'>
            <div className='text-foreground line-clamp-2 text-sm font-medium'>
              {item.prompt}
            </div>
            <VideoMeta item={item} className='mt-2' />
          </div>
          <div className='flex shrink-0 items-center gap-1.5'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => onCopyPrompt(item.prompt)}
              className='h-8 gap-1.5 text-xs'
            >
              <CopyIcon className='size-3.5' />
              {t('Copy')}
            </Button>
            {item.status === 'succeeded' && url && (
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() => onDownload(item)}
                className='h-8 gap-1.5 text-xs'
              >
                <DownloadIcon className='size-3.5' />
                {t('Download')}
              </Button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

function VideoQueueCard({
  item,
  isActive,
  onSelect,
  onRetry,
  onDelete,
  onCopyPrompt,
  onDownload,
}: {
  item: VideoGenerationItem
  isActive: boolean
  onSelect: (id: string) => void
  onRetry: (item: VideoGenerationItem) => void
  onDelete: (item: VideoGenerationItem) => void
  onCopyPrompt: (text: string) => void
  onDownload: (item: VideoGenerationItem) => void
}) {
  const { t } = useTranslation()
  const [now, setNow] = useState(() => Date.now())
  const durationMs = generationDurationMs(item, now)
  const durationText =
    typeof durationMs === 'number' ? formatGenerationDuration(durationMs) : null
  const url = getVideoResultUrl(item)

  useEffect(() => {
    if (item.status !== 'running' && item.status !== 'queued') return
    setNow(Date.now())
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [item.id, item.status])

  return (
    <div
      className={cn(
        'border-border/60 bg-card/60 group/card overflow-hidden rounded-lg border shadow-sm transition-shadow hover:shadow-md',
        isActive && 'border-amber-500/40 ring-2 ring-amber-500/15'
      )}
      onClick={() => onSelect(item.id)}
      role='button'
      tabIndex={0}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          onSelect(item.id)
        }
      }}
    >
      <div className='bg-muted/25 relative aspect-video w-full overflow-hidden'>
        <div className='absolute top-2 right-2 z-10'>
          <DropdownMenu>
            <DropdownMenuTrigger
              onClick={(event) => event.stopPropagation()}
              className='bg-background/90 text-muted-foreground hover:text-foreground inline-flex size-8 items-center justify-center rounded-md border opacity-0 shadow-sm backdrop-blur transition-opacity group-hover/card:opacity-100 focus-visible:opacity-100 data-[popup-open]:opacity-100'
              aria-label={t('Video actions')}
            >
              <MoreHorizontalIcon className='size-4' />
            </DropdownMenuTrigger>
            <DropdownMenuContent align='end' sideOffset={6} className='w-36'>
              <DropdownMenuItem
                onClick={() => onCopyPrompt(item.prompt)}
                className='gap-2'
              >
                <CopyIcon className='size-3.5' />
                {t('复制提示词')}
              </DropdownMenuItem>
              {item.status === 'succeeded' && url && (
                <DropdownMenuItem
                  onClick={() => onDownload(item)}
                  className='gap-2'
                >
                  <DownloadIcon className='size-3.5' />
                  {t('下载')}
                </DropdownMenuItem>
              )}
              {item.status === 'failed' && (
                <DropdownMenuItem
                  onClick={() => onRetry(item)}
                  className='gap-2'
                >
                  <RotateCcwIcon className='size-3.5' />
                  {t('重试')}
                </DropdownMenuItem>
              )}
              <DropdownMenuSeparator />
              <DropdownMenuItem
                variant='destructive'
                onClick={() => onDelete(item)}
                className='gap-2'
              >
                <Trash2Icon className='size-3.5' />
                {t('删除')}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        {item.status === 'succeeded' && url ? (
          <video
            muted
            playsInline
            src={url}
            className='size-full object-cover'
          />
        ) : item.status === 'failed' ? (
          <div className='text-muted-foreground flex size-full flex-col items-center justify-center gap-2 px-5 text-center'>
            <AlertCircleIcon className='text-destructive size-6' />
            <span className='line-clamp-3 text-xs leading-relaxed'>
              {item.errorMessage || t('Generation failed')}
            </span>
          </div>
        ) : (
          <div className='absolute inset-0 flex items-center justify-center'>
            <div className='flex w-4/5 flex-col items-center gap-2'>
              <PlayCircleIcon className='text-muted-foreground size-7' />
              <span className='text-muted-foreground inline-flex items-center gap-1 text-[11px]'>
                <ClockIcon className='size-3' />
                {durationText
                  ? `${item.status === 'queued' ? t('Queued') : t('Rendering...')} ${durationText}`
                  : item.status === 'queued'
                    ? t('Queued')
                    : t('Rendering...')}
              </span>
              <Progress
                value={item.progress ?? 12}
                className='w-full max-w-44'
              />
            </div>
          </div>
        )}
      </div>
      <div className='flex min-h-24 flex-col gap-2 px-3 pt-2.5 pb-3'>
        <div className='text-foreground line-clamp-2 text-sm leading-snug font-medium'>
          {item.prompt}
        </div>
        <VideoMeta item={item} durationText={durationText} />
      </div>
    </div>
  )
}

function VideoMeta({
  item,
  durationText,
  className,
}: {
  item: VideoGenerationItem
  durationText?: string | null
  className?: string
}) {
  const { t } = useTranslation()

  return (
    <div
      className={cn(
        'text-muted-foreground flex min-w-0 flex-wrap items-center gap-1.5 text-[10px]',
        className
      )}
    >
      <span
        className='bg-muted max-w-[8rem] truncate rounded px-1.5 py-0.5'
        title={item.model}
      >
        {item.model}
      </span>
      {item.group && item.group !== 'default' && (
        <span
          className='bg-muted max-w-[5.5rem] truncate rounded px-1.5 py-0.5'
          title={item.group}
        >
          {item.group}
        </span>
      )}
      <span className='bg-muted shrink-0 rounded px-1.5 py-0.5'>
        {item.params.aspectRatio}
      </span>
      <span className='bg-muted shrink-0 rounded px-1.5 py-0.5'>
        {item.params.resolution}
      </span>
      <span className='bg-muted shrink-0 rounded px-1.5 py-0.5'>
        {item.params.durationSec}s
      </span>
      {item.referenceImages && item.referenceImages.length > 0 && (
        <span className='bg-muted shrink-0 rounded px-1.5 py-0.5'>
          {t('Reference images {{count}}', {
            count: item.referenceImages.length,
          })}
        </span>
      )}
      {durationText && (
        <span className='bg-muted inline-flex shrink-0 items-center gap-0.5 rounded px-1.5 py-0.5'>
          <ClockIcon className='size-2.5' />
          {t('Duration')} {durationText}
        </span>
      )}
    </div>
  )
}

function Field({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className='space-y-1.5'>
      <Label className='text-muted-foreground text-[10px] font-medium tracking-wider uppercase'>
        {label}
      </Label>
      {children}
    </div>
  )
}
