/*
Copyright (C) 2026 SunAPI Project Owner

This file is part of the SunAPI Creator Studio Components.

Licensed under the SunAPI Creator Studio Source-Available Commercial License
v1.0. See LICENSE-CREATOR-STUDIO at the repository root.

This source is available for evaluation, but modification, redistribution,
hosting, sale, resale, sublicensing, or commercial use requires prior written
permission from the SunAPI project owner.
*/
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
  type DragEvent,
  type ReactNode,
} from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  addEdge,
  applyEdgeChanges,
  applyNodeChanges,
  Background,
  BaseEdge,
  Controls,
  EdgeLabelRenderer,
  getSmoothStepPath,
  Handle,
  MarkerType,
  MiniMap,
  Position,
  ReactFlow,
  ReactFlowProvider,
  useReactFlow,
  useUpdateNodeInternals,
  type AriaLabelConfig,
  type Connection,
  type Edge,
  type EdgeChange,
  type EdgeProps,
  type Node,
  type NodeChange,
  type NodeProps,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import {
  BellIcon,
  BookOpenIcon,
  BotIcon,
  ChartNetworkIcon,
  CheckCircle2Icon,
  ChevronDownIcon,
  ChevronUpIcon,
  ClockIcon,
  CpuIcon,
  FileSpreadsheetIcon,
  FileTextIcon,
  FilterIcon,
  GlobeIcon,
  ImageIcon,
  MergeIcon,
  MessagesSquareIcon,
  MonitorIcon,
  NetworkIcon,
  PauseIcon,
  PlayIcon,
  PlusIcon,
  Redo2Icon,
  RotateCcwIcon,
  SaveIcon,
  SearchIcon,
  Settings2Icon,
  SparklesIcon,
  StopCircleIcon,
  Table2Icon,
  TriangleAlertIcon,
  Trash2Icon,
  Undo2Icon,
  UploadIcon,
  WebhookIcon,
  WrenchIcon,
  XIcon,
  ZapIcon,
  type LucideIcon,
} from 'lucide-react'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  NativeSelect,
  NativeSelectOptGroup,
  NativeSelectOption,
} from '@/components/ui/native-select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  createAgent,
  createBlueprint,
  deleteBlueprint,
  getAgents,
  getBlueprintsByAgent,
  listXianyuReplyRecords,
  listBlueprintExecutionLogs,
  pauseBlueprint,
  resumeBlueprint,
  runBlueprint,
  updateBlueprint,
} from '@/features/agents/api'
import {
  ArchiveType,
  FrequencyType,
  TriggerType,
  type AgentExecutionLog,
  type AgentXianyuReplyRecord,
  type Blueprint,
  type TriggerConfig,
} from '@/features/agents/types'
import {
  createAgentComponent,
  createAgentSkill,
  deleteAgentComponent,
  deleteAgentSkill,
  getUserModels,
  listAgentSkillMarketplace,
  listAgentSkills,
  listAgentFiles,
  listAgentComponents,
  uploadAgentFile,
} from '../api'
import {
  AGENT_BLUEPRINT_COMMAND_EVENT,
  AGENT_BLUEPRINTS_CHANGED_EVENT,
  AGENT_BLUEPRINT_STORAGE_KEY,
  getAgentBlueprintStatusLabel,
  notifyAgentBlueprintsChanged,
  type AgentBlueprintCommand,
  type AgentBlueprintStatus,
} from '../lib/agent-blueprints'
import type { AgentSkillItem, AgentSkillMarketplaceItem } from '../types'
import type {
  AgentFileUpload,
  AgentFileUploadPurpose,
  AgentComponentItem,
  AgentComponentFormData,
  GroupOption,
  ModelOption,
} from '../types'
import {
  KnowledgeScopeSelector,
  type KnowledgeScopeValue,
} from './knowledge-scope-selector'

type AgentNodeKind =
  | 'start'
  | 'schedule'
  | 'browser'
  | 'xianyu'
  | 'xianyu_reply'
  | 'xianyu_review'
  | 'request'
  | 'transform'
  | 'image'
  | 'report'
  | 'webpage'
  | 'notify'
  | 'stop'
  | 'custom'

type AgentNodeStatus = 'ready' | 'waiting' | 'running' | 'done' | 'failed'
type AgentNodeLogStatus =
  | 'pending'
  | 'running'
  | 'completed'
  | 'stopped'
  | 'failed'

type ModelRole = 'system' | 'browser' | 'fast' | 'reasoning' | 'image'

type AgentNodeData = {
  kind: AgentNodeKind
  componentId?: string
  title: string
  description: string
  group?: string
  model: string
  skill: string
  inputs: string[]
  outputs: string[]
  settings: Record<string, string>
  status: AgentNodeStatus
}

type AgentNodeExecutionLog = {
  nodeId: string
  status: AgentNodeLogStatus
  startedAt?: number
  endedAt?: number
  durationMs?: number
  message: string
  inputSummary: string[]
  outputSummary: string[]
  modelOutput: string
  steps: string[]
  error?: string
}

type AgentExecutionLogMap = Record<string, AgentNodeExecutionLog>

type AgentFlowNode = Node<AgentNodeData, 'agentNode'>
type AgentBranchRoute = 'true' | 'false'
type AgentRouteKind = 'branch' | 'switch' | 'loop'
type AgentFlowEdgeData = {
  branchRoute?: AgentBranchRoute
  routeKey?: string
  routeKind?: AgentRouteKind
  routeLabel?: string
}
type AgentFlowEdge = Edge<AgentFlowEdgeData>
type AgentEdgeDraft = Partial<AgentFlowEdge> & {
  source?: string | null
  target?: string | null
}

const BRANCH_TRUE_HANDLE = 'branch-true'
const BRANCH_FALSE_HANDLE = 'branch-false'
const SWITCH_HANDLE_PREFIX = 'switch-'
const SWITCH_DEFAULT_HANDLE = 'switch-default'
const LOOP_ITEM_HANDLE = 'loop-item'
const LOOP_DONE_HANDLE = 'loop-done'

type BackendAgentNodeResult = {
  node_id?: string
  kind?: string
  title?: string
  status?: string
  message?: string
  input?: unknown
  output?: unknown
  started_at?: number
  ended_at?: number
  duration_ms?: number
  error?: string
  archive_path?: string
}

type BackendAgentGraphOutput = {
  mode?: string
  nodes?: BackendAgentNodeResult[]
  final_output?: unknown
  warnings?: string[]
}

type BackendAgentProgress = {
  phase?: string
  node_id?: string
  node_kind?: string
  node_title?: string
  message?: string
  current?: number
  total?: number
  completed?: number
  completed_nodes?: number
  total_nodes?: number
  concurrency?: number
  [key: string]: unknown
}

type ModelChoice = {
  label: string
  value: string
  description?: string
  supportsImage?: boolean
}

type SkillChoice = {
  label: string
  value: string
  description: string
  source: SkillSource
  reference?: string
  capabilities?: string[]
  marketplaceId?: string
  modelGroup?: string
  modelName?: string
  tags?: string[]
  /** Undefined keeps legacy saved Skills available on every node. */
  appliesTo?: AgentNodeKind[]
}

type SkillSource = 'built-in' | 'github' | 'marketplace' | 'custom'

type SkillDraftSource = Exclude<SkillSource, 'built-in'>

type ComponentPortOption = {
  value: string
  label: string
  setting?: string
}

const CUSTOM_COMPONENT_INPUT_OPTIONS: ComponentPortOption[] = [
  { value: 'upstream-data', label: '上游数据' },
  { value: 'import-file', label: '导入文件', setting: '导入文件' },
  { value: 'text-input', label: '文本输入', setting: '文本输入' },
  { value: 'url-input', label: '网址', setting: '网址' },
  { value: 'json-input', label: 'JSON 数据', setting: 'JSON 数据' },
  { value: 'browser-session', label: '浏览器会话', setting: '会话名称' },
  { value: 'blueprint-variable', label: '蓝图变量', setting: '变量名' },
]

const CUSTOM_COMPONENT_OUTPUT_OPTIONS: ComponentPortOption[] = [
  { value: 'execution-result', label: '执行结果' },
  { value: 'processed-data', label: '处理数据' },
  { value: 'json-output', label: 'JSON 数据' },
  { value: 'text-output', label: '文本结果' },
  { value: 'generated-file', label: '生成文件' },
  { value: 'file-path', label: '文件路径' },
  { value: 'image-url', label: '图片 URL' },
]

function componentPortLabels(values: string[], options: ComponentPortOption[]) {
  return values
    .map((value) => options.find((option) => option.value === value)?.label)
    .filter((label): label is string => Boolean(label))
}

function componentInputSettings(values: string[]) {
  return values.reduce<Record<string, string>>((settings, value) => {
    const option = CUSTOM_COMPONENT_INPUT_OPTIONS.find(
      (candidate) => candidate.value === value
    )
    if (option?.setting) settings[option.setting] = ''
    return settings
  }, {})
}

type SkillStore = {
  id: string
  name: string
  description: string
  url?: string
  builtin?: boolean
}

type AgentToolPanel = 'components' | 'skills' | 'knowledge'

type BuiltinSkillDefinition = SkillChoice & {
  appliesTo: AgentNodeKind[]
  instructions: string[]
}

type TemplateBinding = {
  path: string
  originalName: string
  preview?: string
  hash?: string
}

type AgentPaletteGroup = '触发' | '采集' | '处理' | '落档' | '结束'

type WorkflowValidationIssue = {
  message: string
  nodeId?: string
}

type WorkflowStageState = {
  label: AgentPaletteGroup
  required: boolean
  present: boolean
  complete: boolean
}

type WorkflowReadiness = {
  ready: boolean
  issues: WorkflowValidationIssue[]
  stages: WorkflowStageState[]
}

type NodeTemplate = {
  kind: AgentNodeKind
  componentId?: string
  source?: 'builtin' | 'custom'
  title: string
  description: string
  paletteGroup?: AgentPaletteGroup
  group?: string
  model: string
  skill: string
  inputs: string[]
  outputs: string[]
  settings: Record<string, string>
  status?: AgentNodeStatus
}

type AgentWorkspaceProps = {
  models: ModelOption[]
  groups?: GroupOption[]
  currentModel?: string
  currentGroup?: string
  seedPrompt?: string
}

type SavedAgentBlueprint = {
  id: string
  title: string
  nodes: AgentFlowNode[]
  edges: AgentFlowEdge[]
  createdAt: number
  updatedAt: number
  status?: AgentBlueprintStatus
  executionLog?: AgentExecutionLogMap
  backendAgentId?: string
  backendBlueprintId?: string
  syncState?: 'synced' | 'draft'
  knowledgeScope?: KnowledgeScopeValue
}

type BlueprintChoice = {
  label: string
  localId: string
  value: string
}

type AgentPreviewRunMode = 'default' | 'manual'

type BackendBlueprintFormOptions = {
  forceCreate?: boolean
  triggerMode?: 'auto' | 'manual'
  modelsValidated?: boolean
}

type AgentBlueprintHistorySnapshot = {
  title: string
  nodes: AgentFlowNode[]
  edges: AgentFlowEdge[]
  executionLog: AgentExecutionLogMap
  selectedNodeId: string
}

const DRAG_NODE_TYPE = 'application/sunapi-agent-node'
const AGENT_SKILLS_STORAGE_KEY = 'sunapi.playground.agent.skills.v1'
const AGENT_SKILL_STORES_STORAGE_KEY = 'sunapi.playground.skill.stores.v1'
const MAX_BLUEPRINT_HISTORY = 50
const AGENT_BACKEND_POLL_INTERVAL_MS = 900
// Full-history archive runs can legitimately exceed ten minutes. The backend
// keeps the run alive for an hour; keep polling for a day so the UI does not
// silently stop updating while a large knowledge-base sync is still running.
const AGENT_BACKEND_POLL_TIMEOUT_MS = 24 * 60 * 60 * 1000

const CREATOR_FLOW_ARIA_LABEL_CONFIG: Partial<AriaLabelConfig> = {
  'controls.ariaLabel': '画布控制',
  'controls.zoomIn.ariaLabel': '放大',
  'controls.zoomOut.ariaLabel': '缩小',
  'controls.fitView.ariaLabel': '适应视图',
  'controls.interactive.ariaLabel': '锁定/解锁画布',
}

const NO_SKILL_VALUE = ''
const NO_SKILL_CHOICE: SkillChoice = {
  label: '不使用 Skill',
  value: NO_SKILL_VALUE,
  description: '该节点仅按基础提示词执行，不叠加任何 Skill 优化。',
  source: 'built-in',
}

const DEFAULT_SKILL_STORE: SkillStore = {
  id: 'official',
  name: '官方 Skill 商城',
  description: '内置常用 Skill，优先保证可用性和通用场景覆盖。',
  builtin: true,
}

const BUILTIN_SKILL_DEFINITIONS: BuiltinSkillDefinition[] = [
  {
    label: '网页研究 Skill',
    value: 'skill.web-research',
    description: '检索、网页阅读和事实核验，适合资料收集与趋势调研。',
    source: 'built-in',
    appliesTo: ['browser', 'request', 'webpage', 'report', 'custom'],
    capabilities: ['关键词规划', '来源交叉验证', '引用摘要'],
    instructions: [
      '先把任务拆成 2-5 个可检索问题，再围绕当前节点输入进行查找和摘要。',
      '优先使用权威、官方、近期或一手来源；多个来源冲突时明确说明差异。',
      '输出结论、依据和不确定点，不要把未验证内容当作事实。',
    ],
  },
  {
    label: '文件文档 Skill',
    value: 'skill.file-document',
    description: '读取整理 Excel、CSV、PDF、Word、Markdown、HTML、TXT 等文件。',
    source: 'built-in',
    appliesTo: ['transform', 'report', 'webpage', 'custom'],
    capabilities: ['多格式解析', '模板遵循', '结构保真'],
    instructions: [
      '只使用当前上传或上游显式提供的文件内容，不猜测未提供的表格、标题或业务背景。',
      '保留文件中的关键字段、工作表、表头、段落层级和格式要求。',
      '输出格式必须跟模板或节点配置一致；无法读取时说明具体缺失文件或格式限制。',
    ],
  },
  {
    label: '浏览器自动化 Skill',
    value: 'skill.browser-automation',
    description: '处理登录态页面、表单、动态网页、截图证据和 DOM 抽取。',
    source: 'built-in',
    appliesTo: ['browser', 'xianyu', 'xianyu_reply', 'custom'],
    capabilities: ['页面导航', 'DOM 抽取', '操作安全'],
    instructions: [
      '按节点给出的 URL、登录态和任务描述执行导航、读取、截图和字段抽取。',
      '遇到弹窗、登录、分页、懒加载或动态内容时记录处理步骤和剩余限制。',
      '默认只读；购买、删除、提交、发布、转账等高风险动作必须停下并等待确认。',
    ],
  },
  {
    label: '代码解释器 Skill',
    value: 'skill.code-interpreter',
    description: '用于数据清洗、计算、图表、文件转换和可复现脚本化分析。',
    source: 'built-in',
    appliesTo: ['transform', 'image', 'report', 'custom'],
    capabilities: ['数据清洗', '计算分析', '文件生成'],
    instructions: [
      '优先用确定性的表格/脚本处理计算、清洗、去重、排序、转换和统计任务。',
      '输出关键步骤、计算口径、生成文件路径或可复用结果，避免只给泛泛建议。',
      '如果数据不足或格式不一致，先列出阻塞字段和建议修复方式。',
    ],
  },
  {
    label: 'API 函数 Skill',
    value: 'skill.api-function',
    description: '组织 HTTP/API 调用、函数参数、重试策略和结构化响应解析。',
    source: 'built-in',
    appliesTo: ['request', 'notify', 'custom'],
    capabilities: ['参数校验', 'JSON 解析', '错误恢复'],
    instructions: [
      '根据节点中的 endpoint、method、headers、body、鉴权和超时配置组织请求。',
      '把响应解析成稳定 JSON、表格或日志摘要，保留状态码、错误信息和关键字段。',
      '遇到分页、限流、空响应或非 JSON 响应时说明处理策略，不伪造结果。',
    ],
  },
  {
    label: '知识检索 Skill',
    value: 'skill.rag-file-search',
    description: '知识库、文档检索、片段引用和问答总结的检索增强能力。',
    source: 'built-in',
    appliesTo: [
      'browser',
      'xianyu',
      'xianyu_reply',
      'request',
      'transform',
      'report',
      'custom',
    ],
    capabilities: ['语义检索', '片段引用', '答案合成'],
    instructions: [
      '围绕问题检索最相关的文档片段，并基于片段回答，不使用未命中的外部背景补全。',
      '保留来源文件、页码、段落或片段标识，区分事实、推断和待确认内容。',
      '当检索结果不足时，明确说明没有足够依据，而不是生成看似完整的答案。',
    ],
  },
  {
    label: '数据库 SQL Skill',
    value: 'skill.database-sql',
    description: '结构化数据查询、SQL 生成、指标口径和结果解释。',
    source: 'built-in',
    appliesTo: ['request', 'transform', 'report', 'custom'],
    capabilities: ['Schema 理解', 'SQL 生成', '结果校验'],
    instructions: [
      '先确认表结构、字段含义、过滤条件和时间范围，再生成查询或解释结果。',
      '优先使用只读查询；避免破坏性 SQL，涉及写入、删除、更新时必须请求确认。',
      '输出 SQL、参数、结果摘要和可能的数据质量风险。',
    ],
  },
  {
    label: 'MCP 工作流 Skill',
    value: 'skill.mcp-workflow',
    description:
      '适配 MCP/GitHub/文件系统等工具编排和本地流程控制，强调权限边界和可追踪步骤。',
    source: 'built-in',
    appliesTo: ['start', 'schedule', 'custom', 'notify', 'stop'],
    capabilities: ['工具选择', '流程控制', '分支循环', '步骤追踪'],
    instructions: [
      '根据任务选择最少必要工具，说明每一步输入、输出和失败补偿方案。',
      '处理条件分支、等待延迟、列表循环和批处理时，必须保留条件、限制、截断、耗时和取消状态。',
      '严格遵守工具权限边界，不越权读取、写入或调用未配置的外部系统。',
      '对 GitHub、文件系统、任务管理等工具操作保留可审计摘要。',
    ],
  },
]

const BASE_SKILL_CHOICES: SkillChoice[] = BUILTIN_SKILL_DEFINITIONS

// 内置 Skill 的展示元数据：图标、颜色、可用状态
const BUILTIN_SKILL_META: Record<
  string,
  { icon: LucideIcon; accent: string; badge: string }
> = {
  'skill.web-research': {
    icon: SearchIcon,
    accent: 'bg-blue-500/12 text-blue-600 dark:text-blue-400',
    badge: '可用',
  },
  'skill.file-document': {
    icon: FileTextIcon,
    accent: 'bg-amber-500/12 text-amber-600 dark:text-amber-400',
    badge: '可用',
  },
  'skill.browser-automation': {
    icon: GlobeIcon,
    accent: 'bg-cyan-500/12 text-cyan-600 dark:text-cyan-400',
    badge: '可用',
  },
  'skill.code-interpreter': {
    icon: CpuIcon,
    accent: 'bg-violet-500/12 text-violet-600 dark:text-violet-400',
    badge: '可用',
  },
  'skill.api-function': {
    icon: NetworkIcon,
    accent: 'bg-emerald-500/12 text-emerald-600 dark:text-emerald-400',
    badge: '可用',
  },
  'skill.rag-file-search': {
    icon: ChartNetworkIcon,
    accent: 'bg-sky-500/12 text-sky-600 dark:text-sky-400',
    badge: '可用',
  },
  'skill.database-sql': {
    icon: Table2Icon,
    accent: 'bg-slate-500/12 text-slate-600 dark:text-slate-400',
    badge: '可用',
  },
  'skill.mcp-workflow': {
    icon: SparklesIcon,
    accent: 'bg-rose-500/12 text-rose-600 dark:text-rose-400',
    badge: '可用',
  },
}

const LEGACY_BUILTIN_SKILL_ALIASES: Record<string, string> = {
  'skill.system-orchestration': 'skill.mcp-workflow',
  'skill.schedule-guard': 'skill.mcp-workflow',
  'skill.browser-reading': 'skill.browser-automation',
  'skill.network-fetch': 'skill.api-function',
  'skill.data-cleaning': 'skill.code-interpreter',
  'skill.image-generation': 'skill.code-interpreter',
  'skill.report-writing': 'skill.file-document',
  'skill.notification-routing': 'skill.mcp-workflow',
}

const DEFAULT_SKILL_BY_NODE_KIND: Partial<Record<AgentNodeKind, string>> = {
  start: 'skill.mcp-workflow',
  schedule: 'skill.mcp-workflow',
  browser: 'skill.browser-automation',
  xianyu: 'skill.browser-automation',
  xianyu_reply: 'skill.browser-automation',
  xianyu_review: 'skill.browser-automation',
  request: 'skill.api-function',
  transform: 'skill.code-interpreter',
  image: 'skill.code-interpreter',
  report: 'skill.file-document',
  webpage: 'skill.file-document',
  notify: 'skill.api-function',
  stop: 'skill.mcp-workflow',
}

const FREQUENCY_OPTIONS = ['每天', '工作日', '每周', '每月', '每小时', '仅一次']
const TIMEZONE_OPTIONS = [
  'Asia/Shanghai',
  'Asia/Singapore',
  'Asia/Tokyo',
  'UTC',
  'America/Los_Angeles',
  'America/New_York',
  'Europe/London',
  'Europe/Berlin',
]
const REPORT_OUTPUT_FORMAT_SETTING = '输出格式'
const REPORT_OUTPUT_PATH_LABEL = '归档路径'
const BROWSER_PAGE_DATA_SETTING = '页面数据'
const BROWSER_CAPTURE_TEXT_SETTING = '采集文本'
const BROWSER_SCREENSHOT_SETTING = '采集截图'
const BROWSER_SCREENSHOT_ERROR_SETTING = '采集截图错误'
const BROWSER_DATA_MISSING_MARKER = 'browser_data_missing'
const HTTP_RESPONSE_SETTING = 'HTTP 响应'
const HTTP_STATUS_SETTING = 'HTTP 状态码'
const HTTP_ERROR_SETTING = 'HTTP 错误'
const IMAGE_TASK_ID_SETTING = '图片任务 ID'
const IMAGE_TASK_STATUS_SETTING = '图片任务状态'
const IMAGE_TASK_URLS_SETTING = '图片 URL'
const IMAGE_TASK_ERROR_SETTING = '图片错误'
const IMAGE_TASK_PROMPT_SETTING = '图片任务提示词'
const DEFAULT_REPORT_OUTPUT_FORMATS = ['Markdown 文档', 'CSV 表格']
const REPORT_OUTPUT_FORMAT_PRESETS = [
  'Markdown 文档',
  'CSV 表格',
  'Excel 表格',
  'Word 文档',
  'PDF 文档',
  'HTML 网页',
  'TXT 文本',
  'JSON 数据',
]
const SETTING_SELECT_OPTIONS: Record<string, string[]> = {
  触发方式: ['手动', '定时', 'Webhook', '对话触发'],
  运行模式: ['单次执行', '循环执行', '仅预览', '失败后重试'],
  登录态: ['持久化登录会话（首次登录后复用）', '每次等待手动登录', '无需登录'],
  登录等待: ['30s', '60s', '120s', '180s'],
  列表等待: ['3s', '5s', '8s', '12s'],
  单会话等待: ['1s', '2s', '3s'],
  评价等级: ['好评'],
  消息轮询间隔: ['2s', '3s', '5s', '10s', '15s'],
  监听范围: ['仅未读消息'],
  每轮最大会话数: ['10', '20', '30', '50'],
  单次未清楚补答: ['最多1次'],
  启动时处理现有未读: ['是', '否'],
  最大回复字数: ['100', '200', '300', '500'],
  默认回复: ['开启', '关闭'],
  无知识库命中: ['安全澄清回复'],
  发送方式: ['直接自动发送'],
  历史加载轮数: ['1', '3', '6', '10'],
  摘要并发数: ['1', '2'],
  会话数量: ['全部', '10', '20', '50', '100'],
  评价数量: ['全部', '10', '20', '50', '100'],
  最大评价保护上限: ['500', '1000', '2000', '5000'],
  最大会话保护上限: ['500', '1000', '2000', '5000'],
  同步范围: ['仅归档未归档或有新消息的会话', '每次归档当前列表全部会话'],
  权限: ['只读采集', '读写操作需确认', '禁止写入'],
  方法: ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'],
  超时: ['10s', '30s', '60s', '120s'],
  重试: ['0 次', '1 次', '2 次', '3 次'],
  数据类型: [
    '自动识别',
    '通用表格/CSV',
    'JSON 对象/数组',
    '日志/事件数据',
    '纯文本',
  ],
  输出格式: [
    '自动整理',
    '标准表格',
    'Markdown 表格',
    'CSV',
    'JSON 数组',
    '关键值摘要',
  ],
  表格字段: [
    '自动按数据类型推断',
    '名称, 数值, 状态, 备注',
    'ID, 标题, 类型, 数值, 更新时间',
    '日期, 渠道, 访客, 成交, 转化率',
    '自定义字段',
  ],
  去重规则: ['按 ID', '按链接', '按标题 + 日期', '不去重'],
  缺失处理: ['保留空值并标注', '填充 0', '跳过该行', '请求人工确认'],
  尺寸: ['16:9', '1:1', '4:3', '9:16'],
  风格: ['清爽数据看板', '极简图表', '文档配图', '写实图片'],
  数量: ['1', '2', '3', '4'],
  格式: ['Markdown + CSV', 'Markdown', 'PDF', 'Markdown + 图片'],
  语气: ['简洁客观', '专业分析', '自然说明', '行动建议'],
  渠道: ['邮件', '企业微信', '钉钉', '飞书', 'Slack', 'Webhook'],
  确认: ['发送前确认', '自动发送', '失败时确认'],
  结束方式: ['本轮完成后停止', '达到次数后停止', '手动停止', '出错停止'],
  输出模式: ['输出网页 + 数据', '仅输出数据'],
  步骤来源: ['系统内置', '自定义'],
  执行策略: ['按描述执行', '结构化处理', '只读采集', '写入前确认'],
  模式: ['读取', '写入', '追加', '转换'],
  鉴权: ['无', '签名校验', 'Token', 'Basic Auth'],
  合并方式: ['追加', '按字段', '按索引', '保留全部分支'],
  冲突处理: ['保留最新', '保留最早', '输出冲突记录', '请求人工确认'],
  条件: ['字段存在', '表达式为真', '数值比较', '文本包含'],
  未命中处理: ['跳过', '输出空数组', '中断流程', '请求人工确认'],
  为空时: ['使用上游数据', '保留空值'],
}

const NODE_META: Record<
  AgentNodeKind,
  {
    label: string
    group: AgentPaletteGroup
    role: ModelRole
    icon: LucideIcon
    accent: string
    description: string
    inputs: string[]
    outputs: string[]
    settings: Record<string, string>
  }
> = {
  start: {
    label: '开始',
    group: '触发',
    role: 'system',
    icon: PlayIcon,
    accent: 'bg-emerald-500/12 text-emerald-700 dark:text-emerald-300',
    description: '手动启动一次蓝图任务。',
    inputs: [],
    outputs: ['执行信号'],
    settings: {
      触发方式: '手动',
      运行模式: '单次执行',
    },
  },
  schedule: {
    label: '定时',
    group: '触发',
    role: 'system',
    icon: ClockIcon,
    accent: 'bg-sky-500/12 text-sky-700 dark:text-sky-300',
    description: '按固定时间自动触发任务。',
    inputs: [],
    outputs: ['定时信号'],
    settings: {
      执行频率: '每天',
      执行时间: '08:30',
      时区: 'Asia/Shanghai',
    },
  },
  browser: {
    label: '打开浏览器',
    group: '采集',
    role: 'browser',
    icon: GlobeIcon,
    accent: 'bg-blue-500/12 text-blue-700 dark:text-blue-300',
    description: '打开网页并采集页面内容；需要登录时可复用已保存的浏览器会话。',
    inputs: ['网址', '登录要求'],
    outputs: ['浏览器会话', '登录状态'],
    settings: {
      网址: '',
      登录态: '持久化登录会话（首次登录后复用）',
      会话名称: '默认',
      登录等待: '120s',
      权限: '只读采集',
      采集等待: '6s',
    },
  },
  xianyu: {
    label: '闲鱼客服归档',
    group: '采集',
    role: 'reasoning',
    icon: MessagesSquareIcon,
    accent: 'bg-orange-500/12 text-orange-700 dark:text-orange-300',
    description:
      '复用浏览器中的闲鱼卖家登录态，只读遍历客服会话，逐条完整读取历史，再按固定标题生成脱敏知识库内容。',
    inputs: ['闲鱼客服会话', '当前智能体知识库'],
    outputs: ['知识库文档', '会话统计'],
    settings: {
      网址: 'https://seller.goofish.com/?site=COMM',
      登录态: '持久化登录会话（首次登录后复用）',
      会话名称: '闲鱼客服',
      登录等待: '180s',
      列表等待: '8s',
      会话数量: '全部',
      同步范围: '仅归档未归档或有新消息的会话',
      单会话等待: '1s',
      历史加载轮数: '6',
      摘要并发数: '2',
      最大会话保护上限: '5000',
      对话列表选择器: '',
      消息区域选择器: '',
      消息滚动容器选择器: '',
      商品详情选择器: '',
      权限: '只读采集',
      节点超时: '不限制',
    },
  },
  xianyu_reply: {
    label: '闲鱼实时智能客服',
    group: '处理',
    role: 'reasoning',
    icon: MessagesSquareIcon,
    accent: 'bg-emerald-500/12 text-emerald-700 dark:text-emerald-300',
    description:
      '监听已打开的闲鱼卖家工作台页面，在新买家消息到达时检索知识库并直接自动回复。',
    inputs: ['闲鱼新消息', '当前智能体知识库'],
    outputs: ['自动回复统计', '监听状态'],
    settings: {
      网址: 'https://seller.goofish.com/?site=COMM',
      登录态: '持久化登录会话（首次登录后复用）',
      会话名称: '闲鱼客服',
      登录等待: '180s',
      列表等待: '8s',
      消息轮询间隔: '2s',
      监听范围: '仅未读消息',
      每轮最大会话数: '20',
      单次未清楚补答: '最多1次',
      单会话等待: '1s',
      启动时处理现有未读: '是',
      最大回复字数: '300',
      默认回复: '开启',
      无知识库命中: '安全澄清回复',
      发送方式: '直接自动发送',
      对话列表选择器: '',
      消息区域选择器: '',
      消息滚动容器选择器: '',
      商品详情选择器: '',
      消息输入框选择器: '',
      发送按钮选择器: '',
      节点超时: '不限制',
    },
  },
  xianyu_review: {
    label: '闲鱼自动评价',
    group: '处理',
    role: 'browser',
    icon: MessagesSquareIcon,
    accent: 'bg-amber-500/12 text-amber-700 dark:text-amber-300',
    description:
      '先进入闲鱼评价管理的待卖家评价列表，逐页全选并批量提交好评，确认当前页完成后再翻页直到没有待评价商品。',
    inputs: ['待卖家评价订单'],
    outputs: ['评价统计', '操作日志'],
    settings: {
      网址:
        'https://seller.goofish.com/?site=COMMONPRO#/seller-trade/evaluation-manage',
      登录态: '持久化登录会话（首次登录后复用）',
      会话名称: '闲鱼卖家工作台',
      登录等待: '180s',
      列表等待: '8s',
      评价内容: '沟通顺畅，交易愉快',
      评价等级: '好评',
      评价数量: '全部',
      最大评价保护上限: '2000',
      单次评价等待: '3s',
      订单列表选择器: '',
      评价按钮选择器: '',
      评价弹窗选择器: '',
      好评选项选择器: '',
      评价输入框选择器: '',
      评价提交按钮选择器: '',
      权限: '读写操作',
      节点超时: '不限制',
    },
  },
  request: {
    label: 'API / 网页请求',
    group: '采集',
    role: 'fast',
    icon: NetworkIcon,
    accent: 'bg-cyan-500/12 text-cyan-700 dark:text-cyan-300',
    description:
      '发起裸 HTTP 请求，获取接口或页面的结构化原始数据，不使用浏览器登录态。',
    inputs: ['任务描述'],
    outputs: ['JSON 数据', '请求日志'],
    settings: {
      网址: '',
      请求参数: '',
      方法: 'GET',
      超时: '30s',
      重试: '2 次',
    },
  },
  transform: {
    label: '整理数据',
    group: '处理',
    role: 'reasoning',
    icon: Table2Icon,
    accent: 'bg-violet-500/12 text-violet-700 dark:text-violet-300',
    description:
      '清洗、归并、排序数据；有模板时按模板输出，没有模板时按数据类型自动整理。',
    inputs: ['原始数据', '字段/格式模板', '任务描述'],
    outputs: ['标准表格', '关键结论'],
    settings: {
      数据类型: '自动识别',
      输出格式: '自动整理',
      表格字段: '自动按数据类型推断',
      整理模板: '未上传模板',
      模板原文件名: '',
      模板预览: '',
      模板内容: '',
      去重规则: '按 ID',
      缺失处理: '保留空值并标注',
    },
  },
  image: {
    label: '生成图片',
    group: '处理',
    role: 'image',
    icon: ImageIcon,
    accent: 'bg-fuchsia-500/12 text-fuchsia-700 dark:text-fuchsia-300',
    description: '根据数据和提示词生成封面图、趋势图或运营看板图片。',
    inputs: ['数据摘要', '图片提示词'],
    outputs: ['图片 URL', '图片说明'],
    settings: {
      尺寸: '16:9',
      风格: '清爽数据看板',
      数量: '1',
      负面提示词: '',
    },
  },
  report: {
    label: '生成报告',
    group: '落档',
    role: 'reasoning',
    icon: FileTextIcon,
    accent: 'bg-amber-500/12 text-amber-700 dark:text-amber-300',
    description: '把处理结果保存为文档或结构化文件，供后续查看和复用。',
    inputs: ['标准表格', '关键结论', '图片 URL'],
    outputs: [...DEFAULT_REPORT_OUTPUT_FORMATS, REPORT_OUTPUT_PATH_LABEL],
    settings: {
      模板文件: '',
      模板原文件名: '',
      模板预览: '',
      输出格式: DEFAULT_REPORT_OUTPUT_FORMATS.join('、'),
      输出目录: 'data/output/reports',
    },
  },
  notify: {
    label: '发送通知',
    group: '落档',
    role: 'fast',
    icon: BellIcon,
    accent: 'bg-rose-500/12 text-rose-700 dark:text-rose-300',
    description: '将结果发送到指定渠道，或提醒相关人员进行确认。',
    inputs: ['处理结果', '通知目标'],
    outputs: ['发送结果'],
    settings: {
      渠道: 'Webhook',
      通知目标: '',
      确认: '发送前确认',
      Webhook: '',
    },
  },
  custom: {
    label: '自定义步骤',
    group: '处理',
    role: 'fast',
    icon: WrenchIcon,
    accent: 'bg-amber-500/12 text-amber-700 dark:text-amber-300',
    description: '用任务描述、输入和输出定义一个可复用的通用步骤。',
    inputs: ['上游数据'],
    outputs: ['执行结果'],
    settings: {
      步骤来源: '自定义',
      执行策略: '按描述执行',
    },
  },
  webpage: {
    label: '输出网页',
    group: '落档',
    role: 'reasoning',
    icon: MonitorIcon,
    accent: 'bg-teal-500/12 text-teal-700 dark:text-teal-300',
    description: '将处理结果渲染为可访问的网页，可绑定已有网页模板。',
    inputs: ['Markdown 文档', 'CSV 表格'],
    outputs: ['网页 URL', '数据文件'],
    settings: {
      输出目录: 'data/output/web-reports',
      网页模板: '',
      模板原文件名: '',
      模板预览: '',
      输出模式: '输出网页 + 数据',
    },
  },
  stop: {
    label: '结束',
    group: '结束',
    role: 'system',
    icon: StopCircleIcon,
    accent: 'bg-slate-500/12 text-slate-700 dark:text-slate-300',
    description: '记录本轮任务的完成状态并结束流程。',
    inputs: ['执行结果'],
    outputs: ['结束状态'],
    settings: {
      结束方式: '本轮完成后停止',
    },
  },
}

const COMPONENT_VISUAL_META: Record<
  string,
  {
    label: string
    group: AgentPaletteGroup
    icon: LucideIcon
    accent: string
  }
> = {
  'component.webhook': {
    label: 'Webhook 触发',
    group: '触发',
    icon: WebhookIcon,
    accent: 'bg-cyan-500/12 text-cyan-700 dark:text-cyan-300',
  },
  'component.branch': {
    label: '条件分支',
    group: '处理',
    icon: ChartNetworkIcon,
    accent: 'bg-emerald-500/12 text-emerald-700 dark:text-emerald-300',
  },
  'component.switch': {
    label: '多路判断',
    group: '处理',
    icon: MergeIcon,
    accent: 'bg-sky-500/12 text-sky-700 dark:text-sky-300',
  },
  'component.set_variable': {
    label: '设置变量',
    group: '处理',
    icon: CpuIcon,
    accent: 'bg-violet-500/12 text-violet-700 dark:text-violet-300',
  },
  'component.subflow': {
    label: '调用子蓝图',
    group: '处理',
    icon: ChartNetworkIcon,
    accent: 'bg-pink-500/12 text-pink-700 dark:text-pink-300',
  },
  'component.wait': {
    label: '等待延迟',
    group: '处理',
    icon: ClockIcon,
    accent: 'bg-amber-500/12 text-amber-700 dark:text-amber-300',
  },
  'component.loop': {
    label: '列表循环',
    group: '处理',
    icon: RotateCcwIcon,
    accent: 'bg-indigo-500/12 text-indigo-700 dark:text-indigo-300',
  },
  'component.loop_collect': {
    label: '汇总循环',
    group: '处理',
    icon: RotateCcwIcon,
    accent: 'bg-purple-500/12 text-purple-700 dark:text-purple-300',
  },
  'component.filter': {
    label: '条件过滤',
    group: '处理',
    icon: FilterIcon,
    accent: 'bg-lime-500/12 text-lime-700 dark:text-lime-300',
  },
  'component.merge': {
    label: '合并数据',
    group: '处理',
    icon: MergeIcon,
    accent: 'bg-blue-500/12 text-blue-700 dark:text-blue-300',
  },
  'component.dedupe_sort': {
    label: '去重排序',
    group: '处理',
    icon: Table2Icon,
    accent: 'bg-teal-500/12 text-teal-700 dark:text-teal-300',
  },
  'component.database': {
    label: '数据库查询',
    group: '采集',
    icon: CpuIcon,
    accent: 'bg-slate-500/12 text-slate-700 dark:text-slate-300',
  },
  'component.file': {
    label: '读写文件',
    group: '处理',
    icon: FileSpreadsheetIcon,
    accent: 'bg-orange-500/12 text-orange-700 dark:text-orange-300',
  },
  'component.xianyu-reply': {
    label: '闲鱼实时智能客服',
    group: '处理',
    icon: MessagesSquareIcon,
    accent: 'bg-emerald-500/12 text-emerald-700 dark:text-emerald-300',
  },
  'component.xianyu-review': {
    label: '闲鱼自动评价',
    group: '处理',
    icon: MessagesSquareIcon,
    accent: 'bg-amber-500/12 text-amber-700 dark:text-amber-300',
  },
}

function getAgentNodeVisualMeta(kind: AgentNodeKind, componentId?: string) {
  return componentId
    ? (COMPONENT_VISUAL_META[componentId] ?? NODE_META[kind])
    : NODE_META[kind]
}

const COMPONENT_KIND_ALIASES: Record<string, AgentNodeKind> = {
  'component.xianyu': 'xianyu',
  'component.xianyu-reply': 'xianyu_reply',
  'component.xianyu-review': 'xianyu_review',
}

const PALETTE_GROUPS: Array<{
  title: AgentPaletteGroup
  kinds: AgentNodeKind[]
}> = [
  { title: '触发', kinds: ['start', 'schedule'] },
  { title: '采集', kinds: ['browser', 'xianyu', 'request'] },
  { title: '处理', kinds: ['xianyu_reply', 'xianyu_review', 'transform', 'image'] },
  { title: '落档', kinds: ['report', 'webpage', 'notify'] },
  { title: '结束', kinds: ['stop'] },
]

const WORKFLOW_STAGE_ORDER: AgentPaletteGroup[] = [
  '触发',
  '采集',
  '处理',
  '落档',
  '结束',
]

const REQUIRED_WORKFLOW_STAGES: AgentPaletteGroup[] = [
  '触发',
  '采集',
  '落档',
  '结束',
]

const COMPOSITE_NODE_STAGE_COVERAGE: Partial<
  Record<AgentNodeKind, AgentPaletteGroup[]>
> = {
  xianyu: ['处理', '落档'],
  xianyu_reply: ['采集', '落档'],
  xianyu_review: ['采集', '落档'],
}

const COMPOSITE_COMPONENT_STAGE_COVERAGE: Record<string, AgentPaletteGroup[]> =
  {
    'component.xianyu': ['采集', '处理', '落档'],
    'component.xianyu-reply': ['采集', '处理', '落档'],
    'component.xianyu-review': ['采集', '处理', '落档'],
  }

function workflowStagesForNode(node: AgentFlowNode) {
  const primaryStage = getAgentNodeVisualMeta(
    node.data.kind,
    node.data.componentId
  ).group
  return Array.from(
    new Set([
      primaryStage,
      ...(COMPOSITE_NODE_STAGE_COVERAGE[node.data.kind] ?? []),
      ...(COMPOSITE_COMPONENT_STAGE_COVERAGE[node.data.componentId ?? ''] ??
        []),
    ])
  )
}

function validateWorkflowStructure(
  nodes: AgentFlowNode[],
  edges: AgentFlowEdge[]
): WorkflowReadiness {
  const nodeById = new Map(nodes.map((node) => [node.id, node]))
  const stagesByNodeId = new Map(
    nodes.map((node) => [node.id, workflowStagesForNode(node)])
  )
  const validEdges = edges.filter(
    (edge) => nodeById.has(edge.source) && nodeById.has(edge.target)
  )
  const outgoing = new Map<string, string[]>()
  const incoming = new Map<string, string[]>()

  for (const edge of validEdges) {
    outgoing.set(edge.source, [
      ...(outgoing.get(edge.source) ?? []),
      edge.target,
    ])
    incoming.set(edge.target, [
      ...(incoming.get(edge.target) ?? []),
      edge.source,
    ])
  }

  const walk = (starts: string[], graph: Map<string, string[]>) => {
    const visited = new Set<string>()
    const queue = [...starts]
    while (queue.length > 0) {
      const nodeId = queue.shift()
      if (!nodeId || visited.has(nodeId)) continue
      visited.add(nodeId)
      queue.push(...(graph.get(nodeId) ?? []))
    }
    return visited
  }

  const triggerIds = nodes
    .filter((node) => stagesByNodeId.get(node.id)?.includes('触发'))
    .map((node) => node.id)
  const endIds = nodes
    .filter((node) => stagesByNodeId.get(node.id)?.includes('结束'))
    .map((node) => node.id)
  const reachableFromTrigger = walk(triggerIds, outgoing)
  const canReachEnd = walk(endIds, incoming)
  const issues: WorkflowValidationIssue[] = []

  for (const stage of REQUIRED_WORKFLOW_STAGES) {
    if (!nodes.some((node) => stagesByNodeId.get(node.id)?.includes(stage))) {
      issues.push({ message: `请添加“${stage}”阶段的步骤。` })
    }
  }

  if (validEdges.length !== edges.length) {
    issues.push({ message: '存在指向已删除步骤的连线，请重新连接流程。' })
  }

  if (triggerIds.length > 0) {
    for (const node of nodes) {
      if (!reachableFromTrigger.has(node.id)) {
        issues.push({
          nodeId: node.id,
          message: `“${node.data.title}”没有接入触发路径。`,
        })
      }
    }
  }

  if (endIds.length > 0) {
    for (const node of nodes) {
      if (!canReachEnd.has(node.id)) {
        issues.push({
          nodeId: node.id,
          message: `“${node.data.title}”还没有连接到结束步骤。`,
        })
      }
    }
  }

  for (const node of nodes) {
    if (
      (node.data.kind === 'browser' ||
        node.data.kind === 'xianyu' ||
        node.data.kind === 'xianyu_reply' ||
        node.data.kind === 'request') &&
      !node.data.settings['网址']?.trim()
    ) {
      issues.push({
        nodeId: node.id,
        message: `请为“${node.data.title}”填写网址。`,
      })
    }
    if (
      node.data.kind === 'notify' &&
      !node.data.settings['通知目标']?.trim() &&
      !node.data.settings['Webhook']?.trim()
    ) {
      issues.push({
        nodeId: node.id,
        message: `请为“${node.data.title}”填写通知目标。`,
      })
    }
  }

  const nodeIdsWithIssues = new Set(
    issues.flatMap((issue) => (issue.nodeId ? [issue.nodeId] : []))
  )
  const stages = WORKFLOW_STAGE_ORDER.map((label) => {
    const stageNodes = nodes.filter((node) =>
      stagesByNodeId.get(node.id)?.includes(label)
    )
    return {
      label,
      required: REQUIRED_WORKFLOW_STAGES.includes(label),
      present: stageNodes.length > 0,
      complete: stageNodes.some(
        (node) =>
          reachableFromTrigger.has(node.id) &&
          canReachEnd.has(node.id) &&
          !nodeIdsWithIssues.has(node.id)
      ),
    }
  })

  return {
    ready: issues.length === 0,
    issues,
    stages,
  }
}

type PaletteTemplateGroup = {
  title: AgentPaletteGroup
  templates: NodeTemplate[]
}

type ComponentPaletteSection = {
  title: string
  badge: string
  templates: NodeTemplate[]
}

const INITIAL_EDGES: AgentFlowEdge[] = [
  { id: 'e-start-collect', source: 'start', target: 'collect' },
  { id: 'e-collect-process', source: 'collect', target: 'process' },
  { id: 'e-process-archive', source: 'process', target: 'archive' },
  { id: 'e-archive-stop', source: 'archive', target: 'stop' },
]

export function AgentWorkspace({
  models,
  groups = [],
  currentModel,
  currentGroup,
  seedPrompt,
}: AgentWorkspaceProps) {
  const [managedSkills, setManagedSkills] =
    useState<SkillChoice[]>(readSavedSkills)
  const [marketplaceSkills, setMarketplaceSkills] = useState<SkillChoice[]>([])
  const [isSkillCatalogLoading, setIsSkillCatalogLoading] = useState(false)
  const [componentCatalog, setComponentCatalog] = useState<
    AgentComponentItem[]
  >([])
  const [isComponentCatalogLoading, setIsComponentCatalogLoading] =
    useState(false)
  const skillChoices = useMemo(
    () => mergeSkillChoices(BASE_SKILL_CHOICES, managedSkills),
    [managedSkills]
  )
  const modelChoices = useMemo(() => buildModelChoices(models), [models])
  const groupChoices = useMemo(
    () => buildGroupChoices(groups, currentGroup),
    [currentGroup, groups]
  )
  const templates = useMemo(
    () => buildTemplates(models, currentModel, currentGroup, componentCatalog),
    [componentCatalog, currentGroup, currentModel, models]
  )
  const initialBlueprints = useMemo(() => readSavedBlueprints(), [])
  const initialBlueprint = initialBlueprints[0] ?? null
  const templateByKind = useMemo(
    () => new Map(templates.map((template) => [template.kind, template])),
    [templates]
  )
  const templateById = useMemo(
    () =>
      new Map(
        templates.map((template) => [
          template.componentId ?? template.kind,
          template,
        ])
      ),
    [templates]
  )
  const [blueprints, setBlueprints] =
    useState<SavedAgentBlueprint[]>(initialBlueprints)
  const [currentBlueprintId, setCurrentBlueprintId] = useState<string | null>(
    initialBlueprint?.id ?? null
  )
  const [blueprintTitle, setBlueprintTitle] = useState(
    initialBlueprint?.title ?? '自动化任务蓝图'
  )
  const [knowledgeScope, setKnowledgeScope] = useState<KnowledgeScopeValue>(
    initialBlueprint?.knowledgeScope ?? {}
  )
  const [lastSavedAt, setLastSavedAt] = useState<number | null>(
    initialBlueprint && !isLocalBlueprintDraft(initialBlueprint)
      ? initialBlueprint.updatedAt
      : null
  )
  const [nodes, setNodes] = useState<AgentFlowNode[]>(
    () => initialBlueprint?.nodes ?? createInitialNodes(templates)
  )
  const [edges, setEdges] = useState<AgentFlowEdge[]>(
    () =>
      initialBlueprint?.edges ?? INITIAL_EDGES.map((edge) => decorateEdge(edge))
  )
  const [executionLog, setExecutionLog] = useState<AgentExecutionLogMap>(
    () => initialBlueprint?.executionLog ?? {}
  )
  const [backendExecutionLogs, setBackendExecutionLogs] = useState<
    AgentExecutionLog[]
  >([])
  const [replyRecords, setReplyRecords] = useState<AgentXianyuReplyRecord[]>([])
  const [replyRecordsOpen, setReplyRecordsOpen] = useState(false)
  const [replyRecordsLoading, setReplyRecordsLoading] = useState(false)
  const [selectedNodeId, setSelectedNodeId] = useState<string>(
    initialBlueprint?.nodes[0]?.id ?? 'collect'
  )
  const [search, setSearch] = useState('')
  const addCounterRef = useRef(0)
  const lastSeedPromptRef = useRef<string | null>(null)
  const previewRunIdRef = useRef(0)
  const undoStackRef = useRef<AgentBlueprintHistorySnapshot[]>([])
  const redoStackRef = useRef<AgentBlueprintHistorySnapshot[]>([])
  const [, setHistoryVersion] = useState(0)

  const selectedNode = useMemo(
    () => nodes.find((node) => node.id === selectedNodeId) ?? null,
    [nodes, selectedNodeId]
  )

  const currentReplyBackendIds = useMemo(() => {
    const blueprint = blueprints.find((item) => item.id === currentBlueprintId)
    return {
      agentId: blueprint?.backendAgentId ?? '',
      blueprintId: blueprint?.backendBlueprintId ?? '',
    }
  }, [blueprints, currentBlueprintId])

  const refreshReplyRecords = useCallback(async () => {
    const { agentId, blueprintId } = currentReplyBackendIds
    if (!agentId || !blueprintId) {
      setReplyRecords([])
      return
    }
    setReplyRecordsLoading(true)
    try {
      setReplyRecords(await listXianyuReplyRecords(agentId, blueprintId, 100))
    } catch {
      setReplyRecords([])
    } finally {
      setReplyRecordsLoading(false)
    }
  }, [currentReplyBackendIds])

  useEffect(() => {
    if (!replyRecordsOpen) return
    void refreshReplyRecords()
    const timer = window.setInterval(() => void refreshReplyRecords(), 5000)
    return () => window.clearInterval(timer)
  }, [refreshReplyRecords, replyRecordsOpen])

  useEffect(() => {
    if (models.length === 0) return
    const availableModels = new Set(models.map((model) => model.value))

    setNodes((current) => {
      let changed = false
      const next = current.map((node) => {
        const nodeGroup = node.data.group || currentGroup
        if (currentGroup && nodeGroup !== currentGroup) return node
        if (
          node.data.model &&
          !node.data.model.startsWith('system.') &&
          availableModels.has(node.data.model)
        ) {
          return node
        }

        const model = pickDefaultModel(node.data.kind, models, currentModel)
        if (model === node.data.model) return node
        changed = true
        return { ...node, data: { ...node.data, model } }
      })
      return changed ? next : current
    })
  }, [currentGroup, currentModel, models])

  const executionNodes = useMemo(() => orderNodes(nodes, edges), [edges, nodes])
  const currentBlueprintStatus = useMemo(
    () =>
      getCurrentBlueprintStatus(
        nodes,
        currentBlueprintId
          ? blueprints.find((blueprint) => blueprint.id === currentBlueprintId)
              ?.status
          : undefined
      ),
    [blueprints, currentBlueprintId, nodes]
  )
  const hasScheduledBlueprint = useMemo(() => hasScheduleNode(nodes), [nodes])
  const hasManualTrigger = useMemo(() => hasManualTriggerNode(nodes), [nodes])
  const paletteGroups = useMemo(
    () => buildPaletteTemplateGroups(templates),
    [templates]
  )
  const filteredPaletteGroups = useMemo(
    () => filterPaletteTemplateGroups(paletteGroups, search),
    [paletteGroups, search]
  )
  const canUndoBlueprint = undoStackRef.current.length > 0
  const canRedoBlueprint = redoStackRef.current.length > 0
  const cancelPreviewRun = useCallback(() => {
    previewRunIdRef.current += 1
  }, [])
  const createHistorySnapshot = useCallback(
    (): AgentBlueprintHistorySnapshot => ({
      title: blueprintTitle,
      nodes,
      edges,
      executionLog,
      selectedNodeId,
    }),
    [blueprintTitle, edges, executionLog, nodes, selectedNodeId]
  )
  const bumpHistoryVersion = useCallback(
    () => setHistoryVersion((version) => version + 1),
    []
  )
  const recordHistorySnapshot = useCallback(() => {
    undoStackRef.current.push(createHistorySnapshot())
    if (undoStackRef.current.length > MAX_BLUEPRINT_HISTORY) {
      undoStackRef.current.shift()
    }
    redoStackRef.current = []
    bumpHistoryVersion()
  }, [bumpHistoryVersion, createHistorySnapshot])
  const refreshBackendExecutionLogs = useCallback(
    async (
      agentId: string,
      blueprintId: string,
      shouldApply: () => boolean = () => true
    ) => {
      const logs = await listBlueprintExecutionLogs(agentId, blueprintId, 5)
      if (shouldApply()) {
        setBackendExecutionLogs(logs)
      }
      return logs
    },
    []
  )
  const pollBackendExecutionLogs = useCallback(
    async (
      agentId: string,
      blueprintId: string,
      isActive: () => boolean,
      onProgress?: (log: AgentExecutionLog) => void
    ) => {
      const deadline = Date.now() + AGENT_BACKEND_POLL_TIMEOUT_MS
      let logs: AgentExecutionLog[] = []

      while (isActive() && Date.now() < deadline) {
        logs = await refreshBackendExecutionLogs(agentId, blueprintId, isActive)
        const latest = logs[0]
        if (latest) onProgress?.(latest)
        if (
          latest &&
          latest.status !== 'running' &&
          latest.status !== 'pending'
        ) {
          return logs
        }
        await waitForAgentBackendPoll(AGENT_BACKEND_POLL_INTERVAL_MS)
      }

      return logs
    },
    [refreshBackendExecutionLogs]
  )
  const restoreHistorySnapshot = useCallback(
    (snapshot: AgentBlueprintHistorySnapshot) => {
      cancelPreviewRun()
      setBlueprintTitle(snapshot.title)
      setNodes(snapshot.nodes)
      setEdges(snapshot.edges)
      setExecutionLog(snapshot.executionLog)
      setSelectedNodeId(snapshot.selectedNodeId)
    },
    [cancelPreviewRun]
  )
  const handleUndoBlueprint = useCallback(() => {
    const snapshot = undoStackRef.current.pop()
    if (!snapshot) return
    redoStackRef.current.push(createHistorySnapshot())
    restoreHistorySnapshot(snapshot)
    bumpHistoryVersion()
  }, [bumpHistoryVersion, createHistorySnapshot, restoreHistorySnapshot])
  const handleRedoBlueprint = useCallback(() => {
    const snapshot = redoStackRef.current.pop()
    if (!snapshot) return
    undoStackRef.current.push(createHistorySnapshot())
    restoreHistorySnapshot(snapshot)
    bumpHistoryVersion()
  }, [bumpHistoryVersion, createHistorySnapshot, restoreHistorySnapshot])
  const persistCurrentBlueprintSnapshot = useCallback(
    (
      nextNodes: AgentFlowNode[],
      nextExecutionLog: AgentExecutionLogMap,
      statusOverride?: AgentBlueprintStatus,
      nextEdges?: AgentFlowEdge[]
    ) => {
      if (!currentBlueprintId) return

      const now = Date.now()
      setLastSavedAt(now)
      setBlueprints((current) => {
        const nextBlueprints = current.map((blueprint) =>
          blueprint.id === currentBlueprintId
            ? {
                ...blueprint,
                nodes: nextNodes,
                edges: nextEdges ?? blueprint.edges,
                updatedAt: now,
                status: statusOverride ?? getBlueprintStatus(nextNodes),
                executionLog: nextExecutionLog,
                knowledgeScope,
              }
            : blueprint
        )
        writeSavedBlueprints(nextBlueprints)
        notifyAgentBlueprintsChanged()
        return nextBlueprints
      })
    },
    [currentBlueprintId, knowledgeScope]
  )

  const applyBackendExecutionLogToPreview = useCallback(
    (
      log: AgentExecutionLog,
      activeNodes: AgentFlowNode[],
      activeEdges: AgentFlowEdge[],
      statusOverride?: AgentBlueprintStatus,
      snapshotEdges: AgentFlowEdge[] = activeEdges,
      persistSnapshot = true
    ) => {
      const preview = buildBackendExecutionPreview(
        log,
        activeNodes,
        activeEdges
      )
      if (!preview) {
        const activeNodeIds = new Set(activeNodes.map((node) => node.id))
        const fallbackExecutionLog: AgentExecutionLogMap = {}
        setExecutionLog(fallbackExecutionLog)
        setNodes((current) => {
          const nextNodes = current.map((node) =>
            activeNodeIds.has(node.id) && node.data.status === 'running'
              ? {
                  ...node,
                  data: {
                    ...node.data,
                    status: 'waiting' as AgentNodeStatus,
                  },
                }
              : node
          )
          if (persistSnapshot) {
            persistCurrentBlueprintSnapshot(
              stripTransientNodeSettings(nextNodes),
              fallbackExecutionLog,
              statusOverride ??
                backendBlueprintStatus(
                  log.status,
                  new Map<string, AgentNodeStatus>()
                ),
              snapshotEdges
            )
          }
          return nextNodes
        })
        return
      }

      setExecutionLog(preview.executionLog)
      setNodes((current) => {
        const statusById = preview.statusById
        const activeById = new Map(activeNodes.map((node) => [node.id, node]))
        const nextNodes = current.map((node) => {
          const activeNode = activeById.get(node.id)
          const status = statusById.get(node.id)
          if (!activeNode && !status) return node
          return {
            ...(activeNode ?? node),
            data: {
              ...(activeNode?.data ?? node.data),
              status: status ?? activeNode?.data.status ?? node.data.status,
            },
          }
        })
        if (persistSnapshot) {
          persistCurrentBlueprintSnapshot(
            stripTransientNodeSettings(nextNodes),
            preview.executionLog,
            statusOverride ?? preview.blueprintStatus,
            snapshotEdges
          )
        }
        return nextNodes
      })
    },
    [persistCurrentBlueprintSnapshot]
  )

  useEffect(() => {
    return () => {
      cancelPreviewRun()
    }
  }, [cancelPreviewRun])

  const onNodesChange = useCallback(
    (changes: NodeChange<AgentFlowNode>[]) => {
      const removedIds = new Set(
        changes
          .filter((change) => change.type === 'remove')
          .map((change) => change.id)
      )
      if (removedIds.size > 0) {
        recordHistorySnapshot()
      }
      setNodes((current) => applyNodeChanges(changes, current))
      if (removedIds.size > 0) {
        setEdges((current) =>
          current.filter(
            (edge) =>
              !removedIds.has(edge.source) && !removedIds.has(edge.target)
          )
        )
        setExecutionLog((current) => {
          const next = { ...current }
          removedIds.forEach((id) => {
            delete next[id]
          })
          return next
        })
        setSelectedNodeId((current) => (removedIds.has(current) ? '' : current))
      }
    },
    [recordHistorySnapshot]
  )

  const onEdgesChange = useCallback(
    (changes: EdgeChange<AgentFlowEdge>[]) => {
      if (changes.some((change) => change.type === 'remove')) {
        recordHistorySnapshot()
      }
      setEdges((current) => applyEdgeChanges(changes, current))
    },
    [recordHistorySnapshot]
  )

  const onConnect = useCallback(
    (connection: Connection) => {
      if (!isAgentConnectionValid(nodes, edges, connection)) {
        toast.error('不能形成循环连线，请使用列表循环步骤。')
        return
      }
      recordHistorySnapshot()
      const sourceNode = nodes.find((node) => node.id === connection.source)
      const routePort = sourceNode
        ? getAgentRoutePorts(sourceNode.data).find(
            (port) => port.id === connection.sourceHandle
          )
        : undefined
      setEdges((current) =>
        addEdge(
          decorateEdge({
            ...connection,
            id: `e-${Date.now()}`,
            data: routePort ? { routeLabel: routePort.label } : undefined,
          }),
          current
        )
      )
    },
    [edges, nodes, recordHistorySnapshot]
  )

  useEffect(() => {
    const nodeById = new Map(nodes.map((node) => [node.id, node]))
    setEdges((current) => {
      let changed = false
      const next = current.map((edge) => {
        const sourceNode = nodeById.get(edge.source)
        const routePort = sourceNode
          ? getAgentRoutePorts(sourceNode.data).find(
              (port) => port.id === edge.sourceHandle
            )
          : undefined
        if (!routePort || edge.data?.routeLabel === routePort.label) {
          return edge
        }
        changed = true
        return decorateEdge({
          ...edge,
          data: { ...edge.data, routeLabel: routePort.label },
        })
      })
      return changed ? next : current
    })
  }, [nodes])

  const handleDeleteNode = useCallback(
    (nodeId: string) => {
      recordHistorySnapshot()
      const fallbackNodeId = nodes.find((node) => node.id !== nodeId)?.id ?? ''
      setNodes((current) => current.filter((node) => node.id !== nodeId))
      setEdges((current) =>
        current.filter(
          (edge) => edge.source !== nodeId && edge.target !== nodeId
        )
      )
      setExecutionLog((current) => {
        if (!current[nodeId]) return current
        const next = { ...current }
        delete next[nodeId]
        return next
      })
      setSelectedNodeId((current) =>
        current === nodeId ? fallbackNodeId : current
      )
    },
    [nodes, recordHistorySnapshot]
  )

  const handleDeleteEdge = useCallback(
    (edgeId: string) => {
      recordHistorySnapshot()
      setEdges((current) => current.filter((edge) => edge.id !== edgeId))
    },
    [recordHistorySnapshot]
  )

  const handleReset = useCallback(() => {
    cancelPreviewRun()
    recordHistorySnapshot()
    setNodes([])
    setEdges([])
    setExecutionLog({})
    setBackendExecutionLogs([])
    setSelectedNodeId('')
  }, [cancelPreviewRun, recordHistorySnapshot])

  const handleNewBlueprint = useCallback(() => {
    cancelPreviewRun()
    undoStackRef.current = []
    redoStackRef.current = []
    bumpHistoryVersion()
    setBlueprintTitle('新的智能体蓝图')
    setCurrentBlueprintId(null)
    setKnowledgeScope({})
    setLastSavedAt(null)
    setNodes(createInitialNodes(templates))
    setEdges(INITIAL_EDGES.map((edge) => decorateEdge(edge)))
    setExecutionLog({})
    setBackendExecutionLogs([])
    setSelectedNodeId('collect')
  }, [bumpHistoryVersion, cancelPreviewRun, templates])

  const handleSaveBlueprint = useCallback(async () => {
    const readiness = validateWorkflowStructure(nodes, edges)
    if (!readiness.ready) {
      const issue = readiness.issues[0]
      if (issue?.nodeId) setSelectedNodeId(issue.nodeId)
      toast.error('流程尚未闭环', {
        description: issue?.message ?? '请补全必需阶段后再保存。',
      })
      return
    }

    try {
      await validateAgentNodeModels(nodes)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '节点模型校验失败')
      return
    }

    const now = Date.now()
    const title = blueprintTitle.trim() || '未命名蓝图'
    const id = currentBlueprintId ?? `blueprint-${now}`
    const existing = blueprints.find((blueprint) => blueprint.id === id)
    const blueprint: SavedAgentBlueprint = {
      id,
      title,
      nodes,
      edges,
      createdAt: existing?.createdAt ?? now,
      updatedAt: now,
      status: currentBlueprintStatus,
      executionLog,
      backendAgentId: existing?.backendAgentId,
      backendBlueprintId: existing?.backendBlueprintId,
      syncState: 'draft',
      knowledgeScope,
    }

    setBlueprintTitle(title)
    setCurrentBlueprintId(id)

    try {
      const synced = await syncSavedBlueprintToBackend(blueprint, {
        modelsValidated: true,
      })
      if (!synced) throw new Error('后端未返回蓝图数据')

      const next = [
        synced,
        ...blueprints.filter((item) => item.id !== id),
      ].sort((a, b) => b.updatedAt - a.updatedAt)
      setLastSavedAt(synced.updatedAt)
      setBlueprints(next)
      writeSavedBlueprints(next)
      notifyAgentBlueprintsChanged()

      if (synced.backendAgentId && synced.backendBlueprintId) {
        void refreshBackendExecutionLogs(
          synced.backendAgentId,
          synced.backendBlueprintId
        ).catch(() => undefined)
      }
    } catch (error) {
      const drafts = [
        blueprint,
        ...blueprints.filter((item) => item.id !== id),
      ].sort((a, b) => b.updatedAt - a.updatedAt)
      setLastSavedAt(null)
      setBlueprints(drafts)
      writeSavedBlueprints(drafts)
      notifyAgentBlueprintsChanged()
      toast.error(
        error instanceof Error
          ? `数据库保存失败，已保留本地草稿: ${error.message}`
          : '数据库保存失败，已保留本地草稿'
      )
    }
  }, [
    blueprintTitle,
    blueprints,
    currentBlueprintStatus,
    currentBlueprintId,
    edges,
    executionLog,
    nodes,
    knowledgeScope,
    refreshBackendExecutionLogs,
  ])

  const handleLoadBlueprint = useCallback(
    (blueprint: SavedAgentBlueprint) => {
      const normalizedNodes = normalizeSavedBlueprintNodes(
        blueprint.nodes.map((node) => normalizeNode(node)),
        blueprint.status
      )
      cancelPreviewRun()
      undoStackRef.current = []
      redoStackRef.current = []
      bumpHistoryVersion()
      setCurrentBlueprintId(blueprint.id)
      setBlueprintTitle(blueprint.title)
      setKnowledgeScope(blueprint.knowledgeScope ?? {})
      setLastSavedAt(
        isLocalBlueprintDraft(blueprint) ? null : blueprint.updatedAt
      )
      setNodes(normalizedNodes)
      setEdges(blueprint.edges)
      setExecutionLog(
        executionLogForBlueprintStatus(blueprint.executionLog, blueprint.status)
      )
      setBackendExecutionLogs([])
      if (blueprint.backendAgentId && blueprint.backendBlueprintId) {
        const requestId = previewRunIdRef.current
        void refreshBackendExecutionLogs(
          blueprint.backendAgentId,
          blueprint.backendBlueprintId,
          () => previewRunIdRef.current === requestId
        ).catch(() => undefined)
      }
      setSelectedNodeId(normalizedNodes[0]?.id ?? '')
    },
    [bumpHistoryVersion, cancelPreviewRun, refreshBackendExecutionLogs]
  )

  useEffect(() => {
    let active = true

    const hydrateBlueprints = async () => {
      try {
        const agent = await ensureCreatorStudioAgent()
        const backendBlueprints = await getBlueprintsByAgent(agent.id)
        if (!active) return

        const restored = mergeBackendBlueprints(
          readSavedBlueprints(),
          backendBlueprints,
          agent.id
        )
        setBlueprints(restored)
        writeSavedBlueprints(restored)
        notifyAgentBlueprintsChanged()

        const selected =
          restored.find((item) => item.id === initialBlueprint?.id) ??
          restored[0]
        if (selected) {
          handleLoadBlueprint(selected)
        }
      } catch (error) {
        console.warn('Failed to restore creator studio blueprints', error)
        toast.error('从数据库加载蓝图失败，已使用本地缓存')
      }
    }

    void hydrateBlueprints()
    return () => {
      active = false
    }
  }, [handleLoadBlueprint, initialBlueprint?.id])

  const reloadComponents = useCallback(async () => {
    setIsComponentCatalogLoading(true)
    try {
      const components = await listAgentComponents()
      setComponentCatalog(components)
    } catch (error) {
      console.warn('Failed to load agent components', error)
    } finally {
      setIsComponentCatalogLoading(false)
    }
  }, [])

  useEffect(() => {
    void reloadComponents()
  }, [reloadComponents])

  const reloadSkills = useCallback(async () => {
    setIsSkillCatalogLoading(true)
    try {
      const [skills, marketplace] = await Promise.all([
        listAgentSkills(),
        listAgentSkillMarketplace(),
      ])
      const installed = skills
        .map(skillChoiceFromAgentSkill)
        .filter((skill): skill is SkillChoice => Boolean(skill))
      const marketplaceChoices = marketplace.map(skillChoiceFromMarketplaceItem)
      setMarketplaceSkills(marketplaceChoices)
      setManagedSkills((current) => {
        const backendValues = new Set(installed.map((skill) => skill.value))
        const backendMarketplaceIds = new Set(
          installed
            .map((skill) => skill.marketplaceId)
            .filter((value): value is string => Boolean(value))
        )
        const localOnly = current.filter(
          (skill) =>
            !backendValues.has(skill.value) &&
            (!skill.marketplaceId ||
              !backendMarketplaceIds.has(skill.marketplaceId)) &&
            !(
              skill.reference &&
              installed.some((item) => item.reference === skill.reference)
            )
        )
        const next = mergeSkillChoices(installed, localOnly).filter(
          (skill) => skill.source !== 'built-in'
        )
        writeSavedSkills(next)
        return next
      })
    } catch (error) {
      console.warn('Failed to load agent skills', error)
    } finally {
      setIsSkillCatalogLoading(false)
    }
  }, [])

  useEffect(() => {
    void reloadSkills()
  }, [reloadSkills])

  const handleDeleteBlueprint = useCallback(
    async (blueprintId: string) => {
      const blueprint = blueprints.find((item) => item.id === blueprintId)
      if (blueprint?.backendAgentId && blueprint.backendBlueprintId) {
        try {
          const backendBlueprints = await getBlueprintsByAgent(
            blueprint.backendAgentId
          )
          const matchingIds = new Set<string>([blueprint.backendBlueprintId])
          backendBlueprints.forEach((item) => {
            if (backendBlueprintLocalId(item) === blueprint.id) {
              matchingIds.add(item.id)
            }
          })
          await Promise.all(
            Array.from(matchingIds).map((id) =>
              deleteBlueprint(blueprint.backendAgentId!, id)
            )
          )
        } catch (error) {
          toast.error(
            error instanceof Error
              ? `删除数据库蓝图失败: ${error.message}`
              : '删除数据库蓝图失败'
          )
          return
        }
      }

      const next = blueprints.filter((item) => item.id !== blueprintId)
      setBlueprints(next)
      writeSavedBlueprints(next)
      notifyAgentBlueprintsChanged()

      if (currentBlueprintId !== blueprintId) return

      cancelPreviewRun()
      const fallback = next[0]
      if (fallback) {
        handleLoadBlueprint(fallback)
        return
      }

      setCurrentBlueprintId(null)
      setLastSavedAt(null)
      setBlueprintTitle('新的智能体蓝图')
      setNodes(createInitialNodes(templates))
      setEdges(INITIAL_EDGES.map((edge) => decorateEdge(edge)))
      setExecutionLog({})
      setSelectedNodeId('collect')
    },
    [
      blueprints,
      cancelPreviewRun,
      currentBlueprintId,
      handleLoadBlueprint,
      templates,
    ]
  )

  const handleRunPreview = useCallback(
    (mode: AgentPreviewRunMode = 'default') => {
      const isManualPreview = mode === 'manual'
      const readiness = validateWorkflowStructure(nodes, edges)
      if (!readiness.ready) {
        const issue = readiness.issues[0]
        if (issue?.nodeId) setSelectedNodeId(issue.nodeId)
        toast.error('流程还不能运行', {
          description: issue?.message ?? '请补全必需阶段后再运行。',
        })
        return
      }
      if (
        nodes.some(
          (node) =>
            node.data.kind === 'xianyu' || node.data.kind === 'xianyu_reply'
        ) &&
        (knowledgeScope.knowledgeBaseIds?.length ?? 0) === 0
      ) {
        toast.error('请先选择至少一个知识库')
        return
      }
      if (
        (isManualPreview || !hasScheduledBlueprint) &&
        !hasManualTriggerNode(nodes)
      ) {
        return
      }

      if (currentBlueprintStatus === 'paused' && !isManualPreview) {
        const currentBlueprint = blueprints.find(
          (item) => item.id === currentBlueprintId
        )
        if (
          currentBlueprint?.backendAgentId &&
          currentBlueprint.backendBlueprintId
        ) {
          void resumeBlueprint(
            currentBlueprint.backendAgentId,
            currentBlueprint.backendBlueprintId
          )
            .then((backendBlueprint) => {
              setBlueprints((current) => {
                const next = current.map((item) =>
                  item.id === currentBlueprint.id
                    ? {
                        ...item,
                        status: localBlueprintStatusFromBackend(
                          backendBlueprint.status
                        ),
                      }
                    : item
                )
                writeSavedBlueprints(next)
                notifyAgentBlueprintsChanged()
                return next
              })
              persistCurrentBlueprintSnapshot(nodes, executionLog, 'running')
              toast.success('定时蓝图已继续运行')
            })
            .catch((error) => {
              toast.error(
                error instanceof Error
                  ? `继续失败: ${error.message}`
                  : '继续失败'
              )
            })
          return
        }
        persistCurrentBlueprintSnapshot(nodes, executionLog, 'running')
        toast.success('定时蓝图已继续运行')
        return
      }

      const prepared = ensureTerminalStopNode(nodes, edges, templateByKind)
      const runGraph = isManualPreview
        ? buildManualPreviewGraph(prepared.nodes, prepared.edges)
        : { nodes: prepared.nodes, edges: prepared.edges }
      const runExecutionNodes = orderNodes(runGraph.nodes, runGraph.edges)
      if (runExecutionNodes.length === 0) return
      const realtimeListenerIndex = runExecutionNodes.findIndex(
        (node) => node.data.kind === 'xianyu_reply'
      )

      cancelPreviewRun()
      setBackendExecutionLogs([])
      const runId = previewRunIdRef.current
      const statusAfterManualPreview: AgentBlueprintStatus | undefined =
        isManualPreview && hasScheduledBlueprint
          ? currentBlueprintStatus === 'paused'
            ? 'paused'
            : 'running'
          : undefined

      if (prepared.changed) {
        setEdges(prepared.edges)
      }

      const backendBlueprint: SavedAgentBlueprint = {
        id: currentBlueprintId ?? `blueprint-${Date.now()}`,
        title: blueprintTitle.trim() || '未命名蓝图',
        nodes: stripTransientNodeSettings(runGraph.nodes),
        edges: runGraph.edges,
        createdAt:
          blueprints.find((item) => item.id === currentBlueprintId)
            ?.createdAt ?? Date.now(),
        updatedAt: Date.now(),
        status: 'running',
        executionLog,
        backendAgentId: blueprints.find(
          (item) => item.id === currentBlueprintId
        )?.backendAgentId,
        backendBlueprintId: blueprints.find(
          (item) => item.id === currentBlueprintId
        )?.backendBlueprintId,
        knowledgeScope,
      }
      void runSavedBlueprintOnBackend(
        backendBlueprint,
        isManualPreview
          ? { forceCreate: true, triggerMode: 'manual' }
          : undefined
      )
        .then(async (synced) => {
          if (!synced) return
          if (previewRunIdRef.current !== runId) return
          if (!isManualPreview) {
            setBlueprints((current) => {
              const exists = current.some((item) => item.id === synced.id)
              const next = exists
                ? current.map((item) =>
                    item.id === synced.id
                      ? {
                          ...item,
                          backendAgentId: synced.backendAgentId,
                          backendBlueprintId: synced.backendBlueprintId,
                        }
                      : item
                  )
                : [synced, ...current]
              writeSavedBlueprints(next)
              notifyAgentBlueprintsChanged()
              return next
            })
          }
          if (synced.backendAgentId && synced.backendBlueprintId) {
            try {
              if (realtimeListenerIndex >= 0) {
                await waitForAgentBackendPoll(1000)
                const logs = await refreshBackendExecutionLogs(
                  synced.backendAgentId,
                  synced.backendBlueprintId,
                  () => previewRunIdRef.current === runId
                )
                const latestLog = logs[0]
                if (
                  latestLog?.status === 'failed' &&
                  previewRunIdRef.current === runId
                ) {
                  applyBackendExecutionLogToPreview(
                    latestLog,
                    runExecutionNodes,
                    runGraph.edges,
                    statusAfterManualPreview,
                    prepared.edges
                  )
                }
                return
              }
              const logs = await pollBackendExecutionLogs(
                synced.backendAgentId,
                synced.backendBlueprintId,
                () => previewRunIdRef.current === runId,
                (progressLog) => {
                  if (previewRunIdRef.current !== runId) return
                  applyBackendExecutionLogToPreview(
                    progressLog,
                    runExecutionNodes,
                    runGraph.edges,
                    statusAfterManualPreview,
                    prepared.edges,
                    false
                  )
                }
              )
              const latestLog = logs[0]
              if (previewRunIdRef.current !== runId) return
              if (latestLog) {
                applyBackendExecutionLogToPreview(
                  latestLog,
                  runExecutionNodes,
                  runGraph.edges,
                  statusAfterManualPreview,
                  prepared.edges
                )
              }
            } catch (error) {
              toast.error(
                error instanceof Error
                  ? `后端执行日志同步失败: ${error.message}`
                  : '后端执行日志同步失败'
              )
            }
          }
        })
        .catch((error) => {
          if (previewRunIdRef.current !== runId) return
          const message =
            error instanceof Error ? error.message : '后端正式运行触发失败'
          const statusById = new Map<string, AgentNodeStatus>()
          runExecutionNodes.forEach((node, index) => {
            statusById.set(node.id, index === 0 ? 'failed' : 'waiting')
          })
          const failedLog = buildPreviewExecutionLog(
            runExecutionNodes,
            statusById,
            skillChoices,
            runGraph.edges
          )
          const firstNode = runExecutionNodes[0]
          if (firstNode) {
            failedLog[firstNode.id] = {
              ...failedLog[firstNode.id],
              status: 'failed',
              message: '后端正式运行触发失败。',
              error: message,
            }
          }
          setExecutionLog(failedLog)
          setNodes((current) => {
            const nextNodes = current.map((node) => ({
              ...node,
              data: {
                ...node.data,
                status: statusById.get(node.id) ?? node.data.status,
              },
            }))
            persistCurrentBlueprintSnapshot(
              stripTransientNodeSettings(nextNodes),
              failedLog,
              statusAfterManualPreview ?? 'failed',
              prepared.edges
            )
            return nextNodes
          })
          toast.error(
            error instanceof Error
              ? `后端正式运行触发失败: ${error.message}`
              : '后端正式运行触发失败'
          )
        })

      const initialStatusById = buildPreviewStatusMap(
        runExecutionNodes,
        realtimeListenerIndex >= 0 ? realtimeListenerIndex - 1 : -1
      )
      const initialExecutionLog = buildPreviewExecutionLog(
        runExecutionNodes,
        initialStatusById,
        skillChoices,
        runGraph.edges
      )
      setExecutionLog(initialExecutionLog)
      setNodes((current) => {
        const activeNodeById = new Map(
          runExecutionNodes.map((node) => [node.id, node])
        )
        const currentById = new Map(current.map((node) => [node.id, node]))
        const nextNodes = current.map((node) => {
          const activeNode = activeNodeById.get(node.id)
          const status = initialStatusById.get(node.id)
          if (!activeNode && !status) return node
          return {
            ...(activeNode ?? node),
            data: {
              ...(activeNode?.data ?? node.data),
              status: status ?? activeNode?.data.status ?? node.data.status,
            },
          }
        })
        runExecutionNodes.forEach((activeNode) => {
          if (currentById.has(activeNode.id)) return
          nextNodes.push({
            ...activeNode,
            data: {
              ...activeNode.data,
              status:
                initialStatusById.get(activeNode.id) ?? activeNode.data.status,
            },
          })
        })
        persistCurrentBlueprintSnapshot(
          stripTransientNodeSettings(nextNodes),
          initialExecutionLog,
          statusAfterManualPreview ?? 'running',
          prepared.edges
        )
        return nextNodes
      })
    },
    [
      applyBackendExecutionLogToPreview,
      blueprintTitle,
      blueprints,
      cancelPreviewRun,
      currentBlueprintId,
      currentBlueprintStatus,
      edges,
      executionLog,
      hasScheduledBlueprint,
      knowledgeScope,
      nodes,
      pollBackendExecutionLogs,
      persistCurrentBlueprintSnapshot,
      refreshBackendExecutionLogs,
      skillChoices,
      templateByKind,
    ]
  )

  const handleStopPreview = useCallback(() => {
    const currentBlueprint = blueprints.find(
      (item) => item.id === currentBlueprintId
    )
    if (
      currentBlueprint?.backendAgentId &&
      currentBlueprint.backendBlueprintId
    ) {
      void pauseBlueprint(
        currentBlueprint.backendAgentId,
        currentBlueprint.backendBlueprintId
      ).catch((error) => {
        toast.error(
          error instanceof Error
            ? `暂停后端蓝图失败: ${error.message}`
            : '暂停后端蓝图失败'
        )
      })
    }
    cancelPreviewRun()
    const stoppedAt = Date.now()
    const nextExecutionLog = stopRunningExecutionLogs(executionLog, stoppedAt)
    setExecutionLog(nextExecutionLog)
    setNodes((current) => {
      const nextNodes = current.map((node) =>
        node.data.status === 'running'
          ? {
              ...node,
              data: { ...node.data, status: 'waiting' as AgentNodeStatus },
            }
          : node
      )
      persistCurrentBlueprintSnapshot(nextNodes, nextExecutionLog, 'paused')
      return nextNodes
    })
  }, [
    blueprints,
    cancelPreviewRun,
    currentBlueprintId,
    executionLog,
    persistCurrentBlueprintSnapshot,
  ])

  const handleAddSkill = useCallback(
    async (skill: SkillChoice) => {
      let installedSkill = skill
      try {
        if (skill.source === 'marketplace') {
          const created = await createAgentSkill({
            name: '',
            description: '',
            source_type: 'marketplace',
            marketplace_id: skill.marketplaceId ?? skill.value,
          })
          installedSkill = skillChoiceFromAgentSkill(created) ?? {
            ...skill,
            value: created.id,
          }
        } else if (skill.source === 'github') {
          const created = await createAgentSkill({
            name: '',
            description: '',
            source_type: 'github',
            source_url: skill.reference ?? skill.value,
            tags: skill.tags,
          })
          installedSkill = skillChoiceFromAgentSkill(created) ?? {
            ...skill,
            value: created.id,
          }
        } else if (skill.source === 'custom') {
          const created = await createAgentSkill({
            name: skill.label,
            description: skill.description,
            source_type: 'custom',
            model_group: skill.modelGroup || currentGroup || 'default',
            model_name: skill.modelName || currentModel || '',
            tags: skill.tags,
          })
          installedSkill = skillChoiceFromAgentSkill(created) ?? {
            ...skill,
            value: created.id,
          }
        }

        setManagedSkills((current) => {
          const next = mergeSkillChoices([installedSkill], current).filter(
            (item) => item.source !== 'built-in'
          )
          writeSavedSkills(next)
          return next
        })
        void reloadSkills()
        toast.success('Skill 已安装')
      } catch (error) {
        toast.error(error instanceof Error ? error.message : 'Skill 安装失败')
        throw error
      }
    },
    [currentGroup, currentModel, reloadSkills]
  )

  const handleDeleteSkill = useCallback(
    async (skillValue: string) => {
      try {
        await deleteAgentSkill(skillValue)
        setManagedSkills((current) => {
          const next = current.filter((skill) => skill.value !== skillValue)
          writeSavedSkills(next)
          return next
        })
        void reloadSkills()
        toast.success('Skill 已删除')
      } catch (error) {
        toast.error(error instanceof Error ? error.message : 'Skill 删除失败')
        throw error
      }
    },
    [reloadSkills]
  )

  const handleCreateComponent = useCallback(
    async (payload: AgentComponentFormData) => {
      const component = await createAgentComponent(payload)
      setComponentCatalog((current) => [
        ...current.filter((item) => item.id !== component.id),
        component,
      ])
      toast.success('自定义步骤已创建')
    },
    []
  )

  const handleDeleteComponent = useCallback(async (componentId: string) => {
    await deleteAgentComponent(componentId)
    setComponentCatalog((current) =>
      current.filter((component) => component.id !== componentId)
    )
    toast.success('自定义步骤已删除')
  }, [])

  const handleAddNode = useCallback(
    (
      templateKey: AgentNodeKind | string,
      position?: { x: number; y: number }
    ) => {
      const template =
        templateById.get(templateKey) ??
        templateByKind.get(templateKey as AgentNodeKind)
      if (!template) return
      recordHistorySnapshot()
      addCounterRef.current += 1
      const id = `${template.kind}-${Date.now()}-${addCounterRef.current}`
      const node = createNodeFromTemplate(template, id, {
        x: position?.x ?? 120 + addCounterRef.current * 36,
        y: position?.y ?? 120 + addCounterRef.current * 28,
      })
      setNodes((current) => [...current, node])
      setSelectedNodeId(id)
    },
    [recordHistorySnapshot, templateById, templateByKind]
  )

  const handleUpdateNode = useCallback(
    (patch: Partial<AgentNodeData>) => {
      if (!selectedNode) return
      recordHistorySnapshot()
      setNodes((current) =>
        current.map((node) =>
          node.id === selectedNode.id
            ? { ...node, data: { ...node.data, ...patch } }
            : node
        )
      )
    },
    [recordHistorySnapshot, selectedNode]
  )

  const handleUpdateSetting = useCallback(
    (key: string, value: string) => {
      if (!selectedNodeId) return
      recordHistorySnapshot()
      setNodes((current) =>
        current.map((node) =>
          node.id === selectedNodeId
            ? updateNodeSettingValue(node, key, value)
            : node
        )
      )
    },
    [recordHistorySnapshot, selectedNodeId]
  )

  const handleSetNodeSkill = useCallback(
    (nodeId: string, skill: string) => {
      recordHistorySnapshot()
      setNodes((current) =>
        current.map((node) =>
          node.id === nodeId ? { ...node, data: { ...node.data, skill } } : node
        )
      )
    },
    [recordHistorySnapshot]
  )

  const handleTitleChange = useCallback(
    (title: string) => {
      recordHistorySnapshot()
      setBlueprintTitle(title)
    },
    [recordHistorySnapshot]
  )

  useEffect(() => {
    if (!seedPrompt || seedPrompt === lastSeedPromptRef.current) return

    lastSeedPromptRef.current = seedPrompt
    const title = buildSeedBlueprintTitle(seedPrompt)
    setBlueprintTitle(title)
    setCurrentBlueprintId(null)
    setLastSavedAt(null)
    setExecutionLog({})
    setNodes(() =>
      createInitialNodes(templates).map((node) =>
        node.id === 'process'
          ? {
              ...node,
              data: {
                ...node.data,
                title: '处理任务',
                description: seedPrompt,
              },
            }
          : node
      )
    )
    setEdges(INITIAL_EDGES.map((edge) => decorateEdge(edge)))
    setSelectedNodeId('process')
  }, [seedPrompt, templates])

  useEffect(() => {
    const handleCommand = (event: Event) => {
      const command = (event as CustomEvent<AgentBlueprintCommand>).detail
      if (!command) return

      if (command.type === 'new') {
        handleNewBlueprint()
        return
      }

      if (command.type === 'delete') {
        void handleDeleteBlueprint(command.id)
        return
      }

      const blueprint = readSavedBlueprints().find(
        (item) => item.id === command.id
      )
      if (blueprint) {
        handleLoadBlueprint(blueprint)
      }
    }

    const handleChanged = () => {
      const next = readSavedBlueprints()
      setBlueprints(next)
      if (
        currentBlueprintId &&
        !next.some((blueprint) => blueprint.id === currentBlueprintId)
      ) {
        const fallback = next[0]
        if (fallback) {
          handleLoadBlueprint(fallback)
        } else {
          handleNewBlueprint()
        }
      }
    }

    window.addEventListener(AGENT_BLUEPRINT_COMMAND_EVENT, handleCommand)
    window.addEventListener(AGENT_BLUEPRINTS_CHANGED_EVENT, handleChanged)
    return () => {
      window.removeEventListener(AGENT_BLUEPRINT_COMMAND_EVENT, handleCommand)
      window.removeEventListener(AGENT_BLUEPRINTS_CHANGED_EVENT, handleChanged)
    }
  }, [currentBlueprintId, handleLoadBlueprint, handleNewBlueprint])

  return (
    <ReactFlowProvider>
      <BlueprintWorkspace
        blueprintChoices={blueprints
          .filter((blueprint) => blueprint.backendBlueprintId)
          .map((blueprint) => ({
            label: blueprint.title,
            localId: blueprint.id,
            value: blueprint.backendBlueprintId!,
          }))}
        blueprintTitle={blueprintTitle}
        knowledgeScope={knowledgeScope}
        backendExecutionLogs={backendExecutionLogs}
        replyRecords={replyRecords}
        replyRecordsOpen={replyRecordsOpen}
        replyRecordsLoading={replyRecordsLoading}
        canRedo={canRedoBlueprint}
        canUndo={canUndoBlueprint}
        currentBlueprintStatus={currentBlueprintStatus}
        currentGroup={currentGroup}
        edges={edges}
        executionLog={executionLog}
        executionNodes={executionNodes}
        groupChoices={groupChoices}
        lastSavedAt={lastSavedAt}
        modelChoices={modelChoices}
        nodes={nodes}
        skillChoices={skillChoices}
        managedSkills={managedSkills}
        marketplaceSkills={marketplaceSkills}
        paletteGroups={filteredPaletteGroups}
        isComponentCatalogLoading={isComponentCatalogLoading}
        isSkillCatalogLoading={isSkillCatalogLoading}
        onAddSkill={handleAddSkill}
        onAddNode={handleAddNode}
        onCreateComponent={handleCreateComponent}
        onConnect={onConnect}
        onDeleteEdge={handleDeleteEdge}
        onDeleteNode={handleDeleteNode}
        onDeleteComponent={handleDeleteComponent}
        onDeleteSkill={handleDeleteSkill}
        onEdgesChange={onEdgesChange}
        onNodesChange={onNodesChange}
        onRedo={handleRedoBlueprint}
        onReset={handleReset}
        onRunPreview={handleRunPreview}
        onRefreshReplyRecords={() => void refreshReplyRecords()}
        onReplyRecordsOpenChange={setReplyRecordsOpen}
        onStopPreview={handleStopPreview}
        onSaveBlueprint={handleSaveBlueprint}
        onKnowledgeScopeChange={setKnowledgeScope}
        onSearchChange={setSearch}
        onSelectNode={setSelectedNodeId}
        onSetNodeSkill={handleSetNodeSkill}
        onTitleChange={handleTitleChange}
        onUndo={handleUndoBlueprint}
        onUpdateNode={handleUpdateNode}
        onUpdateSetting={handleUpdateSetting}
        search={search}
        selectedNode={selectedNode}
        currentBlueprintId={currentBlueprintId}
        hasManualTrigger={hasManualTrigger}
        hasScheduledBlueprint={hasScheduledBlueprint}
      />
    </ReactFlowProvider>
  )
}

type BlueprintWorkspaceProps = {
  blueprintChoices: BlueprintChoice[]
  nodes: AgentFlowNode[]
  edges: AgentFlowEdge[]
  selectedNode: AgentFlowNode | null
  executionNodes: AgentFlowNode[]
  executionLog: AgentExecutionLogMap
  modelChoices: ModelChoice[]
  groupChoices: GroupOption[]
  skillChoices: SkillChoice[]
  managedSkills: SkillChoice[]
  marketplaceSkills: SkillChoice[]
  paletteGroups: PaletteTemplateGroup[]
  isComponentCatalogLoading: boolean
  isSkillCatalogLoading: boolean
  blueprintTitle: string
  knowledgeScope: KnowledgeScopeValue
  backendExecutionLogs: AgentExecutionLog[]
  replyRecords: AgentXianyuReplyRecord[]
  replyRecordsOpen: boolean
  replyRecordsLoading: boolean
  canRedo: boolean
  canUndo: boolean
  currentBlueprintStatus: AgentBlueprintStatus
  currentBlueprintId: string | null
  currentGroup?: string
  hasManualTrigger: boolean
  hasScheduledBlueprint: boolean
  lastSavedAt: number | null
  search: string
  onSearchChange: (value: string) => void
  onNodesChange: (changes: NodeChange<AgentFlowNode>[]) => void
  onEdgesChange: (changes: EdgeChange<AgentFlowEdge>[]) => void
  onConnect: (connection: Connection) => void
  onSelectNode: (nodeId: string) => void
  onAddNode: (
    templateKey: AgentNodeKind | string,
    position?: { x: number; y: number }
  ) => void
  onDeleteNode: (nodeId: string) => void
  onDeleteEdge: (edgeId: string) => void
  onTitleChange: (value: string) => void
  onSaveBlueprint: () => void
  onKnowledgeScopeChange: (value: KnowledgeScopeValue) => void
  onUpdateNode: (patch: Partial<AgentNodeData>) => void
  onUpdateSetting: (key: string, value: string) => void
  onAddSkill: (skill: SkillChoice) => Promise<void>
  onDeleteSkill: (skillValue: string) => Promise<void>
  onCreateComponent: (payload: AgentComponentFormData) => Promise<void>
  onDeleteComponent: (componentId: string) => Promise<void>
  onSetNodeSkill: (nodeId: string, skill: string) => void
  onRedo: () => void
  onReset: () => void
  onRunPreview: (mode?: AgentPreviewRunMode) => void
  onRefreshReplyRecords: () => void
  onReplyRecordsOpenChange: (open: boolean) => void
  onStopPreview: () => void
  onUndo: () => void
}

function BlueprintWorkspace({
  blueprintChoices,
  nodes,
  edges,
  selectedNode,
  executionNodes,
  executionLog,
  modelChoices,
  groupChoices,
  skillChoices,
  managedSkills,
  marketplaceSkills,
  paletteGroups,
  isComponentCatalogLoading,
  isSkillCatalogLoading,
  blueprintTitle,
  knowledgeScope,
  backendExecutionLogs,
  replyRecords,
  replyRecordsOpen,
  replyRecordsLoading,
  canRedo,
  canUndo,
  currentBlueprintStatus,
  currentBlueprintId,
  currentGroup,
  hasManualTrigger,
  hasScheduledBlueprint,
  lastSavedAt,
  search,
  onSearchChange,
  onNodesChange,
  onEdgesChange,
  onConnect,
  onSelectNode,
  onAddNode,
  onDeleteNode,
  onDeleteEdge,
  onTitleChange,
  onSaveBlueprint,
  onKnowledgeScopeChange,
  onUpdateNode,
  onUpdateSetting,
  onAddSkill,
  onDeleteSkill,
  onCreateComponent,
  onDeleteComponent,
  onSetNodeSkill,
  onRedo,
  onReset,
  onRunPreview,
  onRefreshReplyRecords,
  onReplyRecordsOpenChange,
  onStopPreview,
  onUndo,
}: BlueprintWorkspaceProps) {
  const reactFlow = useReactFlow<AgentFlowNode, AgentFlowEdge>()
  const workflowReadiness = useMemo(
    () => validateWorkflowStructure(nodes, edges),
    [edges, nodes]
  )
  const isValidConnection = useCallback(
    (connection: Connection | AgentFlowEdge) =>
      isAgentConnectionValid(nodes, edges, connection),
    [edges, nodes]
  )
  const [isPreviewOpen, setIsPreviewOpen] = useState(false)
  const [activeToolPanel, setActiveToolPanel] =
    useState<AgentToolPanel>('components')
  const nodeTypes = useMemo(
    () => ({
      agentNode: (props: NodeProps<AgentFlowNode>) => (
        <BlueprintNode
          {...props}
          skillChoices={skillChoices}
          onSetSkill={onSetNodeSkill}
        />
      ),
    }),
    [onSetNodeSkill, skillChoices]
  )
  const edgeTypes = useMemo(
    () => ({
      deletable: (props: EdgeProps<AgentFlowEdge>) => (
        <DeletableEdge {...props} onDeleteEdge={onDeleteEdge} />
      ),
    }),
    [onDeleteEdge]
  )

  const handleDrop = useCallback(
    (event: DragEvent<HTMLDivElement>) => {
      event.preventDefault()
      const templateKey = event.dataTransfer.getData(DRAG_NODE_TYPE)
      if (!templateKey) return
      const position = reactFlow.screenToFlowPosition({
        x: event.clientX,
        y: event.clientY,
      })
      onAddNode(templateKey, position)
    },
    [onAddNode, reactFlow]
  )

  return (
    <div className='bg-background flex size-full min-h-0 overflow-hidden'>
      <aside className='border-border/70 bg-muted/25 hidden w-68 shrink-0 flex-col border-r lg:flex'>
        <Palette
          activePanel={activeToolPanel}
          groupChoices={groupChoices}
          groups={paletteGroups}
          isComponentCatalogLoading={isComponentCatalogLoading}
          isSkillCatalogLoading={isSkillCatalogLoading}
          marketplaceSkills={marketplaceSkills}
          managedSkills={managedSkills}
          modelChoices={modelChoices}
          knowledgeScope={knowledgeScope}
          onAddNode={onAddNode}
          onAddSkill={onAddSkill}
          onCreateComponent={onCreateComponent}
          onDeleteComponent={onDeleteComponent}
          onDeleteSkill={onDeleteSkill}
          onKnowledgeScopeChange={onKnowledgeScopeChange}
          onPanelChange={setActiveToolPanel}
          onSearchChange={onSearchChange}
          search={search}
          skillChoices={skillChoices}
        />
      </aside>

      <div className='relative flex min-w-0 flex-1 flex-col'>
        <div className='border-border/70 flex min-h-0 shrink-0 items-center gap-2 border-b px-3 py-2 lg:hidden'>
          <CompactPalette groups={paletteGroups} onAddNode={onAddNode} />
        </div>

        <main
          className='relative min-h-0 min-w-0 flex-1'
          onDragOver={(event) => {
            event.preventDefault()
            event.dataTransfer.dropEffect = 'move'
          }}
          onDrop={handleDrop}
        >
          <div className='pointer-events-none absolute top-3 right-3 left-3 z-10 flex flex-wrap items-start justify-between gap-2'>
            <div className='bg-background/88 border-border/70 pointer-events-auto flex max-w-full min-w-0 items-start gap-2 rounded-[8px] border px-2 py-1.5 shadow-sm backdrop-blur'>
              <div className='bg-primary text-primary-foreground flex size-7 shrink-0 items-center justify-center rounded-[6px]'>
                <ChartNetworkIcon className='size-4' />
              </div>
              <div className='min-w-0'>
                <Input
                  value={blueprintTitle}
                  onChange={(event) => onTitleChange(event.target.value)}
                  className='h-7 min-w-45 rounded-[6px] border-0 bg-transparent px-0 text-sm font-semibold shadow-none focus-visible:ring-0'
                />
                <div className='text-muted-foreground flex min-w-0 items-center gap-1.5 text-[11px]'>
                  <span className='truncate'>
                    {nodes.length} 个节点 · {edges.length} 条连线 ·{' '}
                    {getAgentBlueprintStatusLabel(currentBlueprintStatus)}
                    {lastSavedAt
                      ? ` · ${formatBlueprintTime(lastSavedAt)} 已保存`
                      : ''}
                  </span>
                </div>
              </div>
            </div>

            <div className='pointer-events-auto flex max-w-full items-start gap-2'>
              <TooltipProvider>
                <div className='bg-background/88 border-border/70 flex items-center gap-1 rounded-[8px] border p-1 shadow-sm backdrop-blur'>
                  <CanvasToolbarButton label='重置' onClick={onReset}>
                    <RotateCcwIcon className='size-3.5' />
                  </CanvasToolbarButton>
                  <CanvasToolbarButton
                    label='前一步'
                    disabled={!canUndo}
                    onClick={onUndo}
                  >
                    <Undo2Icon className='size-3.5' />
                  </CanvasToolbarButton>
                  <CanvasToolbarButton
                    label='后一步'
                    disabled={!canRedo}
                    onClick={onRedo}
                  >
                    <Redo2Icon className='size-3.5' />
                  </CanvasToolbarButton>
                  <CanvasToolbarButton label='保存' onClick={onSaveBlueprint}>
                    <SaveIcon className='size-3.5' />
                  </CanvasToolbarButton>
                  {hasScheduledBlueprint ? (
                    <>
                      {currentBlueprintStatus === 'paused' ? (
                        <CanvasToolbarButton
                          label='继续定时'
                          onClick={() => onRunPreview()}
                        >
                          <PlayIcon className='size-3.5' />
                        </CanvasToolbarButton>
                      ) : (
                        <CanvasToolbarButton
                          label='暂停定时'
                          variant='destructive'
                          onClick={onStopPreview}
                        >
                          <PauseIcon className='size-3.5' />
                        </CanvasToolbarButton>
                      )}
                      {hasManualTrigger && (
                        <CanvasToolbarButton
                          label='立即运行一次'
                          onClick={() => onRunPreview('manual')}
                        >
                          <ZapIcon className='size-3.5' />
                        </CanvasToolbarButton>
                      )}
                    </>
                  ) : hasManualTrigger &&
                    currentBlueprintStatus === 'running' ? (
                    <CanvasToolbarButton
                      label='停止'
                      variant='destructive'
                      onClick={onStopPreview}
                    >
                      <StopCircleIcon className='size-3.5' />
                    </CanvasToolbarButton>
                  ) : hasManualTrigger ? (
                    <CanvasToolbarButton
                      label={getRunPreviewButtonLabel(currentBlueprintStatus)}
                      onClick={() => onRunPreview()}
                    >
                      <ZapIcon className='size-3.5' />
                    </CanvasToolbarButton>
                  ) : null}
                </div>
              </TooltipProvider>
            </div>
          </div>

          <ReactFlow<AgentFlowNode, AgentFlowEdge>
            ariaLabelConfig={CREATOR_FLOW_ARIA_LABEL_CONFIG}
            className='agent-blueprint-canvas'
            colorMode='system'
            connectionLineStyle={{
              stroke: 'var(--primary)',
              strokeWidth: 1.8,
            }}
            defaultEdgeOptions={{
              markerEnd: { type: MarkerType.ArrowClosed },
              style: {
                stroke: 'var(--primary)',
                strokeWidth: 1.8,
              },
            }}
            deleteKeyCode={['Backspace', 'Delete']}
            edges={edges}
            edgeTypes={edgeTypes}
            edgesFocusable
            elementsSelectable
            fitView
            fitViewOptions={{ padding: 0.22 }}
            isValidConnection={isValidConnection}
            maxZoom={1.25}
            minZoom={0.35}
            nodeTypes={nodeTypes}
            nodesDraggable
            nodesFocusable
            nodes={nodes}
            onConnect={onConnect}
            onEdgeDoubleClick={(_, edge) => onDeleteEdge(edge.id)}
            onEdgesChange={onEdgesChange}
            onNodeClick={(_, node) => onSelectNode(node.id)}
            onNodesChange={onNodesChange}
            panOnScroll
            proOptions={{ hideAttribution: true }}
            selectNodesOnDrag={false}
            snapGrid={[16, 16]}
            snapToGrid
          >
            <Background
              bgColor='var(--sidebar)'
              color='var(--border)'
              gap={20}
            />
            <Controls
              className='!bottom-12 !left-2 rounded-[8px] border shadow-none'
              position='bottom-left'
            />
            <MiniMap
              className='border-border/70 bg-background/90 !right-2 !bottom-12 overflow-hidden rounded-[8px] border shadow-sm'
              maskColor='color-mix(in oklch, var(--background) 68%, transparent)'
              nodeBorderRadius={8}
              nodeColor={(node) =>
                node.selected ? 'var(--primary)' : 'var(--muted-foreground)'
              }
              pannable
              position='bottom-right'
              zoomable
            />
          </ReactFlow>
        </main>

        <div className='pointer-events-none absolute inset-x-0 bottom-0 z-20'>
          <div className='pointer-events-auto'>
            <ExecutionPreviewDock
              backendExecutionLogs={backendExecutionLogs}
              blueprintStatus={currentBlueprintStatus}
              executionLog={executionLog}
              executionNodes={executionNodes}
              isOpen={isPreviewOpen}
              readiness={workflowReadiness}
              selectedNode={selectedNode}
              skillChoices={skillChoices}
              onSelectNode={onSelectNode}
              onOpenChange={setIsPreviewOpen}
            />
          </div>
        </div>
      </div>

      <aside className='border-border/70 bg-background hidden w-[333px] min-w-0 shrink-0 flex-col overflow-hidden border-l xl:flex'>
          <Inspector
          blueprintId={currentBlueprintId}
          blueprintChoices={blueprintChoices}
          currentGroup={currentGroup}
          groupChoices={groupChoices}
          modelChoices={modelChoices}
          skillChoices={skillChoices}
          selectedNode={selectedNode}
          onDeleteNode={onDeleteNode}
          onUpdateNode={onUpdateNode}
            onUpdateSetting={onUpdateSetting}
          />
          {selectedNode?.data.kind === 'xianyu_reply' && (
            <XianyuReplyRecordsPanel
              loading={replyRecordsLoading}
              open={replyRecordsOpen}
              records={replyRecords}
              onOpenChange={onReplyRecordsOpenChange}
              onRefresh={onRefreshReplyRecords}
            />
          )}
        </aside>
    </div>
  )
}

function WorkflowStageStrip({
  readiness,
  className,
}: {
  readiness: WorkflowReadiness
  className?: string
}) {
  return (
    <div
      aria-label='调用流程缩略图'
      className={cn(
        'min-w-0 items-center gap-1 overflow-hidden whitespace-nowrap',
        className
      )}
    >
      {readiness.stages.map((stage, index) => (
        <div key={stage.label} className='flex shrink-0 items-center gap-1'>
          {index > 0 && (
            <span className='text-muted-foreground/60 text-[10px]' aria-hidden>
              &rarr;
            </span>
          )}
          <span
            title={
              stage.complete
                ? `${stage.label}阶段已就绪`
                : !stage.required && !stage.present
                  ? `${stage.label}阶段可跳过`
                  : stage.present
                    ? `${stage.label}阶段待完善`
                    : `缺少${stage.label}阶段`
            }
            className={cn(
              'inline-flex items-center gap-1 rounded-[6px] border px-1.5 py-0.5 text-[10px] font-medium',
              stage.complete
                ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
                : !stage.required && !stage.present
                  ? 'border-border/70 bg-muted/40 text-muted-foreground'
                  : stage.present
                    ? 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300'
                    : 'border-dashed border-amber-500/40 bg-amber-500/5 text-amber-700 dark:text-amber-300'
            )}
          >
            {stage.complete && <CheckCircle2Icon className='size-3' />}
            {!stage.present && stage.required && (
              <TriangleAlertIcon className='size-3' />
            )}
            {stage.label}
          </span>
        </div>
      ))}
    </div>
  )
}

function Palette({
  activePanel,
  groupChoices,
  groups,
  isComponentCatalogLoading,
  isSkillCatalogLoading,
  skillChoices,
  managedSkills,
  marketplaceSkills,
  modelChoices,
  knowledgeScope,
  search,
  onSearchChange,
  onAddNode,
  onAddSkill,
  onCreateComponent,
  onDeleteComponent,
  onDeleteSkill,
  onKnowledgeScopeChange,
  onPanelChange,
}: {
  activePanel: AgentToolPanel
  groupChoices: GroupOption[]
  groups: PaletteTemplateGroup[]
  isComponentCatalogLoading: boolean
  isSkillCatalogLoading: boolean
  skillChoices: SkillChoice[]
  managedSkills: SkillChoice[]
  marketplaceSkills: SkillChoice[]
  modelChoices: ModelChoice[]
  knowledgeScope: KnowledgeScopeValue
  search: string
  onSearchChange: (value: string) => void
  onAddNode: (templateKey: AgentNodeKind | string) => void
  onAddSkill: (skill: SkillChoice) => Promise<void>
  onCreateComponent: (payload: AgentComponentFormData) => Promise<void>
  onDeleteComponent: (componentId: string) => Promise<void>
  onDeleteSkill: (skillValue: string) => Promise<void>
  onKnowledgeScopeChange: (value: KnowledgeScopeValue) => void
  onPanelChange: (panel: AgentToolPanel) => void
}) {
  return (
    <div className='flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden'>
      <Tabs
        value={activePanel}
        onValueChange={(value) => onPanelChange(value as AgentToolPanel)}
        className='flex min-h-0 flex-1 flex-col'
      >
        {/* 顶部：标题 + Tab 导航 */}
        <div className='border-border/70 shrink-0 border-b px-3 pt-3'>
          <div className='mb-2.5 flex items-center gap-2'>
            <div className='bg-primary text-primary-foreground flex size-7 shrink-0 items-center justify-center rounded-[7px]'>
              <BotIcon className='size-3.5' />
            </div>
            <span className='min-w-0 flex-1 truncate text-sm font-semibold'>
              处理节点
            </span>
            <Button
              type='button'
              size='sm'
              variant={activePanel === 'knowledge' ? 'secondary' : 'ghost'}
              className='h-7 shrink-0 gap-1.5 rounded-[6px] px-2 text-xs'
              aria-pressed={activePanel === 'knowledge'}
              onClick={() => onPanelChange('knowledge')}
            >
              <BookOpenIcon className='size-3.5' />
              知识库
              {!!knowledgeScope.knowledgeBaseIds?.length && (
                <span className='bg-background/80 rounded-full px-1.5 text-[10px] leading-[14px]'>
                  {knowledgeScope.knowledgeBaseIds.length}
                </span>
              )}
            </Button>
          </div>

          <TabsList className='h-7 w-full gap-0 rounded-none border-0 bg-transparent p-0'>
            <TabsTrigger
              value='components'
              className='data-active:border-primary h-7 flex-1 rounded-none border-b-2 border-transparent bg-transparent px-2 text-xs font-medium shadow-none data-active:bg-transparent data-active:shadow-none'
            >
              处理节点
            </TabsTrigger>
            <TabsTrigger
              value='skills'
              className='data-active:border-primary h-7 flex-1 rounded-none border-b-2 border-transparent bg-transparent px-2 text-xs font-medium shadow-none data-active:bg-transparent data-active:shadow-none'
            >
              Skill
              <span className='bg-muted text-muted-foreground ml-1 rounded-full px-1.5 py-0 text-[10px] leading-[14px]'>
                {skillChoices.length}
              </span>
            </TabsTrigger>
          </TabsList>
        </div>

        {/* ── 组件 Tab ── */}
        <TabsContent
          value='components'
          className='m-0 flex min-h-0 flex-1 flex-col data-[state=inactive]:hidden'
        >
          {/* 搜索框 */}
          <div className='shrink-0 px-2 py-2'>
            <div className='relative'>
              <SearchIcon className='text-muted-foreground absolute top-1/2 left-2.5 size-3 -translate-y-1/2' />
              <Input
                value={search}
                onChange={(e) => onSearchChange(e.target.value)}
                placeholder='搜索处理节点…'
                className='h-7 rounded-[7px] pl-7 text-xs'
              />
            </div>
          </div>

          {/* 组件列表（可滚动） */}
          <div className='min-h-0 flex-1 overflow-y-auto px-2 pb-3'>
            <ComponentManagerPanel
              groupChoices={groupChoices}
              isLoading={isComponentCatalogLoading}
              modelChoices={modelChoices}
              onCreateComponent={onCreateComponent}
            />
            {buildComponentPaletteSections(groups).map((section) => (
              <div key={section.title} className='mb-2'>
                <div className='text-muted-foreground flex items-center gap-1 px-1 pt-0.5 pb-1 text-[10px] font-semibold tracking-widest uppercase'>
                  <span>{section.title}</span>
                  <span className='bg-muted rounded-full px-1.5 py-0 text-[9px] tracking-normal'>
                    {section.badge}
                  </span>
                </div>
                <div className='grid gap-0.5'>
                  {section.templates.map((template) => (
                    <PaletteItem
                      key={template.componentId ?? template.kind}
                      template={template}
                      onAddNode={onAddNode}
                      onDeleteComponent={onDeleteComponent}
                    />
                  ))}
                </div>
              </div>
            ))}
          </div>
        </TabsContent>

        {/* ── Skill Tab ── */}
        <TabsContent
          value='skills'
          className='m-0 min-h-0 flex-1 overflow-y-auto data-[state=inactive]:hidden'
        >
          <div className='px-2 py-2'>
            <SkillManagerPanel
              groupChoices={groupChoices}
              isLoading={isSkillCatalogLoading}
              marketplaceSkills={marketplaceSkills}
              managedSkills={managedSkills}
              modelChoices={modelChoices}
              skillChoices={skillChoices}
              onAddSkill={onAddSkill}
              onDeleteSkill={onDeleteSkill}
            />
          </div>
        </TabsContent>

        <TabsContent
          value='knowledge'
          className='m-0 flex min-h-0 flex-1 flex-col data-[state=inactive]:hidden'
        >
          <KnowledgeScopeSelector
            value={knowledgeScope}
            onChange={onKnowledgeScopeChange}
            className='mx-2 my-2'
          />
        </TabsContent>
      </Tabs>
    </div>
  )
}

function ComponentManagerPanel({
  groupChoices,
  isLoading,
  modelChoices,
  onCreateComponent,
}: {
  groupChoices: GroupOption[]
  isLoading: boolean
  modelChoices: ModelChoice[]
  onCreateComponent: (payload: AgentComponentFormData) => Promise<void>
}) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [group, setGroup] = useState('process')
  const [modelGroup, setModelGroup] = useState(
    groupChoices[0]?.value ?? 'default'
  )
  const [modelName, setModelName] = useState(modelChoices[0]?.value ?? '')
  const [inputPorts, setInputPorts] = useState<string[]>([])
  const [outputPorts, setOutputPorts] = useState<string[]>([])
  const [tags, setTags] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  useEffect(() => {
    if (!groupChoices.some((option) => option.value === modelGroup)) {
      setModelGroup(groupChoices[0]?.value ?? 'default')
    }
  }, [groupChoices, modelGroup])

  useEffect(() => {
    if (modelName) return
    setModelName(modelChoices[0]?.value ?? '')
  }, [modelChoices, modelName])

  const reset = () => {
    setName('')
    setDescription('')
    setGroup('process')
    setModelGroup(groupChoices[0]?.value ?? 'default')
    setModelName(modelChoices[0]?.value ?? '')
    setInputPorts([])
    setOutputPorts([])
    setTags('')
  }

  const handleSubmit = async () => {
    if (!name.trim()) {
      toast.error('请输入步骤名称')
      return
    }
    if (!description.trim()) {
      toast.error('请输入步骤要求')
      return
    }
    if (outputPorts.length === 0) {
      toast.error('请至少选择一个输出字段')
      return
    }
    setIsSubmitting(true)
    try {
      await onCreateComponent({
        name: name.trim(),
        description: description.trim(),
        group,
        model_group: modelGroup,
        model_name: modelName,
        inputs: componentPortLabels(inputPorts, CUSTOM_COMPONENT_INPUT_OPTIONS),
        outputs: componentPortLabels(
          outputPorts,
          CUSTOM_COMPONENT_OUTPUT_OPTIONS
        ),
        settings: componentInputSettings(inputPorts),
        tags: splitListInput(tags),
      })
      reset()
      setOpen(false)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '创建步骤失败')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className='border-border/60 mb-2 rounded-[9px] border p-2'>
      <button
        type='button'
        className='flex w-full items-center justify-between gap-2 text-left'
        onClick={() => setOpen((value) => !value)}
      >
        <span className='flex min-w-0 items-center gap-2'>
          <WrenchIcon className='text-muted-foreground size-3.5 shrink-0' />
          <span className='truncate text-[11px] font-medium'>自定义步骤</span>
          {isLoading && (
            <span className='text-muted-foreground text-[10px]'>同步中</span>
          )}
        </span>
        {open ? (
          <ChevronUpIcon className='text-muted-foreground size-3.5' />
        ) : (
          <ChevronDownIcon className='text-muted-foreground size-3.5' />
        )}
      </button>
      {open && (
        <div className='mt-2 flex flex-col gap-2'>
          <div className='bg-muted/30 rounded-[8px] px-2 py-1.5'>
            <div className='text-[11px] font-semibold'>创建自定义步骤</div>
          </div>

          <div className='grid gap-1'>
            <span className='text-muted-foreground px-0.5 text-[10px] font-medium'>
              步骤名称
            </span>
            <Input
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder='例如：检查异常数据'
              className='h-7 rounded-[6px] text-xs'
            />
          </div>

          <div className='grid gap-1'>
            <span className='text-muted-foreground px-0.5 text-[10px] font-medium'>
              步骤要求
            </span>
            <Textarea
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              placeholder='说明它如何处理上游输入，以及要产出什么结果'
              className='min-h-[60px] resize-none rounded-[6px] text-xs'
            />
          </div>

          <div className='grid gap-1'>
            <span className='text-muted-foreground px-0.5 text-[10px] font-medium'>
              使用模型
            </span>
            <div className='grid grid-cols-2 gap-1.5'>
              <NativeSelect
                value={modelGroup}
                onChange={(event) => setModelGroup(event.target.value)}
                className='h-7 rounded-[6px] text-xs'
              >
                {groupChoices.map((option) => (
                  <NativeSelectOption key={option.value} value={option.value}>
                    {option.label}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
              <NativeSelect
                value={modelName}
                onChange={(event) => setModelName(event.target.value)}
                className='h-7 rounded-[6px] text-xs'
              >
                {modelChoices.map((option) => (
                  <NativeSelectOption key={option.value} value={option.value}>
                    {option.label}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </div>
          </div>

          <div className='grid gap-1'>
            <span className='text-muted-foreground px-0.5 text-[10px] font-medium'>
              所属阶段
            </span>
            <NativeSelect
              value={group}
              onChange={(event) => setGroup(event.target.value)}
              className='h-7 rounded-[6px] text-xs'
            >
              <NativeSelectOption value='trigger'>触发</NativeSelectOption>
              <NativeSelectOption value='collect'>采集</NativeSelectOption>
              <NativeSelectOption value='process'>处理</NativeSelectOption>
              <NativeSelectOption value='output'>落档</NativeSelectOption>
            </NativeSelect>
          </div>

          <ComponentPortPicker
            label='输入字段'
            emptyLabel='选择输入字段'
            options={CUSTOM_COMPONENT_INPUT_OPTIONS}
            values={inputPorts}
            onChange={setInputPorts}
          />

          <ComponentPortPicker
            label='输出字段'
            emptyLabel='选择输出字段'
            options={CUSTOM_COMPONENT_OUTPUT_OPTIONS}
            values={outputPorts}
            onChange={setOutputPorts}
          />

          <div className='grid gap-1'>
            <span className='text-muted-foreground px-0.5 text-[10px] font-medium'>
              标签
            </span>
            <Input
              value={tags}
              onChange={(event) => setTags(event.target.value)}
              placeholder='可选，逗号分隔'
              className='h-7 rounded-[6px] text-xs'
            />
          </div>

          <Button
            type='button'
            size='sm'
            className='h-7 rounded-[6px] text-xs'
            disabled={isSubmitting}
            onClick={handleSubmit}
          >
            创建步骤
          </Button>
        </div>
      )}
    </div>
  )
}

function ComponentPortPicker({
  label,
  emptyLabel,
  options,
  values,
  onChange,
}: {
  label: string
  emptyLabel: string
  options: ComponentPortOption[]
  values: string[]
  onChange: (values: string[]) => void
}) {
  const [pendingValue, setPendingValue] = useState('')
  const availableOptions = options.filter(
    (option) => !values.includes(option.value)
  )
  const addPort = () => {
    if (!pendingValue || values.includes(pendingValue)) return
    onChange([...values, pendingValue])
    setPendingValue('')
  }

  return (
    <div className='grid gap-1'>
      <span className='text-muted-foreground px-0.5 text-[10px] font-medium'>
        {label}
      </span>
      <div className='grid grid-cols-[minmax(0,1fr)_28px] gap-1'>
        <NativeSelect
          value={pendingValue}
          onChange={(event) => setPendingValue(event.target.value)}
          className='h-7 rounded-[6px] text-xs'
        >
          <NativeSelectOption value=''>{emptyLabel}</NativeSelectOption>
          {availableOptions.map((option) => (
            <NativeSelectOption key={option.value} value={option.value}>
              {option.label}
            </NativeSelectOption>
          ))}
        </NativeSelect>
        <Button
          type='button'
          size='icon-sm'
          variant='outline'
          className='size-7 rounded-[6px]'
          disabled={!pendingValue}
          onClick={addPort}
          title={`添加${label}`}
        >
          <PlusIcon className='size-3.5' />
          <span className='sr-only'>{`添加${label}`}</span>
        </Button>
      </div>
      {values.length > 0 && (
        <div className='flex flex-wrap gap-1'>
          {values.map((value) => {
            const option = options.find(
              (candidate) => candidate.value === value
            )
            if (!option) return null
            return (
              <button
                key={value}
                type='button'
                className='border-border/70 bg-muted/45 hover:bg-muted inline-flex h-6 items-center gap-1 rounded-[5px] border px-1.5 text-[10px]'
                onClick={() =>
                  onChange(values.filter((item) => item !== value))
                }
                title={`移除${option.label}`}
              >
                <span>{option.label}</span>
                <XIcon className='size-3' />
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}

function CompactPalette({
  groups,
  onAddNode,
}: {
  groups: PaletteTemplateGroup[]
  onAddNode: (templateKey: AgentNodeKind | string) => void
}) {
  const templates = groups.flatMap((group) => group.templates)
  return (
    <div className='no-scrollbar flex min-w-0 gap-1 overflow-x-auto'>
      {templates.map((template) => {
        const meta = getAgentNodeVisualMeta(template.kind, template.componentId)
        const Icon = meta.icon
        const templateKey = template.componentId ?? template.kind
        return (
          <button
            key={templateKey}
            type='button'
            className='border-border bg-background flex h-8 shrink-0 items-center gap-1.5 rounded-[8px] border px-2 text-xs'
            draggable
            onClick={() => onAddNode(templateKey)}
            onDragStart={(event) => {
              event.dataTransfer.setData(DRAG_NODE_TYPE, templateKey)
              event.dataTransfer.effectAllowed = 'move'
            }}
          >
            <Icon className='size-3.5' />
            {template.title}
          </button>
        )
      })}
    </div>
  )
}

function PaletteItem({
  template,
  onAddNode,
  onDeleteComponent,
}: {
  template: NodeTemplate
  onAddNode: (templateKey: AgentNodeKind | string) => void
  onDeleteComponent: (componentId: string) => Promise<void>
}) {
  const kind = template.kind
  const meta = getAgentNodeVisualMeta(kind, template.componentId)
  const Icon = meta.icon
  const templateKey = template.componentId ?? kind
  const isCustom = template.source === 'custom'
  const canDelete = isCustom && !template.componentId?.startsWith('component.')

  return (
    <button
      type='button'
      className='hover:bg-accent/60 group flex w-full items-start gap-2.5 rounded-[7px] px-2 py-2 text-left transition-colors'
      draggable
      onClick={() => onAddNode(templateKey)}
      onDragStart={(event) => {
        event.dataTransfer.setData(DRAG_NODE_TYPE, templateKey)
        event.dataTransfer.effectAllowed = 'move'
      }}
    >
      <span
        className={cn(
          'mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-[6px]',
          meta.accent
        )}
      >
        <Icon className='size-3.5' />
      </span>
      <span className='min-w-0 flex-1'>
        <span className='flex items-center gap-1.5'>
          <span className='block min-w-0 truncate text-xs font-medium'>
            {template.title}
          </span>
          <span
            className={cn(
              'rounded px-1 text-[9px]',
              isCustom
                ? 'bg-amber-500/10 text-amber-700'
                : 'bg-emerald-500/10 text-emerald-700'
            )}
          >
            {isCustom ? '自定义' : '内置'}
          </span>
        </span>
        <span className='text-muted-foreground line-clamp-2 block text-[10px] leading-snug break-words whitespace-normal'>
          {template.description}
        </span>
      </span>
      {canDelete ? (
        <span
          role='button'
          tabIndex={0}
          className='text-muted-foreground hover:text-destructive mt-1.5 shrink-0 rounded-[5px] p-1 transition-colors'
          onClick={(event) => {
            event.stopPropagation()
            if (template.componentId) {
              void onDeleteComponent(template.componentId)
            }
          }}
          onKeyDown={(event) => {
            if (
              (event.key === 'Enter' || event.key === ' ') &&
              template.componentId
            ) {
              event.stopPropagation()
              void onDeleteComponent(template.componentId)
            }
          }}
          title='删除自定义步骤'
        >
          <Trash2Icon className='size-3' />
        </span>
      ) : (
        <PlusIcon className='text-muted-foreground mt-2 size-3 shrink-0' />
      )}
    </button>
  )
}

function CanvasToolbarButton({
  children,
  disabled,
  label,
  onClick,
  variant = 'ghost',
}: {
  children: ReactNode
  disabled?: boolean
  label: string
  onClick: () => void
  variant?: 'ghost' | 'destructive'
}) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            type='button'
            size='icon-sm'
            variant={variant}
            className='size-8 rounded-[6px]'
            disabled={disabled}
            title={label}
            onClick={onClick}
          />
        }
      >
        {children}
        <span className='sr-only'>{label}</span>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}

function DeletableEdge({
  id,
  data,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  markerEnd,
  style,
  selected,
  onDeleteEdge,
}: EdgeProps<AgentFlowEdge> & { onDeleteEdge: (edgeId: string) => void }) {
  const [isHovered, setIsHovered] = useState(false)
  const [edgePath, labelX, labelY] = getSmoothStepPath({
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
  })
  const showButton = isHovered || selected

  return (
    <>
      <BaseEdge id={id} path={edgePath} markerEnd={markerEnd} style={style} />
      {/* Wider invisible path to make hovering the thin edge easier */}
      <path
        d={edgePath}
        fill='none'
        stroke='transparent'
        strokeWidth={20}
        onMouseEnter={() => setIsHovered(true)}
        onMouseLeave={() => setIsHovered(false)}
      />
      <EdgeLabelRenderer>
        {data?.routeLabel && (
          <span
            className={cn(
              'bg-background pointer-events-none absolute rounded-[5px] border px-1.5 py-0.5 text-[10px] font-medium shadow-sm',
              data.branchRoute === 'true'
                ? 'border-emerald-500/35 text-emerald-700 dark:text-emerald-300'
                : data.branchRoute === 'false'
                  ? 'border-rose-500/35 text-rose-700 dark:text-rose-300'
                  : data.routeKey === SWITCH_DEFAULT_HANDLE ||
                      data.routeKey === LOOP_DONE_HANDLE
                    ? 'border-slate-500/35 text-slate-700 dark:text-slate-300'
                    : data.routeKind === 'loop'
                      ? 'border-indigo-500/35 text-indigo-700 dark:text-indigo-300'
                      : 'border-sky-500/35 text-sky-700 dark:text-sky-300'
            )}
            style={{
              transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY - 15}px)`,
            }}
          >
            {data.routeLabel}
          </span>
        )}
        <button
          type='button'
          className={cn(
            'border-border/70 bg-background text-muted-foreground hover:border-destructive hover:text-destructive absolute flex size-5 items-center justify-center rounded-full border shadow-sm transition-opacity',
            showButton ? 'opacity-100' : 'pointer-events-none opacity-0'
          )}
          style={{
            transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY + (data?.routeLabel ? 10 : 0)}px)`,
            pointerEvents: showButton ? 'all' : 'none',
          }}
          onMouseEnter={() => setIsHovered(true)}
          onMouseLeave={() => setIsHovered(false)}
          onClick={(event) => {
            event.stopPropagation()
            onDeleteEdge(id)
          }}
          title='删除连线'
        >
          <XIcon className='size-3' />
        </button>
      </EdgeLabelRenderer>
    </>
  )
}

function BlueprintNode({
  id,
  data,
  selected,
  isConnectable,
  skillChoices,
  onSetSkill,
}: NodeProps<AgentFlowNode> & {
  skillChoices: SkillChoice[]
  onSetSkill: (nodeId: string, skill: string) => void
}) {
  const meta = getAgentNodeVisualMeta(data.kind, data.componentId)
  const Icon = meta.icon
  const canReceive = data.inputs.length > 0
  const canSend = data.outputs.length > 0
  const routePorts = getAgentRoutePorts(data)
  const updateNodeInternals = useUpdateNodeInternals()
  const routePortSignature = routePorts
    .map((port) => `${port.id}:${port.label}`)
    .join('|')

  useEffect(() => {
    updateNodeInternals(id)
  }, [id, routePortSignature, updateNodeInternals])

  return (
    <div
      className={cn(
        'bg-background border-border/80 relative w-72 rounded-[8px] border shadow-sm transition-shadow',
        selected && 'ring-primary/40 ring-3'
      )}
    >
      {canReceive && (
        <>
          <Handle
            className='!border-background !bg-primary !size-3'
            isConnectable={isConnectable}
            position={Position.Left}
            type='target'
          />
          <span
            className='bg-background text-muted-foreground border-border/70 pointer-events-none absolute left-[-8px] z-10 rounded-[5px] border px-1.5 py-0.5 text-[10px] whitespace-nowrap shadow-sm'
            style={{ top: '50%', transform: 'translate(-100%, -50%)' }}
          >
            输入
          </span>
        </>
      )}
      {routePorts.length > 0
        ? routePorts.map((port, index) => {
            const top = `${routePortTop(index, routePorts.length)}%`
            return (
              <div key={port.id}>
                <Handle
                  className={cn(
                    '!border-background !size-3',
                    routePortHandleClass(port)
                  )}
                  id={port.id}
                  isConnectable={isConnectable}
                  position={Position.Right}
                  style={{ top }}
                  type='source'
                />
                <span
                  className={cn(
                    'bg-background pointer-events-none absolute right-[-8px] z-10 max-w-36 truncate rounded-[5px] border px-1.5 py-0.5 text-[10px] font-medium whitespace-nowrap shadow-sm',
                    routePortLabelClass(port)
                  )}
                  style={{ top, transform: 'translate(100%, -50%)' }}
                  title={port.label}
                >
                  {port.label}
                </span>
              </div>
            )
          })
        : canSend && (
            <>
              <Handle
                className='!border-background !bg-primary !size-3'
                isConnectable={isConnectable}
                position={Position.Right}
                type='source'
              />
              <span
                className='bg-background text-muted-foreground border-border/70 pointer-events-none absolute right-[-8px] z-10 rounded-[5px] border px-1.5 py-0.5 text-[10px] whitespace-nowrap shadow-sm'
                style={{ top: '50%', transform: 'translate(100%, -50%)' }}
              >
                输出 · 可多连
              </span>
            </>
          )}

      <div className='flex items-start gap-2 border-b px-3 py-2'>
        <div
          className={cn(
            'flex size-8 shrink-0 items-center justify-center rounded-[8px]',
            meta.accent
          )}
        >
          <Icon className='size-4' />
        </div>
        <div className='min-w-0 flex-1'>
          <div className='flex min-w-0 items-center gap-2'>
            <div className='truncate text-sm font-semibold'>{data.title}</div>
            <StatusDot status={data.status} />
          </div>
          <div className='text-muted-foreground truncate text-[11px]'>
            {meta.label} · {meta.group}
          </div>
        </div>
      </div>

      <div className='space-y-2 px-3 py-2.5'>
        <p className='text-muted-foreground line-clamp-2 text-xs leading-relaxed'>
          {data.description}
        </p>
        <div className='bg-muted/45 flex items-center gap-1.5 rounded-[6px] px-2 py-1.5'>
          <SparklesIcon className='text-muted-foreground size-3.5 shrink-0' />
          <span className='text-muted-foreground shrink-0 text-[11px]'>
            模型
          </span>
          <span className='min-w-0 truncate text-xs font-medium'>
            {data.model}
          </span>
        </div>
        <SkillLinkBadge
          skill={data.skill}
          nodeKind={data.kind}
          skillChoices={skillChoices}
          onSelectSkill={(value) => onSetSkill(id, value)}
        />
        <div className='grid grid-cols-2 gap-2'>
          <IoList label='输入' values={data.inputs} />
          <IoList label='输出' values={data.outputs} />
        </div>
      </div>
    </div>
  )
}

type AgentRoutePort = {
  id: string
  kind: AgentRouteKind
  label: string
  route?: AgentBranchRoute
  isDefault?: boolean
}

function getAgentRoutePorts(data: AgentNodeData): AgentRoutePort[] {
  if (data.componentId === 'component.branch') {
    return [
      {
        id: BRANCH_TRUE_HANDLE,
        kind: 'branch',
        label: '条件成立',
        route: 'true',
      },
      {
        id: BRANCH_FALSE_HANDLE,
        kind: 'branch',
        label: '条件不成立',
        route: 'false',
      },
    ]
  }
  if (data.componentId === 'component.loop') {
    return [
      { id: LOOP_ITEM_HANDLE, kind: 'loop', label: '逐项处理' },
      {
        id: LOOP_DONE_HANDLE,
        kind: 'loop',
        label: '循环完成',
        isDefault: true,
      },
    ]
  }
  if (data.componentId !== 'component.switch') return []
  const branches = parseSwitchBranchSettings(data.settings['分支配置'])
  return [
    ...branches.map((branch, index) => ({
      id: `${SWITCH_HANDLE_PREFIX}${index}`,
      kind: 'switch' as const,
      label: branch.label,
    })),
    {
      id: SWITCH_DEFAULT_HANDLE,
      kind: 'switch' as const,
      label: data.settings['默认分支']?.trim() || '其他',
      isDefault: true,
    },
  ]
}

function parseSwitchBranchSettings(value?: string) {
  return (value || '')
    .split(/[\r\n;；]+/)
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const separator = line.includes('=') ? '=' : ':'
      const [rawValue, ...labelParts] = line.split(separator)
      const branchValue = rawValue.trim()
      const label = labelParts.join(separator).trim() || branchValue
      return { value: branchValue, label }
    })
    .filter((branch) => branch.value)
    .slice(0, 6)
}

function routePortTop(index: number, count: number) {
  if (count <= 1) return 50
  return 28 + (index * 52) / (count - 1)
}

function routePortHandleClass(port: AgentRoutePort) {
  if (port.route === 'true') return '!bg-emerald-500'
  if (port.route === 'false') return '!bg-rose-500'
  if (port.isDefault) return '!bg-slate-500'
  if (port.kind === 'loop') return '!bg-indigo-500'
  return '!bg-sky-500'
}

function routePortLabelClass(port: AgentRoutePort) {
  if (port.route === 'true') {
    return 'border-emerald-500/35 text-emerald-700 dark:text-emerald-300'
  }
  if (port.route === 'false') {
    return 'border-rose-500/35 text-rose-700 dark:text-rose-300'
  }
  if (port.isDefault) {
    return 'border-slate-500/35 text-slate-700 dark:text-slate-300'
  }
  if (port.kind === 'loop') {
    return 'border-indigo-500/35 text-indigo-700 dark:text-indigo-300'
  }
  return 'border-sky-500/35 text-sky-700 dark:text-sky-300'
}

function StatusDot({ status }: { status: AgentNodeStatus }) {
  const labelMap: Record<AgentNodeStatus, string> = {
    ready: '就绪',
    waiting: '等待',
    running: '运行',
    done: '完成',
    failed: '失败',
  }
  return (
    <span
      className={cn(
        'inline-flex h-5 shrink-0 items-center gap-1 rounded-full px-1.5 text-[10px]',
        status === 'ready' && 'bg-muted text-muted-foreground',
        status === 'waiting' && 'bg-amber-500/12 text-amber-700',
        status === 'running' && 'bg-blue-500/12 text-blue-700',
        status === 'done' && 'bg-emerald-500/12 text-emerald-700',
        status === 'failed' && 'bg-destructive/10 text-destructive'
      )}
    >
      <span
        className={cn(
          'size-1.5 rounded-full',
          status === 'ready' && 'bg-muted-foreground',
          status === 'waiting' && 'bg-amber-500',
          status === 'running' && 'bg-blue-500',
          status === 'done' && 'bg-emerald-500',
          status === 'failed' && 'bg-destructive'
        )}
      />
      {labelMap[status]}
    </span>
  )
}

function SkillLinkBadge({
  skill,
  nodeKind,
  skillChoices,
  onSelectSkill,
}: {
  skill: string
  nodeKind: AgentNodeKind
  skillChoices: SkillChoice[]
  onSelectSkill: (value: string) => void
}) {
  const hasSkill = Boolean(skill)
  const skillChoice = getSkillChoice(skill, skillChoices)

  return (
    <div
      className={cn(
        'flex items-center gap-1.5 rounded-[6px] border px-2 py-1.5',
        hasSkill
          ? 'border-fuchsia-500/20 bg-fuchsia-500/8'
          : 'border-border/60 border-dashed'
      )}
    >
      <span
        className={cn(
          'flex shrink-0 items-center gap-1.5 text-[11px] font-medium',
          hasSkill
            ? 'text-fuchsia-700 dark:text-fuchsia-300'
            : 'text-muted-foreground'
        )}
      >
        <span
          className={cn(
            'size-2 rounded-full',
            hasSkill ? 'bg-fuchsia-500' : 'bg-muted-foreground/40'
          )}
        />
        Skill
      </span>
      {!hasSkill && (
        <PlusIcon className='text-muted-foreground size-3 shrink-0' />
      )}
      <select
        value={skill}
        onClick={(event) => event.stopPropagation()}
        onPointerDown={(event) => event.stopPropagation()}
        onChange={(event) => {
          event.stopPropagation()
          onSelectSkill(event.target.value)
        }}
        className={cn(
          'min-w-0 flex-1 cursor-pointer truncate bg-transparent text-xs font-medium outline-none',
          !hasSkill && 'text-muted-foreground'
        )}
        title={hasSkill ? skillChoice.description : '点击添加 Skill'}
      >
        <option value={NO_SKILL_CHOICE.value}>{NO_SKILL_CHOICE.label}</option>
        {skillChoicesForNodeKind(skillChoices, nodeKind).map((choice) => (
          <option key={choice.value} value={choice.value}>
            {choice.label}
          </option>
        ))}
      </select>
    </div>
  )
}

function IoList({ label, values }: { label: string; values: string[] }) {
  return (
    <div className='min-w-0'>
      <div className='text-muted-foreground mb-1 text-[10px]'>{label}</div>
      <div className='flex min-h-10 flex-col gap-1'>
        {values.length === 0 ? (
          <span className='text-muted-foreground text-[11px]'>无</span>
        ) : (
          values.slice(0, 2).map((value) => (
            <span
              key={value}
              className='border-border/70 bg-muted/35 truncate rounded-[6px] border px-1.5 py-0.5 text-[11px]'
              title={value}
            >
              {value}
            </span>
          ))
        )}
      </div>
    </div>
  )
}

function Inspector({
  selectedNode,
  blueprintId,
  blueprintChoices,
  modelChoices,
  groupChoices,
  skillChoices,
  currentGroup,
  onDeleteNode,
  onUpdateNode,
  onUpdateSetting,
}: {
  selectedNode: AgentFlowNode | null
  blueprintId: string | null
  blueprintChoices: BlueprintChoice[]
  modelChoices: ModelChoice[]
  groupChoices: GroupOption[]
  skillChoices: SkillChoice[]
  currentGroup?: string
  onDeleteNode: (nodeId: string) => void
  onUpdateNode: (patch: Partial<AgentNodeData>) => void
  onUpdateSetting: (key: string, value: string) => void
}) {
  return (
    <div className='flex min-h-0 flex-1 flex-col'>
      {/* 顶部标题栏 */}
      <div className='border-border/70 shrink-0 border-b px-3 pt-3'>
        <div className='mb-3 flex items-center gap-2'>
          <div className='bg-muted flex size-7 shrink-0 items-center justify-center rounded-[7px]'>
            <Settings2Icon className='size-3.5' />
          </div>
          <span className='text-sm font-semibold'>节点配置</span>
        </div>
      </div>

      <div className='min-h-0 min-w-0 flex-1 overflow-x-hidden overflow-y-auto'>
        {selectedNode ? (
          <div className='min-w-0 space-y-4 overflow-hidden p-3'>
            <NodeConfigPanel
              blueprintId={blueprintId}
              blueprintChoices={blueprintChoices}
              currentGroup={currentGroup}
              groupChoices={groupChoices}
              modelChoices={modelChoices}
              node={selectedNode}
              skillChoices={skillChoices}
              onDeleteNode={onDeleteNode}
              onUpdateNode={onUpdateNode}
              onUpdateSetting={onUpdateSetting}
            />
          </div>
        ) : (
          <div className='text-muted-foreground p-6 text-center text-sm'>
            选中画布上的步骤后查看和配置。
          </div>
        )}
      </div>
    </div>
  )
}

function ExecutionLogPanel({
  executionNodes,
  executionLog,
}: {
  executionNodes: AgentFlowNode[]
  executionLog: AgentExecutionLogMap
}) {
  if (executionNodes.length === 0) {
    return (
      <div className='text-muted-foreground flex flex-col items-center justify-center gap-2 p-8 text-center text-xs'>
        <FileSpreadsheetIcon className='size-8 opacity-30' />
        <p>暂无执行记录</p>
        <p className='opacity-60'>
          点击「试运行」后此处会显示每个节点的执行状态
        </p>
      </div>
    )
  }

  const hasDetailedLogs = Object.keys(executionLog).length > 0

  return (
    <div className='flex flex-col gap-2 p-3'>
      {!hasDetailedLogs && (
        <div className='text-muted-foreground border-border/70 rounded-[8px] border border-dashed px-3 py-2 text-xs'>
          还没有生成详细日志。点击「试运行」后会记录每个节点的输入、输出、步骤和耗时。
        </div>
      )}
      {executionNodes.map((node) => {
        const meta = getAgentNodeVisualMeta(
          node.data.kind,
          node.data.componentId
        )
        const Icon = meta.icon
        const status = node.data.status
        const log = executionLog[node.id]
        const logStatus = log?.status ?? statusToLogStatus(status)

        return (
          <div
            key={node.id}
            className={cn(
              'rounded-[8px] border p-2.5',
              status === 'running' && 'border-blue-500/30 bg-blue-500/5',
              status === 'done' && 'border-emerald-500/30 bg-emerald-500/5',
              status === 'failed' && 'border-destructive/30 bg-destructive/5',
              status === 'waiting' && 'border-border/50 bg-muted/10',
              status === 'ready' && 'border-border/50 bg-muted/10'
            )}
          >
            <div className='flex items-center gap-2'>
              <div
                className={cn(
                  'flex size-6 shrink-0 items-center justify-center rounded-[6px]',
                  meta.accent
                )}
              >
                <Icon className='size-3' />
              </div>
              <div className='min-w-0 flex-1'>
                <div className='truncate text-[11px] font-medium'>
                  {node.data.title}
                </div>
                <div className='text-muted-foreground truncate text-[10px]'>
                  {node.data.model}
                </div>
              </div>
              <StatusDot status={status} />
            </div>

            <div className='mt-2 space-y-2 pl-8'>
              <div className='flex flex-wrap items-center gap-1.5 text-[10px]'>
                <span
                  className={cn(
                    'rounded-full px-1.5 py-0.5 font-medium',
                    logStatus === 'running' && 'bg-blue-500/12 text-blue-700',
                    logStatus === 'completed' &&
                      'bg-emerald-500/12 text-emerald-700',
                    logStatus === 'pending' && 'bg-amber-500/12 text-amber-700',
                    logStatus === 'stopped' && 'bg-slate-500/12 text-slate-700',
                    logStatus === 'failed' &&
                      'bg-destructive/10 text-destructive'
                  )}
                >
                  {formatLogStatus(logStatus)}
                </span>
                <span className='text-muted-foreground'>
                  开始{' '}
                  {log?.startedAt ? formatLogTime(log.startedAt) : '未开始'}
                </span>
                <span className='text-muted-foreground'>
                  耗时 {formatDuration(log?.durationMs)}
                </span>
              </div>

              <p className='text-muted-foreground text-[10px] leading-relaxed'>
                {log?.message ??
                  '节点尚未开始执行，等待上游节点完成后进入队列。'}
              </p>

              {log && (
                <>
                  <LogSummaryList title='输入' values={log.inputSummary} />
                  <LogSummaryList title='输出' values={log.outputSummary} />
                  <ModelOutputBlock output={log.modelOutput} />
                  <div>
                    <div className='text-muted-foreground mb-1 text-[10px] font-medium'>
                      步骤
                    </div>
                    <ol className='space-y-1'>
                      {log.steps.map((step, stepIndex) => (
                        <li
                          key={`${node.id}-step-${stepIndex}`}
                          className='text-muted-foreground flex gap-1.5 text-[10px] leading-relaxed'
                        >
                          <span className='bg-muted text-foreground mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full text-[9px]'>
                            {stepIndex + 1}
                          </span>
                          <span>{step}</span>
                        </li>
                      ))}
                    </ol>
                  </div>

                  {log.error && (
                    <div className='text-destructive border-destructive/30 rounded-[6px] border px-2 py-1.5 text-[10px]'>
                      {log.error}
                    </div>
                  )}
                </>
              )}
            </div>
          </div>
        )
      })}
    </div>
  )
}

function LogSummaryList({
  title,
  values,
}: {
  title: string
  values: string[]
}) {
  return (
    <div>
      <div className='text-muted-foreground mb-1 text-[10px] font-medium'>
        {title}
      </div>
      <div className='space-y-1'>
        {values.length === 0 ? (
          <div className='text-muted-foreground text-[10px]'>无</div>
        ) : (
          values.map((value, index) => (
            <div
              key={`${title}-${index}-${value}`}
              className='bg-background/70 text-muted-foreground rounded-[6px] px-2 py-1 text-[10px] leading-relaxed'
            >
              {value}
            </div>
          ))
        )}
      </div>
    </div>
  )
}

function ModelOutputBlock({ output }: { output: string }) {
  return (
    <div>
      <div className='text-muted-foreground mb-1 text-[10px] font-medium'>
        模型输出结果
      </div>
      <div className='bg-background/80 border-border/60 max-h-32 overflow-y-auto rounded-[6px] border px-2 py-1.5 text-[10px] leading-relaxed whitespace-pre-wrap'>
        {output || '暂无模型输出。'}
      </div>
    </div>
  )
}

function formatLogStatus(status: AgentNodeLogStatus) {
  const labels: Record<AgentNodeLogStatus, string> = {
    pending: '等待',
    running: '执行中',
    completed: '完成',
    stopped: '已停止',
    failed: '执行失败',
  }
  return labels[status]
}

function statusToLogStatus(status: AgentNodeStatus): AgentNodeLogStatus {
  if (status === 'running') return 'running'
  if (status === 'done') return 'completed'
  if (status === 'failed') return 'failed'
  return 'pending'
}

function formatLogTime(timestamp: number) {
  const date = new Date(timestamp)
  const hour = `${date.getHours()}`.padStart(2, '0')
  const minute = `${date.getMinutes()}`.padStart(2, '0')
  const second = `${date.getSeconds()}`.padStart(2, '0')
  return `${hour}:${minute}:${second}`
}

function formatDuration(durationMs?: number) {
  if (!durationMs || durationMs <= 0) return '-'
  if (durationMs < 1000) return `${durationMs}ms`
  return `${(durationMs / 1000).toFixed(1)}s`
}

type ExecutionPreviewSummaryStatus =
  | 'idle'
  | 'running'
  | 'completed'
  | 'failed'
  | 'paused'

type ExecutionPreviewSummary = {
  status: ExecutionPreviewSummaryStatus
  hasExecution: boolean
  hasNodeProgress: boolean
  isProgressIndeterminate: boolean
  totalNodes: number
  completedNodes: number
  progressPercent: number
  currentNodeId?: string
  elapsedMs?: number
}

function formatElapsedTime(durationMs?: number) {
  if (
    durationMs === undefined ||
    !Number.isFinite(durationMs) ||
    durationMs < 0
  ) {
    return '-'
  }

  const totalSeconds = Math.floor(durationMs / 1000)
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  if (hours > 0) {
    return `${hours.toString().padStart(2, '0')}:${minutes
      .toString()
      .padStart(2, '0')}:${seconds.toString().padStart(2, '0')}`
  }
  return `${minutes.toString().padStart(2, '0')}:${seconds
    .toString()
    .padStart(2, '0')}`
}

function formatExecutionPreviewStatus(status: ExecutionPreviewSummaryStatus) {
  const labels: Record<ExecutionPreviewSummaryStatus, string> = {
    idle: '未执行',
    running: '执行中',
    completed: '已完成',
    failed: '执行失败',
    paused: '已暂停',
  }
  return labels[status]
}

function buildExecutionPreviewSummary(
  executionNodes: AgentFlowNode[],
  executionLog: AgentExecutionLogMap,
  latestBackendLog: AgentExecutionLog | undefined,
  now: number,
  blueprintStatus?: AgentBlueprintStatus
): ExecutionPreviewSummary {
  const loggedNodeIds = new Set(Object.keys(executionLog))
  const loggedNodes = executionNodes.filter((node) =>
    loggedNodeIds.has(node.id)
  )
  const hasUnreconciledPersistedStatus =
    !latestBackendLog &&
    (blueprintStatus === 'failed' || blueprintStatus === 'paused')
  const progressNodes =
    loggedNodes.length > 0
      ? loggedNodes
      : hasUnreconciledPersistedStatus
        ? []
        : executionNodes
  const backendStatus = latestBackendLog?.status
  const backendGraph = latestBackendLog
    ? parseBackendGraphOutput(latestBackendLog.output)
    : null
  const nodeStatuses = progressNodes.map(
    (node) =>
      executionLog[node.id]?.status ?? statusToLogStatus(node.data.status)
  )
  const localRunningNode = progressNodes.find(
    (node) =>
      executionLog[node.id]?.status === 'running' ||
      node.data.status === 'running'
  )
  const hasLocalRunningNode = nodeStatuses.includes('running')
  const hasLocalStoppedNode = nodeStatuses.includes('stopped')
  const hasLocalFailedNode = nodeStatuses.includes('failed')
  const hasBackendRunningLog =
    backendStatus === 'running' || backendStatus === 'pending'
  const hasPersistedTerminalFailure =
    !latestBackendLog && blueprintStatus === 'failed'
  const hasPersistedPausedStatus =
    !latestBackendLog && blueprintStatus === 'paused'

  let status: ExecutionPreviewSummaryStatus = 'idle'
  if (backendStatus === 'failed' || hasPersistedTerminalFailure) {
    status = 'failed'
  } else if (backendStatus === 'paused' || hasPersistedPausedStatus) {
    status = 'paused'
  } else if (backendStatus === 'completed') {
    status = 'completed'
  } else if (hasLocalStoppedNode) {
    status = 'paused'
  } else if (hasLocalFailedNode) {
    status = 'failed'
  } else if (hasLocalRunningNode || hasBackendRunningLog) {
    status = 'running'
  } else if (nodeStatuses.includes('completed')) {
    status = 'completed'
  }

  const hasNodeProgress =
    progressNodes.length > 0 ||
    nodeStatuses.some((nodeStatus) => nodeStatus !== 'pending')
  const hasExecution =
    Boolean(latestBackendLog) ||
    hasNodeProgress ||
    hasPersistedTerminalFailure ||
    hasPersistedPausedStatus
  const totalNodes = progressNodes.length
  const completedNodeCount = backendGraph?.nodes
    ? backendGraph.nodes.filter((node) => node.status === 'completed').length
    : latestBackendLog?.status === 'failed' ||
        latestBackendLog?.status === 'paused'
      ? 0
      : nodeStatuses.filter((nodeStatus) => nodeStatus === 'completed').length
  const completedNodes =
    hasNodeProgress && status === 'completed' && backendStatus === 'completed'
      ? totalNodes
      : completedNodeCount
  const progressPercent =
    totalNodes > 0
      ? Math.min(
          100,
          Math.max(0, Math.round((completedNodes / totalNodes) * 100))
        )
      : 0

  const localStartTimes = Object.values(executionLog)
    .map((log) => log.startedAt)
    .filter(
      (timestamp): timestamp is number =>
        typeof timestamp === 'number' &&
        Number.isFinite(timestamp) &&
        timestamp > 0
    )
  const localEndTimes = Object.values(executionLog)
    .map((log) => log.endedAt)
    .filter(
      (timestamp): timestamp is number =>
        typeof timestamp === 'number' &&
        Number.isFinite(timestamp) &&
        timestamp > 0
    )
  const localDurations = Object.values(executionLog)
    .map((log) => log.durationMs)
    .filter(
      (duration): duration is number =>
        typeof duration === 'number' &&
        Number.isFinite(duration) &&
        duration > 0
    )
  const localStartedAt =
    localStartTimes.length > 0 ? Math.min(...localStartTimes) : undefined
  const localEndedAt =
    localEndTimes.length > 0 ? Math.max(...localEndTimes) : undefined
  const localDurationMs =
    localDurations.length > 0 ? Math.max(...localDurations) : undefined
  const localElapsedMs =
    localStartedAt !== undefined && localEndedAt !== undefined
      ? Math.max(0, localEndedAt - localStartedAt)
      : localDurationMs
  const backendDurationMs =
    typeof latestBackendLog?.duration_ms === 'number' &&
    Number.isFinite(latestBackendLog.duration_ms) &&
    latestBackendLog.duration_ms > 0
      ? latestBackendLog.duration_ms
      : undefined

  const startedAt =
    status === 'running'
      ? (localStartedAt ?? latestBackendLog?.start_time)
      : hasLocalStoppedNode || hasLocalFailedNode
        ? (localStartedAt ?? latestBackendLog?.start_time)
        : (latestBackendLog?.start_time ?? localStartedAt)
  const elapsedMs =
    status === 'running'
      ? startedAt !== undefined
        ? Math.max(0, now - startedAt)
        : localDurationMs
      : hasLocalStoppedNode || hasLocalFailedNode
        ? (localElapsedMs ?? backendDurationMs)
        : (backendDurationMs ?? localElapsedMs)

  return {
    status,
    hasExecution,
    hasNodeProgress,
    isProgressIndeterminate:
      status === 'running' && hasNodeProgress && completedNodes === 0,
    totalNodes,
    completedNodes,
    progressPercent,
    currentNodeId: status === 'running' ? localRunningNode?.id : undefined,
    elapsedMs,
  }
}

function browserCapturedText(node: AgentFlowNode) {
  return (node.data.settings[BROWSER_CAPTURE_TEXT_SETTING] || '').trim()
}

function browserCaptureError(node: AgentFlowNode) {
  return node.data.settings['采集错误']?.trim() || ''
}

function browserScreenshotPath(node: AgentFlowNode) {
  return (node.data.settings[BROWSER_SCREENSHOT_SETTING] || '').trim()
}

function browserScreenshotError(node: AgentFlowNode) {
  return (node.data.settings[BROWSER_SCREENSHOT_ERROR_SETTING] || '').trim()
}

function browserScreenshotSummary(node: AgentFlowNode) {
  const path = browserScreenshotPath(node)
  if (path) return `截图证据: ${path}`
  const error = browserScreenshotError(node)
  if (error) return `截图证据: 截图失败，${error}`
  return '截图证据: 未返回截图文件'
}

function stripTransientNodeSettings(nodes: AgentFlowNode[]) {
  return nodes.map((node) => {
    const settings = { ...node.data.settings }
    if (node.data.kind === 'browser') {
      for (const key of TRANSIENT_BROWSER_SETTING_LABELS) {
        delete settings[key]
      }
    }
    if (node.data.kind === 'request') {
      for (const key of TRANSIENT_HTTP_SETTING_LABELS) {
        delete settings[key]
      }
    }
    if (node.data.kind === 'image') {
      for (const key of TRANSIENT_IMAGE_SETTING_LABELS) {
        delete settings[key]
      }
    }
    return {
      ...node,
      data: {
        ...node.data,
        settings,
      },
    }
  })
}

function stopRunningExecutionLogs(
  current: AgentExecutionLogMap,
  stoppedAt: number
): AgentExecutionLogMap {
  const next: AgentExecutionLogMap = {}
  for (const [nodeId, log] of Object.entries(current)) {
    if (log.status !== 'running') {
      next[nodeId] = log
      continue
    }
    const durationMs = log.startedAt ? stoppedAt - log.startedAt : 0
    next[nodeId] = {
      ...log,
      status: 'stopped',
      endedAt: stoppedAt,
      durationMs,
      message: '试运行已手动停止，节点保留最后一次执行上下文。',
      modelOutput:
        log.modelOutput || '执行被停止，尚未产生完整的模型输出结果。',
      steps: [...log.steps, '收到停止指令并写入节点日志。'],
    }
  }
  return next
}

function buildPreviewStatusMap(
  executionNodes: AgentFlowNode[],
  completedIndex: number
): Map<string, AgentNodeStatus> {
  const statusById = new Map<string, AgentNodeStatus>()
  executionNodes.forEach((node, index) => {
    if (index <= completedIndex) {
      statusById.set(node.id, 'done')
    } else if (index === completedIndex + 1) {
      statusById.set(node.id, 'running')
    } else {
      statusById.set(node.id, 'waiting')
    }
  })
  return statusById
}

function buildPreviewExecutionLog(
  executionNodes: AgentFlowNode[],
  statusById: Map<string, AgentNodeStatus>,
  skillChoices: SkillChoice[],
  edges: AgentFlowEdge[] = []
): AgentExecutionLogMap {
  const now = Date.now()
  let completedIndex = 0
  const completedCount = executionNodes.filter(
    (node) => statusById.get(node.id) === 'done'
  ).length

  return executionNodes.reduce<AgentExecutionLogMap>((acc, node, index) => {
    const status = statusById.get(node.id) ?? node.data.status
    const logStatus = statusToLogStatus(status)
    const estimateMs = estimateNodeDuration(node, index)
    let startedAt: number | undefined
    let endedAt: number | undefined
    let durationMs: number | undefined

    if (logStatus === 'completed') {
      startedAt = now - (completedCount - completedIndex + 1) * 1400
      endedAt = startedAt + estimateMs
      durationMs = estimateMs
      completedIndex += 1
    } else if (logStatus === 'running') {
      startedAt = now - 900
      durationMs = now - startedAt
    }

    const upstreamLogs = directUpstreamLogs(node, acc, edges)

    acc[node.id] = {
      nodeId: node.id,
      status: logStatus,
      startedAt,
      endedAt,
      durationMs,
      message: buildNodeLogMessage(node, logStatus),
      inputSummary: buildNodeInputSummary(node, upstreamLogs),
      outputSummary: buildNodeOutputSummary(node, logStatus, upstreamLogs),
      modelOutput: buildNodeModelOutput(
        node,
        logStatus,
        skillChoices,
        upstreamLogs
      ),
      steps: buildNodeLogSteps(node, logStatus, skillChoices, upstreamLogs),
    }
    return acc
  }, {})
}

function buildBackendExecutionPreview(
  log: AgentExecutionLog,
  executionNodes: AgentFlowNode[],
  edges: AgentFlowEdge[]
): {
  executionLog: AgentExecutionLogMap
  statusById: Map<string, AgentNodeStatus>
  blueprintStatus: AgentBlueprintStatus
} | null {
  const graph = parseBackendGraphOutput(log.output)
  if (!graph || !Array.isArray(graph.nodes)) {
    const progress = parseBackendProgress(log.output)
    if (!progress) return null

    const executionLog: AgentExecutionLogMap = {}
    const statusById = new Map<string, AgentNodeStatus>()
    const currentNodeId = progress.node_id
    const currentIndex = currentNodeId
      ? executionNodes.findIndex((node) => node.id === currentNodeId)
      : -1
    const now = Date.now()
    executionNodes.forEach((node, index) => {
      let status: AgentNodeStatus = 'waiting'
      if (currentIndex >= 0 && index < currentIndex) status = 'done'
      if (node.id === currentNodeId) status = 'running'
      statusById.set(node.id, status)
      if (node.id === currentNodeId) {
        const result: BackendAgentNodeResult = {
          node_id: node.id,
          kind: progress.node_kind ?? node.data.kind,
          title: progress.node_title ?? node.data.title,
          status: 'running',
          message:
            progress.message || '正在执行，实时结果会在完成后写入输出摘要。',
          output: progress,
          started_at: log.start_time,
          duration_ms: Math.max(0, now - log.start_time),
        }
        executionLog[node.id] = buildBackendNodeExecutionLog(
          node,
          result,
          {
            mode: 'node_graph',
            nodes: [result],
            warnings: [],
          },
          edges,
          executionLog
        )
      }
    })
    return {
      executionLog,
      statusById,
      blueprintStatus: 'running',
    }
  }

  const resultById = new Map(
    graph.nodes
      .filter((result) => typeof result.node_id === 'string')
      .map((result) => [result.node_id as string, result])
  )
  const executionLog: AgentExecutionLogMap = {}
  const statusById = new Map<string, AgentNodeStatus>()

  for (const node of executionNodes) {
    const result = resultById.get(node.id)
    if (!result) {
      statusById.set(node.id, 'waiting')
      continue
    }
    const nodeStatus = backendNodeStatus(result.status)
    statusById.set(node.id, nodeStatus)
    executionLog[node.id] = buildBackendNodeExecutionLog(
      node,
      result,
      graph,
      edges,
      executionLog
    )
  }

  return {
    executionLog,
    statusById,
    blueprintStatus: backendBlueprintStatus(log.status, statusById),
  }
}

function parseBackendGraphOutput(
  value: unknown
): BackendAgentGraphOutput | null {
  const parsed =
    typeof value === 'string'
      ? parseMaybeJSON(value)
      : value && typeof value === 'object'
        ? value
        : null
  if (!parsed || typeof parsed !== 'object') return null
  const graph = parsed as BackendAgentGraphOutput
  if (!Array.isArray(graph.nodes)) return null
  return graph
}

function parseBackendProgress(value: unknown): BackendAgentProgress | null {
  const parsed =
    typeof value === 'string'
      ? parseMaybeJSON(value)
      : value && typeof value === 'object'
        ? value
        : null
  if (!parsed || typeof parsed !== 'object') return null
  const envelope = parsed as { mode?: unknown; progress?: unknown }
  if (envelope.mode !== 'node_graph_progress') return null
  return asPlainRecord(envelope.progress) as BackendAgentProgress | null
}

function buildBackendNodeExecutionLog(
  node: AgentFlowNode,
  result: BackendAgentNodeResult,
  graph: BackendAgentGraphOutput,
  edges: AgentFlowEdge[],
  currentLogs: AgentExecutionLogMap
): AgentNodeExecutionLog {
  const logStatus = backendNodeLogStatus(result.status)
  const upstreamLogs = directUpstreamLogs(node, currentLogs, edges)
  const steps = [
    '后端真实执行节点。',
    result.message || backendNodeStatusText(logStatus),
    result.archive_path ? `归档路径: ${result.archive_path}` : '',
    ...(graph.warnings ?? [])
      .filter((warning) => warning.includes(result.node_id ?? ''))
      .map((warning) => `警告: ${warning}`),
  ].filter(Boolean)

  return {
    nodeId: result.node_id || node.id,
    status: logStatus,
    startedAt: result.started_at,
    endedAt: result.ended_at,
    durationMs: result.duration_ms,
    message: result.message || backendNodeStatusText(logStatus),
    inputSummary: backendInputSummary(result.input, upstreamLogs),
    outputSummary: backendOutputSummary(result),
    modelOutput: backendNodeModelOutput(result),
    steps,
    error: result.error,
  }
}

function backendNodeStatus(value?: string): AgentNodeStatus {
  switch (value) {
    case 'completed':
      return 'done'
    case 'running':
      return 'running'
    case 'failed':
      return 'failed'
    default:
      return 'waiting'
  }
}

function backendNodeLogStatus(value?: string): AgentNodeLogStatus {
  switch (value) {
    case 'completed':
      return 'completed'
    case 'running':
      return 'running'
    case 'failed':
      return 'failed'
    case 'stopped':
      return 'stopped'
    default:
      return 'pending'
  }
}

function backendBlueprintStatus(
  status: string,
  statusById: Map<string, AgentNodeStatus>
): AgentBlueprintStatus {
  if (status === 'failed' || [...statusById.values()].includes('failed')) {
    return 'failed'
  }
  if (status === 'running' || status === 'pending') return 'running'
  if (status === 'paused') return 'paused'
  return 'ended'
}

function backendNodeStatusText(status: AgentNodeLogStatus) {
  if (status === 'failed') return '后端节点执行失败。'
  if (status === 'completed') return '后端节点执行完成。'
  if (status === 'running') return '后端节点正在执行。'
  if (status === 'stopped') return '后端节点已停止。'
  return '后端节点等待执行。'
}

function backendInputSummary(
  input: unknown,
  upstreamLogs: AgentNodeExecutionLog[]
) {
  const record = asPlainRecord(input)
  const summaries: string[] = []
  if (record) {
    const settings = asPlainRecord(record.settings)
    if (settings && Object.keys(settings).length > 0) {
      summaries.push(`配置字段: ${Object.keys(settings).join('、')}`)
    }
    const upstream = asPlainRecord(record.upstream)
    if (upstream && Object.keys(upstream).length > 0) {
      summaries.push(`上游节点: ${Object.keys(upstream).join('、')}`)
    }
  }
  if (summaries.length === 0 && upstreamLogs.length > 0) {
    summaries.push(
      `上游日志: ${upstreamLogs.map((log) => log.nodeId).join('、')}`
    )
  }
  if (summaries.length === 0 && input !== undefined && input !== null) {
    summaries.push(truncateText(formatBackendValue(input), 180))
  }
  return summaries.length > 0 ? summaries : ['无显式输入']
}

function backendOutputSummary(result: BackendAgentNodeResult) {
  if (result.error) return [`执行失败: ${result.error}`]
  const output = asPlainRecord(result.output)
  if (!output) {
    return result.output === undefined || result.output === null
      ? ['无显式输出']
      : [truncateText(formatBackendValue(result.output), 180)]
  }

  const summaries: string[] = []
  const files = Array.isArray(output.files) ? output.files : []
  for (const file of files) {
    const fileRecord = asPlainRecord(file)
    if (!fileRecord) continue
    summaries.push(
      `实际文件: ${String(fileRecord.label ?? '文件')} -> ${String(fileRecord.path ?? '')}`
    )
  }
  if (typeof output.output_directory === 'string') {
    summaries.push(`归档目录: ${output.output_directory}`)
  }
  if (typeof output.screenshot_path === 'string' && output.screenshot_path) {
    summaries.push(`截图证据: ${output.screenshot_path}`)
  }
  if (typeof output.text_length === 'number') {
    summaries.push(`页面文本: ${output.text_length} 字符`)
  }
  if (typeof output.row_count === 'number') {
    summaries.push(`行数: ${output.row_count}`)
  } else if (typeof output.count === 'number') {
    summaries.push(`数量: ${output.count}`)
  }
  if (typeof output.status === 'string') {
    summaries.push(`状态: ${output.status}`)
  }
  if (summaries.length === 0) {
    const keys = Object.keys(output)
    summaries.push(
      keys.length > 0 ? `输出字段: ${keys.join('、')}` : '无显式输出'
    )
  }
  return summaries
}

function backendNodeModelOutput(result: BackendAgentNodeResult) {
  const output = asPlainRecord(result.output)
  if (output && typeof output.result === 'string') {
    return truncateText(output.result, 6000)
  }
  if (output && typeof output.content === 'string') {
    return truncateText(output.content, 6000)
  }
  return truncateText(formatBackendValue(result.output ?? result), 6000)
}

function formatBackendValue(value: unknown) {
  if (typeof value === 'string') return value
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

function asPlainRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  return value as Record<string, unknown>
}

function directUpstreamLogs(
  node: AgentFlowNode,
  logs: AgentExecutionLogMap,
  edges: AgentFlowEdge[]
) {
  return edges
    .filter((edge) => edge.target === node.id)
    .map((edge) => logs[edge.source])
    .filter((log): log is AgentNodeExecutionLog => Boolean(log))
}

function estimateNodeDuration(node: AgentFlowNode, index: number) {
  const baseByKind: Record<AgentNodeKind, number> = {
    start: 220,
    schedule: 260,
    browser: 1800,
    xianyu: 30000,
    xianyu_reply: 30000,
    xianyu_review: 30000,
    request: 1200,
    transform: 980,
    image: 2600,
    report: 1600,
    webpage: 1400,
    notify: 720,
    stop: 240,
    custom: 1000,
  }
  return baseByKind[node.data.kind] + index * 90
}

function buildNodeInputSummary(
  node: AgentFlowNode,
  upstreamLogs: AgentNodeExecutionLog[] = []
) {
  const summaries = node.data.inputs.map((input) => {
    const settingValue = node.data.settings[input]
    if (settingValue) return `${input}: ${settingValue}`
    return `${input}: 来自上一节点输出`
  })

  if (node.data.kind === 'browser') {
    const pageData = browserCapturedText(node)
    const error = browserCaptureError(node)
    summaries.push(
      pageData
        ? `页面数据: 已自动采集 ${pageData.length} 个字符`
        : error
          ? `页面数据: 自动采集失败，${error}`
          : '页面数据: 等待自动浏览器采集'
    )
  } else if (node.data.kind === 'request') {
    const statusCode = node.data.settings[HTTP_STATUS_SETTING]
    const error = node.data.settings[HTTP_ERROR_SETTING]
    summaries.push(
      statusCode
        ? `HTTP 响应: 状态码 ${statusCode}，${node.data.settings[HTTP_RESPONSE_SETTING]?.length ?? 0} 个字符`
        : error
          ? `HTTP 响应: 请求失败，${error}`
          : 'HTTP 响应: 等待真实请求执行'
    )
  } else if (upstreamLogs.length > 0) {
    summaries.push(
      `上游输出: ${upstreamLogs.filter((log) => log.status === 'completed').length} 个已完成节点`
    )
  }

  if (summaries.length > 0) return summaries

  const triggerSetting =
    node.data.settings['触发方式'] ?? node.data.settings['执行频率']
  return [triggerSetting ? `触发条件: ${triggerSetting}` : '无显式输入']
}

function buildNodeOutputSummary(
  node: AgentFlowNode,
  status: AgentNodeLogStatus,
  upstreamLogs: AgentNodeExecutionLog[] = []
) {
  if (node.data.outputs.length === 0) return ['无显式输出']

  if (node.data.kind === 'browser' && status === 'completed') {
    const capturedText = browserCapturedText(node)
    const error = browserCaptureError(node)
    return [
      error
        ? `页面数据: 自动采集失败，${error}`
        : capturedText
          ? `页面数据: 已采集真实页面文本 ${capturedText.length} 字符`
          : '页面数据: 未获取可解析的真实页面数据',
      browserScreenshotSummary(node),
    ]
  }

  if (node.data.kind === 'request' && status === 'completed') {
    const statusCode = node.data.settings[HTTP_STATUS_SETTING]
    const response = node.data.settings[HTTP_RESPONSE_SETTING] || ''
    const error = node.data.settings[HTTP_ERROR_SETTING]
    return [
      statusCode
        ? `JSON 数据: 已获取真实 HTTP 响应，状态码 ${statusCode}`
        : `JSON 数据: 请求失败，${error || '未返回响应'}`,
      `请求日志: ${response ? `响应体 ${response.length} 字符` : '无响应体'}`,
    ]
  }

  if (node.data.kind === 'image' && status === 'completed') {
    const taskId = node.data.settings[IMAGE_TASK_ID_SETTING]
    const taskStatus = node.data.settings[IMAGE_TASK_STATUS_SETTING]
    const urls = splitLines(node.data.settings[IMAGE_TASK_URLS_SETTING] || '')
    const error = node.data.settings[IMAGE_TASK_ERROR_SETTING]
    if (error || taskStatus === 'failed') {
      return [
        `图片 URL: 任务提交失败`,
        `图片说明: ${error || '图片任务未成功提交'}`,
      ]
    }
    return [
      urls.length > 0
        ? `图片 URL: ${urls.length} 个结果已写入绘图历史`
        : `图片 URL: 真实图片任务已提交，等待后台生成`,
      `图片说明: 任务 ${taskId || '未记录'}，状态 ${taskStatus || 'running'}`,
    ]
  }

  if (
    (node.data.kind === 'transform' || node.data.kind === 'report') &&
    hasMissingBrowserData(upstreamLogs) &&
    status === 'completed'
  ) {
    return node.data.outputs.map((output) => `${output}: 上游真实数据缺失`)
  }

  if (node.data.kind === 'report') {
    return buildReportOutputSummary(node, status)
  }

  const suffix: Record<AgentNodeLogStatus, string> = {
    pending: '等待生成',
    running: '生成中',
    completed: '已根据真实输入生成',
    failed: '执行失败',
    stopped: '已停止，保留部分输入',
  }

  return node.data.outputs.map((output) => `${output}: ${suffix[status]}`)
}

function buildNodeModelOutput(
  node: AgentFlowNode,
  status: AgentNodeLogStatus,
  skillChoices: SkillChoice[],
  upstreamLogs: AgentNodeExecutionLog[] = []
) {
  if (status === 'pending') {
    return '节点尚未调用模型，等待上游节点完成后生成输出。'
  }
  if (status === 'running' && node.data.kind === 'xianyu_reply') {
    return '页面监听中。收到新的买家消息后，将检索当前知识库并直接自动发送回复。'
  }
  if (status === 'running' && node.data.kind === 'xianyu_review') {
    return '正在读取闲鱼评价管理中的待卖家评价订单并逐页批量提交好评。'
  }
  if (status === 'running') {
    return '模型调用进行中，输出会在当前节点完成后写入这里。'
  }
  if (status === 'stopped') {
    return '执行被停止，尚未产生完整的模型输出结果。'
  }

  const skill = getSkillChoice(node.data.skill, skillChoices)
  const skillLine = node.data.skill ? `\nSkill: ${skill.label}` : ''

  switch (node.data.kind) {
    case 'start':
      return [
        '本地执行结果: 已触发当前流程。',
        `启动节点: ${node.data.title}`,
        '后续节点仅接收上一节点输出和自身描述。',
      ].join('\n')
    case 'schedule':
      return [
        '本地执行结果: 定时触发条件已匹配。',
        `执行频率: ${node.data.settings['执行频率'] || '每日 09:00'}`,
        '已向后续节点传递调度触发信号。',
      ].join('\n')
    case 'browser':
      return buildBrowserNodeModelOutput(node, skillLine)
    case 'request':
      return buildRequestNodeModelOutput(node, skillLine)
    case 'transform': {
      const templateName = node.data.settings['整理模板']
      const templateOriginalName = node.data.settings['模板原文件名']
      if (hasMissingBrowserData(upstreamLogs)) {
        return buildBlockedByMissingBrowserDataOutput(node)
      }
      const upstreamSummary = summarizeUpstreamLogs(upstreamLogs, 5000)
      if (!upstreamSummary) {
        return [
          `模型: ${node.data.model}${skillLine}`,
          '无法整理数据: 当前节点没有收到已完成的上游真实输出。',
          '处理: 请连接浏览器取数、网页请求、文件读取或其他真实数据节点后重新执行。',
          '限制: 不会生成示例字段、示例值或根据蓝图标题补全业务数据。',
        ].join('\n')
      }
      return [
        `模型: ${node.data.model}${skillLine}`,
        '整理结果: 仅使用上一节点真实输出和当前节点描述。',
        '',
        formatGenericUpstreamSummaryMarkdown(upstreamLogs),
        '',
        '关键结论:',
        ...buildGenericInsightLines(upstreamLogs),
        '',
        '限制: 未识别到专用结构化指标时，只保留上游真实输出摘要，不编造字段或数据。',
        '',
        '上游原文摘录:',
        upstreamSummary,
        '',
        templateName && templateName !== '未上传模板'
          ? `已按当前模板文件「${templateOriginalName || templateName}」整理字段。`
          : '未上传模板，已按上一节点数据结构自动归并。',
      ].join('\n')
    }
    case 'image':
      return buildImageNodeModelOutput(node, skillLine)
    case 'report': {
      const outputDir =
        node.data.settings['输出目录'] || DEFAULT_OUTPUT_DIR_REPORT
      const outputFormats = reportFileOutputLabels(node)
      const previewPaths = reportPreviewPaths(node)
      const templateBindings = parseTemplateBindings(
        node.data.settings['模板文件'] || '',
        node.data.settings
      )
      if (hasMissingBrowserData(upstreamLogs)) {
        return buildBlockedByMissingBrowserDataOutput(node)
      }
      return [
        `模型: ${node.data.model}${skillLine}`,
        '# 报告预览',
        '',
        '## 摘要',
        '- 根据上一节点提供的数据整理主要结论。',
        templateBindings.length > 0
          ? `- 按当前模板文件「${templateBindingNames(templateBindings)}」和节点描述控制标题、格式、语气和归档要求。`
          : '- 未绑定模板文件，按当前节点描述控制标题、格式、语气和归档要求。',
        '',
        '## 后续处理',
        '1. 保留上一节点数据中的关键字段。',
        `2. 按当前节点输出格式生成: ${outputFormats.join('、')}。`,
        '3. 正式执行时输出到当前节点配置的归档目录。',
        '',
        `预览输出目录: ${outputDir}`,
        ...previewPaths.map((path) => `预览文件: ${path}`),
      ].join('\n')
    }
    case 'webpage': {
      const templateBindings = parseTemplateBindings(
        node.data.settings['网页模板'] || '',
        node.data.settings
      )
      return [
        `模型: ${node.data.model}${skillLine}`,
        '网页输出结果:',
        '- 已生成交互式结果页面。',
        templateBindings.length > 0
          ? `- 按当前网页模板「${templateBindingNames(templateBindings)}」和节点描述渲染页面。`
          : '- 未绑定网页模板，按当前节点描述和上一节点输出渲染页面。',
        `- 输出目录: ${node.data.settings['输出目录'] || DEFAULT_OUTPUT_DIR_WEB}`,
        '- 页面仅使用上一节点输出和当前节点描述中的渲染要求。',
      ].join('\n')
    }
    case 'notify':
      return buildNotifyNodeModelOutput(node, skillLine)
    case 'stop':
      return [
        '本地执行结果: 流程结束节点已执行。',
        `结束方式: ${node.data.settings['结束方式'] || '本轮完成后停止'}`,
        '产物目录: 以上游输出节点配置为准。',
      ].join('\n')
    case 'custom':
      return buildCustomNodeModelOutput(node, skillLine, upstreamLogs)
    default:
      return `模型: ${node.data.model}${skillLine}\n节点已完成，输出将传给下一节点。`
  }
}

function buildBrowserNodeModelOutput(node: AgentFlowNode, skillLine: string) {
  const capturedText = browserCapturedText(node)
  const captureError = browserCaptureError(node)

  if (capturedText) {
    return [
      `模型: ${node.data.model}${skillLine}`,
      '数据来源: 浏览器节点自动采集的真实页面文本。',
      `采集网址: ${node.data.settings['采集网址'] || node.data.settings['网址'] || '未记录'}`,
      browserScreenshotSummary(node),
      '',
      '页面文本摘录:',
      truncateText(capturedText, 4000),
      '',
      '限制: 仅向下游传递当前页面实际采集文本、截图路径和节点配置，不使用模板、标题或历史内容补全业务数据。',
    ].join('\n')
  }

  return [
    `模型: ${node.data.model}${skillLine}`,
    `${BROWSER_DATA_MISSING_MARKER}: true`,
    '未拿到真实浏览器页面数据。',
    `原因: ${captureError || '浏览器自动采集未返回可解析的页面文本。'}`,
    `目标网址: ${node.data.settings['网址'] || '未配置'}`,
    browserScreenshotSummary(node),
    '处理: 请确认目标页面已登录并能在浏览器中正常打开，然后重新执行。',
    '限制: 不会根据模板、标题、历史内容或模型常识补全页面数据。',
  ].join('\n')
}

function buildRequestNodeModelOutput(node: AgentFlowNode, skillLine: string) {
  const response = node.data.settings[HTTP_RESPONSE_SETTING] || ''
  const statusCode = node.data.settings[HTTP_STATUS_SETTING] || ''
  const error = node.data.settings[HTTP_ERROR_SETTING] || ''
  const finalUrl =
    node.data.settings['HTTP 最终网址'] || node.data.settings['网址'] || ''
  const contentType = node.data.settings['HTTP 内容类型'] || ''
  const duration = node.data.settings['HTTP 耗时'] || ''

  if (error || !statusCode) {
    return [
      `模型: ${node.data.model}${skillLine}`,
      'HTTP 请求失败。',
      `目标网址: ${node.data.settings['网址'] || '未配置'}`,
      `错误: ${error || '未返回状态码'}`,
      '限制: 下游节点不得根据示例或模型常识补全接口数据。',
    ].join('\n')
  }

  const parsed = parseMaybeJSON(response)
  return [
    `模型: ${node.data.model}${skillLine}`,
    '数据来源: 网页请求节点执行的真实 HTTP 响应。',
    `请求方法: ${node.data.settings['方法'] || 'GET'}`,
    `目标网址: ${node.data.settings['网址'] || ''}`,
    `最终网址: ${finalUrl}`,
    `状态码: ${statusCode}`,
    contentType ? `内容类型: ${contentType}` : '',
    duration ? `耗时: ${duration}` : '',
    '',
    '响应摘要:',
    parsed
      ? formatJSONPreview(parsed)
      : truncateText(response || '响应体为空。', 4000),
  ]
    .filter(Boolean)
    .join('\n')
}

function buildImageNodeModelOutput(node: AgentFlowNode, skillLine: string) {
  const taskId = node.data.settings[IMAGE_TASK_ID_SETTING] || ''
  const status = node.data.settings[IMAGE_TASK_STATUS_SETTING] || ''
  const urls = splitLines(node.data.settings[IMAGE_TASK_URLS_SETTING] || '')
  const error = node.data.settings[IMAGE_TASK_ERROR_SETTING] || ''
  const prompt = node.data.settings[IMAGE_TASK_PROMPT_SETTING] || ''

  if (error || status === 'failed') {
    return [
      `模型: ${node.data.model}${skillLine}`,
      '图片任务提交失败。',
      `任务 ID: ${taskId || '未创建'}`,
      `错误: ${error || '图片服务未返回成功状态'}`,
      '限制: 不使用演示图片或固定 URL 作为节点输出。',
      prompt ? `提交提示词:\n${truncateText(prompt, 1600)}` : '',
    ]
      .filter(Boolean)
      .join('\n')
  }

  return [
    `模型: ${node.data.model}${skillLine}`,
    '图片生成结果:',
    `- 真实图片任务已提交到创作台绘图队列。`,
    `- 任务 ID: ${taskId || '未记录'}`,
    `- 当前状态: ${status || 'running'}`,
    urls.length > 0
      ? `- 图片 URL:\n${urls.map((url) => `  - ${url}`).join('\n')}`
      : '- 图片 URL: 后台生成完成后写入绘图日志和图片历史。',
    prompt ? `\n提交提示词:\n${truncateText(prompt, 1600)}` : '',
  ]
    .filter(Boolean)
    .join('\n')
}

function buildNotifyNodeModelOutput(node: AgentFlowNode, skillLine: string) {
  const channel = node.data.settings['渠道'] || 'Webhook'
  const confirm = node.data.settings['确认'] || '发送前确认'
  const statusCode = node.data.settings[HTTP_STATUS_SETTING]
  const error = node.data.settings[HTTP_ERROR_SETTING]
  return [
    `模型: ${node.data.model}${skillLine}`,
    '通知结果:',
    `- 渠道: ${channel}`,
    statusCode
      ? `- 已真实请求，状态码 ${statusCode}。`
      : `- 未发送成功: ${error || '未配置目标 URL'}`,
    `- 目标: ${node.data.settings['Webhook'] || node.data.settings['通知目标'] || node.data.settings['网址'] || '未配置'}`,
    `- 确认策略: ${confirm}`,
  ].join('\n')
}

function parseMaybeJSON(value: string) {
  const trimmed = value.trim()
  if (!trimmed) return null
  if (!trimmed.startsWith('{') && !trimmed.startsWith('[')) return null
  try {
    return JSON.parse(trimmed) as unknown
  } catch {
    return null
  }
}

function formatJSONPreview(value: unknown) {
  return truncateText(JSON.stringify(value, null, 2), 4000)
}

function splitLines(value: string) {
  return value
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
}

function summarizeUpstreamLogs(
  logs: AgentNodeExecutionLog[],
  maxLength: number
) {
  const completedLogs = logs.filter((log) => log.status === 'completed')
  if (completedLogs.length === 0) return ''
  return truncateText(
    completedLogs
      .map((log) => {
        const output = log.modelOutput.trim()
        return [
          `### ${log.nodeId}`,
          log.message ? `状态: ${log.message}` : '',
          output,
        ]
          .filter(Boolean)
          .join('\n')
      })
      .join('\n\n'),
    maxLength
  )
}

function formatGenericUpstreamSummaryMarkdown(logs: AgentNodeExecutionLog[]) {
  const rows = logs
    .filter((log) => log.status === 'completed')
    .map((log) => [
      log.nodeId,
      log.outputSummary.join('；') || log.message,
      truncateText(log.modelOutput.replace(/\s+/g, ' ').trim(), 160),
    ])

  if (rows.length === 0) {
    return '未收到已完成的上游真实输出。'
  }

  return [
    '| 上游节点 | 输出摘要 | 真实内容摘录 |',
    '| --- | --- | --- |',
    ...rows.map(
      ([nodeId, summary, excerpt]) =>
        `| ${escapeMarkdownTableCell(nodeId)} | ${escapeMarkdownTableCell(summary)} | ${escapeMarkdownTableCell(excerpt)} |`
    ),
  ].join('\n')
}

function buildGenericInsightLines(logs: AgentNodeExecutionLog[]) {
  const completedLogs = logs.filter((log) => log.status === 'completed')
  if (completedLogs.length === 0) {
    return ['- 上游没有已完成输出，无法生成结论。']
  }

  return completedLogs.map((log) => {
    const excerpt =
      log.outputSummary.find(Boolean) ||
      truncateText(log.modelOutput.replace(/\s+/g, ' ').trim(), 120)
    return `- ${log.nodeId}: ${excerpt}`
  })
}

function buildCustomNodeModelOutput(
  node: AgentFlowNode,
  skillLine: string,
  upstreamLogs: AgentNodeExecutionLog[]
) {
  const upstreamSummary = summarizeUpstreamLogs(upstreamLogs, 4000)
  if (!upstreamSummary) {
    return [
      `模型: ${node.data.model}${skillLine}`,
      `自定义步骤: ${node.data.title}`,
      '无法执行: 当前自定义步骤没有收到已完成的上游真实输出。',
      '处理: 请在步骤要求中明确输入字段，或连接真实数据节点后重新执行。',
      '限制: 不使用内置样例或默认业务上下文补全输出。',
    ].join('\n')
  }

  return [
    `模型: ${node.data.model}${skillLine}`,
    `自定义步骤: ${node.data.title}`,
    '执行依据: 仅使用当前步骤要求、配置字段和上一节点真实输出。',
    '',
    '上游输入摘要:',
    formatGenericUpstreamSummaryMarkdown(upstreamLogs),
    '',
    '处理要求:',
    node.data.description || '未填写步骤要求。',
    '',
    '输出:',
    upstreamSummary,
  ].join('\n')
}

function escapeMarkdownTableCell(value: string) {
  return value.replace(/\|/g, '\\|').replace(/\r?\n/g, '<br />')
}

function truncateText(value: string, maxLength: number) {
  return value.length > maxLength
    ? `${value.slice(0, maxLength)}\n...（已截断）`
    : value
}

function buildBlockedByMissingBrowserDataOutput(node: AgentFlowNode) {
  return [
    `模型: ${node.data.model}`,
    `${BROWSER_DATA_MISSING_MARKER}: true`,
    '无法生成有效输出: 上游浏览器取数节点没有提供真实页面数据。',
    '请先让浏览器取数节点自动采集到可解析的真实页面数据。',
    '为避免错误数据，本节点不会根据模板、标题或隐含上下文补全页面数据。',
  ].join('\n')
}

function hasMissingBrowserData(logs: AgentNodeExecutionLog[]) {
  return logs.some(
    (log) =>
      log.status === 'completed' &&
      log.modelOutput.includes(BROWSER_DATA_MISSING_MARKER)
  )
}

function buildReportOutputSummary(
  node: AgentFlowNode,
  status: AgentNodeLogStatus
) {
  const templateBindings = parseTemplateBindings(
    node.data.settings['模板文件'] || '',
    node.data.settings
  )
  const outputDir = node.data.settings['输出目录'] || DEFAULT_OUTPUT_DIR_REPORT
  const outputFormats = reportFileOutputLabels(node)
  const statusText: Record<AgentNodeLogStatus, string> = {
    pending: '等待生成',
    running: '生成中',
    completed: '已生成预览',
    stopped: '已停止',
    failed: '执行失败',
  }
  const formatLines = outputFormats.map(
    (format) => `${format}: ${statusText[status]}`
  )

  if (status === 'completed') {
    return [
      ...formatLines,
      templateBindings.length > 0
        ? `模板文件: ${templateBindingNames(templateBindings)}`
        : '模板文件: 未绑定，按节点描述生成',
      `预览输出目录: ${outputDir}`,
    ]
  }
  if (status === 'running') {
    return [...formatLines, `输出目录: ${outputDir}`]
  }
  if (status === 'stopped') {
    return [...formatLines, '输出: 保留部分上下文']
  }
  return [...formatLines, `输出目录: ${outputDir}`]
}

function buildNodeLogSteps(
  node: AgentFlowNode,
  status: AgentNodeLogStatus,
  skillChoices: SkillChoice[],
  upstreamLogs: AgentNodeExecutionLog[] = []
) {
  const skill = getSkillChoice(node.data.skill, skillChoices)
  const capabilityLine =
    node.data.skill && skill.capabilities?.length
      ? `Skill 能力: ${skill.capabilities.join('、')}`
      : ''
  const steps = [
    '读取节点配置、输入字段和上游依赖。',
    node.data.skill
      ? `加载 Skill: ${skill.label}`
      : '未启用 Skill，使用节点基础提示执行。',
  ]
  if (capabilityLine) {
    steps.push(capabilityLine)
  }

  if (NODE_META[node.data.kind].role === 'system') {
    steps.push('执行本地编排逻辑。')
  } else {
    steps.push(`准备模型调用: ${node.data.model}`)
  }

  if (node.data.kind === 'browser') {
    const capturedText = browserCapturedText(node)
    const error = browserCaptureError(node)
    steps.push(
      capturedText
        ? `从自动采集的页面文本读取 ${capturedText.length} 个字符。`
        : error
          ? `浏览器自动采集失败: ${error}`
          : '等待 Edge/浏览器 DOM 采集器返回真实页面文本。'
    )
  }

  if (node.data.kind === 'transform') {
    const templateName = node.data.settings['整理模板']
    const templateOriginalName = node.data.settings['模板原文件名']
    const dataType = node.data.settings['数据类型'] || '自动识别'
    steps.push(
      upstreamLogs.length > 0
        ? `使用 ${upstreamLogs.length} 个上游真实节点输出整理，不扩展隐含上下文。`
        : templateName && templateName !== '未上传模板'
          ? `应用当前模板文件: ${templateOriginalName || templateName}`
          : `未上传模板，按数据类型「${dataType}」自动推断字段与格式。`
    )
  }

  if (node.data.kind === 'report') {
    const templateBindings = parseTemplateBindings(
      node.data.settings['模板文件'] || '',
      node.data.settings
    )
    const outputFormats = reportFileOutputLabels(node)
    const outputDir =
      node.data.settings['输出目录'] || DEFAULT_OUTPUT_DIR_REPORT
    steps.push(
      templateBindings.length > 0
        ? `读取当前模板文件: ${templateBindingNames(templateBindings)}`
        : '未绑定模板文件，按当前节点描述生成报告。'
    )
    if (hasMissingBrowserData(upstreamLogs)) {
      steps.push('上游真实页面数据缺失，本节点停止生成业务内容。')
    } else {
      steps.push(`使用 ${upstreamLogs.length} 个上游真实节点输出生成。`)
    }
    steps.push(`输出格式: ${outputFormats.join('、')}`)
    steps.push(`生成报告预览；正式执行时写入输出目录: ${outputDir}`)
  }

  if (node.data.kind === 'webpage') {
    const templateBindings = parseTemplateBindings(
      node.data.settings['网页模板'] || '',
      node.data.settings
    )
    steps.push(
      templateBindings.length > 0
        ? `读取当前网页模板文件: ${templateBindingNames(templateBindings)}`
        : '未绑定网页模板，按当前节点描述渲染网页。'
    )
  }

  if (node.data.kind === 'stop') {
    steps.push('结束当前流程；不覆盖上游输出节点的产物目录。')
  }

  if (status === 'completed') {
    steps.push(
      node.data.kind === 'stop'
        ? '流程结束，产物目录以上游输出节点配置为准。'
        : '校验输出字段并传递给下一节点。'
    )
  } else if (status === 'running') {
    steps.push('节点正在执行，等待模型或工具返回结果。')
  } else if (status === 'stopped') {
    steps.push('执行被停止，已写入停止状态。')
  } else {
    steps.push('等待上游节点完成后进入执行队列。')
  }

  return steps
}

function buildNodeLogMessage(node: AgentFlowNode, status: AgentNodeLogStatus) {
  if (status === 'completed') {
    if (node.data.kind === 'report') {
      return `${node.data.title} 已根据上游结果生成报告。`
    }
    if (node.data.kind === 'stop') {
      return `${node.data.title} 已完成流程结束动作，输出目录以上游节点配置为准。`
    }
    return `${node.data.title} 已完成，输出已进入后续节点上下文。`
  }
  if (status === 'running') {
    if (node.data.kind === 'xianyu_reply') {
      return `${node.data.title} 正在监听闲鱼消息页面。`
    }
    if (node.data.kind === 'xianyu_review') {
      return `${node.data.title} 正在处理评价管理中的待卖家评价订单。`
    }
    return `${node.data.title} 正在执行，实时结果会在完成后写入输出摘要。`
  }
  if (status === 'stopped') {
    return `${node.data.title} 已停止，保留已收集到的执行上下文。`
  }
  return `${node.data.title} 正在等待上游节点完成。`
}

function NodeConfigPanel({
  node,
  blueprintId,
  blueprintChoices,
  modelChoices,
  groupChoices,
  skillChoices,
  currentGroup,
  onDeleteNode,
  onUpdateNode,
  onUpdateSetting,
}: {
  node: AgentFlowNode
  blueprintId: string | null
  blueprintChoices: BlueprintChoice[]
  modelChoices: ModelChoice[]
  groupChoices: GroupOption[]
  skillChoices: SkillChoice[]
  currentGroup?: string
  onDeleteNode: (nodeId: string) => void
  onUpdateNode: (patch: Partial<AgentNodeData>) => void
  onUpdateSetting: (key: string, value: string) => void
}) {
  const parameterEntries = Object.entries(node.data.settings).filter(
    ([key]) => !EXECUTION_POLICY_SETTING_LABELS.has(key)
  )
  return (
    <div className='space-y-3'>
      <ConfigCard title='步骤信息'>
        <NodeCommonConfig
          currentGroup={currentGroup}
          groupChoices={groupChoices}
          modelChoices={modelChoices}
          node={node}
          skillChoices={skillChoices}
          onDeleteNode={onDeleteNode}
          onUpdateNode={onUpdateNode}
          onUpdateSetting={onUpdateSetting}
        />
      </ConfigCard>

      <ConfigCard title='任务要求'>
        <Textarea
          value={node.data.description}
          onChange={(event) =>
            onUpdateNode({ description: event.target.value })
          }
          className='min-h-22 rounded-[8px] px-2.5 py-2 text-sm leading-relaxed'
        />
      </ConfigCard>

      <ConfigCard title='步骤设置'>
        {parameterEntries.length === 0 ? (
          <div className='text-muted-foreground text-xs'>无参数</div>
        ) : (
          <div className='space-y-2'>
            {parameterEntries.map(([key, value]) => (
              <SettingControl
                key={key}
                label={key}
                value={value}
                nodeKind={node.data.kind}
                blueprintId={blueprintId}
                blueprintChoices={blueprintChoices}
                settings={node.data.settings}
                onChange={(nextValue) => onUpdateSetting(key, nextValue)}
                onChangeSetting={onUpdateSetting}
              />
            ))}
          </div>
        )}
      </ConfigCard>

      <div className='grid grid-cols-2 gap-2'>
        <ConfigCard title='使用的数据'>
          <FieldList values={node.data.inputs} />
        </ConfigCard>
        <ConfigCard title='产生的结果'>
          <FieldList values={node.data.outputs} />
        </ConfigCard>
      </div>
    </div>
  )
}

const EXECUTION_POLICY_SETTING_LABELS = new Set([
  '节点超时',
  '失败重试',
  '重试间隔',
  '失败处理',
])

function ExecutionPolicyConfig({
  settings,
  onUpdateSetting,
}: {
  settings: Record<string, string>
  onUpdateSetting: (key: string, value: string) => void
}) {
  const controls = [
    {
      key: '节点超时',
      label: '节点超时',
      value: settings['节点超时'] || '不限制',
      options: ['不限制', '10s', '30s', '60s', '5m', '10m', '30m', '1h'],
    },
    {
      key: '失败重试',
      label: '失败重试',
      value: settings['失败重试'] || '0 次',
      options: ['0 次', '1 次', '2 次', '3 次', '5 次'],
    },
    {
      key: '重试间隔',
      label: '重试间隔',
      value: settings['重试间隔'] || '1s',
      options: ['0s', '1s', '3s', '5s', '10s'],
    },
    {
      key: '失败处理',
      label: '失败处理',
      value: settings['失败处理'] || '中断流程',
      options: ['中断流程', '继续执行', '跳过节点'],
    },
  ]
  return (
    <div className='grid grid-cols-2 gap-2'>
      {controls.map((control) => (
        <LabeledField key={control.key} label={control.label}>
          <NativeSelect
            value={control.value}
            onChange={(event) =>
              onUpdateSetting(control.key, event.target.value)
            }
            className='w-full'
          >
            {control.options.map((option) => (
              <NativeSelectOption key={option} value={option}>
                {option}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </LabeledField>
      ))}
    </div>
  )
}

function NodeCommonConfig({
  node,
  modelChoices,
  groupChoices,
  skillChoices,
  currentGroup,
  onDeleteNode,
  onUpdateNode,
  onUpdateSetting,
}: {
  node: AgentFlowNode
  modelChoices: ModelChoice[]
  groupChoices: GroupOption[]
  skillChoices: SkillChoice[]
  currentGroup?: string
  onDeleteNode: (nodeId: string) => void
  onUpdateNode: (patch: Partial<AgentNodeData>) => void
  onUpdateSetting: (key: string, value: string) => void
}) {
  const meta = getAgentNodeVisualMeta(node.data.kind, node.data.componentId)
  const Icon = meta.icon
  const selectedGroup =
    node.data.group || currentGroup || groupChoices[0]?.value || 'default'
  const { data: selectedGroupModels, isFetching: isFetchingGroupModels } =
    useQuery({
      queryKey: ['playground-models', selectedGroup],
      queryFn: () => getUserModels(selectedGroup),
      staleTime: 30_000,
    })
  const choices = useMemo(
    () =>
      selectedGroupModels
        ? buildModelChoices(selectedGroupModels)
        : selectedGroup === currentGroup
          ? modelChoices
          : [],
    [currentGroup, modelChoices, selectedGroup, selectedGroupModels]
  )
  const selectedModelValue = choices.some(
    (model) => model.value === node.data.model
  )
    ? node.data.model
    : ''
  const selectedSkill = getSkillChoice(node.data.skill, skillChoices)
  const [advancedOpen, setAdvancedOpen] = useState(false)

  useEffect(() => {
    if (selectedGroupModels === undefined) return
    const nextModel = choices[0]?.value ?? ''
    if (node.data.model !== nextModel && !selectedModelValue) {
      onUpdateNode({ model: nextModel })
    }
  }, [
    choices,
    node.data.model,
    onUpdateNode,
    selectedGroupModels,
    selectedModelValue,
  ])

  return (
    <div className='space-y-3'>
      <div className='flex items-center gap-2'>
        <div
          className={cn(
            'flex size-8 shrink-0 items-center justify-center rounded-[8px]',
            meta.accent
          )}
        >
          <Icon className='size-4' />
        </div>
        <div className='min-w-0 flex-1'>
          <div className='truncate text-sm font-semibold'>{meta.label}</div>
          <div className='text-muted-foreground truncate text-xs'>
            {node.data.description || '按任务要求执行此步骤'}
          </div>
        </div>
        <Badge variant='outline' className='rounded-[6px]'>
          {meta.group}
        </Badge>
        <Button
          type='button'
          size='icon'
          variant='ghost'
          className='text-muted-foreground hover:text-destructive size-8 rounded-[6px]'
          onClick={() => onDeleteNode(node.id)}
          title='删除节点'
        >
          <Trash2Icon className='size-3.5' />
        </Button>
      </div>

      <CompactField label='名称'>
        <Input
          value={node.data.title}
          onChange={(event) => onUpdateNode({ title: event.target.value })}
          className='h-7 rounded-[7px] px-2 text-xs'
        />
      </CompactField>

      <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
        <CollapsibleTrigger className='border-border/70 text-muted-foreground hover:text-foreground flex w-full items-center justify-between rounded-[7px] border px-2.5 py-2 text-xs font-medium transition-colors'>
          <span>高级设置</span>
          <ChevronDownIcon
            className={cn(
              'size-3.5 transition-transform',
              advancedOpen && 'rotate-180'
            )}
          />
        </CollapsibleTrigger>
        <CollapsibleContent className='pt-2'>
          <div className='grid grid-cols-2 gap-2'>
            <CompactField label='模型分组'>
              <NativeSelect
                size='sm'
                value={selectedGroup}
                onChange={(event) =>
                  onUpdateNode({ group: event.target.value, model: '' })
                }
                className='w-full'
              >
                {groupChoices.map((group) => (
                  <NativeSelectOption key={group.value} value={group.value}>
                    {group.label}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </CompactField>

            <CompactField label='模型'>
              <NativeSelect
                size='sm'
                value={selectedModelValue}
                onChange={(event) =>
                  onUpdateNode({ model: event.target.value })
                }
                className='w-full'
              >
                {choices.length > 0 ? (
                  <NativeSelectOptGroup
                    label={`当前分组模型: ${selectedGroup}`}
                  >
                    {choices.map((model) => (
                      <NativeSelectOption key={model.value} value={model.value}>
                        {model.label}
                      </NativeSelectOption>
                    ))}
                  </NativeSelectOptGroup>
                ) : (
                  <NativeSelectOption value='' disabled>
                    {isFetchingGroupModels
                      ? '正在加载模型...'
                      : '该分组暂无可用模型'}
                  </NativeSelectOption>
                )}
              </NativeSelect>
            </CompactField>

            <CompactField label='Skill'>
              <NativeSelect
                size='sm'
                value={normalizeBuiltinSkillValue(node.data.skill)}
                title={selectedSkill.description}
                onChange={(event) =>
                  onUpdateNode({ skill: event.target.value })
                }
                className='w-full'
              >
                <NativeSelectOption value={NO_SKILL_CHOICE.value}>
                  {NO_SKILL_CHOICE.label}
                </NativeSelectOption>
                {skillChoicesForNodeKind(skillChoices, node.data.kind).map(
                  (skill) => (
                    <NativeSelectOption key={skill.value} value={skill.value}>
                      {skill.label}
                    </NativeSelectOption>
                  )
                )}
              </NativeSelect>
            </CompactField>
          </div>
          <div className='border-border/60 mt-3 border-t pt-3'>
            <div className='text-muted-foreground mb-2 text-[11px] font-medium'>
              执行策略
            </div>
            <ExecutionPolicyConfig
              settings={node.data.settings}
              onUpdateSetting={onUpdateSetting}
            />
          </div>
          <div className='text-muted-foreground mt-2 text-[10px]'>
            节点 ID：{node.id}
          </div>
        </CollapsibleContent>
      </Collapsible>
    </div>
  )
}

function ConfigCard({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  return (
    <section className='border-border/70 rounded-[8px] border p-3'>
      <div className='mb-2 text-xs font-medium'>{title}</div>
      {children}
    </section>
  )
}

function CompactField({
  label,
  children,
  className,
}: {
  label: string
  children: ReactNode
  className?: string
}) {
  return (
    <label className={cn('grid min-w-0 gap-1', className)}>
      <span className='text-muted-foreground text-[11px]'>{label}</span>
      {children}
    </label>
  )
}

function ExecutionPreviewDock({
  backendExecutionLogs,
  blueprintStatus,
  executionLog,
  executionNodes,
  skillChoices,
  isOpen,
  readiness,
  selectedNode,
  onSelectNode,
  onOpenChange,
}: {
  backendExecutionLogs: AgentExecutionLog[]
  blueprintStatus: AgentBlueprintStatus
  executionLog: AgentExecutionLogMap
  executionNodes: AgentFlowNode[]
  skillChoices: SkillChoice[]
  isOpen: boolean
  readiness: WorkflowReadiness
  selectedNode: AgentFlowNode | null
  onSelectNode: (nodeId: string) => void
  onOpenChange: (open: boolean) => void
}) {
  const logNode = selectedNode ?? executionNodes[0] ?? null
  const latestBackendLog = backendExecutionLogs[0]
  const latestProgress = parseBackendProgress(latestBackendLog?.output)
  const [clockNow, setClockNow] = useState(() => Date.now())
  const executionSummary = buildExecutionPreviewSummary(
    executionNodes,
    executionLog,
    latestBackendLog,
    clockNow,
    blueprintStatus
  )
  const currentExecutionNode = executionSummary.currentNodeId
    ? executionNodes.find((node) => node.id === executionSummary.currentNodeId)
    : null
  const showRuntimeSummary =
    executionNodes.length > 0 && executionSummary.hasExecution
  const showNodeProgress =
    showRuntimeSummary && executionSummary.hasNodeProgress

  useEffect(() => {
    if (executionSummary.status !== 'running') return

    const timer = window.setInterval(() => setClockNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [executionSummary.status])

  return (
    <section className='border-border/70 bg-background shrink-0 border-t'>
      <button
        type='button'
        className='hover:bg-accent/40 flex h-10 w-full items-center gap-2 px-3 text-left transition-colors'
        onClick={() => onOpenChange(!isOpen)}
      >
        {isOpen ? (
          <ChevronDownIcon className='text-muted-foreground size-3.5 shrink-0' />
        ) : (
          <ChevronUpIcon className='text-muted-foreground size-3.5 shrink-0' />
        )}
        <FileSpreadsheetIcon className='text-muted-foreground size-3.5 shrink-0' />
        <span className='text-xs font-semibold'>执行预览</span>
        <WorkflowStageStrip
          readiness={readiness}
          className='hidden shrink-0 lg:flex'
        />
        <span
          className={cn(
            'flex min-w-0 items-center gap-1 text-[11px] font-medium',
            readiness.ready
              ? 'shrink-0 text-emerald-600 dark:text-emerald-400'
              : 'flex-1 text-amber-700 dark:text-amber-300'
          )}
          title={readiness.issues.map((issue) => issue.message).join('\n')}
        >
          {readiness.ready ? (
            <CheckCircle2Icon className='size-3 shrink-0' />
          ) : (
            <TriangleAlertIcon className='size-3 shrink-0' />
          )}
          <span className='truncate'>
            {readiness.ready
              ? '流程完整'
              : `缺 ${readiness.issues.length} 项 · ${readiness.issues[0]?.message ?? '请完善调用流程。'}`}
          </span>
        </span>
        {showRuntimeSummary && readiness.ready ? (
          <span className='text-muted-foreground flex min-w-0 flex-1 items-center gap-x-2 overflow-hidden text-[11px] whitespace-nowrap'>
            <span className='flex shrink-0 items-center gap-1'>
              <span
                className={cn(
                  'size-1.5 rounded-full',
                  executionSummary.status === 'running' && 'bg-blue-500',
                  executionSummary.status === 'completed' && 'bg-emerald-500',
                  executionSummary.status === 'failed' && 'bg-destructive',
                  executionSummary.status === 'paused' && 'bg-amber-500'
                )}
              />
              {formatExecutionPreviewStatus(executionSummary.status)}
            </span>
            {showNodeProgress && (
              <span>
                {executionSummary.isProgressIndeterminate
                  ? `节点进度 ${executionSummary.completedNodes}/${executionSummary.totalNodes} · 同步中`
                  : `节点进度 ${executionSummary.completedNodes}/${executionSummary.totalNodes} · ${executionSummary.progressPercent}%`}
              </span>
            )}
            {currentExecutionNode && (
              <span className='max-w-52 min-w-0 truncate'>
                当前：{currentExecutionNode.data.title}
              </span>
            )}
            {latestProgress?.message && (
              <span className='max-w-80 min-w-0 truncate text-blue-700 dark:text-blue-300'>
                {latestProgress.message}
              </span>
            )}
            {executionSummary.elapsedMs !== undefined && (
              <span className='shrink-0'>
                已用时 {formatElapsedTime(executionSummary.elapsedMs)}
              </span>
            )}
          </span>
        ) : readiness.ready ? (
          <span className='text-muted-foreground truncate text-[11px]'>
            {executionNodes.length} 个节点 · 当前蓝图链路
          </span>
        ) : null}
      </button>

      {isOpen && (
        <div className='border-border/70 grid max-h-72 min-h-0 gap-2 overflow-hidden border-t p-3 xl:grid-cols-[minmax(0,1.45fr)_minmax(20rem,0.9fr)]'>
          {!readiness.ready && (
            <div className='min-w-0 rounded-[8px] border border-amber-500/35 bg-amber-500/5 px-3 py-2 xl:col-span-2'>
              <div className='flex items-center gap-1.5 text-[11px] font-semibold text-amber-700 dark:text-amber-300'>
                <TriangleAlertIcon className='size-3.5 shrink-0' />
                调用流程还缺 {readiness.issues.length} 项
              </div>
              <div className='mt-1.5 flex max-h-14 flex-wrap gap-1 overflow-y-auto'>
                {readiness.issues.map((issue, index) => {
                  const content = (
                    <>
                      <span className='text-amber-700/70 dark:text-amber-300/70'>
                        {index + 1}.
                      </span>
                      <span>{issue.message}</span>
                    </>
                  )
                  return issue.nodeId ? (
                    <button
                      key={`${issue.nodeId}-${issue.message}`}
                      type='button'
                      className='bg-background inline-flex max-w-full items-center gap-1 rounded-[5px] border border-amber-500/30 px-1.5 py-0.5 text-left text-[10px] text-amber-800 transition-colors hover:bg-amber-500/10 dark:text-amber-200'
                      onClick={() => onSelectNode(issue.nodeId!)}
                      title='定位到对应节点'
                    >
                      {content}
                    </button>
                  ) : (
                    <span
                      key={`${index}-${issue.message}`}
                      className='bg-background inline-flex max-w-full items-center gap-1 rounded-[5px] border border-amber-500/30 px-1.5 py-0.5 text-[10px] text-amber-800 dark:text-amber-200'
                    >
                      {content}
                    </span>
                  )
                })}
              </div>
            </div>
          )}
          {showRuntimeSummary && (
            <div className='border-border/70 bg-muted/10 min-w-0 rounded-[8px] border px-3 py-2 xl:col-span-2'>
              <div className='text-muted-foreground flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-[11px]'>
                <span className='text-foreground font-semibold'>执行进度</span>
                <span className='flex shrink-0 items-center gap-1'>
                  <span
                    className={cn(
                      'size-1.5 rounded-full',
                      executionSummary.status === 'running' && 'bg-blue-500',
                      executionSummary.status === 'completed' &&
                        'bg-emerald-500',
                      executionSummary.status === 'failed' && 'bg-destructive',
                      executionSummary.status === 'paused' && 'bg-amber-500'
                    )}
                  />
                  {formatExecutionPreviewStatus(executionSummary.status)}
                </span>
                {showNodeProgress && (
                  <span>
                    {executionSummary.isProgressIndeterminate
                      ? `节点进度 ${executionSummary.completedNodes}/${executionSummary.totalNodes} · 同步中`
                      : `节点进度 ${executionSummary.completedNodes}/${executionSummary.totalNodes} · ${executionSummary.progressPercent}%`}
                  </span>
                )}
                <span className='min-w-0 truncate'>
                  当前节点：
                  {currentExecutionNode?.data.title ??
                    (executionSummary.status === 'completed' ? '已完成' : '-')}
                </span>
                {latestProgress?.message && (
                  <span className='min-w-0 flex-1 truncate text-blue-700 dark:text-blue-300'>
                    {latestProgress.message}
                  </span>
                )}
                {executionSummary.elapsedMs !== undefined && (
                  <span className='shrink-0'>
                    已用时 {formatElapsedTime(executionSummary.elapsedMs)}
                  </span>
                )}
              </div>
              {showNodeProgress && (
                <div
                  aria-label={
                    executionSummary.isProgressIndeterminate
                      ? '执行进度同步中'
                      : `执行进度 ${executionSummary.progressPercent}%`
                  }
                  aria-valuemax={100}
                  aria-valuemin={0}
                  aria-valuenow={
                    executionSummary.isProgressIndeterminate
                      ? undefined
                      : executionSummary.progressPercent
                  }
                  className='bg-muted mt-2 h-1.5 overflow-hidden rounded-full'
                  role='progressbar'
                >
                  <div
                    className={cn(
                      'h-full rounded-full transition-[width] duration-500',
                      executionSummary.isProgressIndeterminate &&
                        'w-1/3 animate-pulse',
                      executionSummary.status === 'running' && 'bg-blue-500',
                      executionSummary.status === 'completed' &&
                        'bg-emerald-500',
                      executionSummary.status === 'failed' && 'bg-destructive',
                      executionSummary.status === 'paused' && 'bg-amber-500'
                    )}
                    style={
                      executionSummary.isProgressIndeterminate
                        ? undefined
                        : { width: `${executionSummary.progressPercent}%` }
                    }
                  />
                </div>
              )}
            </div>
          )}
          {latestBackendLog && (
            <BackendExecutionLogBanner log={latestBackendLog} />
          )}
          {executionNodes.length === 0 ? (
            <div className='text-muted-foreground py-4 text-center text-xs xl:col-span-2'>
              画布上还没有可执行的步骤，请从左侧添加。
            </div>
          ) : (
            <>
              <div className='min-h-0 overflow-y-auto pr-1'>
                <div className='grid gap-2 sm:grid-cols-2 2xl:grid-cols-3'>
                  {executionNodes.map((node, index) => {
                    const meta = getAgentNodeVisualMeta(
                      node.data.kind,
                      node.data.componentId
                    )
                    const Icon = meta.icon
                    const selected = logNode?.id === node.id
                    return (
                      <button
                        key={node.id}
                        type='button'
                        className={cn(
                          'border-border/70 bg-muted/20 hover:bg-accent/40 flex min-h-16 items-start gap-2 rounded-[8px] border p-2 text-left transition-colors',
                          selected &&
                            'border-primary/50 bg-primary/5 ring-primary/20 ring-2'
                        )}
                        onClick={() => onSelectNode(node.id)}
                      >
                        <div className='bg-background flex size-7 shrink-0 items-center justify-center rounded-[6px] border text-xs font-medium'>
                          {index + 1}
                        </div>
                        <div className='min-w-0 flex-1'>
                          <div className='flex min-w-0 items-center gap-1.5'>
                            <Icon className='text-muted-foreground size-3.5 shrink-0' />
                            <div className='truncate text-xs font-medium'>
                              {node.data.title}
                            </div>
                          </div>
                          <div className='text-muted-foreground mt-0.5 truncate text-[11px]'>
                            {node.data.model}
                          </div>
                          <div className='text-muted-foreground mt-0.5 truncate text-[11px]'>
                            Skill ·{' '}
                            {
                              getSkillChoice(node.data.skill, skillChoices)
                                .label
                            }
                          </div>
                        </div>
                        <StatusDot status={node.data.status} />
                      </button>
                    )
                  })}
                </div>
              </div>

              <div className='border-border/70 bg-muted/10 min-h-0 overflow-hidden rounded-[8px] border'>
                <div className='border-border/70 flex h-9 items-center gap-2 border-b px-3'>
                  <FileSpreadsheetIcon className='text-muted-foreground size-3.5' />
                  <span className='text-xs font-semibold'>本节点日志</span>
                  {logNode && (
                    <span className='text-muted-foreground min-w-0 truncate text-[11px]'>
                      {logNode.data.title}
                    </span>
                  )}
                </div>
                <div className='max-h-[calc(18rem-2.25rem)] overflow-y-auto'>
                  {logNode ? (
                    <ExecutionLogPanel
                      executionLog={executionLog}
                      executionNodes={[logNode]}
                    />
                  ) : (
                    <div className='text-muted-foreground p-4 text-center text-xs'>
                      选中一个节点后查看日志。
                    </div>
                  )}
                </div>
              </div>
            </>
          )}
        </div>
      )}
    </section>
  )
}

function BackendExecutionLogBanner({ log }: { log: AgentExecutionLog }) {
  const isFailed = log.status === 'failed'
  const isRunning = log.status === 'running' || log.status === 'pending'
  const isPaused = log.status === 'paused'
  const isCompleted = log.status === 'completed'
  const progress = parseBackendProgress(log.output)
  return (
    <div
      className={cn(
        'flex min-w-0 flex-wrap items-center gap-2 rounded-[8px] border px-3 py-2 text-[11px] xl:col-span-2',
        isFailed && 'border-destructive/35 bg-destructive/5 text-destructive',
        isRunning && 'border-blue-500/30 bg-blue-500/5 text-blue-700',
        isPaused && 'border-amber-500/30 bg-amber-500/5 text-amber-700',
        isCompleted &&
          'border-emerald-500/30 bg-emerald-500/5 text-emerald-700',
        !isFailed &&
          !isRunning &&
          !isPaused &&
          !isCompleted &&
          'border-border/70 bg-muted/20 text-muted-foreground'
      )}
    >
      <span className='font-semibold'>后端正式运行</span>
      <span>{formatBackendLogStatus(log.status)}</span>
      <span className='text-muted-foreground'>
        开始 {formatLogTime(log.start_time)}
      </span>
      <span className='text-muted-foreground'>
        耗时 {formatDuration(log.duration_ms)}
      </span>
      {progress?.message && (
        <span className='min-w-0 flex-1 truncate font-medium'>
          {progress.message}
        </span>
      )}
      {progress && typeof progress.completed === 'number' && typeof progress.total === 'number' && (
        <span className='text-muted-foreground shrink-0'>
          总结进度 {progress.completed}/{progress.total}
        </span>
      )}
      {log.archive_path && (
        <span className='text-muted-foreground min-w-0 truncate'>
          归档 {log.archive_path}
        </span>
      )}
      {log.error_msg && (
        <span className='min-w-0 truncate'>{log.error_msg}</span>
      )}
    </div>
  )
}

function XianyuReplyRecordsPanel({
  records,
  loading,
  open,
  onOpenChange,
  onRefresh,
}: {
  records: AgentXianyuReplyRecord[]
  loading: boolean
  open: boolean
  onOpenChange: (open: boolean) => void
  onRefresh: () => void
}) {
  const [expandedId, setExpandedId] = useState<string | null>(null)

  return (
    <section className='border-border/70 bg-background shrink-0 border-t'>
      <div className='flex h-10 items-center gap-2 px-3'>
        <button
          type='button'
          className='hover:bg-accent/40 flex min-w-0 flex-1 items-center gap-2 text-left text-xs font-semibold'
          onClick={() => onOpenChange(!open)}
        >
          {open ? (
            <ChevronDownIcon className='text-muted-foreground size-3.5 shrink-0' />
          ) : (
            <ChevronUpIcon className='text-muted-foreground size-3.5 shrink-0' />
          )}
          <MessagesSquareIcon className='text-muted-foreground size-3.5 shrink-0' />
          <span className='truncate'>智能客服对话记录</span>
          <span className='text-muted-foreground shrink-0 font-normal'>
            {records.length} 条
          </span>
        </button>
        <button
          type='button'
          className='text-muted-foreground hover:bg-accent/60 rounded-[5px] p-1.5 transition-colors disabled:opacity-50'
          aria-label='刷新对话记录'
          title='刷新对话记录'
          disabled={loading}
          onClick={onRefresh}
        >
          <RotateCcwIcon className={cn('size-3.5', loading && 'animate-spin')} />
        </button>
      </div>
      {open && (
        <div className='border-border/70 max-h-80 overflow-y-auto border-t'>
          {records.length === 0 ? (
            <div className='text-muted-foreground px-3 py-5 text-center text-[11px]'>
              {loading ? '正在读取对话记录…' : '暂无自动回复记录'}
            </div>
          ) : (
            records.map((record) => {
              const expanded = expandedId === record.id
              const status = formatXianyuReplyRecordStatus(record.status)
              return (
                <div key={record.id} className='border-border/60 border-b px-3 py-2 last:border-b-0'>
                  <button
                    type='button'
                    className='flex w-full min-w-0 items-start gap-2 text-left'
                    onClick={() => setExpandedId(expanded ? null : record.id)}
                  >
                    <span
                      className={cn(
                        'mt-1 size-1.5 shrink-0 rounded-full',
                        record.status.startsWith('sent')
                          ? 'bg-emerald-500'
                          : record.status.includes('error')
                            ? 'bg-destructive'
                            : 'bg-amber-500'
                      )}
                    />
                    <span className='min-w-0 flex-1'>
                      <span className='flex items-center justify-between gap-2'>
                        <span className='truncate text-[11px] font-medium'>
                          {record.conversation_title || '闲鱼会话'}
                        </span>
                        <span className='text-muted-foreground shrink-0 text-[10px]'>
                          {formatLogTime(record.created_at)}
                        </span>
                      </span>
                      <span className='text-muted-foreground mt-0.5 block truncate text-[10px]'>
                        买家：{record.incoming_message || '未读取到消息'}
                      </span>
                    </span>
                    <span className='text-muted-foreground shrink-0 text-[10px]'>{expanded ? '收起' : status}</span>
                  </button>
                  {expanded && (
                    <div className='mt-2 grid gap-1.5 pl-3.5 text-[10px]'>
                      <div className='text-muted-foreground'>
                        知识库命中：
                        <span className='text-foreground ml-1 font-medium'>
                          {record.knowledge_match_count}
                        </span>
                        {record.fallback && (
                          <span className='text-amber-700 dark:text-amber-300'>
                            {' '}· 使用兜底回复
                          </span>
                        )}
                      </div>
                      {record.product?.title && (
                        <div className='text-muted-foreground truncate'>
                          商品：<span className='text-foreground'>{record.product.title}</span>
                        </div>
                      )}
                      <div>
                        <span className='text-muted-foreground'>最终发送：</span>
                        <span className='text-foreground'>{record.final_reply || '未发送'}</span>
                      </div>
                      {record.model_draft && record.model_draft !== record.final_reply && (
                        <div className='text-muted-foreground'>模型草稿：{record.model_draft}</div>
                      )}
                      <div className='text-muted-foreground'>
                        总耗时 {formatDuration(record.duration_ms)}
                        {record.model_duration_ms ? ` · 模型 ${formatDuration(record.model_duration_ms)}` : ''}
                      </div>
                      {record.error && <div className='text-destructive break-words'>错误：{record.error}</div>}
                      {record.recent_messages && record.recent_messages.length > 0 && (
                        <div className='border-border/60 mt-0.5 grid gap-1 border-t pt-1.5'>
                          {record.recent_messages.slice(-6).map((message, index) => (
                            <div key={`${message.key ?? index}-${index}`} className='text-muted-foreground'>
                              <span className='text-foreground font-medium'>
                                {message.direction === 'self' ? '客服' : '买家'}：
                              </span>
                              {message.text}
                            </div>
                          ))}
                        </div>
                      )}
                    </div>
                  )}
                </div>
              )
            })
          )}
        </div>
      )}
    </section>
  )
}

function formatXianyuReplyRecordStatus(status: string) {
  if (status.startsWith('sent')) return status.includes('fallback') ? '已回复·兜底' : '已回复'
  if (status.includes('model_error')) return '模型失败'
  if (status.includes('send_error')) return '发送失败'
  if (status === 'no_knowledge_match') return '待回复'
  if (status === 'processing') return '处理中'
  return status || '未知状态'
}

function formatBackendLogStatus(status: string) {
  const labels: Record<string, string> = {
    pending: '准备中',
    running: '执行中',
    completed: '已完成',
    failed: '执行失败',
    paused: '已暂停',
  }
  return labels[status] ?? status
}

const DEFAULT_OUTPUT_DIR_REPORT = 'data/output/reports'
const DEFAULT_OUTPUT_DIR_WEB = 'data/output/web-reports'
const LEGACY_TEMPLATE_DIR_REPORT = '/templates/reports'
const LEGACY_TEMPLATE_DIR_WEB = '/templates/web'
const LEGACY_OUTPUT_DIR_REPORT = '/output/reports'
const LEGACY_OUTPUT_DIR_WEB = '/output/web-reports'
const TEMPLATE_FILE_ACCEPT =
  '.xlsx,.xls,.doc,.docx,.pdf,.html,.htm,.md,.txt,.csv,.json,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,application/vnd.ms-excel,application/vnd.openxmlformats-officedocument.wordprocessingml.document,application/msword,application/pdf,text/html,text/markdown,text/plain,text/csv,application/json'
const COMPONENT_INPUT_FILE_ACCEPT =
  TEMPLATE_FILE_ACCEPT +
  ',.png,.jpg,.jpeg,.webp,image/png,image/jpeg,image/webp'
const TRANSIENT_BROWSER_SETTING_LABELS = new Set([
  BROWSER_PAGE_DATA_SETTING,
  BROWSER_CAPTURE_TEXT_SETTING,
  BROWSER_SCREENSHOT_SETTING,
  BROWSER_SCREENSHOT_ERROR_SETTING,
  '采集标题',
  '采集网址',
  '采集时间',
  '采集错误',
])
const TRANSIENT_HTTP_SETTING_LABELS = new Set([
  HTTP_RESPONSE_SETTING,
  HTTP_STATUS_SETTING,
  HTTP_ERROR_SETTING,
  'HTTP 最终网址',
  'HTTP 内容类型',
  'HTTP 耗时',
])
const TRANSIENT_IMAGE_SETTING_LABELS = new Set([
  IMAGE_TASK_ID_SETTING,
  IMAGE_TASK_STATUS_SETTING,
  IMAGE_TASK_URLS_SETTING,
  IMAGE_TASK_ERROR_SETTING,
  IMAGE_TASK_PROMPT_SETTING,
])
const HIDDEN_SETTING_LABELS = new Set([
  '模板目录',
  '模板原文件名',
  '模板预览',
  ...TRANSIENT_BROWSER_SETTING_LABELS,
  ...TRANSIENT_HTTP_SETTING_LABELS,
  ...TRANSIENT_IMAGE_SETTING_LABELS,
])
const DEFAULT_TRANSFORM_TEMPLATE_PLACEHOLDER =
  '字段: 名称, 数值, 状态, 备注\n格式: Markdown 表格 + JSON 数组\n排序: 按数值降序\n说明: 缺失字段保留空值并标注来源'

function defaultOutputDirForNode(kind: AgentNodeKind) {
  if (kind === 'webpage') return DEFAULT_OUTPUT_DIR_WEB
  return DEFAULT_OUTPUT_DIR_REPORT
}

function shouldUseDefaultOutputDirectory(
  value: string | undefined,
  kind: AgentNodeKind
) {
  const normalized = (value ?? '').trim()
  if (normalized === '') return true
  return legacyOrDefaultOutputDirsForNode(kind).includes(normalized)
}

function legacyOrDefaultOutputDirsForNode(kind: AgentNodeKind) {
  if (kind === 'webpage') return [LEGACY_OUTPUT_DIR_WEB, DEFAULT_OUTPUT_DIR_WEB]
  return [LEGACY_OUTPUT_DIR_REPORT, DEFAULT_OUTPUT_DIR_REPORT]
}

function splitReportOutputFormatSetting(value: string) {
  return value
    .split(/[、,，;；|/\n\r]+/)
    .map((item) => item.trim())
    .filter(Boolean)
}

function isReportPathOutputLabel(label: string) {
  return /归档|路径|目录/.test(label)
}

function normalizeReportOutputFormats(
  value = '',
  fallbackOutputs: string[] = []
) {
  const explicitFormats = splitReportOutputFormatSetting(value)
  const source =
    explicitFormats.length > 0
      ? explicitFormats
      : fallbackOutputs.filter((output) => !isReportPathOutputLabel(output))
  const seen = new Set<string>()
  const formats: string[] = []

  for (const item of source) {
    const format = item.trim()
    if (!format || isReportPathOutputLabel(format) || seen.has(format)) {
      continue
    }
    seen.add(format)
    formats.push(format)
  }

  return formats.length > 0 ? formats : [...DEFAULT_REPORT_OUTPUT_FORMATS]
}

function reportOutputsFromFormatSetting(
  value: string,
  fallbackOutputs: string[] = []
) {
  return [
    ...normalizeReportOutputFormats(value, fallbackOutputs),
    REPORT_OUTPUT_PATH_LABEL,
  ]
}

function reportFileOutputLabels(node: AgentFlowNode) {
  const templateFormats = reportOutputFormatsFromTemplates(
    parseTemplateBindings(
      node.data.settings['模板文件'] || '',
      node.data.settings
    )
  )
  if (templateFormats.length > 0) return templateFormats

  return normalizeReportOutputFormats(
    node.data.settings[REPORT_OUTPUT_FORMAT_SETTING] || '',
    node.data.outputs
  )
}

function reportOutputFormatsFromTemplates(bindings: TemplateBinding[]) {
  const formats: string[] = []
  for (const binding of bindings) {
    const format = reportOutputFormatForTemplatePath(binding.path)
    if (!format) continue
    formats.push(format)
  }
  return formats
}

function reportOutputFormatForTemplatePath(path: string) {
  const ext = normalizeExtension(path.split('.').pop() || '')
  switch (ext) {
    case 'xlsx':
    case 'xls':
      return 'Excel 表格'
    case 'csv':
      return 'CSV 表格'
    case 'md':
    case 'markdown':
      return 'Markdown 文档'
    case 'doc':
    case 'docx':
      return 'Word 文档'
    case 'pdf':
      return 'PDF 文档'
    case 'html':
    case 'htm':
      return 'HTML 网页'
    case 'txt':
      return 'TXT 文本'
    case 'json':
      return 'JSON 数据'
    default:
      return ''
  }
}

function reportExtensionForLabel(label: string) {
  const lower = label.toLowerCase()
  if (lower.includes('markdown') || /\bmd\b/.test(lower)) return 'md'
  if (
    lower.includes('html') ||
    lower.includes('htm') ||
    label.includes('网页')
  ) {
    return 'html'
  }
  if (lower.includes('json')) return 'json'
  if (lower.includes('txt') || label.includes('文本')) return 'txt'
  if (
    lower.includes('xlsx') ||
    lower.includes('excel') ||
    label.includes('Excel')
  ) {
    return 'xlsx'
  }
  if (lower.includes('pdf')) return 'pdf'
  if (
    lower.includes('docx') ||
    lower.includes('word') ||
    (label.includes('文档') && !lower.includes('pdf'))
  ) {
    return 'docx'
  }
  if (
    lower.includes('csv') ||
    lower.includes('excel') ||
    label.includes('表格')
  ) {
    return 'csv'
  }
  return 'md'
}

function normalizeExtension(value: string) {
  return value.trim().toLowerCase().replace(/^\./, '')
}

function reportPreviewPaths(node: AgentFlowNode) {
  const outputDir = node.data.settings['输出目录'] || DEFAULT_OUTPUT_DIR_REPORT
  const labels = reportFileOutputLabels(node)
  return labels.map((label, index) => {
    const suffix = labels.length > 1 ? `-${index + 1}` : ''
    return `${outputDir}/daily-report${suffix}.${reportExtensionForLabel(label)}`
  })
}

function parseTemplateBindings(
  value: string,
  settings: Record<string, string>
): TemplateBinding[] {
  const trimmed = value.trim()
  if (!trimmed || trimmed === '未上传模板') return []

  if (trimmed.startsWith('[')) {
    try {
      const parsed: unknown = JSON.parse(trimmed)
      if (Array.isArray(parsed)) {
        return parsed
          .map((item) => {
            if (typeof item === 'string') {
              return {
                path: item,
                originalName: item.split('/').pop() || item,
              }
            }
            if (!item || typeof item !== 'object') return null
            const candidate = item as Partial<TemplateBinding>
            const path =
              typeof candidate.path === 'string' ? candidate.path : ''
            if (!path.trim()) return null
            return {
              path,
              originalName:
                typeof candidate.originalName === 'string' &&
                candidate.originalName.trim()
                  ? candidate.originalName
                  : path.split('/').pop() || path,
              preview:
                typeof candidate.preview === 'string'
                  ? candidate.preview
                  : undefined,
              hash:
                typeof candidate.hash === 'string' ? candidate.hash : undefined,
            }
          })
          .filter((item): item is TemplateBinding => !!item)
      }
    } catch {
      /* Fall back to legacy single path below. */
    }
  }

  return [
    {
      path: trimmed,
      originalName:
        settings['模板原文件名']?.trim() || trimmed.split('/').pop() || trimmed,
      preview: settings['模板预览']?.trim() || undefined,
    },
  ]
}

function serializeTemplateBindings(bindings: TemplateBinding[]) {
  if (bindings.length === 0) return ''
  if (bindings.length === 1) return bindings[0].path
  return JSON.stringify(bindings)
}

function agentUploadToTemplateBinding(
  upload: AgentFileUpload
): TemplateBinding {
  return {
    path: upload.path,
    originalName: upload.original_name || upload.filename || upload.path,
    preview: upload.content_preview,
    hash: upload.hash,
  }
}

function mergeTemplateBindings(
  current: TemplateBinding[],
  additions: TemplateBinding[]
) {
  let next = [...current]
  for (const addition of additions) {
    const nameKey = addition.originalName.trim().toLowerCase()
    next = next.filter((item) => {
      if (item.path === addition.path) return false
      return item.originalName.trim().toLowerCase() !== nameKey
    })
    next.push(addition)
  }
  return next
}

function templateBindingNames(bindings: TemplateBinding[]) {
  return bindings
    .map((binding) => binding.originalName || binding.path)
    .join('、')
}

function SettingControl({
  label,
  value,
  nodeKind,
  blueprintId,
  blueprintChoices = [],
  settings,
  onChange,
  onChangeSetting,
}: {
  label: string
  value: string
  nodeKind: AgentNodeKind
  blueprintId?: string | null
  blueprintChoices?: BlueprintChoice[]
  settings: Record<string, string>
  onChange: (value: string) => void
  onChangeSetting?: (key: string, value: string) => void
}) {
  const [isUploadingTemplate, setIsUploadingTemplate] = useState(false)
  const [isTemplateLibraryOpen, setIsTemplateLibraryOpen] = useState(false)
  const [isLoadingTemplateLibrary, setIsLoadingTemplateLibrary] =
    useState(false)
  const [templateLibrary, setTemplateLibrary] = useState<AgentFileUpload[]>([])
  const [isUploadingImportedFile, setIsUploadingImportedFile] = useState(false)
  const [isImportFileLibraryOpen, setIsImportFileLibraryOpen] = useState(false)
  const [isLoadingImportFileLibrary, setIsLoadingImportFileLibrary] =
    useState(false)
  const [importFileLibrary, setImportFileLibrary] = useState<AgentFileUpload[]>(
    []
  )

  if (label === '条件') {
    return <ConditionSettingControl value={value} onChange={onChange} />
  }

  if (label === '蓝图ID') {
    const choices = blueprintChoices.filter(
      (choice) => choice.localId !== blueprintId
    )
    const hasCurrentValue = choices.some((choice) => choice.value === value)
    return (
      <LabeledField label='子蓝图'>
        <NativeSelect
          value={value}
          onChange={(event) => onChange(event.target.value)}
          className='w-full'
        >
          <NativeSelectOption value=''>选择子蓝图</NativeSelectOption>
          {value && !hasCurrentValue && (
            <NativeSelectOption value={value}>{value}</NativeSelectOption>
          )}
          {choices.map((choice) => (
            <NativeSelectOption key={choice.value} value={choice.value}>
              {choice.label}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      </LabeledField>
    )
  }

  // 请求 URL —— 文本输入，带描述提示
  if (label === '请求URL') {
    return (
      <LabeledField label='请求 URL'>
        <Input
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder='https://api.example.com/data'
          className='h-8 rounded-[8px] text-sm'
        />
      </LabeledField>
    )
  }

  // 请求参数 —— 多行文本，提示 key=value 格式
  if (label === '请求参数') {
    return (
      <LabeledField label='请求参数'>
        <textarea
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder={'key1=value1\nkey2=value2\n或 JSON: {"key":"value"}'}
          className='border-input bg-background placeholder:text-muted-foreground focus-visible:ring-ring flex min-h-[64px] w-full resize-none rounded-[8px] border px-3 py-1.5 text-sm shadow-sm focus-visible:ring-1 focus-visible:outline-none'
        />
        <div className='text-muted-foreground text-[11px] leading-relaxed'>
          支持 key=value 或 JSON 格式，仅用于 POST/PUT 请求体或 URL 查询参数。
        </div>
      </LabeledField>
    )
  }

  if (label === '分支配置') {
    return (
      <LabeledField label='分支配置'>
        <Textarea
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder={'ok=正常\nerror=异常'}
          className='min-h-24 rounded-[8px] font-mono text-sm'
        />
        <div className='text-muted-foreground text-[11px] leading-relaxed'>
          每行填写“字段值=分支名称”，最多六条；没有匹配时进入默认分支。
        </div>
      </LabeledField>
    )
  }

  if (label === '输入映射') {
    return (
      <LabeledField label='输入映射'>
        <Textarea
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder={'目标字段=$父变量\n目标字段=上游字段'}
          className='min-h-20 rounded-[8px] font-mono text-sm'
        />
      </LabeledField>
    )
  }

  if (label === '会话名称') {
    return (
      <LabeledField label='会话名称'>
        <Input
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder='默认'
          className='h-8 rounded-[8px] text-sm'
        />
        <div className='text-muted-foreground text-[11px] leading-relaxed'>
          同名浏览器节点共享已登录
          Cookie；需要保持登录态的流程使用“默认”会话即可复用。
        </div>
      </LabeledField>
    )
  }

  if (nodeKind === 'xianyu_review' && label === '评价内容') {
    return (
      <LabeledField label='评价内容'>
        <Input
          value='沟通顺畅，交易愉快'
          readOnly
          className='bg-muted/40 h-8 rounded-[8px] text-sm'
        />
        <div className='text-muted-foreground text-[11px] leading-relaxed'>
          自动评价始终使用此固定内容。
        </div>
      </LabeledField>
    )
  }

  if (nodeKind === 'custom' && label === '路径' && /webhook/i.test(value)) {
    const concretePath = blueprintId
      ? `/api/agent-webhooks/${blueprintId}`
      : '/api/agent-webhooks/{保存蓝图后生成的 ID}'
    return (
      <LabeledField label='Webhook 地址'>
        <Input
          value={concretePath}
          readOnly
          className='h-8 rounded-[8px] font-mono text-xs'
        />
        <div className='text-muted-foreground text-[11px] leading-relaxed'>
          外部系统向此地址发送请求会触发当前蓝图执行；若配置密钥，请在
          X-SunAPI-Webhook-Secret 请求头或 secret 查询参数中传入。
        </div>
      </LabeledField>
    )
  }

  if (nodeKind === 'report' && label === REPORT_OUTPUT_FORMAT_SETTING) {
    const selectedFormats = normalizeReportOutputFormats(value)
    const togglePreset = (preset: string) => {
      const exists = selectedFormats.includes(preset)
      const next = exists
        ? selectedFormats.filter((format) => format !== preset)
        : [...selectedFormats, preset]
      onChange(next.join('、'))
    }

    return (
      <LabeledField label='输出格式'>
        <div className='space-y-2'>
          <Textarea
            value={value}
            onChange={(event) => onChange(event.target.value)}
            placeholder='Markdown 文档、CSV 表格、Excel 表格、Word 文档、PDF 文档、HTML 网页、TXT 文本、JSON 数据'
            className='min-h-16 rounded-[8px] text-sm'
          />
          <div className='flex flex-wrap gap-1.5'>
            {REPORT_OUTPUT_FORMAT_PRESETS.map((preset) => {
              const active = selectedFormats.includes(preset)
              return (
                <Button
                  key={preset}
                  type='button'
                  size='sm'
                  variant={active ? 'default' : 'outline'}
                  className='h-7 rounded-[6px] px-2 text-xs'
                  onClick={() => togglePreset(preset)}
                >
                  {preset}
                </Button>
              )
            })}
          </div>
        </div>
        <div className='text-muted-foreground text-[11px] leading-relaxed'>
          绑定模板后会优先按模板扩展名自动选择输出格式；没有模板时可用顿号、逗号或换行分隔手动指定。归档路径会自动返回，不单独生成文件。
        </div>
      </LabeledField>
    )
  }

  // 输出目录 —— 文本输入
  if (label === '输出目录') {
    return (
      <LabeledField label='输出目录'>
        <Input
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder={defaultOutputDirForNode(nodeKind)}
          className='h-8 rounded-[8px] text-sm'
        />
        <div className='text-muted-foreground text-[11px] leading-relaxed'>
          生成的文件默认写入 exe 同级 data
          目录；标题、格式、语气、归档规则等要求请写在任务描述中。
        </div>
      </LabeledField>
    )
  }

  if (label === '导入文件') {
    const selectedPath = value.trim()
    const selectedName = selectedPath
      ? selectedPath.split('/').filter(Boolean).at(-1) || selectedPath
      : ''
    const refreshImportFileLibrary = async () => {
      setIsLoadingImportFileLibrary(true)
      try {
        setImportFileLibrary(await listAgentFiles('component-input'))
      } catch (error) {
        toast.error(error instanceof Error ? error.message : '文件列表加载失败')
      } finally {
        setIsLoadingImportFileLibrary(false)
      }
    }
    const handleImportFileUpload = async (
      event: ChangeEvent<HTMLInputElement>
    ) => {
      const input = event.currentTarget
      const file = input.files?.[0]
      if (!file) return
      setIsUploadingImportedFile(true)
      try {
        const uploaded = await uploadAgentFile(file, 'component-input')
        onChange(uploaded.path)
        toast.success('文件已设为节点输入', {
          description: uploaded.original_name || uploaded.path,
        })
        if (isImportFileLibraryOpen) void refreshImportFileLibrary()
      } catch (error) {
        toast.error(error instanceof Error ? error.message : '文件上传失败')
      } finally {
        input.value = ''
        setIsUploadingImportedFile(false)
      }
    }

    return (
      <LabeledField label='导入文件'>
        <div className='space-y-2'>
          <div className='border-border/70 bg-muted/20 flex min-w-0 items-center gap-2 rounded-[8px] border px-2 py-2'>
            <FileTextIcon className='text-muted-foreground size-4 shrink-0' />
            <div className='min-w-0 flex-1'>
              <div className='truncate text-xs font-medium'>
                {selectedName || '未选择文件'}
              </div>
              <div className='text-muted-foreground truncate text-[11px]'>
                {selectedPath || '上传文件或从已上传文件中选择'}
              </div>
            </div>
            {selectedPath && (
              <Button
                type='button'
                size='icon-sm'
                variant='ghost'
                className='size-7 shrink-0 rounded-[6px]'
                onClick={() => onChange('')}
                title='清除导入文件'
              >
                <XIcon className='size-3.5' />
                <span className='sr-only'>清除导入文件</span>
              </Button>
            )}
          </div>

          <div className='grid grid-cols-2 gap-2'>
            <label
              aria-disabled={isUploadingImportedFile}
              className='bg-primary text-primary-foreground hover:bg-primary/90 inline-flex h-8 cursor-pointer items-center justify-center gap-1.5 rounded-[6px] px-2.5 text-xs font-medium aria-disabled:pointer-events-none aria-disabled:opacity-50'
            >
              <UploadIcon className='size-3.5 shrink-0' />
              <span>{isUploadingImportedFile ? '上传中' : '上传文件'}</span>
              <input
                type='file'
                accept={COMPONENT_INPUT_FILE_ACCEPT}
                className='sr-only'
                disabled={isUploadingImportedFile}
                onChange={handleImportFileUpload}
              />
            </label>
            <Button
              type='button'
              size='sm'
              variant='outline'
              className='h-8 rounded-[6px] px-2.5 text-xs'
              onClick={() => {
                const nextOpen = !isImportFileLibraryOpen
                setIsImportFileLibraryOpen(nextOpen)
                if (nextOpen) void refreshImportFileLibrary()
              }}
            >
              选择已上传文件
            </Button>
          </div>

          {isImportFileLibraryOpen && (
            <div className='border-border/70 bg-muted/20 max-h-40 overflow-y-auto rounded-[8px] border p-1.5'>
              {isLoadingImportFileLibrary ? (
                <div className='text-muted-foreground py-2 text-center text-xs'>
                  正在加载文件
                </div>
              ) : importFileLibrary.length === 0 ? (
                <div className='text-muted-foreground py-2 text-center text-xs'>
                  暂无可选择的文件
                </div>
              ) : (
                <div className='space-y-1'>
                  {importFileLibrary.map((file) => {
                    const selected = file.path === selectedPath
                    return (
                      <button
                        key={file.path}
                        type='button'
                        className={cn(
                          'border-border/60 hover:bg-accent/50 grid w-full min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-2 rounded-[6px] border px-2 py-1.5 text-left',
                          selected && 'border-primary/50 bg-primary/5'
                        )}
                        onClick={() => onChange(file.path)}
                      >
                        <span className='min-w-0'>
                          <span className='block truncate text-xs font-medium'>
                            {file.original_name || file.filename}
                          </span>
                          <span className='text-muted-foreground block truncate text-[10px]'>
                            {file.path}
                          </span>
                        </span>
                        <span className='text-muted-foreground text-[10px]'>
                          {selected ? '已选择' : '选择'}
                        </span>
                      </button>
                    )
                  })}
                </div>
              )}
            </div>
          )}
        </div>
      </LabeledField>
    )
  }

  if (HIDDEN_SETTING_LABELS.has(label)) {
    return null
  }

  // 模板文件 —— 每次上传绑定当前文件，允许重复上传同名模板。
  if (label === '模板文件' || label === '网页模板') {
    const purpose: AgentFileUploadPurpose =
      nodeKind === 'webpage' ? 'web-template' : 'report-template'
    const bindings = parseTemplateBindings(value, settings)
    const selectedPaths = new Set(bindings.map((binding) => binding.path))

    const applyTemplateBindings = (nextBindings: TemplateBinding[]) => {
      onChange(serializeTemplateBindings(nextBindings))
      onChangeSetting?.('模板原文件名', templateBindingNames(nextBindings))
      onChangeSetting?.(
        '模板预览',
        nextBindings
          .map((binding) => binding.preview)
          .filter(Boolean)
          .join('\n\n')
      )
      if (nodeKind === 'report') {
        const templateFormats = reportOutputFormatsFromTemplates(nextBindings)
        if (templateFormats.length > 0) {
          onChangeSetting?.(
            REPORT_OUTPUT_FORMAT_SETTING,
            templateFormats.join('、')
          )
        }
      }
    }

    const refreshTemplateLibrary = async () => {
      setIsLoadingTemplateLibrary(true)
      try {
        setTemplateLibrary(await listAgentFiles(purpose))
      } catch (error) {
        toast.error(error instanceof Error ? error.message : '模板列表加载失败')
      } finally {
        setIsLoadingTemplateLibrary(false)
      }
    }

    const handleTemplateFileUpload = async (
      event: ChangeEvent<HTMLInputElement>
    ) => {
      const input = event.currentTarget
      const files = Array.from(input.files ?? [])
      if (files.length === 0) return

      setIsUploadingTemplate(true)
      try {
        const uploadedFiles: AgentFileUpload[] = []
        for (const file of files) {
          const uploaded = await uploadAgentFile(file, purpose)
          uploadedFiles.push(uploaded)
          if (
            uploaded.output_directory &&
            shouldUseDefaultOutputDirectory(settings['输出目录'], nodeKind)
          ) {
            onChangeSetting?.('输出目录', uploaded.output_directory)
          }
        }
        applyTemplateBindings(
          mergeTemplateBindings(
            bindings,
            uploadedFiles.map(agentUploadToTemplateBinding)
          )
        )
        if (isTemplateLibraryOpen) {
          void refreshTemplateLibrary()
        }
        const reusedCount = uploadedFiles.filter((file) => file.reused).length
        const replacedCount = uploadedFiles.filter(
          (file) => file.replaced
        ).length
        toast.success('模板已更新', {
          description:
            reusedCount || replacedCount
              ? `复用 ${reusedCount} 个，替换 ${replacedCount} 个`
              : `已绑定 ${uploadedFiles.length} 个模板`,
        })
      } catch (error) {
        toast.error(error instanceof Error ? error.message : '模板上传失败')
      } finally {
        input.value = ''
        setIsUploadingTemplate(false)
      }
    }

    return (
      <LabeledField label='当前模板文件'>
        <div className='space-y-2'>
          <div className='border-border/70 bg-muted/20 rounded-[8px] border p-2'>
            {bindings.length === 0 ? (
              <div className='border-border/60 bg-background/80 flex min-h-12 items-center gap-2 rounded-[7px] border px-2.5 py-2'>
                <span className='bg-muted text-muted-foreground flex size-8 shrink-0 items-center justify-center rounded-[6px]'>
                  <FileTextIcon className='size-4' />
                </span>
                <div className='min-w-0 flex-1'>
                  <div className='text-xs font-medium'>未上传模板文件</div>
                  <div className='text-muted-foreground truncate text-[11px]'>
                    上传或从模板库选择后会绑定到当前节点
                  </div>
                </div>
              </div>
            ) : (
              <TooltipProvider delay={250}>
                <div className='min-w-0 space-y-1.5 overflow-hidden'>
                  {bindings.map((binding) => (
                    <div
                      key={binding.path}
                      className='border-border/60 bg-background/80 grid w-full min-w-0 grid-cols-[2rem_minmax(0,1fr)_1.75rem] items-center gap-2 overflow-hidden rounded-[7px] border px-2.5 py-2'
                    >
                      <span className='bg-muted text-muted-foreground flex size-8 items-center justify-center rounded-[6px]'>
                        <FileTextIcon className='size-4' />
                      </span>
                      <Tooltip>
                        <TooltipTrigger
                          render={
                            <div className='min-w-0 cursor-default overflow-hidden' />
                          }
                        >
                          <div className='truncate text-xs font-medium'>
                            {binding.originalName}
                          </div>
                          <div className='text-muted-foreground truncate text-[11px]'>
                            {binding.path}
                          </div>
                        </TooltipTrigger>
                        <TooltipContent
                          side='left'
                          align='center'
                          className='block max-w-80 space-y-1 break-all'
                        >
                          <div className='font-medium'>
                            {binding.originalName}
                          </div>
                          <div className='opacity-80'>{binding.path}</div>
                        </TooltipContent>
                      </Tooltip>
                      <Button
                        type='button'
                        size='icon-sm'
                        variant='ghost'
                        className='size-7 justify-self-end rounded-[6px]'
                        title='移除模板'
                        onClick={() =>
                          applyTemplateBindings(
                            bindings.filter(
                              (item) => item.path !== binding.path
                            )
                          )
                        }
                      >
                        <XIcon className='size-3.5' />
                        <span className='sr-only'>移除模板</span>
                      </Button>
                    </div>
                  ))}
                </div>
              </TooltipProvider>
            )}
          </div>

          <div className='grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-2'>
            <div className='grid min-w-0 grid-cols-2 gap-2 overflow-hidden'>
              <label
                aria-disabled={isUploadingTemplate}
                className='bg-primary text-primary-foreground hover:bg-primary/90 inline-flex h-8 min-w-0 cursor-pointer items-center justify-center gap-1.5 rounded-[6px] px-2.5 text-xs font-medium whitespace-nowrap aria-disabled:pointer-events-none aria-disabled:opacity-50'
              >
                <UploadIcon className='size-3.5 shrink-0' />
                <span className='truncate'>
                  {isUploadingTemplate ? '上传中' : '上传模板'}
                </span>
                <input
                  type='file'
                  multiple
                  accept={TEMPLATE_FILE_ACCEPT}
                  className='sr-only'
                  disabled={isUploadingTemplate}
                  onChange={handleTemplateFileUpload}
                />
              </label>
              <Button
                type='button'
                size='sm'
                variant='outline'
                className='h-8 min-w-0 rounded-[6px] px-2.5 text-xs'
                onClick={() => {
                  const nextOpen = !isTemplateLibraryOpen
                  setIsTemplateLibraryOpen(nextOpen)
                  if (nextOpen) {
                    void refreshTemplateLibrary()
                  }
                }}
              >
                <span className='truncate'>使用之前的模板</span>
              </Button>
            </div>
            {bindings.length > 0 && (
              <Button
                type='button'
                size='sm'
                variant='ghost'
                className='h-8 shrink-0 rounded-[6px] px-2 text-xs'
                onClick={() => applyTemplateBindings([])}
              >
                清空
              </Button>
            )}
          </div>

          {isTemplateLibraryOpen && (
            <div className='border-border/70 bg-muted/20 max-h-44 overflow-y-auto rounded-[8px] border p-2'>
              {isLoadingTemplateLibrary ? (
                <div className='text-muted-foreground py-2 text-center text-xs'>
                  正在加载模板…
                </div>
              ) : templateLibrary.length === 0 ? (
                <div className='text-muted-foreground py-2 text-center text-xs'>
                  还没有可复用模板。
                </div>
              ) : (
                <div className='space-y-1.5'>
                  {templateLibrary.map((template) => {
                    const selected = selectedPaths.has(template.path)
                    return (
                      <button
                        key={template.path}
                        type='button'
                        className='border-border/60 bg-background/80 hover:bg-accent/50 grid w-full min-w-0 grid-cols-[2rem_minmax(0,1fr)_auto] items-center gap-2 overflow-hidden rounded-[7px] border px-2.5 py-2 text-left'
                        onClick={() =>
                          applyTemplateBindings(
                            mergeTemplateBindings(bindings, [
                              agentUploadToTemplateBinding(template),
                            ])
                          )
                        }
                      >
                        <span className='bg-muted text-muted-foreground flex size-8 shrink-0 items-center justify-center rounded-[6px]'>
                          <FileTextIcon className='size-4' />
                        </span>
                        <div
                          className='min-w-0 overflow-hidden'
                          title={`${template.original_name || template.filename}\n${template.path}`}
                        >
                          <div className='truncate text-xs font-medium'>
                            {template.original_name || template.filename}
                          </div>
                          <div className='text-muted-foreground truncate text-[11px]'>
                            {template.path}
                          </div>
                        </div>
                        <span className='text-muted-foreground shrink-0 text-[11px]'>
                          {selected ? '已使用' : '使用'}
                        </span>
                      </button>
                    )
                  })}
                </div>
              )}
            </div>
          )}
        </div>
        <div className='text-muted-foreground text-[11px] leading-relaxed'>
          支持一次上传多个模板；相同内容会合并为一个模板，同名新模板只保留最新文件。生成时只参考当前绑定模板、上一节点输出和当前节点描述。
        </div>
      </LabeledField>
    )
  }

  if (label === '整理模板') {
    const hasTemplate = value.trim() !== '' && value !== '未上传模板'
    const originalName = settings['模板原文件名']?.trim()

    const handleTemplateUpload = async (
      event: ChangeEvent<HTMLInputElement>
    ) => {
      const input = event.currentTarget
      const file = input.files?.[0]
      if (!file) return

      setIsUploadingTemplate(true)
      try {
        const uploaded = await uploadAgentFile(file, 'report-template')
        onChange(uploaded.path)
        onChangeSetting?.('模板原文件名', uploaded.original_name || file.name)
        onChangeSetting?.('模板预览', uploaded.content_preview || '')
        onChangeSetting?.('模板内容', '')
        toast.success('模板已上传', {
          description: uploaded.original_name || uploaded.path,
        })
      } catch (error) {
        toast.error(error instanceof Error ? error.message : '模板上传失败')
      } finally {
        input.value = ''
        setIsUploadingTemplate(false)
      }
    }

    return (
      <LabeledField label='整理模板'>
        <div className='border-border/70 bg-muted/20 flex items-center gap-2 rounded-[8px] border px-2 py-2'>
          <FileTextIcon className='text-muted-foreground size-4 shrink-0' />
          <div className='min-w-0 flex-1'>
            <div className='truncate text-xs font-medium'>
              {hasTemplate ? originalName || value : '未上传模板'}
            </div>
            <div className='text-muted-foreground truncate text-[11px]'>
              {hasTemplate ? value : '未上传时按数据类型自动推断字段和格式'}
            </div>
          </div>
          {hasTemplate && (
            <Button
              type='button'
              size='sm'
              variant='ghost'
              className='h-7 rounded-[6px] px-2 text-xs'
              onClick={() => {
                onChange('未上传模板')
                onChangeSetting?.('模板内容', '')
                onChangeSetting?.('模板原文件名', '')
                onChangeSetting?.('模板预览', '')
              }}
            >
              清除
            </Button>
          )}
          <label
            aria-disabled={isUploadingTemplate}
            className='bg-primary text-primary-foreground hover:bg-primary/90 inline-flex h-7 shrink-0 cursor-pointer items-center rounded-[6px] px-2.5 text-xs font-medium aria-disabled:pointer-events-none aria-disabled:opacity-50'
          >
            {isUploadingTemplate ? '上传中' : hasTemplate ? '替换' : '上传'}
            <input
              type='file'
              accept={TEMPLATE_FILE_ACCEPT}
              className='sr-only'
              disabled={isUploadingTemplate}
              onChange={handleTemplateUpload}
            />
          </label>
        </div>
        <div className='text-muted-foreground text-[11px] leading-relaxed'>
          每次上传都会保存为独立文件，并绑定当前节点；整理时只参考该模板文件、上一节点数据和当前节点描述。支持
          Excel、Word、PDF、网页、Markdown、TXT、CSV、JSON。
        </div>
      </LabeledField>
    )
  }

  if (label === '模板内容') {
    return (
      <LabeledField label='字段 / 格式模板'>
        <Textarea
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder={DEFAULT_TRANSFORM_TEMPLATE_PLACEHOLDER}
          className='min-h-28 rounded-[8px] text-sm'
        />
        <div className='text-muted-foreground text-[11px] leading-relaxed'>
          上传模板文件后优先按当前文件输出；这里可补充字段顺序、格式和排序规则，留空时按上游数据结构自动整理。
        </div>
      </LabeledField>
    )
  }

  if (label === '脚本') {
    return (
      <LabeledField label='代码处理脚本'>
        <Textarea
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder={
            'select id, title, amount\nfilter amount>0\nsort -amount\nlimit 20'
          }
          className='min-h-32 rounded-[8px] font-mono text-xs'
        />
        <div className='text-muted-foreground text-[11px] leading-relaxed'>
          支持 select、filter、sort、limit、count、sum、average、groupBy
          等安全数据处理指令；只处理上游真实输出，不执行系统命令。
        </div>
      </LabeledField>
    )
  }

  if (label === '执行频率') {
    return (
      <LabeledField label={label}>
        <NativeSelect
          value={value}
          onChange={(event) => onChange(event.target.value)}
          className='w-full'
        >
          {FREQUENCY_OPTIONS.map((option) => (
            <NativeSelectOption key={option} value={option}>
              {option}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      </LabeledField>
    )
  }

  if (label === '执行时间') {
    return (
      <LabeledField label={label}>
        <Input
          type='time'
          value={value}
          onChange={(event) => onChange(event.target.value)}
          className='h-8 rounded-[8px] text-sm'
        />
      </LabeledField>
    )
  }

  if (label === '时区') {
    return (
      <LabeledField label={label}>
        <NativeSelect
          value={value}
          onChange={(event) => onChange(event.target.value)}
          className='w-full'
        >
          {TIMEZONE_OPTIONS.map((option) => (
            <NativeSelectOption key={option} value={option}>
              {option}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      </LabeledField>
    )
  }

  const selectOptions = SETTING_SELECT_OPTIONS[label]
  if (selectOptions) {
    const options = selectOptions.includes(value)
      ? selectOptions
      : [value, ...selectOptions]
    return (
      <LabeledField label={label}>
        <NativeSelect
          value={value}
          onChange={(event) => onChange(event.target.value)}
          className='w-full'
        >
          {options.map((option) => (
            <NativeSelectOption key={option} value={option}>
              {option}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      </LabeledField>
    )
  }

  return (
    <LabeledField label={label}>
      <Input
        value={value}
        onChange={(event) => onChange(event.target.value)}
        className='h-8 rounded-[8px] text-sm'
      />
    </LabeledField>
  )
}

function ConditionSettingControl({
  value,
  onChange,
}: {
  value: string
  onChange: (value: string) => void
}) {
  const condition = parseConditionSetting(value)
  const update = (patch: Partial<typeof condition>) => {
    const next = { ...condition, ...patch }
    if (!next.field.trim()) {
      onChange('')
      return
    }
    onChange(
      next.operator === 'exists'
        ? `${next.field.trim()} exists`
        : `${next.field.trim()} ${next.operator} ${next.comparison.trim()}`
    )
  }
  return (
    <LabeledField label='条件'>
      <div className='grid grid-cols-[1fr_88px] gap-1.5'>
        <Input
          value={condition.field}
          onChange={(event) => update({ field: event.target.value })}
          placeholder='字段，例如 status'
          className='h-8 rounded-[8px] text-sm'
        />
        <NativeSelect
          value={condition.operator}
          onChange={(event) =>
            update({ operator: event.target.value as ConditionOperator })
          }
          className='w-full'
        >
          <NativeSelectOption value='exists'>存在</NativeSelectOption>
          <NativeSelectOption value='='>等于</NativeSelectOption>
          <NativeSelectOption value='!='>不等于</NativeSelectOption>
          <NativeSelectOption value='>'>大于</NativeSelectOption>
          <NativeSelectOption value='>='>大于等于</NativeSelectOption>
          <NativeSelectOption value='<'>小于</NativeSelectOption>
          <NativeSelectOption value='<='>小于等于</NativeSelectOption>
          <NativeSelectOption value='contains'>包含</NativeSelectOption>
        </NativeSelect>
      </div>
      {condition.operator !== 'exists' && (
        <Input
          value={condition.comparison}
          onChange={(event) => update({ comparison: event.target.value })}
          placeholder='比较值'
          className='h-8 rounded-[8px] text-sm'
        />
      )}
    </LabeledField>
  )
}

type ConditionOperator =
  | 'exists'
  | '='
  | '!='
  | '>'
  | '>='
  | '<'
  | '<='
  | 'contains'

function parseConditionSetting(value: string): {
  field: string
  operator: ConditionOperator
  comparison: string
} {
  const normalized = value.trim()
  if (!normalized || normalized === '字段存在') {
    return { field: '', operator: 'exists', comparison: '' }
  }
  const wordMatch = normalized.match(/^(.+?)\s+(contains|exists)(?:\s+(.*))?$/)
  if (wordMatch) {
    return {
      field: wordMatch[1].trim(),
      operator: wordMatch[2] as ConditionOperator,
      comparison: wordMatch[3]?.trim() || '',
    }
  }
  const symbolMatch = normalized.match(/^(.+?)\s*(>=|<=|!=|=|>|<)\s*(.*)$/)
  if (symbolMatch) {
    return {
      field: symbolMatch[1].trim(),
      operator: symbolMatch[2] as ConditionOperator,
      comparison: symbolMatch[3].trim(),
    }
  }
  return { field: normalized, operator: 'exists', comparison: '' }
}

function SkillManagerPanel({
  groupChoices,
  isLoading,
  marketplaceSkills,
  modelChoices,
  skillChoices,
  managedSkills: _managedSkills,
  compact: _compact,
  onAddSkill,
  onDeleteSkill,
}: {
  groupChoices: GroupOption[]
  isLoading: boolean
  marketplaceSkills: SkillChoice[]
  modelChoices: ModelChoice[]
  skillChoices: SkillChoice[]
  managedSkills: SkillChoice[]
  compact?: boolean
  onAddSkill: (skill: SkillChoice) => Promise<void>
  onDeleteSkill: (skillValue: string) => Promise<void>
}) {
  const [activeTab, setActiveTab] = useState<SkillDraftSource>('github')
  const [githubUrl, setGithubUrl] = useState('')
  const [skillStores, setSkillStores] =
    useState<SkillStore[]>(readSavedSkillStores)
  const [storeName, setStoreName] = useState('')
  const [storeUrl, setStoreUrl] = useState('')
  const [activeStore, setActiveStore] = useState<SkillStore | null>(null)
  const [storeSearch, setStoreSearch] = useState('')
  const [customName, setCustomName] = useState('')
  const [customDescription, setCustomDescription] = useState('')
  const [customModelGroup, setCustomModelGroup] = useState(
    groupChoices[0]?.value ?? 'default'
  )
  const [customModelName, setCustomModelName] = useState(
    modelChoices[0]?.value ?? ''
  )
  const [customTags, setCustomTags] = useState('')
  const [submittingKey, setSubmittingKey] = useState<string | null>(null)

  useEffect(() => {
    if (!groupChoices.some((group) => group.value === customModelGroup)) {
      setCustomModelGroup(groupChoices[0]?.value ?? 'default')
    }
  }, [customModelGroup, groupChoices])

  useEffect(() => {
    if (customModelName) return
    setCustomModelName(modelChoices[0]?.value ?? '')
  }, [customModelName, modelChoices])

  const installedValues = useMemo(
    () => new Set(skillChoices.map((s) => s.value)),
    [skillChoices]
  )
  const installedMarketplaceIds = useMemo(
    () =>
      new Set(
        skillChoices
          .map((s) => s.marketplaceId)
          .filter((value): value is string => Boolean(value))
      ),
    [skillChoices]
  )
  const visibleStoreSkills = useMemo(
    () => filterMarketplaceSkills(marketplaceSkills, storeSearch),
    [marketplaceSkills, storeSearch]
  )
  const hotStoreSkills = useMemo(
    () => getHotMarketplaceSkills(marketplaceSkills, installedMarketplaceIds),
    [installedMarketplaceIds, marketplaceSkills]
  )

  const handleAdd = async (skill?: SkillChoice) => {
    const submitKey = skill?.value ?? activeTab
    if (activeTab === 'github') {
      const url = githubUrl.trim()
      if (!url) return
      setSubmittingKey(submitKey)
      try {
        await onAddSkill({
          label: `${formatSkillNameFromReference(url)} Skill`,
          value: `skill.github.${createStableId(url)}`,
          description: '从 GitHub 地址添加的 Skill。',
          source: 'github',
          reference: url,
        })
        setGithubUrl('')
      } finally {
        setSubmittingKey(null)
      }
      return
    }
    if (activeTab === 'marketplace') {
      const marketplaceSkill =
        skill ??
        marketplaceSkills.find(
          (s) =>
            !installedValues.has(s.value) &&
            (!s.marketplaceId || !installedMarketplaceIds.has(s.marketplaceId))
        )
      if (!marketplaceSkill) return
      setSubmittingKey(marketplaceSkill.value)
      try {
        await onAddSkill(marketplaceSkill)
      } finally {
        setSubmittingKey(null)
      }
      return
    }
    const name = customName.trim()
    if (!name) return
    setSubmittingKey(submitKey)
    try {
      await onAddSkill({
        label: name.endsWith('Skill') ? name : `${name} Skill`,
        value: `skill.custom.${createStableId(name)}`,
        description: customDescription.trim() || '用户自定义 Skill。',
        source: 'custom',
        modelGroup: customModelGroup,
        modelName: customModelName,
        tags: splitSkillTags(customTags),
      })
      setCustomName('')
      setCustomDescription('')
      setCustomTags('')
    } finally {
      setSubmittingKey(null)
    }
  }

  const handleCreateStore = () => {
    const name = storeName.trim()
    if (!name) return
    const url = storeUrl.trim()
    const store: SkillStore = {
      id: `store.${createStableId(`${name}:${url || Date.now()}`)}`,
      name,
      description: url || '本地添加的 Skill 商城入口。',
      url: url || undefined,
    }
    setSkillStores((current) => {
      const next = mergeSkillStores([...current, store])
      writeSavedSkillStores(next.filter((item) => !item.builtin))
      return next
    })
    setStoreName('')
    setStoreUrl('')
  }

  const handleDeleteStore = (storeId: string) => {
    setSkillStores((current) => {
      const next = current.filter(
        (store) => store.builtin || store.id !== storeId
      )
      writeSavedSkillStores(next.filter((item) => !item.builtin))
      return next
    })
  }

  const builtinSkills = skillChoices.filter((s) => s.source === 'built-in')
  const customSkills = skillChoices.filter((s) => s.source === 'custom')
  const installedSkills = skillChoices.filter(
    (s) => s.source === 'github' || s.source === 'marketplace'
  )

  return (
    <div className='flex flex-col gap-3'>
      {/* ── 添加来源 Tab ── */}
      <div className='border-border/60 rounded-[10px] border p-3'>
        <div className='text-muted-foreground mb-2 text-[10px] font-semibold tracking-wider uppercase'>
          添加 Skill
        </div>

        {/* 来源切换按钮组 */}
        <div className='bg-muted/50 mb-3 flex rounded-[7px] p-0.5'>
          {(
            [
              { value: 'github', label: 'GitHub' },
              { value: 'marketplace', label: '商城' },
              { value: 'custom', label: '自定义' },
            ] as const
          ).map((opt) => (
            <button
              key={opt.value}
              type='button'
              onClick={() => setActiveTab(opt.value)}
              className={cn(
                'flex-1 rounded-[6px] py-1 text-[11px] font-medium transition-colors',
                activeTab === opt.value
                  ? 'bg-background text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'
              )}
            >
              {opt.label}
            </button>
          ))}
        </div>

        {/* GitHub 输入 */}
        {activeTab === 'github' && (
          <div className='flex gap-1.5'>
            <Input
              value={githubUrl}
              onChange={(e) => setGithubUrl(e.target.value)}
              placeholder='https://github.com/org/skill'
              className='h-7 flex-1 rounded-[6px] text-xs'
              onKeyDown={(e) => {
                if (e.key === 'Enter') void handleAdd()
              }}
            />
            <Button
              type='button'
              size='sm'
              className='h-7 rounded-[6px] px-2.5 text-xs'
              onClick={() => void handleAdd()}
              disabled={!githubUrl.trim() || submittingKey === 'github'}
            >
              {submittingKey === 'github' ? '安装中' : '添加'}
            </Button>
          </div>
        )}

        {/* 商城选择 */}
        {activeTab === 'marketplace' && (
          <div className='flex flex-col gap-2'>
            <div className='grid grid-cols-[1fr_auto] gap-1.5'>
              <Input
                value={storeName}
                onChange={(event) => setStoreName(event.target.value)}
                placeholder='商城名称'
                className='h-7 rounded-[6px] text-xs'
              />
              <Button
                type='button'
                size='sm'
                className='h-7 rounded-[6px] px-2.5 text-xs'
                disabled={!storeName.trim()}
                onClick={handleCreateStore}
              >
                添加商城
              </Button>
            </div>
            <Input
              value={storeUrl}
              onChange={(event) => setStoreUrl(event.target.value)}
              placeholder='商城地址，可选'
              className='h-7 rounded-[6px] text-xs'
            />
            {skillStores.map((store) => (
              <div
                key={store.id}
                className='border-border/50 flex items-center justify-between gap-2 rounded-[7px] border px-2.5 py-2'
              >
                <button
                  type='button'
                  className='min-w-0 flex-1 text-left'
                  onClick={() => {
                    setStoreSearch('')
                    setActiveStore(store)
                  }}
                >
                  <div className='truncate text-[11px] font-medium'>
                    {store.name}
                  </div>
                  <div className='text-muted-foreground truncate text-[10px]'>
                    {store.description}
                  </div>
                </button>
                {store.builtin ? (
                  <span className='shrink-0 rounded bg-emerald-500/10 px-1.5 py-0.5 text-[9px] font-medium text-emerald-600'>
                    默认
                  </span>
                ) : (
                  <button
                    type='button'
                    className='text-muted-foreground hover:text-destructive shrink-0 rounded-[5px] p-1'
                    title='删除商城'
                    onClick={() => handleDeleteStore(store.id)}
                  >
                    <Trash2Icon className='size-3' />
                  </button>
                )}
              </div>
            ))}
          </div>
        )}

        {/* 自定义 */}
        {activeTab === 'custom' && (
          <div className='flex flex-col gap-1.5'>
            <Input
              value={customName}
              onChange={(e) => setCustomName(e.target.value)}
              placeholder='Skill 名称'
              className='h-7 rounded-[6px] text-xs'
            />
            <Textarea
              value={customDescription}
              onChange={(e) => setCustomDescription(e.target.value)}
              placeholder='描述（可选）'
              className='min-h-[52px] resize-none rounded-[6px] text-xs'
            />
            <div className='grid grid-cols-2 gap-1.5'>
              <NativeSelect
                value={customModelGroup}
                onChange={(event) => setCustomModelGroup(event.target.value)}
                className='h-7 rounded-[6px] text-xs'
              >
                {groupChoices.map((group) => (
                  <NativeSelectOption key={group.value} value={group.value}>
                    {group.label}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
              <NativeSelect
                value={customModelName}
                onChange={(event) => setCustomModelName(event.target.value)}
                className='h-7 rounded-[6px] text-xs'
              >
                {modelChoices.map((model) => (
                  <NativeSelectOption key={model.value} value={model.value}>
                    {model.label}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </div>
            <Input
              value={customTags}
              onChange={(e) => setCustomTags(e.target.value)}
              placeholder='标签，逗号分隔'
              className='h-7 rounded-[6px] text-xs'
            />
            <Button
              type='button'
              size='sm'
              className='h-7 rounded-[6px] text-xs'
              onClick={() => void handleAdd()}
              disabled={!customName.trim() || submittingKey === 'custom'}
            >
              {submittingKey === 'custom' ? '创建中' : '创建 Skill'}
            </Button>
          </div>
        )}
      </div>

      <SkillListSection
        title='自定义 Skill'
        badge={`${customSkills.length}`}
        skills={customSkills}
        submittingKey={submittingKey}
        onDeleteSkill={onDeleteSkill}
        onSubmittingChange={setSubmittingKey}
      />

      <SkillListSection
        title='已添加 Skill'
        badge='GitHub / 商城'
        skills={installedSkills}
        submittingKey={submittingKey}
        onDeleteSkill={onDeleteSkill}
        onSubmittingChange={setSubmittingKey}
      />

      {/* ── 内置 Skill ── */}
      {builtinSkills.length > 0 && (
        <div>
          <div className='text-muted-foreground mb-1.5 px-0.5 text-[10px] font-semibold tracking-wider uppercase'>
            内置 Skill ({builtinSkills.length})
          </div>
          <div className='flex flex-col gap-1'>
            {builtinSkills.map((skill) => {
              const meta = BUILTIN_SKILL_META[skill.value]
              const Icon = meta?.icon ?? ZapIcon
              const accent = meta?.accent ?? 'bg-muted/60 text-muted-foreground'
              return (
                <div
                  key={skill.value}
                  className='hover:bg-accent/50 group flex items-start gap-2.5 rounded-[8px] px-2 py-2 transition-colors'
                >
                  <div
                    className={cn(
                      'mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-[7px]',
                      accent
                    )}
                  >
                    <Icon className='size-3.5' />
                  </div>
                  <div className='min-w-0 flex-1'>
                    <div className='flex items-center gap-1.5'>
                      <span className='truncate text-[11px] font-semibold'>
                        {skill.label}
                      </span>
                      <span className='inline-flex shrink-0 items-center gap-0.5 rounded-full bg-emerald-500/12 px-1.5 py-0 text-[9px] font-medium text-emerald-600 dark:text-emerald-400'>
                        <CheckCircle2Icon className='size-2.5' />
                        可用
                      </span>
                    </div>
                    <div className='text-muted-foreground truncate text-[10px] leading-snug'>
                      {skill.description}
                    </div>
                    {skill.capabilities?.length ? (
                      <div className='text-muted-foreground mt-0.5 truncate text-[10px] leading-snug'>
                        {skill.capabilities.join(' · ')}
                      </div>
                    ) : null}
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      )}
      <SkillStoreDialog
        hotSkills={hotStoreSkills}
        installedMarketplaceIds={installedMarketplaceIds}
        installedValues={installedValues}
        isLoading={isLoading}
        open={Boolean(activeStore)}
        search={storeSearch}
        skills={visibleStoreSkills}
        store={activeStore}
        submittingKey={submittingKey}
        onAddSkill={(skill) => void handleAdd(skill)}
        onOpenChange={(open) => {
          if (!open) setActiveStore(null)
        }}
        onSearchChange={setStoreSearch}
      />
    </div>
  )
}

function SkillListSection({
  badge,
  skills,
  submittingKey,
  title,
  onDeleteSkill,
  onSubmittingChange,
}: {
  badge: string
  skills: SkillChoice[]
  submittingKey: string | null
  title: string
  onDeleteSkill: (skillValue: string) => Promise<void>
  onSubmittingChange: (value: string | null) => void
}) {
  if (skills.length === 0) return null

  return (
    <div>
      <div className='text-muted-foreground mb-1.5 flex items-center gap-1 px-0.5 text-[10px] font-semibold tracking-wider uppercase'>
        <span>{title}</span>
        <span className='bg-muted rounded-full px-1.5 py-0 text-[9px] tracking-normal'>
          {badge}
        </span>
      </div>
      <div className='flex flex-col gap-1'>
        {skills.map((skill) => (
          <SkillListItem
            key={skill.value}
            skill={skill}
            submitting={submittingKey === skill.value}
            onDeleteSkill={onDeleteSkill}
            onSubmittingChange={onSubmittingChange}
          />
        ))}
      </div>
    </div>
  )
}

function SkillListItem({
  skill,
  submitting,
  onDeleteSkill,
  onSubmittingChange,
}: {
  skill: SkillChoice
  submitting: boolean
  onDeleteSkill: (skillValue: string) => Promise<void>
  onSubmittingChange: (value: string | null) => void
}) {
  return (
    <div className='bg-muted/30 hover:bg-muted/50 flex items-center gap-2 rounded-[7px] px-2.5 py-2 transition-colors'>
      <div className='min-w-0 flex-1'>
        <div className='flex items-center gap-1.5'>
          <span className='truncate text-[11px] font-medium'>
            {skill.label}
          </span>
          <span
            className={cn(
              'shrink-0 rounded px-1 py-0 text-[9px] font-medium',
              getSkillSourceBadgeClass(skill.source)
            )}
          >
            {formatSkillSource(skill.source)}
          </span>
        </div>
        <div className='text-muted-foreground truncate text-[10px]'>
          {skill.description}
        </div>
        {skill.reference ? (
          <div className='text-muted-foreground truncate text-[10px] opacity-70'>
            {skill.reference}
          </div>
        ) : null}
      </div>
      <button
        type='button'
        className='text-muted-foreground hover:text-destructive shrink-0 rounded-[5px] p-1 transition-colors disabled:opacity-50'
        disabled={submitting}
        onClick={() => {
          onSubmittingChange(skill.value)
          void onDeleteSkill(skill.value).finally(() =>
            onSubmittingChange(null)
          )
        }}
        title='删除 Skill'
      >
        <Trash2Icon className='size-3' />
      </button>
    </div>
  )
}

function SkillStoreDialog({
  hotSkills,
  installedMarketplaceIds,
  installedValues,
  isLoading,
  open,
  search,
  skills,
  store,
  submittingKey,
  onAddSkill,
  onOpenChange,
  onSearchChange,
}: {
  hotSkills: SkillChoice[]
  installedMarketplaceIds: Set<string>
  installedValues: Set<string>
  isLoading: boolean
  open: boolean
  search: string
  skills: SkillChoice[]
  store: SkillStore | null
  submittingKey: string | null
  onAddSkill: (skill: SkillChoice) => void
  onOpenChange: (open: boolean) => void
  onSearchChange: (value: string) => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-w-[min(34rem,calc(100vw-2rem))] gap-3'>
        <DialogHeader>
          <DialogTitle>{store?.name ?? 'Skill 商城'}</DialogTitle>
          <DialogDescription className='line-clamp-2 text-xs'>
            {store?.description ?? '搜索并添加适合当前工作流的 Skill。'}
          </DialogDescription>
        </DialogHeader>
        <div className='relative'>
          <SearchIcon className='text-muted-foreground absolute top-1/2 left-2.5 size-3 -translate-y-1/2' />
          <Input
            value={search}
            onChange={(event) => onSearchChange(event.target.value)}
            placeholder='搜索 Skill...'
            className='h-8 rounded-[7px] pl-7 text-xs'
          />
        </div>
        {!search.trim() && hotSkills.length > 0 ? (
          <div>
            <div className='text-muted-foreground mb-1.5 text-[10px] font-semibold tracking-wider uppercase'>
              热门 Skill
            </div>
            <div className='grid grid-cols-1 gap-1.5 sm:grid-cols-2'>
              {hotSkills.map((skill) => (
                <MarketplaceSkillCard
                  key={skill.value}
                  compact
                  installed={isSkillInstalled(
                    skill,
                    installedValues,
                    installedMarketplaceIds
                  )}
                  installing={submittingKey === skill.value}
                  skill={skill}
                  onAddSkill={onAddSkill}
                />
              ))}
            </div>
          </div>
        ) : null}
        <div>
          <div className='text-muted-foreground mb-1.5 text-[10px] font-semibold tracking-wider uppercase'>
            {search.trim() ? '搜索结果' : '全部 Skill'}
          </div>
          <div className='max-h-[22rem] space-y-1.5 overflow-y-auto pr-1'>
            {isLoading && skills.length === 0 ? (
              <div className='text-muted-foreground rounded-[7px] border px-2.5 py-2 text-[11px]'>
                正在加载 Skill...
              </div>
            ) : null}
            {!isLoading && skills.length === 0 ? (
              <div className='text-muted-foreground rounded-[7px] border px-2.5 py-2 text-[11px]'>
                没有找到可安装 Skill
              </div>
            ) : null}
            {skills.map((skill) => (
              <MarketplaceSkillCard
                key={skill.value}
                installed={isSkillInstalled(
                  skill,
                  installedValues,
                  installedMarketplaceIds
                )}
                installing={submittingKey === skill.value}
                skill={skill}
                onAddSkill={onAddSkill}
              />
            ))}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function MarketplaceSkillCard({
  compact = false,
  installed,
  installing,
  skill,
  onAddSkill,
}: {
  compact?: boolean
  installed: boolean
  installing: boolean
  skill: SkillChoice
  onAddSkill: (skill: SkillChoice) => void
}) {
  return (
    <div className='border-border/50 flex items-center justify-between gap-2 rounded-[7px] border px-2.5 py-2'>
      <div className='min-w-0 flex-1'>
        <div className='truncate text-[11px] font-medium'>{skill.label}</div>
        <div
          className={cn(
            'text-muted-foreground text-[10px]',
            compact ? 'line-clamp-1' : 'line-clamp-2'
          )}
        >
          {skill.description}
        </div>
        {!compact && skill.capabilities?.length ? (
          <div className='text-muted-foreground mt-0.5 truncate text-[10px]'>
            {skill.capabilities.slice(0, 3).join(' · ')}
          </div>
        ) : null}
      </div>
      <Button
        type='button'
        size='sm'
        variant={installed ? 'ghost' : 'outline'}
        className='h-6 shrink-0 rounded-[5px] px-2 text-[10px]'
        disabled={installed || installing}
        onClick={() => !installed && onAddSkill(skill)}
      >
        {installed ? '已添加' : installing ? '安装中' : '+ 添加'}
      </Button>
    </div>
  )
}

function LabeledField({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <label className='grid max-w-full min-w-0 gap-1.5 overflow-hidden'>
      <span className='text-muted-foreground text-xs'>{label}</span>
      {children}
    </label>
  )
}

function FieldList({ values }: { values: string[] }) {
  return (
    <div className='space-y-1'>
      {values.length === 0 ? (
        <div className='text-muted-foreground text-xs'>无</div>
      ) : (
        values.map((value) => (
          <div
            key={value}
            className='bg-muted/55 truncate rounded-[6px] px-1.5 py-1 text-xs'
            title={value}
          >
            {value}
          </div>
        ))
      )}
    </div>
  )
}

function buildModelChoices(models: ModelOption[]) {
  const choices: ModelChoice[] = models.map((model) => ({
    label: model.label || model.value,
    value: model.value,
    description: model.description,
    supportsImage: model.supportsImage,
  }))
  const seen = new Set<string>()
  return choices.filter((choice) => {
    if (seen.has(choice.value)) return false
    seen.add(choice.value)
    return true
  })
}

function buildGroupChoices(groups: GroupOption[], currentGroup?: string) {
  const choices = groups.map((group) => ({
    ...group,
    label: group.label || group.value,
  }))

  if (currentGroup && !choices.some((group) => group.value === currentGroup)) {
    choices.unshift({
      label: currentGroup,
      value: currentGroup,
      ratio: 1,
    })
  }

  if (choices.length > 0) return choices

  return [
    {
      label: 'default',
      value: 'default',
      ratio: 1,
    },
  ]
}

function mergeSkillChoices(...groups: SkillChoice[][]) {
  const seen = new Set<string>()
  return groups.flat().filter((skill) => {
    if (seen.has(skill.value)) return false
    seen.add(skill.value)
    return true
  })
}

function skillChoicesForNodeKind(
  skills: SkillChoice[],
  nodeKind: AgentNodeKind
): SkillChoice[] {
  return skills.filter(
    (skill) =>
      !skill.appliesTo ||
      skill.appliesTo.length === 0 ||
      skill.appliesTo.includes(nodeKind)
  )
}

function readSkillNodeKinds(
  config?: Record<string, unknown>
): AgentNodeKind[] | undefined {
  const raw = config?.node_kinds
  if (!Array.isArray(raw)) return undefined
  const validKinds = new Set<AgentNodeKind>([
    'start',
    'schedule',
    'browser',
    'xianyu',
    'xianyu_reply',
    'xianyu_review',
    'request',
    'transform',
    'image',
    'report',
    'webpage',
    'notify',
    'stop',
    'custom',
  ])
  const kinds = raw.filter(
    (item): item is AgentNodeKind =>
      typeof item === 'string' && validKinds.has(item as AgentNodeKind)
  )
  return kinds.length > 0 ? kinds : undefined
}

function skillChoiceFromAgentSkill(skill: AgentSkillItem): SkillChoice | null {
  if (!skill.enabled) return null
  if (skill.source === 'builtin') return null
  const source =
    skill.source === 'marketplace' ||
    skill.source === 'github' ||
    skill.source === 'custom'
      ? skill.source
      : 'custom'
  const marketplaceId =
    typeof skill.config?.marketplace_id === 'string'
      ? skill.config.marketplace_id
      : undefined
  const capabilities = Array.isArray(skill.config?.capabilities)
    ? skill.config.capabilities.filter(
        (item): item is string => typeof item === 'string'
      )
    : undefined
  return {
    label: skill.name,
    value: skill.id,
    description: skill.description,
    source,
    reference: skill.source_url || marketplaceId,
    capabilities,
    marketplaceId,
    appliesTo: readSkillNodeKinds(skill.config),
  }
}

function skillChoiceFromMarketplaceItem(
  skill: AgentSkillMarketplaceItem
): SkillChoice {
  return {
    label: skill.name,
    value: skill.id,
    description: skill.description,
    source: 'marketplace',
    reference: skill.source_url,
    capabilities: skill.capabilities,
    marketplaceId: skill.id,
    appliesTo: readSkillNodeKinds(skill.config),
  }
}

function splitSkillTags(value: string) {
  return Array.from(
    new Set(
      value
        .split(/[,\n，、]/)
        .map((item) => item.trim())
        .filter(Boolean)
    )
  )
}

function readSavedSkills(): SkillChoice[] {
  if (typeof window === 'undefined') return []

  try {
    const raw = window.localStorage.getItem(AGENT_SKILLS_STORAGE_KEY)
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.filter(isSkillChoice)
  } catch {
    return []
  }
}

function writeSavedSkills(skills: SkillChoice[]) {
  if (typeof window === 'undefined') return

  try {
    window.localStorage.setItem(
      AGENT_SKILLS_STORAGE_KEY,
      JSON.stringify(skills)
    )
  } catch {
    /* localStorage may be unavailable or full. */
  }
}

function readSavedSkillStores(): SkillStore[] {
  if (typeof window === 'undefined') return [DEFAULT_SKILL_STORE]

  try {
    const raw = window.localStorage.getItem(AGENT_SKILL_STORES_STORAGE_KEY)
    if (!raw) return [DEFAULT_SKILL_STORE]
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return [DEFAULT_SKILL_STORE]
    return mergeSkillStores([
      DEFAULT_SKILL_STORE,
      ...parsed.filter(isSkillStore),
    ])
  } catch {
    return [DEFAULT_SKILL_STORE]
  }
}

function writeSavedSkillStores(stores: SkillStore[]) {
  if (typeof window === 'undefined') return

  try {
    window.localStorage.setItem(
      AGENT_SKILL_STORES_STORAGE_KEY,
      JSON.stringify(stores)
    )
  } catch {
    /* localStorage may be unavailable or full. */
  }
}

function mergeSkillStores(stores: SkillStore[]) {
  const seen = new Set<string>()
  return stores.filter((store) => {
    const key = `${store.name.trim().toLowerCase()}|${store.url ?? ''}`
    if (!store.name.trim() || seen.has(key)) return false
    seen.add(key)
    return true
  })
}

function isSkillStore(value: unknown): value is SkillStore {
  if (!value || typeof value !== 'object') return false
  const candidate = value as Partial<SkillStore>
  return (
    typeof candidate.id === 'string' &&
    typeof candidate.name === 'string' &&
    typeof candidate.description === 'string'
  )
}

function filterMarketplaceSkills(skills: SkillChoice[], search: string) {
  const keyword = search.trim().toLowerCase()
  if (!keyword) return skills
  return skills.filter((skill) =>
    [
      skill.label,
      skill.description,
      skill.reference ?? '',
      ...(skill.capabilities ?? []),
      ...(skill.tags ?? []),
    ]
      .join(' ')
      .toLowerCase()
      .includes(keyword)
  )
}

function getHotMarketplaceSkills(
  skills: SkillChoice[],
  installedMarketplaceIds: Set<string>
) {
  return skills
    .filter(
      (skill) =>
        !skill.marketplaceId ||
        !installedMarketplaceIds.has(skill.marketplaceId)
    )
    .slice(0, 6)
}

function isSkillInstalled(
  skill: SkillChoice,
  installedValues: Set<string>,
  installedMarketplaceIds: Set<string>
) {
  return (
    installedValues.has(skill.value) ||
    Boolean(
      skill.marketplaceId && installedMarketplaceIds.has(skill.marketplaceId)
    )
  )
}

function isSkillChoice(value: unknown): value is SkillChoice {
  if (!value || typeof value !== 'object') return false
  const candidate = value as Partial<SkillChoice>
  return (
    typeof candidate.label === 'string' &&
    typeof candidate.value === 'string' &&
    typeof candidate.description === 'string' &&
    (candidate.source === 'github' ||
      candidate.source === 'marketplace' ||
      candidate.source === 'custom')
  )
}

function readSavedBlueprints(): SavedAgentBlueprint[] {
  if (typeof window === 'undefined') return []

  try {
    const raw = window.localStorage.getItem(AGENT_BLUEPRINT_STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []

    return parsed
      .filter(isSavedBlueprint)
      .filter((blueprint) => blueprint.nodes.every(hasSupportedAgentNodeKind))
      .map((blueprint) => {
        const normalizedNodes = normalizeSavedBlueprintNodes(
          blueprint.nodes.map((node) => normalizeNode(node)),
          blueprint.status
        )
        const normalizedEdges = normalizeLegacyBranchEdges(
          normalizedNodes,
          blueprint.edges.map((edge) => normalizeEdge(edge))
        )
        return {
          ...blueprint,
          nodes: normalizedNodes,
          edges: normalizedEdges,
          status: normalizeSavedBlueprintStatus(
            normalizedNodes,
            blueprint.status
          ),
          executionLog: normalizeExecutionLog(blueprint.executionLog),
        }
      })
      .sort((a, b) => b.updatedAt - a.updatedAt)
  } catch {
    return []
  }
}

function mergeBackendBlueprints(
  localBlueprints: SavedAgentBlueprint[],
  backendBlueprints: Blueprint[],
  agentId: string
) {
  const localById = new Map(
    localBlueprints.map((blueprint) => [blueprint.id, blueprint])
  )
  const localByBackendId = new Map(
    localBlueprints
      .filter((blueprint) => blueprint.backendBlueprintId)
      .map((blueprint) => [blueprint.backendBlueprintId!, blueprint])
  )
  const restored: SavedAgentBlueprint[] = []
  const restoredIds = new Set<string>()

  for (const backendBlueprint of [...backendBlueprints].sort(
    (a, b) => b.updated_time - a.updated_time
  )) {
    if (backendBlueprintRecordType(backendBlueprint) === 'execution_snapshot') {
      continue
    }
    const backendNodes = backendBlueprint.nodes ?? []
    if (
      !backendNodes.every(isSerializedAgentNode) ||
      backendNodes.some((node) => !hasSupportedAgentNodeKind(node))
    ) {
      continue
    }

    const storedLocalId = backendBlueprintLocalId(backendBlueprint)
    const local =
      localByBackendId.get(backendBlueprint.id) ??
      (storedLocalId ? localById.get(storedLocalId) : undefined)
    const localId = storedLocalId ?? local?.id ?? backendBlueprint.id
    if (restoredIds.has(localId)) continue

    if (
      local?.syncState === 'draft' &&
      local.updatedAt > backendBlueprint.updated_time
    ) {
      restored.push({
        ...local,
        backendAgentId: agentId,
        backendBlueprintId: backendBlueprint.id,
      })
    } else {
      restored.push(
        savedBlueprintFromBackend(backendBlueprint, agentId, localId, local)
      )
    }
    restoredIds.add(localId)
  }

  for (const local of localBlueprints) {
    if (isLocalBlueprintDraft(local) && !restoredIds.has(local.id)) {
      restored.push({ ...local, syncState: 'draft' })
      restoredIds.add(local.id)
    }
  }

  return restored.sort((a, b) => b.updatedAt - a.updatedAt)
}

function isLocalBlueprintDraft(blueprint: SavedAgentBlueprint) {
  return blueprint.syncState === 'draft' || !blueprint.backendBlueprintId
}

function executionLogForBlueprintStatus(
  executionLog: AgentExecutionLogMap | undefined,
  status?: AgentBlueprintStatus
): AgentExecutionLogMap {
  const normalized = normalizeExecutionLog(executionLog)
  if (status === 'failed') {
    return Object.values(normalized).some((log) => log.status === 'failed')
      ? normalized
      : {}
  }
  if (status === 'paused') {
    return Object.values(normalized).some((log) => log.status === 'stopped')
      ? normalized
      : {}
  }
  return normalized
}

function savedBlueprintFromBackend(
  backendBlueprint: Blueprint,
  agentId: string,
  localId: string,
  local?: SavedAgentBlueprint
): SavedAgentBlueprint {
  const normalizedNodes = (backendBlueprint.nodes ?? [])
    .filter(isSerializedAgentNode)
    .map((node) => normalizeNode(node))
  const normalizedEdges = (backendBlueprint.edges ?? [])
    .filter(isSerializedAgentEdge)
    .map((edge) => normalizeEdge(edge))
  const legacyCompatibleEdges = normalizeLegacyBranchEdges(
    normalizedNodes,
    normalizedEdges
  )
  const status = normalizeSavedBlueprintStatus(
    normalizedNodes,
    localBlueprintStatusFromBackend(backendBlueprint.status)
  )

  return {
    id: localId,
    title: backendBlueprint.name,
    nodes: normalizedNodes,
    edges: legacyCompatibleEdges,
    createdAt: backendBlueprint.created_time,
    updatedAt: backendBlueprint.updated_time,
    status,
    executionLog: executionLogForBlueprintStatus(local?.executionLog, status),
    backendAgentId: agentId,
    backendBlueprintId: backendBlueprint.id,
    syncState: 'synced',
    knowledgeScope: knowledgeScopeFromBackendBlueprint(backendBlueprint),
  }
}

function knowledgeScopeFromBackendBlueprint(
  blueprint: Blueprint
): KnowledgeScopeValue {
  const rawIDs = blueprint.input_variables?.knowledge_base_ids
  const knowledgeBaseIds = Array.isArray(rawIDs)
    ? rawIDs.filter((value): value is string => typeof value === 'string')
    : []
  return {
    knowledgeBaseIds,
    knowledgeDisabled: knowledgeBaseIds.length === 0,
    knowledgeManual: true,
  }
}

function backendBlueprintLocalId(blueprint: Blueprint) {
  const value = blueprint.input_variables?.local_blueprint_id
  return typeof value === 'string' && value.trim() ? value : undefined
}

function backendBlueprintRecordType(blueprint: Blueprint) {
  const value = blueprint.input_variables?.record_type
  return typeof value === 'string' ? value : undefined
}

function localBlueprintStatusFromBackend(
  status: Blueprint['status']
): AgentBlueprintStatus {
  if (status === 'completed') return 'ended'
  if (status === 'paused') return 'paused'
  if (status === 'failed') return 'failed'
  if (status === 'running') return 'running'
  return 'pending'
}

function isSerializedAgentNode(value: unknown): value is AgentFlowNode {
  if (!value || typeof value !== 'object') return false
  const node = value as Partial<AgentFlowNode>
  return typeof node.id === 'string' && Boolean(node.data)
}

function isSerializedAgentEdge(value: unknown): value is AgentFlowEdge {
  if (!value || typeof value !== 'object') return false
  const edge = value as Partial<AgentFlowEdge>
  return typeof edge.source === 'string' && typeof edge.target === 'string'
}

function hasSupportedAgentNodeKind(node: AgentFlowNode) {
  return Object.prototype.hasOwnProperty.call(NODE_META, node.data?.kind)
}

function writeSavedBlueprints(blueprints: SavedAgentBlueprint[]) {
  if (typeof window === 'undefined') return

  try {
    window.localStorage.setItem(
      AGENT_BLUEPRINT_STORAGE_KEY,
      JSON.stringify(blueprints)
    )
  } catch {
    /* localStorage may be unavailable or full. */
  }
}

function waitForAgentBackendPoll(ms: number) {
  return new Promise((resolve) => {
    window.setTimeout(resolve, ms)
  })
}

async function syncSavedBlueprintToBackend(
  blueprint: SavedAgentBlueprint,
  options: BackendBlueprintFormOptions = {}
): Promise<SavedAgentBlueprint | null> {
  if (!options.modelsValidated) {
    await validateAgentNodeModels(blueprint.nodes)
  }
  const agent = await ensureCreatorStudioAgent()
  const payload = buildBackendBlueprintForm(blueprint, options)
  const saved =
    blueprint.backendBlueprintId && !options.forceCreate
      ? await updateBlueprint(agent.id, blueprint.backendBlueprintId, payload)
      : await createBlueprint(agent.id, payload)

  return {
    ...blueprint,
    createdAt: saved.created_time,
    updatedAt: saved.updated_time,
    backendAgentId: agent.id,
    backendBlueprintId: saved.id,
    syncState: 'synced',
  }
}

async function validateAgentNodeModels(nodes: AgentFlowNode[]) {
  const groups = Array.from(
    new Set(nodes.map((node) => node.data.group?.trim() || 'default'))
  )
  const entries = await Promise.all(
    groups.map(async (group) => [group, await getUserModels(group)] as const)
  )
  const modelsByGroup = new Map(
    entries.map(([group, groupModels]) => [
      group,
      new Set(groupModels.map((model) => model.value)),
    ])
  )

  for (const node of nodes) {
    const group = node.data.group?.trim() || 'default'
    const model = node.data.model.trim()
    const availableModels = modelsByGroup.get(group)
    if (!availableModels || availableModels.size === 0) {
      throw new Error(`模型分组「${group}」没有可用模型`)
    }
    if (!model || !availableModels.has(model)) {
      const modelLabel = model || '未选择'
      throw new Error(
        `节点「${node.data.title}」的模型「${modelLabel}」不属于分组「${group}」，请重新选择`
      )
    }
  }
}

async function runSavedBlueprintOnBackend(
  blueprint: SavedAgentBlueprint,
  options: BackendBlueprintFormOptions = {}
): Promise<SavedAgentBlueprint | null> {
  const synced = await syncSavedBlueprintToBackend(blueprint, options)
  if (!synced?.backendAgentId || !synced.backendBlueprintId) return synced
  if (options.triggerMode !== 'manual' && hasScheduleNode(blueprint.nodes)) {
    return synced
  }
  await runBlueprint(synced.backendAgentId, synced.backendBlueprintId)
  return synced
}

async function ensureCreatorStudioAgent() {
  const agents = await getAgents()
  const existing = agents.find((agent) => agent.name === '创作台智能体')
  if (existing) return existing
  return createAgent({
    name: '创作台智能体',
    description: '由创作台蓝图自动同步，用于后端正式执行节点化工作流。',
  })
}

function buildBackendBlueprintForm(
  blueprint: SavedAgentBlueprint,
  options: BackendBlueprintFormOptions = {}
) {
  const orderedNodes = orderNodes(blueprint.nodes, blueprint.edges)
  const firstNode = orderedNodes[0]
  const firstModelNode =
    orderedNodes.find((node) => node.data.model) ?? firstNode
  const skills = orderedNodes
    .map((node) => node.data.skill)
    .filter((skill): skill is string => Boolean(skill))
  return {
    name: blueprint.title,
    description:
      firstModelNode?.data.description ||
      firstNode?.data.description ||
      '创作台蓝图同步任务',
    trigger: buildBackendTriggerConfig(orderedNodes, options.triggerMode),
    model_group: firstModelNode?.data.group || 'default',
    model_name: firstModelNode?.data.model || '',
    skills: Array.from(new Set(skills)),
    input_variables: buildBackendInputVariables(blueprint, options),
    archive_config: {
      type: ArchiveType.PLATFORM,
      triggerOn: 'success' as const,
    },
    nodes: stripTransientNodeSettings(blueprint.nodes),
    edges: blueprint.edges,
  }
}

function buildBackendTriggerConfig(
  nodes: AgentFlowNode[],
  mode: BackendBlueprintFormOptions['triggerMode'] = 'auto'
): TriggerConfig {
  if (mode === 'manual') {
    return {
      type: TriggerType.MANUAL,
      timezone: 'Asia/Shanghai',
    }
  }

  const schedule = nodes.find((node) => node.data.kind === 'schedule')
  if (!schedule) {
    return {
      type: TriggerType.MANUAL,
      timezone: 'Asia/Shanghai',
    }
  }
  return {
    type: TriggerType.SCHEDULED,
    frequency: backendFrequencyFromLabel(schedule.data.settings['执行频率']),
    time: schedule.data.settings['执行时间'] || '08:30',
    timezone: schedule.data.settings['时区'] || 'Asia/Shanghai',
  }
}

function backendFrequencyFromLabel(value?: string): FrequencyType {
  if (value === '每小时') return FrequencyType.HOURLY
  if (value === '每周') return FrequencyType.WEEKLY
  if (value === '每月') return FrequencyType.MONTHLY
  if (value === '仅一次') return FrequencyType.ONCE
  return FrequencyType.DAILY
}

function buildBackendInputVariables(
  blueprint: SavedAgentBlueprint,
  options: BackendBlueprintFormOptions = {}
) {
  return {
    local_blueprint_id: blueprint.id,
    record_type: options.forceCreate ? 'execution_snapshot' : 'blueprint',
    title: blueprint.title,
    saved_at: blueprint.updatedAt,
    nodes_count: blueprint.nodes.length,
    edges_count: blueprint.edges.length,
    knowledge_base_ids: blueprint.knowledgeScope?.knowledgeBaseIds ?? [],
    knowledge_disabled: !blueprint.knowledgeScope?.knowledgeBaseIds?.length,
    knowledge_manual: true,
  }
}

function getBlueprintStatus(nodes: AgentFlowNode[]): AgentBlueprintStatus {
  if (nodes.some((node) => node.data.status === 'failed')) return 'failed'
  if (nodes.some((node) => node.data.status === 'running')) return 'running'
  if (hasScheduleNode(nodes)) return 'running'
  if (nodes.length > 0 && nodes.every((node) => node.data.status === 'done')) {
    return 'ended'
  }
  // paused is set externally via handleStopPreview
  return 'pending'
}

function normalizeSavedBlueprintNodes(
  nodes: AgentFlowNode[],
  savedStatus?: AgentBlueprintStatus
): AgentFlowNode[] {
  if (savedStatus !== 'ended') return nodes
  if (!nodes.some((node) => node.data.kind === 'schedule')) return nodes

  return nodes.map((node) =>
    node.data.kind === 'schedule' && node.data.status === 'done'
      ? { ...node, data: { ...node.data, status: 'waiting' } }
      : node
  )
}

function normalizeSavedBlueprintStatus(
  nodes: AgentFlowNode[],
  savedStatus?: AgentBlueprintStatus
): AgentBlueprintStatus {
  if (savedStatus === 'paused') return 'paused'
  if (savedStatus === 'failed') return 'failed'
  if (nodes.some((node) => node.data.status === 'failed')) return 'failed'
  if (hasScheduleNode(nodes)) return 'running'
  if (
    savedStatus === 'running' &&
    nodes.some((node) => node.data.status === 'running')
  ) {
    return 'running'
  }
  if (
    savedStatus === 'ended' &&
    nodes.some((node) => node.data.kind === 'schedule')
  ) {
    return 'running'
  }
  return savedStatus ?? getBlueprintStatus(nodes)
}

function getCurrentBlueprintStatus(
  nodes: AgentFlowNode[],
  savedStatus?: AgentBlueprintStatus
): AgentBlueprintStatus {
  // A persisted terminal state is authoritative when the node snapshot is
  // from an earlier run. Otherwise stale running/done nodes can mask a
  // backend failure or pause in the header and execution preview.
  if (savedStatus === 'paused') return 'paused'
  if (savedStatus === 'failed') return 'failed'
  if (nodes.some((node) => node.data.status === 'failed')) return 'failed'
  if (nodes.some((node) => node.data.status === 'running')) return 'running'
  if (hasScheduleNode(nodes)) return 'running'
  return getBlueprintStatus(nodes)
}

function hasScheduleNode(nodes: AgentFlowNode[]) {
  return nodes.some((node) => node.data.kind === 'schedule')
}

function hasManualTriggerNode(nodes: AgentFlowNode[]) {
  return nodes.some(
    (node) =>
      node.data.kind === 'start' ||
      node.data.componentId === 'component.webhook'
  )
}

function getRunPreviewButtonLabel(status: AgentBlueprintStatus) {
  if (status === 'paused') return '继续'
  if (status === 'ended' || status === 'failed') return '重新执行'
  return '试运行'
}

function buildSeedBlueprintTitle(prompt: string) {
  const normalized = prompt.replace(/\s+/g, ' ').trim()
  const title = normalized.replace(/^\/agent\s*/i, '')
  const chars = Array.from(title || '对话智能体任务')
  return `对话任务：${chars.slice(0, 20).join('')}`
}

function isSavedBlueprint(value: unknown): value is SavedAgentBlueprint {
  if (!value || typeof value !== 'object') return false
  const candidate = value as Partial<SavedAgentBlueprint>
  return (
    typeof candidate.id === 'string' &&
    typeof candidate.title === 'string' &&
    Array.isArray(candidate.nodes) &&
    Array.isArray(candidate.edges) &&
    typeof candidate.createdAt === 'number' &&
    typeof candidate.updatedAt === 'number'
  )
}

function normalizeExecutionLog(value: unknown): AgentExecutionLogMap {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}

  const out: AgentExecutionLogMap = {}
  for (const [nodeId, rawLog] of Object.entries(
    value as Record<string, unknown>
  )) {
    if (!rawLog || typeof rawLog !== 'object' || Array.isArray(rawLog)) {
      continue
    }

    const candidate = rawLog as Partial<AgentNodeExecutionLog>
    if (!isAgentNodeLogStatus(candidate.status)) continue

    out[nodeId] = {
      nodeId,
      status: candidate.status,
      startedAt: numberOrUndefined(candidate.startedAt),
      endedAt: numberOrUndefined(candidate.endedAt),
      durationMs: numberOrUndefined(candidate.durationMs),
      message: typeof candidate.message === 'string' ? candidate.message : '',
      inputSummary: stringArray(candidate.inputSummary),
      outputSummary: stringArray(candidate.outputSummary),
      modelOutput:
        typeof candidate.modelOutput === 'string' ? candidate.modelOutput : '',
      steps: stringArray(candidate.steps),
      error: typeof candidate.error === 'string' ? candidate.error : undefined,
    }
  }
  return out
}

function isAgentNodeLogStatus(status: unknown): status is AgentNodeLogStatus {
  return (
    status === 'pending' ||
    status === 'running' ||
    status === 'completed' ||
    status === 'stopped' ||
    status === 'failed'
  )
}

function numberOrUndefined(value: unknown) {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}

function stringArray(value: unknown) {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === 'string')
    : []
}

function updateNodeSettingValue(
  node: AgentFlowNode,
  key: string,
  value: string
): AgentFlowNode {
  const settings = {
    ...node.data.settings,
    [key]: value,
  }
  const outputs =
    node.data.kind === 'report' && key === REPORT_OUTPUT_FORMAT_SETTING
      ? reportOutputsFromFormatSetting(value, node.data.outputs)
      : node.data.outputs

  return {
    ...node,
    data: {
      ...node.data,
      settings,
      outputs,
    },
  }
}

function normalizeNode(node: AgentFlowNode): AgentFlowNode {
  const data = node.data ?? ({} as Partial<AgentNodeData>)
  const kind =
    data.kind && NODE_META[data.kind]
      ? data.kind
      : (COMPONENT_KIND_ALIASES[data.componentId ?? ''] ?? 'custom')
  const meta = NODE_META[kind]
  const outputs = Array.isArray(data.outputs) ? data.outputs : meta.outputs
  const settings = normalizeNodeSettings(kind, data.settings, meta.settings)
  if (kind === 'report' && !data.settings?.[REPORT_OUTPUT_FORMAT_SETTING]) {
    settings[REPORT_OUTPUT_FORMAT_SETTING] = normalizeReportOutputFormats(
      '',
      outputs
    ).join('、')
  }
  const normalizedOutputs =
    kind === 'report'
      ? reportOutputsFromFormatSetting(
          settings[REPORT_OUTPUT_FORMAT_SETTING] || '',
          outputs
        )
      : outputs

  return {
    ...node,
    id: node.id || `${kind}-${Date.now()}`,
    type: 'agentNode',
    position: node.position ?? { x: 0, y: 0 },
    data: {
      kind,
      componentId: data.componentId ?? `component.${kind.replace(/_/g, '-')}`,
      title: data.title ?? meta.label,
      description: data.description ?? meta.description,
      group: data.group,
      model: data.model?.startsWith('system.') ? '' : (data.model ?? ''),
      skill: normalizeBuiltinSkillValue(data.skill ?? pickDefaultSkill(kind)),
      inputs: Array.isArray(data.inputs) ? data.inputs : meta.inputs,
      outputs: normalizedOutputs,
      settings,
      status: data.status ?? 'ready',
    },
  }
}

function normalizeNodeSettings(
  kind: AgentNodeKind,
  settings: Record<string, string> | undefined,
  defaults: Record<string, string>
) {
  const normalized = { ...defaults, ...(settings ?? {}) }

  if (kind === 'xianyu_review') {
    // Existing blueprints may still carry settings from the old chat-list
    // implementation. Remove them so the editor and payload expose only the
    // evaluation-management workflow.
    for (const key of [
      '评价星级',
      '评价内容',
      '会话数量',
      '最大会话保护上限',
      '单会话等待',
      '对话列表选择器',
      '会话列表选择器',
      '订单区域选择器',
      '评价入口选择器',
    ]) {
      delete normalized[key]
    }
    normalized['评价内容'] = '沟通顺畅，交易愉快'
    if (
      !normalized['网址'] ||
      !normalized['网址'].includes('site=COMMONPRO') ||
      !normalized['网址'].includes('/seller-trade/evaluation-manage')
    ) {
      normalized['网址'] =
        'https://seller.goofish.com/?site=COMMONPRO#/seller-trade/evaluation-manage'
    }
  }

  if (kind === 'report') {
    delete normalized['模板目录']
    if (normalized['模板文件'] === LEGACY_TEMPLATE_DIR_REPORT) {
      normalized['模板文件'] = ''
    }
    normalized[REPORT_OUTPUT_FORMAT_SETTING] = normalizeReportOutputFormats(
      normalized[REPORT_OUTPUT_FORMAT_SETTING] || ''
    ).join('、')
  }
  if (kind === 'webpage') {
    delete normalized['模板目录']
    if (
      normalized['网页模板'] === '默认模板' ||
      normalized['网页模板'] === LEGACY_TEMPLATE_DIR_WEB
    ) {
      normalized['网页模板'] = ''
    }
  }
  if (kind === 'browser') {
    for (const key of TRANSIENT_BROWSER_SETTING_LABELS) {
      delete normalized[key]
    }
    if (!normalized['采集等待']) {
      normalized['采集等待'] = '6s'
    }
    if (!normalized['登录等待']) {
      normalized['登录等待'] = '120s'
    }
    if (!normalized['会话名称']) {
      normalized['会话名称'] = '默认'
    }
    if (
      normalized['登录态'] === '使用当前设备浏览器' ||
      normalized['登录态'] === '使用已保存登录态'
    ) {
      normalized['登录态'] = '持久化登录会话（首次登录后复用）'
    }
  }
  if (kind === 'request') {
    if (!normalized['网址'] && normalized['请求URL']) {
      normalized['网址'] = normalized['请求URL']
    }
    delete normalized['请求URL']
    for (const key of TRANSIENT_HTTP_SETTING_LABELS) {
      delete normalized[key]
    }
  }
  if (kind === 'image') {
    for (const key of TRANSIENT_IMAGE_SETTING_LABELS) {
      delete normalized[key]
    }
  }
  if (kind === 'stop') {
    delete normalized['归档']
    delete normalized['输出目录']
    delete normalized['归档位置']
  }

  if (
    normalized['输出目录'] === LEGACY_OUTPUT_DIR_REPORT ||
    (kind === 'report' && !normalized['输出目录'])
  ) {
    normalized['输出目录'] = DEFAULT_OUTPUT_DIR_REPORT
  }
  if (
    normalized['输出目录'] === LEGACY_OUTPUT_DIR_WEB ||
    (kind === 'webpage' && !normalized['输出目录'])
  ) {
    normalized['输出目录'] = DEFAULT_OUTPUT_DIR_WEB
  }
  return normalized
}

function normalizeEdge(edge: AgentFlowEdge): AgentFlowEdge {
  return decorateEdge(edge)
}

function normalizeLegacyBranchEdges(
  nodes: AgentFlowNode[],
  edges: AgentFlowEdge[]
) {
  const branchNodeIds = new Set(
    nodes
      .filter((node) => node.data.componentId === 'component.branch')
      .map((node) => node.id)
  )
  const branchEdgeCounts = new Map<string, number>()
  return edges.map((edge) => {
    if (
      !branchNodeIds.has(edge.source) ||
      edge.sourceHandle ||
      edge.targetHandle
    ) {
      return edge
    }
    const index = branchEdgeCounts.get(edge.source) ?? 0
    branchEdgeCounts.set(edge.source, index + 1)
    return normalizeEdge({
      ...edge,
      sourceHandle: index === 0 ? BRANCH_TRUE_HANDLE : BRANCH_FALSE_HANDLE,
    })
  })
}

function formatBlueprintTime(timestamp: number) {
  const date = new Date(timestamp)
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  const hour = `${date.getHours()}`.padStart(2, '0')
  const minute = `${date.getMinutes()}`.padStart(2, '0')
  return `${month}-${day} ${hour}:${minute}`
}

function buildTemplates(
  models: ModelOption[],
  currentModel?: string,
  currentGroup?: string,
  components: AgentComponentItem[] = []
) {
  const baseTemplates = (Object.keys(NODE_META) as AgentNodeKind[])
    // Business-specific workflows are cataloged as custom components.
    .filter(
      (kind) =>
        kind !== 'custom' && kind !== 'xianyu' && kind !== 'xianyu_reply' && kind !== 'xianyu_review'
    )
    .map((kind) => {
      const meta = NODE_META[kind]
      return {
        kind,
        componentId: kind,
        source: 'builtin' as const,
        title: meta.label,
        description: meta.description,
        paletteGroup: meta.group,
        group: currentGroup || 'default',
        model: pickDefaultModel(kind, models, currentModel),
        skill: pickDefaultSkill(kind),
        inputs: meta.inputs,
        outputs: meta.outputs,
        settings: meta.settings,
        status: 'ready' as AgentNodeStatus,
      }
    })
  if (!components.length) return baseTemplates

  const baseComponentIds = new Set(
    baseTemplates.flatMap((template) => [
      template.componentId ?? template.kind,
      `component.${template.kind}`,
      `component.${template.kind.replace(/_/g, '-')}`,
    ])
  )
  const dynamicTemplates = components
    .filter((component) =>
      shouldUseCatalogComponent(component, baseComponentIds)
    )
    .map((component) =>
      componentToNodeTemplate(component, models, currentModel, currentGroup)
    )
  return [...baseTemplates, ...dynamicTemplates]
}

function shouldUseCatalogComponent(
  component: AgentComponentItem,
  baseComponentIds: Set<string>
) {
  if (!component.enabled || !isSupportedComponentGroup(component.group)) {
    return false
  }
  if (component.source === 'custom') return true
  if (baseComponentIds.has(component.id)) return false

  return normalizeTemplateKind(component.kind) === 'custom'
}

const SUPPORTED_COMPONENT_GROUPS = new Set([
  'trigger',
  'collect',
  'collection',
  'source',
  'process',
  'processor',
  'transform',
  'control',
  'flow',
  'workflow',
  'output',
  'sink',
  '触发',
  '采集',
  '处理',
  '落档',
])

function isSupportedComponentGroup(group?: string) {
  const normalized = group?.trim().toLowerCase()
  if (!normalized) return true
  return SUPPORTED_COMPONENT_GROUPS.has(normalized)
}

function componentToNodeTemplate(
  component: AgentComponentItem,
  models: ModelOption[],
  currentModel?: string,
  currentGroup?: string
): NodeTemplate {
  const kind = normalizeTemplateKind(component.kind)
  const meta = NODE_META[kind]
  const config = component.config ?? {}
  const group = config.model_group || currentGroup || 'default'
  const configuredModel =
    group === (currentGroup || 'default') &&
    models.some((model) => model.value === config.model_name)
      ? config.model_name
      : undefined
  const settings = {
    ...meta.settings,
    ...(config.settings ?? {}),
    步骤来源: component.source === 'custom' ? '自定义' : '系统内置',
  }
  return {
    kind,
    componentId: component.id,
    source: component.source,
    title: component.name || meta.label,
    description: component.description || meta.description,
    paletteGroup: componentPaletteGroup(component.group, meta.group),
    group,
    model:
      group === (currentGroup || 'default')
        ? configuredModel || pickDefaultModel(kind, models, currentModel)
        : '',
    skill: config.skill || pickDefaultSkill(kind),
    inputs: config.inputs?.length ? config.inputs : meta.inputs,
    outputs: config.outputs?.length ? config.outputs : meta.outputs,
    settings,
    status: 'ready',
  }
}

function normalizeTemplateKind(kind: string): AgentNodeKind {
  return kind && NODE_META[kind as AgentNodeKind]
    ? (kind as AgentNodeKind)
    : 'custom'
}

function componentPaletteGroup(
  group: string | undefined,
  fallback: AgentPaletteGroup
) {
  switch ((group ?? '').trim().toLowerCase()) {
    case 'trigger':
    case '触发':
      return '触发'
    case 'collect':
    case 'collection':
    case 'source':
    case '采集':
      return '采集'
    case 'control':
    case 'flow':
    case 'workflow':
      return '处理'
    case 'output':
    case 'sink':
    case '落档':
      return '落档'
    case 'process':
    case 'processor':
    case 'transform':
    case '处理':
      return '处理'
    default:
      return fallback
  }
}

function pickDefaultModel(
  kind: AgentNodeKind,
  models: ModelOption[],
  currentModel?: string
) {
  const meta = NODE_META[kind]
  const model = chooseModelByRole(models, meta.role, currentModel)
  return model?.value ?? ''
}

function pickDefaultSkill(kind: AgentNodeKind) {
  return DEFAULT_SKILL_BY_NODE_KIND[kind] ?? NO_SKILL_VALUE
}

function getSkillChoice(value: string, skillChoices: SkillChoice[]) {
  const normalizedValue = normalizeBuiltinSkillValue(value)
  if (!normalizedValue) return NO_SKILL_CHOICE
  return (
    skillChoices.find((skill) => skill.value === normalizedValue) ?? {
      label: normalizedValue,
      value: normalizedValue,
      description: '自定义 Skill',
      source: 'custom',
    }
  )
}

function normalizeBuiltinSkillValue(value: string) {
  return LEGACY_BUILTIN_SKILL_ALIASES[value] ?? value
}

function formatSkillSource(source: SkillSource) {
  const labels: Record<SkillSource, string> = {
    'built-in': '内置',
    github: 'GitHub',
    marketplace: 'Skill 商城',
    custom: '自定义',
  }
  return labels[source]
}

function getSkillSourceBadgeClass(source: SkillSource) {
  if (source === 'github') return 'bg-purple-500/10 text-purple-600'
  if (source === 'marketplace') return 'bg-blue-500/10 text-blue-600'
  if (source === 'custom') return 'bg-amber-500/10 text-amber-600'
  return 'bg-emerald-500/10 text-emerald-600'
}

function formatSkillNameFromReference(reference: string) {
  const trimmed = reference.trim().replace(/\/+$/, '')
  const parts = trimmed.split(/[\\/]/).filter(Boolean)
  return parts.at(-1)?.replace(/[-_]+/g, ' ') || 'GitHub'
}

function createStableId(value: string) {
  const slug = value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/(^-|-$)/g, '')
    .slice(0, 48)
  return `${slug || 'skill'}-${Date.now().toString(36)}`
}

function chooseModelByRole(
  models: ModelOption[],
  role: ModelRole,
  currentModel?: string
) {
  const selectedModel = models.find((model) => model.value === currentModel)
  if (role === 'system') return selectedModel ?? models[0]
  if (role === 'image') {
    return (
      models.find((model) => model.supportsImage) ??
      findByPatterns(models, ['gpt-image', 'image', 'imagen', 'dall']) ??
      selectedModel ??
      models[0]
    )
  }
  if (role === 'browser') {
    return (
      findByPatterns(models, [
        'computer',
        'browser',
        'claude',
        'sonnet',
        'gpt-4.1',
        'gpt-4o',
        'gemini',
      ]) ??
      selectedModel ??
      models[0]
    )
  }
  if (role === 'reasoning') {
    return (
      findByPatterns(models, [
        'o3',
        'o4',
        'reason',
        'terra',
        'gpt-4.1',
        'sonnet',
        'deepseek',
        'qwen',
      ]) ??
      selectedModel ??
      models[0]
    )
  }
  return (
    findByPatterns(models, ['mini', 'flash', 'turbo', 'haiku', 'lite']) ??
    selectedModel ??
    models[0]
  )
}

function findByPatterns(models: ModelOption[], patterns: string[]) {
  return models.find((model) => {
    const haystack = `${model.label} ${model.value} ${model.category ?? ''}`
      .toLowerCase()
      .trim()
    return patterns.some((pattern) => haystack.includes(pattern))
  })
}

function createInitialNodes(templates: NodeTemplate[]) {
  const byKind = new Map(templates.map((template) => [template.kind, template]))
  const overrides: Array<{
    kind: AgentNodeKind
    id: string
    position: { x: number; y: number }
    patch: Partial<NodeTemplate>
  }> = [
    {
      kind: 'start',
      id: 'start',
      position: { x: -560, y: 0 },
      patch: {
        title: '触发',
        description: '设置任务何时开始，可改为定时或外部事件触发。',
      },
    },
    {
      kind: 'request',
      id: 'collect',
      position: { x: -280, y: 0 },
      patch: {
        title: '采集',
        description: '从网页、接口、文件或数据库取得本次任务需要的数据。',
        settings: {
          网址: '',
          请求参数: '',
        },
      },
    },
    {
      kind: 'transform',
      id: 'process',
      position: { x: 0, y: 0 },
      patch: {
        title: '处理',
        description: '说明要如何筛选、判断、汇总或转换采集到的数据。',
      },
    },
    {
      kind: 'report',
      id: 'archive',
      position: { x: 280, y: 0 },
      patch: {
        title: '落档',
        description: '将处理结果保存为文件或报告，也可替换为网页或通知。',
      },
    },
    {
      kind: 'stop',
      id: 'stop',
      position: { x: 560, y: 0 },
      patch: {
        title: '结束',
        description: '记录任务完成状态并结束本轮流程。',
      },
    },
  ]

  return overrides
    .map(({ kind, id, position, patch }) => {
      const template = byKind.get(kind)
      if (!template) return null
      return createNodeFromTemplate(
        {
          ...template,
          ...patch,
          settings: {
            ...template.settings,
            ...patch.settings,
          },
        },
        id,
        position
      )
    })
    .filter((node): node is AgentFlowNode => Boolean(node))
}

function createNodeFromTemplate(
  template: NodeTemplate,
  id: string,
  position: { x: number; y: number }
): AgentFlowNode {
  return {
    id,
    type: 'agentNode',
    position,
    data: {
      kind: template.kind,
      componentId: template.componentId,
      title: template.title,
      description: template.description,
      group: template.group,
      model: template.model,
      skill: template.skill,
      inputs: [...template.inputs],
      outputs: [...template.outputs],
      settings: { ...template.settings },
      status: template.status ?? 'ready',
    },
  }
}

function ensureTerminalStopNode(
  nodes: AgentFlowNode[],
  edges: AgentFlowEdge[],
  templateByKind: Map<AgentNodeKind, NodeTemplate>
) {
  const stopTemplate = templateByKind.get('stop')
  if (!stopTemplate) return { nodes, edges, changed: false }

  let changed = false
  let nextNodes = nodes
  const stopNode =
    nodes.find((node) => node.data.kind === 'stop') ??
    createNodeFromTemplate(stopTemplate, `stop-${Date.now()}`, {
      x: Math.max(...nodes.map((node) => node.position.x), 0) + 350,
      y: findTerminalSourceNode(nodes, edges)?.position.y ?? 95,
    })

  if (!nodes.some((node) => node.id === stopNode.id)) {
    nextNodes = [...nodes, stopNode]
    changed = true
  }

  const terminalSource = findTerminalSourceNode(nextNodes, edges)
  if (!terminalSource || terminalSource.id === stopNode.id) {
    return { nodes: nextNodes, edges, changed }
  }

  const hasStopIncoming = edges.some((edge) => edge.target === stopNode.id)
  if (hasStopIncoming) {
    return { nodes: nextNodes, edges, changed }
  }

  return {
    nodes: nextNodes,
    edges: [
      ...edges,
      decorateEdge({
        id: `e-${terminalSource.id}-${stopNode.id}`,
        source: terminalSource.id,
        target: stopNode.id,
      }),
    ],
    changed: true,
  }
}

function findTerminalSourceNode(
  nodes: AgentFlowNode[],
  edges: AgentFlowEdge[]
) {
  const outgoing = new Set(edges.map((edge) => edge.source).filter(Boolean))
  const preferred = nodes.find((node) => node.data.kind === 'report')
  if (preferred && !outgoing.has(preferred.id)) return preferred

  return [...nodes]
    .filter((node) => node.data.kind !== 'stop' && !outgoing.has(node.id))
    .sort(
      (a, b) => b.position.x - a.position.x || b.position.y - a.position.y
    )[0]
}

function decorateEdge(edge: AgentEdgeDraft): AgentFlowEdge {
  const route = agentRouteFromHandle(edge.sourceHandle)
  const branchRoute = route?.kind === 'branch' ? route.branchRoute : undefined
  const stroke =
    branchRoute === 'true'
      ? '#10b981'
      : branchRoute === 'false'
        ? '#f43f5e'
        : route?.key === SWITCH_DEFAULT_HANDLE ||
            route?.key === LOOP_DONE_HANDLE
          ? '#64748b'
          : route?.kind === 'loop'
            ? '#6366f1'
            : route?.kind === 'switch'
              ? '#0ea5e9'
              : 'var(--primary)'
  return {
    ...edge,
    id:
      edge.id ??
      `e-${edge.source ?? 'source'}-${edge.target ?? 'target'}-${Date.now()}`,
    source: edge.source ?? '',
    target: edge.target ?? '',
    type: 'deletable',
    animated: true,
    deletable: true,
    focusable: true,
    interactionWidth: 18,
    markerEnd: { type: MarkerType.ArrowClosed },
    selectable: true,
    data: {
      ...edge.data,
      ...(route
        ? {
            routeKey: route.key,
            routeKind: route.kind,
            branchRoute,
            routeLabel: edge.data?.routeLabel ?? defaultAgentRouteLabel(route),
          }
        : {}),
    },
    style: {
      ...edge.style,
      stroke,
      strokeWidth: 1.8,
    },
  } as AgentFlowEdge
}

function agentRouteFromHandle(handle?: string | null) {
  if (handle === BRANCH_TRUE_HANDLE) {
    return {
      key: BRANCH_TRUE_HANDLE,
      kind: 'branch' as const,
      branchRoute: 'true' as const,
    }
  }
  if (handle === BRANCH_FALSE_HANDLE) {
    return {
      key: BRANCH_FALSE_HANDLE,
      kind: 'branch' as const,
      branchRoute: 'false' as const,
    }
  }
  if (handle?.startsWith(SWITCH_HANDLE_PREFIX)) {
    return { key: handle, kind: 'switch' as const }
  }
  if (handle === LOOP_ITEM_HANDLE || handle === LOOP_DONE_HANDLE) {
    return { key: handle, kind: 'loop' as const }
  }
  return undefined
}

function defaultAgentRouteLabel(
  route: NonNullable<ReturnType<typeof agentRouteFromHandle>>
) {
  if (route.kind === 'branch' && route.branchRoute === 'true') {
    return '条件成立'
  }
  if (route.kind === 'branch' && route.branchRoute === 'false') {
    return '条件不成立'
  }
  if (route.key === SWITCH_DEFAULT_HANDLE) return '其他'
  if (route.kind === 'loop' && route.key === LOOP_ITEM_HANDLE) return '逐项处理'
  if (route.kind === 'loop' && route.key === LOOP_DONE_HANDLE) return '循环完成'
  const index = Number.parseInt(
    route.key.slice(SWITCH_HANDLE_PREFIX.length),
    10
  )
  return Number.isFinite(index) ? `分支 ${index + 1}` : '分支'
}

function buildPaletteTemplateGroups(
  templates: NodeTemplate[]
): PaletteTemplateGroup[] {
  return PALETTE_GROUPS.map((group) => ({
    title: group.title,
    templates: templates.filter((template) => {
      const meta = NODE_META[template.kind]
      return (template.paletteGroup ?? meta.group) === group.title
    }),
  })).filter((group) => group.templates.length > 0)
}

function buildComponentPaletteSections(
  groups: PaletteTemplateGroup[]
): ComponentPaletteSection[] {
  const templates = groups.flatMap((group) => group.templates)
  const customTemplates = templates.filter(
    (template) => template.source === 'custom'
  )
  const builtinSections = groups
    .map((group) => ({
      title: group.title,
      badge: '内置',
      templates: group.templates.filter(
        (template) => template.source !== 'custom'
      ),
    }))
    .filter((section) => section.templates.length > 0)

  return [
    ...(customTemplates.length
      ? [
          {
            title: '自定义步骤',
            badge: '自定义',
            templates: customTemplates,
          },
        ]
      : []),
    ...builtinSections,
  ]
}

function filterPaletteTemplateGroups(
  groups: PaletteTemplateGroup[],
  search: string
) {
  const keyword = search.trim().toLowerCase()
  if (!keyword) return groups

  return groups
    .map((group) => ({
      ...group,
      templates: group.templates.filter((template) => {
        const meta = NODE_META[template.kind]
        return `${template.title} ${template.paletteGroup ?? meta.group} ${template.description} ${
          template.source === 'custom' ? '自定义' : '内置'
        }`
          .toLowerCase()
          .includes(keyword)
      }),
    }))
    .filter((group) => group.templates.length > 0)
}

function splitListInput(value: string) {
  return value
    .split(/[,，\n]+/)
    .map((item) => item.trim())
    .filter(Boolean)
}

function buildManualPreviewGraph(
  nodes: AgentFlowNode[],
  edges: AgentFlowEdge[]
) {
  const nodeById = new Map(nodes.map((node) => [node.id, node]))
  const startIds = nodes
    .filter(
      (node) =>
        node.data.kind === 'start' ||
        node.data.componentId === 'component.webhook'
    )
    .map((node) => node.id)

  if (startIds.length === 0) {
    return filterGraphByNodeIds(nodes, edges, new Set())
  }

  const outgoing = new Map<string, string[]>()
  for (const edge of edges) {
    if (!edge.source || !edge.target) continue
    outgoing.set(edge.source, [
      ...(outgoing.get(edge.source) ?? []),
      edge.target,
    ])
  }

  const reachable = new Set<string>()
  const queue = [...startIds]
  while (queue.length > 0) {
    const id = queue.shift()
    if (!id || reachable.has(id)) continue
    const node = nodeById.get(id)
    if (!node || node.data.kind === 'schedule') continue

    reachable.add(id)
    for (const targetId of outgoing.get(id) ?? []) {
      if (!reachable.has(targetId)) {
        queue.push(targetId)
      }
    }
  }

  return filterGraphByNodeIds(nodes, edges, reachable)
}

function filterGraphByNodeIds(
  nodes: AgentFlowNode[],
  edges: AgentFlowEdge[],
  nodeIds: Set<string>
) {
  return {
    nodes: nodes.filter((node) => nodeIds.has(node.id)),
    edges: edges.filter(
      (edge) => nodeIds.has(edge.source) && nodeIds.has(edge.target)
    ),
  }
}

function isAgentConnectionValid(
  nodes: AgentFlowNode[],
  edges: AgentFlowEdge[],
  connection: Pick<Connection, 'source' | 'target'>
) {
  const source = connection.source
  const target = connection.target
  if (!source || !target || source === target) return false
  const nodeIds = new Set(nodes.map((node) => node.id))
  if (!nodeIds.has(source) || !nodeIds.has(target)) return false

  const outgoing = new Map<string, string[]>()
  for (const edge of edges) {
    outgoing.set(edge.source, [
      ...(outgoing.get(edge.source) ?? []),
      edge.target,
    ])
  }
  outgoing.set(source, [...(outgoing.get(source) ?? []), target])

  const visited = new Set<string>()
  const queue = [target]
  while (queue.length > 0) {
    const id = queue.shift()
    if (!id || visited.has(id)) continue
    if (id === source) return false
    visited.add(id)
    queue.push(...(outgoing.get(id) ?? []))
  }
  return true
}

function orderNodes(nodes: AgentFlowNode[], edges: AgentFlowEdge[]) {
  const byId = new Map(nodes.map((node) => [node.id, node]))
  const outgoing = new Map<string, string[]>()
  const incomingCount = new Map(nodes.map((node) => [node.id, 0]))

  for (const edge of edges) {
    if (!edge.source || !edge.target) continue
    outgoing.set(edge.source, [
      ...(outgoing.get(edge.source) ?? []),
      edge.target,
    ])
    incomingCount.set(edge.target, (incomingCount.get(edge.target) ?? 0) + 1)
  }

  const queue = nodes
    .filter((node) => (incomingCount.get(node.id) ?? 0) === 0)
    .sort((a, b) => a.position.x - b.position.x || a.position.y - b.position.y)
  const ordered: AgentFlowNode[] = []
  const seen = new Set<string>()

  while (queue.length > 0) {
    const node = queue.shift()
    if (!node || seen.has(node.id)) continue
    seen.add(node.id)
    ordered.push(node)
    for (const targetId of outgoing.get(node.id) ?? []) {
      const target = byId.get(targetId)
      if (target && !seen.has(target.id)) {
        queue.push(target)
      }
    }
    queue.sort(
      (a, b) => a.position.x - b.position.x || a.position.y - b.position.y
    )
  }

  const remaining = nodes
    .filter((node) => !seen.has(node.id))
    .sort((a, b) => a.position.x - b.position.x || a.position.y - b.position.y)

  return [...ordered, ...remaining]
}
