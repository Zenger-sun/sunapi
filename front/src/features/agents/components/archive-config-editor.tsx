/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { ArchiveType, type ArchiveConfig } from '../types'

type ArchiveConfigEditorProps = {
  value?: ArchiveConfig
  onChange: (value: ArchiveConfig) => void
}

const ARCHIVE_TYPES = [
  { value: ArchiveType.PLATFORM, label: '平台内置存储', icon: '🗄️' },
  { value: ArchiveType.NOTION, label: 'Notion', icon: '📓' },
  { value: ArchiveType.FEISHU, label: '飞书文档', icon: '📋' },
  { value: ArchiveType.GITHUB, label: 'GitHub 仓库', icon: '🐙' },
  { value: ArchiveType.WECHAT, label: '企业微信', icon: '💬' },
  { value: ArchiveType.WEBHOOK, label: '自定义 Webhook', icon: '🔗' },
  { value: ArchiveType.LOCAL, label: '本地文件系统', icon: '💾' },
]

const NAMING_RULES = [
  { value: '{{agentName}}_{{date}}', label: '智能体名_日期' },
  { value: '{{agentName}}_{{date}}_{{index}}', label: '智能体名_日期_序号' },
  { value: '{{date}}_{{agentName}}', label: '日期_智能体名' },
  { value: '{{timestamp}}', label: '时间戳' },
]

const RETENTION_POLICIES = [
  { value: 'keep-10', label: '保留最近 10 条' },
  { value: 'keep-30', label: '保留最近 30 条' },
  { value: 'keep-100', label: '保留最近 100 条' },
  { value: 'keep-all', label: '永久保留' },
  { value: 'keep-7d', label: '保留 7 天' },
  { value: 'keep-30d', label: '保留 30 天' },
]

const DEFAULT_VALUE: ArchiveConfig = {
  type: ArchiveType.PLATFORM,
  path: '/archives',
  namingRule: '{{agentName}}_{{date}}',
  retentionPolicy: 'keep-30',
  triggerOn: 'success',
}

