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
import { useState, useEffect, useRef } from 'react'
import {
  FilmIcon,
  SendIcon,
  SquareIcon,
  AlertCircleIcon,
  ClockIcon,
  ConstructionIcon,
  PlayCircleIcon,
  DownloadIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'
import { fetchVideoTask, submitVideoTask } from '../api'
import type { VideoGenerationItem } from '../types'

export type VideoGenerationParams = VideoGenerationItem['params']

interface VideoWorkspaceProps {
  params: VideoGenerationParams
  model: string
  group: string
  onGeneratingChange?: (generating: boolean) => void
}

function generateId() {
  return Math.random().toString(36).slice(2, 10) + Date.now().toString(36)
}

const ASPECT_RATIOS: {
  value: VideoGenerationItem['params']['aspectRatio']
  label: string
}[] = [
  { value: '16:9', label: '16:9 (Landscape)' },
  { value: '9:16', label: '9:16 (Portrait)' },
  { value: '1:1', label: '1:1 (Square)' },
]

const DURATIONS = [3, 5, 8, 10]

export function VideoWorkspace({
  params,
  model,
  group,
  onGeneratingChange,
}: VideoWorkspaceProps) {
  const { t } = useTranslation()
  const [items, setItems] = useState<VideoGenerationItem[]>([])
  const [prompt, setPrompt] = useState('')
  const [generating, setGenerating] = useState(false)
  const composerRef = useRef<HTMLTextAreaElement | null>(null)

  useEffect(() => {
    onGeneratingChange?.(generating)
  }, [generating, onGeneratingChange])

  const handleSubmit = () => {
    const text = prompt.trim()
    if (!text) {
      toast.info(t('Please describe the video you want to create'))
      composerRef.current?.focus()
      return
    }
    if (generating) return

    const newItem: VideoGenerationItem = {
      id: generateId(),
      prompt: text,
      params: { ...params },
      model,
      group,
      status: 'pending',
      createdAt: Date.now(),
    }
    setItems((prev) => [newItem, ...prev])
    setPrompt('')
    setGenerating(true)

    const startedAt = Date.now()
    const localId = newItem.id
    let pollHandle: number | null = null

    const markFailed = (message: string) => {
      setItems((prev) =>
        prev.map((item) =>
          item.id === localId
            ? {
                ...item,
                status: 'failed',
                errorMessage: message,
                durationMs: Date.now() - startedAt,
              }
            : item
        )
      )
      setGenerating(false)
    }

    const stopPolling = () => {
      if (pollHandle !== null) {
        window.clearInterval(pollHandle)
        pollHandle = null
      }
    }

    const markSucceeded = (
      taskItem: Partial<VideoGenerationItem> & { id?: string }
    ) => {
      setItems((prev) =>
        prev.map((item) =>
          item.id === localId
            ? {
                ...item,
                status: 'succeeded',
                previewUrl: taskItem.previewUrl ?? item.previewUrl,
                durationMs: Date.now() - startedAt,
              }
            : item
        )
      )
      setGenerating(false)
    }

    const startPolling = (taskId: string) => {
      pollHandle = window.setInterval(async () => {
        try {
          const result = await fetchVideoTask(taskId)
          const status = (result.status || '').toLowerCase()
          if (
            status === 'succeeded' ||
            status === 'success' ||
            status === 'completed'
          ) {
            stopPolling()
            markSucceeded({ previewUrl: result.preview_url || result.url })
          } else if (
            status === 'failed' ||
            status === 'error' ||
            status === 'cancelled'
          ) {
            stopPolling()
            markFailed(result.error?.message || t('Failed'))
          }
          // otherwise continue polling
        } catch {
          // transient network error: keep polling, do not fail the task yet
        }
      }, 3000)
    }

    void (async () => {
      try {
        const response = await submitVideoTask({
          model,
          group,
          prompt: text,
          duration_sec: params.durationSec,
          aspect_ratio: params.aspectRatio,
          seed: params.seed,
        })
        setItems((prev) =>
          prev.map((item) =>
            item.id === localId
              ? { ...item, status: 'running', id: response.id || item.id }
              : item
          )
        )
        const taskId = response.id || response.task_id
        if (taskId) {
          startPolling(taskId)
        } else {
          markFailed(t('Backend did not return a task id'))
        }
      } catch (error) {
        stopPolling()
        const message =
          error instanceof Error ? error.message : t('Video generation failed')
        setItems((prev) =>
          prev.map((item) =>
            item.id === localId
              ? {
                  ...item,
                  status: 'failed',
                  errorMessage: message,
                  durationMs: Date.now() - startedAt,
                }
              : item
          )
        )
        toast.error(message)
        setGenerating(false)
      }
    })()
  }

  const handleStop = () => {
    if (!generating) return
    setItems((prev) =>
      prev.map((item) =>
        item.status === 'running' || item.status === 'pending'
          ? { ...item, status: 'failed', errorMessage: t('Cancelled by user') }
          : item
      )
    )
    setGenerating(false)
  }

  return (
    <div className='flex size-full min-h-0 flex-col overflow-hidden'>
      <main className='bg-background flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden'>
        <Gallery items={items} />
      </main>

      <div className='bg-background shrink-0 border-t px-3 py-3 md:px-6'>
        <div className='mx-auto w-full max-w-3xl'>
          <VideoComposer
            ref={composerRef}
            value={prompt}
            onChange={setPrompt}
            onSubmit={handleSubmit}
            onStop={handleStop}
            isGenerating={generating}
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
      <ConstructionIcon className='mt-0.5 size-3.5 shrink-0' />
      <div>
        <div className='mb-0.5 font-medium'>
          {t('Video generation uses task queue')}
        </div>
        <div>
          {t(
            'Submit a prompt to create a queued video task. Results refresh automatically after the upstream channel starts processing.'
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
}

export function VideoParamsPanel({
  value,
  onChange,
  disabled,
}: VideoParamsPanelProps) {
  const { t } = useTranslation()

  const update = <K extends keyof VideoGenerationParams>(
    key: K,
    next: VideoGenerationParams[K]
  ) => onChange({ ...value, [key]: next })

  return (
    <div className='border-border/60 bg-card/40 space-y-3 rounded-lg border p-3'>
      <div className='text-foreground text-xs font-medium'>
        {t('Video parameters')}
      </div>
      <div className='grid grid-cols-2 gap-3'>
        <div className='space-y-1.5'>
          <Label className='text-muted-foreground text-[10px] font-medium tracking-wider uppercase'>
            {t('Aspect ratio')}
          </Label>
          <Select
            value={value.aspectRatio}
            onValueChange={(v) =>
              update('aspectRatio', v as VideoGenerationParams['aspectRatio'])
            }
            disabled={disabled}
          >
            <SelectTrigger className='h-8 w-full text-xs'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {ASPECT_RATIOS.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  <span className='text-xs'>{opt.label}</span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className='space-y-1.5'>
          <Label className='text-muted-foreground text-[10px] font-medium tracking-wider uppercase'>
            {t('Duration (sec)')}
          </Label>
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
        </div>

        <div className='col-span-2 space-y-1.5'>
          <Label className='text-muted-foreground text-[10px] font-medium tracking-wider uppercase'>
            {t('Seed (optional)')}
          </Label>
          <Input
            type='number'
            value={value.seed ?? ''}
            onChange={(e) => {
              const next = e.target.value
              update('seed', next === '' ? null : Number(next))
            }}
            placeholder={t('Random')}
            className='h-8 text-xs'
            disabled={disabled}
          />
        </div>
      </div>
    </div>
  )
}

export function useVideoDefaultParams(): VideoGenerationParams {
  return {
    durationSec: 5,
    aspectRatio: '16:9',
    seed: null,
  }
}

interface VideoComposerProps {
  value: string
  onChange: (value: string) => void
  onSubmit: () => void
  onStop: () => void
  isGenerating: boolean
}

const VideoComposer = ({
  value,
  onChange,
  onSubmit,
  onStop,
  isGenerating,
  ref,
}: VideoComposerProps & { ref?: React.Ref<HTMLTextAreaElement> }) => {
  const { t } = useTranslation()
  return (
    <div
      className={cn(
        'border-input bg-background rounded-xl border shadow-xs transition-colors',
        'focus-within:border-ring focus-within:ring-ring/50 focus-within:ring-3'
      )}
    >
      <Textarea
        ref={ref}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={t('Describe the video you want to create...')}
        disabled={isGenerating}
        className='min-h-24 resize-none rounded-b-none border-0 px-5 py-4 text-sm shadow-none focus-visible:ring-0 md:text-base'
      />
      <div className='flex items-center justify-between gap-2 border-t p-2.5'>
        <div className='text-muted-foreground inline-flex items-center gap-1.5 px-1.5 text-xs'>
          <FilmIcon className='size-3.5' />
          {t('Video generation')}
        </div>
        <div className='flex items-center gap-1.5'>
          {isGenerating ? (
            <Button
              type='button'
              size='sm'
              variant='secondary'
              onClick={onStop}
              className='gap-1.5 text-xs'
            >
              <SquareIcon className='size-3.5 fill-current' />
              {t('Stop')}
            </Button>
          ) : (
            <Button
              type='button'
              size='sm'
              onClick={onSubmit}
              disabled={!value.trim()}
              className='gap-1.5 text-xs'
            >
              <SendIcon className='size-3.5' />
              {t('Generate')}
            </Button>
          )}
        </div>
      </div>
    </div>
  )
}

function Gallery({ items }: { items: VideoGenerationItem[] }) {
  const { t } = useTranslation()
  if (items.length === 0) {
    return (
      <div className='flex size-full items-center justify-center px-4 py-12'>
        <div className='text-muted-foreground flex max-w-md flex-col items-center gap-3 text-center'>
          <div className='border-border/70 bg-muted/30 flex aspect-video w-full max-w-sm items-center justify-center rounded-xl border-2 border-dashed'>
            <div className='flex flex-col items-center gap-2'>
              <FilmIcon className='size-6' />
              <span className='text-xs'>{t('No video tasks yet')}</span>
            </div>
          </div>
          <div className='text-xs leading-relaxed'>
            {t(
              'Describe a video scene to queue a generation. Tasks will refresh here while they run.'
            )}
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className='size-full overflow-y-auto px-3 py-4 md:px-5'>
      <div className='mx-auto grid max-w-5xl grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3'>
        {items.map((item) => (
          <VideoCard item={item} key={item.id} />
        ))}
      </div>
    </div>
  )
}

function VideoCard({ item }: { item: VideoGenerationItem }) {
  const { t } = useTranslation()
  return (
    <div className='border-border/60 bg-card/40 flex flex-col gap-2 overflow-hidden rounded-lg border'>
      <div className='bg-muted/30 relative aspect-video w-full overflow-hidden'>
        {item.status === 'running' && (
          <div className='absolute inset-0 flex items-center justify-center'>
            <div className='flex flex-col items-center gap-2'>
              <Skeleton className='h-24 w-32 rounded-md' />
              <span className='text-muted-foreground inline-flex items-center gap-1 text-[11px]'>
                <ClockIcon className='size-3' />
                {t('Rendering...')}
              </span>
            </div>
          </div>
        )}
        {item.status === 'failed' && (
          <div className='text-muted-foreground flex size-full flex-col items-center justify-center gap-1.5 px-3 text-center'>
            <AlertCircleIcon className='text-destructive size-5' />
            <span className='text-xs'>{item.errorMessage || t('Failed')}</span>
          </div>
        )}
        {item.status === 'succeeded' && (
          <div className='flex size-full flex-col items-center justify-center gap-2 p-2'>
            {item.previewUrl ? (
              <video
                controls
                src={item.previewUrl}
                className='max-h-full max-w-full rounded'
              />
            ) : (
              <>
                <PlayCircleIcon className='text-muted-foreground size-10' />
                <span className='text-muted-foreground text-[11px]'>
                  {t('Preview unavailable in preview build')}
                </span>
              </>
            )}
            {item.previewUrl && (
              <Button
                type='button'
                variant='outline'
                size='sm'
                className='gap-1.5 text-xs'
                onClick={() => window.open(item.previewUrl, '_blank')}
              >
                <DownloadIcon className='size-3' />
                {t('Open')}
              </Button>
            )}
          </div>
        )}
        {item.status === 'pending' && (
          <div className='text-muted-foreground flex size-full items-center justify-center text-[11px]'>
            {t('Queued')}
          </div>
        )}
      </div>
      <div className='flex flex-col gap-1 px-2.5 pb-2.5'>
        <div className='text-foreground line-clamp-2 text-xs leading-snug font-medium'>
          {item.prompt}
        </div>
        <div className='text-muted-foreground flex items-center gap-1.5 text-[10px]'>
          <span className='bg-muted rounded px-1.5 py-0.5'>{item.model}</span>
          <span className='bg-muted rounded px-1.5 py-0.5'>
            {item.params.aspectRatio}
          </span>
          <span className='bg-muted rounded px-1.5 py-0.5'>
            {item.params.durationSec}s
          </span>
        </div>
      </div>
    </div>
  )
}
