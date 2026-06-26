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
import { useTranslation } from 'react-i18next'
import { useStatus } from '@/hooks/use-status'
import { Button } from '@/components/ui/button'
import { CopyButton } from '@/components/copy-button'
import { PublicLayout } from '@/components/layout'

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

export function Docs() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const endpoint = normalizeServerAddress(
    getStatusString(status, 'server_address') ?? getBrowserOrigin()
  )
  const openAIBaseURL = joinPath(endpoint, '/v1')
  const setupSteps = [
    {
      title: t('Create groups'),
      description: t(
        'Create usable groups first. Channels must be assigned to groups, and groups are also used for later price multiplier statistics.'
      ),
      href: '/groups',
      action: t('Create group'),
    },
    {
      title: t('Add and test channels'),
      description: t(
        'Open Channels, fill in the upstream Base URL, API key, model names, and prices, choose a group, then save and test. After the test passes, you can use it in Playground.'
      ),
      href: '/channels',
      action: t('Create channel'),
    },
    {
      title: t('Create API keys'),
      description: t(
        'When using SunAPI as a relay, create a token on the API Keys page first, then use one-click Codex or one-click Claude to connect your client.'
      ),
      href: '/keys',
      action: t('Create token'),
    },
  ]
  const clientRows = [
    {
      name: t('Claude / Anthropic compatible clients'),
      baseURL: t('Local endpoint'),
      note: t('Do not append /v1 to the Base URL.'),
    },
    {
      name: t('OpenAI compatible clients'),
      baseURL: t('Local endpoint + /v1'),
      note: t('For OpenAI-style APIs such as chat/completions and responses.'),
    },
  ]

  return (
    <PublicLayout showMainContainer={false}>
      <main className='bg-background min-h-screen px-4 pt-24 pb-12 md:px-6'>
        <div className='mx-auto flex w-full max-w-6xl flex-col gap-8'>
          <section className='grid gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]'>
            <div className='flex flex-col gap-5'>
              <div className='border-border bg-card/70 inline-flex w-fit items-center gap-2 rounded-full border px-3 py-1 text-xs font-medium'>
                <BookOpen className='size-3.5' />
                {t('SunAPI local setup guide')}
              </div>
              <div className='space-y-3'>
                <h1 className='max-w-3xl text-4xl leading-tight font-semibold tracking-normal md:text-5xl'>
                  {t('Connect upstream models to one local endpoint')}
                </h1>
                <p className='text-muted-foreground max-w-3xl text-base leading-7'>
                  {t(
                    'This local edition keeps channels, groups, request logs, and analytics. After you add an upstream channel, the local endpoint forwards requests and aggregates tokens, request counts, and estimated cost.'
                  )}
                </p>
              </div>
              <div className='flex flex-wrap gap-3'>
                <Button render={<Link to='/dashboard' />}>
                  {t('Go to Console')}
                </Button>
                <Button variant='outline' render={<Link to='/playground' />}>
                  {t('Open Playground')}
                </Button>
              </div>
            </div>

            <div className='bg-card rounded-2xl border p-4 shadow-xs'>
              <div className='flex items-center justify-between gap-3'>
                <div>
                  <div className='text-sm font-semibold'>
                    {t('Local endpoint')}
                  </div>
                  <div className='text-muted-foreground text-xs'>
                    {t('Use this address in the console and Claude clients')}
                  </div>
                </div>
                <CopyButton
                  value={endpoint}
                  variant='outline'
                  size='sm'
                  tooltip={t('Copy endpoint')}
                  successTooltip={t('Copied')}
                >
                  <Copy className='size-3.5' />
                  {t('Copy')}
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
                        {row.baseURL}, {row.note}
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
                  {step.action}
                </Button>
              </div>
            ))}
          </section>

          <section className='grid gap-4 lg:grid-cols-2'>
            <div className='bg-card rounded-2xl border p-5'>
              <div className='flex items-center gap-2 text-base font-semibold'>
                <Cable className='size-4' />
                {t('OpenAI compatible configuration')}
              </div>
              <p className='text-muted-foreground mt-2 text-sm leading-6'>
                {t(
                  'For clients that use the OpenAI protocol, such as the OpenAI SDK, Cherry Studio, and Chatbox, use the Base URL below.'
                )}
              </p>
              <div className='bg-muted/40 mt-4 rounded-xl border px-3 py-3 font-mono text-sm break-all'>
                {openAIBaseURL}
              </div>
            </div>

            <div className='bg-card rounded-2xl border p-5'>
              <div className='flex items-center gap-2 text-base font-semibold'>
                <Route className='size-4' />
                {t('Claude compatible configuration')}
              </div>
              <p className='text-muted-foreground mt-2 text-sm leading-6'>
                {t(
                  'For Claude Code or Anthropic-compatible clients, use the local endpoint as the Base URL. The client will request the matching Claude API path itself.'
                )}
              </p>
              <div className='bg-muted/40 mt-4 rounded-xl border px-3 py-3 font-mono text-sm break-all'>
                {endpoint}
              </div>
            </div>
          </section>

          <section className='bg-card rounded-2xl border p-5'>
            <div className='flex items-center gap-2 text-base font-semibold'>
              <Settings2 className='size-4' />
              {t('Analytics and pricing')}
            </div>
            <div className='text-muted-foreground mt-4 grid gap-3 text-sm md:grid-cols-3'>
              <p>
                {t(
                  'After each forwarded request completes, SunAPI writes a request log with model, channel, group, tokens, latency, and status code.'
                )}
              </p>
              <p>
                {t(
                  'Cost is estimated from channel input/output prices and group multipliers. It is for local analytics only, not billing.'
                )}
              </p>
              <p>
                {t(
                  'The dashboard aggregates requests, tokens, RPM, TPM, and total cost by model and time.'
                )}
              </p>
            </div>
          </section>
        </div>
      </main>
    </PublicLayout>
  )
}
