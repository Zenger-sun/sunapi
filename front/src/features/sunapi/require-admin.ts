import { redirect } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { getAdminAuthStatus } from './auth-api'

export async function resolveAdminAuthPage(mode: 'login' | 'setup') {
  const status = await getAdminAuthStatus()
  if (status.authenticated && status.user) {
    useAuthStore.getState().auth.setUser(status.user)
    throw redirect({ to: '/dashboard' })
  }
  if (mode === 'login' && !status.initialized) {
    throw redirect({ to: '/setup' })
  }
  if (mode === 'setup' && status.initialized) {
    throw redirect({ to: '/login' })
  }
  return status
}

export async function requireAdmin() {
  const status = await getAdminAuthStatus()
  if (!status.initialized) {
    throw redirect({ to: '/setup' })
  }
  if (!status.authenticated || !status.user) {
    useAuthStore.getState().auth.reset()
    throw redirect({ to: '/login' })
  }
  useAuthStore.getState().auth.setUser(status.user)
  return status.user
}
