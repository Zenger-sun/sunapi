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
  BotIcon,
  ChartNetworkIcon,
  CheckCircle2Icon,
  ChevronDownIcon,
  ChevronUpIcon,
  ClockIcon,
  CpuIcon,
  FileSpreadsheetIcon,
  FileTextIcon,
  GlobeIcon,
  ImageIcon,
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
  Trash2Icon,
  Undo2Icon,
  XIcon,
  ZapIcon,
  type LucideIcon,
} from 'lucide-react'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  NativeSelect,
  NativeSelectOptGroup,
  NativeSelectOption,
} from '@/components/ui/native-select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import {
  captureAgentBrowserPage,
  listAgentFiles,
  uploadAgentFile,
  writeAgentReport,
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
import type {
  AgentFileUpload,
  AgentFileUploadPurpose,
  AgentReportOutputFile,
  AgentReportWrittenFile,
  GroupOption,
  ModelOption,
} from '../types'

type AgentNodeKind =
  | 'start'
  | 'schedule'
  | 'browser'
  | 'request'
  | 'transform'
  | 'image'
  | 'report'
  | 'webpage'
  | 'notify'
  | 'stop'

type AgentNodeStatus = 'ready' | 'waiting' | 'running' | 'done'
type AgentNodeLogStatus = 'pending' | 'running' | 'completed' | 'stopped'

type ModelRole = 'system' | 'browser' | 'fast' | 'reasoning' | 'image'

