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
import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Save } from 'lucide-react'
import { toast } from 'sonner'
import { SectionPageLayout } from '@/components/layout'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  type AppSettings,
  getSettings,
  updateSettings,
} from '@/features/sunapi/api'

export function Settings() {
  const queryClient = useQueryClient()
  const settingsQuery = useQuery({
    queryKey: ['sunapi-settings'],
    queryFn: getSettings,
  })
  const [form, setForm] = useState<AppSettings | null>(null)

  useEffect(() => {
    if (settingsQuery.data) setForm(settingsQuery.data)
  }, [settingsQuery.data])

  const saveMutation = useMutation({
    mutationFn: () => updateSettings(form as AppSettings),
    onSuccess: (settings) => {
      setForm(settings)
      toast.success('设置已保存')
      queryClient.invalidateQueries({ queryKey: ['sunapi-dashboard'] })
    },
  })

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>设置</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        管理本地服务监听、默认价格和启动行为。
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
                <CardDescription>修改端口后需要重启 exe 生效</CardDescription>
              </CardHeader>
              <CardContent className='grid gap-4'>
                <div className='grid gap-2'>
                  <Label>产品名称</Label>
                  <Input
                    value={form.system_name}
                    onChange={(event) =>
                      setForm((prev) =>
                        prev ? { ...prev, system_name: event.target.value } : prev
                      )
                    }
                  />
                </div>
                <div className='grid gap-4 md:grid-cols-2'>
                  <div className='grid gap-2'>
                    <Label>监听地址</Label>
                    <Input
                      value={form.listen_host}
                      onChange={(event) =>
                        setForm((prev) =>
                          prev ? { ...prev, listen_host: event.target.value } : prev
                        )
                      }
                    />
                  </div>
                  <div className='grid gap-2'>
                    <Label>端口</Label>
                    <Input
                      type='number'
                      min='1'
                      max='65535'
                      value={form.listen_port}
                      onChange={(event) =>
                        setForm((prev) =>
                          prev
                            ? {
                                ...prev,
                                listen_port: Number(event.target.value),
                              }
                            : prev
                        )
                      }
                    />
                  </div>
                </div>
                <div className='flex items-center justify-between rounded-lg border p-3'>
                  <div>
                    <Label>启动后打开页面</Label>
                    <p className='text-muted-foreground text-xs'>
                      exe 启动后自动打开本地控制台
                    </p>
                  </div>
                  <Switch
                    checked={form.auto_open_browser}
                    onCheckedChange={(auto_open_browser) =>
                      setForm((prev) =>
                        prev ? { ...prev, auto_open_browser } : prev
                      )
                    }
                  />
                </div>
              </CardContent>
            </Card>

            <Card className='rounded-lg'>
              <CardHeader>
                <CardTitle>价格</CardTitle>
                <CardDescription>
                  仅用于本地统计估算，不产生计费。
                </CardDescription>
              </CardHeader>
              <CardContent className='grid gap-4'>
                <div className='grid gap-2'>
                  <Label>默认分组</Label>
                  <Input
                    value={form.default_group}
                    onChange={(event) =>
                      setForm((prev) =>
                        prev ? { ...prev, default_group: event.target.value } : prev
                      )
                    }
                  />
                </div>
                <div className='grid gap-4 md:grid-cols-2'>
                  <div className='grid gap-2'>
                    <Label>默认输入价格 / 1K Token</Label>
                    <Input
                      type='number'
                      min='0'
                      step='0.0001'
                      value={form.default_input_price_per_1k}
                      onChange={(event) =>
                        setForm((prev) =>
                          prev
                            ? {
                                ...prev,
                                default_input_price_per_1k: Number(
                                  event.target.value
                                ),
                              }
                            : prev
                        )
                      }
                    />
                  </div>
                  <div className='grid gap-2'>
                    <Label>默认输出价格 / 1K Token</Label>
                    <Input
                      type='number'
                      min='0'
                      step='0.0001'
                      value={form.default_output_price_per_1k}
                      onChange={(event) =>
                        setForm((prev) =>
                          prev
                            ? {
                                ...prev,
                                default_output_price_per_1k: Number(
                                  event.target.value
                                ),
                              }
                            : prev
                        )
                      }
                    />
                  </div>
                </div>
                <div className='grid gap-2'>
                  <Label>货币符号</Label>
                  <Input
                    value={form.currency_symbol}
                    onChange={(event) =>
                      setForm((prev) =>
                        prev ? { ...prev, currency_symbol: event.target.value } : prev
                      )
                    }
                  />
                </div>
              </CardContent>
            </Card>
          </div>
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
