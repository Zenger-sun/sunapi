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
import { api } from '@/lib/api'

type ApiEnvelope<T> = {
  success: boolean
  message?: string
  data: T
}

export type Channel = {
  id: number
  type: number
  name: string
  group: string
  base_url: string
  api_key?: string
  models: string
  enabled: boolean
  input_price_per_1k: number
  output_price_per_1k: number
  used_tokens: number
  request_count: number
  created_at: number
  updated_at: number
  remark?: string
}

export type ChannelPayload = Omit<
  Channel,
  'id' | 'used_tokens' | 'request_count' | 'created_at' | 'updated_at'
>

export type ChannelMetadataPayload = Pick<
  ChannelPayload,
  'type' | 'base_url' | 'api_key'
>

export type ChannelMetadata = {
  models: string[]
  input_price_per_1k: number
  output_price_per_1k: number
  source: 'upstream' | 'preset'
}

export type Group = {
  id: number
  name: string
  description?: string
  price_multiplier: number
  channels: number
  used_tokens: number
  request_count: number
  cost: number
  created_at: number
  updated_at: number
}

export type GroupPayload = Pick<
  Group,
  'name' | 'description' | 'price_multiplier'
>

export type UsageLog = {
  id: number
  created_at: number
  channel_id: number
  channel_name: string
  group: string
  model: string
  endpoint: string
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  cost: number
  duration_ms: number
  status_code: number
  error?: string
}

export type AppSettings = {
  system_name: string
  listen_host: string
  listen_port: number
  default_group: string
  default_input_price_per_1k: number
  default_output_price_per_1k: number
  currency_symbol: string
  auto_open_browser: boolean
}

export type DashboardData = {
  total_cost: number
  total_tokens: number
  prompt_tokens: number
  completion_tokens: number
  total_requests: number
  last_24h_cost: number
  last_24h_tokens: number
  last_24h_requests: number
  enabled_channels: number
  total_channels: number
  groups: number
  average_rpm: number
  average_tpm: number
  chart: Array<{
    date: string
    cost: number
    tokens: number
    requests: number
  }>
  top_channels: Array<{
    id: number
    name: string
    cost: number
    tokens: number
    requests: number
  }>
  settings: AppSettings
}

async function unwrap<T>(request: Promise<{ data: ApiEnvelope<T> }>) {
  const response = await request
  return response.data.data
}

export function getDashboard() {
  return unwrap<DashboardData>(api.get('/api/dashboard'))
}

export function getChannels() {
  return unwrap<{ items: Channel[]; total: number }>(api.get('/api/channels'))
}

export function createChannel(payload: ChannelPayload) {
  return unwrap<Channel>(api.post('/api/channels', payload))
}

export function updateChannel(id: number, payload: ChannelPayload) {
  return unwrap<Channel>(api.put(`/api/channels/${id}`, payload))
}

export function deleteChannel(id: number) {
  return unwrap<{ id: number }>(api.delete(`/api/channels/${id}`))
}

export function setChannelEnabled(id: number, enabled: boolean) {
  return unwrap<Channel>(api.patch(`/api/channels/${id}/enabled`, { enabled }))
}

export function syncChannelMetadata(payload: ChannelMetadataPayload) {
  return unwrap<ChannelMetadata>(api.post('/api/channel-metadata/sync', payload))
}

export function getGroups() {
  return unwrap<{ items: Group[]; total: number }>(api.get('/api/groups'))
}

export function createGroup(payload: GroupPayload) {
  return unwrap<Group>(api.post('/api/groups', payload))
}

export function updateGroup(id: number, payload: GroupPayload) {
  return unwrap<Group>(api.put(`/api/groups/${id}`, payload))
}

export function deleteGroup(id: number) {
  return unwrap<{ id: number }>(api.delete(`/api/groups/${id}`))
}

export function getUsageLogs(params: { limit?: number } = {}) {
  return unwrap<{ items: UsageLog[]; total: number }>(
    api.get('/api/usage-logs', { params })
  )
}

export function clearUsageLogs() {
  return unwrap<{ deleted: number }>(api.delete('/api/usage-logs'))
}

export function getSettings() {
  return unwrap<AppSettings>(api.get('/api/settings'))
}

export function updateSettings(payload: AppSettings) {
  return unwrap<AppSettings>(api.put('/api/settings', payload))
}
