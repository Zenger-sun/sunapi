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
import { useState, useMemo } from 'react'
import { ChevronsUpDown, Check, CpuIcon, CompassIcon } from 'lucide-react'
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
import type { ModelOption, GroupOption } from '../types'

interface ModelCapsuleProps {
  model: string
  models: ModelOption[]
  group: string
  groups: GroupOption[]
  onModelChange: (value: string) => void
  onGroupChange: (value: string) => void
  onBrowseModels?: () => void
  disabled?: boolean
  isLoading?: boolean
}

function formatContextWindow(value?: number) {
  if (!value) return null
  if (value >= 1000) {
    return `${Math.round(value / 1000)}K`
  }
  return `${value}`
}

export function ModelCapsule({
  model,
  models,
  group,
  groups,
  onModelChange,
  onGroupChange,
  onBrowseModels,
  disabled,
  isLoading,
}: ModelCapsuleProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')

  const currentModel = useMemo(
    () => models.find((m) => m.value === model),
    [models, model]
  )
  const currentGroup = useMemo(
    () => groups.find((g) => g.value === group),
    [groups, group]
  )

  const filtered = useMemo(() => {
    if (!search.trim()) return models
    const query = search.toLowerCase()
    return models.filter(
      (m) =>
        m.label.toLowerCase().includes(query) ||
        m.value.toLowerCase().includes(query) ||
        m.description?.toLowerCase().includes(query)
    )
  }, [models, search])

  const handleSelect = (value: string) => {
    onModelChange(value)
    setOpen(false)
    setSearch('')
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={disabled}
            data-model-capsule-trigger=''
            className={cn(
              'border-border/60 h-8 gap-2 rounded-full px-3 font-medium shadow-none',
              'hover:bg-accent'
            )}
            aria-expanded={open}
          />
        }
      >
        <CpuIcon className='text-muted-foreground size-3.5' />
        <span className='text-foreground max-w-[12rem] truncate text-xs'>
          {currentModel?.label || model || t('Select model')}
        </span>
        {currentGroup && currentGroup.value !== 'default' && (
          <span className='bg-muted text-muted-foreground rounded-md px-1.5 py-0.5 text-[10px] font-medium'>
            {currentGroup.label}
          </span>
        )}
        <ChevronsUpDown className='text-muted-foreground size-3.5 opacity-60' />
      </PopoverTrigger>
      <PopoverContent
        className='bg-popover w-[min(28rem,90vw)] rounded-lg border p-0 shadow-lg'
        align='start'
        side='bottom'
        sideOffset={6}
      >
        <Command className='rounded-lg' shouldFilter={false}>
          <CommandInput
            value={search}
            onValueChange={setSearch}
            placeholder={t('Search models...')}
            className='h-9'
          />
          <CommandList className='max-h-80'>
            <CommandEmpty>
              {isLoading ? t('Loading...') : t('No model found.')}
            </CommandEmpty>

            {groups.length > 0 && (
              <CommandGroup heading={t('Model group')}>
                <div className='flex flex-wrap gap-1 px-2 pb-1'>
                  {groups.map((g) => {
                    const active = g.value === group
                    return (
                      <button
                        key={g.value}
                        type='button'
                        onClick={() => onGroupChange(g.value)}
                        className={cn(
                          'rounded-md border px-2 py-0.5 text-[10px] font-medium transition-colors',
                          active
                            ? 'border-primary/40 bg-primary/10 text-primary'
                            : 'text-muted-foreground hover:bg-accent border-transparent'
                        )}
                      >
                        {g.label}
                        {g.ratio ? ` - ${g.ratio}x` : ''}
                      </button>
                    )
                  })}
                </div>
              </CommandGroup>
            )}

            <CommandGroup heading={t('Available models')}>
              {isLoading || filtered.length === 0 ? (
                <div className='text-muted-foreground px-3 py-4 text-xs'>
                  {isLoading ? t('Loading...') : t('No model found.')}
                </div>
              ) : (
                filtered.map((m) => {
                  const isActive = m.value === model
                  const ctx = formatContextWindow(m.contextWindow)
                  return (
                    <CommandItem
                      key={m.value}
                      value={m.value}
                      onSelect={handleSelect}
                      className='flex items-center gap-2 px-2 py-2 text-xs'
                    >
                      <div className='flex min-w-0 flex-1 flex-col gap-0.5'>
                        <div className='flex items-center gap-1.5'>
                          <span className='truncate font-medium'>
                            {m.label}
                          </span>
                          {m.supportsImage && (
                            <span className='rounded bg-fuchsia-500/10 px-1 text-[9px] text-fuchsia-600 dark:text-fuchsia-300'>
                              IMG
                            </span>
                          )}
                          {m.supportsVideo && (
                            <span className='rounded bg-amber-500/10 px-1 text-[9px] text-amber-600 dark:text-amber-300'>
                              VID
                            </span>
                          )}
                        </div>
                        {m.description && (
                          <div className='text-muted-foreground truncate text-[10px]'>
                            {m.description}
                          </div>
                        )}
                      </div>
                      <div className='text-muted-foreground flex shrink-0 items-center gap-2 text-[10px]'>
                        {ctx && <span>ctx {ctx}</span>}
                        {m.pricePerKToken ? (
                          <span>${m.pricePerKToken.toFixed(4)}/1K</span>
                        ) : null}
                      </div>
                      <Check
                        className={cn(
                          'size-3.5 shrink-0',
                          isActive ? 'opacity-100' : 'opacity-0'
                        )}
                      />
                    </CommandItem>
                  )
                })
              )}
            </CommandGroup>
          </CommandList>
          {onBrowseModels && (
            <div className='border-border/60 border-t p-1'>
              <Button
                type='button'
                variant='ghost'
                size='sm'
                className='text-muted-foreground w-full justify-start gap-2 text-xs'
                onClick={() => {
                  onBrowseModels()
                  setOpen(false)
                }}
              >
                <CompassIcon className='size-3.5' />
                {t('Browse model marketplace')}
              </Button>
            </div>
          )}
        </Command>
      </PopoverContent>
    </Popover>
  )
}
