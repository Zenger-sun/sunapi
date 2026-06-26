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
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'

interface ChatSuggestionsProps {
  onSelect: (text: string) => void
}

interface SuggestionItem {
  icon: ComponentType<{ className?: string }>
  labelKey: string
  promptKey: string
  color: string
}

const SUGGESTIONS: SuggestionItem[] = [
  {
    icon: BarChartIcon,
    labelKey: 'Analyze data',
    promptKey: 'Analyze recent usage logs and find unusual trends',
    color: '#76d0eb',
  },
  {
    icon: BoxIcon,
    labelKey: 'Surprise me',
    promptKey: 'Give me an interesting prompt',
    color: '#76d0eb',
  },
  {
    icon: NotepadTextIcon,
    labelKey: 'Summarize text',
    promptKey: 'Summarize this month channel usage',
    color: '#ea8444',
  },
  {
    icon: CodeSquareIcon,
    labelKey: 'Code',
    promptKey: 'Help me write a React Hook',
    color: '#6c71ff',
  },
  {
    icon: GraduationCapIcon,
    labelKey: 'Get advice',
    promptKey: 'Which model should I use to translate documents?',
    color: '#76d0eb',
  },
  {
    icon: SparklesIcon,
    labelKey: 'Brainstorm',
    promptKey: 'Help me come up with 5 product names',
    color: '#ea8444',
  },
]

export function ChatSuggestions({ onSelect }: ChatSuggestionsProps) {
  const { t } = useTranslation()

  return (
    <div className='grid w-full grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3'>
      {SUGGESTIONS.map((item) => {
        const Icon = item.icon
        return (
          <button
            key={item.labelKey}
            type='button'
            onClick={() => onSelect(t(item.promptKey))}
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
                {t(item.labelKey)}
              </span>
              <span className='text-muted-foreground line-clamp-2 text-[11px] leading-snug'>
                {t(item.promptKey)}
              </span>
            </span>
          </button>
        )
      })}
    </div>
  )
}
