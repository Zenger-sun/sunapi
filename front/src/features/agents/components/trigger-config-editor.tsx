/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useEffect, useMemo, useState } from 'react'
import { addDays, format } from 'date-fns'
import { zhCN } from 'date-fns/locale'
import { CalendarIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Calendar } from '@/components/ui/calendar'
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
import { FrequencyType, TriggerType, type TriggerConfig } from '../types'

// ============================================================================
// 常用时区列表
// ============================================================================

const COMMON_TIMEZONES = [
  { value: 'Asia/Shanghai', label: 'Asia/Shanghai (UTC+8)', short: 'CST' },
  { value: 'Asia/Tokyo', label: 'Asia/Tokyo (UTC+9)', short: 'JST' },
  { value: 'Asia/Seoul', label: 'Asia/Seoul (UTC+9)', short: 'KST' },
  {
    value: 'America/New_York',
    label: 'America/New_York (UTC-5)',
    short: 'EST',
  },
  {
    value: 'America/Los_Angeles',
    label: 'America/Los_Angeles (UTC-8)',
    short: 'PST',
  },
  { value: 'Europe/London', label: 'Europe/London (UTC+0)', short: 'GMT' },
  { value: 'Europe/Paris', label: 'Europe/Paris (UTC+1)', short: 'CET' },
  { value: 'UTC', label: 'UTC (UTC+0)', short: 'UTC' },
]

const WEEKDAY_LABELS = ['日', '一', '二', '三', '四', '五', '六']

// ============================================================================
// 时间选择器（小时+分钟下拉）
// ============================================================================

type TimeSelectorProps = {
  value?: string // "HH:mm"
  onChange: (value: string) => void
}

