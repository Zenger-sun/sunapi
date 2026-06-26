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
import { useState, useRef, useEffect, useCallback } from 'react'
import {
  SendIcon,
  ImageIcon,
  PaperclipIcon,
  AlertCircleIcon,
  ClockIcon,
  DownloadIcon,
  CopyIcon,
  ImagePlusIcon,
  RotateCcwIcon,
  MoreHorizontalIcon,
  Trash2Icon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  fetchAuthenticatedBlob,
  useAuthenticatedImageSource,
} from '@/lib/authenticated-media'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Lightbox } from '@/components/ui/lightbox'
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
  deleteImageGenerationHistory,
  getImageGenerationHistory,
  startImageGenerationTask,
  uploadPlaygroundAttachment,
} from '../api'
import type {
  GroupOption,
  ImageGenerationItem,
  ImageGenerationParams,
  MessageAttachment,
  ModelOption,
} from '../types'
import { ModelCapsule } from './model-capsule'

interface ImageWorkspaceProps {
  params: ImageGenerationParams
  model: string
  group: string
  models: ModelOption[]
  groups: GroupOption[]
  isLoadingModels?: boolean
  onModelChange: (value: string) => void
  onGroupChange: (value: string) => void
  onGeneratingChange?: (generating: boolean) => void
  isLocked?: boolean
  onUnlock?: () => void
}

const MAX_CONCURRENT_IMAGE_TASKS = 5

function generateId() {
  return Math.random().toString(36).slice(2, 10) + Date.now().toString(36)
}

function generationDurationMs(item: ImageGenerationItem, now: number) {
  if (typeof item.durationMs === 'number' && item.durationMs >= 0) {
    return item.durationMs
  }
  if (item.status === 'running') {
    return Math.max(0, now - item.createdAt)
  }
  if (item.updatedAt && item.createdAt) {
    return Math.max(0, item.updatedAt - item.createdAt)
  }
  return null
}

function formatGenerationDuration(ms: number) {
  const totalSeconds = Math.max(0, Math.floor(ms / 1000))
  if (totalSeconds < 60) {
    return `${totalSeconds}s`
  }
  const totalMinutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  if (totalMinutes < 60) {
    return `${totalMinutes}m ${seconds.toString().padStart(2, '0')}s`
  }
  const hours = Math.floor(totalMinutes / 60)
  const minutes = totalMinutes % 60
  return `${hours}h ${minutes.toString().padStart(2, '0')}m`
}

function useImageStore() {
  const [items, setItems] = useState<ImageGenerationItem[]>([])
  const [submittingCount, setSubmittingCount] = useState(0)
  return { items, setItems, submittingCount, setSubmittingCount }
}

function mergeImageHistory(
  current: ImageGenerationItem[],
  history: ImageGenerationItem[]
) {
  const merged = new Map(current.map((item) => [item.id, item]))
  for (const item of history) {
    const existing = merged.get(item.id)
    const existingTime = existing?.updatedAt ?? existing?.createdAt ?? 0
    const itemTime = item.updatedAt ?? item.createdAt ?? 0
    if (!existing || itemTime >= existingTime) {
      merged.set(item.id, { ...existing, ...item })
    }
  }
  return Array.from(merged.values()).sort((a, b) => b.createdAt - a.createdAt)
}

