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
import { BarChart3, FileText, KeyRound, Radio, Settings } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { type SidebarData } from '@/components/layout/types'

export function useSidebarData(): SidebarData {
  const { t } = useTranslation()
  const config = useSystemConfigStore((state) => state.config)
  const showDashboard = config.showDashboard ?? true
  const showApiKeys = config.showApiKeys ?? true
  const showUsageLogs = config.showUsageLogs ?? true

  return {
    navGroups: [
      {
        id: 'general',
        title: t('General'),
        items: [
          ...(showDashboard
            ? [
                {
                  title: t('Dashboard'),
                  url: '/dashboard',
                  icon: BarChart3,
                },
              ]
            : []),
          ...(showApiKeys
            ? [
                {
                  title: t('API Keys'),
                  url: '/keys',
                  icon: KeyRound,
                },
              ]
            : []),
          {
            title: t('Channels & Groups'),
            url: '/channels',
            activeUrls: ['/channels', '/groups'],
            configUrls: ['/channels', '/groups'],
            icon: Radio,
          },
          ...(showUsageLogs
            ? [
                {
                  title: t('Usage Logs'),
                  url: '/usage-logs',
                  icon: FileText,
                },
              ]
            : []),
          {
            title: t('Settings'),
            url: '/settings',
            icon: Settings,
          },
        ],
      },
    ],
  }
}
