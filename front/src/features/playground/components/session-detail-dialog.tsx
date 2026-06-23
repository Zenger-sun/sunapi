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
import { ClockIcon, MessageSquareIcon, RotateCcwIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Skeleton } from '@/components/ui/skeleton'
import type { PlaygroundSession } from '../types'
import { PlaygroundChat } from './playground-chat'

type SessionDetailDialogProps = {
  session: PlaygroundSession | null
  isLoading: boolean
  onOpenChange: (open: boolean) => void
  onRestore: (session: PlaygroundSession) => void
}

function formatTimestamp(timestamp?: number) {
  if (!timestamp) return ''
  return new Date(timestamp * 1000).toLocaleString()
}

export function SessionDetailDialog({
  session,
  isLoading,
  onOpenChange,
  onRestore,
}: SessionDetailDialogProps) {
  const { t } = useTranslation()
  const messages = session?.messages ?? []
  const canRestore = !!session && messages.length > 0

  return (
    <Dialog open={!!session} onOpenChange={onOpenChange}>
      <DialogContent className='h-[min(44rem,calc(100vh-2rem))] max-w-[min(56rem,calc(100vw-2rem))] grid-rows-[auto_1fr_auto] gap-3 p-0'>
        <DialogHeader className='border-b px-5 py-4 pr-12'>
          <DialogTitle className='line-clamp-2 pr-2'>
            {session?.title || t('Untitled')}
          </DialogTitle>
          <DialogDescription className='flex flex-wrap items-center gap-3 text-xs'>
            <span className='inline-flex items-center gap-1'>
              <MessageSquareIcon className='size-3.5' />
              {t('{{count}} msgs', {
                count: session?.message_count ?? messages.length,
              })}
            </span>
            {session?.model && <span>{session.model}</span>}
            {session?.updated_time && (
              <span className='inline-flex items-center gap-1'>
                <ClockIcon className='size-3.5' />
                {formatTimestamp(session.updated_time)}
              </span>
            )}
          </DialogDescription>
        </DialogHeader>

        <div className='min-h-0 overflow-hidden'>
          {isLoading ? (
            <div className='space-y-3 p-5'>
              <Skeleton className='h-16 w-3/4' />
              <Skeleton className='ml-auto h-20 w-2/3' />
              <Skeleton className='h-24 w-4/5' />
            </div>
          ) : messages.length > 0 ? (
            <PlaygroundChat messages={messages} />
          ) : (
            <div className='text-muted-foreground flex size-full items-center justify-center px-6 text-center text-sm'>
              {t('No conversation details')}
            </div>
          )}
        </div>

        <DialogFooter className='px-5 py-4'>
          <Button
            type='button'
            className='gap-2'
            disabled={!canRestore}
            onClick={() => session && onRestore(session)}
          >
            <RotateCcwIcon className='size-4' />
            {t('Restore conversation')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
