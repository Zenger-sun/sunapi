/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  ExternalLink,
  GitBranch,
  Package,
  Plug,
  Plus,
  Settings,
  ShieldCheck,
  Store,
  Trash2,
  Wrench,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import {
  skillsQueryOptions,
  skillMarketplaceQueryOptions,
  useCreateSkillMutation,
  useDeleteSkillMutation,
  useToggleSkillMutation,
} from '../api'
import { getSkillCapabilities, getSkillTags } from '../lib/skill-utils'
import {
  SkillSourceType,
  type Skill,
  type SkillMarketplaceItem,
} from '../types'
import { getUserGroups, getUserModels } from '@/features/playground/api'
import type {
  GroupOption,
  ModelOption,
} from '@/features/playground/types'

// ============================================================================
// Skill 来源徽章
// ============================================================================

const SOURCE_CONFIG: Record<
  SkillSourceType,
  { label: string; className: string }
> = {
  [SkillSourceType.BUILTIN]: {
    label: '系统内置',
    className: 'bg-slate-100 text-slate-700 border-slate-200',
  },
  [SkillSourceType.MARKETPLACE]: {
    label: '商城安装',
    className: 'bg-blue-50 text-blue-700 border-blue-200',
  },
  [SkillSourceType.GITHUB]: {
    label: 'GitHub',
    className: 'bg-purple-50 text-purple-700 border-purple-200',
  },
  [SkillSourceType.CUSTOM]: {
    label: '自定义',
    className: 'bg-amber-50 text-amber-700 border-amber-200',
  },
}

// ============================================================================
// 单个 Skill 卡片
// ============================================================================

function SkillCard({ skill }: { skill: Skill }) {
  const { t } = useTranslation()
  const toggleMutation = useToggleSkillMutation()
  const deleteMutation = useDeleteSkillMutation()
  const src = SOURCE_CONFIG[skill.source]
  const isBuiltin = skill.source === SkillSourceType.BUILTIN
  const capabilities = getSkillCapabilities(skill)
  const tags = getSkillTags(skill)

  return (
    <div className='border-border/60 hover:bg-muted/10 flex items-start gap-3 rounded-lg border p-4 transition-colors'>
      {/* 图标占位 */}
      <div className='bg-muted flex h-9 w-9 shrink-0 items-center justify-center rounded-md text-base'>
        {isBuiltin ? (
          <ShieldCheck className='h-4 w-4' aria-hidden='true' />
        ) : (
          <Plug className='h-4 w-4' aria-hidden='true' />
        )}
      </div>

      {/* 信息 */}
      <div className='min-w-0 flex-1'>
        <div className='flex flex-wrap items-center gap-2'>
          <span className='text-sm font-medium'>{skill.name}</span>
          <span className='text-muted-foreground text-xs'>
            v{skill.version}
          </span>
          <span
            className={cn(
              'rounded border px-1.5 py-0.5 text-[10px] font-medium',
              src.className
            )}
          >
            {t(src.label)}
          </span>
        </div>
        <p className='text-muted-foreground mt-0.5 line-clamp-1 text-xs'>
          {skill.description}
        </p>
        {capabilities.length > 0 && (
          <div className='mt-1.5 flex flex-wrap gap-1'>
            {capabilities.map((capability) => (
              <Badge
                key={capability}
                variant='secondary'
                className='h-4 px-1.5 text-[10px]'
              >
                {capability}
              </Badge>
            ))}
          </div>
        )}
        {tags.length > 0 && (
          <div className='mt-1.5 flex flex-wrap gap-1'>
            {tags.map((tag) => (
              <Badge
                key={tag}
                variant='outline'
                className='h-4 px-1.5 text-[10px]'
              >
                #{tag}
              </Badge>
            ))}
          </div>
        )}
        {skill.source_url && (
          <a
            href={skill.source_url}
            target='_blank'
            rel='noopener noreferrer'
            className='text-primary mt-0.5 flex items-center gap-1 text-[10px] hover:underline'
          >
            <ExternalLink className='h-2.5 w-2.5' aria-hidden='true' />
            {skill.source_url}
          </a>
        )}
        {skill.permissions && skill.permissions.length > 0 && (
          <div className='mt-1.5 flex flex-wrap gap-1'>
            {skill.permissions.map((p) => (
              <Badge
                key={p}
                variant='secondary'
                className='h-4 px-1.5 text-[10px]'
              >
                {p}
              </Badge>
            ))}
          </div>
        )}
      </div>

      {/* 操作 */}
      <div className='flex shrink-0 items-center gap-2'>
        <Switch
          checked={skill.enabled}
          onCheckedChange={(enabled) => {
            if (isBuiltin) return
            toggleMutation.mutate(
              { id: skill.id, enabled },
              {
                onSuccess: () =>
                  toast.success(
                    enabled ? t('Skill enabled') : t('Skill disabled')
                  ),
              }
            )
          }}
          disabled={isBuiltin || toggleMutation.isPending}
          aria-label={
            isBuiltin
              ? t('Built-in skill is always enabled')
              : skill.enabled
                ? t('Disable skill')
                : t('Enable skill')
          }
        />
        {!isBuiltin && (
          <>
            <Button size='sm' variant='ghost' className='h-7 w-7 p-0'>
              <Settings className='h-3.5 w-3.5' aria-hidden='true' />
            </Button>
            <Button
              size='sm'
              variant='ghost'
              className='text-destructive hover:text-destructive h-7 w-7 p-0'
              onClick={() => {
                deleteMutation.mutate(skill.id, {
                  onSuccess: () => toast.success(t('Skill removed')),
                })
              }}
            >
              <Trash2 className='h-3.5 w-3.5' aria-hidden='true' />
            </Button>
          </>
        )}
      </div>
    </div>
  )
}