function TimeSelector({ value, onChange }: TimeSelectorProps) {
  const [hour, minute] = (value ?? '08:00').split(':')

  const hours = Array.from({ length: 24 }, (_, i) =>
    String(i).padStart(2, '0')
  )
  const minutes = ['00', '05', '10', '15', '20', '25', '30', '35', '40', '45', '50', '55']

  return (
    <div className='flex items-center gap-1'>
      <Select
        value={hour}
        onValueChange={(h) => onChange(`${h}:${minute ?? '00'}`)}
      >
        <SelectTrigger className='h-8 w-[72px] text-xs'>
          <SelectValue />
        </SelectTrigger>
        <SelectContent className='max-h-48'>
          {hours.map((h) => (
            <SelectItem key={h} value={h} className='text-xs'>
              {h}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <span className='text-muted-foreground text-sm font-medium'>:</span>
      <Select
        value={minute}
        onValueChange={(m) => onChange(`${hour ?? '08'}:${m}`)}
      >
        <SelectTrigger className='h-8 w-[72px] text-xs'>
          <SelectValue />
        </SelectTrigger>
        <SelectContent className='max-h-48'>
          {minutes.map((m) => (
            <SelectItem key={m} value={m} className='text-xs'>
              {m}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}

// ============================================================================
// 主组件
// ============================================================================

type TriggerConfigEditorProps = {
  value: TriggerConfig
  onChange: (value: TriggerConfig) => void
}

export function TriggerConfigEditor({
  value,
  onChange,
}: TriggerConfigEditorProps) {
  const { t } = useTranslation()

  const update = (patch: Partial<TriggerConfig>) =>
    onChange({ ...value, ...patch })

  // 下次执行时间预览
  const nextExecution = useMemo(() => {
    if (value.type !== TriggerType.SCHEDULED || !value.time) return null
    const [h, m] = value.time.split(':').map(Number)
    const now = new Date()
    const next = new Date()
    next.setHours(h, m, 0, 0)
    if (next <= now) next.setDate(next.getDate() + 1)
    return next
  }, [value])

  const toggleWeekday = (day: number) => {
    const current = value.weekdays ?? []
    const next = current.includes(day)
      ? current.filter((d) => d !== day)
      : [...current, day].sort()
    update({ weekdays: next })
  }

  return (
    <div className='flex flex-col gap-4'>
      {/* 触发方式 */}
      <div className='flex items-center gap-3'>
        <label className='text-muted-foreground w-16 shrink-0 text-xs'>
          {t('Trigger')}
        </label>
        <div className='flex gap-1.5'>
          {[
            { value: TriggerType.SCHEDULED, label: '定时执行' },
            { value: TriggerType.MANUAL, label: '手动触发' },
            { value: TriggerType.WEBHOOK, label: 'Webhook' },
          ].map((opt) => (
            <button
              key={opt.value}
              type='button'
              onClick={() => update({ type: opt.value })}
              className={cn(
                'rounded-md border px-3 py-1.5 text-xs font-medium transition-colors',
                value.type === opt.value
                  ? 'bg-primary text-primary-foreground border-primary'
                  : 'border-border hover:bg-muted/60'
              )}
            >
              {t(opt.label)}
            </button>
          ))}
        </div>
      </div>

      {/* 定时配置 */}
      {value.type === TriggerType.SCHEDULED && (
        <>
          {/* 频率 */}
          <div className='flex items-center gap-3'>
            <label className='text-muted-foreground w-16 shrink-0 text-xs'>
              {t('Frequency')}
            </label>
            <Select
              value={value.frequency ?? FrequencyType.DAILY}
              onValueChange={(v) =>
                update({ frequency: v as FrequencyType, weekdays: [], dayOfMonth: undefined })
              }
            >
              <SelectTrigger className='h-8 w-[140px] text-xs'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {[
                  { value: FrequencyType.ONCE, label: '一次性' },
                  { value: FrequencyType.HOURLY, label: '每小时' },
                  { value: FrequencyType.DAILY, label: '每天' },
                  { value: FrequencyType.WEEKLY, label: '每周' },
                  { value: FrequencyType.MONTHLY, label: '每月' },
                  { value: FrequencyType.CUSTOM, label: '自定义...' },
                ].map((opt) => (
                  <SelectItem key={opt.value} value={opt.value} className='text-xs'>
                    {t(opt.label)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* 执行时间（非每小时显示） */}
          {value.frequency !== FrequencyType.HOURLY && (
            <div className='flex items-center gap-3'>
              <label className='text-muted-foreground w-16 shrink-0 text-xs'>
                {t('Time')}
              </label>
              <TimeSelector
                value={value.time}
                onChange={(t) => update({ time: t })}
              />
            </div>
          )}

          {/* 每周执行日 */}
          {value.frequency === FrequencyType.WEEKLY && (
            <div className='flex items-start gap-3'>
              <label className='text-muted-foreground mt-1.5 w-16 shrink-0 text-xs'>
                {t('Days')}
              </label>
              <div className='flex flex-wrap gap-1.5'>
                {WEEKDAY_LABELS.map((label, idx) => {
                  const selected = (value.weekdays ?? []).includes(idx)
                  return (
                    <button
                      key={idx}
                      type='button'
                      onClick={() => toggleWeekday(idx)}
                      className={cn(
                        'flex h-7 w-7 items-center justify-center rounded-full border text-xs font-medium transition-colors',
                        selected
                          ? 'bg-primary text-primary-foreground border-primary'
                          : 'border-border hover:bg-muted/60'
                      )}
                    >
                      {label}
                    </button>
                  )
                })}
              </div>
            </div>
          )}

          {/* 每月执行日 */}
          {value.frequency === FrequencyType.MONTHLY && (
            <div className='flex items-center gap-3'>
              <label className='text-muted-foreground w-16 shrink-0 text-xs'>
                {t('Day of month')}
              </label>
              <Select
                value={String(value.dayOfMonth ?? 1)}
                onValueChange={(v) => update({ dayOfMonth: Number(v) })}
              >
                <SelectTrigger className='h-8 w-[100px] text-xs'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent className='max-h-48'>
                  {Array.from({ length: 28 }, (_, i) => i + 1).map((d) => (
                    <SelectItem key={d} value={String(d)} className='text-xs'>
                      {d} 日
                    </SelectItem>
                  ))}
                  <SelectItem value='31' className='text-xs'>
                    最后一天
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>
          )}

          {/* 一次性：日期选择 */}
          {value.frequency === FrequencyType.ONCE && (
            <div className='flex items-center gap-3'>
              <label className='text-muted-foreground w-16 shrink-0 text-xs'>
                {t('Date')}
              </label>
              <Popover>
                <PopoverTrigger asChild>
                  <Button
                    variant='outline'
                    className='h-8 w-[160px] justify-start gap-2 text-xs font-normal'
                  >
                    <CalendarIcon className='h-3.5 w-3.5' aria-hidden='true' />
                    {value.time
                      ? format(addDays(new Date(), 1), 'yyyy-MM-dd')
                      : t('Pick a date')}
                  </Button>
                </PopoverTrigger>
                <PopoverContent className='w-auto p-0' align='start'>
                  <Calendar
                    mode='single'
                    locale={zhCN}
                    disabled={(d) => d < new Date()}
                  />
                </PopoverContent>
              </Popover>
            </div>
          )}

          {/* 时区 */}
          <div className='flex items-center gap-3'>
            <label className='text-muted-foreground w-16 shrink-0 text-xs'>
              {t('Timezone')}
            </label>
            <Select
              value={value.timezone}
              onValueChange={(v) => update({ timezone: v })}
            >
              <SelectTrigger className='h-8 flex-1 text-xs'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {COMMON_TIMEZONES.map((tz) => (
                  <SelectItem key={tz.value} value={tz.value} className='text-xs'>
                    {tz.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* 下次执行时间预览 */}
          {nextExecution && (
            <div className='bg-muted/40 rounded-md px-3 py-2.5 text-xs'>
              <span className='text-muted-foreground'>下次执行时间：</span>
              <span className='font-medium'>
                {format(nextExecution, 'yyyy-MM-dd HH:mm:ss')}
              </span>
              <span className='text-muted-foreground ml-1'>
                (
                {COMMON_TIMEZONES.find((z) => z.value === value.timezone)?.short ??
                  value.timezone}
                )
              </span>
            </div>
          )}
        </>
      )}

      {/* Webhook 配置提示 */}
      {value.type === TriggerType.WEBHOOK && (
        <div className='bg-muted/40 rounded-md px-3 py-2.5 text-xs'>
          <p className='text-muted-foreground'>
            保存后将生成专属 Webhook 地址，向该地址发送 POST 请求即可触发任务。
          </p>
        </div>
      )}
    </div>
  )
}
