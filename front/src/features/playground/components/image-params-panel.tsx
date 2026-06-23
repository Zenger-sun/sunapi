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
  hint: string
}[] = [
  { value: 'auto', label: 'Auto', hint: '按提示词自动选择清晰度' },
  { value: '1k', label: '1K', hint: '速度快，适合预览' },
  { value: '2k', label: '2K', hint: '更清晰，适合常用出图' },
  { value: '4k', label: '4K', hint: '高精细，需模型支持' },
]

const ASPECT_RATIO_OPTIONS: {
  value: ImageAspectRatio
  label: string
  hint: string
}[] = [
  { value: 'auto', label: 'Auto', hint: '按提示词自动选择画面比例' },
  { value: '1:1', label: '1:1', hint: '方图 / 头像 / 产品图' },
  { value: '16:9', label: '16:9', hint: '横屏 / 封面 / 桌面壁纸' },
  { value: '9:16', label: '9:16', hint: '竖屏 / 手机壁纸 / 短视频' },
  { value: '4:3', label: '4:3', hint: '经典横幅 / 演示图' },
  { value: '3:4', label: '3:4', hint: '竖幅 / 人像' },
  { value: '3:2', label: '3:2', hint: '摄影横幅' },
  { value: '2:3', label: '2:3', hint: '海报竖幅' },
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
          {t('生成参数')}
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
          <Field label={t('清晰度')}>
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
                        {t(opt.hint)}
                      </span>
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>

          <Field label={t('比例')}>
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
                        {t(opt.hint)}
                      </span>
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>

          <Field label={t('质量')}>
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
                <SelectItem value='auto'>{t('自动')}</SelectItem>
                <SelectItem value='standard'>{t('标准')}</SelectItem>
                <SelectItem value='hd'>{t('高清')}</SelectItem>
              </SelectContent>
            </Select>
          </Field>

          <Field label={t('风格')}>
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
                <SelectItem value='auto'>{t('自动')}</SelectItem>
                <SelectItem value='vivid'>{t('鲜艳')}</SelectItem>
                <SelectItem value='natural'>{t('自然')}</SelectItem>
              </SelectContent>
            </Select>
          </Field>

          <Field label={t('数量')}>
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
                    {t('{{count}} 张', { count: n })}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        </div>

        <Field label={t('负面提示词（可选）')}>
          <Textarea
            value={value.negativePrompt}
            onChange={(e) => update('negativePrompt', e.target.value)}
            placeholder={t('不想出现的内容，例如：模糊、低质、水印、畸形手')}
            className='min-h-16 resize-none text-xs'
            disabled={disabled}
          />
        </Field>

        <Field label={t('种子（可选）')}>
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
              placeholder={t('随机')}
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
              {t('随机')}
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
        {t('提示词建议')}
      </div>
      <ul className='list-disc space-y-1 pl-4'>
        <li>{t('说明主体、风格、光线、构图和用途。')}</li>
        <li>{t('先选比例，再按需要提高到 2K 或 4K。')}</li>
        <li>{t('需要改图或保持角色一致时，把参考图放进输入框。')}</li>
        <li>{t('固定种子可以尽量复现相近结果。')}</li>
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
