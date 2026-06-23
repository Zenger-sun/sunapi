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
import { useMemo, useState, useEffect, useRef } from 'react'
import {
  MessageSquarePlus,
  SearchIcon,
  PinIcon,
  PinOffIcon,
  PencilIcon,
  Trash2Icon,
  MessageSquareIcon,
  ImageIcon,
  FilmIcon,
  SparklesIcon,
  MoreHorizontalIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { useLayout } from '@/context/layout-provider'
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
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSkeleton,
  SidebarRail,
} from '@/components/ui/sidebar'
import { Skeleton } from '@/components/ui/skeleton'
import type { PlaygroundSession } from '../types'

export type PlaygroundCapability = 'chat' | 'image' | 'video' | 'agent'

export type CapabilityDescriptor = {
  value: PlaygroundCapability
  label: string
  icon: React.ComponentType<{ className?: string }>
  badge?: string
}

const CAPABILITY_META: Record<
  PlaygroundCapability,
  {
    label: string
    icon: React.ComponentType<{ className?: string }>
    accent: string
  }
> = {
  chat: {
    label: 'Conversation',
    icon: MessageSquareIcon,
    accent:
      'data-[active=true]:bg-blue-500/10 data-[active=true]:text-blue-600 dark:data-[active=true]:text-blue-300',
  },
  image: {
    label: 'Images',
    icon: ImageIcon,
    accent:
      'data-[active=true]:bg-fuchsia-500/10 data-[active=true]:text-fuchsia-600 dark:data-[active=true]:text-fuchsia-300',
  },
  video: {
    label: 'Videos',
    icon: FilmIcon,
    accent:
      'data-[active=true]:bg-amber-500/10 data-[active=true]:text-amber-600 dark:data-[active=true]:text-amber-300',
  },
  agent: {
    label: 'Agents',
    icon: SparklesIcon,
    accent:
      'data-[active=true]:bg-emerald-500/10 data-[active=true]:text-emerald-600 dark:data-[active=true]:text-emerald-300',
  },
}

type GroupKey = 'pinned' | 'today' | 'yesterday' | 'thisWeek' | 'older'

type GroupedSessions = Record<GroupKey, PlaygroundSession[]>

function startOfDay(timestamp: number) {
  const d = new Date(timestamp)
  d.setHours(0, 0, 0, 0)
  return d.getTime()
}

function groupSessions(sessions: PlaygroundSession[]): GroupedSessions {
  const now = Date.now()
  const todayStart = startOfDay(now)
  const yesterdayStart = todayStart - 24 * 60 * 60 * 1000
  const weekStart = todayStart - 7 * 24 * 60 * 60 * 1000

  const result: GroupedSessions = {
    pinned: [],
    today: [],
    yesterday: [],
    thisWeek: [],
    older: [],
  }
  for (const session of sessions) {
    const ts = session.updated_time * 1000
    if (session.pinned) {
      result.pinned.push(session)
    } else if (ts >= todayStart) {
      result.today.push(session)
    } else if (ts >= yesterdayStart) {
      result.yesterday.push(session)
    } else if (ts >= weekStart) {
      result.thisWeek.push(session)
    } else {
      result.older.push(session)
    }
  }
  return result
}

type PlaygroundSidebarProps = {
  activeCapability: PlaygroundCapability
  onCapabilityChange: (capability: PlaygroundCapability) => void
  activeSessionId: number | null
  hasDraftSession: boolean
  isGenerating: boolean
  isSavingSession: boolean
  isLoadingSessions: boolean
  sessions: PlaygroundSession[]
  onNewChat: () => void
  onSelectSession: (sessionId: number) => void
  onRenameSession: (sessionId: number, title: string) => void
  onDeleteSession: (sessionId: number) => void
  onTogglePinSession: (sessionId: number) => void
  workspaceContent?: React.ReactNode
}

