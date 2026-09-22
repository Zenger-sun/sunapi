/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api'
import {
  type Agent,
  type AgentExecutionLog,
  type AgentXianyuReplyRecord,
  type Blueprint,
  type Skill,
  type SkillMarketplaceItem,
  type AgentFormData,
  type BlueprintFormData,
  type SkillFormData,
} from './types'

// ============================================================================
// 通用响应提取
// ============================================================================

function extractData<T>(resp: { data: { data?: T } }): T {
  return resp.data.data as T
}

// ============================================================================
// 智能体 API
// ============================================================================

export async function getAgents(): Promise<Agent[]> {
  const resp = await api.get('/api/agents')
  return extractData<Agent[]>(resp) ?? []
}

export async function createAgent(data: AgentFormData): Promise<Agent> {
  const resp = await api.post('/api/agents', data)
  return extractData<Agent>(resp)
}

export async function updateAgent(
  id: string,
  data: Partial<AgentFormData>
): Promise<Agent> {
  const resp = await api.put(`/api/agents/${id}`, data)
  return extractData<Agent>(resp)
}

export async function deleteAgent(id: string): Promise<void> {
  await api.delete(`/api/agents/${id}`)
}

// ============================================================================
// 蓝图 API
// ============================================================================

export async function getBlueprintsByAgent(agentId: string): Promise<Blueprint[]> {
  const normalizedAgentId = agentId.trim()
  if (!normalizedAgentId) return []
  const resp = await api.get(`/api/agents/${normalizedAgentId}/blueprints`, {
    // This is a read-only panel request. Let the caller render a retry state
    // instead of showing a global toast (and then retrying the same 500).
    skipErrorHandler: true,
  })
  return extractData<Blueprint[]>(resp) ?? []
}

export async function createBlueprint(
  agentId: string,
  data: BlueprintFormData
): Promise<Blueprint> {
  const resp = await api.post(`/api/agents/${agentId}/blueprints`, data)
  return extractData<Blueprint>(resp)
}

export async function updateBlueprint(
  agentId: string,
  id: string,
  data: Partial<BlueprintFormData>
): Promise<Blueprint> {
  const resp = await api.put(`/api/agents/${agentId}/blueprints/${id}`, data)
  return extractData<Blueprint>(resp)
}

export async function deleteBlueprint(agentId: string, id: string): Promise<void> {
  await api.delete(`/api/agents/${agentId}/blueprints/${id}`)
}

export async function pauseBlueprint(agentId: string, id: string): Promise<Blueprint> {
  const resp = await api.post(`/api/agents/${agentId}/blueprints/${id}/pause`)
  return extractData<Blueprint>(resp)
}

export async function resumeBlueprint(agentId: string, id: string): Promise<Blueprint> {
  const resp = await api.post(`/api/agents/${agentId}/blueprints/${id}/resume`)
  return extractData<Blueprint>(resp)
}

export async function retryBlueprint(agentId: string, id: string): Promise<Blueprint> {
  const resp = await api.post(`/api/agents/${agentId}/blueprints/${id}/retry`)
  return extractData<Blueprint>(resp)
}

export async function runBlueprint(agentId: string, id: string): Promise<Blueprint> {
  const resp = await api.post(`/api/agents/${agentId}/blueprints/${id}/run`)
  return extractData<Blueprint>(resp)
}

export async function listBlueprintExecutionLogs(
  agentId: string,
  id: string,
  limit = 5
): Promise<AgentExecutionLog[]> {
  const normalizedAgentId = agentId.trim()
  const normalizedBlueprintId = id.trim()
  if (!normalizedAgentId || !normalizedBlueprintId) return []
  const resp = await api.get(
    `/api/agents/${normalizedAgentId}/blueprints/${normalizedBlueprintId}/logs`,
    {
      params: { limit },
      skipErrorHandler: true,
    }
  )
  return extractData<AgentExecutionLog[]>(resp) ?? []
}

