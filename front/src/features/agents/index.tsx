/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { formatDistanceToNow } from 'date-fns'
import { zhCN } from 'date-fns/locale'
import {
  Bot,
  ChevronRight,
  MoreHorizontal,
  Plus,
  Trash2,
  Zap,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { SectionPageLayout } from '@/components/layout'
import { CopyButton } from '@/components/copy-button'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import {
  agentsQueryOptions,
  useCreateAgentMutation,
  useDeleteAgentMutation,
} from './api'
import { BlueprintDrawer } from './components/blueprint-drawer'
import { BlueprintList } from './components/blueprint-list'
import { SkillsPage } from './components/skills-page'
import { AgentStatus, type Agent } from './types'

// ============================================================================
// 新建智能体弹窗
// ============================================================================

function CreateAgentDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (v: boolean) => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const createMutation = useCreateAgentMutation()

  const handleCreate = () => {
    if (!name.trim()) {
      toast.error(t('Please enter an agent name'))
      return
    }
    createMutation.mutate(
      { name: name.trim(), description: description.trim() },
      {
        onSuccess: () => {
          toast.success(t('Agent created'))
          onOpenChange(false)
          setName('')
          setDescription('')
        },
        onError: () => toast.error(t('Failed to create agent')),
      }
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-w-sm'>
        <DialogHeader>
          <DialogTitle>{t('Create Agent')}</DialogTitle>
        </DialogHeader>
        <div className='flex flex-col gap-3 py-1'>
          <div className='flex flex-col gap-1.5'>
            <Label className='text-xs'>{t('Agent Name')} *</Label>
            <Input
              className='h-8 text-sm'
              placeholder={t('e.g. Daily News Summarizer')}
              value={name}
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
            />
          </div>
          <div className='flex flex-col gap-1.5'>
            <Label className='text-xs text-muted-foreground'>
              {t('Description')}
            </Label>
            <Textarea
              className='min-h-[60px] resize-none text-xs'
              placeholder={t('What does this agent do?')}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button
            variant='outline'
            size='sm'
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button
            size='sm'
            onClick={handleCreate}
            disabled={createMutation.isPending}
          >
            {createMutation.isPending ? t('Creating...') : t('Create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ============================================================================
// 智能体卡片
// ============================================================================

function AgentCard({
  agent,
  selected,
  onSelect,
}: {
  agent: Agent
  selected: boolean
  onSelect: () => void
}) {
  const { t } = useTranslation()
  const deleteMutation = useDeleteAgentMutation()

  const isActive = agent.status === AgentStatus.ACTIVE

  return (
    <button
      type='button'
      onClick={onSelect}
      className={`group flex w-full items-start gap-3 rounded-lg border p-3 text-left transition-all ${
        selected
          ? 'border-primary/50 bg-primary/5'
          : 'border-border/60 hover:bg-muted/20'
      }`}
    >
      {/* 头像 */}
      <div className='bg-muted flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-lg'>
        {agent.avatar ?? '🤖'}
      </div>

      {/* 信息 */}
      <div className='min-w-0 flex-1'>
        <div className='flex items-center gap-2'>
          <span className='truncate text-sm font-medium'>{agent.name}</span>
          <span
            className={`inline-block h-1.5 w-1.5 rounded-full ${
              isActive ? 'bg-emerald-500' : 'bg-slate-300'
            }`}
            aria-label={isActive ? t('Active') : t('Inactive')}
          />
        </div>
        {agent.description && (
          <p className='text-muted-foreground mt-0.5 line-clamp-1 text-xs'>
            {agent.description}
          </p>
        )}
        <div className='text-muted-foreground mt-1.5 flex items-center gap-2 text-[11px]'>
          <span className='flex items-center gap-1'>
            <Zap className='h-3 w-3' aria-hidden='true' />
            {agent.blueprint_count} 个蓝图
          </span>
          {agent.last_executed_time && (
            <span>
              {formatDistanceToNow(agent.last_executed_time, {
                addSuffix: true,
                locale: zhCN,
              })}
            </span>
          )}
        </div>
      </div>

      {/* 操作 & 箭头 */}
      <div className='flex shrink-0 items-center gap-1'>
        <DropdownMenu>
          <DropdownMenuTrigger
            asChild
            onClick={(e) => e.stopPropagation()}
          >
            <Button
              size='sm'
              variant='ghost'
              className='h-7 w-7 p-0 opacity-0 group-hover:opacity-100'
              aria-label={t('More')}
            >
              <MoreHorizontal className='h-4 w-4' aria-hidden='true' />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align='end' className='w-36'>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              className='text-destructive focus:text-destructive'
              onClick={(e) => {
                e.stopPropagation()
                deleteMutation.mutate(agent.id, {
                  onSuccess: () => toast.success(t('Agent deleted')),
                })
              }}
            >
              <Trash2 className='mr-2 h-4 w-4' aria-hidden='true' />
              {t('Delete')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <ChevronRight
          className={`h-4 w-4 transition-colors ${
            selected ? 'text-primary' : 'text-muted-foreground'
          }`}
          aria-hidden='true'
        />
      </div>
    </button>
  )
}

// ============================================================================
// 智能体详情面板
// ============================================================================

function AgentDetailPanel({ agent }: { agent: Agent }) {
  const { t } = useTranslation()
  const [blueprintDrawerOpen, setBlueprintDrawerOpen] = useState(false)
  const [tab, setTab] = useState('blueprints')

  return (
    <div className='flex min-h-0 flex-1 flex-col'>
      {/* 智能体头部 */}
      <div className='border-border/60 flex items-center gap-3 border-b px-4 py-3'>
        <div className='bg-muted flex h-10 w-10 items-center justify-center rounded-lg text-xl'>
          {agent.avatar ?? '🤖'}
        </div>
        <div className='min-w-0 flex-1'>
          <h2 className='truncate text-sm font-semibold'>{agent.name}</h2>
          {agent.description && (
            <p className='text-muted-foreground truncate text-xs'>
              {agent.description}
            </p>
          )}
          <div className='text-muted-foreground mt-1 flex items-center gap-1 text-[10px]'>
            <span className='truncate'>ID: {agent.id}</span>
            <CopyButton
              value={agent.id}
              size='icon'
              className='size-4 p-0'
              iconClassName='size-2.5'
              tooltip='复制智能体 ID'
            />
          </div>
        </div>
      </div>

      {/* Tab 区域 */}
      <Tabs
        value={tab}
        onValueChange={setTab}
        className='flex min-h-0 flex-1 flex-col'
      >
        <TabsList className='mx-4 mt-2 h-7 self-start rounded-full bg-muted p-1'>
          <TabsTrigger
            value='blueprints'
            className='h-5 rounded-full px-3 text-xs data-active:shadow-none'
          >
            {t('Blueprints')}
            <span className='text-muted-foreground ml-1'>
              ({agent.blueprint_count})
            </span>
          </TabsTrigger>
          <TabsTrigger
            value='skills'
            className='h-5 rounded-full px-3 text-xs data-active:shadow-none'
          >
            {t('Skills')}
          </TabsTrigger>
        </TabsList>

        <TabsContent
          value='blueprints'
          className='mt-0 flex-1 overflow-y-auto px-4 py-3'
        >
          <BlueprintList
            agentId={agent.id}
            onCreateBlueprint={() => setBlueprintDrawerOpen(true)}
          />
        </TabsContent>

        <TabsContent
          value='skills'
          className='mt-0 flex-1 overflow-y-auto px-4 py-3'
        >
          <SkillsPage />
        </TabsContent>
      </Tabs>

      {/* 蓝图创建 Drawer */}
      <BlueprintDrawer
        agentId={agent.id}
        open={blueprintDrawerOpen}
        onOpenChange={setBlueprintDrawerOpen}
      />
    </div>
  )
}

// ============================================================================
// 主页面
// ============================================================================

export function Agents() {
  const { t } = useTranslation()
  const [createOpen, setCreateOpen] = useState(false)
  const [selectedAgentId, setSelectedAgentId] = useState<string | null>(null)

  const { data: agents = [], isLoading } = useQuery(agentsQueryOptions)

  const selectedAgent = agents.find((a) => a.id === selectedAgentId) ?? null

  // 默认选中第一个
  if (!selectedAgentId && agents.length > 0 && !isLoading) {
    setSelectedAgentId(agents[0].id)
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Agents')}</SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t('Manage your AI agents and automation blueprints')}
        </SectionPageLayout.Description>
        <SectionPageLayout.Actions>
          <Button
            size='sm'
            className='h-8 gap-1.5 text-xs'
            onClick={() => setCreateOpen(true)}
          >
            <Plus className='h-3.5 w-3.5' aria-hidden='true' />
            {t('New Agent')}
          </Button>
        </SectionPageLayout.Actions>

        <SectionPageLayout.Content>
          <div className='flex h-full min-h-0 gap-4'>
            {/* 左侧智能体列表 */}
            <div className='flex w-[260px] shrink-0 flex-col gap-2 overflow-y-auto'>
              {isLoading ? (
                <div className='flex flex-col gap-2'>
                  {[1, 2, 3].map((i) => (
                    <div
                      key={i}
                      className='bg-muted/40 h-[76px] animate-pulse rounded-lg'
                    />
                  ))}
                </div>
              ) : agents.length === 0 ? (
                <div className='text-muted-foreground flex flex-col items-center justify-center py-16 text-center text-sm'>
                  <Bot
                    className='mb-3 h-10 w-10 opacity-30'
                    aria-hidden='true'
                  />
                  <p>{t('No agents yet')}</p>
                  <Button
                    size='sm'
                    variant='outline'
                    className='mt-3'
                    onClick={() => setCreateOpen(true)}
                  >
                    <Plus className='mr-1.5 h-3.5 w-3.5' aria-hidden='true' />
                    {t('Create Agent')}
                  </Button>
                </div>
              ) : (
                agents.map((agent) => (
                  <AgentCard
                    key={agent.id}
                    agent={agent}
                    selected={agent.id === selectedAgentId}
                    onSelect={() => setSelectedAgentId(agent.id)}
                  />
                ))
              )}
            </div>

            {/* 右侧详情面板 */}
            <div className='border-border/60 flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border'>
              {selectedAgent ? (
                <AgentDetailPanel agent={selectedAgent} />
              ) : (
                <div className='text-muted-foreground flex flex-1 flex-col items-center justify-center gap-2 text-sm'>
                  <Bot className='h-10 w-10 opacity-20' aria-hidden='true' />
                  <p>{t('Select an agent to view details')}</p>
                </div>
              )}
            </div>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <CreateAgentDialog open={createOpen} onOpenChange={setCreateOpen} />
    </>
  )
}