export function PlaygroundSidebar({
  activeCapability,
  onCapabilityChange,
  activeSessionId,
  hasDraftSession,
  isGenerating,
  isSavingSession,
  isLoadingSessions,
  sessions,
  onNewChat,
  onSelectSession,
  onRenameSession,
  onDeleteSession,
  onTogglePinSession,
  workspaceContent,
}: PlaygroundSidebarProps) {
  const { t } = useTranslation()
  const { collapsible: collapsiblePref, variant } = useLayout()
  // The Playground sidebar packs capability tabs, grouped session history
  // and a model marketplace footer into one column, so it never collapses
  // to icon-only because that mode would force text labels to overlap. Use
  // offcanvas so collapsing slides the whole panel off-screen instead.
  const collapsible = collapsiblePref === 'icon' ? 'offcanvas' : collapsiblePref
  const [search, setSearch] = useState('')

  const filteredSessions = useMemo(() => {
    const trimmed = search.trim().toLowerCase()
    if (!trimmed) return sessions
    return sessions.filter((session) =>
      `${session.title ?? ''} ${session.summary ?? ''}`
        .toLowerCase()
        .includes(trimmed)
    )
  }, [sessions, search])

  const grouped = useMemo(
    () => groupSessions(filteredSessions),
    [filteredSessions]
  )

  const isMutating = isGenerating || isSavingSession
  const isSessionSelectionDisabled = isGenerating
  const draftLabel = t('Current conversation')

  return (
    <Sidebar collapsible={collapsible} variant={variant}>
      <SidebarContent className='gap-0 py-3'>
        <CapabilityTabs
          activeCapability={activeCapability}
          onChange={onCapabilityChange}
        />

        {workspaceContent && (
          <SidebarGroup className='px-2 pt-1 pb-1'>
            <SidebarGroupContent>
              <div className='flex flex-col gap-3'>{workspaceContent}</div>
            </SidebarGroupContent>
          </SidebarGroup>
        )}

        {activeCapability === 'chat' && (
          <>
            <SidebarGroup className='px-3 pt-1 pb-2'>
              <Button
                type='button'
                variant='default'
                className='h-9 w-full justify-center gap-2 text-sm font-medium'
                onClick={onNewChat}
                disabled={isMutating}
              >
                <MessageSquarePlus className='size-4' />
                {t('New conversation')}
              </Button>

              <div className='relative mt-2'>
                <SearchIcon className='text-muted-foreground absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2' />
                <Input
                  value={search}
                  onChange={(event) => setSearch(event.target.value)}
                  placeholder={t('Search conversations')}
                  className='h-8 pl-8 text-xs'
                />
              </div>
            </SidebarGroup>

            {hasDraftSession && (
              <SidebarGroup className='px-2 pt-1 pb-1'>
                <SidebarGroupContent>
                  <SidebarMenu>
                    <SidebarMenuItem>
                      <SidebarMenuButton
                        type='button'
                        isActive={activeSessionId === null}
                        onClick={onNewChat}
                        disabled={isMutating}
                        className='h-auto min-h-11 items-start py-2'
                      >
                        <span className='grid min-w-0 flex-1 gap-0.5'>
                          <span className='line-clamp-2 text-sm leading-snug font-medium'>
                            {draftLabel}
                          </span>
                          <span className='text-muted-foreground text-xs font-normal'>
                            {t('Unsaved draft')}
                          </span>
                        </span>
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  </SidebarMenu>
                </SidebarGroupContent>
              </SidebarGroup>
            )}

            {isLoadingSessions ? (
              <SidebarGroup className='px-2'>
                <SidebarGroupContent>
                  <SidebarMenu>
                    {[0, 1, 2, 3].map((index) => (
                      <SidebarMenuItem key={index}>
                        <SidebarMenuSkeleton />
                      </SidebarMenuItem>
                    ))}
                  </SidebarMenu>
                </SidebarGroupContent>
              </SidebarGroup>
            ) : sessions.length === 0 ? (
              <SidebarGroup className='px-2 pt-1 pb-1'>
                <SidebarGroupContent>
                  <div className='text-muted-foreground px-2 py-6 text-center text-xs'>
                    {t('No conversations yet')}
                  </div>
                </SidebarGroupContent>
              </SidebarGroup>
            ) : (
              <>
                <SessionSection
                  title={t('Pinned')}
                  sessions={grouped.pinned}
                  empty={null}
                  activeSessionId={activeSessionId}
                  activeCapability={activeCapability}
                  isSelectionDisabled={isSessionSelectionDisabled}
                  onSelect={onSelectSession}
                  onRename={onRenameSession}
                  onDelete={onDeleteSession}
                  onTogglePin={onTogglePinSession}
                />
                <SessionSection
                  title={t('Today')}
                  sessions={grouped.today}
                  empty={null}
                  activeSessionId={activeSessionId}
                  activeCapability={activeCapability}
                  isSelectionDisabled={isSessionSelectionDisabled}
                  onSelect={onSelectSession}
                  onRename={onRenameSession}
                  onDelete={onDeleteSession}
                  onTogglePin={onTogglePinSession}
                />
                <SessionSection
                  title={t('Yesterday')}
                  sessions={grouped.yesterday}
                  empty={null}
                  activeSessionId={activeSessionId}
                  activeCapability={activeCapability}
                  isSelectionDisabled={isSessionSelectionDisabled}
                  onSelect={onSelectSession}
                  onRename={onRenameSession}
                  onDelete={onDeleteSession}
                  onTogglePin={onTogglePinSession}
                />
                <SessionSection
                  title={t('This week')}
                  sessions={grouped.thisWeek}
                  empty={null}
                  activeSessionId={activeSessionId}
                  activeCapability={activeCapability}
                  isSelectionDisabled={isSessionSelectionDisabled}
                  onSelect={onSelectSession}
                  onRename={onRenameSession}
                  onDelete={onDeleteSession}
                  onTogglePin={onTogglePinSession}
                />
                <SessionSection
                  title={t('Older')}
                  sessions={grouped.older}
                  empty={null}
                  activeSessionId={activeSessionId}
                  activeCapability={activeCapability}
                  isSelectionDisabled={isSessionSelectionDisabled}
                  onSelect={onSelectSession}
                  onRename={onRenameSession}
                  onDelete={onDeleteSession}
                  onTogglePin={onTogglePinSession}
                />
              </>
            )}
          </>
        )}
      </SidebarContent>

      <SidebarRail />
    </Sidebar>
  )
}

