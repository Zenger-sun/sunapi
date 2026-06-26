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
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useSystemConfigStore } from '@/stores/system-config-store'
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
import { ConfirmDialog } from '@/components/confirm-dialog'
import { SectionPageLayout } from '@/components/layout'
import { PasswordInput } from '@/components/password-input'
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
  { value: '/home', labelKey: 'Home' },
  { value: '/dashboard', labelKey: 'Dashboard' },
  { value: '/channels', labelKey: 'Channels & Groups' },
  { value: '/keys', labelKey: 'API Keys' },
  { value: '/playground', labelKey: 'Playground' },
  { value: '/docs', labelKey: 'Docs' },
]

const optionalNavItems = [
  {
    key: 'show_dashboard',
    titleKey: 'Dashboard',
    descriptionKey:
      'Hide this entry from navigation while keeping analytics available.',
    icon: BarChart3,
  },
  {
    key: 'show_api_keys',
    titleKey: 'API Keys',
    descriptionKey:
      'Hide relay token management when you only use the playground.',
    icon: KeyRound,
  },
  {
    key: 'show_usage_logs',
    titleKey: 'Usage Logs',
    descriptionKey:
      'Hide the log entry when you do not need request-level diagnostics.',
    icon: FileText,
  },
  {
    key: 'show_playground',
    titleKey: 'Playground',
    descriptionKey:
      'Hide creative tools when SunAPI is only used as a local relay.',
    icon: Sparkles,
  },
] satisfies Array<{
  key: keyof Pick<
    AppSettings,
    'show_dashboard' | 'show_api_keys' | 'show_usage_logs' | 'show_playground'
  >
  titleKey: string
  descriptionKey: string
  icon: React.ElementType
}>

const lockedNavItems = [
  {
    titleKey: 'Channels & Groups',
    descriptionKey:
      'Core entry for creating groups, maintaining channels, and syncing upstream models.',
    icon: Radio,
  },
  {
    titleKey: 'Settings',
    descriptionKey:
      'Always available so you can restore navigation and adjust service settings.',
    icon: SettingsIcon,
  },
]

function settingsWithDefaults(settings: AppSettings): AppSettings {
  return {
    ...DEFAULT_PRICE_FIELDS,
    ...settings,
    default_start_page: settings.default_start_page || '/home',
    show_dashboard: settings.show_dashboard ?? true,
    show_api_keys: settings.show_api_keys ?? true,
    show_usage_logs: settings.show_usage_logs ?? true,
    show_playground: settings.show_playground ?? true,
  }
}

