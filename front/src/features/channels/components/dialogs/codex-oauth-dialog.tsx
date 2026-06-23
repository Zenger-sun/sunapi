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
import {
  Check,
  Copy,
  ExternalLink,
  KeyRound,
  Link2,
  Loader2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { tryPrettyJson } from '@/lib/utils'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Textarea } from '@/components/ui/textarea'
import {
  completeCodexOAuth,
  importCodexRefreshToken,
  startCodexOAuth,
} from '../../api'

type CodexOAuthDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onKeyGenerated: (key: string, count: number) => void
  proxy?: string
  batchMode?: boolean
}

type AuthorizationMethod =
  | 'manual'
  | 'refresh-token'
  | 'mobile-refresh-token'
  | 'json-at'

type CodexCredential = {
  access_token?: string
  refresh_token?: string
  account_id?: string
  chatgpt_account_id?: string
  chatgpt_user_id?: string
  client_id?: string
  id_token?: string
  email?: string
  type?: string
  expired?: string
  expires_at?: string | number
  last_refresh?: string
}

type CodexCredentialContainer = {
  account?: {
    credentials?: CodexCredential
    name?: string
  }
  credentials?: CodexCredential
  oauth?: CodexCredential
}

function splitInputLines(value: string): string[] {
  return value
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
}

function splitCredentialInputs(
  value: string,
  method: AuthorizationMethod
): string[] {
  const trimmed = value.trim()
  if (!trimmed) return []

  if (method === 'refresh-token' || method === 'mobile-refresh-token') {
    return splitInputLines(value)
  }

  if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
    try {
      const parsed = JSON.parse(trimmed) as unknown
      if (Array.isArray(parsed)) {
        return parsed.map((item) =>
          typeof item === 'string' ? item.trim() : JSON.stringify(item)
        )
      }
      return [trimmed]
    } catch {
      // Fall back to line mode for newline-delimited credentials.
    }
  }

  return splitInputLines(value)
}

function parseJwtPayload(token: string): Record<string, unknown> | null {
  const parts = token.trim().split('.')
  if (parts.length !== 3) return null
  try {
    const normalized = parts[1].replace(/-/g, '+').replace(/_/g, '/')
    const padded = normalized.padEnd(
      normalized.length + ((4 - (normalized.length % 4)) % 4),
      '='
    )
    const binary = atob(padded)
    const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0))
    return JSON.parse(new TextDecoder().decode(bytes)) as Record<
      string,
      unknown
    >
  } catch {
    return null
  }
}

function getAccountIdFromAccessToken(accessToken: string): string {
  const claims = parseJwtPayload(accessToken)
  const authClaim = claims?.['https://api.openai.com/auth']
  if (
    authClaim &&
    typeof authClaim === 'object' &&
    'chatgpt_account_id' in authClaim
  ) {
    const value = (authClaim as Record<string, unknown>).chatgpt_account_id
    return typeof value === 'string' ? value.trim() : ''
  }
  return ''
}

function getEmailFromAccessToken(accessToken: string): string {
  const claims = parseJwtPayload(accessToken)
  const value = claims?.email
  return typeof value === 'string' ? value.trim() : ''
}

function getStringField(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function getCodexCredentialSource(parsed: unknown): CodexCredential | null {
  if (!parsed || typeof parsed !== 'object') return null

  const container = parsed as CodexCredentialContainer & CodexCredential
  const nested =
    container.account?.credentials ||
    container.credentials ||
    container.oauth ||
    null

  if (nested && typeof nested === 'object') {
    return nested
  }
  return container
}

function normalizeExpiresAt(value: unknown): string {
  if (typeof value === 'number' && Number.isFinite(value) && value > 0) {
    return new Date(value * 1000).toISOString()
  }
  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (!trimmed) return ''
    if (/^\d+$/.test(trimmed)) {
      const timestamp = Number(trimmed)
      if (Number.isFinite(timestamp) && timestamp > 0) {
        return new Date(timestamp * 1000).toISOString()
      }
    }
    return trimmed
  }
  return ''
}