type CapabilityTabsProps = {
  activeCapability: PlaygroundCapability
  onChange: (capability: PlaygroundCapability) => void
}

function CapabilityTabs({ activeCapability, onChange }: CapabilityTabsProps) {
  const { t } = useTranslation()
  const order: PlaygroundCapability[] = ['chat', 'image', 'video', 'agent']

  return (
    <SidebarGroup className='px-2 pt-1 pb-1'>
      <SidebarGroupLabel className='text-muted-foreground/70 px-2 text-[10px] font-medium tracking-wider uppercase'>
        {t('Workspace')}
      </SidebarGroupLabel>
      <SidebarGroupContent>
        <div className='bg-muted/40 flex flex-col gap-1 rounded-lg p-1'>
          {order.map((capability) => {
            const meta = CAPABILITY_META[capability]
            const Icon = meta.icon
            const isActive = activeCapability === capability
            return (
              <button
                key={capability}
                type='button'
                onClick={() => onChange(capability)}
                data-active={isActive}
                className={cn(
                  'text-muted-foreground hover:text-foreground flex h-9 items-center justify-start gap-2 rounded-md px-2 text-xs font-medium transition-colors',
                  'data-[active=true]:bg-background data-[active=true]:shadow-sm',
                  meta.accent
                )}
                title={t(meta.label)}
              >
                <Icon className='size-4 shrink-0' />
                <span className='truncate'>{t(meta.label)}</span>
              </button>
            )
          })}
        </div>
      </SidebarGroupContent>
    </SidebarGroup>
  )
}

type SessionSectionProps = {
  title: string
  sessions: PlaygroundSession[]
  empty: React.ReactNode
  activeSessionId: number | null
  activeCapability: PlaygroundCapability
  isSelectionDisabled: boolean
  onSelect: (sessionId: number) => void
  onRename: (sessionId: number, title: string) => void
  onDelete: (sessionId: number) => void
  onTogglePin: (sessionId: number) => void
}

function SessionSection({
  title,
  sessions,
  empty,
  activeSessionId,
  activeCapability,
  isSelectionDisabled,
  onSelect,
  onRename,
  onDelete,
  onTogglePin,
}: SessionSectionProps) {
  if (sessions.length === 0) {
    return empty
  }

  return (
    <SidebarGroup className='px-2 pt-1 pb-1'>
      <SidebarGroupLabel className='text-muted-foreground/70 px-2 text-[10px] font-medium tracking-wider uppercase'>
        {title}
      </SidebarGroupLabel>
      <SidebarGroupContent>
        <SidebarMenu>
          {sessions.map((session) => (
            <SessionItem
              key={session.id}
              session={session}
              isActive={
                activeCapability === 'chat' && activeSessionId === session.id
              }
              isSelectionDisabled={isSelectionDisabled}
              onSelect={onSelect}
              onRename={onRename}
              onDelete={onDelete}
              onTogglePin={onTogglePin}
            />
          ))}
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  )
}

type SessionItemProps = {
  session: PlaygroundSession
  isActive: boolean
  isSelectionDisabled: boolean
  onSelect: (sessionId: number) => void
  onRename: (sessionId: number, title: string) => void
  onDelete: (sessionId: number) => void
  onTogglePin: (sessionId: number) => void
}

