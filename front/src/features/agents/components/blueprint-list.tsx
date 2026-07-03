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
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { formatDistanceToNow } from 'date-fns'
import { zhCN } from 'date-fns/locale'
import {
  AlertTriangle,
  Clock,
  MoreHorizontal,
  Pause,
  Play,
  Plus,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  blueprintsByAgentQueryOptions,
  useDeleteBlueprintMutation,
  usePauseBlueprintMutation,
  useResumeBlueprintMutation,
  useRetryBlueprintMutation,
} from '../api'
import { BlueprintStatus, type Blueprint } from '../types'
import { BlueprintStatusBadge } from './blueprint-status-badge'

type BlueprintListProps = {
  agentId: string
  onCreateBlueprint?: () => void
  onViewBlueprint?: (blueprint: Blueprint) => void
}

const FREQUENCY_LABELS: Record<string, string> = {
  once: '一次性',
  hourly: '每小时',
  daily: '每天',
  weekly: '每周',
  monthly: '每月',
  custom: '自定义',
}

function formatDuration(ms?: number): string {
  if (!ms) return '-'
  if (ms < 60000) return `${Math.round(ms / 1000)}s`
  return `${Math.round(ms / 60000)}m ${Math.round((ms % 60000) / 1000)}s`
}

function BlueprintCard({
  blueprint,
  onView,
}: {
  blueprint: Blueprint
  onView?: () => void
}) {
  const { t } = useTranslation()
  const deleteMutation = useDeleteBlueprintMutation(blueprint.agent_id)
  const pauseMutation = usePauseBlueprintMutation(blueprint.agent_id)
  const resumeMutation = useResumeBlueprintMutation(blueprint.agent_id)
  const retryMutation = useRetryBlueprintMutation(blueprint.agent_id)

  const handleDelete = () => {
    deleteMutation.mutate(blueprint.id, {
      onSuccess: () => toast.success(t('Blueprint deleted')),
    })
  }

  const handlePause = () => {
    pauseMutation.mutate(blueprint.id, {
      onSuccess: () => toast.success(t('Blueprint paused')),
    })
  }

  const handleResume = () => {
    resumeMutation.mutate(blueprint.id, {
      onSuccess: () => toast.success(t('Blueprint resumed')),
    })
  }

  const handleRetry = () => {
    retryMutation.mutate(blueprint.id, {
      onSuccess: () => toast.success(t('Blueprint retrying')),
    })
  }

  const isRunning = blueprint.status === BlueprintStatus.RUNNING
  const isPaused = blueprint.status === BlueprintStatus.PAUSED
  const isFailed = blueprint.status === BlueprintStatus.FAILED
  const isPending = blueprint.status === BlueprintStatus.PENDING

  return (
    <div className='border-border/60 hover:bg-muted/20 group flex flex-col gap-2 rounded-lg border p-3 transition-colors sm:flex-row sm:items-start sm:gap-4 sm:p-4'>
      {/* 左侧：状态徽章 + 名称 */}
      <div className='flex min-w-0 flex-1 flex-col gap-1'>
        <div className='flex flex-wrap items-center gap-2'>
          <BlueprintStatusBadge status={blueprint.status} />
          <span className='truncate text-sm font-medium'>{blueprint.name}</span>
        </div>
        {blueprint.description && (
          <p className='text-muted-foreground line-clamp-1 text-xs'>
            {blueprint.description}
          </p>
        )}

        {/* 执行信息行 */}
        <div className='text-muted-foreground mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs'>
          {blueprint.trigger.frequency && (
            <span className='flex items-center gap-1'>
              <Clock className='h-3 w-3' aria-hidden='true' />
              {FREQUENCY_LABELS[blueprint.trigger.frequency] ?? blueprint.trigger.frequency}
              {blueprint.trigger.time && ` ${blueprint.trigger.time}`}
            </span>
          )}

          {blueprint.next_execution_time && isPending && (
            <span className='text-amber-600 dark:text-amber-400'>
              下次执行:{' '}
              {formatDistanceToNow(blueprint.next_execution_time, {
                addSuffix: true,
                locale: zhCN,
              })}
            </span>
          )}

          {blueprint.last_executed_time && (isRunning || isPaused) && (
            <span>
              上次执行:{' '}
              {formatDistanceToNow(blueprint.last_executed_time, {
                addSuffix: true,
                locale: zhCN,
              })}
            </span>
          )}

          {blueprint.status === BlueprintStatus.COMPLETED && (
            <>
              {blueprint.execution_duration && (
                <span>耗时: {formatDuration(blueprint.execution_duration)}</span>
              )}
              {blueprint.execution_count > 0 && (
                <span>共执行 {blueprint.execution_count} 次</span>
              )}
            </>
          )}

          {isFailed && blueprint.failure_reason && (
            <span className='flex items-center gap-1 text-red-600 dark:text-red-400'>
              <AlertTriangle className='h-3 w-3' aria-hidden='true' />
              {blueprint.failure_reason}
            </span>
          )}
        </div>
      </div>

      {/* 右侧：操作按钮 */}
      <div className='flex shrink-0 items-center gap-1.5'>
        {isRunning && (
          <Button
            size='sm'
            variant='outline'
            className='h-7 gap-1.5 px-2.5 text-xs'
            onClick={handlePause}
            disabled={pauseMutation.isPending}
          >
            <Pause className='h-3 w-3' aria-hidden='true' />
            {t('Pause')}
          </Button>
        )}

        {isPaused && (
          <Button
            size='sm'
            variant='outline'
            className='h-7 gap-1.5 px-2.5 text-xs'
            onClick={handleResume}
            disabled={resumeMutation.isPending}
          >
            <Play className='h-3 w-3' aria-hidden='true' />
            {t('Resume')}
          </Button>
        )}

        {isFailed && (
          <Button
            size='sm'
            variant='outline'
            className='h-7 gap-1.5 px-2.5 text-xs'
            onClick={handleRetry}
            disabled={retryMutation.isPending}
          >
            <RefreshCw className='h-3 w-3' aria-hidden='true' />
            {t('Retry')}
          </Button>
        )}

        <Button
          size='sm'
          variant='ghost'
          className='h-7 px-2.5 text-xs'
          onClick={onView}
        >
          {t('View')}
        </Button>

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              size='sm'
              variant='ghost'
              className='h-7 w-7 p-0'
              aria-label={t('More actions')}
            >
              <MoreHorizontal className='h-4 w-4' aria-hidden='true' />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align='end' className='w-40'>
            <DropdownMenuItem onClick={onView}>
              {t('View details')}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              className='text-destructive focus:text-destructive'
              onClick={handleDelete}
            >
              <Trash2 className='mr-2 h-4 w-4' aria-hidden='true' />
              {t('Delete')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  )
}

export function BlueprintList({
  agentId,
  onCreateBlueprint,
  onViewBlueprint,
}: BlueprintListProps) {
  const { t } = useTranslation()
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const [search, setSearch] = useState('')

  const { data: blueprints = [], isLoading } = useQuery(
    blueprintsByAgentQueryOptions(agentId)
  )

  const filtered = blueprints.filter((bp) => {
    const matchStatus = statusFilter === 'all' || bp.status === statusFilter
    const matchSearch =
      !search || bp.name.toLowerCase().includes(search.toLowerCase())
    return matchStatus && matchSearch
  })

  return (
    <div className='flex flex-col gap-3'>
      {/* 顶部工具栏 */}
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div className='flex flex-wrap items-center gap-2'>
          {/* 状态筛选 */}
          <Select value={statusFilter} onValueChange={setStatusFilter}>
            <SelectTrigger className='h-8 w-[110px] text-xs'>
              <SelectValue placeholder={t('All status')} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='all'>{t('All')}</SelectItem>
              <SelectItem value={BlueprintStatus.RUNNING}>
                {t('Running')}
              </SelectItem>
              <SelectItem value={BlueprintStatus.PENDING}>
                {t('Pending')}
              </SelectItem>
              <SelectItem value={BlueprintStatus.COMPLETED}>
                {t('Completed')}
              </SelectItem>
              <SelectItem value={BlueprintStatus.FAILED}>
                {t('Failed')}
              </SelectItem>
              <SelectItem value={BlueprintStatus.PAUSED}>
                {t('Paused')}
              </SelectItem>
            </SelectContent>
          </Select>

          {/* 搜索框 */}
          <div className='relative'>
            <Input
              className='h-8 w-[180px] pr-3 text-xs'
              placeholder={t('Search blueprints...')}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
        </div>

        <Button
          size='sm'
          className='h-8 gap-1.5 text-xs'
          onClick={onCreateBlueprint}
        >
          <Plus className='h-3.5 w-3.5' aria-hidden='true' />
          {t('New Blueprint')}
        </Button>
      </div>

      {/* 蓝图列表 */}
      {isLoading ? (
        <div className='flex flex-col gap-2'>
          {[1, 2, 3].map((i) => (
            <div
              key={i}
              className='bg-muted/40 h-[72px] animate-pulse rounded-lg'
            />
          ))}
        </div>
      ) : filtered.length === 0 ? (
        <div className='text-muted-foreground flex flex-col items-center justify-center py-10 text-sm'>
          <p>{t('No blueprints found')}</p>
          {blueprints.length === 0 && (
            <Button
              size='sm'
              variant='outline'
              className='mt-3'
              onClick={onCreateBlueprint}
            >
              <Plus className='mr-1.5 h-3.5 w-3.5' aria-hidden='true' />
              {t('Create your first blueprint')}
            </Button>
          )}
        </div>
      ) : (
        <div className='flex flex-col gap-2'>
          {filtered.map((bp) => (
            <BlueprintCard
              key={bp.id}
              blueprint={bp}
              onView={() => onViewBlueprint?.(bp)}
            />
          ))}
        </div>
      )}
    </div>
  )
}
