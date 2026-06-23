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
import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Activity, Clock3, Layers3, Radio, Server } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { getCurrencyLabel, isCurrencyDisplayEnabled } from '@/lib/currency'
import { formatNumber, formatQuota } from '@/lib/format'
import { computeTimeRange } from '@/lib/time'
import { cn } from '@/lib/utils'
import { useStatus } from '@/hooks/use-status'
import { useSystemConfig } from '@/hooks/use-system-config'
import { StaggerContainer, StaggerItem } from '@/components/page-transition'
import { getUserQuotaDates } from '@/features/dashboard/api'
import { useSummaryCardsConfig } from '@/features/dashboard/hooks/use-dashboard-config'
import type { QuotaDataItem } from '@/features/dashboard/types'
import { getDashboard } from '@/features/sunapi/api'
import { StatCard } from '../ui/stat-card'

const SUMMARY_SPARKLINE_BUCKETS = 12

type SummarySparklineKey = 'balance' | 'usage' | 'requests'

function costToQuotaUnits(cost: number | null | undefined, quotaPerUnit: number) {
  const numericCost = Number(cost) || 0
  const unit = Number(quotaPerUnit) || 0
  return Math.round(numericCost * unit)
}

function getBucketIndex(
  timestamp: number,
  start: number,
  end: number,
  bucketCount: number
): number {
  if (end <= start) return 0
  const ratio = (timestamp - start) / (end - start)
  return Math.min(bucketCount - 1, Math.max(0, Math.floor(ratio * bucketCount)))
}

function buildSummarySparklines(
  data: QuotaDataItem[],
  currentBalance: number,
  start: number,
  end: number
): Record<SummarySparklineKey, number[]> {
  const usage = Array.from({ length: SUMMARY_SPARKLINE_BUCKETS }, () => 0)
  const requests = Array.from({ length: SUMMARY_SPARKLINE_BUCKETS }, () => 0)

  for (const item of data) {
    const timestamp = Number(item.created_at) || start
    const index = getBucketIndex(
      timestamp,
      start,
      end,
      SUMMARY_SPARKLINE_BUCKETS
    )
    usage[index] += Number(item.quota) || 0
    requests[index] += Number(item.count) || 0
  }

  let balance = currentBalance
  const balanceTrend = Array.from(
    { length: SUMMARY_SPARKLINE_BUCKETS },
    () => 0
  )

  for (let index = SUMMARY_SPARKLINE_BUCKETS - 1; index >= 0; index--) {
    balanceTrend[index] = Math.max(0, balance)
    balance += usage[index]
  }

  return {
    balance: balanceTrend,
    usage,
    requests,
  }
}

function getSummarySparkline(
  key: string,
  sparklineData: Record<SummarySparklineKey, number[]>
): number[] | undefined {
  if (key === 'usage') return sparklineData.usage
  if (key === 'requests') return sparklineData.requests
  return undefined
}

function getRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object'
    ? (value as Record<string, unknown>)
    : null
}

function getStatusString(status: unknown, key: string): string | undefined {
  const root = getRecord(status)
  const nested = getRecord(root?.data)
  const value = root?.[key] ?? nested?.[key]
  return typeof value === 'string' && value.trim() ? value.trim() : undefined
}

function getStatusNumber(status: unknown, key: string): number | undefined {
  const root = getRecord(status)
  const nested = getRecord(root?.data)
  const value = root?.[key] ?? nested?.[key]
  const numeric = Number(value)
  return Number.isFinite(numeric) && numeric > 0 ? numeric : undefined
}

function getBrowserOrigin(): string {
  if (typeof window === 'undefined') return ''
  return window.location.origin
}

function normalizeServerAddress(address: string): string {
  return address.replace(/\/+$/, '')
}

function buildEndpointURL(address: string): string {
  const normalized = normalizeServerAddress(address)
  return normalized || '/'
}

function formatUptime(
  startTime: number | undefined,
  labels: { days: string; hours: string; minutes: string; seconds: string }
): string {
  if (!startTime) return '-'

  const totalSeconds = Math.max(0, Math.floor(Date.now() / 1000 - startTime))
  const days = Math.floor(totalSeconds / 86400)
  const hours = Math.floor((totalSeconds % 86400) / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)

  if (days > 0) return `${days} ${labels.days} ${hours} ${labels.hours}`
  if (hours > 0) return `${hours} ${labels.hours} ${minutes} ${labels.minutes}`
  if (minutes > 0) return `${minutes} ${labels.minutes}`
  return `${totalSeconds} ${labels.seconds}`
}

