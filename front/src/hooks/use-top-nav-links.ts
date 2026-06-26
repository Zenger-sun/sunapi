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
import { useTranslation } from 'react-i18next'
import { useSystemConfigStore } from '@/stores/system-config-store'

export type TopNavLink = {
  title: string
  href: string
  disabled?: boolean
  requiresAuth?: boolean
  external?: boolean
}

export function useTopNavLinks(): TopNavLink[] {
  const { t } = useTranslation()
  const config = useSystemConfigStore((state) => state.config)
  const showDashboard = config.showDashboard ?? true
  const showPlayground = config.showPlayground ?? true

  return [
    { title: t('Home'), href: '/home' },
    { title: t('Console'), href: showDashboard ? '/dashboard' : '/channels' },
    ...(showPlayground
      ? [{ title: t('Playground'), href: '/playground' }]
      : []),
    { title: t('Docs'), href: '/docs' },
  ]
}