function SessionItem({
  session,
  isActive,
  isSelectionDisabled,
  onSelect,
  onRename,
  onDelete,
  onTogglePin,
}: SessionItemProps) {
  const { t } = useTranslation()
  const [isRenaming, setIsRenaming] = useState(false)
  const [draftTitle, setDraftTitle] = useState(session.title)
  const inputRef = useRef<HTMLInputElement | null>(null)
  const sessionSummary =
    session.summary && session.summary !== session.title ? session.summary : ''
  const isPending = session.id < 0
  const metaItems = [
    session.model || '-',
    ...(typeof session.message_count === 'number'
      ? [t('{{count}} msgs', { count: session.message_count })]
      : []),
    ...(isPending ? [t('Saving')] : []),
  ]

  useEffect(() => {
    if (!isRenaming) {
      setDraftTitle(session.title)
    }
  }, [session.title, isRenaming])

  useEffect(() => {
    if (isRenaming) {
      inputRef.current?.focus()
      inputRef.current?.select()
    }
  }, [isRenaming])

  const handleSubmitRename = () => {
    const next = draftTitle.trim()
    setIsRenaming(false)
    if (!next || next === session.title) {
      setDraftTitle(session.title)
      return
    }
    onRename(session.id, next)
  }

  return (
    <SidebarMenuItem>
      <div
        className={cn(
          'group/session relative flex w-full min-w-0 items-start gap-1 rounded-md transition-colors',
          isActive && 'bg-accent text-accent-foreground'
        )}
      >
        {isRenaming ? (
          <Input
            ref={inputRef}
            value={draftTitle}
            onChange={(event) => setDraftTitle(event.target.value)}
            onBlur={handleSubmitRename}
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                event.preventDefault()
                handleSubmitRename()
              } else if (event.key === 'Escape') {
                event.preventDefault()
                setDraftTitle(session.title)
                setIsRenaming(false)
              }
            }}
            className='h-8 px-2 text-xs'
          />
        ) : (
          <>
            <button
              type='button'
              disabled={isSelectionDisabled || isPending}
              onClick={() => onSelect(session.id)}
              className='flex min-h-10 w-full min-w-0 flex-1 items-start gap-1 rounded-md px-2 py-1.5 text-left disabled:opacity-50'
            >
              <span className='grid min-w-0 flex-1 gap-0.5'>
                <span className='line-clamp-2 text-sm leading-snug font-medium break-all'>
                  {session.title || t('Untitled')}
                </span>
                {sessionSummary && (
                  <span className='text-muted-foreground line-clamp-2 text-xs leading-snug font-normal break-all'>
                    {sessionSummary}
                  </span>
                )}
                <span className='text-muted-foreground flex min-w-0 items-center text-[11px] font-normal'>
                  {metaItems.map((item, index) => (
                    <span
                      key={`${item}-${index}`}
                      className={index === 0 ? 'truncate' : 'shrink-0'}
                    >
                      {index > 0 && (
                        <span className='px-1' aria-hidden>
                          |
                        </span>
                      )}
                      {item}
                    </span>
                  ))}
                </span>
                <span className='hidden'>
                  <span className='truncate'>{session.model || '-'}</span>
                  {typeof session.message_count === 'number' && (
                    <>
                      <span aria-hidden>·</span>
                      <span>
                        {t('{{count}} msgs', {
                          count: session.message_count,
                        })}
                      </span>
                    </>
                  )}
                  {isPending && (
                    <>
                      <span aria-hidden>|</span>
                      <span>{t('Saving')}</span>
                    </>
                  )}
                </span>
              </span>
            </button>

            {!isPending && (
              <DropdownMenu>
                <DropdownMenuTrigger
                  className={cn(
                    'text-muted-foreground hover:text-foreground absolute top-1.5 right-1 inline-flex size-6 items-center justify-center rounded-md opacity-0 transition-opacity',
                    'group-hover/session:opacity-100 data-[popup-open]:opacity-100',
                    isActive && 'opacity-100'
                  )}
                  aria-label={t('Session actions')}
                >
                  <MoreHorizontalIcon className='size-3.5' />
                </DropdownMenuTrigger>
                <DropdownMenuContent side='right' align='start' sideOffset={4}>
                  <DropdownMenuItem
                    onClick={() => onTogglePin(session.id)}
                    className='gap-2'
                  >
                    {session.pinned ? (
                      <>
                        <PinOffIcon className='size-3.5' />
                        {t('Unpin')}
                      </>
                    ) : (
                      <>
                        <PinIcon className='size-3.5' />
                        {t('Pin')}
                      </>
                    )}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={() => setIsRenaming(true)}
                    className='gap-2'
                  >
                    <PencilIcon className='size-3.5' />
                    {t('Rename')}
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    variant='destructive'
                    onClick={() => onDelete(session.id)}
                    className='gap-2'
                  >
                    <Trash2Icon className='size-3.5' />
                    {t('Delete')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            )}
          </>
        )}
      </div>
    </SidebarMenuItem>
  )
}

export { Skeleton as PlaygroundSidebarSkeleton }
