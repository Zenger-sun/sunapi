import type { ReactNode } from 'react'
import type { TFunction } from 'i18next'

export type SectionDefinition<TSettings, TExtraArgs extends unknown[] = []> = {
  id: string
  titleKey: string
  build: (settings: TSettings, ...extraArgs: TExtraArgs) => ReactNode
}

export type SectionRegistryConfig<
  TSectionId extends string,
  TSettings,
  TExtraArgs extends unknown[] = [],
> = {
  sections: readonly SectionDefinition<TSettings, TExtraArgs>[]
  defaultSection: TSectionId
  basePath: string
  urlStyle?: 'query' | 'path'
}

export function createSectionRegistry<
  TSectionId extends string,
  TSettings,
  TExtraArgs extends unknown[] = [],
>(config: SectionRegistryConfig<TSectionId, TSettings, TExtraArgs>) {
  const { sections, defaultSection, basePath, urlStyle = 'query' } = config

  type SectionId = TSectionId

  const sectionIds = sections.map((section) => section.id) as [
    SectionId,
    ...SectionId[],
  ]

  function getSectionNavItems(t: TFunction) {
    return sections.map((section) => ({
      title: t(section.titleKey),
      url:
        urlStyle === 'path'
          ? `${basePath}/${section.id}`
          : `${basePath}?section=${section.id}`,
    }))
  }

  function getSectionContent(
    sectionId: SectionId,
    settings: TSettings,
    ...extraArgs: TExtraArgs
  ) {
    return getSectionMeta(sectionId).build(settings, ...extraArgs)
  }

  function getSectionMeta(sectionId: SectionId) {
    return sections.find((item) => item.id === sectionId) ?? sections[0]
  }

  return {
    sectionIds,
    defaultSection,
    getSectionNavItems,
    getSectionContent,
    getSectionMeta,
  }
}