// ============================================================================
// 添加 Skill 弹窗
// ============================================================================

type AddSkillDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

function splitTags(value: string) {
  return value
    .split(/[,，\s]+/)
    .map((item) => item.trim())
    .filter(Boolean)
}

function pickDefaultGroup(groups: GroupOption[]) {
  return (
    groups.find((group) => group.value === 'default')?.value ??
    groups[0]?.value ??
    'default'
  )
}

function MarketplaceSkillRow({
  skill,
  onInstall,
  installing,
}: {
  skill: SkillMarketplaceItem
  onInstall: (skill: SkillMarketplaceItem) => void
  installing: boolean
}) {
  return (
    <div className='border-border/60 flex items-start gap-3 rounded-lg border p-3'>
      <div className='bg-muted flex h-8 w-8 shrink-0 items-center justify-center rounded-md'>
        <Package className='h-4 w-4' aria-hidden='true' />
      </div>
      <div className='min-w-0 flex-1'>
        <div className='flex items-center gap-2'>
          <span className='truncate text-sm font-medium'>{skill.name}</span>
          <span className='text-muted-foreground shrink-0 text-[10px]'>
            v{skill.version}
          </span>
        </div>
        <p className='text-muted-foreground mt-0.5 line-clamp-2 text-xs'>
          {skill.description}
        </p>
        {!!skill.tags?.length && (
          <div className='mt-1.5 flex flex-wrap gap-1'>
            {skill.tags.slice(0, 4).map((tag) => (
              <Badge key={tag} variant='outline' className='h-4 px-1.5 text-[10px]'>
                #{tag}
              </Badge>
            ))}
          </div>
        )}
      </div>
      <Button
        size='sm'
        variant={skill.installed ? 'outline' : 'default'}
        className='h-7 shrink-0 text-xs'
        disabled={skill.installed || installing}
        onClick={() => onInstall(skill)}
      >
        {skill.installed ? '已安装' : '安装'}
      </Button>
    </div>
  )
}

