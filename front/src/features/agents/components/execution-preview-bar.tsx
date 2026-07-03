/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useState } from 'react'
import {
  ArrowRight,
  ChevronUp,
  Clock,
  FlaskConical,
  Zap,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { type Blueprint } from '../types'

type ExecutionPreviewBarProps = {
  blueprint?: Partial<Blueprint>
  className?: string
}

type FlowStep = {
  id: string
  label: string
  detail?: string
  type: 'trigger' | 'input' | 'skill' | 'model' | 'output'
}

function buildFlowSteps(blueprint?: Partial<Blueprint>): FlowStep[] {
  if (!blueprint) return []

  const steps: FlowStep[] = []

  // 触发器步骤
  const freq = blueprint.trigger?.frequency
  const time = blueprint.trigger?.time
  steps.push({
    id: 'trigger',
    type: 'trigger',
    label: '触发器',
    detail: freq
      ? `${freq === 'daily' ? '每天' : freq === 'weekly' ? '每周' : freq} ${time ?? ''}`
      : '手动触发',
  })

  // 输入变量步骤（如有）
  const vars = blueprint.input_variables
  if (vars && Object.keys(vars).length > 0) {
    steps.push({
      id: 'input',
      type: 'input',
      label: '输入变量',
      detail: Object.entries(vars)
        .map(([k, v]) => `${k}: "${v}"`)
        .join(', '),
    })
  }

  // Skill 步骤
  if (blueprint.skills && blueprint.skills.length > 0) {
    blueprint.skills.forEach((skillId, idx) => {
      steps.push({
        id: `skill-${idx}`,
        type: 'skill',
        label: `Skill: ${skillId}`,
        detail: undefined,
      })
    })
  }

  // 模型步骤
  if (blueprint.model_name) {
    steps.push({
      id: 'model',
      type: 'model',
      label: `模型: ${blueprint.model_name}`,
      detail: '预估消耗 ~2000 tokens',
    })
  }

  // 输出步骤
  if (blueprint.archive_config) {
    steps.push({
      id: 'output',
      type: 'output',
      label: '输出归档',
      detail:
        blueprint.archive_config.type === 'platform'
          ? `存储至 ${blueprint.archive_config.path ?? '/archives'}`
          : blueprint.archive_config.type,
    })
  }

  return steps
}

const STEP_COLORS: Record<FlowStep['type'], string> = {
  trigger: 'bg-blue-500',
  input: 'bg-amber-500',
  skill: 'bg-violet-500',
  model: 'bg-emerald-500',
  output: 'bg-orange-500',
}

export function ExecutionPreviewBar({
  blueprint,
  className,
}: ExecutionPreviewBarProps) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)

  const steps = buildFlowSteps(blueprint)
  const stepCount = steps.length
  const skillCount = steps.filter((s) => s.type === 'skill').length
  const hasModel = steps.some((s) => s.type === 'model')

  return (
    <div
      className={cn(
        'border-border/60 bg-background/95 border-t backdrop-blur-sm',
        className
      )}
    >
      {/* 折叠内容 */}
      {expanded && (
        <div className='border-border/40 border-b px-4 py-4'>
          <h4 className='mb-3 text-sm font-semibold'>
            {t('Execution Flow')}
          </h4>

          {steps.length === 0 ? (
            <p className='text-muted-foreground text-xs'>
              {t('Configure blueprint to see preview')}
            </p>
          ) : (
            <div className='flex flex-wrap items-center gap-1'>
              {steps.map((step, idx) => (
                <div key={step.id} className='flex items-center gap-1'>
                  {/* 步骤节点 */}
                  <div className='flex flex-col items-start gap-0.5'>
                    <div
                      className={cn(
                        'flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-white',
                        STEP_COLORS[step.type]
                      )}
                    >
                      <span className='text-xs font-medium'>{step.label}</span>
                    </div>
                    {step.detail && (
                      <span className='text-muted-foreground max-w-[160px] truncate pl-1 text-[10px]'>
                        {step.detail}
                      </span>
                    )}
                  </div>

                  {/* 箭头 */}
                  {idx < steps.length - 1 && (
                    <ArrowRight
                      className='text-muted-foreground mx-0.5 h-3.5 w-3.5 shrink-0'
                      aria-hidden='true'
                    />
                  )}
                </div>
              ))}
            </div>
          )}

          {/* 预估信息 */}
          {stepCount > 0 && (
            <div className='text-muted-foreground mt-4 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs'>
              <span className='flex items-center gap-1'>
                <Clock className='h-3.5 w-3.5' aria-hidden='true' />
                预估耗时: ~30s
              </span>
              {hasModel && <span>预估费用: ¥0.08 / 次</span>}
              {skillCount > 0 && <span>调用 Skill: {skillCount} 个</span>}
              <span>输入变量: {blueprint?.input_variables ? Object.keys(blueprint.input_variables).length : 0} 个</span>
            </div>
          )}

          {/* 测试运行按钮 */}
          <div className='mt-3'>
            <Button
              size='sm'
              variant='outline'
              className='h-7 gap-1.5 text-xs'
            >
              <FlaskConical className='h-3.5 w-3.5' aria-hidden='true' />
              {t('Test Run')}
            </Button>
          </div>
        </div>
      )}

      {/* 常驻底部栏 */}
      <button
        type='button'
        onClick={() => setExpanded((v) => !v)}
        className='hover:bg-muted/30 flex w-full items-center justify-between px-4 py-2 transition-colors'
        aria-expanded={expanded}
      >
        <div className='flex items-center gap-2'>
          <Zap
            className='text-amber-500 h-3.5 w-3.5'
            aria-hidden='true'
          />
          <span className='text-xs font-medium'>{t('Execution Preview')}</span>
          {stepCount > 0 && (
            <span className='text-muted-foreground text-xs'>
              {stepCount} 步骤
              {skillCount > 0 && ` · ${skillCount} 个 Skill`}
              {hasModel && ' · 预估 ~30s'}
            </span>
          )}
        </div>
        <ChevronUp
          className={cn(
            'text-muted-foreground h-4 w-4 transition-transform',
            expanded && 'rotate-180'
          )}
          aria-hidden='true'
        />
      </button>
    </div>
  )
}