export function SummaryCards() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const { status, loading } = useStatus()
  const { currency } = useSystemConfig()

  const summaryTimeRange = useMemo(() => computeTimeRange(1), [])
  const remainQuota = Number(user?.quota ?? 0)

  const usageTrendQuery = useQuery({
    queryKey: [
      'dashboard',
      'overview',
      'summary-sparklines',
      summaryTimeRange.start_timestamp,
      summaryTimeRange.end_timestamp,
    ],
    queryFn: async () =>
      getUserQuotaDates({
        start_timestamp: summaryTimeRange.start_timestamp,
        end_timestamp: summaryTimeRange.end_timestamp,
        default_time: 'hour',
      }),
    staleTime: 60 * 1000,
  })

  const localDashboardQuery = useQuery({
    queryKey: ['sunapi', 'dashboard', 'local-overview'],
    queryFn: getDashboard,
    staleTime: 60 * 1000,
  })

  const dashboard = localDashboardQuery.data
  const totalUsedQuota = dashboard
    ? costToQuotaUnits(dashboard.total_cost, currency.quotaPerUnit)
    : Number(user?.used_quota ?? 0)
  const totalRequestCount = dashboard
    ? Number(dashboard.total_requests) || 0
    : Number(user?.request_count ?? 0)

  const summaryValues = useMemo(() => {
    return {
      usedDisplay: formatQuota(totalUsedQuota),
      requestCountDisplay: formatNumber(totalRequestCount),
    }
  }, [totalRequestCount, totalUsedQuota])

  const currencyEnabledFromStore = isCurrencyDisplayEnabled()
  const statusCurrencyFlag =
    typeof status?.display_in_currency === 'boolean'
      ? Boolean(status.display_in_currency)
      : undefined
  const currencyEnabled =
    statusCurrencyFlag !== undefined
      ? statusCurrencyFlag
      : currencyEnabledFromStore
  const currencyLabel = currencyEnabled ? getCurrencyLabel() : 'Tokens'

  const sparklineData = useMemo(
    () =>
      buildSummarySparklines(
        usageTrendQuery.data?.data ?? [],
        remainQuota,
        summaryTimeRange.start_timestamp,
        summaryTimeRange.end_timestamp
      ),
    [
      remainQuota,
      summaryTimeRange.end_timestamp,
      summaryTimeRange.start_timestamp,
      usageTrendQuery.data?.data,
    ]
  )

  const recentUsage = useMemo(
    () =>
      (usageTrendQuery.data?.data ?? []).reduce(
        (total, item) => total + (Number(item.quota) || 0),
        0
      ),
    [usageTrendQuery.data?.data]
  )

  const todayUsage = dashboard
    ? costToQuotaUnits(dashboard.last_24h_cost, currency.quotaPerUnit)
    : recentUsage
  const todayUsageDisplay = formatQuota(todayUsage)

  const serverAddress = useMemo(
    () =>
      normalizeServerAddress(
        getStatusString(status, 'server_address') ?? getBrowserOrigin()
      ),
    [status]
  )
  const endpointURL = useMemo(
    () => buildEndpointURL(serverAddress),
    [serverAddress]
  )
  const uptimeDisplay = useMemo(
    () =>
      formatUptime(getStatusNumber(status, 'start_time'), {
        days: t('days'),
        hours: t('hours'),
        minutes: t('minutes'),
        seconds: t('seconds'),
      }),
    [status, t]
  )

  const localOverviewItems = useMemo(
    () => [
      {
        key: 'uptime',
        label: t('Uptime'),
        value: uptimeDisplay,
        icon: Clock3,
      },
      {
        key: 'channels',
        label: t('Channels'),
        value: dashboard
          ? `${formatNumber(dashboard.enabled_channels)} / ${formatNumber(
              dashboard.total_channels
            )}`
          : '-',
        icon: Radio,
      },
      {
        key: 'groups',
        label: t('Groups'),
        value: dashboard ? formatNumber(dashboard.groups) : '-',
        icon: Layers3,
      },
      {
        key: 'tokens',
        label: t('Total Tokens'),
        value: dashboard ? formatNumber(dashboard.total_tokens) : '-',
        icon: Activity,
      },
    ],
    [dashboard, t, uptimeDisplay]
  )

  const items = useSummaryCardsConfig({
    ...summaryValues,
    todayUsageDisplay,
    currencyEnabled,
    currencyLabel,
  }).map((config, index) => {
    const tones = ['rose', 'teal', 'gray'] as const

    return {
      key: config.key,
      title: config.title,
      value: config.value,
      desc: config.description,
      icon: config.icon,
      tone: tones[index] ?? 'gray',
      sparkline:
        config.key === 'todayUsage'
          ? sparklineData.usage
          : getSummarySparkline(config.key, sparklineData),
      sparklineVariant: 'line' as const,
    }
  })

  return (
    <div className='bg-card overflow-hidden rounded-2xl border shadow-xs'>
      <div className='grid xl:grid-cols-[minmax(0,1fr)_19rem]'>
        <div className='flex flex-col gap-3 p-4 sm:p-5'>
          <div className='flex flex-wrap items-start justify-between gap-3'>
            <div className='flex flex-col gap-1'>
              <h3 className='text-base font-semibold'>{t('Usage at a glance')}</h3>
              <p className='text-muted-foreground text-sm'>
                {t('Aggregated usage metrics and trend charts.')}
              </p>
            </div>
          </div>
          <StaggerContainer className='grid gap-3 md:grid-cols-3'>
            {items.map((it) => (
              <StaggerItem
                key={it.key}
                className='bg-background/60 rounded-xl border p-3'
              >
                <StatCard
                  title={it.title}
                  value={it.value}
                  description={it.desc}
                  icon={it.icon}
                  tone={it.tone}
                  sparkline={it.sparkline}
                  sparklineVariant={it.sparklineVariant}
                  loading={loading}
                />
              </StaggerItem>
            ))}
          </StaggerContainer>
        </div>

        <div className='bg-muted/30 flex flex-col justify-between gap-4 border-t p-4 sm:p-5 xl:border-t-0 xl:border-l'>
          <div className='flex flex-col gap-3'>
            <div className='flex items-center justify-between'>
              <span className='text-muted-foreground text-xs font-medium'>
                {t('Local')} {t('Overview')}
              </span>
              <span className='flex items-center gap-1.5'>
                <span
                  className={cn(
                    'size-1.5 rounded-full',
                    localDashboardQuery.isError ? 'bg-destructive' : 'bg-success'
                  )}
                  aria-hidden='true'
                />
                <span className='text-muted-foreground text-[11px] font-medium'>
                  {localDashboardQuery.isError ? t('Disabled') : t('Running')}
                </span>
              </span>
            </div>

            <div className='bg-background/70 rounded-xl border px-3 py-2'>
              <div className='flex min-w-0 items-center gap-2'>
                <div className='text-muted-foreground flex shrink-0 items-center gap-1.5 text-[11px] leading-none font-medium'>
                  <Server className='size-3 shrink-0' aria-hidden='true' />
                  <span>{t('Endpoint')}</span>
                </div>
                <div
                  className='text-foreground min-w-0 flex-1 truncate whitespace-nowrap font-mono text-sm font-semibold'
                  title={endpointURL}
                >
                  {endpointURL}
                </div>
              </div>
            </div>

            <div className='mt-3 grid grid-cols-2 gap-2'>
              {localOverviewItems.map((item) => {
                const Icon = item.icon
                return (
                  <div
                    key={item.key}
                    className='bg-background/70 rounded-lg border border-transparent px-2.5 py-2'
                  >
                    <div className='text-muted-foreground flex items-center gap-1 text-[11px] leading-none font-medium'>
                      <Icon className='size-3 shrink-0' aria-hidden='true' />
                      <span className='truncate'>{item.label}</span>
                    </div>
                    <div
                      className='text-foreground mt-1.5 truncate text-xs font-semibold tabular-nums'
                      title={item.value}
                    >
                      {item.value}
                    </div>
                  </div>
                )
              })}
            </div>
          </div>

        </div>
      </div>
    </div>
  )
}