function getImageErrorMessage(error: unknown, fallback: string) {
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

export function ImageWorkspace({
  params,
  model,
  group,
  models,
  groups,
  isLoadingModels = false,
  onModelChange,
  onGroupChange,
  onGeneratingChange,
  isLocked,
  onUnlock,
}: ImageWorkspaceProps) {
  const { t } = useTranslation()
  const { items, setItems, submittingCount, setSubmittingCount } =
    useImageStore()
  const [prompt, setPrompt] = useState('')
  const [lightboxSrc, setLightboxSrc] = useState<string | null>(null)
  const composerRef = useRef<HTMLTextAreaElement | null>(null)
  const activeTaskCountRef = useRef(0)
  const runningTasksCount = items.filter(
    (item) => item.status === 'running'
  ).length
  const activeTaskCount = runningTasksCount + submittingCount
  activeTaskCountRef.current = activeTaskCount
  const hasRunningTasks = runningTasksCount > 0
  const isAtConcurrencyLimit = activeTaskCount >= MAX_CONCURRENT_IMAGE_TASKS

  const reserveImageTaskSlot = () => {
    if (activeTaskCountRef.current >= MAX_CONCURRENT_IMAGE_TASKS) {
      return false
    }
    activeTaskCountRef.current += 1
    setSubmittingCount((count) => count + 1)
    return true
  }

  const refreshHistory = useCallback(async () => {
    const history = await getImageGenerationHistory()
    if (history.length === 0) return
    setItems((prev) => mergeImageHistory(prev, history))
  }, [setItems])

  useEffect(() => {
    onGeneratingChange?.(activeTaskCount > 0)
  }, [activeTaskCount, onGeneratingChange])

  useEffect(() => {
    void refreshHistory().catch(() => undefined)
  }, [refreshHistory])

  useEffect(() => {
    if (!hasRunningTasks) return
    const timer = window.setInterval(() => {
      void refreshHistory().catch(() => undefined)
    }, 3000)
    return () => window.clearInterval(timer)
  }, [hasRunningTasks, refreshHistory])

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
    generationParams: ImageGenerationParams
    referenceImages: MessageAttachment[]
    targetModel: string
    targetGroup: string
  }) => {
    const taskItem: ImageGenerationItem = {
      id: itemId,
      prompt: text,
      negativePrompt: generationParams.negativePrompt || undefined,
      params: generationParams,
      model: targetModel,
      group: targetGroup,
      referenceImages,
      urls: [],
      status: 'running',
      errorMessage: undefined,
      durationMs: undefined,
      createdAt,
      updatedAt: Date.now(),
    }
    setItems((prev) =>
      prev.map((item) => (item.id === itemId ? { ...item, ...taskItem } : item))
    )
    try {
      const savedItem = await startImageGenerationTask(taskItem)
      setItems((prev) => mergeImageHistory(prev, [savedItem]))
      void refreshHistory().catch(() => undefined)
    } catch (error) {
      const messageText = getImageErrorMessage(
        error,
        t('\u56fe\u7247\u4efb\u52a1\u63d0\u4ea4\u5931\u8d25')
      )
      const failedItem: ImageGenerationItem = {
        ...taskItem,
        urls: [],
        status: 'failed',
        errorMessage: messageText,
        durationMs: Math.max(0, Date.now() - createdAt),
        updatedAt: Date.now(),
      }
      setItems((prev) =>
        prev.map((item) =>
          item.id === itemId ? { ...item, ...failedItem } : item
        )
      )
      toast.error(messageText)
    }
  }

  const handleSubmit = async (message: PromptInputMessage) => {
    const text = (message.text ?? prompt).trim()
    if (!text) {
      toast.info(
        t('\u8bf7\u63cf\u8ff0\u4f60\u60f3\u751f\u6210\u7684\u56fe\u7247')
      )
      composerRef.current?.focus()
      return
    }
    if (isAtConcurrencyLimit) {
      toast.info(
        t(
          '\u6700\u591a\u540c\u65f6\u8fd0\u884c {{count}} \u4e2a\u56fe\u7247\u4efb\u52a1',
          {
            count: MAX_CONCURRENT_IMAGE_TASKS,
          }
        )
      )
      return
    }

    if (!reserveImageTaskSlot()) {
      toast.info(
        t(
          '\u6700\u591a\u540c\u65f6\u8fd0\u884c {{count}} \u4e2a\u56fe\u7247\u4efb\u52a1',
          {
            count: MAX_CONCURRENT_IMAGE_TASKS,
          }
        )
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
          if (!file)
            throw new Error(t('\u53c2\u8003\u56fe\u8bfb\u53d6\u5931\u8d25'))
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
      const newItem: ImageGenerationItem = {
        id: itemId,
        prompt: text,
        negativePrompt: params.negativePrompt || undefined,
        params,
        model,
        group,
        referenceImages,
        urls: [],
        status: 'running',
        createdAt,
      }
      setItems((prev) => [newItem, ...prev])
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
      const messageText = getImageErrorMessage(
        error,
        t('\u56fe\u7247\u751f\u6210\u5931\u8d25')
      )
      toast.error(messageText)
    } finally {
      releaseSubmittingSlot()
    }
  }

  const handleRetry = async (item: ImageGenerationItem) => {
    if (isAtConcurrencyLimit) {
      toast.info(
        t(
          '\u6700\u591a\u540c\u65f6\u8fd0\u884c {{count}} \u4e2a\u56fe\u7247\u4efb\u52a1',
          {
            count: MAX_CONCURRENT_IMAGE_TASKS,
          }
        )
      )
      return
    }
    if (!reserveImageTaskSlot()) {
      toast.info(
        t(
          '\u6700\u591a\u540c\u65f6\u8fd0\u884c {{count}} \u4e2a\u56fe\u7247\u4efb\u52a1',
          {
            count: MAX_CONCURRENT_IMAGE_TASKS,
          }
        )
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
      const generationPromise = runGeneration({
        itemId: item.id,
        createdAt: item.createdAt,
        text: item.prompt,
        generationParams: item.params,
        referenceImages: item.referenceImages ?? [],
        targetModel: item.model || model,
        targetGroup: item.group || group,
      })
      slotTransferredToTask = true
      releaseSubmittingSlot()
      await generationPromise
    } finally {
      releaseSubmittingSlot()
    }
  }

  const handleDownload = async (url: string) => {
    try {
      const blob = await fetchAuthenticatedBlob(url)
      const objectUrl = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = objectUrl
      anchor.download = `sunapi-image-${Date.now()}.png`
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
      URL.revokeObjectURL(objectUrl)
    } catch {
      window.open(url, '_blank')
    }
  }

  const handleCopyPrompt = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text)
      toast.success(t('\u63d0\u793a\u8bcd\u5df2\u590d\u5236'))
    } catch {
      toast.error(t('\u590d\u5236\u5931\u8d25'))
    }
  }

  const handleDelete = async (item: ImageGenerationItem) => {
    const itemToRestore = item
    setItems((prev) => prev.filter((candidate) => candidate.id !== item.id))

    try {
      await deleteImageGenerationHistory(item.id)
      toast.success(t('\u5df2\u5220\u9664'))
    } catch (error) {
      setItems((prev) => {
        if (prev.some((candidate) => candidate.id === itemToRestore.id)) {
          return prev
        }
        return [itemToRestore, ...prev].sort(
          (a, b) => b.createdAt - a.createdAt
        )
      })
      toast.error(getImageErrorMessage(error, t('\u5220\u9664\u5931\u8d25')))
    }
  }

  return (
    <div className='bg-background flex size-full min-h-0 flex-col overflow-hidden'>
      <main className='flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden'>
        <Gallery
          items={items}
          onOpenLightbox={setLightboxSrc}
          onDownload={handleDownload}
          onCopyPrompt={handleCopyPrompt}
          onRetry={handleRetry}
          onDelete={handleDelete}
          onUnlock={onUnlock}
        />
      </main>

      <div className='bg-background shrink-0 px-3 pt-2 pb-3 md:px-6'>
        <div className='mx-auto w-full max-w-3xl'>
          <ImageComposer
            ref={composerRef}
            value={prompt}
            onChange={setPrompt}
            onSubmit={handleSubmit}
            isGenerating={isAtConcurrencyLimit}
            activeTaskCount={activeTaskCount}
            maxConcurrentTasks={MAX_CONCURRENT_IMAGE_TASKS}
            isLocked={isLocked}
            onUnlock={onUnlock}
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

      <Lightbox
        src={lightboxSrc}
        filename={t('sunapi-image.png')}
        onClose={() => setLightboxSrc(null)}
      />
    </div>
  )
}