type AgentNodeData = {
  kind: AgentNodeKind
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
type AgentFlowEdge = Edge
type AgentEdgeDraft = Partial<AgentFlowEdge> & {
  source?: string | null
  target?: string | null
}

type ModelChoice = {
  label: string
  value: string
  description?: string
  supportsImage?: boolean
  source?: 'system' | 'current' | 'fallback'
}

type SkillChoice = {
  label: string
  value: string
  description: string
  source: SkillSource
  reference?: string
  capabilities?: string[]
}

type SkillSource = 'built-in' | 'github' | 'marketplace' | 'custom'

type SkillDraftSource = Exclude<SkillSource, 'built-in'>

type AgentToolPanel = 'components' | 'skills'

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

type XianyuOverviewMetric = {
  label: string
  current: string
  previous: string
  change: string
}

type NodeTemplate = {
  kind: AgentNodeKind
  title: string
  description: string
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
const MAX_BLUEPRINT_HISTORY = 50

const NO_SKILL_VALUE = ''
const NO_SKILL_CHOICE: SkillChoice = {
  label: '不使用 Skill',
  value: NO_SKILL_VALUE,
  description: '该节点仅按基础提示词执行，不叠加任何 Skill 优化。',
  source: 'built-in',
}

const BUILTIN_SKILL_DEFINITIONS: BuiltinSkillDefinition[] = [
  {
    label: '网页研究 Skill',
    value: 'skill.web-research',
    description: '检索、网页阅读和事实核验，适合资料收集与趋势调研。',
    source: 'built-in',
    appliesTo: ['browser', 'request', 'webpage', 'report'],
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
    appliesTo: ['transform', 'report', 'webpage'],
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
    appliesTo: ['browser'],
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
    appliesTo: ['transform', 'image', 'report'],
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
    appliesTo: ['request', 'notify'],
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
    appliesTo: ['browser', 'request', 'transform', 'report'],
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
    appliesTo: ['request', 'transform', 'report'],
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
      '适配 MCP/GitHub/文件系统等工具编排，强调权限边界和可追踪步骤。',
    source: 'built-in',
    appliesTo: ['start', 'schedule', 'notify', 'stop'],
    capabilities: ['工具选择', '权限边界', '步骤追踪'],
    instructions: [
      '根据任务选择最少必要工具，说明每一步输入、输出和失败补偿方案。',
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
  request: 'skill.api-function',
  transform: 'skill.code-interpreter',
  image: 'skill.code-interpreter',
  report: 'skill.file-document',
  webpage: 'skill.file-document',
  notify: 'skill.api-function',
  stop: 'skill.mcp-workflow',
}

const MARKETPLACE_SKILLS: SkillChoice[] = [
  {
    label: '电商运营分析 Skill',
    value: 'skill.marketplace.ecommerce-ops',
    description: '面向商品指标、竞品跟踪和日报建议的运营 Skill。',
    source: 'marketplace',
    reference: 'SunAPI Skill 商城',
  },
  {
    label: '工单总结 Skill',
    value: 'skill.marketplace.ticket-summary',
    description: '整理客服工单、归类问题并生成跟进清单。',
    source: 'marketplace',
    reference: 'SunAPI Skill 商城',
  },
  {
    label: '财务对账 Skill',
    value: 'skill.marketplace.reconciliation',
    description: '抽取账单字段、核对差异并生成归档记录。',
    source: 'marketplace',
    reference: 'SunAPI Skill 商城',
  },
]

const FREQUENCY_OPTIONS = ['每天', '工作日', '每周', '每月', '每小时']
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
const BROWSER_DATA_MISSING_MARKER = 'browser_data_missing'
const XIANYU_OVERVIEW_METRICS_MARKER = 'xianyu_overview_metrics'
const DEFAULT_REPORT_OUTPUT_FORMATS = ['Markdown 日报', 'CSV 表格']
const REPORT_OUTPUT_FORMAT_PRESETS = [
  'Markdown 日报',
  'CSV 表格',
  'Excel 表格',
  'Word 文档',
  'PDF 文档',
  'HTML 网页',
  'TXT 文本',
  'JSON 数据',
]
const XIANYU_OVERVIEW_METRIC_LABELS = [
  '商品访问次数',
  '商品访问人数',
  '商品曝光次数',
  '商品曝光人数',
  '商品浏览次数',
  '商品浏览人数',
  '支付笔数',
  '支付金额（元）',
  '发起退款笔数',
  '发起退款金额（元）',
]
const SETTING_SELECT_OPTIONS: Record<string, string[]> = {
  触发方式: ['手动', '定时', 'Webhook', '对话触发'],
  运行模式: ['单次执行', '循环执行', '仅预览', '失败后重试'],
  登录态: ['使用当前设备浏览器', '使用已保存登录态', '无需登录'],
  权限: ['只读采集', '读写操作需确认', '禁止写入'],
  方法: ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'],
  超时: ['10s', '30s', '60s', '120s'],
  重试: ['0 次', '1 次', '2 次', '3 次'],
  数据类型: [
    '自动识别',
    '商品运营数据',
    '交易/订单数据',
    '用户行为数据',
    '日志/事件数据',
    '财务/对账数据',
    '通用表格/CSV',
    'JSON 对象/数组',
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
  风格: ['清爽数据看板', '运营日报封面', '极简图表', '品牌营销图'],
  数量: ['1', '2', '3', '4'],
  格式: ['Markdown + CSV', 'Markdown', 'PDF', 'Markdown + 图片'],
  语气: ['运营复盘', '简洁客观', '专业分析', '行动建议'],
  渠道: ['邮件', '企业微信', '钉钉', '飞书', 'Slack', 'Webhook'],
  确认: ['发送前确认', '自动发送', '失败时确认'],
  结束方式: ['本轮完成后停止', '达到次数后停止', '手动停止', '出错停止'],
  日报模板: ['默认模板', '简洁版', '详细分析版', '自定义模板'],
  输出模式: ['输出网页 + 数据', '仅输出数据'],
}

const MODEL_FALLBACKS: Record<ModelRole, ModelChoice[]> = {
  system: [
    {
      label: 'SunAPI System Runner',
      value: 'system.runner',
      description: '本地编排执行器',
    },
    {
      label: 'SunAPI Scheduler',
      value: 'system.scheduler',
      description: '定时任务执行器',
    },
  ],
  browser: [
    {
      label: 'Claude Sonnet 4',
      value: 'claude-sonnet-4',
      description: '浏览器操作与页面理解',
    },
    {
      label: 'GPT-4.1',
      value: 'gpt-4.1',
      description: '网页任务理解',
    },
  ],
  fast: [
    {
      label: 'GPT-4.1 mini',
      value: 'gpt-4.1-mini',
      description: '低成本结构化处理',
    },
    {
      label: 'Qwen Turbo',
      value: 'qwen-turbo',
      description: '快速文本处理',
    },
  ],
  reasoning: [
    {
      label: 'GPT-4.1',
      value: 'gpt-4.1',
      description: '复杂整理与报告生成',
    },
    {
      label: 'Claude Sonnet 4',
      value: 'claude-sonnet-4',
      description: '多步骤分析',
    },
  ],
  image: [
    {
      label: 'GPT Image 1',
      value: 'gpt-image-1',
      description: '图像生成',
      supportsImage: true,
    },
    {
      label: 'Imagen',
      value: 'imagen',
      description: '图片生成模型',
      supportsImage: true,
    },
  ],
}

const NODE_META: Record<
  AgentNodeKind,
  {
    label: string
    group: '触发' | '采集' | '智能处理' | '输出'
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
    label: '浏览器取数',
    group: '采集',
    role: 'browser',
    icon: GlobeIcon,
    accent: 'bg-blue-500/12 text-blue-700 dark:text-blue-300',
    description: '打开当前设备浏览器页面，读取登录态下的后台数据。',
    inputs: ['网址', '任务描述'],
    outputs: ['页面数据', '截图证据'],
    settings: {
      网址: 'https://www.goofish.com/',
      登录态: '使用当前设备浏览器',
      权限: '只读采集',
      采集等待: '6s',
    },
  },
  request: {
    label: '网页请求',
    group: '采集',
    role: 'fast',
    icon: NetworkIcon,
    accent: 'bg-cyan-500/12 text-cyan-700 dark:text-cyan-300',
    description:
      '发起裸 HTTP 请求，获取接口或页面的结构化原始数据，不使用浏览器登录态。',
    inputs: ['任务描述'],
    outputs: ['JSON 数据', '请求日志'],
    settings: {
      请求URL: 'https://api.example.com/data',
      请求参数: '',
      方法: 'GET',
      超时: '30s',
      重试: '2 次',
    },
  },
  transform: {
    label: '整理数据',
    group: '智能处理',
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
    group: '智能处理',
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
    },
  },
  report: {
    label: '生成日报',
    group: '输出',
    role: 'reasoning',
    icon: FileTextIcon,
    accent: 'bg-amber-500/12 text-amber-700 dark:text-amber-300',
    description:
      '把表格、结论和图片整理成日报文档；格式、语气、标题等要求写在任务描述里。',
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
    group: '输出',
    role: 'fast',
    icon: BellIcon,
    accent: 'bg-rose-500/12 text-rose-700 dark:text-rose-300',
    description: '将日报发送到指定渠道，或提醒人工确认。',
    inputs: ['日报文档', '通知目标'],
    outputs: ['发送结果'],
    settings: {
      渠道: '邮件',
      确认: '发送前确认',
    },
  },
  webpage: {
    label: '输出网页',
    group: '输出',
    role: 'reasoning',
    icon: MonitorIcon,
    accent: 'bg-teal-500/12 text-teal-700 dark:text-teal-300',
    description:
      '将日报数据渲染成可交互的网页，支持自定义模板，后续只更新数据无需重新生成网页。',
    inputs: ['Markdown 日报', 'CSV 表格'],
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
    label: '停止',
    group: '触发',
    role: 'system',
    icon: StopCircleIcon,
    accent: 'bg-slate-500/12 text-slate-700 dark:text-slate-300',
    description: '可选控制节点，用于循环任务、中断流程或标记流程结束。',
    inputs: ['执行结果'],
    outputs: ['结束状态'],
    settings: {
      结束方式: '本轮完成后停止',
    },
  },
}

const PALETTE_GROUPS: Array<{
  title: '触发' | '采集' | '智能处理' | '输出'
  kinds: AgentNodeKind[]
}> = [
  { title: '触发', kinds: ['start', 'schedule', 'stop'] },
  { title: '采集', kinds: ['browser', 'request'] },
  { title: '智能处理', kinds: ['transform', 'image'] },
  { title: '输出', kinds: ['report', 'webpage', 'notify'] },
]

const INITIAL_EDGES: AgentFlowEdge[] = [
  { id: 'e-schedule-browser', source: 'schedule', target: 'browser' },
  { id: 'e-start-browser', source: 'start', target: 'browser' },
  { id: 'e-browser-transform', source: 'browser', target: 'transform' },
  { id: 'e-transform-image', source: 'transform', target: 'image' },
  { id: 'e-transform-report', source: 'transform', target: 'report' },
  { id: 'e-image-report', source: 'image', target: 'report' },
  { id: 'e-report-stop', source: 'report', target: 'stop' },
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
    () => buildTemplates(models, currentModel, currentGroup),
    [currentGroup, currentModel, models]
  )
  const initialBlueprints = useMemo(() => readSavedBlueprints(), [])
  const initialBlueprint = initialBlueprints[0] ?? null
  const templateByKind = useMemo(
    () => new Map(templates.map((template) => [template.kind, template])),
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
  const [lastSavedAt, setLastSavedAt] = useState<number | null>(
    initialBlueprint?.updatedAt ?? null
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
  const [selectedNodeId, setSelectedNodeId] = useState<string>(
    initialBlueprint?.nodes[0]?.id ?? 'browser'
  )
  const [search, setSearch] = useState('')
  const addCounterRef = useRef(0)
  const lastSeedPromptRef = useRef<string | null>(null)
  const previewRunIdRef = useRef(0)
  const previewTimerRefs = useRef<number[]>([])
  const undoStackRef = useRef<AgentBlueprintHistorySnapshot[]>([])
  const redoStackRef = useRef<AgentBlueprintHistorySnapshot[]>([])
  const [, setHistoryVersion] = useState(0)

  const selectedNode = useMemo(
    () => nodes.find((node) => node.id === selectedNodeId) ?? null,
    [nodes, selectedNodeId]
  )

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
  const canUndoBlueprint = undoStackRef.current.length > 0
  const canRedoBlueprint = redoStackRef.current.length > 0
  const clearPreviewTimers = useCallback(() => {
    previewTimerRefs.current.forEach((timer) => window.clearTimeout(timer))
    previewTimerRefs.current = []
  }, [])
  const cancelPreviewRun = useCallback(() => {
    previewRunIdRef.current += 1
    clearPreviewTimers()
  }, [clearPreviewTimers])
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
              }
            : blueprint
        )
        writeSavedBlueprints(nextBlueprints)
        notifyAgentBlueprintsChanged()
        return nextBlueprints
      })
    },
    [currentBlueprintId]
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
      recordHistorySnapshot()
      setEdges((current) =>
        addEdge(decorateEdge({ ...connection, id: `e-${Date.now()}` }), current)
      )
    },
    [recordHistorySnapshot]
  )

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
    setSelectedNodeId('')
  }, [cancelPreviewRun, recordHistorySnapshot])

  const handleNewBlueprint = useCallback(() => {
    cancelPreviewRun()
    undoStackRef.current = []
    redoStackRef.current = []
    bumpHistoryVersion()
    setBlueprintTitle('新的智能体蓝图')
    setCurrentBlueprintId(null)
    setLastSavedAt(null)
    setNodes(createInitialNodes(templates))
    setEdges(INITIAL_EDGES.map((edge) => decorateEdge(edge)))
    setExecutionLog({})
    setSelectedNodeId('browser')
  }, [bumpHistoryVersion, cancelPreviewRun, templates])

  const handleSaveBlueprint = useCallback(() => {
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
    }
    const next = [
      blueprint,
      ...blueprints.filter((item) => item.id !== id),
    ].sort((a, b) => b.updatedAt - a.updatedAt)

    setBlueprintTitle(title)
    setCurrentBlueprintId(id)
    setLastSavedAt(now)
    setBlueprints(next)
    writeSavedBlueprints(next)
    notifyAgentBlueprintsChanged()
  }, [
    blueprintTitle,
    blueprints,
    currentBlueprintStatus,
    currentBlueprintId,
    edges,
    executionLog,
    nodes,
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
      setLastSavedAt(blueprint.updatedAt)
      setNodes(normalizedNodes)
      setEdges(blueprint.edges)
      setExecutionLog(blueprint.executionLog ?? {})
      setSelectedNodeId(normalizedNodes[0]?.id ?? '')
    },
    [bumpHistoryVersion, cancelPreviewRun]
  )

  const handleDeleteBlueprint = useCallback(
    (blueprintId: string) => {
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
      setSelectedNodeId('browser')
    },
    [
      blueprints,
      cancelPreviewRun,
      currentBlueprintId,
      handleLoadBlueprint,
      templates,
    ]
  )

  const handleRunPreview = useCallback(() => {
    const prepared = ensureTerminalStopNode(nodes, edges, templateByKind)
    const runExecutionNodes = orderNodes(prepared.nodes, prepared.edges)
    if (runExecutionNodes.length === 0) return

    cancelPreviewRun()
    const runId = previewRunIdRef.current
    const timers: number[] = []

    if (prepared.changed) {
      setEdges(prepared.edges)
    }

    const commitPreviewStateForNodes = (
      activeNodes: AgentFlowNode[],
      statusById: Map<string, AgentNodeStatus>,
      finalizeTriggers = false
    ) => {
      const displayStatusById = finalizeTriggers
        ? finalizeTriggerNodeStatuses(activeNodes, statusById)
        : statusById
      const nextExecutionLog = buildPreviewExecutionLog(
        activeNodes,
        statusById,
        skillChoices,
        prepared.edges
      )

      setExecutionLog(nextExecutionLog)
      setNodes((current) => {
        const activeNodeById = new Map(
          activeNodes.map((node) => [node.id, node])
        )
        const currentById = new Map(current.map((node) => [node.id, node]))
        const nextNodes = current.map((node) => {
          const activeNode = activeNodeById.get(node.id)
          const status = displayStatusById.get(node.id)
          if (!activeNode && !status) return node
          return {
            ...(activeNode ?? node),
            data: {
              ...(activeNode?.data ?? node.data),
              status: status ?? activeNode?.data.status ?? node.data.status,
            },
          }
        })
        activeNodes.forEach((activeNode) => {
          if (currentById.has(activeNode.id)) return
          const status =
            displayStatusById.get(activeNode.id) ?? activeNode.data.status
          nextNodes.push({
            ...activeNode,
            data: { ...activeNode.data, status },
          })
        })
        persistCurrentBlueprintSnapshot(
          stripTransientNodeSettings(nextNodes),
          nextExecutionLog,
          finalizeTriggers ? undefined : 'running',
          prepared.edges
        )
        return nextNodes
      })
    }

    const appendReportOutputLogs = (
      outputsByNodeId: Record<
        string,
        { files: AgentReportWrittenFile[]; outputDirectory: string }
      >
    ) => {
      if (Object.keys(outputsByNodeId).length === 0) return
      setExecutionLog((current) => {
        const next = { ...current }
        for (const [nodeId, output] of Object.entries(outputsByNodeId)) {
          const log = next[nodeId]
          if (!log) continue
          next[nodeId] = {
            ...log,
            message: '日报文件已写入生成日报节点配置的输出目录。',
            outputSummary: [
              ...log.outputSummary.filter(
                (item) =>
                  !item.startsWith('实际文件:') && !item.startsWith('归档目录:')
              ),
              `归档目录: ${output.outputDirectory}`,
              ...output.files.map(
                (file) => `实际文件: ${file.label} -> ${file.path}`
              ),
            ],
            steps: [
              ...log.steps,
              ...output.files.map((file) => `写入 ${file.label}: ${file.path}`),
            ],
          }
        }
        setNodes((currentNodes) => {
          persistCurrentBlueprintSnapshot(
            stripTransientNodeSettings(currentNodes),
            next,
            undefined,
            prepared.edges
          )
          return currentNodes
        })
        return next
      })
    }

    const startTimedPreview = (capturedNodes: AgentFlowNode[]) => {
      capturedNodes.forEach((_, index) => {
        const timer = window.setTimeout(
          () => {
            if (previewRunIdRef.current !== runId) return

            const isFinalStep = index === capturedNodes.length - 1
            const statusById = buildPreviewStatusMap(capturedNodes, index)
            commitPreviewStateForNodes(capturedNodes, statusById, isFinalStep)

            if (isFinalStep) {
              previewTimerRefs.current = []
              const completedLog = buildPreviewExecutionLog(
                capturedNodes,
                statusById,
                skillChoices,
                prepared.edges
              )
              void writeCompletedReportOutputs(capturedNodes, completedLog)
                .then((outputsByNodeId) => {
                  if (previewRunIdRef.current !== runId) return
                  appendReportOutputLogs(outputsByNodeId)
                })
                .catch((error) => {
                  toast.error(
                    error instanceof Error ? error.message : '日报文件写入失败'
                  )
                })
            }
          },
          480 * (index + 1)
        )
        timers.push(timer)
      })
      previewTimerRefs.current = timers
    }

    commitPreviewStateForNodes(
      runExecutionNodes,
      buildPreviewStatusMap(runExecutionNodes, -1)
    )

    void captureBrowserNodes(runExecutionNodes)
      .then((capturedNodes) => {
        if (previewRunIdRef.current !== runId) return
        startTimedPreview(capturedNodes)
      })
      .catch((error) => {
        if (previewRunIdRef.current !== runId) return
        toast.error(
          error instanceof Error ? error.message : '浏览器自动采集失败'
        )
        startTimedPreview(markBrowserCaptureError(runExecutionNodes, error))
      })
  }, [
    cancelPreviewRun,
    edges,
    nodes,
    persistCurrentBlueprintSnapshot,
    skillChoices,
    templateByKind,
  ])

  const handleStopPreview = useCallback(() => {
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
  }, [cancelPreviewRun, executionLog, persistCurrentBlueprintSnapshot])

  const handleAddSkill = useCallback((skill: SkillChoice) => {
    setManagedSkills((current) => {
      const next = mergeSkillChoices(current, [skill]).filter(
        (item) => item.source !== 'built-in'
      )
      writeSavedSkills(next)
      return next
    })
  }, [])

  const handleDeleteSkill = useCallback((skillValue: string) => {
    setManagedSkills((current) => {
      const next = current.filter((skill) => skill.value !== skillValue)
      writeSavedSkills(next)
      return next
    })
  }, [])

  const handleAddNode = useCallback(
    (kind: AgentNodeKind, position?: { x: number; y: number }) => {
      const template = templateByKind.get(kind)
      if (!template) return
      recordHistorySnapshot()
      addCounterRef.current += 1
      const id = `${kind}-${Date.now()}-${addCounterRef.current}`
      const node = createNodeFromTemplate(template, id, {
        x: position?.x ?? 120 + addCounterRef.current * 36,
        y: position?.y ?? 120 + addCounterRef.current * 28,
      })
      setNodes((current) => [...current, node])
      setSelectedNodeId(id)
    },
    [recordHistorySnapshot, templateByKind]
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
        node.id === 'start'
          ? {
              ...node,
              data: {
                ...node.data,
                title: '从对话开启任务',
                description: seedPrompt,
              },
            }
          : node
      )
    )
    setEdges(INITIAL_EDGES.map((edge) => decorateEdge(edge)))
    setSelectedNodeId('start')
  }, [seedPrompt, templates])

  useEffect(() => {
    const handleCommand = (event: Event) => {
      const command = (event as CustomEvent<AgentBlueprintCommand>).detail
      if (!command) return

      if (command.type === 'new') {
        handleNewBlueprint()
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
        handleDeleteBlueprint(currentBlueprintId)
      }
    }

    window.addEventListener(AGENT_BLUEPRINT_COMMAND_EVENT, handleCommand)
    window.addEventListener(AGENT_BLUEPRINTS_CHANGED_EVENT, handleChanged)
    return () => {
      window.removeEventListener(AGENT_BLUEPRINT_COMMAND_EVENT, handleCommand)
      window.removeEventListener(AGENT_BLUEPRINTS_CHANGED_EVENT, handleChanged)
    }
  }, [
    currentBlueprintId,
    handleDeleteBlueprint,
    handleLoadBlueprint,
    handleNewBlueprint,
  ])

  return (
    <ReactFlowProvider>
      <BlueprintWorkspace
        blueprintTitle={blueprintTitle}
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
        onAddSkill={handleAddSkill}
        onAddNode={handleAddNode}
        onConnect={onConnect}
        onDeleteEdge={handleDeleteEdge}
        onDeleteNode={handleDeleteNode}
        onDeleteSkill={handleDeleteSkill}
        onEdgesChange={onEdgesChange}
        onNodesChange={onNodesChange}
        onRedo={handleRedoBlueprint}
        onReset={handleReset}
        onRunPreview={handleRunPreview}
        onStopPreview={handleStopPreview}
        onSaveBlueprint={handleSaveBlueprint}
        onSearchChange={setSearch}
        onSelectNode={setSelectedNodeId}
        onSetNodeSkill={handleSetNodeSkill}
        onTitleChange={handleTitleChange}
        onUndo={handleUndoBlueprint}
        onUpdateNode={handleUpdateNode}
        onUpdateSetting={handleUpdateSetting}
        search={search}
        selectedNode={selectedNode}
        hasScheduledBlueprint={hasScheduledBlueprint}
      />
    </ReactFlowProvider>
  )
}

