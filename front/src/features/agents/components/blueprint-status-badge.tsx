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
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { BlueprintStatus } from '../types'

type BlueprintStatusBadgeProps = {
  status: BlueprintStatus
  className?: string
}

const STATUS_CONFIG: Record<
  BlueprintStatus,
  { label: string; dot: string; badge: string }
> = {
  [BlueprintStatus.RUNNING]: {
    label: '执行中',
    dot: 'bg-blue-500 animate-pulse',
    badge: 'bg-blue-50 text-blue-700 border-blue-200 dark:bg-blue-950 dark:text-blue-300 dark:border-blue-800',
  },
  [BlueprintStatus.PENDING]: {
    label: '待执行',
    dot: 'bg-amber-500',
    badge: 'bg-amber-50 text-amber-700 border-amber-200 dark:bg-amber-950 dark:text-amber-300 dark:border-amber-800',
  },
  [BlueprintStatus.COMPLETED]: {
    label: '已结束',
    dot: 'bg-emerald-500',
    badge: 'bg-emerald-50 text-emerald-700 border-emerald-200 dark:bg-emerald-950 dark:text-emerald-300 dark:border-emerald-800',
  },
  [BlueprintStatus.FAILED]: {
    label: '执行失败',
    dot: 'bg-red-500',
    badge: 'bg-red-50 text-red-700 border-red-200 dark:bg-red-950 dark:text-red-300 dark:border-red-800',
  },
  [BlueprintStatus.PAUSED]: {
    label: '已暂停',
    dot: 'bg-slate-400',
    badge: 'bg-slate-50 text-slate-600 border-slate-200 dark:bg-slate-900 dark:text-slate-400 dark:border-slate-700',
  },
}

export function BlueprintStatusBadge({
  status,
  className,
}: BlueprintStatusBadgeProps) {
  const { t } = useTranslation()
  const config = STATUS_CONFIG[status]

  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-xs font-medium',
        config.badge,
        className
      )}
    >
      <span className={cn('size-1.5 rounded-full', config.dot)} />
      {t(config.label)}
    </span>
  )
}
