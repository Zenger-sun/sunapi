import { type Skill, type SkillConfig } from '../types'

export function getSkillCapabilities(skill?: Pick<Skill, 'config'>) {
  const config = skill?.config as SkillConfig | undefined
  if (!Array.isArray(config?.capabilities)) {
    return []
  }
  return config.capabilities
    .filter((item): item is string => typeof item === 'string')
    .map((item) => item.trim())
    .filter(Boolean)
}

export function getSkillTags(skill?: Pick<Skill, 'config'>) {
  const config = skill?.config as SkillConfig | undefined
  if (!Array.isArray(config?.tags)) {
    return []
  }
  return config.tags
    .filter((item): item is string => typeof item === 'string')
    .map((item) => item.trim())
    .filter(Boolean)
}