export function ArchiveConfigEditor({
  value = DEFAULT_VALUE,
  onChange,
}: ArchiveConfigEditorProps) {
  const { t } = useTranslation()

  const update = (patch: Partial<ArchiveConfig>) =>
    onChange({ ...value, ...patch })

  const updateExternal = (key: string, val: string) =>
    onChange({
      ...value,
      externalConfig: { ...(value.externalConfig ?? {}), [key]: val },
    })

  const selectedType = ARCHIVE_TYPES.find((a) => a.value === value.type)

  return (
    <div className='flex flex-col gap-4'>
      {/* 归档位置类型 */}
      <div className='flex items-start gap-3'>
        <label className='text-muted-foreground mt-2 w-16 shrink-0 text-xs'>
          {t('Storage')}
        </label>
        <div className='flex flex-1 flex-wrap gap-2'>
          {ARCHIVE_TYPES.map((opt) => (
            <button
              key={opt.value}
              type='button'
              onClick={() => update({ type: opt.value })}
              className={cn(
                'flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors',
                value.type === opt.value
                  ? 'bg-primary/10 border-primary text-primary'
                  : 'border-border hover:bg-muted/60'
              )}
            >
              <span>{opt.icon}</span>
              {t(opt.label)}
            </button>
          ))}
        </div>
      </div>

      {/* 平台内置存储 */}
      {value.type === ArchiveType.PLATFORM && (
        <>
          <div className='flex items-center gap-3'>
            <label className='text-muted-foreground w-16 shrink-0 text-xs'>
              {t('Path')}
            </label>
            <Input
              className='h-8 flex-1 text-xs'
              placeholder='/archives/my-agent'
              value={value.path ?? ''}
              onChange={(e) => update({ path: e.target.value })}
            />
          </div>
          <div className='flex items-center gap-3'>
            <label className='text-muted-foreground w-16 shrink-0 text-xs'>
              {t('Naming')}
            </label>
            <Select
              value={value.namingRule ?? NAMING_RULES[0].value}
              onValueChange={(v) => update({ namingRule: v })}
            >
              <SelectTrigger className='h-8 flex-1 text-xs'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {NAMING_RULES.map((r) => (
                  <SelectItem key={r.value} value={r.value} className='text-xs'>
                    {r.label}
                    <span className='text-muted-foreground ml-1 text-[10px]'>
                      ({r.value})
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className='flex items-center gap-3'>
            <label className='text-muted-foreground w-16 shrink-0 text-xs'>
              {t('Retention')}
            </label>
            <Select
              value={value.retentionPolicy ?? 'keep-30'}
              onValueChange={(v) => update({ retentionPolicy: v })}
            >
              <SelectTrigger className='h-8 w-[180px] text-xs'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {RETENTION_POLICIES.map((p) => (
                  <SelectItem key={p.value} value={p.value} className='text-xs'>
                    {t(p.label)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </>
      )}

      {/* Notion 配置 */}
      {value.type === ArchiveType.NOTION && (
        <>
          <div className='flex items-center gap-3'>
            <label className='text-muted-foreground w-16 shrink-0 text-xs'>
              数据库 ID
            </label>
            <Input
              className='h-8 flex-1 text-xs'
              placeholder='Notion Database ID'
              value={value.externalConfig?.database_id ?? ''}
              onChange={(e) => updateExternal('database_id', e.target.value)}
            />
          </div>
          <div className='flex items-center gap-3'>
            <label className='text-muted-foreground w-16 shrink-0 text-xs'>
              API Token
            </label>
            <Input
              className='h-8 flex-1 text-xs'
              type='password'
              placeholder='secret_...'
              value={value.externalConfig?.api_token ?? ''}
              onChange={(e) => updateExternal('api_token', e.target.value)}
            />
          </div>
          <div className='pl-[76px]'>
            <button
              type='button'
              className='text-primary hover:text-primary/80 text-xs underline-offset-2 hover:underline'
            >
              授权连接 Notion →
            </button>
          </div>
        </>
      )}

      {/* 自定义 Webhook */}
      {value.type === ArchiveType.WEBHOOK && (
        <div className='flex items-center gap-3'>
          <label className='text-muted-foreground w-16 shrink-0 text-xs'>
            Webhook URL
          </label>
          <Input
            className='h-8 flex-1 text-xs'
            placeholder='https://your-endpoint.com/archive'
            value={value.externalConfig?.url ?? ''}
            onChange={(e) => updateExternal('url', e.target.value)}
          />
        </div>
      )}

      {/* 飞书文档 */}
      {value.type === ArchiveType.FEISHU && (
        <div className='flex items-center gap-3'>
          <label className='text-muted-foreground w-16 shrink-0 text-xs'>
            文档 Token
          </label>
          <Input
            className='h-8 flex-1 text-xs'
            placeholder='飞书文档 Token'
            value={value.externalConfig?.doc_token ?? ''}
            onChange={(e) => updateExternal('doc_token', e.target.value)}
          />
        </div>
      )}

      {/* 归档触发条件 */}
      <div className='flex items-center gap-3'>
        <label className='text-muted-foreground w-16 shrink-0 text-xs'>
          {t('Trigger on')}
        </label>
        <div className='flex gap-1.5'>
          {[
            { value: 'always', label: '每次执行' },
            { value: 'success', label: '仅成功时' },
            { value: 'manual', label: '手动归档' },
          ].map((opt) => (
            <button
              key={opt.value}
              type='button'
              onClick={() => update({ triggerOn: opt.value as ArchiveConfig['triggerOn'] })}
              className={cn(
                'rounded-md border px-3 py-1.5 text-xs font-medium transition-colors',
                value.triggerOn === opt.value
                  ? 'bg-primary text-primary-foreground border-primary'
                  : 'border-border hover:bg-muted/60'
              )}
            >
              {t(opt.label)}
            </button>
          ))}
        </div>
      </div>
    </div>
  )
}
