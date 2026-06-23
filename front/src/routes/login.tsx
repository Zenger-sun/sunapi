import { createFileRoute } from '@tanstack/react-router'
import { AdminAuthPage } from '@/features/sunapi/admin-auth-page'
import { resolveAdminAuthPage } from '@/features/sunapi/require-admin'

export const Route = createFileRoute('/login')({
  beforeLoad: () => resolveAdminAuthPage('login'),
  component: () => <AdminAuthPage mode='login' />,
})
