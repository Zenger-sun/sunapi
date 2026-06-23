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
import { CpuIcon, ClockIcon, HashIcon, CoinsIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import type { Message } from '../types'

interface MessageMetaProps {
  message: Message
  className?: string
}

export function hasMessageMeta(message: Message): boolean {
  return Boolean(
    message.model ||
    typeof message.durationMs === 'number' ||
    message.tokens?.total ||
    message.tokens?.prompt ||
    message.tokens?.completion
  )
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms} ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`
  const minutes = Math.floor(ms / 60_000)
  const seconds = Math.floor((ms % 60_000) / 1000)
  return `${minutes}m ${seconds}s`
}

function formatTokens(value: number): string {
  if (value < 1000) return `${value}`
  if (value < 10_000) return `${(value / 1000).toFixed(2)}K`
  if (value < 1_000_000) return `${Math.round(value / 1000)}K`
  return `${(value / 1_000_000).toFixed(2)}M`
}

export function MessageMeta({ message, className }: MessageMetaProps) {
  const { t } = useTranslation()
  const tokens = message.tokens
  const items: Array<{ key: string; node: React.ReactNode }> = []

  if (message.model) {
    items.push({
      key: 'model',
      node: (
        <span className='text-muted-foreground inline-flex items-center gap-1'>
          <CpuIcon className='size-3' />
          <span className='truncate font-mono text-[10px]'>
            {message.model}
          </span>
        </span>
      ),
    })
  }

  if (typeof message.durationMs === 'number') {
    items.push({
      key: 'duration',
      node: (
        <span className='text-muted-foreground inline-flex items-center gap-1'>
          <ClockIcon className='size-3' />
          <span>{formatDuration(message.durationMs)}</span>
        </span>
      ),
    })
  }

  if (tokens?.total) {
    items.push({
      key: 'tokens-total',
      node: (
        <span className='text-muted-foreground inline-flex items-center gap-1'>
          <HashIcon className='size-3' />
          <span>
            {t('{{count}} tokens', { count: formatTokens(tokens.total) })}
          </span>
        </span>
      ),
    })
  } else if (tokens?.prompt || tokens?.completion) {
    items.push({
      key: 'tokens-split',
      node: (
        <span className='text-muted-foreground inline-flex items-center gap-1'>
          <CoinsIcon className='size-3' />
          <span>
            {t('{{in}} -> {{out}}', {
              in: formatTokens(tokens.prompt ?? 0),
              out: formatTokens(tokens.completion ?? 0),
            })}
          </span>
        </span>
      ),
    })
  }

  if (items.length === 0) return null

  return (
    <div
      className={cn(
        'text-muted-foreground mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-[10px]',
        className
      )}
    >
      {items.map((item, index) => (
        <span key={item.key} className='inline-flex items-center gap-1'>
          {item.node}
          {index < items.length - 1 && (
            <span aria-hidden className='text-muted-foreground/50'>
              -
            </span>
          )}
        </span>
      ))}
    </div>
  )
}