export async function listXianyuReplyRecords(
  agentId: string,
  blueprintId: string,
  limit = 50
): Promise<AgentXianyuReplyRecord[]> {
  const normalizedAgentId = agentId.trim()
  const normalizedBlueprintId = blueprintId.trim()
  if (!normalizedAgentId || !normalizedBlueprintId) return []
  const resp = await api.get(
    `/api/agents/${normalizedAgentId}/blueprints/${normalizedBlueprintId}/reply-records`,
    { params: { limit }, skipErrorHandler: true }
  )
  return extractData<AgentXianyuReplyRecord[]>(resp) ?? []
}

// ============================================================================
// Skill API
// ============================================================================

async function getSkills(): Promise<Skill[]> {
  const resp = await api.get('/api/agent-skills')
  return extractData<Skill[]>(resp) ?? []
}

async function getSkillMarketplace(): Promise<SkillMarketplaceItem[]> {
  const resp = await api.get('/api/agent-skills/marketplace')
  return extractData<SkillMarketplaceItem[]>(resp) ?? []
}

async function createSkill(data: SkillFormData): Promise<Skill> {
  const resp = await api.post('/api/agent-skills', data)
  return extractData<Skill>(resp)
}

async function toggleSkill(id: string, enabled: boolean): Promise<void> {
  await api.put(`/api/agent-skills/${id}/toggle`, { enabled })
}

async function deleteSkill(id: string): Promise<void> {
  await api.delete(`/api/agent-skills/${id}`)
}

// ============================================================================
// React Query Options
// ============================================================================

export const agentsQueryOptions = queryOptions({
  queryKey: ['agents'],
  queryFn: getAgents,
})

export const blueprintsByAgentQueryOptions = (agentId: string) =>
  queryOptions({
    queryKey: ['blueprints', agentId],
    queryFn: () => getBlueprintsByAgent(agentId),
    enabled: !!agentId,
    retry: false,
    // A run completes asynchronously. Poll only while it is active so the UI
    // cannot keep showing a completed or failed execution as "running".
    refetchInterval: (query) =>
      query.state.data?.some((blueprint) => blueprint.status === 'running')
        ? 3000
        : false,
  })

export const skillsQueryOptions = queryOptions({
  queryKey: ['skills'],
  queryFn: getSkills,
})

export const skillMarketplaceQueryOptions = queryOptions({
  queryKey: ['skills', 'marketplace'],
  queryFn: getSkillMarketplace,
})

// ============================================================================
// React Query Mutations
// ============================================================================

export function useCreateAgentMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: createAgent,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['agents'] }),
  })
}

export function useUpdateAgentMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<AgentFormData> }) =>
      updateAgent(id, data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['agents'] }),
  })
}

export function useDeleteAgentMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: deleteAgent,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['agents'] }),
  })
}

export function useCreateBlueprintMutation(agentId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: BlueprintFormData) => createBlueprint(agentId, data),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ['blueprints', agentId] }),
  })
}

export function useUpdateBlueprintMutation(agentId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<BlueprintFormData> }) =>
      updateBlueprint(agentId, id, data),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ['blueprints', agentId] }),
  })
}

export function useDeleteBlueprintMutation(agentId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteBlueprint(agentId, id),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ['blueprints', agentId] }),
  })
}

export function usePauseBlueprintMutation(agentId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => pauseBlueprint(agentId, id),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ['blueprints', agentId] }),
  })
}

export function useResumeBlueprintMutation(agentId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => resumeBlueprint(agentId, id),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ['blueprints', agentId] }),
  })
}

export function useRetryBlueprintMutation(agentId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => retryBlueprint(agentId, id),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ['blueprints', agentId] }),
  })
}

export function useRunBlueprintMutation(agentId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => runBlueprint(agentId, id),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ['blueprints', agentId] }),
  })
}

export function useCreateSkillMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: createSkill,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['skills'] })
      queryClient.invalidateQueries({ queryKey: ['skills', 'marketplace'] })
    },
  })
}

export function useToggleSkillMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      toggleSkill(id, enabled),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['skills'] }),
  })
}

export function useDeleteSkillMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: deleteSkill,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['skills'] }),
  })
}
