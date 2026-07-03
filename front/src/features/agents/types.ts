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
import { z } from 'zod'

// ============================================================================
// 智能体状态枚举
// ============================================================================

export enum AgentStatus {
  ACTIVE = 1,
  INACTIVE = 2,
  ARCHIVED = 3,
}

// ============================================================================
// 蓝图/任务状态枚举
// ============================================================================

export enum BlueprintStatus {
  PENDING = 'pending',      // 待执行
  RUNNING = 'running',      // 执行中
  COMPLETED = 'completed',  // 已结束
  FAILED = 'failed',        // 执行失败
  PAUSED = 'paused',        // 已暂停
}

// ============================================================================
// 触发器类型枚举
// ============================================================================

export enum TriggerType {
  SCHEDULED = 'scheduled',  // 定时触发
  MANUAL = 'manual',        // 手动触发
  WEBHOOK = 'webhook',      // Webhook触发
}

// ============================================================================
// 频率类型枚举
// ============================================================================

export enum FrequencyType {
  ONCE = 'once',           // 一次性
  HOURLY = 'hourly',       // 每小时
  DAILY = 'daily',         // 每天
  WEEKLY = 'weekly',       // 每周
  MONTHLY = 'monthly',     // 每月
  CUSTOM = 'custom',       // 自定义
}

// ============================================================================
// 归档类型枚举
// ============================================================================

export enum ArchiveType {
  PLATFORM = 'platform',   // 平台内置存储
  NOTION = 'notion',       // Notion
  FEISHU = 'feishu',       // 飞书
  GITHUB = 'github',       // GitHub
  WECHAT = 'wechat',       // 企业微信
  WEBHOOK = 'webhook',     // 自定义Webhook
  LOCAL = 'local',         // 本地文件系统
}

// ============================================================================
// Skill 来源类型
// ============================================================================

export enum SkillSourceType {
  BUILTIN = 'builtin',     // 系统内置
  MARKETPLACE = 'marketplace', // 商城安装
  GITHUB = 'github',       // GitHub导入
  CUSTOM = 'custom',       // 自定义创建
}

// ============================================================================
// Skill Schema
// ============================================================================

export const skillSchema = z.object({
  id: z.string(),
  name: z.string(),
  description: z.string(),
  version: z.string(),
  source: z.nativeEnum(SkillSourceType),
  source_url: z.string().optional(),
  enabled: z.boolean(),
  config: z.record(z.any()).optional(),
  permissions: z.array(z.string()).optional(),
  created_time: z.number(),
  updated_time: z.number(),
})

export type Skill = z.infer<typeof skillSchema>

// ============================================================================
// 触发器配置 Schema
// ============================================================================

export const triggerConfigSchema = z.object({
  type: z.nativeEnum(TriggerType),
  frequency: z.nativeEnum(FrequencyType).optional(),
  time: z.string().optional(), // HH:mm 格式
  weekdays: z.array(z.number()).optional(), // 0-6, 0是周日
  dayOfMonth: z.number().optional(), // 1-31
  timezone: z.string().default('Asia/Shanghai'),
  cronExpression: z.string().optional(), // 自定义cron表达式
})

export type TriggerConfig = z.infer<typeof triggerConfigSchema>

// ============================================================================
// 归档配置 Schema
// ============================================================================

export const archiveConfigSchema = z.object({
  type: z.nativeEnum(ArchiveType),
  path: z.string().optional(),
  namingRule: z.string().optional(),
  retentionPolicy: z.string().optional(),
  externalConfig: z.record(z.any()).optional(), // Notion/飞书等外部配置
  triggerOn: z.enum(['always', 'success', 'manual']).default('success'),
})

export type ArchiveConfig = z.infer<typeof archiveConfigSchema>

// ============================================================================
// 蓝图/任务 Schema
// ============================================================================

export const blueprintSchema = z.object({
  id: z.string(),
  agent_id: z.string(),
  name: z.string(),
  description: z.string().optional(),
  status: z.nativeEnum(BlueprintStatus),
  trigger: triggerConfigSchema,
  model_group: z.string(),
  model_name: z.string(),
  skills: z.array(z.string()),
  input_variables: z.record(z.any()).optional(),
  archive_config: archiveConfigSchema.optional(),
  created_time: z.number(),
  updated_time: z.number(),
  last_executed_time: z.number().optional(),
  next_execution_time: z.number().optional(),
  execution_count: z.number().default(0),
  failure_reason: z.string().optional(),
  execution_duration: z.number().optional(),
})

export type Blueprint = z.infer<typeof blueprintSchema>

// ============================================================================
// 智能体 Schema
// ============================================================================

export const agentSchema = z.object({
  id: z.string(),
  name: z.string(),
  description: z.string().optional(),
  status: z.nativeEnum(AgentStatus),
  avatar: z.string().optional(),
  created_time: z.number(),
  updated_time: z.number(),
  blueprint_count: z.number().default(0),
  last_executed_time: z.number().optional(),
})

export type Agent = z.infer<typeof agentSchema>

// ============================================================================
// API 响应类型
// ============================================================================

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface PaginatedResponse<T> {
  items: T[]
  total: number
  page: number
  pageSize: number
}

// ============================================================================
// 表单数据类型
// ============================================================================

export interface AgentFormData {
  name: string
  description?: string
  avatar?: string
}

export interface BlueprintFormData {
  name: string
  description?: string
  trigger: TriggerConfig
  model_group: string
  model_name: string
  skills: string[]
  input_variables?: Record<string, any>
  archive_config?: ArchiveConfig
}

export interface SkillFormData {
  name: string
  description: string
  source_type: SkillSourceType
  source_url?: string
  config?: Record<string, any>
}