function normalizeCodexCredentialSource(source: CodexCredential): string {
  const accessToken = getStringField(source.access_token)
  const refreshToken = getStringField(source.refresh_token)
  const idToken = getStringField(source.id_token)
  const accountId =
    getStringField(source.account_id) ||
    getStringField(source.chatgpt_account_id) ||
    getAccountIdFromAccessToken(accessToken)

  if (!accessToken || !accountId) {
    throw new Error('Codex JSON must include access_token and account_id')
  }

  const credential: CodexCredential = {
    access_token: accessToken,
    refresh_token: refreshToken,
    account_id: accountId,
    id_token: idToken,
    email: getStringField(source.email) || getEmailFromAccessToken(accessToken),
    type: getStringField(source.type) || 'codex',
    expired: normalizeExpiresAt(source.expired || source.expires_at),
    last_refresh: getStringField(source.last_refresh),
  }

  Object.keys(credential).forEach((key) => {
    if (!credential[key as keyof CodexCredential]) {
      delete credential[key as keyof CodexCredential]
    }
  })

  return JSON.stringify(credential)
}

function extractRefreshTokenInput(raw: string): string {
  const value = raw.trim()
  if (!value.startsWith('{')) return value

  const parsed = JSON.parse(value) as CodexCredentialContainer & CodexCredential
  const source = getCodexCredentialSource(parsed)
  const refreshToken = getStringField(source?.refresh_token)
  if (!refreshToken) {
    throw new Error('Codex JSON must include refresh_token')
  }
  return refreshToken
}

function normalizeCredential(raw: string): string {
  const value = raw.trim()
  if (!value) {
    throw new Error('Empty credential')
  }

  if (value.startsWith('{')) {
    const parsed = JSON.parse(value) as CodexCredentialContainer &
      CodexCredential
    const source = getCodexCredentialSource(parsed)
    if (!source) {
      throw new Error('Credential must be a JSON object')
    }
    return normalizeCodexCredentialSource(source)
  }

  const accountId = getAccountIdFromAccessToken(value)
  if (!accountId) {
    throw new Error('Access Token must include account_id in JWT claims')
  }
  return JSON.stringify({
    access_token: value,
    account_id: accountId,
    email: getEmailFromAccessToken(value),
    type: 'codex',
  } satisfies CodexCredential)
}