function AddSkillDialog({ open, onOpenChange }: AddSkillDialogProps) {
  const { t } = useTranslation()
  const [tab, setTab] = useState<'marketplace' | 'github' | 'custom'>(
    'marketplace'
  )
  const [githubUrl, setGithubUrl] = useState('')
  const [customName, setCustomName] = useState('')
  const [customDesc, setCustomDesc] = useState('')
  const [customTags, setCustomTags] = useState('')
  const createMutation = useCreateSkillMutation()
  const { data: marketplace = [], isLoading: marketplaceLoading } = useQuery(
    skillMarketplaceQueryOptions
  )
  const { data: groups = [] } = useQuery({
    queryKey: ['playground-groups'],
    queryFn: getUserGroups,
  })
  const defaultGroup = pickDefaultGroup(groups)
  const [selectedGroup, setSelectedGroup] = useState(defaultGroup)
  const effectiveGroup = selectedGroup || defaultGroup
  const { data: models = [] } = useQuery({
    queryKey: ['playground-models', effectiveGroup],
    queryFn: () => getUserModels(effectiveGroup),
    enabled: !!effectiveGroup,
  })
  const selectedModel = useMemo(
    () => models[0]?.value ?? 'gpt-4.1-mini',
    [models]
  )
  const [customModel, setCustomModel] = useState('')
  const effectiveModel = customModel || selectedModel

  useEffect(() => {
    if (!selectedGroup && defaultGroup) {
      setSelectedGroup(defaultGroup)
    }
  }, [defaultGroup, selectedGroup])

  const installMarketplaceSkill = (skill: SkillMarketplaceItem) => {
    createMutation.mutate(
      {
        name: skill.name,
        description: skill.description,
        source_type: SkillSourceType.MARKETPLACE,
        source_url: skill.source_url,
        marketplace_id: skill.id,
      },
      {
        onSuccess: () => toast.success(t('Skill installed')),
      }
    )
  }

  const handleImportGithub = () => {
    if (!githubUrl.trim()) {
      toast.error(t('Please enter a GitHub URL'))
      return
    }
    createMutation.mutate(
      {
        name: '',
        description: '',
        source_type: SkillSourceType.GITHUB,
        source_url: githubUrl,
      },
      {
        onSuccess: () => {
          toast.success(t('Skill imported'))
          onOpenChange(false)
          setGithubUrl('')
        },
      }
    )
  }

  const handleCreateCustom = () => {
    if (!customName.trim()) {
      toast.error(t('Please enter a skill name'))
      return
    }
    if (!customDesc.trim()) {
      toast.error(t('Please describe what this skill should do'))
      return
    }
    createMutation.mutate(
      {
        name: customName,
        description: customDesc,
        source_type: SkillSourceType.CUSTOM,
        model_group: effectiveGroup,
        model_name: effectiveModel,
        tags: splitTags(customTags),
      },
      {
        onSuccess: () => {
          toast.success(t('Skill created'))
          onOpenChange(false)
          setCustomName('')
          setCustomDesc('')
          setCustomTags('')
        },
      }
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-w-2xl'>
        <DialogHeader>
          <DialogTitle>{t('Add Skill')}</DialogTitle>
          <DialogDescription>
            {t('Choose how to add a new skill')}
          </DialogDescription>
        </DialogHeader>

        <Tabs value={tab} onValueChange={(v) => setTab(v as typeof tab)}>
          <TabsList className='grid h-8 w-full grid-cols-3'>
            <TabsTrigger value='marketplace' className='gap-1.5 text-xs'>
              <Store className='h-3 w-3' aria-hidden='true' />
              {t('Marketplace')}
            </TabsTrigger>
            <TabsTrigger value='github' className='gap-1.5 text-xs'>
              <GitBranch className='h-3 w-3' aria-hidden='true' />
              GitHub
            </TabsTrigger>
            <TabsTrigger value='custom' className='gap-1.5 text-xs'>
              <Wrench className='h-3 w-3' aria-hidden='true' />
              {t('Custom')}
            </TabsTrigger>
          </TabsList>

          {/* 商城 */}
          <TabsContent value='marketplace' className='mt-4'>
            <div className='max-h-[360px] space-y-2 overflow-y-auto pr-1'>
              {marketplaceLoading ? (
                <div className='flex flex-col gap-2'>
                  {[1, 2, 3].map((item) => (
                    <div
                      key={item}
                      className='bg-muted/40 h-[86px] animate-pulse rounded-lg'
                    />
                  ))}
                </div>
              ) : (
                marketplace.map((skill) => (
                  <MarketplaceSkillRow
                    key={skill.id}
                    skill={skill}
                    installing={createMutation.isPending}
                    onInstall={installMarketplaceSkill}
                  />
                ))
              )}
            </div>
          </TabsContent>

          {/* GitHub 导入 */}
          <TabsContent value='github' className='mt-4'>
            <div className='flex flex-col gap-3'>
              <div className='flex flex-col gap-1.5'>
                <Label className='text-xs'>{t('Repository URL')}</Label>
                <Input
                  className='h-8 text-xs'
                  placeholder='https://github.com/user/my-skill'
                  value={githubUrl}
                  onChange={(e) => setGithubUrl(e.target.value)}
                />
              </div>
              <p className='text-muted-foreground text-[11px]'>
                {t(
                  'Repository must contain skill.json or SKILL.md; imported metadata is parsed on the server.'
                )}
              </p>
            </div>
          </TabsContent>

          {/* 自定义 */}
          <TabsContent value='custom' className='mt-4'>
            <div className='flex flex-col gap-3'>
              <div className='flex flex-col gap-1.5'>
                <Label className='text-xs'>{t('Skill Name')} *</Label>
                <Input
                  className='h-8 text-xs'
                  placeholder={t('e.g. My Custom Skill')}
                  value={customName}
                  onChange={(e) => setCustomName(e.target.value)}
                />
              </div>
              <div className='flex flex-col gap-1.5'>
                <Label className='text-xs'>{t('Description')} *</Label>
                <Textarea
                  className='min-h-[60px] resize-none text-xs'
                  placeholder={t('What does this skill do?')}
                  value={customDesc}
                  onChange={(e) => setCustomDesc(e.target.value)}
                />
              </div>
              <div className='grid grid-cols-2 gap-3'>
                <div className='flex flex-col gap-1.5'>
                  <Label className='text-xs'>{t('Group')}</Label>
                  <Select
                    value={effectiveGroup}
                    onValueChange={(value) => {
                      setSelectedGroup(value)
                      setCustomModel('')
                    }}
                  >
                    <SelectTrigger className='h-8 text-xs'>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {groups.length === 0 ? (
                        <SelectItem value='default'>default</SelectItem>
                      ) : (
                        groups.map((group) => (
                          <SelectItem key={group.value} value={group.value}>
                            {group.label}
                          </SelectItem>
                        ))
                      )}
                    </SelectContent>
                  </Select>
                </div>
                <div className='flex flex-col gap-1.5'>
                  <Label className='text-xs'>{t('Model')}</Label>
                  <Select value={effectiveModel} onValueChange={setCustomModel}>
                    <SelectTrigger className='h-8 text-xs'>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {models.length === 0 ? (
                        <SelectItem value='gpt-4.1-mini'>gpt-4.1-mini</SelectItem>
                      ) : (
                        models.map((model: ModelOption) => (
                          <SelectItem key={model.value} value={model.value}>
                            {model.label}
                          </SelectItem>
                        ))
                      )}
                    </SelectContent>
                  </Select>
                </div>
              </div>
              <div className='flex flex-col gap-1.5'>
                <Label className='text-xs'>{t('Tags')}</Label>
                <Input
                  className='h-8 text-xs'
                  placeholder='report, data, browser'
                  value={customTags}
                  onChange={(e) => setCustomTags(e.target.value)}
                />
              </div>
            </div>
          </TabsContent>
        </Tabs>

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
            onClick={tab === 'github' ? handleImportGithub : handleCreateCustom}
            disabled={createMutation.isPending || tab === 'marketplace'}
          >
            {createMutation.isPending
              ? t('Adding...')
              : tab === 'github'
                ? t('Import')
                : t('Create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ============================================================================
// Skill 管理主页
// ============================================================================

export function SkillsPage() {
  const { t } = useTranslation()
  const [addOpen, setAddOpen] = useState(false)
  const [search, setSearch] = useState('')

  const { data: skills = [], isLoading } = useQuery(skillsQueryOptions)

  const filtered = skills.filter(
    (s) =>
      !search ||
      s.name.toLowerCase().includes(search.toLowerCase()) ||
      s.description.toLowerCase().includes(search.toLowerCase())
  )

  const installed = filtered.filter((s) => s.source !== SkillSourceType.BUILTIN)
  const builtin = filtered.filter((s) => s.source === SkillSourceType.BUILTIN)

  return (
    <>
      <div className='flex flex-col gap-4'>
        {/* 顶部操作栏 */}
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <Input
            className='h-8 w-[220px] text-xs'
            placeholder={t('Search skills...')}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <Button
            size='sm'
            className='h-8 gap-1.5 text-xs'
            onClick={() => setAddOpen(true)}
          >
            <Plus className='h-3.5 w-3.5' aria-hidden='true' />
            {t('Add Skill')}
          </Button>
        </div>

        {isLoading ? (
          <div className='flex flex-col gap-2'>
            {[1, 2, 3].map((i) => (
              <div
                key={i}
                className='bg-muted/40 h-[76px] animate-pulse rounded-lg'
              />
            ))}
          </div>
        ) : (
          <div className='flex flex-col gap-6'>
            {/* 已安装 */}
            {installed.length > 0 && (
              <div className='flex flex-col gap-2'>
                <h3 className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
                  {t('Installed')} ({installed.length})
                </h3>
                <div className='flex flex-col gap-2'>
                  {installed.map((s) => (
                    <SkillCard key={s.id} skill={s} />
                  ))}
                </div>
              </div>
            )}

            {/* 内置 */}
            {builtin.length > 0 && (
              <div className='flex flex-col gap-2'>
                <h3 className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
                  {t('Built-in')} ({builtin.length})
                </h3>
                <div className='flex flex-col gap-2'>
                  {builtin.map((s) => (
                    <SkillCard key={s.id} skill={s} />
                  ))}
                </div>
              </div>
            )}

            {filtered.length === 0 && (
              <div className='text-muted-foreground flex flex-col items-center justify-center py-10 text-sm'>
                <p>{t('No skills found')}</p>
                <Button
                  size='sm'
                  variant='outline'
                  className='mt-3'
                  onClick={() => setAddOpen(true)}
                >
                  <Plus className='mr-1.5 h-3.5 w-3.5' aria-hidden='true' />
                  {t('Add your first skill')}
                </Button>
              </div>
            )}
          </div>
        )}
      </div>

      <AddSkillDialog open={addOpen} onOpenChange={setAddOpen} />
    </>
  )
}
