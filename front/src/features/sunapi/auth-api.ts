import { api } from '@/lib/api'
import type { AuthUser } from '@/stores/auth-store'

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

function unwrapUser(data: unknown): AuthUser {
  const payload = data as { data?: AuthUser }
  return payload.data as AuthUser
}

export function adminAuthErrorMessage(error: unknown, fallback: string) {
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
      return '首次使用前需要先设置管理员密码'
    case 'admin login required':
      return '请先登录'
    case 'invalid username or password':
      return '密码不正确'
    case 'invalid current password':
      return '当前密码不正确'
    case 'username and password are required':
      return '请输入管理员密码'
    case 'username and password are required; password must be at least 8 characters':
      return '请设置至少 8 位管理员密码'
    case 'current password is required; new password must be at least 8 characters':
      return '请输入当前密码，并设置至少 8 位新密码'
    case 'admin user already initialized':
      return '管理员已初始化，请直接登录'
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
  return unwrapUser(res.data)
}

export async function loginAdmin(credentials: AdminCredentials) {
  const res = await api.post('/api/auth/login', credentials, {
    skipErrorHandler: true,
  })
  return unwrapUser(res.data)
}

export async function logoutAdmin() {
  const res = await api.post('/api/auth/logout', {})
  return res.data
}

export async function updateAdminPassword(payload: AdminPasswordPayload) {
  const res = await api.post('/api/auth/password', payload, {
    skipErrorHandler: true,
  })
  return unwrapUser(res.data)
}
