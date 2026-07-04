/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Bot,
  Check,
  ChevronDown,
  Clock,
  Settings2,
  Sparkles,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Textarea } from '@/components/ui/textarea'
import {
  skillsQueryOptions,
  useCreateBlueprintMutation,
  useUpdateBlueprintMutation,
} from '../api'
import { getSkillCapabilities } from '../lib/skill-utils'
import {
  FrequencyType,
  SkillSourceType,
  TriggerType,
  type Blueprint,
  type BlueprintFormData,
  type Skill,
} from '../types'
import { ArchiveConfigEditor } from './archive-config-editor'
import { ExecutionPreviewBar } from './execution-preview-bar'
import { ModelSelector } from './model-selector'
import { TriggerConfigEditor } from './trigger-config-editor'

// ============================================================================
// Props
// ============================================================================

type BlueprintDrawerProps = {
  agentId: string
  blueprint?: Blueprint // 编辑模式传入，创建模式不传
  open: boolean
  onOpenChange: (open: boolean) => void
}

// ============================================================================
// 初始值
// ============================================================================

const DEFAULT_FORM: BlueprintFormData = {
  name: '',
  description: '',
  trigger: {
    type: TriggerType.SCHEDULED,
    frequency: FrequencyType.DAILY,
    time: '08:00',
    timezone: 'Asia/Shanghai',
  },
  model_group: 'domestic',
  model_name: 'qwen-max',
  skills: [],
}

function toFormData(bp: Blueprint): BlueprintFormData {
  return {
    name: bp.name,
    description: bp.description,
    trigger: bp.trigger,
    model_group: bp.model_group,
    model_name: bp.model_name,
    skills: bp.skills,
    input_variables: bp.input_variables,
    archive_config: bp.archive_config,
  }
}

function toggleSkillId(selected: string[], skillId: string, checked: boolean) {
  if (checked) {
    return selected.includes(skillId) ? selected : [...selected, skillId]
  }
  return selected.filter((id) => id !== skillId)
}

// ============================================================================
// 分区折叠组件
// ============================================================================

function Section({
  icon,
  title,
  subtitle,
  children,
  defaultOpen = true,
}: {
  icon?: React.ReactNode
  title: string
  subtitle?: string
  children: React.ReactNode
  defaultOpen?: boolean
}) {
  const [open, setOpen] = useState(defaultOpen)

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger className='hover:bg-muted/40 border-border/50 flex w-full items-center justify-between rounded-lg border px-3 py-2.5 text-left transition-colors'>
        <div className='flex items-center gap-2.5'>
          {icon && (
            <span className='bg-muted text-muted-foreground flex h-7 w-7 items-center justify-center rounded-md text-sm'>
              {icon}
            </span>
          )}
          <div>
            <div className='text-[13px] font-semibold'>{title}</div>
            {subtitle && (
              <div className='text-muted-foreground text-[11px]'>
                {subtitle}
              </div>
            )}
          </div>
        </div>
        <ChevronDown
          className={cn(
            'text-muted-foreground h-4 w-4 shrink-0 transition-transform',
            open && 'rotate-180'
          )}
          aria-hidden='true'
        />
      </CollapsibleTrigger>
      <CollapsibleContent className='mt-3 px-1'>{children}</CollapsibleContent>
    </Collapsible>
  )
}

function SkillSelector({
  skills,
  value,
  onChange,
}: {
  skills: Skill[]
  value: string[]
  onChange: (value: string[]) => void
}) {
  const { t } = useTranslation()
  const available = skills.filter((skill) => skill.enabled)
  const builtin = available.filter(
    (skill) => skill.source === SkillSourceType.BUILTIN
  )
  const custom = available.filter(
    (skill) => skill.source !== SkillSourceType.BUILTIN
  )
  const groups = [
    { key: 'builtin', title: t('Built-in'), items: builtin },
    { key: 'custom', title: t('Installed'), items: custom },
  ].filter((group) => group.items.length > 0)

  if (available.length === 0) {
    return (
      <div className='text-muted-foreground rounded-lg border border-dashed px-3 py-4 text-center text-xs'>
        {t('No available skills')}
      </div>
    )
  }

  return (
    <div className='flex flex-col gap-3'>
      {groups.map((group) => (
        <div key={group.key} className='flex flex-col gap-2'>
          <div className='text-muted-foreground text-[11px] font-medium tracking-wide uppercase'>
            {group.title}
          </div>
          <div className='grid gap-2'>
            {group.items.map((skill) => {
              const checked = value.includes(skill.id)
              const capabilities = getSkillCapabilities(skill)
              return (
                <label
                  key={skill.id}
                  className={cn(
                    'border-border/70 hover:bg-muted/30 flex cursor-pointer items-start gap-2.5 rounded-lg border p-3 transition-colors',
                    checked && 'border-primary/50 bg-primary/5'
                  )}
                >
                  <Checkbox
                    checked={checked}
                    onCheckedChange={(next) =>
                      onChange(toggleSkillId(value, skill.id, !!next))
                    }
                    className='mt-0.5'
                    aria-label={skill.name}
                  />
                  <div className='min-w-0 flex-1'>
                    <div className='flex items-center gap-1.5'>
                      <span className='truncate text-xs font-medium'>
                        {skill.name}
                      </span>
                      {checked && (
                        <Check
                          className='text-primary h-3 w-3 shrink-0'
                          aria-hidden='true'
                        />
                      )}
                    </div>
                    <p className='text-muted-foreground mt-0.5 line-clamp-2 text-[11px] leading-relaxed'>
                      {skill.description}
                    </p>
                    {capabilities.length > 0 && (
                      <div className='mt-1.5 flex flex-wrap gap-1'>
                        {capabilities.slice(0, 3).map((capability) => (
                          <span
                            key={capability}
                            className='bg-muted text-muted-foreground rounded px-1.5 py-0.5 text-[10px]'
                          >
                            {capability}
                          </span>
                        ))}
                      </div>
                    )}
                  </div>
                </label>
              )
            })}
          </div>
        </div>
      ))}
    </div>
  )
}

