/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useEffect, useState } from 'react'
import { Check, ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

// ============================================================================
// 模型配置数据（与对话页保持一致）
// ============================================================================

export type ModelOption = {
  id: string
  name: string
  label: string
  contextLength?: string
  badge?: 'recommended' | 'fast' | 'long-context'
}

export type ModelGroup = {
  id: string
  label: string
  models: ModelOption[]
}

export const MODEL_GROUPS: ModelGroup[] = [
  {
    id: 'domestic',
    label: '国内模型',
    models: [
      {
        id: 'qwen-max',
        name: 'qwen-max',
        label: '通义千问 Qwen-Max',
        contextLength: '128K',
        badge: 'recommended',
      },
      {
        id: 'qwen-plus',
        name: 'qwen-plus',
        label: '通义千问 Qwen-Plus',
        contextLength: '128K',
      },
      {
        id: 'qwen-turbo',
        name: 'qwen-turbo',
        label: '通义千问 Qwen-Turbo',
        contextLength: '128K',
        badge: 'fast',
      },
      {
        id: 'ernie-4',
        name: 'ernie-4.0',
        label: '百度文心 ERNIE-4.0',
        contextLength: '8K',
      },
      {
        id: 'ernie-speed',
        name: 'ernie-speed',
        label: '百度文心 ERNIE-Speed',
        contextLength: '8K',
        badge: 'fast',
      },
      {
        id: 'glm-4',
        name: 'glm-4',
        label: '智谱 GLM-4',
        contextLength: '128K',
      },
    ],
  },
  {
    id: 'overseas',
    label: '海外模型',
    models: [
      {
        id: 'gpt-4o',
        name: 'gpt-4o',
        label: 'OpenAI GPT-4o',
        contextLength: '128K',
        badge: 'recommended',
      },
      {
        id: 'gpt-4o-mini',
        name: 'gpt-4o-mini',
        label: 'OpenAI GPT-4o Mini',
        contextLength: '128K',
        badge: 'fast',
      },
      {
        id: 'claude-3-5-sonnet',
        name: 'claude-3-5-sonnet-20241022',
        label: 'Claude 3.5 Sonnet',
        contextLength: '200K',
        badge: 'long-context',
      },
      {
        id: 'gemini-1-5-pro',
        name: 'gemini-1.5-pro',
        label: 'Gemini 1.5 Pro',
        contextLength: '1M',
        badge: 'long-context',
      },
    ],
  },
  {
    id: 'opensource',
    label: '开源模型',
    models: [
      {
        id: 'llama-3-70b',
        name: 'llama-3-70b',
        label: 'Llama 3 70B',
        contextLength: '8K',
      },
      {
        id: 'deepseek-v2',
        name: 'deepseek-v2',
        label: 'DeepSeek V2',
        contextLength: '128K',
      },
    ],
  },
]

const BADGE_LABELS: Record<string, string> = {
  recommended: '推荐',
  fast: '⚡ 快速',
  'long-context': '长上下文',
}

const BADGE_CLASSES: Record<string, string> = {
  recommended:
    'bg-blue-50 text-blue-700 border-blue-200 dark:bg-blue-950 dark:text-blue-300',
  fast: 'bg-amber-50 text-amber-700 border-amber-200 dark:bg-amber-950 dark:text-amber-300',
  'long-context':
    'bg-purple-50 text-purple-700 border-purple-200 dark:bg-purple-950 dark:text-purple-300',
}

// ============================================================================
// 组件 Props
// ============================================================================

type ModelSelectorProps = {
  groupId?: string
  modelName?: string
  onGroupChange?: (groupId: string) => void
  onModelChange?: (modelName: string) => void
}

export function ModelSelector({
  groupId,
  modelName,
  onGroupChange,
  onModelChange,
}: ModelSelectorProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [selectedGroupId, setSelectedGroupId] = useState(
    groupId ?? MODEL_GROUPS[0].id
  )

  // 当外部 groupId 变化时同步
  useEffect(() => {
    if (groupId) setSelectedGroupId(groupId)
  }, [groupId])

  const selectedGroup =
    MODEL_GROUPS.find((g) => g.id === selectedGroupId) ?? MODEL_GROUPS[0]

  const selectedModel =
    selectedGroup.models.find((m) => m.name === modelName) ??
    selectedGroup.models[0]

  const handleGroupChange = (val: string) => {
    setSelectedGroupId(val)
    onGroupChange?.(val)
    // 切换分组时重置为该分组第一个模型
    const newGroup = MODEL_GROUPS.find((g) => g.id === val)
    if (newGroup) onModelChange?.(newGroup.models[0].name)
  }

  const handleModelSelect = (model: ModelOption) => {
    onModelChange?.(model.name)
    setOpen(false)
  }

  return (
    <div className='flex flex-col gap-2'>
      {/* 分组选择 */}
      <div className='flex items-center gap-2'>
        <label className='text-muted-foreground w-16 shrink-0 text-xs'>
          {t('Group')}
        </label>
        <Select value={selectedGroupId} onValueChange={handleGroupChange}>
          <SelectTrigger className='h-8 flex-1 text-xs'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {MODEL_GROUPS.map((g) => (
              <SelectItem key={g.id} value={g.id}>
                {t(g.label)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* 模型选择 */}
      <div className='flex items-center gap-2'>
        <label className='text-muted-foreground w-16 shrink-0 text-xs'>
          {t('Model')}
        </label>
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger asChild>
            <Button
              variant='outline'
              role='combobox'
              aria-expanded={open}
              className='h-8 flex-1 justify-between text-xs'
            >
              <span className='truncate'>{selectedModel?.label}</span>
              <ChevronDown
                className={cn(
                  'ml-1 h-3.5 w-3.5 shrink-0 transition-transform',
                  open && 'rotate-180'
                )}
                aria-hidden='true'
              />
            </Button>
          </PopoverTrigger>
          <PopoverContent className='w-[320px] p-0' align='start'>
            <Command>
              <CommandInput
                placeholder={t('Search model...')}
                className='h-8 text-xs'
              />
              <CommandList>
                <CommandEmpty>{t('No model found')}</CommandEmpty>
                <CommandGroup>
                  {selectedGroup.models.map((model) => (
                    <CommandItem
                      key={model.id}
                      value={model.label}
                      onSelect={() => handleModelSelect(model)}
                      className='flex items-center justify-between gap-2 py-2'
                    >
                      <div className='flex min-w-0 flex-1 items-center gap-2'>
                        <Check
                          className={cn(
                            'h-3.5 w-3.5 shrink-0',
                            model.name === selectedModel?.name
                              ? 'opacity-100'
                              : 'opacity-0'
                          )}
                          aria-hidden='true'
                        />
                        <span className='truncate text-xs'>{model.label}</span>
                      </div>
                      <div className='flex shrink-0 items-center gap-1.5'>
                        {model.contextLength && (
                          <span className='text-muted-foreground text-[10px]'>
                            {model.contextLength}
                          </span>
                        )}
                        {model.badge && (
                          <span
                            className={cn(
                              'rounded border px-1.5 py-0.5 text-[10px] font-medium',
                              BADGE_CLASSES[model.badge]
                            )}
                          >
                            {BADGE_LABELS[model.badge]}
                          </span>
                        )}
                      </div>
                    </CommandItem>
                  ))}
                </CommandGroup>
              </CommandList>
            </Command>
          </PopoverContent>
        </Popover>
      </div>

      {/* 所选模型信息展示 */}
      {selectedModel && (
        <div className='text-muted-foreground mt-0.5 flex items-center gap-2 pl-[72px] text-[11px]'>
          {selectedModel.contextLength && (
            <span>上下文 {selectedModel.contextLength}</span>
          )}
          {selectedModel.badge && (
            <span
              className={cn(
                'rounded border px-1.5 py-0.5 text-[10px] font-medium',
                BADGE_CLASSES[selectedModel.badge]
              )}
            >
              {BADGE_LABELS[selectedModel.badge]}
            </span>
          )}
        </div>
      )}
    </div>
  )
}
