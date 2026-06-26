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
import { useState } from 'react'
import {
  Settings2Icon,
  ChevronDownIcon,
  SparklesIcon,
  WandSparklesIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import type {
  ImageAspectRatio,
  ImageGenerationParams,
  ImageResolutionPreset,
} from '../types'

interface ImageParamsPanelProps {
  value: ImageGenerationParams
  onChange: (next: ImageGenerationParams) => void
  disabled?: boolean
  compact?: boolean
}

const RESOLUTION_OPTIONS: {
  value: ImageResolutionPreset
  label: string
  hintKey: string
}[] = [
  { value: 'auto', label: 'Auto', hintKey: 'Choose resolution from the prompt' },
  { value: '1k', label: '1K', hintKey: 'Fast, good for previews' },
  { value: '2k', label: '2K', hintKey: 'Sharper, good for regular images' },
  { value: '4k', label: '4K', hintKey: 'High detail, model support required' },
]

const ASPECT_RATIO_OPTIONS: {
  value: ImageAspectRatio
  label: string
  hintKey: string
}[] = [
  { value: 'auto', label: 'Auto', hintKey: 'Choose aspect ratio from the prompt' },
  { value: '1:1', label: '1:1', hintKey: 'Square / avatar / product image' },
  { value: '16:9', label: '16:9', hintKey: 'Landscape / cover / desktop wallpaper' },
  { value: '9:16', label: '9:16', hintKey: 'Portrait / phone wallpaper / short video' },
  { value: '4:3', label: '4:3', hintKey: 'Classic banner / presentation image' },
  { value: '3:4', label: '3:4', hintKey: 'Portrait frame / people' },
  { value: '3:2', label: '3:2', hintKey: 'Photography landscape' },
  { value: '2:3', label: '2:3', hintKey: 'Poster portrait' },
]

export function ImageParamsPanel({
  value,
  onChange,
  disabled,
  compact = false,
}: ImageParamsPanelProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(true)

  const update = <K extends keyof ImageGenerationParams>(
    key: K,
    next: ImageGenerationParams[K]
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
          {t('Generation parameters')}
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
          <Field label={t('Resolution')}>
            <Select
              value={value.resolution}
              onValueChange={(v) =>
                update('resolution', v as ImageResolutionPreset)
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

          <Field label={t('Aspect ratio')}>
            <Select
              value={value.aspectRatio}
              onValueChange={(v) =>
                update('aspectRatio', v as ImageAspectRatio)
              }
              disabled={disabled}
            >
              <SelectTrigger className='h-8 w-full text-xs'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {ASPECT_RATIO_OPTIONS.map((opt) => (
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

          <Field label={t('Quality')}>
            <Select
              value={value.quality}
              onValueChange={(v) =>
                update('quality', v as ImageGenerationParams['quality'])
              }
              disabled={disabled}
            >
              <SelectTrigger className='h-8 w-full text-xs'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='auto'>{t('Auto')}</SelectItem>
                <SelectItem value='standard'>{t('Standard')}</SelectItem>
                <SelectItem value='hd'>{t('HD')}</SelectItem>
              </SelectContent>
            </Select>
          </Field>

          <Field label={t('Style')}>
            <Select
              value={value.style}
              onValueChange={(v) =>
                update('style', v as ImageGenerationParams['style'])
              }
              disabled={disabled}
            >
              <SelectTrigger className='h-8 w-full text-xs'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='auto'>{t('Auto')}</SelectItem>
                <SelectItem value='vivid'>{t('Vivid')}</SelectItem>
                <SelectItem value='natural'>{t('Natural')}</SelectItem>
              </SelectContent>
            </Select>
          </Field>

          <Field label={t('Count')}>
            <Select
              value={String(value.n)}
              onValueChange={(v) => update('n', Number(v))}
              disabled={disabled}
            >
              <SelectTrigger className='h-8 w-full text-xs'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {[1, 2, 3, 4].map((n) => (
                  <SelectItem key={n} value={String(n)}>
                    {t('{{count}} image(s)', { count: n })}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        </div>

        <Field label={t('Negative prompt (optional)')}>
          <Textarea
            value={value.negativePrompt}
            onChange={(e) => update('negativePrompt', e.target.value)}
            placeholder={t(
              'Avoid content such as blur, low quality, watermark, malformed hands'
            )}
            className='min-h-16 resize-none text-xs'
            disabled={disabled}
          />
        </Field>

        <Field label={t('Seed (optional)')}>
          <div
            className={cn(
              'flex gap-2',
              compact ? 'flex-col items-stretch' : 'items-center'
            )}
          >
            <Input
              type='number'
              value={value.seed ?? ''}
              onChange={(event) => {
                const v = event.target.value
                update('seed', v === '' ? null : Number(v))
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
      </CollapsibleContent>
    </Collapsible>
  )
}

export function ImagePromptTips() {
  const { t } = useTranslation()
  return (
    <div className='border-border/60 bg-card/40 text-muted-foreground rounded-lg border p-3 text-[11px] leading-relaxed'>
      <div className='text-foreground mb-1.5 flex items-center gap-1.5 text-xs font-medium'>
        <WandSparklesIcon className='size-3.5' />
        {t('Prompt tips')}
      </div>
      <ul className='list-disc space-y-1 pl-4'>
        <li>{t('Describe the subject, style, lighting, composition, and use case.')}</li>
        <li>{t('Pick an aspect ratio first, then raise resolution to 2K or 4K when needed.')}</li>
        <li>{t('Add reference images when editing or keeping a character consistent.')}</li>
        <li>{t('Use a fixed seed to reproduce similar results when possible.')}</li>
      </ul>
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

export function useImageDefaultParams(): ImageGenerationParams {
  return {
    resolution: 'auto',
    aspectRatio: 'auto',
    quality: 'auto',
    style: 'auto',
    n: 1,
    seed: null,
    negativePrompt: '',
  }
}