export function CodexOAuthDialog({
  open,
  onOpenChange,
  onKeyGenerated,
  proxy = '',
  batchMode = false,
}: CodexOAuthDialogProps) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })

  const [method, setMethod] = useState<AuthorizationMethod>('manual')
  const [credentialInput, setCredentialInput] = useState('')
  const [state, setState] = useState({
    authorizeUrl: '',
    callbackUrl: '',
    isStarting: false,
    isCompleting: false,
    isImporting: false,
  })

  useEffect(() => {
    if (!open) {
      setMethod('manual')
      setCredentialInput('')
      setState({
        authorizeUrl: '',
        callbackUrl: '',
        isStarting: false,
        isCompleting: false,
        isImporting: false,
      })
    }
  }, [open])

  const canCopyAuthorizeUrl = Boolean(state.authorizeUrl && !state.isStarting)
  const canComplete = useMemo(
    () => Boolean(state.callbackUrl.trim()) && !state.isCompleting,
    [state.callbackUrl, state.isCompleting]
  )

  const handleStart = async () => {
    setState((prev) => ({ ...prev, isStarting: true }))
    try {
      const res = await startCodexOAuth()
      if (!res.success) {
        throw new Error(res.message || 'Failed to start OAuth')
      }

      const url = res.data?.authorize_url || ''
      if (!url) {
        throw new Error('Missing authorize_url in response')
      }

      setState((prev) => ({ ...prev, authorizeUrl: url }))
      try {
        window.open(url, '_blank', 'noopener,noreferrer')
        toast.success(t('Opened authorization page'))
      } catch (error) {
        // eslint-disable-next-line no-console
        console.warn('Failed to open authorization page:', error)
        toast.warning(t('Please manually copy and open the authorization link'))
      }
    } catch (error) {
      toast.error(
        error instanceof Error ? t(error.message) : t('OAuth start failed')
      )
    } finally {
      setState((prev) => ({ ...prev, isStarting: false }))
    }
  }

  const handleComplete = async () => {
    if (!state.callbackUrl.trim()) return
    setState((prev) => ({ ...prev, isCompleting: true }))
    try {
      const res = await completeCodexOAuth(state.callbackUrl.trim(), proxy)
      if (!res.success) {
        throw new Error(res.message || 'OAuth failed')
      }

      const rawKey = res.data?.key || ''
      if (!rawKey) {
        throw new Error('Missing key in response')
      }

      onKeyGenerated(tryPrettyJson(rawKey), 1)
      toast.success(t('Credential generated'))
      onOpenChange(false)
    } catch (error) {
      toast.error(error instanceof Error ? t(error.message) : t('OAuth failed'))
    } finally {
      setState((prev) => ({ ...prev, isCompleting: false }))
    }
  }

  const handleImportCredentials = async () => {
    const entries = splitCredentialInputs(credentialInput, method)
    if (!entries.length) return

    setState((prev) => ({ ...prev, isImporting: true }))
    try {
      const keys: string[] = []
      if (method === 'refresh-token' || method === 'mobile-refresh-token') {
        for (const entry of entries) {
          const refreshToken = extractRefreshTokenInput(entry)
          const res = await importCodexRefreshToken(refreshToken, proxy)
          if (!res.success || !res.data?.key) {
            throw new Error(res.message || 'Refresh Token verification failed')
          }
          keys.push(res.data.key)
        }
      } else {
        keys.push(...entries.map((entry) => normalizeCredential(entry)))
      }

      onKeyGenerated(
        keys.length === 1 ? tryPrettyJson(keys[0]) : keys.join('\n'),
        keys.length
      )
      toast.success(
        t('Generated {{count}} credential(s)', { count: keys.length })
      )
      onOpenChange(false)
    } catch (error) {
      toast.error(
        error instanceof Error
          ? t(error.message)
          : t('Credential import failed')
      )
    } finally {
      setState((prev) => ({ ...prev, isImporting: false }))
    }
  }

  const credentialLabel =
    method === 'json-at'
      ? t('Codex JSON / Access Token')
      : method === 'mobile-refresh-token'
        ? t('Mobile Refresh Token')
        : t('Refresh Token')

  const credentialPlaceholder =
    method === 'json-at'
      ? t(
          'Paste Codex JSON credential, JSON array, or Access Token. JSON can be formatted across multiple lines.'
        )
      : method === 'mobile-refresh-token'
        ? t('Paste OpenAI Mobile Refresh Token, one per line')
        : t('Paste OpenAI Refresh Token, one per line')

  const showImportPanel = method !== 'manual'

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[calc(100dvh-2rem)] overflow-hidden sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>{t('OpenAI Account Authorization')}</DialogTitle>
          <DialogDescription>
            {t('Generate Codex OAuth credentials for the channel key field.')}
          </DialogDescription>
        </DialogHeader>

        <div className='min-h-0 min-w-0 space-y-4 overflow-y-auto pr-1'>
          <div className='border-border/60 min-w-0 rounded-md border p-4'>
            <div className='mb-3 flex items-center gap-3'>
              <span className='bg-primary/10 text-primary flex size-9 shrink-0 items-center justify-center rounded-md'>
                <Link2 className='h-4 w-4' />
              </span>
              <div className='min-w-0'>
                <div className='text-sm font-semibold'>
                  {t('Authorization Method')}
                </div>
                <div className='text-muted-foreground text-xs'>
                  {batchMode
                    ? method === 'json-at'
                      ? t(
                          'Batch input supports a JSON array or one credential per line.'
                        )
                      : t('Batch input is supported, one credential per line.')
                    : t('Choose how to generate the Codex channel credential.')}
                </div>
              </div>
            </div>

            <RadioGroup
              value={method}
              onValueChange={(value) => setMethod(value as AuthorizationMethod)}
              className='grid gap-2 sm:grid-cols-2'
            >
              {[
                ['manual', t('Manual authorization')],
                ['refresh-token', t('Manual RT')],
                ['mobile-refresh-token', t('Mobile RT')],
                ['json-at', t('Codex JSON / AT')],
              ].map(([value, label]) => (
                <Label
                  key={value}
                  className='border-border/60 hover:bg-muted/50 flex cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-sm'
                >
                  <RadioGroupItem value={value} />
                  <span>{label}</span>
                </Label>
              ))}
            </RadioGroup>
          </div>

          {method === 'manual' && (
            <div className='space-y-4'>
              <Alert>
                <AlertDescription>
                  {t(
                    '1) Open the authorization page and complete login. 2) Your browser may redirect to localhost. 3) Copy the full URL from the address bar and paste it below. 4) Generate the credential.'
                  )}
                </AlertDescription>
              </Alert>

              <div className='flex flex-wrap gap-2'>
                <Button onClick={handleStart} disabled={state.isStarting}>
                  {state.isStarting ? (
                    <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                  ) : (
                    <ExternalLink className='mr-2 h-4 w-4' />
                  )}
                  {t('Open authorization page')}
                </Button>

                <Button
                  type='button'
                  variant='outline'
                  disabled={!canCopyAuthorizeUrl}
                  onClick={async () => {
                    if (!state.authorizeUrl) return
                    await copyToClipboard(state.authorizeUrl)
                  }}
                  aria-label={t('Copy authorization link')}
                  title={t('Copy authorization link')}
                >
                  {copiedText === state.authorizeUrl ? (
                    <Check className='mr-2 h-4 w-4 text-green-600' />
                  ) : (
                    <Copy className='mr-2 h-4 w-4' />
                  )}
                  {t('Copy authorization link')}
                </Button>
              </div>

              <div className='space-y-2'>
                <div className='text-sm font-medium'>{t('Callback URL')}</div>
                <Input
                  value={state.callbackUrl}
                  onChange={(e) =>
                    setState((prev) => ({
                      ...prev,
                      callbackUrl: e.target.value,
                    }))
                  }
                  placeholder={t(
                    'Paste the full callback URL (includes code & state)'
                  )}
                  autoComplete='off'
                  spellCheck={false}
                />
                <div className='text-muted-foreground text-xs'>
                  {t(
                    'Tip: The generated key is a JSON credential including access_token / refresh_token / account_id.'
                  )}
                </div>
              </div>
            </div>
          )}

          {showImportPanel && (
            <div className='border-border/60 min-w-0 rounded-md border p-4'>
              <div className='mb-2 flex items-center gap-2 text-sm font-medium'>
                <KeyRound className='text-muted-foreground h-4 w-4' />
                {credentialLabel}
              </div>
              <Textarea
                value={credentialInput}
                onChange={(event) => setCredentialInput(event.target.value)}
                placeholder={credentialPlaceholder}
                rows={7}
                wrap='off'
                spellCheck={false}
                className='field-sizing-fixed max-h-56 min-h-36 resize-y overflow-auto font-mono text-xs whitespace-pre'
              />
              <div className='text-muted-foreground mt-2 text-xs'>
                {method === 'json-at'
                  ? t(
                      'Codex JSON can include access_token / refresh_token / account_id. Access Token input must be a JWT containing account_id. Refresh Token input remains one token per line.'
                    )
                  : t(
                      'Refresh Token input is verified server-side and converted to Codex JSON.'
                    )}
              </div>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={state.isStarting || state.isCompleting}
          >
            {t('Cancel')}
          </Button>
          {method === 'manual' ? (
            <Button onClick={handleComplete} disabled={!canComplete}>
              {state.isCompleting && (
                <Loader2 className='mr-2 h-4 w-4 animate-spin' />
              )}
              {state.isCompleting
                ? t('Generating...')
                : t('Generate credential')}
            </Button>
          ) : (
            <Button
              onClick={handleImportCredentials}
              disabled={
                state.isImporting ||
                splitCredentialInputs(credentialInput, method).length === 0
              }
            >
              {state.isImporting && (
                <Loader2 className='mr-2 h-4 w-4 animate-spin' />
              )}
              {state.isImporting
                ? t('Verifying...')
                : t('Verify and create account')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