// ============================================================================
// 主组件
// ============================================================================

export function BlueprintDrawer({
  agentId,
  blueprint,
  open,
  onOpenChange,
}: BlueprintDrawerProps) {
  const { t } = useTranslation()
  const isEdit = !!blueprint

  const [form, setForm] = useState<BlueprintFormData>(
    blueprint ? toFormData(blueprint) : DEFAULT_FORM
  )

  const createMutation = useCreateBlueprintMutation(agentId)
  const updateMutation = useUpdateBlueprintMutation(agentId)
  const { data: skills = [] } = useQuery(skillsQueryOptions)

  const isPending = createMutation.isPending || updateMutation.isPending

  const updateForm = (patch: Partial<BlueprintFormData>) =>
    setForm((prev) => ({ ...prev, ...patch }))

  const handleSubmit = () => {
    if (!form.name.trim()) {
      toast.error(t('Please enter a blueprint name'))
      return
    }

    if (isEdit && blueprint) {
      updateMutation.mutate(
        { id: blueprint.id, data: form },
        {
          onSuccess: () => {
            toast.success(t('Blueprint updated'))
            onOpenChange(false)
          },
          onError: () => toast.error(t('Failed to update blueprint')),
        }
      )
    } else {
      createMutation.mutate(form, {
        onSuccess: () => {
          toast.success(t('Blueprint created'))
          onOpenChange(false)
          setForm(DEFAULT_FORM)
        },
        onError: () => toast.error(t('Failed to create blueprint')),
      })
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side='right'
        className='flex w-full max-w-[520px] flex-col gap-0 p-0'
      >
        {/* 头部 */}
        <SheetHeader className='border-border/60 shrink-0 border-b px-5 py-4'>
          <SheetTitle className='text-base'>
            {isEdit ? t('Edit Blueprint') : t('Create Blueprint')}
          </SheetTitle>
        </SheetHeader>

        {/* 内容区域（可滚动） */}
        <div className='flex-1 overflow-y-auto px-5 py-4'>
          <div className='flex flex-col gap-4'>
            {/* 基本信息 */}
            <div className='flex flex-col gap-3'>
              <div className='flex flex-col gap-1.5'>
                <Label className='text-xs'>{t('Blueprint Name')} *</Label>
                <Input
                  className='h-8 text-sm'
                  placeholder={t('e.g. Daily News Summary')}
                  value={form.name}
                  onChange={(e) => updateForm({ name: e.target.value })}
                />
              </div>
              <div className='flex flex-col gap-1.5'>
                <Label className='text-muted-foreground text-xs'>
                  {t('Description')}
                </Label>
                <Textarea
                  className='min-h-[60px] resize-none text-xs'
                  placeholder={t('Optional description...')}
                  value={form.description ?? ''}
                  onChange={(e) => updateForm({ description: e.target.value })}
                />
              </div>
            </div>

            {/* 触发器 */}
            <Section
              icon={<Clock className='h-3.5 w-3.5' />}
              title={t('Trigger')}
              subtitle={t('When to run this blueprint')}
            >
              <TriggerConfigEditor
                value={form.trigger}
                onChange={(trigger) => updateForm({ trigger })}
              />
            </Section>

            {/* 模型选择 */}
            <Section
              icon={<Bot className='h-3.5 w-3.5' />}
              title={t('Model')}
              subtitle={t('AI model to use')}
            >
              <ModelSelector
                groupId={form.model_group}
                modelName={form.model_name}
                onGroupChange={(g) => updateForm({ model_group: g })}
                onModelChange={(m) => updateForm({ model_name: m })}
              />
            </Section>

            <Section
              icon={<Sparkles className='h-3.5 w-3.5' />}
              title='Skill'
              subtitle={t('Execution constraints and capabilities')}
              defaultOpen={false}
            >
              <SkillSelector
                skills={skills}
                value={form.skills}
                onChange={(selectedSkills) =>
                  updateForm({ skills: selectedSkills })
                }
              />
            </Section>

            {/* 高级配置（折叠） */}
            <Section
              icon={<Settings2 className='h-3.5 w-3.5' />}
              title={t('Archive')}
              subtitle={t('Where to store outputs')}
              defaultOpen={false}
            >
              <ArchiveConfigEditor
                value={form.archive_config}
                onChange={(archive_config) => updateForm({ archive_config })}
              />
            </Section>
          </div>
        </div>

        {/* 执行预览底部栏 */}
        <ExecutionPreviewBar
          blueprint={{ ...form, agent_id: agentId }}
          skills={skills}
        />

        {/* 底部操作栏 */}
        <div className='border-border/60 flex shrink-0 items-center justify-end gap-2 border-t px-5 py-3'>
          <Button
            variant='outline'
            size='sm'
            onClick={() => onOpenChange(false)}
            disabled={isPending}
          >
            {t('Cancel')}
          </Button>
          <Button size='sm' onClick={handleSubmit} disabled={isPending}>
            {isPending
              ? t('Saving...')
              : isEdit
                ? t('Save Changes')
                : t('Create Blueprint')}
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  )
}
