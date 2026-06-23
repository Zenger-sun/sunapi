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

For commercial licensing, please contact support@quantumnous.com
*/
import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  BarChart3,
  Check,
  FileText,
  KeyRound,
  Lock,
  Radio,
  RotateCcw,
  Save,
  Settings as SettingsIcon,
  ShieldCheck,
  Sparkles,
  Trash2,
} from 'lucide-react'
import { toast } from 'sonner'
import { SectionPageLayout } from '@/components/layout'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { PasswordInput } from '@/components/password-input'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { useSystemConfigStore } from '@/stores/system-config-store'
import {
  type AppSettings,
  clearUsageLogs,
  getSettings,
  updateSettings,
} from '@/features/sunapi/api'
import {
  adminAuthErrorMessage,
  updateAdminPassword,
} from '@/features/sunapi/auth-api'

const DEFAULT_PRICE_FIELDS = {
  default_group: 'default',
  default_input_price_per_1k: 0,
  default_output_price_per_1k: 0,
  currency_symbol: '$',
}

const startPageOptions = [
  { value: '/home', label: '首页' },
  { value: '/dashboard', label: '数据看板' },
  { value: '/channels', label: '渠道&分组' },
  { value: '/keys', label: 'API 密钥' },
  { value: '/playground', label: '创作台' },
  { value: '/docs', label: '文档' },
]

const optionalNavItems = [
  {
    key: 'show_dashboard',
    title: '数据看板',
    description: '隐藏后保留统计能力，只从导航中移除入口。',
    icon: BarChart3,
  },
  {
    key: 'show_api_keys',
    title: 'API 密钥',
    description: '只使用创作台时可以隐藏中转 Token 管理。',
    icon: KeyRound,
  },
  {
    key: 'show_usage_logs',
    title: '使用日志',
    description: '不需要排查调用明细时可以隐藏日志入口。',
    icon: FileText,
  },
  {
    key: 'show_playground',
    title: '创作台',
    description: '只把 SunAPI 当本地中转站时可以隐藏创作入口。',
    icon: Sparkles,
  },
] satisfies Array<{
  key: keyof Pick<
    AppSettings,
    'show_dashboard' | 'show_api_keys' | 'show_usage_logs' | 'show_playground'
  >
  title: string
  description: string
  icon: React.ElementType
}>

const lockedNavItems = [
  {
    title: '渠道&分组',
    description: '创建分组、维护渠道和同步上游模型的核心入口。',
    icon: Radio,
  },
  {
    title: '设置',
    description: '用于恢复导航和调整服务参数，始终保留。',
    icon: SettingsIcon,
  },
]

function settingsWithDefaults(settings: AppSettings): AppSettings {
  return {
    ...DEFAULT_PRICE_FIELDS,
    default_start_page: '/home',
    show_dashboard: true,
    show_api_keys: true,
    show_usage_logs: true,
    show_playground: true,
    ...settings,
  }
}

