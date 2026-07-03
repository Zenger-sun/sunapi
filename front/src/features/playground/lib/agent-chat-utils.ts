/*
Copyright (C) 2026 SunAPI Project Owner
*/

import type { Agent } from '@/features/agents/types'

// ============================================================================
// 触发关键词检测
// ============================================================================

/** 检测消息是否包含 @agent 提及，返回提及的智能体名称 */
export function detectAgentMention(
  text: string,
  agents: Agent[]
): Agent | null {
  const mentionRegex = /@([一-龥\w\s·-]{1,30})/g
  let match: RegExpExecArray | null

  while ((match = mentionRegex.exec(text)) !== null) {
    const mentioned = match[1].trim()
    const found = agents.find(
      (a) => a.name === mentioned || a.name.startsWith(mentioned)
    )
    if (found) return found
  }
  return null
}

/** 检测消息是否包含自动化任务意图关键词 */
export function detectAutomationIntent(text: string): boolean {
  const keywords = [
    '每天', '每周', '每月', '定时', '自动', '定期',
    '自动化', '监控', '抓取', '生成报告', '汇总',
    '每日', '周报', '日报', '月报', '定时执行',
    'automatically', 'schedule', 'every day', 'daily',
  ]
  const lower = text.toLowerCase()
  return keywords.some((kw) => lower.includes(kw))
}

/** 从消息中提取动作类型 */
export type AgentChatAction =
  | { type: 'query_status'; agent: Agent }
  | { type: 'run_now'; agent: Agent }
  | { type: 'view_report'; agent: Agent }
  | { type: 'suggest_create'; prompt: string }
  | null

export function parseAgentChatAction(
  text: string,
  agents: Agent[]
): AgentChatAction {
  const mentionedAgent = detectAgentMention(text, agents)

  if (mentionedAgent) {
    const lower = text.toLowerCase()
    if (
      lower.includes('执行了吗') ||
      lower.includes('状态') ||
      lower.includes('完成了') ||
      lower.includes('怎么样') ||
      lower.includes('status')
    ) {
      return { type: 'query_status', agent: mentionedAgent }
    }
    if (
      lower.includes('立即执行') ||
      lower.includes('马上执行') ||
      lower.includes('运行一次') ||
      lower.includes('run now')
    ) {
      return { type: 'run_now', agent: mentionedAgent }
    }
    if (
      lower.includes('最新报告') ||
      lower.includes('最新结果') ||
      lower.includes('上次输出') ||
      lower.includes('latest report')
    ) {
      return { type: 'view_report', agent: mentionedAgent }
    }
    return { type: 'query_status', agent: mentionedAgent }
  }

  if (detectAutomationIntent(text)) {
    return { type: 'suggest_create', prompt: text }
  }

  return null
}