interface ImageComposerProps {
  value: string
  onChange: (value: string) => void
  onSubmit: (message: PromptInputMessage) => void | Promise<void>
  isGenerating: boolean
  activeTaskCount: number
  maxConcurrentTasks: number
  isLocked?: boolean
  onUnlock?: () => void
  model: string
  models: ModelOption[]
  group: string
  groups: GroupOption[]
  isModelLoading?: boolean
  onModelChange: (value: string) => void
  onGroupChange: (value: string) => void
}

const ImageComposer = ({
  value,
  onChange,
  onSubmit,
  isGenerating,
  activeTaskCount,
  maxConcurrentTasks,
  isLocked,
  onUnlock,
  model,
  models,
  group,
  groups,
  isModelLoading = false,
  onModelChange,
  onGroupChange,
  ref,
}: ImageComposerProps & { ref?: React.Ref<HTMLTextAreaElement> }) => {
  const { t } = useTranslation()
  const isSelectorDisabled = isGenerating || isLocked
  const hasImageModel = models.length > 0

  const handleSubmit = (message: PromptInputMessage) => {
    if (!message.text?.trim() || isGenerating || isLocked || !hasImageModel)
      return
    return onSubmit(message)
  }

  return (
    <PromptInput
      accept='image/*'
      globalDrop
      groupClassName='rounded-xl'
      maxFileSize={12 * 1024 * 1024}
      maxFiles={4}
      multiple
      onError={(error) => {
        const message =
          error.code === 'max_files'
            ? t(
                '\u6700\u591a\u53ea\u80fd\u6dfb\u52a0 4 \u5f20\u53c2\u8003\u56fe'
              )
            : error.code === 'max_file_size'
              ? t('\u53c2\u8003\u56fe\u4e0d\u80fd\u8d85\u8fc7 12MB')
              : t(
                  '\u53ea\u80fd\u6dfb\u52a0\u56fe\u7247\u4f5c\u4e3a\u53c2\u8003\u56fe'
                )
        toast.error(message)
      }}
      onSubmit={handleSubmit}
    >
      <ImageReferencePreview />

      <PromptInputTextarea
        ref={ref}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={t(
          '\u63cf\u8ff0\u4f60\u60f3\u751f\u6210\u7684\u56fe\u7247...'
        )}
        disabled={isGenerating || isLocked}
        className='px-5 md:text-base'
      />

      <PromptInputFooter className='p-2.5'>
        <div className='text-muted-foreground inline-flex items-center gap-1.5 px-1.5 text-xs'>
          <ImageIcon className='size-3.5' />
          {hasImageModel
            ? t('\u56fe\u7247\u751f\u6210')
            : t('\u5f53\u524d\u5206\u7ec4\u65e0\u56fe\u7247\u6a21\u578b')}
          {hasImageModel && activeTaskCount > 0 && (
            <span className='bg-muted text-muted-foreground rounded px-1 py-0.5 text-[10px]'>
              {activeTaskCount}/{maxConcurrentTasks}
            </span>
          )}
          {isLocked && (
            <button
              type='button'
              onClick={onUnlock}
              className='text-muted-foreground hover:text-foreground ml-2 inline-flex items-center gap-1 text-[10px] underline-offset-2 hover:underline'
            >
              <AlertCircleIcon className='size-3' />
              {t(
                '\u540e\u7aef\u4e0d\u53ef\u7528\uff0c\u70b9\u51fb\u89e3\u9501\u9884\u89c8'
              )}
            </button>
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
            disabled={isGenerating || isLocked}
            isLoading={isModelLoading}
          />
          <ImageReferenceButton disabled={isSelectorDisabled} />
          <PromptInputButton
            type='submit'
            disabled={!value.trim() || isSelectorDisabled || !hasImageModel}
            variant='secondary'
            className='text-foreground font-medium'
          >
            <SendIcon size={16} />
            <span className='hidden sm:inline'>{t('\u751f\u6210')}</span>
            <span className='sr-only sm:hidden'>{t('\u751f\u6210')}</span>
          </PromptInputButton>
        </div>
      </PromptInputFooter>
    </PromptInput>
  )
}

function ImageReferencePreview() {
  const attachments = usePromptInputAttachments()
  const imageAttachments = attachments.files.filter((file) =>
    file.mediaType?.startsWith('image/')
  )

  if (imageAttachments.length === 0) {
    return null
  }

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

function ImageReferenceButton({ disabled }: { disabled?: boolean }) {
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
      title={t('\u53c2\u8003\u56fe')}
    >
      <PaperclipIcon className='size-3.5' />
      <span className='hidden sm:inline'>{t('\u53c2\u8003\u56fe')}</span>
    </Button>
  )
}

interface GalleryProps {
  items: ImageGenerationItem[]
  onOpenLightbox: (src: string) => void
  onDownload: (url: string) => void
  onCopyPrompt: (text: string) => void
  onRetry: (item: ImageGenerationItem) => void
  onDelete: (item: ImageGenerationItem) => void
  onUnlock?: () => void
}

function Gallery({
  items,
  onOpenLightbox,
  onDownload,
  onCopyPrompt,
  onRetry,
  onDelete,
}: GalleryProps) {
  const { t } = useTranslation()
  if (items.length === 0) {
    return (
      <div className='grid size-full place-items-center px-4 py-12'>
        <div className='text-muted-foreground flex max-w-md flex-col items-center gap-3 text-center'>
          <div className='border-border/70 bg-muted/30 flex aspect-[4/3] w-full max-w-sm items-center justify-center rounded-xl border-2 border-dashed'>
            <div className='flex flex-col items-center gap-2'>
              <ImagePlusIcon className='size-6' />
              <span className='text-xs'>{t('\u6682\u65e0\u56fe\u7247')}</span>
            </div>
          </div>
          <div className='text-xs leading-relaxed'>
            {t(
              '\u5728\u4e0b\u65b9\u8f93\u5165\u63d0\u793a\u8bcd\uff0c\u56fe\u7247\u4f1a\u751f\u6210\u5728\u8fd9\u91cc\u3002'
            )}
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className='size-full overflow-y-auto px-3 py-5 md:px-6 md:py-6'>
      <div className='mx-auto grid w-full max-w-7xl grid-cols-[repeat(auto-fit,minmax(min(100%,320px),390px))] justify-center gap-5 pb-28'>
        {items.map((item) => (
          <GenerationCard
            item={item}
            key={item.id}
            onOpenLightbox={onOpenLightbox}
            onDownload={onDownload}
            onCopyPrompt={onCopyPrompt}
            onRetry={onRetry}
            onDelete={onDelete}
          />
        ))}
      </div>
    </div>
  )
}

interface GenerationCardProps {
  item: ImageGenerationItem
  onOpenLightbox: (src: string) => void
  onDownload: (url: string) => void
  onCopyPrompt: (text: string) => void
  onRetry: (item: ImageGenerationItem) => void
  onDelete: (item: ImageGenerationItem) => void
}

function GenerationCard({
  item,
  onOpenLightbox,
  onDownload,
  onCopyPrompt,
  onRetry,
  onDelete,
}: GenerationCardProps) {
  const { t } = useTranslation()
  const imageSrc = useAuthenticatedImageSource(item.urls[0] || '')
  const [now, setNow] = useState(() => Date.now())
  const durationMs = generationDurationMs(item, now)
  const durationText =
    typeof durationMs === 'number' ? formatGenerationDuration(durationMs) : null

  useEffect(() => {
    if (item.status !== 'running') return
    setNow(Date.now())
    const timer = window.setInterval(() => {
      setNow(Date.now())
    }, 1000)
    return () => window.clearInterval(timer)
  }, [item.id, item.status])

  return (
    <div className='border-border/60 bg-card/60 group/card flex w-full min-w-0 flex-col overflow-hidden rounded-lg border shadow-sm transition-shadow hover:shadow-md'>
      <div className='bg-muted/25 relative aspect-[4/3] w-full overflow-hidden'>
        <div className='absolute top-2 right-2 z-10'>
          <DropdownMenu>
            <DropdownMenuTrigger
              className='bg-background/90 text-muted-foreground hover:text-foreground inline-flex size-8 items-center justify-center rounded-md border opacity-0 shadow-sm backdrop-blur transition-opacity group-hover/card:opacity-100 focus-visible:opacity-100 data-[popup-open]:opacity-100'
              aria-label={t('\u56fe\u7247\u64cd\u4f5c')}
            >
              <MoreHorizontalIcon className='size-4' />
            </DropdownMenuTrigger>
            <DropdownMenuContent align='end' sideOffset={6} className='w-36'>
              <DropdownMenuItem
                onClick={() => onCopyPrompt(item.prompt)}
                className='gap-2'
              >
                <CopyIcon className='size-3.5' />
                {t('\u590d\u5236\u63d0\u793a\u8bcd')}
              </DropdownMenuItem>
              {item.status === 'succeeded' && item.urls[0] && (
                <DropdownMenuItem
                  onClick={() => onDownload(item.urls[0])}
                  className='gap-2'
                >
                  <DownloadIcon className='size-3.5' />
                  {t('\u4e0b\u8f7d')}
                </DropdownMenuItem>
              )}
              {item.status === 'failed' && (
                <DropdownMenuItem
                  onClick={() => onRetry(item)}
                  className='gap-2'
                >
                  <RotateCcwIcon className='size-3.5' />
                  {t('\u91cd\u8bd5')}
                </DropdownMenuItem>
              )}
              <DropdownMenuSeparator />
              <DropdownMenuItem
                variant='destructive'
                onClick={() => onDelete(item)}
                className='gap-2'
              >
                <Trash2Icon className='size-3.5' />
                {t('\u5220\u9664')}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        {item.status === 'running' && (
          <div className='absolute inset-0 flex items-center justify-center'>
            <div className='flex flex-col items-center gap-2'>
              <Skeleton className='h-40 w-40 rounded-md' />
              <span className='text-muted-foreground inline-flex items-center gap-1 text-[11px]'>
                <ClockIcon className='size-3' />
                {durationText
                  ? `${t('Generating...')} ${durationText}`
                  : t('Generating...')}
              </span>
            </div>
          </div>
        )}
        {item.status === 'failed' && (
          <div className='text-muted-foreground flex size-full flex-col items-center justify-center gap-2 px-6 text-center'>
            <AlertCircleIcon className='text-destructive size-6' />
            <span className='line-clamp-3 text-xs leading-relaxed'>
              {item.errorMessage || t('Generation failed')}
            </span>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => onRetry(item)}
              className='mt-1 h-7 gap-1.5 px-2 text-[11px]'
            >
              <RotateCcwIcon className='size-3' />
              {t('Retry')}
            </Button>
          </div>
        )}
        {item.status === 'succeeded' && item.urls[0] && (
          <button
            type='button'
            onClick={() => onOpenLightbox(item.urls[0])}
            className='size-full'
          >
            {imageSrc && (
              <img
                src={imageSrc}
                alt={item.prompt}
                className='size-full object-contain transition-transform group-hover/card:scale-[1.01]'
              />
            )}
          </button>
        )}
        {item.status === 'succeeded' && item.urls.length > 1 && (
          <div className='text-foreground/80 absolute right-2 bottom-2 rounded-full bg-black/40 px-2 py-0.5 text-[10px] font-medium backdrop-blur'>
            {t('{{count}} \u5f20', { count: item.urls.length })}
          </div>
        )}
      </div>
      <div className='flex min-h-24 flex-col gap-2 px-3 pt-2.5 pb-3'>
        <div className='text-foreground line-clamp-2 text-sm leading-snug font-medium'>
          {item.prompt}
        </div>
        <div className='text-muted-foreground flex min-w-0 flex-wrap items-center gap-1.5 text-[10px]'>
          <span
            className='bg-muted max-w-[7.5rem] truncate rounded px-1.5 py-0.5'
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
            {item.params.resolution === 'auto'
              ? t('Auto')
              : item.params.resolution.toUpperCase()}
          </span>
          <span className='bg-muted shrink-0 rounded px-1.5 py-0.5'>
            {item.params.aspectRatio === 'auto'
              ? t('Auto')
              : item.params.aspectRatio}
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
      </div>
    </div>
  )
}
