/*
Copyright (C) 2026 SunAPI Project Owner

This file is part of the SunAPI Creator Studio Components.

Licensed under the SunAPI Creator Studio Source-Available Commercial License
v1.0. See LICENSE-CREATOR-STUDIO at the repository root.

This source is available for evaluation, but modification, redistribution,
hosting, sale, resale, sublicensing, or commercial use requires prior written
permission from the SunAPI project owner.
*/

export type AgentBlueprintStatus = 'pending' | 'running' | 'paused' | 'ended'

export type AgentBlueprintCommand =
  | { type: 'new' }
  | { type: 'load'; id: string }

export type AgentBlueprintSummary = {
  id: string
  title: string
  nodesCount: number
  updatedAt: number
  status: AgentBlueprintStatus
}

type StoredAgentNode = {
  data?: {
    status?: string
  }
}

type StoredAgentBlueprint = {
  id?: unknown
  title?: unknown
  nodes?: unknown
  edges?: unknown
  createdAt?: unknown
  updatedAt?: unknown
  status?: unknown
}

export const AGENT_BLUEPRINT_STORAGE_KEY =
  'sunapi.playground.agent.blueprints.v1'
export const AGENT_BLUEPRINTS_CHANGED_EVENT =
  'sunapi:agent-blueprints-changed'
export const AGENT_BLUEPRINT_COMMAND_EVENT = 'sunapi:agent-blueprint-command'

export function readAgentBlueprintSummaries(): AgentBlueprintSummary[] {
  if (typeof window === 'undefined') return []

  try {
    const raw = window.localStorage.getItem(AGENT_BLUEPRINT_STORAGE_KEY)
    if (!raw) return []

    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []

    return parsed
      .filter(isStoredAgentBlueprint)
      .map((blueprint) => {
        const nodes = Array.isArray(blueprint.nodes) ? blueprint.nodes : []
        return {
          id: blueprint.id,
          title: blueprint.title,
          nodesCount: nodes.length,
          updatedAt: blueprint.updatedAt,
          status: normalizeBlueprintStatus(blueprint.status, nodes),
        }
      })
      .sort((a, b) => b.updatedAt - a.updatedAt)
  } catch {
    return []
  }
}

export function deleteStoredAgentBlueprint(id: string) {
  if (typeof window === 'undefined') return

  try {
    const raw = window.localStorage.getItem(AGENT_BLUEPRINT_STORAGE_KEY)
    if (!raw) return

    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return

    window.localStorage.setItem(
      AGENT_BLUEPRINT_STORAGE_KEY,
      JSON.stringify(
        parsed.filter((item) => {
          if (!item || typeof item !== 'object') return true
          return (item as StoredAgentBlueprint).id !== id
        })
      )
    )
    notifyAgentBlueprintsChanged()
  } catch {
    /* Ignore localStorage errors in the lightweight sidebar index. */
  }
}

export function dispatchAgentBlueprintCommand(command: AgentBlueprintCommand) {
  if (typeof window === 'undefined') return

  window.dispatchEvent(
    new CustomEvent<AgentBlueprintCommand>(AGENT_BLUEPRINT_COMMAND_EVENT, {
      detail: command,
    })
  )
}

export function notifyAgentBlueprintsChanged() {
  if (typeof window === 'undefined') return

  window.dispatchEvent(new Event(AGENT_BLUEPRINTS_CHANGED_EVENT))
}

export function getAgentBlueprintStatusLabel(status: AgentBlueprintStatus) {
  const labels: Record<AgentBlueprintStatus, string> = {
    pending: '待执行',
    running: '执行中',
    paused: '已暂停',
    ended: '已结束',
  }
  return labels[status]
}

function isStoredAgentBlueprint(
  value: unknown
): value is {
  id: string
  title: string
  nodes: unknown[]
  edges: unknown[]
  createdAt: number
  updatedAt: number
  status?: AgentBlueprintStatus
} {
  if (!value || typeof value !== 'object') return false

  const candidate = value as StoredAgentBlueprint
  return (
    typeof candidate.id === 'string' &&
    typeof candidate.title === 'string' &&
    Array.isArray(candidate.nodes) &&
    Array.isArray(candidate.edges) &&
    typeof candidate.createdAt === 'number' &&
    typeof candidate.updatedAt === 'number'
  )
}

function normalizeBlueprintStatus(
  status: unknown,
  nodes: unknown[]
): AgentBlueprintStatus {
  if (
    status === 'pending' ||
    status === 'running' ||
    status === 'paused' ||
    status === 'ended'
  ) {
    return status
  }

  const nodeStatuses = nodes
    .map((node) =>
      node && typeof node === 'object'
        ? (node as StoredAgentNode).data?.status
        : undefined
    )
    .filter(Boolean)

  if (nodeStatuses.some((nodeStatus) => nodeStatus === 'running')) {
    return 'running'
  }
  if (
    nodeStatuses.length > 0 &&
    nodeStatuses.every((nodeStatus) => nodeStatus === 'done')
  ) {
    return 'ended'
  }
  return 'pending'
}