type BlueprintWorkspaceProps = {
  nodes: AgentFlowNode[]
  edges: AgentFlowEdge[]
  selectedNode: AgentFlowNode | null
  executionNodes: AgentFlowNode[]
  executionLog: AgentExecutionLogMap
  modelChoices: ModelChoice[]
  groupChoices: GroupOption[]
  skillChoices: SkillChoice[]
  managedSkills: SkillChoice[]
  blueprintTitle: string
  canRedo: boolean
  canUndo: boolean
  currentBlueprintStatus: AgentBlueprintStatus
  currentGroup?: string
  hasScheduledBlueprint: boolean
  lastSavedAt: number | null
  search: string
  onSearchChange: (value: string) => void
  onNodesChange: (changes: NodeChange<AgentFlowNode>[]) => void
  onEdgesChange: (changes: EdgeChange<AgentFlowEdge>[]) => void
  onConnect: (connection: Connection) => void
  onSelectNode: (nodeId: string) => void
  onAddNode: (kind: AgentNodeKind, position?: { x: number; y: number }) => void
  onDeleteNode: (nodeId: string) => void
  onDeleteEdge: (edgeId: string) => void
  onTitleChange: (value: string) => void
  onSaveBlueprint: () => void
  onUpdateNode: (patch: Partial<AgentNodeData>) => void
  onUpdateSetting: (key: string, value: string) => void
  onAddSkill: (skill: SkillChoice) => void
  onDeleteSkill: (skillValue: string) => void
  onSetNodeSkill: (nodeId: string, skill: string) => void
  onRedo: () => void
  onReset: () => void
  onRunPreview: () => void
  onStopPreview: () => void
  onUndo: () => void
}

function BlueprintWorkspace({
  nodes,
  edges,
  selectedNode,
  executionNodes,
  executionLog,
  modelChoices,
  groupChoices,
  skillChoices,
  managedSkills,
  blueprintTitle,
  canRedo,
  canUndo,
  currentBlueprintStatus,
  currentGroup,
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
  onUpdateNode,
  onUpdateSetting,
  onAddSkill,
  onDeleteSkill,
  onSetNodeSkill,
  onRedo,
  onReset,
  onRunPreview,
  onStopPreview,
  onUndo,
}: BlueprintWorkspaceProps) {
  const reactFlow = useReactFlow<AgentFlowNode, AgentFlowEdge>()
  const filteredGroups = useMemo(() => filterPaletteGroups(search), [search])
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
      const kind = event.dataTransfer.getData(DRAG_NODE_TYPE) as AgentNodeKind
      if (!kind || !NODE_META[kind]) return
      const position = reactFlow.screenToFlowPosition({
        x: event.clientX,
        y: event.clientY,
      })
      onAddNode(kind, position)
    },
    [onAddNode, reactFlow]
  )

  return (
    <div className='bg-background flex size-full min-h-0 overflow-hidden'>
      <aside className='border-border/70 bg-muted/25 hidden w-68 shrink-0 flex-col border-r lg:flex'>
        <Palette
          activePanel={activeToolPanel}
          groups={filteredGroups}
          managedSkills={managedSkills}
          onAddNode={onAddNode}
          onAddSkill={onAddSkill}
          onDeleteSkill={onDeleteSkill}
          onPanelChange={setActiveToolPanel}
          onSearchChange={onSearchChange}
          search={search}
          skillChoices={skillChoices}
        />
      </aside>

      <div className='relative flex min-w-0 flex-1 flex-col'>
        <div className='border-border/70 flex min-h-0 shrink-0 items-center gap-2 border-b px-3 py-2 lg:hidden'>
          <CompactPalette groups={filteredGroups} onAddNode={onAddNode} />
        </div>

        <main
          className='relative min-h-0 min-w-0 flex-1'
          onDragOver={(event) => {
            event.preventDefault()
            event.dataTransfer.dropEffect = 'move'
          }}
          onDrop={handleDrop}
        >
          <div className='pointer-events-none absolute top-3 right-3 left-3 z-10 flex flex-wrap items-center justify-between gap-2'>
            <div className='bg-background/88 border-border/70 pointer-events-auto flex min-w-0 items-center gap-2 rounded-[8px] border px-2 py-1.5 shadow-sm backdrop-blur'>
              <div className='bg-primary text-primary-foreground flex size-7 shrink-0 items-center justify-center rounded-[6px]'>
                <ChartNetworkIcon className='size-4' />
              </div>
              <div className='min-w-0'>
                <Input
                  value={blueprintTitle}
                  onChange={(event) => onTitleChange(event.target.value)}
                  className='h-7 min-w-45 rounded-[6px] border-0 bg-transparent px-0 text-sm font-semibold shadow-none focus-visible:ring-0'
                />
                <div className='text-muted-foreground truncate text-[11px]'>
                  {nodes.length} 个组件 · {edges.length} 条连线
                  {' · '}
                  {getAgentBlueprintStatusLabel(currentBlueprintStatus)}
                  {lastSavedAt
                    ? ` · ${formatBlueprintTime(lastSavedAt)} 已保存`
                    : ''}
                </div>
              </div>
            </div>

            <div className='bg-background/88 border-border/70 pointer-events-auto flex items-center gap-1 rounded-[8px] border p-1 shadow-sm backdrop-blur'>
              <Button
                type='button'
                size='sm'
                variant='ghost'
                className='h-8 gap-1.5 rounded-[6px] px-2'
                onClick={onReset}
              >
                <RotateCcwIcon className='size-3.5' />
                重置
              </Button>
              <Button
                type='button'
                size='sm'
                variant='ghost'
                className='h-8 gap-1.5 rounded-[6px] px-2'
                disabled={!canUndo}
                onClick={onUndo}
              >
                <Undo2Icon className='size-3.5' />
                前一步
              </Button>
              <Button
                type='button'
                size='sm'
                variant='ghost'
                className='h-8 gap-1.5 rounded-[6px] px-2'
                disabled={!canRedo}
                onClick={onRedo}
              >
                <Redo2Icon className='size-3.5' />
                后一步
              </Button>
              <Button
                type='button'
                size='sm'
                variant='ghost'
                className='h-8 gap-1.5 rounded-[6px] px-2'
                onClick={onSaveBlueprint}
              >
                <SaveIcon className='size-3.5' />
                保存
              </Button>
              {currentBlueprintStatus === 'running' ? (
                <Button
                  type='button'
                  size='sm'
                  variant='destructive'
                  className='h-8 gap-1.5 rounded-[6px] px-2'
                  onClick={onStopPreview}
                >
                  {hasScheduledBlueprint ? (
                    <PauseIcon className='size-3.5' />
                  ) : (
                    <StopCircleIcon className='size-3.5' />
                  )}
                  {hasScheduledBlueprint ? '暂停' : '停止'}
                </Button>
              ) : (
                <Button
                  type='button'
                  size='sm'
                  className='h-8 gap-1.5 rounded-[6px] px-2'
                  onClick={onRunPreview}
                >
                  <ZapIcon className='size-3.5' />
                  {getRunPreviewButtonLabel(currentBlueprintStatus)}
                </Button>
              )}
            </div>
          </div>

          <ReactFlow<AgentFlowNode, AgentFlowEdge>
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
              executionLog={executionLog}
              executionNodes={executionNodes}
              isOpen={isPreviewOpen}
              selectedNode={selectedNode}
              skillChoices={skillChoices}
              onSelectNode={onSelectNode}
              onOpenChange={setIsPreviewOpen}
            />
          </div>
        </div>
      </div>

      <aside className='border-border/70 bg-background hidden w-[333px] shrink-0 flex-col border-l xl:flex'>
        <Inspector
          currentGroup={currentGroup}
          groupChoices={groupChoices}
          modelChoices={modelChoices}
          skillChoices={skillChoices}
          selectedNode={selectedNode}
          onDeleteNode={onDeleteNode}
          onUpdateNode={onUpdateNode}
          onUpdateSetting={onUpdateSetting}
        />
      </aside>
    </div>
  )
}