export function Settings() {
  const { t } = useTranslation()
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
      toast.success(t('Settings saved'))
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
      toast.success(t('Admin password updated'))
    },
    onError: (error) => {
      toast.error(
        adminAuthErrorMessage(error, t('Failed to update password'), t)
      )
    },
  })

  const clearLogsMutation = useMutation({
    mutationFn: clearUsageLogs,
    onSuccess: (result) => {
      setClearLogsOpen(false)
      toast.success(
        t('Cleared {{count}} usage log entries', { count: result.deleted })
      )
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
      toast.error(t('Please enter the current password'))
      return
    }
    if (passwordForm.next.length < 8) {
      toast.error(t('New password must be at least 8 characters'))
      return
    }
    if (passwordForm.next !== passwordForm.confirm) {
      toast.error(t('The new passwords do not match'))
      return
    }
    passwordMutation.mutate()
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Settings')}</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t(
          'Manage local service, visible entries, admin security, and data maintenance.'
        )}
      </SectionPageLayout.Description>
      <SectionPageLayout.Actions>
        <Button
          size='sm'
          disabled={!form || saveMutation.isPending}
          onClick={() => saveMutation.mutate()}
        >
          <Save className='size-4' />
          {t('Save')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        {form && (
          <div className='grid gap-4 xl:grid-cols-2'>
            <Card className='rounded-lg'>
              <CardHeader>
                <CardTitle>{t('Service')}</CardTitle>
                <CardDescription>
                  {t(
                    'Port and listen address changes take effect after restarting the exe.'
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent className='grid gap-4'>
                <div className='grid gap-2'>
                  <Label htmlFor='system-name'>{t('Product name')}</Label>
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
                    <Label htmlFor='listen-host'>{t('Listen address')}</Label>
                    <Input
                      id='listen-host'
                      value={form.listen_host}
                      onChange={(event) =>
                        updateForm({ listen_host: event.target.value })
                      }
                    />
                  </div>
                  <div className='grid gap-2'>
                    <Label htmlFor='listen-port'>{t('Port')}</Label>
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
                  <Label>{t('Default start page')}</Label>
                  <Select
                    value={form.default_start_page}
                    onValueChange={(value) =>
                      updateForm({ default_start_page: value ?? '/home' })
                    }
                  >
                    <SelectTrigger className='w-full'>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        {startPageOptions.map((option) => (
                          <SelectItem key={option.value} value={option.value}>
                            {t(option.labelKey)}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </div>
                <div className='flex items-center justify-between gap-4 rounded-lg border p-3'>
                  <div className='grid gap-1'>
                    <Label>{t('Open page after startup')}</Label>
                    <p className='text-muted-foreground text-xs'>
                      {t(
                        'After the exe starts, automatically open the page selected above.'
                      )}
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
                <CardTitle>{t('Interface Display')}</CardTitle>
                <CardDescription>
                  {t(
                    'Hide entries you do not need for your usage mode. Core entries always stay visible.'
                  )}
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
                          <Label>{t(item.titleKey)}</Label>
                          <p className='text-muted-foreground text-xs'>
                            {t(item.descriptionKey)}
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
                        key={item.titleKey}
                        className='bg-muted/35 flex items-center justify-between gap-4 rounded-lg border p-3'
                      >
                        <div className='flex min-w-0 items-start gap-3'>
                          <Icon className='text-muted-foreground mt-0.5 size-4 shrink-0' />
                          <div className='grid gap-1'>
                            <Label>{t(item.titleKey)}</Label>
                            <p className='text-muted-foreground text-xs'>
                              {t(item.descriptionKey)}
                            </p>
                          </div>
                        </div>
                        <div className='text-muted-foreground flex items-center gap-1.5 text-xs font-medium'>
                          <Lock className='size-3.5' />
                          {t('Fixed')}
                        </div>
                      </div>
                    )
                  })}
                </div>

                <div className='text-muted-foreground flex items-center gap-2 pt-1 text-xs'>
                  <Check className='size-3.5' />
                  {t('Currently showing {{count}} console entries', {
                    count: visibleOptionalCount + lockedNavItems.length,
                  })}
                </div>
              </CardContent>
            </Card>

            <Card className='rounded-lg'>
              <CardHeader>
                <CardTitle>{t('Admin Security')}</CardTitle>
                <CardDescription>
                  {t('This local system keeps only the admin account.')}
                </CardDescription>
              </CardHeader>
              <CardContent className='grid gap-4'>
                <div className='grid gap-2'>
                  <Label htmlFor='current-password'>
                    {t('Current password')}
                  </Label>
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
                    <Label htmlFor='new-password'>{t('New password')}</Label>
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
                    <Label htmlFor='confirm-password'>
                      {t('Confirm new password')}
                    </Label>
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
                    {t('Change password')}
                  </Button>
                </div>
              </CardContent>
            </Card>

            <Card className='rounded-lg'>
              <CardHeader>
                <CardTitle>{t('Data Maintenance')}</CardTitle>
                <CardDescription>
                  {t(
                    'Cleaning local analytics data will not delete channels, groups, or API keys.'
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent className='grid gap-3'>
                <div className='flex items-center justify-between gap-4 rounded-lg border p-3'>
                  <div className='flex min-w-0 items-start gap-3'>
                    <Trash2 className='text-muted-foreground mt-0.5 size-4 shrink-0' />
                    <div className='grid gap-1'>
                      <Label>{t('Clear usage logs')}</Label>
                      <p className='text-muted-foreground text-xs'>
                        {t(
                          'Delete request details and analytics source data when you want to restart statistics.'
                        )}
                      </p>
                    </div>
                  </div>
                  <Button
                    type='button'
                    variant='destructive'
                    size='sm'
                    onClick={() => setClearLogsOpen(true)}
                  >
                    {t('Clear')}
                  </Button>
                </div>
                <div className='bg-muted/35 flex items-center gap-3 rounded-lg border p-3'>
                  <RotateCcw className='text-muted-foreground size-4 shrink-0' />
                  <p className='text-muted-foreground text-xs'>
                    {t(
                      'Config import, export, and finer data cleanup will be added in future maintenance tools.'
                    )}
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
        title={t('Clear usage logs')}
        desc={t(
          'This will delete local request logs and analytics data. Channels, groups, and API keys will not be deleted.'
        )}
        destructive
        confirmText={t('Clear')}
        isLoading={clearLogsMutation.isPending}
        handleConfirm={() => clearLogsMutation.mutate()}
      />
    </SectionPageLayout>
  )
}
