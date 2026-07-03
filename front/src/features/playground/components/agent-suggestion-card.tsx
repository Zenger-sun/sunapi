/*
Copyright (C) 2026 SunAPI Project Owner
*/
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Bot, ChevronRight, Play, Settings2, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { agentsQueryOptions, blueprintsByAgentQueryOptions } from '@/features/agents/api'
import { BlueprintStatus, type Agent } from '@/features/agents/types'
import { cn } from '@/lib/utils'

// ============================================================================
// 智能体任务建议卡片（内嵌于对话消息流中）
// ============================================================================

type AgentSuggestionCardProps = {
  /** 触发类型：AI 识别到自动化意图时推荐创建；或直接 @ 了某个智能体 */
  mode: 'suggest_create' | 'query_status' | 'run_now' | 'view_report'
  /** 对应的智能体（query_status / run_now / view_report 时提供） */
  agent?: Agent
  /** 原始对话文本（suggest_create 时用于预填蓝图描述） */
  seedPrompt?: string
  /** 点击"一键开启任务" */
  onStartTask?: (agentId: string) => void
  /** 点击"自定义配置" → 跳转到 /agents */
  onGoToAgent?: (agentId: string) => void
  /** 点击"切换到智能体 Tab" */
  onSwitchToAgent?: () => void
}

export function AgentSuggestionCard({
  mode,
  agent,
  seedPrompt,
  onStartTask,
  onGoToAgent,
  onSwitchToAgent,
}: AgentSuggestionCardProps) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)

  if (mode === 'suggest_create') {
    return (
      <SuggestCreateCard
        seedPrompt={seedPrompt}
        onSwitchToAgent={onSwitchToAgent}
      />
    )
  }

  if (!agent) return null

  return (
    <AgentStatusCard
      agent={agent}
      mode={mode}
      expanded={expanded}
      onToggleExpand={() => setExpanded((v) => !v)}
      onStartTask={onStartTask}
      onGoToAgent={onGoToAgent}
    />
  )
}

// ============================================================================
// 建议创建卡片
// ============================================================================

function SuggestCreateCard({
  seedPrompt,
  onSwitchToAgent,
}: {
  seedPrompt?: string
  onSwitchToAgent?: () => void
}) {
  const { t } = useTranslation()

  return (
    <div className='border-border/60 bg-muted/20 my-2 flex flex-col gap-3 rounded-lg border p-3'>
      <div className='flex items-start gap-2.5'>
        <div className='bg-amber-500/10 flex h-8 w-8 shrink-0 items-center justify-center rounded-md'>
          <Zap className='h-4 w-4 text-amber-600' aria-hidden='true' />
        </div>
        <div className='min-w-0 flex-1'>
          <p className='text-sm font-medium'>
            {t('I noticed you may need an automated task')}
          </p>
          <p className='text-muted-foreground mt-0.5 text-xs'>
            {t('You can create an agent blueprint to automate this workflow')}
          </p>
        </div>
      </div>

      {seedPrompt && (
        <div className='bg-muted/40 rounded-md px-2.5 py-2 text-xs'>
          <span className='text-muted-foreground'>任务描述：</span>
          <span className='line-clamp-2'>{seedPrompt}</span>
        </div>
      )}

      <div className='flex flex-wrap gap-2'>
        <Button
          size='sm'
          className='h-7 gap-1.5 text-xs'
          onClick={onSwitchToAgent}
        >
          <Bot className='h-3.5 w-3.5' aria-hidden='true' />
          {t('Open Agent Workspace')}
        </Button>
        <Button
          size='sm'
          variant='outline'
          className='h-7 text-xs'
          onClick={onSwitchToAgent}
        >
          {t('Learn more')}
        </Button>
      </div>
    </div>
  )
}

// ============================================================================
// 智能体状态/操作卡片
// ============================================================================

const STATUS_CONFIG = {
  [BlueprintStatus.RUNNING]: {
    dot: 'bg-blue-500 animate-pulse',
    text: '执行中',
    color: 'text-blue-600 dark:text-blue-400',
  },
  [BlueprintStatus.PENDING]: {
    dot: 'bg-amber-500',
    text: '待执行',
    color: 'text-amber-600 dark:text-amber-400',
  },
  [BlueprintStatus.COMPLETED]: {
    dot: 'bg-emerald-500',
    text: '已完成',
    color: 'text-emerald-600 dark:text-emerald-400',
  },
  [BlueprintStatus.FAILED]: {
    dot: 'bg-red-500',
    text: '执行失败',
    color: 'text-red-600 dark:text-red-400',
  },
  [BlueprintStatus.PAUSED]: {
    dot: 'bg-slate-400',
    text: '已暂停',
    color: 'text-slate-600 dark:text-slate-400',
  },
}