export function Settings() {
  const queryClient = useQueryClient()
  const setSystemConfig = useSystemConfigStore((state) => state.setConfig)
  const settingsQuery = useQuery({
    queryKey: ['sunapi-settings'],
    queryFn: getSettings,
  })
  const [form, setForm] = useState<AppSettings | null>(null)
  const [passwordForm, setPasswordForm] = useState({
    current: '',
    next: '',
    confirm: '',
  })
  const [clearLogsOpen, setClearLogsOpen] = useState(false)

  useEffect(() => {
    if (settingsQuery.data) {
      setForm(settingsWithDefaults(settingsQuery.data))
    }
  }, [settingsQuery.data])

  const visibleOptionalCount = useMemo(() => {
    if (!form) return 0
    return optionalNavItems.filter((item) => form[item.key]).length
  }, [form])

  const saveMutation = useMutation({
    mutationFn: () => updateSettings(form as AppSettings),
    onSuccess: (settings) => {
      const nextSettings = settingsWithDefaults(settings)
      setForm(nextSettings)
      setSystemConfig({
        systemName: nextSettings.system_name,
        defaultStartPage: nextSettings.default_start_page,
        showDashboard: nextSettings.show_dashboard,
        showApiKeys: nextSettings.show_api_keys,
        showUsageLogs: nextSettings.show_usage_logs,
        showPlayground: nextSettings.show_playground,
      })
      toast.success('设置已保存')
      queryClient.invalidateQueries({ queryKey: ['sunapi-dashboard'] })
      queryClient.invalidateQueries({ queryKey: ['status'] })
    },
  })

  const passwordMutation = useMutation({
    mutationFn: () =>
      updateAdminPassword({
        current_password: passwordForm.current,
        new_password: passwordForm.next,
      }),
    onSuccess: () => {
      setPasswordForm({ current: '', next: '', confirm: '' })
      toast.success('管理员密码已更新')
    },
    onError: (error) => {
      toast.error(adminAuthErrorMessage(error, '密码修改失败'))
    },
  })

  const clearLogsMutation = useMutation({
    mutationFn: clearUsageLogs,
    onSuccess: (result) => {
      setClearLogsOpen(false)
      toast.success(`已清空 ${result.deleted} 条使用日志`)
      queryClient.invalidateQueries({ queryKey: ['sunapi-dashboard'] })
      queryClient.invalidateQueries({ queryKey: ['logs'] })
      queryClient.invalidateQueries({ queryKey: ['usage-logs-stats'] })
    },
  })

  const updateForm = (patch: Partial<AppSettings>) => {
    setForm((prev) => (prev ? { ...prev, ...patch } : prev))
  }

  const handlePasswordSubmit = () => {
    if (!passwordForm.current.trim()) {
      toast.error('请输入当前密码')
      return
    }
    if (passwordForm.next.length < 8) {
      toast.error('新密码至少需要 8 位')
      return
    }
    if (passwordForm.next !== passwordForm.confirm) {
      toast.error('两次输入的新密码不一致')
      return
    }
    passwordMutation.mutate()
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>设置</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        管理本地服务、界面入口、管理员安全和数据维护。
      </SectionPageLayout.Description>
      <SectionPageLayout.Actions>
        <Button
          size='sm'
          disabled={!form || saveMutation.isPending}
          onClick={() => saveMutation.mutate()}
        >
          <Save className='size-4' />
          保存
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        {form && (
          <div className='grid gap-4 xl:grid-cols-2'>
            <Card className='rounded-lg'>
              <CardHeader>
                <CardTitle>服务</CardTitle>
                <CardDescription>
                  端口和监听地址需要重启 exe 后生效。
                </CardDescription>
              </CardHeader>
              <CardContent className='grid gap-4'>
                <div className='grid gap-2'>
                  <Label htmlFor='system-name'>产品名称</Label>
                  <Input
                    id='system-name'
                    value={form.system_name}
                    onChange={(event) =>
                      updateForm({ system_name: event.target.value })
                    }
                  />
                </div>
                <div className='grid gap-4 md:grid-cols-2'>
                  <div className='grid gap-2'>
                    <Label htmlFor='listen-host'>监听地址</Label>
                    <Input
                      id='listen-host'
                      value={form.listen_host}
                      onChange={(event) =>
                        updateForm({ listen_host: event.target.value })
                      }
                    />
                  </div>
                  <div className='grid gap-2'>
                    <Label htmlFor='listen-port'>端口</Label>
                    <Input
                      id='listen-port'
                      type='number'
                      min='1'
                      max='65535'
                      value={form.listen_port}
                      onChange={(event) =>
                        updateForm({ listen_port: Number(event.target.value) })
                      }
                    />
                  </div>
                </div>
                <div className='grid gap-2'>
                  <Label>默认打开页面</Label>
                  <Select
                    value={form.default_start_page}
                    onValueChange={(value) =>
                      updateForm({ default_start_page: value })
                    }
                  >
                    <SelectTrigger className='w-full'>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        {startPageOptions.map((option) => (
                          <SelectItem key={option.value} value={option.value}>
                            {option.label}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </div>
                <div className='flex items-center justify-between gap-4 rounded-lg border p-3'>
                  <div className='grid gap-1'>
                    <Label>启动后打开页面</Label>
                    <p className='text-muted-foreground text-xs'>
                      exe 启动完成后自动打开上面选择的页面。
                    </p>
                  </div>
                  <Switch
                    checked={form.auto_open_browser}
                    onCheckedChange={(auto_open_browser) =>
                      updateForm({ auto_open_browser })
                    }
                  />
                </div>
              </CardContent>
            </Card>

            <Card className='rounded-lg'>
              <CardHeader>
                <CardTitle>界面显示</CardTitle>
                <CardDescription>
                  根据使用模式隐藏不需要的入口，核心入口始终保留。
                </CardDescription>
              </CardHeader>
              <CardContent className='grid gap-3'>
                {optionalNavItems.map((item) => {
                  const Icon = item.icon
                  return (
                    <div
                      key={item.key}
                      className='flex items-center justify-between gap-4 rounded-lg border p-3'
                    >
                      <div className='flex min-w-0 items-start gap-3'>
                        <Icon className='text-muted-foreground mt-0.5 size-4 shrink-0' />
                        <div className='grid gap-1'>
                          <Label>{item.title}</Label>
                          <p className='text-muted-foreground text-xs'>
                            {item.description}
                          </p>
                        </div>
                      </div>
                      <Switch
                        checked={form[item.key]}
                        onCheckedChange={(value) =>
                          updateForm({ [item.key]: value })
                        }
                      />
                    </div>
                  )
                })}

                <div className='grid gap-3 pt-1'>
                  {lockedNavItems.map((item) => {
                    const Icon = item.icon
                    return (
                      <div
                        key={item.title}
                        className='bg-muted/35 flex items-center justify-between gap-4 rounded-lg border p-3'
                      >
                        <div className='flex min-w-0 items-start gap-3'>
                          <Icon className='text-muted-foreground mt-0.5 size-4 shrink-0' />
                          <div className='grid gap-1'>
                            <Label>{item.title}</Label>
                            <p className='text-muted-foreground text-xs'>
                              {item.description}
                            </p>
                          </div>
                        </div>
                        <div className='text-muted-foreground flex items-center gap-1.5 text-xs font-medium'>
                          <Lock className='size-3.5' />
                          固定
                        </div>
                      </div>
                    )
                  })}
                </div>

                <div className='text-muted-foreground flex items-center gap-2 pt-1 text-xs'>
                  <Check className='size-3.5' />
                  当前显示 {visibleOptionalCount + lockedNavItems.length} 个控制台入口
                </div>
              </CardContent>
            </Card>

            <Card className='rounded-lg'>
              <CardHeader>
                <CardTitle>管理员安全</CardTitle>
                <CardDescription>
                  当前系统只保留 admin 管理员账号。
                </CardDescription>
              </CardHeader>
              <CardContent className='grid gap-4'>
                <div className='grid gap-2'>
                  <Label htmlFor='current-password'>当前密码</Label>
                  <PasswordInput
                    id='current-password'
                    value={passwordForm.current}
                    autoComplete='current-password'
                    onChange={(event) =>
                      setPasswordForm((prev) => ({
                        ...prev,
                        current: event.target.value,
                      }))
                    }
                  />
                </div>
                <div className='grid gap-4 md:grid-cols-2'>
                  <div className='grid gap-2'>
                    <Label htmlFor='new-password'>新密码</Label>
                    <PasswordInput
                      id='new-password'
                      value={passwordForm.next}
                      autoComplete='new-password'
                      onChange={(event) =>
                        setPasswordForm((prev) => ({
                          ...prev,
                          next: event.target.value,
                        }))
                      }
                    />
                  </div>
                  <div className='grid gap-2'>
                    <Label htmlFor='confirm-password'>确认新密码</Label>
                    <PasswordInput
                      id='confirm-password'
                      value={passwordForm.confirm}
                      autoComplete='new-password'
                      onChange={(event) =>
                        setPasswordForm((prev) => ({
                          ...prev,
                          confirm: event.target.value,
                        }))
                      }
                    />
                  </div>
                </div>
                <div className='flex justify-end'>
                  <Button
                    type='button'
                    variant='outline'
                    disabled={passwordMutation.isPending}
                    onClick={handlePasswordSubmit}
                  >
                    <ShieldCheck className='size-4' />
                    修改密码
                  </Button>
                </div>
              </CardContent>
            </Card>

            <Card className='rounded-lg'>
              <CardHeader>
                <CardTitle>数据维护</CardTitle>
                <CardDescription>
                  清理本地统计数据不会删除渠道、分组和 API 密钥。
                </CardDescription>
              </CardHeader>
              <CardContent className='grid gap-3'>
                <div className='flex items-center justify-between gap-4 rounded-lg border p-3'>
                  <div className='flex min-w-0 items-start gap-3'>
                    <Trash2 className='text-muted-foreground mt-0.5 size-4 shrink-0' />
                    <div className='grid gap-1'>
                      <Label>清空使用日志</Label>
                      <p className='text-muted-foreground text-xs'>
                        删除调用明细和统计来源，适合重新开始统计。
                      </p>
                    </div>
                  </div>
                  <Button
                    type='button'
                    variant='destructive'
                    size='sm'
                    onClick={() => setClearLogsOpen(true)}
                  >
                    清空
                  </Button>
                </div>
                <div className='bg-muted/35 flex items-center gap-3 rounded-lg border p-3'>
                  <RotateCcw className='text-muted-foreground size-4 shrink-0' />
                  <p className='text-muted-foreground text-xs'>
                    配置导入、导出和更细的数据清理会放在后续维护项里。
                  </p>
                </div>
              </CardContent>
            </Card>
          </div>
        )}
      </SectionPageLayout.Content>

      <ConfirmDialog
        open={clearLogsOpen}
        onOpenChange={setClearLogsOpen}
        title='清空使用日志'
        desc='此操作会删除本地调用日志和统计数据，渠道、分组、API 密钥不会被删除。'
        destructive
        confirmText='清空'
        isLoading={clearLogsMutation.isPending}
        handleConfirm={() => clearLogsMutation.mutate()}
      />
    </SectionPageLayout>
  )
}
