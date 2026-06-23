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
import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { type ColumnDef } from '@tanstack/react-table'
import { Check, ChevronsUpDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { getUserGroups } from '@/lib/api'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import { Progress } from '@/components/ui/progress'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { DataTableColumnHeader } from '@/components/data-table'
import { GroupBadge } from '@/components/group-badge'
import { StatusBadge } from '@/components/status-badge'
import { updateApiKey } from '../api'
import { API_KEY_STATUSES } from '../constants'
import { type ApiKey } from '../types'
import {
  ApiKeyCell,
  ModelLimitsCell,
  IpRestrictionsCell,
} from './api-keys-cells'
import { useApiKeys } from './api-keys-provider'
import { DataTableRowActions } from './data-table-row-actions'

function getQuotaProgressColor(percentage: number): string {
  if (percentage <= 10) return '[&_[data-slot=progress-indicator]]:bg-rose-500'
  if (percentage <= 30) return '[&_[data-slot=progress-indicator]]:bg-amber-500'
  return '[&_[data-slot=progress-indicator]]:bg-emerald-500'
}

type InlineGroupOption = {
  value: string
  label: string
  desc?: string
  ratio?: number
}

function useGroupOptions(): InlineGroupOption[] {
  const { data } = useQuery({
    queryKey: ['user-self-groups'],
    queryFn: getUserGroups,
    staleTime: 5 * 60 * 1000,
    select: (res) => {
      if (!res.success || !res.data) return []
      const options: InlineGroupOption[] = []
      for (const [group, info] of Object.entries(res.data)) {
        const numericRatio = Number(info.ratio)
        options.push({
          value: group,
          label: group,
          desc: info.desc || group,
          ratio: Number.isFinite(numericRatio) ? numericRatio : undefined,
        })
      }
      return options
    },
  })

  return data ?? []
}

function optionListWithCurrentGroup(
  options: InlineGroupOption[],
  group: string,
  ratio?: number
) {
  if (!group || options.some((option) => option.value === group)) {
    return options
  }
  return [
    {
      value: group,
      label: group,
      desc: group,
      ratio,
    },
    ...options,
  ]
}

function apiKeyPayloadWithGroup(apiKey: ApiKey, group: string) {
  return {
    id: apiKey.id,
    name: apiKey.name,
    remain_quota: apiKey.remain_quota,
    expired_time: apiKey.expired_time,
    unlimited_quota: apiKey.unlimited_quota,
    model_limits_enabled: apiKey.model_limits_enabled,
    model_limits: apiKey.model_limits || '',
    allow_ips: apiKey.allow_ips || '',
    group,
    cross_group_retry: group === 'auto' ? !!apiKey.cross_group_retry : false,
  }
}

function ApiKeyGroupSwitcher({
  apiKey,
  options,
}: {
  apiKey: ApiKey
  options: InlineGroupOption[]
}) {
  const { t } = useTranslation()
  const { triggerRefresh } = useApiKeys()
  const [open, setOpen] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  const [selectedGroup, setSelectedGroup] = useState(apiKey.group || 'default')
  const currentOption = options.find((option) => option.value === selectedGroup)
  const currentRatio = selectedGroup === 'auto' ? undefined : currentOption?.ratio
  const groupOptions = optionListWithCurrentGroup(
    options,
    selectedGroup,
    currentRatio
  )

  useEffect(() => {
    setSelectedGroup(apiKey.group || 'default')
  }, [apiKey.group])

  const handleSelect = async (group: string) => {
    if (!group || group === selectedGroup || isSaving) {
      setOpen(false)
      return
    }

    const previousGroup = selectedGroup
    setSelectedGroup(group)
    setOpen(false)
    setIsSaving(true)
    try {
      const result = await updateApiKey(apiKeyPayloadWithGroup(apiKey, group))
      if (result.success) {
        toast.success(t('API Key updated successfully'))
        triggerRefresh()
      } else {
        setSelectedGroup(previousGroup)
        toast.error(result.message || t('Failed to update API key'))
      }
    } catch {
      setSelectedGroup(previousGroup)
      toast.error(t('Failed to update API key'))
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <div className='flex min-w-[136px] items-center gap-1.5'>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger
          render={
            <Button
              type='button'
              variant='ghost'
              disabled={isSaving || options.length === 0}
              className='h-8 min-w-0 justify-start gap-1.5 rounded-md px-1.5 font-normal'
            />
          }
        >
          <GroupBadge group={selectedGroup} ratio={currentRatio} />
          <ChevronsUpDown className='text-muted-foreground size-3.5 shrink-0' />
        </PopoverTrigger>
        <PopoverContent
          align='start'
          className='w-64 p-0'
          onWheel={(event) => event.stopPropagation()}
          onTouchMove={(event) => event.stopPropagation()}
          onPointerDown={(event) => event.stopPropagation()}
        >
          <Command>
            <CommandInput placeholder={t('Search...')} />
            <CommandList>
              <CommandEmpty>{t('No group found.')}</CommandEmpty>
              <CommandGroup>
                {groupOptions.map((option) => (
                  <CommandItem
                    key={option.value}
                    value={option.value}
                    onSelect={() => void handleSelect(option.value)}
                    className='items-center gap-2'
                  >
                    <Check
                      className={cn(
                        'size-4',
                        selectedGroup === option.value
                          ? 'opacity-100'
                          : 'opacity-0'
                      )}
                    />
                    <span className='min-w-0 flex-1 truncate'>
                      {option.label}
                    </span>
                    {option.ratio != null && option.value !== 'auto' && (
                      <span className='bg-muted text-muted-foreground rounded px-1.5 py-0.5 font-mono text-[10px] tabular-nums'>
                        {option.ratio}x
                      </span>
                    )}
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
      {selectedGroup === 'auto' && apiKey.cross_group_retry && (
        <StatusBadge
          label={t('Cross-group')}
          variant='info'
          copyable={false}
        />
      )}
    </div>
  )
}

export function useApiKeysColumns(): ColumnDef<ApiKey>[] {
  const { t } = useTranslation()
  const groupOptions = useGroupOptions()
  return [
    {
      id: 'select',
      header: ({ table }) => (
        <Checkbox
          checked={table.getIsAllPageRowsSelected()}
          indeterminate={table.getIsSomePageRowsSelected()}
          onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
          aria-label='Select all'
          className='translate-y-[2px]'
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(!!value)}
          aria-label='Select row'
          className='translate-y-[2px]'
        />
      ),
      enableSorting: false,
      enableHiding: false,
      meta: { label: t('Select') },
    },
    {
      accessorKey: 'name',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Name')} />
      ),
      cell: ({ row }) => (
        <div className='max-w-[200px] truncate font-medium'>
          {row.getValue('name')}
        </div>
      ),
      meta: { label: t('Name'), mobileTitle: true },
    },
    {
      accessorKey: 'status',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Status')} />
      ),
      cell: ({ row }) => {
        const statusConfig = API_KEY_STATUSES[row.getValue('status') as number]
        if (!statusConfig) return null
        return (
          <StatusBadge
            label={t(statusConfig.label)}
            variant={statusConfig.variant}
            copyable={false}
          />
        )
      },
      filterFn: (row, id, value) => value.includes(String(row.getValue(id))),
      meta: { label: t('Status'), mobileBadge: true },
    },
    {
      id: 'key',
      accessorKey: 'key',
      header: t('API Key'),
      cell: ({ row }) => <ApiKeyCell apiKey={row.original} />,
      enableSorting: false,
      meta: { label: t('API Key') },
    },
    {
      id: 'quota',
      accessorKey: 'remain_quota',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Quota')} />
      ),
      cell: ({ row }) => {
        const apiKey = row.original
        if (apiKey.unlimited_quota) {
          return (
            <StatusBadge
              label={t('Unlimited')}
              variant='neutral'
              copyable={false}
            />
          )
        }

        const used = apiKey.used_quota
        const remaining = apiKey.remain_quota
        const total = used + remaining
        const percentage = total > 0 ? (remaining / total) * 100 : 0

        return (
          <Tooltip>
            <TooltipTrigger render={<div className='w-[150px] space-y-1' />}>
              <div className='flex justify-between text-xs'>
                <span className='font-medium tabular-nums'>
                  {formatQuota(remaining)}
                </span>
                <span className='text-muted-foreground tabular-nums'>
                  {formatQuota(total)}
                </span>
              </div>
              <Progress
                value={percentage}
                className={cn('h-1.5', getQuotaProgressColor(percentage))}
              />
            </TooltipTrigger>
            <TooltipContent>
              <div className='space-y-1 text-xs'>
                <div>
                  {t('Used:')} {formatQuota(used)}
                </div>
                <div>
                  {t('Remaining:')} {formatQuota(remaining)} (
                  {percentage.toFixed(1)}%)
                </div>
                <div>
                  {t('Total:')} {formatQuota(total)}
                </div>
              </div>
            </TooltipContent>
          </Tooltip>
        )
      },
      meta: { label: t('Quota') },
    },
    {
      accessorKey: 'group',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Group')} />
      ),
      cell: ({ row }) => {
        const apiKey = row.original
        const group = row.getValue('group') as string
        const ratio =
          group && group !== 'auto'
            ? groupOptions.find((option) => option.value === group)?.ratio
            : undefined

        return (
          <ApiKeyGroupSwitcher
            apiKey={apiKey}
            options={optionListWithCurrentGroup(groupOptions, group, ratio)}
          />
        )
      },
      meta: { label: t('Group'), mobileHidden: true },
    },
    {
      id: 'model_limits',
      accessorKey: 'model_limits',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Models')} />
      ),
      cell: ({ row }) => <ModelLimitsCell apiKey={row.original} />,
      enableSorting: false,
      meta: { label: t('Models'), mobileHidden: true },
    },
    {
      id: 'allow_ips',
      accessorKey: 'allow_ips',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('IP Restriction')} />
      ),
      cell: ({ row }) => <IpRestrictionsCell apiKey={row.original} />,
      enableSorting: false,
      meta: { label: t('IP Restriction'), mobileHidden: true },
    },
    {
      accessorKey: 'created_time',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Created')} />
      ),
      cell: ({ row }) => (
        <span className='text-muted-foreground font-mono text-xs tabular-nums'>
          {formatTimestampToDate(row.getValue('created_time'))}
        </span>
      ),
      meta: { label: t('Created'), mobileHidden: true },
    },
    {
      accessorKey: 'accessed_time',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Last Used')} />
      ),
      cell: ({ row }) => {
        const accessedTime = row.getValue('accessed_time') as number
        if (!accessedTime) {
          return <span className='text-muted-foreground text-xs'>-</span>
        }
        return (
          <span className='text-muted-foreground font-mono text-xs tabular-nums'>
            {formatTimestampToDate(accessedTime)}
          </span>
        )
      },
      meta: { label: t('Last Used'), mobileHidden: true },
    },
    {
      accessorKey: 'expired_time',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Expires')} />
      ),
      cell: ({ row }) => {
        const expiredTime = row.getValue('expired_time') as number
        if (expiredTime === -1) {
          return (
            <StatusBadge
              label={t('Never')}
              variant='neutral'
              copyable={false}
            />
          )
        }
        const isExpired = expiredTime * 1000 < Date.now()
        return (
          <span
            className={cn(
              'font-mono text-xs tabular-nums',
              isExpired ? 'text-destructive' : 'text-muted-foreground'
            )}
          >
            {formatTimestampToDate(expiredTime)}
          </span>
        )
      },
      meta: { label: t('Expires'), mobileHidden: true },
    },
    {
      id: 'actions',
      header: () => <div className='text-right'>{t('Actions')}</div>,
      cell: ({ row }) => <DataTableRowActions row={row} />,
      meta: {
        label: t('Actions'),
        headerClassName:
          'sticky right-0 z-20 min-w-[280px] bg-background text-right shadow-[-12px_0_16px_-16px_hsl(var(--border))]',
        cellClassName:
          'sticky right-0 z-10 min-w-[280px] bg-background shadow-[-12px_0_16px_-16px_hsl(var(--border))]',
      },
      size: 280,
    },
  ]
}
