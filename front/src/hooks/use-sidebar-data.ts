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
import {
  BarChart3,
  FileText,
  KeyRound,
  Radio,
  Settings,
} from 'lucide-react'
import { type SidebarData } from '@/components/layout/types'
import { useSystemConfigStore } from '@/stores/system-config-store'

export function useSidebarData(): SidebarData {
  const config = useSystemConfigStore((state) => state.config)
  const showDashboard = config.showDashboard ?? true
  const showApiKeys = config.showApiKeys ?? true
  const showUsageLogs = config.showUsageLogs ?? true

  return {
    navGroups: [
      {
        id: 'general',
        title: '常规',
        items: [
          ...(showDashboard
            ? [
                {
                  title: '数据看板',
                  url: '/dashboard',
                  icon: BarChart3,
                },
              ]
            : []),
          ...(showApiKeys
            ? [
                {
                  title: 'API 密钥',
                  url: '/keys',
                  icon: KeyRound,
                },
              ]
            : []),
          {
            title: '渠道&分组',
            url: '/channels',
            activeUrls: ['/channels', '/groups'],
            configUrls: ['/channels', '/groups'],
            icon: Radio,
          },
          ...(showUsageLogs
            ? [
                {
                  title: '使用日志',
                  url: '/usage-logs',
                  icon: FileText,
                },
              ]
            : []),
          {
            title: '设置',
            url: '/settings',
            icon: Settings,
          },
        ],
      },
    ],
  }
}
