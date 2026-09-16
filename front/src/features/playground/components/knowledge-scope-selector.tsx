/*
Copyright (C) 2026 SunAPI Project Owner

This file is part of the SunAPI Creator Studio Components.

Licensed under the SunAPI Creator Studio Source-Available Commercial License
v1.0. See LICENSE-CREATOR-STUDIO at the repository root.

This source is available for evaluation, but modification, redistribution,
hosting, sale, resale, sublicensing, or commercial use requires prior written
permission from the SunAPI project owner.
*/
import { useEffect, useState } from 'react'
import { BookOpenIcon, Loader2Icon } from 'lucide-react'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { listKnowledgeBases } from '../api'
import type { KnowledgeBaseItem } from '../types'

type KnowledgeMode = 'manual' | 'off'

export interface KnowledgeScopeValue {
  knowledgeBaseIds?: string[]
  knowledgeDisabled?: boolean
  knowledgeManual?: boolean
}

interface KnowledgeScopeSelectorProps {
  value: KnowledgeScopeValue
  onChange: (next: KnowledgeScopeValue) => void
  disabled?: boolean
  compact?: boolean
  toolbar?: boolean
  className?: string
}

export function KnowledgeScopeSelector({
  value,
  onChange,
  disabled,
  compact = false,
  toolbar = false,
  className,
}: KnowledgeScopeSelectorProps) {
  const [items, setItems] = useState<KnowledgeBaseItem[]>([])
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    listKnowledgeBases()
      .then((next) => {
        if (!cancelled) setItems(next)
      })
      .catch((error) => {
        if (!cancelled) {
          toast.error(
            error instanceof Error ? error.message : '知识库列表加载失败'
          )
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [])

  const selectedIds = value.knowledgeBaseIds ?? []
  const mode: KnowledgeMode =
    !value.knowledgeDisabled &&
    (value.knowledgeManual || selectedIds.length > 0)
      ? 'manual'
      : 'off'

  const setMode = (nextMode: KnowledgeMode) => {
    if (nextMode === 'off') {
      onChange({
        knowledgeDisabled: true,
        knowledgeBaseIds: [],
        knowledgeManual: false,
      })
      return
    }
    if (nextMode === 'manual') {
      onChange({
        knowledgeDisabled: false,
        knowledgeBaseIds: selectedIds,
        knowledgeManual: true,
      })
      return
    }
  }

  const toggleBase = (id: string, checked: boolean) => {
    const nextIds = checked
      ? Array.from(new Set([...selectedIds, id]))
      : selectedIds.filter((item) => item !== id)
    onChange({
      knowledgeDisabled: false,
      knowledgeBaseIds: nextIds,
      knowledgeManual: true,
    })
  }

  const summary =
    mode === 'off'
      ? '已关闭'
      : mode === 'manual'
        ? selectedIds.length > 0
          ? '已选择 ' + selectedIds.length + ' 个'
          : '选择知识库'
        : '未启用'

  const selectorContent = (
    <>
      <div className='mb-2 flex items-center justify-between gap-2'>
        <div className='flex min-w-0 items-center gap-1.5'>
          <BookOpenIcon className='text-muted-foreground size-3.5 shrink-0' />
          <span className='truncate text-xs font-medium'>知识库</span>
          <Badge variant='secondary' className='h-4 px-1 text-[9px]'>
            {summary}
          </Badge>
        </div>
        {loading && (
          <Loader2Icon className='text-muted-foreground size-3 animate-spin' />
        )}
      </div>

      <div className='grid grid-cols-2 gap-1'>
        {(['manual', 'off'] as KnowledgeMode[]).map((item) => (
          <Button
            key={item}
            type='button'
            size='sm'
            variant={mode === item ? 'default' : 'outline'}
            className='h-7 px-2 text-[10px]'
            disabled={disabled}
            onClick={() => setMode(item)}
          >
            {item === 'manual' ? '选择知识库' : '关闭'}
          </Button>
        ))}
      </div>

      {mode === 'manual' && (
        <div className='mt-2 max-h-40 space-y-1 overflow-y-auto pr-1'>
          {items.length === 0 ? (
            <div className='text-muted-foreground px-1 py-2 text-[10px]'>
              暂无知识库，请先在知识库页面上传文档
            </div>
          ) : (
            items.map((item) => (
              <label
                key={item.id}
                className='border-border/50 hover:bg-muted/50 flex items-center gap-2 rounded-md border px-2 py-1.5 text-[10px]'
              >
                <Checkbox
                  checked={selectedIds.includes(item.id)}
                  disabled={disabled}
                  onCheckedChange={(checked) =>
                    toggleBase(item.id, checked === true)
                  }
                />
                <span className='min-w-0 flex-1'>
                  <span className='block truncate font-medium'>
                    {item.name}
                  </span>
                  <span className='text-muted-foreground block truncate'>
                    {item.document_count} 个文档 / {item.chunk_count} 个分块
                  </span>
                </span>
              </label>
            ))
          )}
        </div>
      )}
    </>
  )

  if (toolbar) {
    return (
      <Popover>
        <PopoverTrigger
          render={
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={disabled}
              aria-label='知识库'
              className={cn(
                'border font-medium',
                mode === 'manual' && 'border-primary/40 bg-primary/5',
                className
              )}
            />
          }
        >
          <BookOpenIcon className='size-3.5' />
          <span>知识库</span>
          <Badge
            variant={mode === 'off' ? 'secondary' : 'default'}
            className='h-4 px-1 text-[9px]'
          >
            {summary}
          </Badge>
        </PopoverTrigger>
        <PopoverContent
          align='start'
          side='top'
          sideOffset={8}
          className='w-[min(21rem,calc(100vw-2rem))] p-2.5'
        >
          {selectorContent}
        </PopoverContent>
      </Popover>
    )
  }

  return (
    <section
      className={cn(
        'border-border/60 bg-card/40 rounded-lg border p-2.5',
        compact && 'p-2',
        className
      )}
    >
      {selectorContent}
    </section>
  )
}
