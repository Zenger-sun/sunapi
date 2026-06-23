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
import type { ComponentType } from 'react'
import {
  BarChartIcon,
  BoxIcon,
  CodeSquareIcon,
  GraduationCapIcon,
  NotepadTextIcon,
  SparklesIcon,
} from 'lucide-react'
import { cn } from '@/lib/utils'

interface ChatSuggestionsProps {
  onSelect: (text: string) => void
}

interface SuggestionItem {
  icon: ComponentType<{ className?: string }>
  label: string
  prompt: string
  color: string
}

const SUGGESTIONS: SuggestionItem[] = [
  {
    icon: BarChartIcon,
    label: '分析数据',
    prompt: '分析最近的调用日志，找出异常趋势',
    color: '#76d0eb',
  },
  {
    icon: BoxIcon,
    label: '给我惊喜',
    prompt: '给我一个有意思的提示词',
    color: '#76d0eb',
  },
  {
    icon: NotepadTextIcon,
    label: '总结文本',
    prompt: '总结一下本月的渠道用量',
    color: '#ea8444',
  },
  {
    icon: CodeSquareIcon,
    label: '代码',
    prompt: '帮我写一个 React Hook',
    color: '#6c71ff',
  },
  {
    icon: GraduationCapIcon,
    label: '获取建议',
    prompt: '我应该用哪个模型来翻译文档？',
    color: '#76d0eb',
  },
  {
    icon: SparklesIcon,
    label: '头脑风暴',
    prompt: '帮我想 5 个产品命名',
    color: '#ea8444',
  },
]

export function ChatSuggestions({ onSelect }: ChatSuggestionsProps) {
  return (
    <div className='grid w-full grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3'>
      {SUGGESTIONS.map((item) => {
        const Icon = item.icon
        return (
          <button
            key={item.label}
            type='button'
            onClick={() => onSelect(item.prompt)}
            className={cn(
              'border-border/60 bg-card/40 hover:border-foreground/30 hover:bg-accent/40',
              'group/suggestion flex items-start gap-2.5 rounded-lg border px-3 py-2.5 text-left transition-colors'
            )}
          >
            <span
              className='mt-0.5 inline-flex size-7 shrink-0 items-center justify-center rounded-md'
              style={{ backgroundColor: `${item.color}1f`, color: item.color }}
            >
              <Icon className='size-3.5' />
            </span>
            <span className='flex min-w-0 flex-1 flex-col gap-0.5'>
              <span className='text-foreground text-xs font-medium'>
                {item.label}
              </span>
              <span className='text-muted-foreground line-clamp-2 text-[11px] leading-snug'>
                {item.prompt}
              </span>
            </span>
          </button>
        )
      })}
    </div>
  )
}