function AgentStatusCard({
  agent,
  mode,
  expanded,
  onToggleExpand,
  onStartTask,
  onGoToAgent,
}: {
  agent: Agent
  mode: 'query_status' | 'run_now' | 'view_report'
  expanded: boolean
  onToggleExpand: () => void
  onStartTask?: (agentId: string) => void
  onGoToAgent?: (agentId: string) => void
}) {
  const { t } = useTranslation()
  const { data: blueprints = [] } = useQuery(
    blueprintsByAgentQueryOptions(agent.id)
  )

  const latestBlueprint = blueprints[0]
  const statusConfig = latestBlueprint
    ? STATUS_CONFIG[latestBlueprint.status]
    : null

  const modeMessages: Record<typeof mode, string> = {
    query_status: `查询 @${agent.name} 状态`,
    run_now: `立即执行 @${agent.name}`,
    view_report: `查看 @${agent.name} 最新报告`,
  }

  return (
    <div className='border-border/60 bg-muted/20 my-2 flex flex-col gap-0 overflow-hidden rounded-lg border'>
      {/* 头部 */}
      <button
        type='button'
        className='hover:bg-muted/30 flex w-full items-center gap-2.5 px-3 py-2.5 text-left transition-colors'
        onClick={onToggleExpand}
      >
        <div className='bg-primary/10 flex h-8 w-8 shrink-0 items-center justify-center rounded-md'>
          <Bot className='text-primary h-4 w-4' aria-hidden='true' />
        </div>
        <div className='min-w-0 flex-1'>
          <p className='text-sm font-medium'>🤖 {agent.name}</p>
          <p className='text-muted-foreground text-xs'>{modeMessages[mode]}</p>
        </div>
        {statusConfig && (
          <span className='flex shrink-0 items-center gap-1.5 text-xs'>
            <span
              className={cn('h-1.5 w-1.5 rounded-full', statusConfig.dot)}
            />
            <span className={statusConfig.color}>{statusConfig.text}</span>
          </span>
        )}
        <ChevronRight
          className={cn(
            'text-muted-foreground h-4 w-4 shrink-0 transition-transform',
            expanded && 'rotate-90'
          )}
          aria-hidden='true'
        />
      </button>

      {/* 展开详情 */}
      {expanded && (
        <div className='border-border/40 border-t px-3 py-3'>
          {/* 蓝图列表（最近3条） */}
          {blueprints.length > 0 ? (
            <div className='flex flex-col gap-1.5'>
              <p className='text-muted-foreground mb-1 text-[11px] font-medium uppercase tracking-wide'>
                近期蓝图
              </p>
              {blueprints.slice(0, 3).map((bp) => {
                const sc = STATUS_CONFIG[bp.status]
                return (
                  <div
                    key={bp.id}
                    className='border-border/40 flex items-center gap-2 rounded-md border px-2 py-1.5'
                  >
                    <span
                      className={cn('h-1.5 w-1.5 rounded-full', sc.dot)}
                    />
                    <span className='min-w-0 flex-1 truncate text-xs'>
                      {bp.name}
                    </span>
                    <span className={cn('shrink-0 text-[10px]', sc.color)}>
                      {sc.text}
                    </span>
                  </div>
                )
              })}
            </div>
          ) : (
            <p className='text-muted-foreground text-xs'>
              该智能体暂无蓝图任务
            </p>
          )}

          {/* 操作按钮 */}
          <div className='mt-3 flex flex-wrap gap-2'>
            {mode === 'run_now' && (
              <Button
                size='sm'
                className='h-7 gap-1.5 text-xs'
                onClick={() => onStartTask?.(agent.id)}
              >
                <Play className='h-3 w-3' aria-hidden='true' />
                {t('Run Now')}
              </Button>
            )}
            <Button
              size='sm'
              variant='outline'
              className='h-7 gap-1.5 text-xs'
              onClick={() => onGoToAgent?.(agent.id)}
            >
              <Settings2 className='h-3 w-3' aria-hidden='true' />
              {t('View Agent')}
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
