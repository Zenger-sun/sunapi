/*
Copyright (C) 2026 SunAPI Project Owner

This file is part of the SunAPI Creator Studio Components.

Licensed under the SunAPI Creator Studio Source-Available Commercial License
v1.0. See LICENSE-CREATOR-STUDIO at the repository root.

This source is available for evaluation, but modification, redistribution,
hosting, sale, resale, sublicensing, or commercial use requires prior written
permission from the SunAPI project owner.
*/
import { useMemo, useState, useEffect, useRef } from 'react'
import {
  BotIcon,
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
import {
  AGENT_BLUEPRINTS_CHANGED_EVENT,
  deleteStoredAgentBlueprint,
  dispatchAgentBlueprintCommand,
  getAgentBlueprintStatusLabel,
  readAgentBlueprintSummaries,
  type AgentBlueprintSummary,
} from '../lib/agent-blueprints'
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
  const [agentBlueprints, setAgentBlueprints] = useState<
    AgentBlueprintSummary[]
  >(readAgentBlueprintSummaries)

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

  const filteredAgentBlueprints = useMemo(() => {
    const trimmed = search.trim().toLowerCase()
    if (!trimmed) return agentBlueprints
    return agentBlueprints.filter((blueprint) =>
      blueprint.title.toLowerCase().includes(trimmed)
    )
  }, [agentBlueprints, search])

  useEffect(() => {
    const refreshBlueprints = () => {
      setAgentBlueprints(readAgentBlueprintSummaries())
    }

    window.addEventListener(AGENT_BLUEPRINTS_CHANGED_EVENT, refreshBlueprints)
    window.addEventListener('storage', refreshBlueprints)
    return () => {
      window.removeEventListener(
        AGENT_BLUEPRINTS_CHANGED_EVENT,
        refreshBlueprints
      )
      window.removeEventListener('storage', refreshBlueprints)
    }
  }, [])

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

        {activeCapability === 'agent' && (
          <AgentBlueprintHistoryPanel
            blueprints={filteredAgentBlueprints}
            search={search}
            onSearchChange={setSearch}
          />
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

type AgentBlueprintHistoryPanelProps = {
  blueprints: AgentBlueprintSummary[]
  search: string
  onSearchChange: (value: string) => void
}

function AgentBlueprintHistoryPanel({
  blueprints,
  search,
  onSearchChange,
}: AgentBlueprintHistoryPanelProps) {
  const { t } = useTranslation()

  const handleNewBlueprint = () => {
    dispatchAgentBlueprintCommand({ type: 'new' })
  }

  const handleLoadBlueprint = (blueprintId: string) => {
    dispatchAgentBlueprintCommand({ type: 'load', id: blueprintId })
  }

  const handleDeleteBlueprint = (blueprintId: string) => {
    deleteStoredAgentBlueprint(blueprintId)
  }

  return (
    <>
      <SidebarGroup className='px-3 pt-1 pb-2'>
        <Button
          type='button'
          variant='default'
          className='h-9 w-full justify-center gap-2 text-sm font-medium'
          onClick={handleNewBlueprint}
        >
          <BotIcon className='size-4' />
          {t('新建蓝图')}
        </Button>

        <div className='relative mt-2'>
          <SearchIcon className='text-muted-foreground absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2' />
          <Input
            value={search}
            onChange={(event) => onSearchChange(event.target.value)}
            placeholder={t('搜索蓝图')}
            className='h-8 pl-8 text-xs'
          />
        </div>
      </SidebarGroup>

      {blueprints.length === 0 ? (
        <SidebarGroup className='px-2 pt-1 pb-1'>
          <SidebarGroupContent>
            <div className='text-muted-foreground px-2 py-6 text-center text-xs'>
              {t('暂无智能体蓝图')}
            </div>
          </SidebarGroupContent>
        </SidebarGroup>
      ) : (
        <SidebarGroup className='px-2 pt-1 pb-1'>
          <SidebarGroupLabel className='text-muted-foreground/70 px-2 text-[10px] font-medium tracking-wider uppercase'>
            {t('蓝图历史')}
          </SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {blueprints.map((blueprint) => (
                <AgentBlueprintItem
                  key={blueprint.id}
                  blueprint={blueprint}
                  onLoad={handleLoadBlueprint}
                  onDelete={handleDeleteBlueprint}
                />
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      )}
    </>
  )
}

type AgentBlueprintItemProps = {
  blueprint: AgentBlueprintSummary
  onLoad: (blueprintId: string) => void
  onDelete: (blueprintId: string) => void
}

function AgentBlueprintItem({
  blueprint,
  onLoad,
  onDelete,
}: AgentBlueprintItemProps) {
  const { t } = useTranslation()
  const statusLabel = getAgentBlueprintStatusLabel(blueprint.status)
  const statusClass = getBlueprintSidebarStatusClass(blueprint.status)

  return (
    <SidebarMenuItem>
      <div className='group/blueprint hover:bg-muted/45 hover:border-border/70 relative flex w-full min-w-0 items-start gap-1 rounded-md border border-transparent transition-colors'>
        <button
          type='button'
          onClick={() => onLoad(blueprint.id)}
          className='flex min-h-16 w-full min-w-0 flex-1 items-start gap-1 rounded-md px-2 py-2 pr-8 text-left'
        >
          <span className='grid min-w-0 flex-1 gap-1.5'>
            <span className='line-clamp-2 text-sm leading-snug font-medium break-all'>
              {blueprint.title || t('未命名蓝图')}
            </span>
            <span className='flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-1 text-[11px] font-normal'>
              <span
                className={cn(
                  'inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 text-[10px] leading-none',
                  statusClass
                )}
              >
                <span className='size-1.5 rounded-full bg-current' />
                {statusLabel}
              </span>
              <span className='text-muted-foreground shrink-0'>
                {t('{{count}} 个节点', { count: blueprint.nodesCount })}
              </span>
              <span className='text-muted-foreground/80 shrink-0'>
                {formatBlueprintSidebarTime(blueprint.updatedAt)}
              </span>
            </span>
          </span>
        </button>

        <DropdownMenu>
          <DropdownMenuTrigger
            className='text-muted-foreground hover:text-foreground absolute top-1.5 right-1 inline-flex size-6 items-center justify-center rounded-md opacity-0 transition-opacity group-hover/blueprint:opacity-100 data-[popup-open]:opacity-100'
            aria-label={t('蓝图操作')}
          >
            <MoreHorizontalIcon className='size-3.5' />
          </DropdownMenuTrigger>
          <DropdownMenuContent side='right' align='start' sideOffset={4}>
            <DropdownMenuItem
              onClick={() => onLoad(blueprint.id)}
              className='gap-2'
            >
              <BotIcon className='size-3.5' />
              {t('载入')}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              variant='destructive'
              onClick={() => onDelete(blueprint.id)}
              className='gap-2'
            >
              <Trash2Icon className='size-3.5' />
              {t('删除')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </SidebarMenuItem>
  )
}

function getBlueprintSidebarStatusClass(
  status: AgentBlueprintSummary['status']
) {
  if (status === 'running') {
    return 'bg-blue-500/10 text-blue-600 dark:text-blue-300'
  }
  if (status === 'ended') {
    return 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-300'
  }
  return 'bg-amber-500/10 text-amber-700 dark:text-amber-300'
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

function formatBlueprintSidebarTime(timestamp: number) {
  const date = new Date(timestamp)
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  const hour = `${date.getHours()}`.padStart(2, '0')
  const minute = `${date.getMinutes()}`.padStart(2, '0')
  return `${month}-${day} ${hour}:${minute}`
}

export { Skeleton as PlaygroundSidebarSkeleton }
