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
import { Link } from '@tanstack/react-router'
import {
  BookOpen,
  Cable,
  CheckCircle2,
  Copy,
  Route,
  Settings2,
} from 'lucide-react'
import { CopyButton } from '@/components/copy-button'
import { PublicLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { useStatus } from '@/hooks/use-status'

function getRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object'
    ? (value as Record<string, unknown>)
    : null
}

function getStatusString(status: unknown, key: string): string | undefined {
  const root = getRecord(status)
  const nested = getRecord(root?.data)
  const value = root?.[key] ?? nested?.[key]
  return typeof value === 'string' && value.trim() ? value.trim() : undefined
}

function getBrowserOrigin(): string {
  if (typeof window === 'undefined') return ''
  return window.location.origin
}

function normalizeServerAddress(address: string): string {
  return address.replace(/\/+$/, '')
}

function joinPath(base: string, pathname: string): string {
  return `${normalizeServerAddress(base)}${pathname}`
}

const setupSteps = [
  {
    title: '添加上游渠道',
    description:
      '进入控制台的渠道页面，填写上游 Base URL、API Key、模型名称和输入/输出单价。',
    href: '/channels',
  },
  {
    title: '配置分组',
    description:
      '默认分组为 default。需要区分不同用途时，可在分组页设置名称和价格倍率。',
    href: '/groups',
  },
  {
    title: '接入本机入口',
    description:
      '应用请求 SunAPI，本地服务会自动选择可用渠道并记录 Token、调用次数和成本。',
    href: '/dashboard',
  },
]

const clientRows = [
  {
    name: 'Claude / Anthropic 兼容客户端',
    baseURL: '本地入口',
    note: 'Base URL 不要额外添加 /v1。',
  },
  {
    name: 'OpenAI 兼容客户端',
    baseURL: '本地入口 + /v1',
    note: '例如 chat/completions、responses 等 OpenAI 风格接口。',
  },
]

export function Docs() {
  const { status } = useStatus()
  const endpoint = normalizeServerAddress(
    getStatusString(status, 'server_address') ?? getBrowserOrigin()
  )
  const openAIBaseURL = joinPath(endpoint, '/v1')

  return (
    <PublicLayout showMainContainer={false}>
      <main className='bg-background min-h-screen px-4 pt-24 pb-12 md:px-6'>
        <div className='mx-auto flex w-full max-w-6xl flex-col gap-8'>
          <section className='grid gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]'>
            <div className='flex flex-col gap-5'>
              <div className='border-border bg-card/70 inline-flex w-fit items-center gap-2 rounded-full border px-3 py-1 text-xs font-medium'>
                <BookOpen className='size-3.5' />
                SunAPI 本地配置教学
              </div>
              <div className='space-y-3'>
                <h1 className='max-w-3xl text-4xl leading-tight font-semibold tracking-normal md:text-5xl'>
                  把上游模型接成本机统一入口
                </h1>
                <p className='text-muted-foreground max-w-3xl text-base leading-7'>
                  这个单机版本只保留渠道、分组、调用日志和统计。添加一个上游渠道后，
                  本机入口会转发请求，并自动汇总 Token、调用次数和估算价格。
                </p>
              </div>
              <div className='flex flex-wrap gap-3'>
                <Button render={<Link to='/dashboard' />}>
                  前往控制台
                </Button>
                <Button variant='outline' render={<Link to='/playground' />}>
                  打开创作台
                </Button>
              </div>
            </div>

            <div className='bg-card rounded-2xl border p-4 shadow-xs'>
              <div className='flex items-center justify-between gap-3'>
                <div>
                  <div className='text-sm font-semibold'>本地入口</div>
                  <div className='text-muted-foreground text-xs'>
                    控制台展示和 Claude 客户端使用此地址
                  </div>
                </div>
                <CopyButton
                  value={endpoint}
                  variant='outline'
                  size='sm'
                  tooltip='复制入口'
                  successTooltip='已复制'
                >
                  <Copy className='size-3.5' />
                  复制
                </CopyButton>
              </div>
              <div className='bg-muted/40 mt-4 rounded-xl border px-3 py-3 font-mono text-sm font-semibold break-all'>
                {endpoint}
              </div>
              <div className='mt-4 grid gap-2 text-sm'>
                {clientRows.map((row) => (
                  <div
                    key={row.name}
                    className='border-border/70 flex items-start gap-2 rounded-lg border px-3 py-2'
                  >
                    <CheckCircle2 className='text-success mt-0.5 size-4 shrink-0' />
                    <div className='min-w-0'>
                      <div className='font-medium'>{row.name}</div>
                      <div className='text-muted-foreground text-xs'>
                        {row.baseURL}，{row.note}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </section>

          <section className='grid gap-4 md:grid-cols-3'>
            {setupSteps.map((step, index) => (
              <div key={step.title} className='bg-card rounded-2xl border p-4'>
                <div className='bg-muted flex size-8 items-center justify-center rounded-lg text-sm font-semibold'>
                  {index + 1}
                </div>
                <h2 className='mt-4 text-base font-semibold'>{step.title}</h2>
                <p className='text-muted-foreground mt-2 text-sm leading-6'>
                  {step.description}
                </p>
                <Button
                  variant='ghost'
                  size='sm'
                  className='mt-4 px-0'
                  render={<Link to={step.href} />}
                >
                  去配置
                </Button>
              </div>
            ))}
          </section>

          <section className='grid gap-4 lg:grid-cols-2'>
            <div className='bg-card rounded-2xl border p-5'>
              <div className='flex items-center gap-2 text-base font-semibold'>
                <Cable className='size-4' />
                OpenAI 兼容配置
              </div>
              <p className='text-muted-foreground mt-2 text-sm leading-6'>
                OpenAI SDK、Cherry Studio、Chatbox 等使用 OpenAI 协议的客户端，
                Base URL 填下面这个地址。
              </p>
              <div className='bg-muted/40 mt-4 rounded-xl border px-3 py-3 font-mono text-sm break-all'>
                {openAIBaseURL}
              </div>
            </div>

            <div className='bg-card rounded-2xl border p-5'>
              <div className='flex items-center gap-2 text-base font-semibold'>
                <Route className='size-4' />
                Claude 兼容配置
              </div>
              <p className='text-muted-foreground mt-2 text-sm leading-6'>
                Claude Code 或 Anthropic 兼容客户端的 Base URL 使用本地入口即可，
                客户端自身会请求对应的 Claude API 路径。
              </p>
              <div className='bg-muted/40 mt-4 rounded-xl border px-3 py-3 font-mono text-sm break-all'>
                {endpoint}
              </div>
            </div>
          </section>

          <section className='bg-card rounded-2xl border p-5'>
            <div className='flex items-center gap-2 text-base font-semibold'>
              <Settings2 className='size-4' />
              统计与价格
            </div>
            <div className='text-muted-foreground mt-4 grid gap-3 text-sm md:grid-cols-3'>
              <p>
                每次转发完成后会写入调用日志，记录模型、渠道、分组、Token、耗时和状态码。
              </p>
              <p>
                价格由渠道的输入/输出单价和分组倍率估算，只用于本地统计，不做计费。
              </p>
              <p>
                数据看板会按模型和时间聚合请求数、Token、RPM、TPM 与总消耗。
              </p>
            </div>
          </section>
        </div>
      </main>
    </PublicLayout>
  )
}
