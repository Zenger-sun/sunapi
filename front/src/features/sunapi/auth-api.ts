import { api } from '@/lib/api'
import type { AuthUser } from '@/stores/auth-store'
import {
  clearAdminSessionToken,
  saveAdminSessionToken,
} from './admin-session-token'

export interface AdminAuthStatus {
  initialized: boolean
  authenticated: boolean
  user: AuthUser | null
}

export interface AdminCredentials {
  username: string
  password: string
}

export interface AdminPasswordPayload {
  current_password: string
  new_password: string
}

interface AdminAuthPayload {
  user: AuthUser
  token?: string
}

function unwrapAuthStatus(data: unknown): AdminAuthStatus {
  const payload = data as {
    success?: boolean
    data?: Partial<AdminAuthStatus>
  }
  return {
    initialized: !!payload.data?.initialized,
    authenticated: !!payload.data?.authenticated,
    user: payload.data?.user ?? null,
  }
}

function unwrapAuthPayload(data: unknown): AuthUser {
  const payload = data as { data?: AuthUser | AdminAuthPayload }
  const authPayload = payload.data as AdminAuthPayload | undefined
  if (authPayload?.user) {
    saveAdminSessionToken(authPayload.token)
    return authPayload.user
  }
  return payload.data as AuthUser
}

export function adminAuthErrorMessage(
  error: unknown,
  fallback: string,
  translate: (key: string) => string = (key) => key
) {
  const err = error as {
    response?: { status?: number; data?: { message?: unknown; error?: unknown } }
    message?: unknown
  }
  const raw =
    typeof err.response?.data?.message === 'string'
      ? err.response.data.message
      : typeof err.response?.data?.error === 'string'
        ? err.response.data.error
        : typeof err.message === 'string'
          ? err.message
          : ''
  const message = raw.trim()

  switch (message) {
    case 'admin setup required':
      return translate('Set the admin password before first use')
    case 'admin login required':
      return translate('Please sign in first')
    case 'invalid username or password':
      return translate('Incorrect password')
    case 'invalid current password':
      return translate('Current password is incorrect')
    case 'username and password are required':
      return translate('Please enter the admin password')
    case 'username and password are required; password must be at least 8 characters':
      return translate('Please set an admin password with at least 8 characters')
    case 'current password is required; new password must be at least 8 characters':
      return translate(
        'Please enter the current password and set a new password with at least 8 characters'
      )
    case 'admin user already initialized':
      return translate('Admin is already initialized. Please sign in directly')
    default:
      return message || fallback
  }
}

export async function getAdminAuthStatus(): Promise<AdminAuthStatus> {
  const res = await api.get('/api/auth/status', {
    skipErrorHandler: true,
    disableDuplicate: true,
  })
  return unwrapAuthStatus(res.data)
}

export async function setupAdmin(credentials: AdminCredentials) {
  const res = await api.post('/api/auth/setup', credentials, {
    skipErrorHandler: true,
  })
  return unwrapAuthPayload(res.data)
}

export async function loginAdmin(credentials: AdminCredentials) {
  const res = await api.post('/api/auth/login', credentials, {
    skipErrorHandler: true,
  })
  return unwrapAuthPayload(res.data)
}

export async function logoutAdmin() {
  const res = await api.post('/api/auth/logout', {})
  clearAdminSessionToken()
  return res.data
}

export async function updateAdminPassword(payload: AdminPasswordPayload) {
  const res = await api.post('/api/auth/password', payload, {
    skipErrorHandler: true,
  })
  return unwrapAuthPayload(res.data)
}
