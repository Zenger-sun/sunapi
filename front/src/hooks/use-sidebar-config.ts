import { useMemo } from 'react'
import { useAuthStore } from '@/stores/auth-store'
import { useStatus } from '@/hooks/use-status'
import type { NavGroup, NavItem } from '@/components/layout/types'

type SidebarSectionConfig = {
  enabled: boolean
  [key: string]: boolean
}

type SidebarModulesAdminConfig = Record<string, SidebarSectionConfig>
type SidebarModulesUserConfig = SidebarModulesAdminConfig | null

const DEFAULT_SIDEBAR_MODULES: SidebarModulesAdminConfig = {
  console: {
    enabled: true,
    detail: true,
    token: true,
    log: true,
  },
  admin: {
    enabled: true,
    channel: true,
    group: true,
    setting: true,
  },
}

const URL_TO_CONFIG_MAP: Record<string, { section: string; module: string }> = {
  '/dashboard': { section: 'console', module: 'detail' },
  '/dashboard/models': { section: 'console', module: 'detail' },
  '/keys': { section: 'console', module: 'token' },
  '/usage-logs': { section: 'console', module: 'log' },
  '/usage-logs/common': { section: 'console', module: 'log' },
  '/usage-logs/drawing': { section: 'console', module: 'log' },
  '/usage-logs/task': { section: 'console', module: 'log' },
  '/channels': { section: 'admin', module: 'channel' },
  '/groups': { section: 'admin', module: 'group' },
  '/settings': { section: 'admin', module: 'setting' },
}

function mergeWithDefaultSidebarModules(
  config: SidebarModulesAdminConfig
): SidebarModulesAdminConfig {
  const merged: SidebarModulesAdminConfig = { ...config }

  Object.entries(DEFAULT_SIDEBAR_MODULES).forEach(
    ([sectionKey, defaultSection]) => {
      const existingSection = merged[sectionKey]
      merged[sectionKey] = existingSection
        ? { ...defaultSection, ...existingSection }
        : { ...defaultSection }
    }
  )

  return merged
}

function parseSidebarConfig(
  value: string | null | undefined
): SidebarModulesAdminConfig {
  if (!value || value.trim() === '') return DEFAULT_SIDEBAR_MODULES

  try {
    const parsed = JSON.parse(value) as SidebarModulesAdminConfig
    return mergeWithDefaultSidebarModules(parsed)
  } catch {
    return DEFAULT_SIDEBAR_MODULES
  }
}

function parseUserSidebarConfig(
  value: string | null | undefined
): SidebarModulesUserConfig {
  if (!value || value.trim() === '') return null

  try {
    const parsed = JSON.parse(value) as SidebarModulesAdminConfig
    return parsed && typeof parsed === 'object' ? parsed : null
  } catch {
    return null
  }
}

function isModuleEnabled(
  url: string,
  adminConfig: SidebarModulesAdminConfig,
  userConfig: SidebarModulesUserConfig
): boolean {
  const mapping = URL_TO_CONFIG_MAP[url]
  if (!mapping) return true

  const adminSection = adminConfig[mapping.section]
  const adminAllowed = Boolean(
    adminSection &&
      adminSection.enabled !== false &&
      adminSection[mapping.module] !== false
  )
  if (!adminAllowed) return false

  if (!userConfig) return true

  const userSection = userConfig[mapping.section]
  if (!userSection) return true
  if (userSection.enabled === false) return false
  return userSection[mapping.module] !== false
}

function isNavItemVisible(
  item: NavItem,
  adminConfig: SidebarModulesAdminConfig,
  userConfig: SidebarModulesUserConfig
): boolean {
  if ('url' in item && item.url) {
    const configUrls = item.configUrls ?? [item.url]
    return configUrls.some((url) =>
      isModuleEnabled(url as string, adminConfig, userConfig)
    )
  }

  if ('items' in item && item.items) {
    return item.items.some((subItem) =>
      isModuleEnabled(subItem.url as string, adminConfig, userConfig)
    )
  }

  return true
}

function filterNavItems(
  items: NavItem[],
  adminConfig: SidebarModulesAdminConfig,
  userConfig: SidebarModulesUserConfig
): NavItem[] {
  return items
    .map((item) => {
      if ('items' in item && item.items) {
        return {
          ...item,
          items: item.items.filter((subItem) =>
            isModuleEnabled(subItem.url as string, adminConfig, userConfig)
          ),
        }
      }
      return item
    })
    .filter((item) => isNavItemVisible(item, adminConfig, userConfig))
}

export function useSidebarConfig(navGroups: NavGroup[]): NavGroup[] {
  const { status } = useStatus()
  const { auth } = useAuthStore()

  const adminConfig = useMemo(
    () =>
      parseSidebarConfig(
        status?.SidebarModulesAdmin as string | null | undefined
      ),
    [status?.SidebarModulesAdmin]
  )

  const userConfig = useMemo(() => {
    if (auth?.user?.permissions?.sidebar_settings === false) return null
    return parseUserSidebarConfig(auth?.user?.sidebar_modules)
  }, [auth?.user?.permissions?.sidebar_settings, auth?.user?.sidebar_modules])

  return useMemo(
    () =>
      navGroups
        .map((group) => ({
          ...group,
          items: filterNavItems(group.items, adminConfig, userConfig),
        }))
        .filter((group) => group.items.length > 0),
    [navGroups, adminConfig, userConfig]
  )
}