function Palette({
  groups,
  skillChoices,
  managedSkills,
  search,
  onSearchChange,
  onAddNode,
  onAddSkill,
  onDeleteSkill,
}: {
  activePanel: AgentToolPanel
  groups: typeof PALETTE_GROUPS
  skillChoices: SkillChoice[]
  managedSkills: SkillChoice[]
  search: string
  onSearchChange: (value: string) => void
  onAddNode: (kind: AgentNodeKind) => void
  onAddSkill: (skill: SkillChoice) => void
  onDeleteSkill: (skillValue: string) => void
  onPanelChange: (panel: AgentToolPanel) => void
}) {
  return (
    <div className='flex min-h-0 flex-1 flex-col'>
      <Tabs defaultValue='components' className='flex min-h-0 flex-1 flex-col'>
        {/* 顶部：标题 + Tab 导航 */}
        <div className='border-border/70 shrink-0 border-b px-3 pt-3'>
          <div className='mb-2.5 flex items-center gap-2'>
            <div className='bg-primary text-primary-foreground flex size-7 shrink-0 items-center justify-center rounded-[7px]'>
              <BotIcon className='size-3.5' />
            </div>
            <span className='text-sm font-semibold'>工具库</span>
          </div>

          <TabsList className='h-7 w-full gap-0 rounded-none border-0 bg-transparent p-0'>
            <TabsTrigger
              value='components'
              className='data-[state=active]:border-primary h-7 flex-1 rounded-none border-b-2 border-transparent bg-transparent px-2 text-xs font-medium shadow-none data-[state=active]:bg-transparent data-[state=active]:shadow-none'
            >
              组件
            </TabsTrigger>
            <TabsTrigger
              value='skills'
              className='data-[state=active]:border-primary h-7 flex-1 rounded-none border-b-2 border-transparent bg-transparent px-2 text-xs font-medium shadow-none data-[state=active]:bg-transparent data-[state=active]:shadow-none'
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
                placeholder='搜索组件…'
                className='h-7 rounded-[7px] pl-7 text-xs'
              />
            </div>
          </div>

          {/* 组件列表（可滚动） */}
          <div className='min-h-0 flex-1 overflow-y-auto px-2 pb-3'>
            {groups.map((group) => (
              <div key={group.title} className='mb-1'>
                <div className='text-muted-foreground px-1 pt-0.5 pb-1 text-[10px] font-semibold tracking-widest uppercase'>
                  {group.title}
                </div>
                <div className='grid gap-0.5'>
                  {group.kinds.map((kind) => (
                    <PaletteItem key={kind} kind={kind} onAddNode={onAddNode} />
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
              managedSkills={managedSkills}
              skillChoices={skillChoices}
              onAddSkill={onAddSkill}
              onDeleteSkill={onDeleteSkill}
            />
          </div>
        </TabsContent>
      </Tabs>
    </div>
  )
}

function CompactPalette({
  groups,
  onAddNode,
}: {
  groups: typeof PALETTE_GROUPS
  onAddNode: (kind: AgentNodeKind) => void
}) {
  const kinds = groups.flatMap((group) => group.kinds)
  return (
    <div className='no-scrollbar flex min-w-0 gap-1 overflow-x-auto'>
      {kinds.map((kind) => {
        const meta = NODE_META[kind]
        const Icon = meta.icon
        return (
          <button
            key={kind}
            type='button'
            className='border-border bg-background flex h-8 shrink-0 items-center gap-1.5 rounded-[8px] border px-2 text-xs'
            draggable
            onClick={() => onAddNode(kind)}
            onDragStart={(event) => {
              event.dataTransfer.setData(DRAG_NODE_TYPE, kind)
              event.dataTransfer.effectAllowed = 'move'
            }}
          >
            <Icon className='size-3.5' />
            {meta.label}
          </button>
        )
      })}
    </div>
  )
}

function PaletteItem({
  kind,
  onAddNode,
}: {
  kind: AgentNodeKind
  onAddNode: (kind: AgentNodeKind) => void
}) {
  const meta = NODE_META[kind]
  const Icon = meta.icon

  return (
    <button
      type='button'
      className='hover:bg-accent/60 group flex w-full items-start gap-2.5 rounded-[7px] px-2 py-2 text-left transition-colors'
      draggable
      onClick={() => onAddNode(kind)}
      onDragStart={(event) => {
        event.dataTransfer.setData(DRAG_NODE_TYPE, kind)
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
        <span className='block truncate text-xs font-medium'>{meta.label}</span>
        <span className='text-muted-foreground line-clamp-2 block text-[10px] leading-snug break-words whitespace-normal'>
          {meta.description}
        </span>
      </span>
      <PlusIcon className='text-muted-foreground mt-2 size-3 shrink-0 opacity-0 transition-opacity group-hover:opacity-100' />
    </button>
  )
}

function DeletableEdge({
  id,
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
        <button
          type='button'
          className={cn(
            'border-border/70 bg-background text-muted-foreground hover:border-destructive hover:text-destructive absolute flex size-5 items-center justify-center rounded-full border shadow-sm transition-opacity',
            showButton ? 'opacity-100' : 'pointer-events-none opacity-0'
          )}
          style={{
            transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
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
  const meta = NODE_META[data.kind]
  const Icon = meta.icon
  const canReceive = data.inputs.length > 0
  const canSend = data.outputs.length > 0

  return (
    <div
      className={cn(
        'bg-background border-border/80 relative w-72 rounded-[8px] border shadow-sm transition-shadow',
        selected && 'ring-primary/40 ring-3'
      )}
    >
      {canReceive && (
        <Handle
          className='!border-background !bg-primary !size-3'
          isConnectable={isConnectable}
          position={Position.Left}
          type='target'
        />
      )}
      {canSend && (
        <Handle
          className='!border-background !bg-primary !size-3'
          isConnectable={isConnectable}
          position={Position.Right}
          type='source'
        />
      )}
      <Handle
        className='!border-background !size-3 !bg-fuchsia-500'
        id='skill'
        isConnectable={isConnectable}
        position={Position.Bottom}
        type='source'
      />

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

function StatusDot({ status }: { status: AgentNodeStatus }) {
  const labelMap: Record<AgentNodeStatus, string> = {
    ready: '就绪',
    waiting: '等待',
    running: '运行',
    done: '完成',
  }
  return (
    <span
      className={cn(
        'inline-flex h-5 shrink-0 items-center gap-1 rounded-full px-1.5 text-[10px]',
        status === 'ready' && 'bg-muted text-muted-foreground',
        status === 'waiting' && 'bg-amber-500/12 text-amber-700',
        status === 'running' && 'bg-blue-500/12 text-blue-700',
        status === 'done' && 'bg-emerald-500/12 text-emerald-700'
      )}
    >
      <span
        className={cn(
          'size-1.5 rounded-full',
          status === 'ready' && 'bg-muted-foreground',
          status === 'waiting' && 'bg-amber-500',
          status === 'running' && 'bg-blue-500',
          status === 'done' && 'bg-emerald-500'
        )}
      />
      {labelMap[status]}
    </span>
  )
}

function SkillLinkBadge({
  skill,
  skillChoices,
  onSelectSkill,
}: {
  skill: string
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
        {skillChoices.map((choice) => (
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
  modelChoices,
  groupChoices,
  skillChoices,
  currentGroup,
  onDeleteNode,
  onUpdateNode,
  onUpdateSetting,
}: {
  selectedNode: AgentFlowNode | null
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

      <div className='min-h-0 flex-1 overflow-y-auto'>
        {selectedNode ? (
          <div className='space-y-4 p-3'>
            <NodeConfigPanel
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
            选中画布上的组件后配置执行模型。
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
        const meta = NODE_META[node.data.kind]
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
                    logStatus === 'stopped' && 'bg-slate-500/12 text-slate-700'
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
  }
  return labels[status]
}

function statusToLogStatus(status: AgentNodeStatus): AgentNodeLogStatus {
  if (status === 'running') return 'running'
  if (status === 'done') return 'completed'
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

function parseBrowserCaptureWaitSeconds(value: string | undefined) {
  const parsed = Number.parseInt(value || '', 10)
  if (!Number.isFinite(parsed)) return 6
  return Math.min(30, Math.max(1, parsed))
}

function browserCapturedText(node: AgentFlowNode) {
  return (
    node.data.settings[BROWSER_CAPTURE_TEXT_SETTING] ||
    node.data.settings[BROWSER_PAGE_DATA_SETTING] ||
    ''
  ).trim()
}

function browserCaptureError(node: AgentFlowNode) {
  return node.data.settings['采集错误']?.trim() || ''
}

function updateNodeRuntimeSettings(
  nodes: AgentFlowNode[],
  nodeId: string,
  patch: Record<string, string>
) {
  return nodes.map((node) =>
    node.id === nodeId
      ? {
          ...node,
          data: {
            ...node.data,
            settings: { ...node.data.settings, ...patch },
          },
        }
      : node
  )
}

async function captureBrowserNodes(nodes: AgentFlowNode[]) {
  let nextNodes = nodes

  for (const node of nodes) {
    if (node.data.kind !== 'browser') continue

    const url = node.data.settings['网址']?.trim()
    if (!url) {
      nextNodes = updateNodeRuntimeSettings(nextNodes, node.id, {
        采集错误: '浏览器取数节点未配置网址。',
      })
      continue
    }

    try {
      const result = await captureAgentBrowserPage({
        url,
        login_state: node.data.settings['登录态'] || '',
        wait_seconds: parseBrowserCaptureWaitSeconds(
          node.data.settings['采集等待']
        ),
      })
      const text = result.text.trim()
      nextNodes = updateNodeRuntimeSettings(nextNodes, node.id, {
        [BROWSER_CAPTURE_TEXT_SETTING]: text,
        [BROWSER_PAGE_DATA_SETTING]: text,
        采集标题: result.title,
        采集网址: result.final_url || result.url,
        采集时间: String(result.captured_at),
        采集错误: text ? '' : '浏览器已打开页面，但未返回可解析的页面文本。',
      })
    } catch (error) {
      nextNodes = updateNodeRuntimeSettings(nextNodes, node.id, {
        [BROWSER_CAPTURE_TEXT_SETTING]: '',
        [BROWSER_PAGE_DATA_SETTING]: '',
        采集错误:
          error instanceof Error ? error.message : '浏览器自动采集失败。',
      })
    }
  }

  return nextNodes
}

function markBrowserCaptureError(nodes: AgentFlowNode[], error: unknown) {
  const message =
    error instanceof Error ? error.message : '浏览器自动采集失败。'
  return nodes.map((node) =>
    node.data.kind === 'browser'
      ? updateNodeRuntimeSettings([node], node.id, {
          [BROWSER_CAPTURE_TEXT_SETTING]: '',
          [BROWSER_PAGE_DATA_SETTING]: '',
          采集错误: message,
        })[0]
      : node
  )
}

function stripTransientNodeSettings(nodes: AgentFlowNode[]) {
  return nodes.map((node) => {
    if (node.data.kind !== 'browser') return node

    const settings = { ...node.data.settings }
    for (const key of TRANSIENT_BROWSER_SETTING_LABELS) {
      delete settings[key]
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

function finalizeTriggerNodeStatuses(
  executionNodes: AgentFlowNode[],
  statusById: Map<string, AgentNodeStatus>
): Map<string, AgentNodeStatus> {
  const next = new Map(statusById)
  executionNodes.forEach((node) => {
    if (statusById.get(node.id) !== 'done') return
    if (node.data.kind === 'schedule') {
      next.set(node.id, 'waiting')
    }
  })
  return next
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

async function writeCompletedReportOutputs(
  executionNodes: AgentFlowNode[],
  executionLog: AgentExecutionLogMap
) {
  const results: Record<
    string,
    { files: AgentReportWrittenFile[]; outputDirectory: string }
  > = {}

  for (const node of executionNodes) {
    if (node.data.kind !== 'report') continue
    const log = executionLog[node.id]
    if (!log || log.status !== 'completed') continue
    const files = buildReportArtifactFiles(node, log)
    if (files.length === 0) continue

    const outputDir =
      node.data.settings['输出目录'] || DEFAULT_OUTPUT_DIR_REPORT
    const result = await writeAgentReport({
      title: node.data.title,
      output_directory: outputDir,
      files,
    })
    results[node.id] = {
      files: result.files,
      outputDirectory: result.output_directory,
    }
  }

  return results
}

function buildReportArtifactFiles(
  node: AgentFlowNode,
  log: AgentNodeExecutionLog
): AgentReportOutputFile[] {
  const templateFiles = buildReportArtifactFilesFromTemplates(node, log)
  if (templateFiles.length > 0) return templateFiles

  return reportFileOutputLabels(node).map((label) => {
    const extension = reportExtensionForLabel(label)
    const template = findReportTemplateForExtension(node, extension)
    return {
      label,
      extension,
      content: buildReportArtifactContent(label, extension, node, log),
      template_path: template?.path,
      template_original_name: template?.originalName,
    }
  })
}

function buildReportArtifactFilesFromTemplates(
  node: AgentFlowNode,
  log: AgentNodeExecutionLog
): AgentReportOutputFile[] {
  const bindings = parseTemplateBindings(
    node.data.settings['模板文件'] || '',
    node.data.settings
  )
  return bindings
    .map<AgentReportOutputFile | null>((binding) => {
      const label = reportOutputFormatForTemplatePath(binding.path)
      if (!label) return null
      const extension = reportExtensionForLabel(label)
      return {
        label,
        extension,
        content: buildReportArtifactContent(label, extension, node, log),
        template_path: binding.path,
        template_original_name: binding.originalName,
      }
    })
    .filter((file): file is AgentReportOutputFile => Boolean(file))
}

function buildReportArtifactContent(
  label: string,
  extension: string,
  node: AgentFlowNode,
  log: AgentNodeExecutionLog
) {
  if (extension === 'csv') return buildReportCsvContent(label, node, log)
  if (extension === 'json') return buildReportJsonContent(label, node, log)
  if (extension === 'html') return buildReportHtmlContent(label, node, log)
  if (extension === 'txt') return log.modelOutput
  return log.modelOutput
}

function buildReportCsvContent(
  label: string,
  node: AgentFlowNode,
  log: AgentNodeExecutionLog
) {
  const rows = [
    ['类型', '内容'],
    ['节点', node.data.title],
    ['输出格式', label],
    ['节点描述', node.data.description],
    ...log.inputSummary.map((item) => ['输入', item]),
    ['生成内容', log.modelOutput],
  ]
  return rows.map((row) => row.map(csvCell).join(',')).join('\n')
}

function buildReportJsonContent(
  label: string,
  node: AgentFlowNode,
  log: AgentNodeExecutionLog
) {
  return JSON.stringify(
    {
      node: node.data.title,
      output_format: label,
      description: node.data.description,
      input_summary: log.inputSummary,
      output_summary: log.outputSummary,
      content: log.modelOutput,
    },
    null,
    2
  )
}

function buildReportHtmlContent(
  label: string,
  node: AgentFlowNode,
  log: AgentNodeExecutionLog
) {
  return [
    '<!doctype html>',
    '<html lang="zh-CN">',
    '<head>',
    '<meta charset="utf-8" />',
    `<title>${escapeHtml(node.data.title)}</title>`,
    '</head>',
    '<body>',
    `<h1>${escapeHtml(node.data.title)}</h1>`,
    `<p><strong>输出格式:</strong> ${escapeHtml(label)}</p>`,
    `<p>${escapeHtml(node.data.description)}</p>`,
    `<pre>${escapeHtml(log.modelOutput)}</pre>`,
    '</body>',
    '</html>',
  ].join('\n')
}

function csvCell(value: string) {
  return `"${value.replace(/"/g, '""')}"`
}

function escapeHtml(value: string) {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function estimateNodeDuration(node: AgentFlowNode, index: number) {
  const baseByKind: Record<AgentNodeKind, number> = {
    start: 220,
    schedule: 260,
    browser: 1800,
    request: 1200,
    transform: 980,
    image: 2600,
    report: 1600,
    webpage: 1400,
    notify: 720,
    stop: 240,
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
    const metrics = parseXianyuOverviewMetrics(browserCapturedText(node))
    const error = browserCaptureError(node)
    return metrics.length > 0
      ? [
          `页面数据: 已解析 ${metrics.length} 个闲鱼数据总览指标`,
          '截图证据: 当前版本先采集 DOM 文本，暂未写入截图文件',
        ]
      : [
          error
            ? `页面数据: 自动采集失败，${error}`
            : '页面数据: 未获取可解析的真实页面数据',
          '截图证据: 当前版本先采集 DOM 文本，暂未写入截图文件',
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
    completed: '已生成预览样本',
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
      return [
        `模型: ${node.data.model}${skillLine}`,
        '输出摘要:',
        '- 已完成接口请求与响应解析。',
        '- 已过滤空字段和重复记录。',
        '- 原始响应已转换为结构化 JSON 数据。',
      ].join('\n')
    case 'transform': {
      const templateName = node.data.settings['整理模板']
      const templateOriginalName = node.data.settings['模板原文件名']
      const metrics = extractXianyuMetricsFromLogs(upstreamLogs)
      if (metrics.length > 0) {
        return [
          `模型: ${node.data.model}${skillLine}`,
          '整理结果: 仅使用上一节点提供的闲鱼数据总览。',
          '',
          formatXianyuMetricsMarkdown(metrics),
          '',
          '限制: 上游未提供商品排行明细，后续节点不得生成商品ID、商品标题或TOP排行。',
          templateName && templateName !== '未上传模板'
            ? `已按当前模板文件「${templateOriginalName || templateName}」整理字段。`
            : '未上传模板，已按上一节点数据结构自动归并。',
        ].join('\n')
      }
      if (hasMissingBrowserData(upstreamLogs)) {
        return buildBlockedByMissingBrowserDataOutput(node)
      }
      return [
        `模型: ${node.data.model}${skillLine}`,
        '整理结果:',
        '| 字段 | 值 | 说明 |',
        '| --- | --- | --- |',
        '| 指标 A | 示例值 1 | 来自上一节点数据 |',
        '| 指标 B | 示例值 2 | 按当前节点描述整理 |',
        '',
        templateName && templateName !== '未上传模板'
          ? `已按当前模板文件「${templateOriginalName || templateName}」整理字段。`
          : '未上传模板，已按上一节点数据结构自动归并。',
      ].join('\n')
    }
    case 'image':
      return [
        `模型: ${node.data.model}${skillLine}`,
        '图片生成结果:',
        '- 已根据上一节点输出和当前节点描述生成图片提示。',
        '- 图片 URL: /api/playground/image-history/files/demo-dashboard.png',
        '- 图片说明: 预览图内容由当前节点描述决定。',
      ].join('\n')
    case 'report': {
      const outputDir =
        node.data.settings['输出目录'] || DEFAULT_OUTPUT_DIR_REPORT
      const outputFormats = reportFileOutputLabels(node)
      const previewPaths = reportPreviewPaths(node)
      const templateBindings = parseTemplateBindings(
        node.data.settings['模板文件'] || '',
        node.data.settings
      )
      const metrics = extractXianyuMetricsFromLogs(upstreamLogs)
      if (metrics.length > 0) {
        return [
          `模型: ${node.data.model}${skillLine}`,
          '# 日报预览',
          '',
          '## 数据来源',
          '- 仅使用上一节点提供的闲鱼数据总览和当前节点描述。',
          '- 未使用标题、模板旧内容或其他隐含上下文补全数据。',
          '',
          '## 数据总览',
          formatXianyuMetricsMarkdown(metrics),
          '',
          '## 结论',
          ...buildXianyuMetricInsightLines(metrics),
          '',
          '## 后续处理',
          `1. 按当前节点输出格式生成: ${outputFormats.join('、')}。`,
          '2. 上游未提供商品排行明细，不生成商品ID、商品标题或TOP排行。',
          '3. 正式执行时输出到当前节点配置的归档目录。',
          '',
          `预览输出目录: ${outputDir}`,
          ...previewPaths.map((path) => `预览文件: ${path}`),
        ].join('\n')
      }
      if (hasMissingBrowserData(upstreamLogs)) {
        return buildBlockedByMissingBrowserDataOutput(node)
      }
      return [
        `模型: ${node.data.model}${skillLine}`,
        '# 日报预览',
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
        '- 已生成交互式日报页面。',
        templateBindings.length > 0
          ? `- 按当前网页模板「${templateBindingNames(templateBindings)}」和节点描述渲染页面。`
          : '- 未绑定网页模板，按当前节点描述和上一节点输出渲染页面。',
        `- 输出目录: ${node.data.settings['输出目录'] || DEFAULT_OUTPUT_DIR_WEB}`,
        '- 页面仅使用上一节点输出和当前节点描述中的渲染要求。',
      ].join('\n')
    }
    case 'notify':
      return [
        `模型: ${node.data.model}${skillLine}`,
        '通知结果:',
        '- 已生成通知摘要。',
        `- 渠道: ${node.data.settings['渠道'] || '邮件'}`,
        `- 确认策略: ${node.data.settings['确认'] || '发送前确认'}`,
      ].join('\n')
    case 'stop':
      return [
        '本地执行结果: 流程结束节点已执行。',
        `结束方式: ${node.data.settings['结束方式'] || '本轮完成后停止'}`,
        '产物目录: 以上游输出节点配置为准。',
      ].join('\n')
    default:
      return `模型: ${node.data.model}${skillLine}\n节点已完成，输出将传给下一节点。`
  }
}

function buildBrowserNodeModelOutput(node: AgentFlowNode, skillLine: string) {
  const capturedText = browserCapturedText(node)
  const captureError = browserCaptureError(node)
  const metrics = parseXianyuOverviewMetrics(capturedText)

  if (metrics.length > 0) {
    return [
      `模型: ${node.data.model}${skillLine}`,
      '数据来源: 浏览器节点自动采集的真实页面文本。',
      `采集网址: ${node.data.settings['采集网址'] || node.data.settings['网址'] || '未记录'}`,
      `${XIANYU_OVERVIEW_METRICS_MARKER}:`,
      JSON.stringify(metrics, null, 2),
      '',
      '数据总览:',
      formatXianyuMetricsMarkdown(metrics),
      '',
      '限制: 当前数据仅包含数据总览；未获取商品排行明细。下游不得生成商品ID、商品标题或TOP排行。',
    ].join('\n')
  }

  return [
    `模型: ${node.data.model}${skillLine}`,
    `${BROWSER_DATA_MISSING_MARKER}: true`,
    '未拿到真实浏览器页面数据。',
    `原因: ${captureError || '浏览器自动采集未返回可解析的页面文本。'}`,
    `目标网址: ${node.data.settings['网址'] || '未配置'}`,
    '处理: 请确认目标页面已登录并能在浏览器中正常打开，然后重新执行。',
    '限制: 不会根据模板、标题、历史内容或模型常识补全商品排行、商品ID、商品标题。',
  ].join('\n')
}

function buildBlockedByMissingBrowserDataOutput(node: AgentFlowNode) {
  return [
    `模型: ${node.data.model}`,
    `${BROWSER_DATA_MISSING_MARKER}: true`,
    '无法生成有效输出: 上游浏览器取数节点没有提供真实页面数据。',
    '请先让浏览器取数节点自动采集到可解析的真实页面数据。',
    '为避免错误数据，本节点不会根据模板、标题或隐含上下文生成商品排行、商品ID、商品标题。',
  ].join('\n')
}

function extractXianyuMetricsFromLogs(logs: AgentNodeExecutionLog[]) {
  for (const log of [...logs].reverse()) {
    if (log.status !== 'completed') continue
    const metrics = parseXianyuOverviewMetrics(log.modelOutput)
    if (metrics.length > 0) return metrics
  }
  return []
}

function hasMissingBrowserData(logs: AgentNodeExecutionLog[]) {
  return logs.some(
    (log) =>
      log.status === 'completed' &&
      log.modelOutput.includes(BROWSER_DATA_MISSING_MARKER)
  )
}

function parseXianyuOverviewMetrics(value: string): XianyuOverviewMetric[] {
  const raw = value.trim()
  if (!raw) return []

  const fromJson = parseXianyuMetricJson(raw)
  if (fromJson.length > 0) return fromJson

  const metrics: XianyuOverviewMetric[] = []
  const compact = raw
    .replace(/\u00a0/g, ' ')
    .replace(/[：:]/g, ' ')
    .replace(/\s+/g, ' ')

  for (const label of XIANYU_OVERVIEW_METRIC_LABELS) {
    const tableMetric = parseMetricFromMarkdownTable(raw, label)
    if (tableMetric) {
      metrics.push(tableMetric)
      continue
    }

    const index = compact.indexOf(label)
    if (index < 0) continue
    const segment = compact.slice(
      index + label.length,
      index + label.length + 120
    )
    const current = segment.match(
      /\s*([¥￥]?\s*(?:-|\d+(?:,\d{3})*(?:\.\d+)?|\d+(?:\.\d+)?))/
    )?.[1]
    if (!current) continue
    const previous = segment.match(
      /前\s*1\s*日\s*([¥￥]?\s*(?:-|\d+(?:,\d{3})*(?:\.\d+)?|\d+(?:\.\d+)?))/
    )?.[1]
    const change = segment.match(/([↑↓]\s*\d+(?:\.\d+)?%|[-－])/)?.[1]
    metrics.push({
      label,
      current: normalizeMetricValue(current),
      previous: normalizeMetricValue(previous || '-'),
      change: normalizeMetricValue(change || '-'),
    })
  }

  return dedupeXianyuMetrics(metrics)
}

function parseXianyuMetricJson(value: string) {
  const marker = `${XIANYU_OVERVIEW_METRICS_MARKER}:`
  const markerIndex = value.indexOf(marker)
  const source =
    markerIndex >= 0 ? value.slice(markerIndex + marker.length) : value
  const jsonText = source.match(/\[[\s\S]*?\]/)?.[0]
  if (!jsonText) return []

  try {
    const parsed: unknown = JSON.parse(jsonText)
    if (!Array.isArray(parsed)) return []
    return dedupeXianyuMetrics(
      parsed
        .map((item) => {
          if (!item || typeof item !== 'object') return null
          const candidate = item as Partial<XianyuOverviewMetric>
          if (typeof candidate.label !== 'string') return null
          if (!XIANYU_OVERVIEW_METRIC_LABELS.includes(candidate.label)) {
            return null
          }
          return {
            label: candidate.label,
            current: normalizeMetricValue(candidate.current || '-'),
            previous: normalizeMetricValue(candidate.previous || '-'),
            change: normalizeMetricValue(candidate.change || '-'),
          }
        })
        .filter((item): item is XianyuOverviewMetric => Boolean(item))
    )
  } catch {
    return []
  }
}

function parseMetricFromMarkdownTable(value: string, label: string) {
  const regex = new RegExp(
    `\\|\\s*${escapeRegExp(label)}\\s*\\|\\s*([^|]+)\\|\\s*([^|]+)\\|\\s*([^|]+)\\|`,
    'm'
  )
  const match = value.match(regex)
  if (!match) return null
  return {
    label,
    current: normalizeMetricValue(match[1]),
    previous: normalizeMetricValue(match[2]),
    change: normalizeMetricValue(match[3]),
  }
}

function dedupeXianyuMetrics(metrics: XianyuOverviewMetric[]) {
  const byLabel = new Map<string, XianyuOverviewMetric>()
  for (const metric of metrics) {
    if (!byLabel.has(metric.label)) {
      byLabel.set(metric.label, metric)
    }
  }
  return XIANYU_OVERVIEW_METRIC_LABELS.map((label) =>
    byLabel.get(label)
  ).filter((metric): metric is XianyuOverviewMetric => Boolean(metric))
}

function normalizeMetricValue(value: string) {
  return value
    .replace(/[￥¥]\s*/g, '¥')
    .replace(/\s+/g, '')
    .replace(/^－$/, '-')
    .trim()
}

function formatXianyuMetricsMarkdown(metrics: XianyuOverviewMetric[]) {
  return [
    '| 指标 | 当前值 | 前1日 | 变化 |',
    '| --- | ---: | ---: | ---: |',
    ...metrics.map(
      (metric) =>
        `| ${metric.label} | ${metric.current} | ${metric.previous} | ${metric.change} |`
    ),
  ].join('\n')
}

function buildXianyuMetricInsightLines(metrics: XianyuOverviewMetric[]) {
  const byLabel = new Map(metrics.map((metric) => [metric.label, metric]))
  const focusLabels = [
    '商品访问次数',
    '商品曝光次数',
    '商品浏览次数',
    '支付笔数',
    '支付金额（元）',
  ]
  const lines = focusLabels
    .map((label) => byLabel.get(label))
    .filter((metric): metric is XianyuOverviewMetric => Boolean(metric))
    .map(
      (metric) =>
        `- ${metric.label}: 当前 ${metric.current}，前1日 ${metric.previous}，变化 ${metric.change}。`
    )
  return lines.length > 0 ? lines : ['- 已按上游结构化指标生成摘要。']
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
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
    const metrics = parseXianyuOverviewMetrics(browserCapturedText(node))
    const error = browserCaptureError(node)
    steps.push(
      metrics.length > 0
        ? `从自动采集的页面文本解析 ${metrics.length} 个闲鱼数据总览指标。`
        : error
          ? `浏览器自动采集失败: ${error}`
          : '等待 Edge/浏览器 DOM 采集器返回真实页面文本。'
    )
  }

  if (node.data.kind === 'transform') {
    const templateName = node.data.settings['整理模板']
    const templateOriginalName = node.data.settings['模板原文件名']
    const dataType = node.data.settings['数据类型'] || '自动识别'
    const metrics = extractXianyuMetricsFromLogs(upstreamLogs)
    steps.push(
      metrics.length > 0
        ? `使用上游 ${metrics.length} 个真实指标整理，不扩展隐含上下文。`
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
    const metrics = extractXianyuMetricsFromLogs(upstreamLogs)
    steps.push(
      templateBindings.length > 0
        ? `读取当前模板文件: ${templateBindingNames(templateBindings)}`
        : '未绑定模板文件，按当前节点描述生成日报。'
    )
    if (metrics.length > 0) {
      steps.push(
        `使用上游 ${metrics.length} 个真实指标生成，不读取模板旧数据。`
      )
    } else if (hasMissingBrowserData(upstreamLogs)) {
      steps.push('上游真实页面数据缺失，本节点停止生成业务内容。')
    }
    steps.push(`输出格式: ${outputFormats.join('、')}`)
    steps.push(`生成日报预览；正式执行时写入输出目录: ${outputDir}`)
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
      return `${node.data.title} 已生成日报预览；正式执行时会写入输出目录。`
    }
    if (node.data.kind === 'stop') {
      return `${node.data.title} 已完成流程结束动作，输出目录以上游节点配置为准。`
    }
    return `${node.data.title} 已完成，输出已进入后续节点上下文。`
  }
  if (status === 'running') {
    return `${node.data.title} 正在执行，实时结果会在完成后写入输出摘要。`
  }
  if (status === 'stopped') {
    return `${node.data.title} 已停止，保留已收集到的执行上下文。`
  }
  return `${node.data.title} 正在等待上游节点完成。`
}

function NodeConfigPanel({
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
  return (
    <div className='space-y-3'>
      <ConfigCard title='基础信息'>
        <NodeCommonConfig
          currentGroup={currentGroup}
          groupChoices={groupChoices}
          modelChoices={modelChoices}
          node={node}
          skillChoices={skillChoices}
          onDeleteNode={onDeleteNode}
          onUpdateNode={onUpdateNode}
        />
      </ConfigCard>

      <ConfigCard title='描述信息'>
        <Textarea
          value={node.data.description}
          onChange={(event) =>
            onUpdateNode({ description: event.target.value })
          }
          className='min-h-22 rounded-[8px] px-2.5 py-2 text-sm leading-relaxed'
        />
      </ConfigCard>

      <ConfigCard title='参数'>
        {Object.keys(node.data.settings).length === 0 ? (
          <div className='text-muted-foreground text-xs'>无参数</div>
        ) : (
          <div className='space-y-2'>
            {Object.entries(node.data.settings).map(([key, value]) => (
              <SettingControl
                key={key}
                label={key}
                value={value}
                nodeKind={node.data.kind}
                settings={node.data.settings}
                onChange={(nextValue) => onUpdateSetting(key, nextValue)}
                onChangeSetting={onUpdateSetting}
              />
            ))}
          </div>
        )}
      </ConfigCard>

      <div className='grid grid-cols-2 gap-2'>
        <ConfigCard title='输入'>
          <FieldList values={node.data.inputs} />
        </ConfigCard>
        <ConfigCard title='输出'>
          <FieldList values={node.data.outputs} />
        </ConfigCard>
      </div>
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
}: {
  node: AgentFlowNode
  modelChoices: ModelChoice[]
  groupChoices: GroupOption[]
  skillChoices: SkillChoice[]
  currentGroup?: string
  onDeleteNode: (nodeId: string) => void
  onUpdateNode: (patch: Partial<AgentNodeData>) => void
}) {
  const meta = NODE_META[node.data.kind]
  const Icon = meta.icon
  const modelExists = modelChoices.some(
    (model) => model.value === node.data.model
  )
  const choices = modelExists
    ? modelChoices
    : [
        {
          label: node.data.model,
          value: node.data.model,
          source: 'current' as const,
        },
        ...modelChoices,
      ]
  const selectedSkill = getSkillChoice(node.data.skill, skillChoices)
  const selectedGroup =
    node.data.group || currentGroup || groupChoices[0]?.value || 'default'

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
            {node.id}
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

      <div className='grid grid-cols-2 gap-2'>
        <CompactField label='名称'>
          <Input
            value={node.data.title}
            onChange={(event) => onUpdateNode({ title: event.target.value })}
            className='h-7 rounded-[7px] px-2 text-xs'
          />
        </CompactField>

        <CompactField label='模型组'>
          <NativeSelect
            size='sm'
            value={selectedGroup}
            onChange={(event) => onUpdateNode({ group: event.target.value })}
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
            value={node.data.model}
            onChange={(event) => onUpdateNode({ model: event.target.value })}
            className='w-full'
          >
            <NativeSelectOptGroup label={`节点分组: ${selectedGroup}`}>
              {choices
                .filter((model) => model.source === 'current')
                .map((model) => (
                  <NativeSelectOption key={model.value} value={model.value}>
                    {model.label}
                  </NativeSelectOption>
                ))}
            </NativeSelectOptGroup>
            <NativeSelectOptGroup label='系统执行器'>
              {choices
                .filter((model) => model.source === 'system')
                .map((model) => (
                  <NativeSelectOption key={model.value} value={model.value}>
                    {model.label}
                  </NativeSelectOption>
                ))}
            </NativeSelectOptGroup>
            <NativeSelectOptGroup label='备用模型'>
              {choices
                .filter((model) => model.source === 'fallback')
                .map((model) => (
                  <NativeSelectOption key={model.value} value={model.value}>
                    {model.label}
                  </NativeSelectOption>
                ))}
            </NativeSelectOptGroup>
          </NativeSelect>
        </CompactField>

        <CompactField label='Skill'>
          <NativeSelect
            size='sm'
            value={normalizeBuiltinSkillValue(node.data.skill)}
            title={selectedSkill.description}
            onChange={(event) => onUpdateNode({ skill: event.target.value })}
            className='w-full'
          >
            <NativeSelectOption value={NO_SKILL_CHOICE.value}>
              {NO_SKILL_CHOICE.label}
            </NativeSelectOption>
            {skillChoices.map((skill) => (
              <NativeSelectOption key={skill.value} value={skill.value}>
                {skill.label}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </CompactField>
      </div>
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
  executionLog,
  executionNodes,
  skillChoices,
  isOpen,
  selectedNode,
  onSelectNode,
  onOpenChange,
}: {
  executionLog: AgentExecutionLogMap
  executionNodes: AgentFlowNode[]
  skillChoices: SkillChoice[]
  isOpen: boolean
  selectedNode: AgentFlowNode | null
  onSelectNode: (nodeId: string) => void
  onOpenChange: (open: boolean) => void
}) {
  const logNode = selectedNode ?? executionNodes[0] ?? null

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
        <span className='text-muted-foreground truncate text-[11px]'>
          {executionNodes.length} 个节点 · 当前蓝图链路
        </span>
      </button>

      {isOpen && (
        <div className='border-border/70 grid max-h-72 min-h-0 gap-2 overflow-hidden border-t p-3 xl:grid-cols-[minmax(0,1.45fr)_minmax(20rem,0.9fr)]'>
          {executionNodes.length === 0 ? (
            <div className='text-muted-foreground py-4 text-center text-xs xl:col-span-2'>
              画布上还没有可执行的节点，从左侧拖入组件开始搭建。
            </div>
          ) : (
            <>
              <div className='min-h-0 overflow-y-auto pr-1'>
                <div className='grid gap-2 sm:grid-cols-2 2xl:grid-cols-3'>
                  {executionNodes.map((node, index) => {
                    const meta = NODE_META[node.data.kind]
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

const DEFAULT_OUTPUT_DIR_REPORT = 'data/output/reports'
const DEFAULT_OUTPUT_DIR_WEB = 'data/output/web-reports'
const LEGACY_TEMPLATE_DIR_REPORT = '/templates/reports'
const LEGACY_TEMPLATE_DIR_WEB = '/templates/web'
const LEGACY_OUTPUT_DIR_REPORT = '/output/reports'
const LEGACY_OUTPUT_DIR_WEB = '/output/web-reports'
const TEMPLATE_FILE_ACCEPT =
  '.xlsx,.xls,.doc,.docx,.pdf,.html,.htm,.md,.txt,.csv,.json,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,application/vnd.ms-excel,application/vnd.openxmlformats-officedocument.wordprocessingml.document,application/msword,application/pdf,text/html,text/markdown,text/plain,text/csv,application/json'
const TRANSIENT_BROWSER_SETTING_LABELS = new Set([
  BROWSER_PAGE_DATA_SETTING,
  BROWSER_CAPTURE_TEXT_SETTING,
  '采集标题',
  '采集网址',
  '采集时间',
  '采集错误',
])
const HIDDEN_SETTING_LABELS = new Set([
  '模板目录',
  '模板原文件名',
  '模板预览',
  ...TRANSIENT_BROWSER_SETTING_LABELS,
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
      return 'Markdown 日报'
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

function findReportTemplateForExtension(
  node: AgentFlowNode,
  extension: string
) {
  const targetExt = normalizeExtension(extension)
  const bindings = parseTemplateBindings(
    node.data.settings['模板文件'] || '',
    node.data.settings
  )
  return bindings.find((binding) => {
    const pathExt = normalizeExtension(binding.path.split('.').pop() || '')
    if (pathExt === targetExt) return true
    if (targetExt === 'html' && pathExt === 'htm') return true
    if (targetExt === 'xlsx' && pathExt === 'xls') return true
    if (targetExt === 'docx' && pathExt === 'doc') return true
    return false
  })
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
  settings,
  onChange,
  onChangeSetting,
}: {
  label: string
  value: string
  nodeKind: AgentNodeKind
  settings: Record<string, string>
  onChange: (value: string) => void
  onChangeSetting?: (key: string, value: string) => void
}) {
  const [isUploadingTemplate, setIsUploadingTemplate] = useState(false)
  const [isTemplateLibraryOpen, setIsTemplateLibraryOpen] = useState(false)
  const [isLoadingTemplateLibrary, setIsLoadingTemplateLibrary] =
    useState(false)
  const [templateLibrary, setTemplateLibrary] = useState<AgentFileUpload[]>([])

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
            placeholder='Markdown 日报、CSV 表格、Excel 表格、Word 文档、PDF 文档、HTML 网页、TXT 文本、JSON 数据'
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
              <div className='flex items-center gap-2'>
                <FileTextIcon className='text-muted-foreground size-4 shrink-0' />
                <div className='min-w-0 flex-1'>
                  <div className='text-xs font-medium'>未上传模板文件</div>
                  <div className='text-muted-foreground truncate text-[11px]'>
                    上传或从模板库选择后会绑定到当前节点
                  </div>
                </div>
              </div>
            ) : (
              <div className='space-y-1.5'>
                {bindings.map((binding) => (
                  <div
                    key={binding.path}
                    className='bg-background/70 flex max-w-full min-w-0 items-center gap-2 overflow-hidden rounded-[7px] px-2 py-1.5'
                  >
                    <FileTextIcon className='text-muted-foreground size-3.5 shrink-0' />
                    <div className='min-w-0 flex-1 overflow-hidden'>
                      <div className='max-w-full truncate overflow-hidden text-xs font-medium whitespace-nowrap'>
                        {binding.originalName}
                      </div>
                      <div className='text-muted-foreground max-w-full truncate overflow-hidden text-[11px] whitespace-nowrap'>
                        {binding.path}
                      </div>
                    </div>
                    <Button
                      type='button'
                      size='sm'
                      variant='ghost'
                      className='h-7 rounded-[6px] px-2 text-xs'
                      onClick={() =>
                        applyTemplateBindings(
                          bindings.filter((item) => item.path !== binding.path)
                        )
                      }
                    >
                      移除
                    </Button>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className='grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-2'>
            <div className='flex min-w-0 items-center gap-2 overflow-hidden'>
              <label
                aria-disabled={isUploadingTemplate}
                className='bg-primary text-primary-foreground hover:bg-primary/90 inline-flex h-7 shrink-0 cursor-pointer items-center rounded-[6px] px-2.5 text-xs font-medium whitespace-nowrap aria-disabled:pointer-events-none aria-disabled:opacity-50'
              >
                {isUploadingTemplate ? '上传中' : '上传模板'}
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
                className='h-7 min-w-0 rounded-[6px] px-2.5 text-xs'
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
                className='h-7 shrink-0 rounded-[6px] px-2 text-xs'
                onClick={() => applyTemplateBindings([])}
              >
                清空
              </Button>
            )}
          </div>

          {isTemplateLibraryOpen && (
            <div className='border-border/70 max-h-44 overflow-y-auto rounded-[8px] border p-2'>
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
                        className='hover:bg-accent/50 flex w-full min-w-0 items-center gap-2 overflow-hidden rounded-[7px] px-2 py-1.5 text-left'
                        onClick={() =>
                          applyTemplateBindings(
                            mergeTemplateBindings(bindings, [
                              agentUploadToTemplateBinding(template),
                            ])
                          )
                        }
                      >
                        <FileTextIcon className='text-muted-foreground size-3.5 shrink-0' />
                        <div className='min-w-0 flex-1 overflow-hidden'>
                          <div className='max-w-full truncate overflow-hidden text-xs font-medium whitespace-nowrap'>
                            {template.original_name || template.filename}
                          </div>
                          <div className='text-muted-foreground max-w-full truncate overflow-hidden text-[11px] whitespace-nowrap'>
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

function SkillManagerPanel({
  skillChoices,
  managedSkills: _managedSkills,
  compact: _compact,
  onAddSkill,
  onDeleteSkill,
}: {
  skillChoices: SkillChoice[]
  managedSkills: SkillChoice[]
  compact?: boolean
  onAddSkill: (skill: SkillChoice) => void
  onDeleteSkill: (skillValue: string) => void
}) {
  const [activeTab, setActiveTab] = useState<SkillDraftSource>('github')
  const [githubUrl, setGithubUrl] = useState('')
  const [customName, setCustomName] = useState('')
  const [customDescription, setCustomDescription] = useState('')

  const installedValues = useMemo(
    () => new Set(skillChoices.map((s) => s.value)),
    [skillChoices]
  )

  const handleAdd = () => {
    if (activeTab === 'github') {
      const url = githubUrl.trim()
      if (!url) return
      onAddSkill({
        label: `${formatSkillNameFromReference(url)} Skill`,
        value: `skill.github.${createStableId(url)}`,
        description: '从 GitHub 地址添加的 Skill。',
        source: 'github',
        reference: url,
      })
      setGithubUrl('')
      return
    }
    if (activeTab === 'marketplace') {
      const skill = MARKETPLACE_SKILLS.find(
        (s) => !installedValues.has(s.value)
      )
      if (skill) onAddSkill(skill)
      return
    }
    const name = customName.trim()
    if (!name) return
    onAddSkill({
      label: name.endsWith('Skill') ? name : `${name} Skill`,
      value: `skill.custom.${createStableId(name)}`,
      description: customDescription.trim() || '用户自定义 Skill。',
      source: 'custom',
    })
    setCustomName('')
    setCustomDescription('')
  }

  const builtinSkills = skillChoices.filter((s) => s.source === 'built-in')
  const userSkills = skillChoices.filter((s) => s.source !== 'built-in')

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
              onKeyDown={(e) => e.key === 'Enter' && handleAdd()}
            />
            <Button
              type='button'
              size='sm'
              className='h-7 rounded-[6px] px-2.5 text-xs'
              onClick={handleAdd}
              disabled={!githubUrl.trim()}
            >
              添加
            </Button>
          </div>
        )}

        {/* 商城选择 */}
        {activeTab === 'marketplace' && (
          <div className='flex flex-col gap-1.5'>
            {MARKETPLACE_SKILLS.map((skill) => {
              const installed = installedValues.has(skill.value)
              return (
                <div
                  key={skill.value}
                  className='border-border/50 flex items-center justify-between gap-2 rounded-[7px] border px-2.5 py-2'
                >
                  <div className='min-w-0 flex-1'>
                    <div className='truncate text-[11px] font-medium'>
                      {skill.label}
                    </div>
                    <div className='text-muted-foreground truncate text-[10px]'>
                      {skill.description}
                    </div>
                  </div>
                  <Button
                    type='button'
                    size='sm'
                    variant={installed ? 'ghost' : 'outline'}
                    className='h-6 shrink-0 rounded-[5px] px-2 text-[10px]'
                    disabled={installed}
                    onClick={() => !installed && onAddSkill(skill)}
                  >
                    {installed ? '已添加' : '+ 添加'}
                  </Button>
                </div>
              )
            })}
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
            <Button
              type='button'
              size='sm'
              className='h-7 rounded-[6px] text-xs'
              onClick={handleAdd}
              disabled={!customName.trim()}
            >
              创建 Skill
            </Button>
          </div>
        )}
      </div>

      {/* ── 已安装 Skill 列表 ── */}
      {userSkills.length > 0 && (
        <div>
          <div className='text-muted-foreground mb-1.5 px-0.5 text-[10px] font-semibold tracking-wider uppercase'>
            已安装 ({userSkills.length})
          </div>
          <div className='flex flex-col gap-1'>
            {userSkills.map((skill) => (
              <div
                key={skill.value}
                className='bg-muted/30 hover:bg-muted/50 group flex items-center gap-2 rounded-[7px] px-2.5 py-2 transition-colors'
              >
                <div className='min-w-0 flex-1'>
                  <div className='truncate text-[11px] font-medium'>
                    {skill.label}
                  </div>
                  <div className='text-muted-foreground flex items-center gap-1 truncate text-[10px]'>
                    <span
                      className={cn(
                        'rounded px-1 py-0 text-[9px] font-medium',
                        skill.source === 'github' &&
                          'bg-purple-500/10 text-purple-600',
                        skill.source === 'marketplace' &&
                          'bg-blue-500/10 text-blue-600',
                        skill.source === 'custom' &&
                          'bg-amber-500/10 text-amber-600'
                      )}
                    >
                      {formatSkillSource(skill.source)}
                    </span>
                    {skill.reference && (
                      <span className='truncate opacity-60'>
                        {skill.reference}
                      </span>
                    )}
                  </div>
                </div>
                <button
                  type='button'
                  className='text-muted-foreground hover:text-destructive shrink-0 rounded-[5px] p-1 opacity-0 transition-opacity group-hover:opacity-100'
                  onClick={() => onDeleteSkill(skill.value)}
                  title='删除'
                >
                  <Trash2Icon className='size-3' />
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* ── 内置 Skill ── */}
      {builtinSkills.length > 0 && (
        <div>
          <div className='text-muted-foreground mb-1.5 px-0.5 text-[10px] font-semibold tracking-wider uppercase'>
            内置 ({builtinSkills.length})
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
    <label className='grid gap-1.5'>
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
  const currentChoices: ModelChoice[] = models.map((model) => ({
    label: model.label || model.value,
    value: model.value,
    description: model.description,
    supportsImage: model.supportsImage,
    source: 'current',
  }))
  const choices: ModelChoice[] = [
    ...currentChoices,
    ...MODEL_FALLBACKS.system.map((model) => ({
      ...model,
      source: 'system' as const,
    })),
    ...[
      ...MODEL_FALLBACKS.browser,
      ...MODEL_FALLBACKS.fast,
      ...MODEL_FALLBACKS.reasoning,
      ...MODEL_FALLBACKS.image,
    ].map((model) => ({
      ...model,
      source: 'fallback' as const,
    })),
  ]
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
      .map((blueprint) => {
        const nodes = normalizeSavedBlueprintNodes(
          blueprint.nodes.map((node) => normalizeNode(node)),
          blueprint.status
        )
        return {
          ...blueprint,
          nodes,
          edges: blueprint.edges.map((edge) => normalizeEdge(edge)),
          status: normalizeSavedBlueprintStatus(nodes, blueprint.status),
          executionLog: normalizeExecutionLog(blueprint.executionLog),
        }
      })
      .sort((a, b) => b.updatedAt - a.updatedAt)
  } catch {
    return []
  }
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

function getBlueprintStatus(nodes: AgentFlowNode[]): AgentBlueprintStatus {
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
  if (nodes.some((node) => node.data.status === 'running')) return 'running'
  if (savedStatus === 'paused') return 'paused'
  if (hasScheduleNode(nodes)) return 'running'
  return getBlueprintStatus(nodes)
}

function hasScheduleNode(nodes: AgentFlowNode[]) {
  return nodes.some((node) => node.data.kind === 'schedule')
}

function getRunPreviewButtonLabel(status: AgentBlueprintStatus) {
  if (status === 'paused') return '继续'
  if (status === 'ended') return '重新执行'
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
    status === 'stopped'
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
    data.kind && NODE_META[data.kind] ? data.kind : ('start' as AgentNodeKind)
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
      title: data.title ?? meta.label,
      description: data.description ?? meta.description,
      group: data.group,
      model:
        data.model ?? MODEL_FALLBACKS[meta.role][0]?.value ?? 'gpt-4.1-mini',
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
  currentGroup?: string
) {
  return (Object.keys(NODE_META) as AgentNodeKind[]).map((kind) => {
    const meta = NODE_META[kind]
    return {
      kind,
      title: meta.label,
      description: meta.description,
      group: currentGroup || 'default',
      model: pickDefaultModel(kind, models, currentModel),
      skill: pickDefaultSkill(kind),
      inputs: meta.inputs,
      outputs: meta.outputs,
      settings: meta.settings,
      status: 'ready' as AgentNodeStatus,
    }
  })
}

function pickDefaultModel(
  kind: AgentNodeKind,
  models: ModelOption[],
  currentModel?: string
) {
  const meta = NODE_META[kind]

  if (kind === 'schedule') return 'system.scheduler'
  if (kind === 'start' || kind === 'stop') return 'system.runner'

  const model = chooseModelByRole(models, meta.role, currentModel)
  if (model) return model.value

  return MODEL_FALLBACKS[meta.role][0]?.value ?? currentModel ?? 'gpt-4.1-mini'
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
  if (role === 'system') return null
  if (role === 'image') {
    return (
      models.find((model) => model.supportsImage) ??
      findByPatterns(models, ['gpt-image', 'image', 'imagen', 'dall'])
    )
  }
  if (role === 'browser') {
    return findByPatterns(models, [
      'computer',
      'browser',
      'claude',
      'sonnet',
      'gpt-4.1',
      'gpt-4o',
      'gemini',
    ])
  }
  if (role === 'reasoning') {
    return (
      findByPatterns(models, [
        'o3',
        'o4',
        'reason',
        'gpt-4.1',
        'sonnet',
        'deepseek',
        'qwen',
      ]) ?? models.find((model) => model.value === currentModel)
    )
  }
  return (
    findByPatterns(models, ['mini', 'flash', 'turbo', 'haiku', 'lite']) ??
    models.find((model) => model.value === currentModel)
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
      kind: 'schedule',
      id: 'schedule',
      position: { x: -360, y: -120 },
      patch: {
        title: '每天早上执行',
        description: '每天 08:30 自动启动当前自动化任务。',
        status: 'waiting',
      },
    },
    {
      kind: 'start',
      id: 'start',
      position: { x: -360, y: 90 },
      patch: {
        title: '立即执行一次',
        description: '手动触发当前蓝图，适合临时执行一次任务。',
      },
    },
    {
      kind: 'browser',
      id: 'browser',
      position: { x: 0, y: -20 },
      patch: {
        title: '打开目标页面',
        description:
          '使用当前设备浏览器登录态打开目标页面，按照节点描述读取需要的数据。',
        settings: {
          网址: 'https://www.baidu.com/',
          登录态: '使用当前设备浏览器',
          权限: '只读采集',
          采集等待: '6s',
        },
      },
    },
    {
      kind: 'transform',
      id: 'transform',
      position: { x: 350, y: -20 },
      patch: {
        title: '整理数据',
        description:
          '根据上一节点输出清洗、归并和排序数据，按当前节点描述生成结构化结果。',
      },
    },
    {
      kind: 'image',
      id: 'image',
      position: { x: 700, y: -145 },
      patch: {
        title: '生成图片',
        description: '根据上一节点输出和当前节点描述生成配图或封面图。',
      },
    },
    {
      kind: 'report',
      id: 'report',
      position: { x: 700, y: 95 },
      patch: {
        title: '生成日报',
        description:
          '根据上一节点给出的表格、结论和图片生成日报文档；标题、格式、语气和归档规则只以当前节点描述为准。',
      },
    },
    {
      kind: 'stop',
      id: 'stop',
      position: { x: 1050, y: 95 },
      patch: {
        title: '结束',
        description:
          '确认上游产物已生成后结束当前流程；产物目录以上游输出节点配置为准。',
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
    style: {
      stroke: 'var(--primary)',
      strokeWidth: 1.8,
    },
  } as AgentFlowEdge
}

function filterPaletteGroups(search: string) {
  const keyword = search.trim().toLowerCase()
  if (!keyword) return PALETTE_GROUPS

  return PALETTE_GROUPS.map((group) => ({
    ...group,
    kinds: group.kinds.filter((kind) => {
      const meta = NODE_META[kind]
      return `${meta.label} ${meta.group} ${meta.description}`
        .toLowerCase()
        .includes(keyword)
    }),
  })).filter((group) => group.kinds.length > 0)
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
