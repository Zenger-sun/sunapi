import { createFileRoute } from '@tanstack/react-router'
import { AdminAuthPage } from '@/features/sunapi/admin-auth-page'
import { resolveAdminAuthPage } from '@/features/sunapi/require-admin'

export const Route = createFileRoute('/setup')({
  beforeLoad: () => resolveAdminAuthPage('setup'),
  component: () => <AdminAuthPage mode='setup' />,
})
