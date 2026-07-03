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
  ArchiveIcon,
  BellIcon,
  BotIcon,
  ChartNetworkIcon,
  CheckCircle2Icon,
  ChevronDownIcon,
  ChevronUpIcon,
  ClockIcon,
  CpuIcon,
  ExternalLinkIcon,
  FileSpreadsheetIcon,
  FileTextIcon,
  FilterIcon,
  GlobeIcon,
  ImageIcon,
  MonitorIcon,
  NetworkIcon,
  PlayIcon,
  PlusIcon,
  RotateCcwIcon,
  SaveIcon,
  SearchIcon,
  Settings2Icon,
  SparklesIcon,
  StopCircleIcon,
  Table2Icon,
  Trash2Icon,
  XIcon,
  ZapIcon,
  type LucideIcon,
} from 'lucide-react'
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
  AGENT_BLUEPRINT_COMMAND_EVENT,
  AGENT_BLUEPRINTS_CHANGED_EVENT,
  AGENT_BLUEPRINT_STORAGE_KEY,
  getAgentBlueprintStatusLabel,
  notifyAgentBlueprintsChanged,
  type AgentBlueprintCommand,
  type AgentBlueprintStatus,
} from '../lib/agent-blueprints'
import type { GroupOption, ModelOption } from '../types'

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
}

type SkillSource = 'built-in' | 'github' | 'marketplace' | 'custom'

type SkillDraftSource = Exclude<SkillSource, 'built-in'>

type AgentToolPanel = 'components' | 'skills'

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

const DRAG_NODE_TYPE = 'application/sunapi-agent-node'
const AGENT_SKILLS_STORAGE_KEY = 'sunapi.playground.agent.skills.v1'

const NO_SKILL_VALUE = ''
const NO_SKILL_CHOICE: SkillChoice = {
  label: '不使用 Skill',
  value: NO_SKILL_VALUE,
  description: '该节点仅按基础提示词执行，不叠加任何 Skill 优化。',
  source: 'built-in',
}

const BASE_SKILL_CHOICES: SkillChoice[] = [
  {
    label: '系统编排 Skill',
    value: 'skill.system-orchestration',
    description: '处理开始、停止、状态归档和执行守护。',
    source: 'built-in',
  },
  {
    label: '定时守护 Skill',
    value: 'skill.schedule-guard',
    description: '校验定时规则、重试窗口和循环停止条件。',
    source: 'built-in',
  },
  {
    label: '浏览器取数 Skill',
    value: 'skill.browser-reading',
    description: '优化浏览器页面读取、登录态复用和字段识别。',
    source: 'built-in',
  },
  {
    label: '网络请求 Skill',
    value: 'skill.network-fetch',
    description: '优化接口请求、分页、重试和 JSON 结构解析。',
    source: 'built-in',
  },
  {
    label: '数据整理 Skill',
    value: 'skill.data-cleaning',
    description: '优化清洗、去重、指标计算和异常标注。',
    source: 'built-in',
  },
  {
    label: '图片生成 Skill',
    value: 'skill.image-generation',
    description: '优化封面图、趋势图和数据看板提示词。',
    source: 'built-in',
  },
  {
    label: '日报写作 Skill',
    value: 'skill.report-writing',
    description: '优化日报结构、表格说明和运营建议。',
    source: 'built-in',
  },
  {
    label: '通知分发 Skill',
    value: 'skill.notification-routing',
    description: '优化发送渠道、确认点和失败补偿。',
    source: 'built-in',
  },
]

// 内置 Skill 的展示元数据：图标、颜色、可用状态
const BUILTIN_SKILL_META: Record<
  string,
  { icon: LucideIcon; accent: string; badge: string }
> = {
  'skill.system-orchestration': {
    icon: CpuIcon,
    accent: 'bg-slate-500/12 text-slate-600 dark:text-slate-400',
    badge: '可用',
  },
  'skill.schedule-guard': {
    icon: ClockIcon,
    accent: 'bg-sky-500/12 text-sky-600 dark:text-sky-400',
    badge: '可用',
  },
  'skill.browser-reading': {
    icon: GlobeIcon,
    accent: 'bg-blue-500/12 text-blue-600 dark:text-blue-400',
    badge: '可用',
  },
  'skill.network-fetch': {
    icon: NetworkIcon,
    accent: 'bg-cyan-500/12 text-cyan-600 dark:text-cyan-400',
    badge: '可用',
  },
  'skill.data-cleaning': {
    icon: FilterIcon,
    accent: 'bg-violet-500/12 text-violet-600 dark:text-violet-400',
    badge: '可用',
  },
  'skill.image-generation': {
    icon: ImageIcon,
    accent: 'bg-fuchsia-500/12 text-fuchsia-600 dark:text-fuchsia-400',
    badge: '可用',
  },
  'skill.report-writing': {
    icon: FileTextIcon,
    accent: 'bg-amber-500/12 text-amber-600 dark:text-amber-400',
    badge: '可用',
  },
  'skill.notification-routing': {
    icon: BellIcon,
    accent: 'bg-rose-500/12 text-rose-600 dark:text-rose-400',
    badge: '可用',
  },
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
const ARCHIVE_LOCATION_OPTIONS = [
  '工作区 / 日报归档',
  '本地执行日志',
  '团队共享空间',
  '对象存储 / agent-runs',
  '不归档',
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
    '商品名称, 曝光, 浏览, 想要, 转化建议',
    '商品 ID, 标题, 价格, 曝光, 浏览, 想要',
    '日期, 渠道, 访客, 成交, 转化率',
    '自定义字段',
  ],
  去重规则: ['按商品 ID', '按商品链接', '按标题 + 日期', '不去重'],
  缺失处理: ['保留空值并标注', '填充 0', '跳过该行', '请求人工确认'],
  尺寸: ['16:9', '1:1', '4:3', '9:16'],
  风格: ['清爽数据看板', '运营日报封面', '极简图表', '品牌营销图'],
  数量: ['1', '2', '3', '4'],
  格式: ['Markdown + CSV', 'Markdown', 'PDF', 'Markdown + 图片'],
  语气: ['运营复盘', '简洁客观', '专业分析', '行动建议'],
  渠道: ['邮件', '企业微信', '钉钉', '飞书', 'Slack', 'Webhook'],
  确认: ['发送前确认', '自动发送', '失败时确认'],
  结束方式: ['本轮完成后停止', '达到次数后停止', '手动停止', '出错停止'],
  归档: ['保存执行日志', '保存日报与日志', '仅保存错误', '不保存'],
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
    },
  },
  request: {
    label: '网页请求',
    group: '采集',
    role: 'fast',
    icon: NetworkIcon,
    accent: 'bg-cyan-500/12 text-cyan-700 dark:text-cyan-300',
    description: '发起裸 HTTP 请求，获取接口或页面的结构化原始数据，不使用浏览器登录态。',
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
    description: '清洗、归并、排序数据；有模板时按模板输出，没有模板时按数据类型自动整理。',
    inputs: ['原始数据', '字段/格式模板', '任务描述'],
    outputs: ['标准表格', '关键结论'],
    settings: {
      数据类型: '自动识别',
      输出格式: '自动整理',
      表格字段: '自动按数据类型推断',
      整理模板: '未上传模板',
      模板内容: '',
      去重规则: '按商品 ID',
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
    description: '把表格、结论和图片整理成日报文档；格式、语气、标题等要求写在任务描述里。',
    inputs: ['标准表格', '关键结论', '图片 URL'],
    outputs: ['Markdown 日报', 'CSV 表格', '归档路径'],
    settings: {
      模板目录: '/templates/reports',
      输出目录: '/output/reports',
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
    description: '将日报数据渲染成可交互的网页，支持自定义模板，后续只更新数据无需重新生成网页。',
    inputs: ['Markdown 日报', 'CSV 表格'],
    outputs: ['网页 URL', '数据文件'],
    settings: {
      输出目录: '/output/web-reports',
      网页模板: '默认模板',
      模板目录: '/templates/web',
      输出模式: '输出网页 + 数据',
    },
  },
  stop: {
    label: '停止',
    group: '触发',
    role: 'system',
    icon: StopCircleIcon,
    accent: 'bg-slate-500/12 text-slate-700 dark:text-slate-300',
    description: '可选控制节点，用于循环任务、中断流程或单独归档执行记录。',
    inputs: ['执行结果'],
    outputs: ['归档状态'],
    settings: {
      结束方式: '本轮完成后停止',
      归档: '保存执行日志',
      输出目录: '/output/agent-runs',
      归档位置: '工作区 / 日报归档',
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
  const modelChoices = useMemo(
    () => buildModelChoices(models, currentModel),
    [currentModel, models]
  )
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
    initialBlueprint?.title ?? '闲鱼经营日报蓝图'
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

  const selectedNode = useMemo(
    () => nodes.find((node) => node.id === selectedNodeId) ?? null,
    [nodes, selectedNodeId]
  )

  const executionNodes = useMemo(() => orderNodes(nodes, edges), [edges, nodes])
  const currentBlueprintStatus = useMemo(() => getBlueprintStatus(nodes), [nodes])

  const onNodesChange = useCallback((changes: NodeChange<AgentFlowNode>[]) => {
    const removedIds = new Set(
      changes
        .filter((change) => change.type === 'remove')
        .map((change) => change.id)
    )
    setNodes((current) => applyNodeChanges(changes, current))
    if (removedIds.size > 0) {
      setEdges((current) =>
        current.filter(
          (edge) => !removedIds.has(edge.source) && !removedIds.has(edge.target)
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
  }, [])

  const onEdgesChange = useCallback((changes: EdgeChange<AgentFlowEdge>[]) => {
    setEdges((current) => applyEdgeChanges(changes, current))
  }, [])

  const onConnect = useCallback((connection: Connection) => {
    setEdges((current) =>
      addEdge(decorateEdge({ ...connection, id: `e-${Date.now()}` }), current)
    )
  }, [])

  const handleDeleteNode = useCallback(
    (nodeId: string) => {
      const fallbackNodeId = nodes.find((node) => node.id !== nodeId)?.id ?? ''
      setNodes((current) => current.filter((node) => node.id !== nodeId))
      setEdges((current) =>
        current.filter((edge) => edge.source !== nodeId && edge.target !== nodeId)
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
    [nodes]
  )

  const handleDeleteEdge = useCallback((edgeId: string) => {
    setEdges((current) => current.filter((edge) => edge.id !== edgeId))
  }, [])

  const handleReset = useCallback(() => {
    setNodes(createInitialNodes(templates))
    setEdges(INITIAL_EDGES.map((edge) => decorateEdge(edge)))
    setExecutionLog({})
    setSelectedNodeId('browser')
  }, [templates])

  const handleNewBlueprint = useCallback(() => {
    setBlueprintTitle('新的智能体蓝图')
    setCurrentBlueprintId(null)
    setLastSavedAt(null)
    setNodes(createInitialNodes(templates))
    setEdges(INITIAL_EDGES.map((edge) => decorateEdge(edge)))
    setExecutionLog({})
    setSelectedNodeId('browser')
  }, [templates])

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
      status: getBlueprintStatus(nodes),
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
  }, [blueprintTitle, blueprints, currentBlueprintId, edges, executionLog, nodes])

  const handleLoadBlueprint = useCallback((blueprint: SavedAgentBlueprint) => {
    setCurrentBlueprintId(blueprint.id)
    setBlueprintTitle(blueprint.title)
    setLastSavedAt(blueprint.updatedAt)
    setNodes(blueprint.nodes)
    setEdges(blueprint.edges)
    setExecutionLog(blueprint.executionLog ?? {})
    setSelectedNodeId(blueprint.nodes[0]?.id ?? '')
  }, [])

  const handleDeleteBlueprint = useCallback(
    (blueprintId: string) => {
      const next = blueprints.filter((item) => item.id !== blueprintId)
      setBlueprints(next)
      writeSavedBlueprints(next)
      notifyAgentBlueprintsChanged()

      if (currentBlueprintId !== blueprintId) return

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
    [blueprints, currentBlueprintId, handleLoadBlueprint, templates]
  )

  const handleRunPreview = useCallback(() => {
    const statusById = new Map<string, AgentNodeStatus>()
    executionNodes.forEach((node) => {
      statusById.set(node.id, 'done')
    })
    const nextExecutionLog = buildPreviewExecutionLog(
      executionNodes,
      statusById,
      skillChoices
    )
    setExecutionLog(nextExecutionLog)
    setNodes((current) => {
      const nextNodes = current.map((node) => {
        const status = statusById.get(node.id)
        if (!status) return node
        return { ...node, data: { ...node.data, status } }
      })
      if (currentBlueprintId) {
        const nextBlueprints = blueprints.map((blueprint) =>
          blueprint.id === currentBlueprintId
            ? {
                ...blueprint,
                nodes: nextNodes,
                updatedAt: Date.now(),
                status: getBlueprintStatus(nextNodes),
                executionLog: nextExecutionLog,
              }
            : blueprint
        )
        setBlueprints(nextBlueprints)
        writeSavedBlueprints(nextBlueprints)
        notifyAgentBlueprintsChanged()
      }
      return nextNodes
    })
  }, [blueprints, currentBlueprintId, executionNodes, skillChoices])

  const handleStopPreview = useCallback(() => {
    const stoppedAt = Date.now()
    const nextExecutionLog = stopRunningExecutionLogs(executionLog, stoppedAt)
    setExecutionLog(nextExecutionLog)
    setNodes((current) => {
      const nextNodes = current.map((node) =>
        node.data.status === 'running'
          ? { ...node, data: { ...node.data, status: 'waiting' as AgentNodeStatus } }
          : node
      )
      if (currentBlueprintId) {
        const nextBlueprints = blueprints.map((blueprint) =>
          blueprint.id === currentBlueprintId
            ? {
                ...blueprint,
                nodes: nextNodes,
                updatedAt: Date.now(),
                status: 'paused' as const,
                executionLog: nextExecutionLog,
              }
            : blueprint
        )
        setBlueprints(nextBlueprints)
        writeSavedBlueprints(nextBlueprints)
        notifyAgentBlueprintsChanged()
      }
      return nextNodes
    })
  }, [blueprints, currentBlueprintId, executionLog])

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
      addCounterRef.current += 1
      const id = `${kind}-${Date.now()}-${addCounterRef.current}`
      const node = createNodeFromTemplate(template, id, {
        x: position?.x ?? 120 + addCounterRef.current * 36,
        y: position?.y ?? 120 + addCounterRef.current * 28,
      })
      setNodes((current) => [...current, node])
      setSelectedNodeId(id)
    },
    [templateByKind]
  )

  const handleUpdateNode = useCallback(
    (patch: Partial<AgentNodeData>) => {
      if (!selectedNode) return
      setNodes((current) =>
        current.map((node) =>
          node.id === selectedNode.id
            ? { ...node, data: { ...node.data, ...patch } }
            : node
        )
      )
    },
    [selectedNode]
  )

  const handleUpdateSetting = useCallback(
    (key: string, value: string) => {
      if (!selectedNode) return
      handleUpdateNode({
        settings: {
          ...selectedNode.data.settings,
          [key]: value,
        },
      })
    },
    [handleUpdateNode, selectedNode]
  )

  const handleSetNodeSkill = useCallback((nodeId: string, skill: string) => {
    setNodes((current) =>
      current.map((node) =>
        node.id === nodeId ? { ...node, data: { ...node.data, skill } } : node
      )
    )
  }, [])

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
        currentBlueprintStatus={currentBlueprintStatus}
        currentGroup={currentGroup}
        currentModel={currentModel}
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
        onReset={handleReset}
        onRunPreview={handleRunPreview}
        onStopPreview={handleStopPreview}
        onSaveBlueprint={handleSaveBlueprint}
        onSearchChange={setSearch}
        onSelectNode={setSelectedNodeId}
        onSetNodeSkill={handleSetNodeSkill}
        onTitleChange={setBlueprintTitle}
        onUpdateNode={handleUpdateNode}
        onUpdateSetting={handleUpdateSetting}
        search={search}
        selectedNode={selectedNode}
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
  currentBlueprintStatus: AgentBlueprintStatus
  currentGroup?: string
  currentModel?: string
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
  onReset: () => void
  onRunPreview: () => void
  onStopPreview: () => void
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
  currentBlueprintStatus,
  currentGroup,
  currentModel,
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
  onReset,
  onRunPreview,
  onStopPreview,
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

      <div className='flex min-w-0 flex-1 flex-col'>
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
                  <StopCircleIcon className='size-3.5' />
                  停止
                </Button>
              ) : (
                <Button
                  type='button'
                  size='sm'
                  className='h-8 gap-1.5 rounded-[6px] px-2'
                  onClick={onRunPreview}
                >
                  <ZapIcon className='size-3.5' />
                  {currentBlueprintStatus === 'paused' ? '继续' : '试运行'}
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
              className='!left-3 !bottom-3 rounded-[8px] border shadow-none'
              position='bottom-left'
            />
            <MiniMap
              className='border-border/70 bg-background/90 !right-3 !bottom-3 overflow-hidden rounded-[8px] border shadow-sm'
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

      <aside className='border-border/70 bg-background hidden w-82 shrink-0 flex-col border-l xl:flex'>
        <Inspector
          currentGroup={currentGroup}
          currentModel={currentModel}
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

          <TabsList className='h-7 w-full rounded-none border-0 bg-transparent p-0 gap-0'>
            <TabsTrigger
              value='components'
              className='h-7 flex-1 rounded-none border-b-2 border-transparent bg-transparent px-2 text-xs font-medium shadow-none data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:shadow-none'
            >
              组件
            </TabsTrigger>
            <TabsTrigger
              value='skills'
              className='h-7 flex-1 rounded-none border-b-2 border-transparent bg-transparent px-2 text-xs font-medium shadow-none data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:shadow-none'
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
                <div className='text-muted-foreground px-1 pb-1 pt-0.5 text-[10px] font-semibold uppercase tracking-widest'>
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

function ToolPanelToggle({
  active,
  description,
  icon: _Icon,
  title: _title,
  onClick: _onClick,
}: {
  active: boolean
  description: string
  icon: LucideIcon
  title: string
  onClick: () => void
}) {
  // Kept for API compatibility but no longer rendered in the new Palette design
  return null
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
      className='hover:bg-accent/60 group flex w-full items-center gap-2.5 rounded-[7px] px-2 py-2 text-left transition-colors'
      draggable
      onClick={() => onAddNode(kind)}
      onDragStart={(event) => {
        event.dataTransfer.setData(DRAG_NODE_TYPE, kind)
        event.dataTransfer.effectAllowed = 'move'
      }}
    >
      <span
        className={cn(
          'flex size-7 shrink-0 items-center justify-center rounded-[6px]',
          meta.accent
        )}
      >
        <Icon className='size-3.5' />
      </span>
      <span className='min-w-0 flex-1'>
        <span className='block truncate text-xs font-medium'>{meta.label}</span>
        <span className='text-muted-foreground line-clamp-2 block whitespace-normal break-words text-[10px] leading-snug'>
          {meta.description}
        </span>
      </span>
      <PlusIcon className='text-muted-foreground size-3 shrink-0 opacity-0 transition-opacity group-hover:opacity-100' />
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
      {!hasSkill && <PlusIcon className='text-muted-foreground size-3 shrink-0' />}
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
  currentModel,
  onDeleteNode,
  onUpdateNode,
  onUpdateSetting,
}: {
  selectedNode: AgentFlowNode | null
  modelChoices: ModelChoice[]
  groupChoices: GroupOption[]
  skillChoices: SkillChoice[]
  currentGroup?: string
  currentModel?: string
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
              currentModel={currentModel}
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
        <p className='opacity-60'>点击「试运行」后此处会显示每个节点的执行状态</p>
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
      {executionNodes.map((node, index) => {
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
              <div className='bg-background flex size-5 shrink-0 items-center justify-center rounded border text-[10px] font-semibold'>
                {index + 1}
              </div>
              <div
                className={cn(
                  'flex size-6 shrink-0 items-center justify-center rounded-[6px]',
                  meta.accent
                )}
              >
                <Icon className='size-3' />
              </div>
              <div className='min-w-0 flex-1'>
                <div className='truncate text-[11px] font-medium'>{node.data.title}</div>
                <div className='text-muted-foreground truncate text-[10px]'>{node.data.model}</div>
              </div>
              <StatusDot status={status} />
            </div>

            <div className='mt-2 space-y-2 pl-[52px]'>
              <div className='flex flex-wrap items-center gap-1.5 text-[10px]'>
                <span
                  className={cn(
                    'rounded-full px-1.5 py-0.5 font-medium',
                    logStatus === 'running' && 'bg-blue-500/12 text-blue-700',
                    logStatus === 'completed' &&
                      'bg-emerald-500/12 text-emerald-700',
                    logStatus === 'pending' &&
                      'bg-amber-500/12 text-amber-700',
                    logStatus === 'stopped' &&
                      'bg-slate-500/12 text-slate-700'
                  )}
                >
                  {formatLogStatus(logStatus)}
                </span>
                <span className='text-muted-foreground'>
                  开始 {log?.startedAt ? formatLogTime(log.startedAt) : '未开始'}
                </span>
                <span className='text-muted-foreground'>
                  耗时 {formatDuration(log?.durationMs)}
                </span>
              </div>

              <p className='text-muted-foreground text-[10px] leading-relaxed'>
                {log?.message ?? '节点尚未开始执行，等待上游节点完成后进入队列。'}
              </p>

              {log && (
                <>
                  <LogSummaryList title='输入' values={log.inputSummary} />
                  <LogSummaryList title='输出' values={log.outputSummary} />
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
      steps: [...log.steps, '收到停止指令并写入节点日志。'],
    }
  }
  return next
}

function buildPreviewExecutionLog(
  executionNodes: AgentFlowNode[],
  statusById: Map<string, AgentNodeStatus>,
  skillChoices: SkillChoice[]
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

    acc[node.id] = {
      nodeId: node.id,
      status: logStatus,
      startedAt,
      endedAt,
      durationMs,
      message: buildNodeLogMessage(node, logStatus),
      inputSummary: buildNodeInputSummary(node),
      outputSummary: buildNodeOutputSummary(node, logStatus),
      steps: buildNodeLogSteps(node, logStatus, skillChoices),
    }
    return acc
  }, {})
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

function buildNodeInputSummary(node: AgentFlowNode) {
  const summaries = node.data.inputs.map((input) => {
    const settingValue = node.data.settings[input]
    if (settingValue) return `${input}: ${settingValue}`
    return `${input}: 来自上游节点或当前运行上下文`
  })

  if (summaries.length > 0) return summaries

  const triggerSetting =
    node.data.settings['触发方式'] ?? node.data.settings['执行频率']
  return [triggerSetting ? `触发条件: ${triggerSetting}` : '无显式输入']
}

function buildNodeOutputSummary(
  node: AgentFlowNode,
  status: AgentNodeLogStatus
) {
  if (node.data.outputs.length === 0) return ['无显式输出']

  if (node.data.kind === 'report') {
    return buildReportOutputSummary(node, status)
  }

  const suffix: Record<AgentNodeLogStatus, string> = {
    pending: '等待生成',
    running: '生成中',
    completed: '已生成预览样本',
    stopped: '已停止，保留部分上下文',
  }

  return node.data.outputs.map((output) => `${output}: ${suffix[status]}`)
}

function buildReportOutputSummary(
  node: AgentFlowNode,
  status: AgentNodeLogStatus
) {
  const templateDir = node.data.settings['模板目录'] || '/templates/reports'
  const outputDir = node.data.settings['输出目录'] || '/output/reports'

  if (status === 'completed') {
    return [
      'Markdown 日报: 已生成',
      'CSV 表格: 已生成',
      `模板目录: ${templateDir}`,
      `输出目录: ${outputDir}`,
    ]
  }
  if (status === 'running') {
    return ['Markdown 日报: 生成中', 'CSV 表格: 生成中', `输出目录: ${outputDir}`]
  }
  if (status === 'stopped') {
    return ['Markdown 日报: 已停止', 'CSV 表格: 已停止', '输出: 保留部分上下文']
  }
  return ['Markdown 日报: 等待生成', 'CSV 表格: 等待生成', `输出目录: ${outputDir}`]
}

function buildNodeLogSteps(
  node: AgentFlowNode,
  status: AgentNodeLogStatus,
  skillChoices: SkillChoice[]
) {
  const skill = getSkillChoice(node.data.skill, skillChoices)
  const steps = [
    '读取节点配置、输入字段和上游依赖。',
    node.data.skill
      ? `加载 Skill: ${skill.label}`
      : '未启用 Skill，使用节点基础提示执行。',
  ]

  if (NODE_META[node.data.kind].role === 'system') {
    steps.push('执行本地编排逻辑。')
  } else {
    steps.push(`准备模型调用: ${node.data.model}`)
  }

  if (node.data.kind === 'transform') {
    const templateName = node.data.settings['整理模板']
    const dataType = node.data.settings['数据类型'] || '自动识别'
    steps.push(
      templateName && templateName !== '未上传模板'
        ? `应用整理模板: ${templateName}`
        : `未上传模板，按数据类型「${dataType}」自动推断字段与格式。`
    )
  }

  if (node.data.kind === 'report') {
    const templateDir = node.data.settings['模板目录'] || '/templates/reports'
    const outputDir = node.data.settings['输出目录'] || '/output/reports'
    steps.push(`读取模板目录: ${templateDir}`)
    steps.push(`将日报产物写入输出目录: ${outputDir}`)
  }

  if (node.data.kind === 'stop') {
    const archiveMode = node.data.settings['归档'] || '保存执行日志'
    const archiveLocation = node.data.settings['归档位置'] || '工作区 / 日报归档'
    steps.push(`结束节点归档策略: ${archiveMode}，位置 ${archiveLocation}。`)
  }

  if (status === 'completed') {
    steps.push(
      node.data.kind === 'stop'
        ? '执行日志归档完成，流程结束。'
        : '校验输出字段并写入下游上下文。'
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

function buildNodeLogMessage(
  node: AgentFlowNode,
  status: AgentNodeLogStatus
) {
  if (status === 'completed') {
    if (node.data.kind === 'report') {
      return `${node.data.title} 已生成日报产物，并写入输出目录。`
    }
    if (node.data.kind === 'stop') {
      return `${node.data.title} 已完成归档与结束动作，流程已结束。`
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
  currentModel,
  onDeleteNode,
  onUpdateNode,
  onUpdateSetting,
}: {
  node: AgentFlowNode
  modelChoices: ModelChoice[]
  groupChoices: GroupOption[]
  skillChoices: SkillChoice[]
  currentGroup?: string
  currentModel?: string
  onDeleteNode: (nodeId: string) => void
  onUpdateNode: (patch: Partial<AgentNodeData>) => void
  onUpdateSetting: (key: string, value: string) => void
}) {
  const meta = NODE_META[node.data.kind]
  const Icon = meta.icon

  return (
    <section className='border-border/70 rounded-[8px] border p-3'>
      <div className='mb-3 flex items-center gap-2'>
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

      <div className='space-y-3'>
        <NodeCommonConfig
          currentGroup={currentGroup}
          currentModel={currentModel}
          groupChoices={groupChoices}
          modelChoices={modelChoices}
          node={node}
          skillChoices={skillChoices}
          onUpdateNode={onUpdateNode}
        />

        <div className='space-y-2'>
          <div className='text-xs font-medium'>参数</div>
          {Object.entries(node.data.settings).map(([key, value]) => (
            <SettingControl
              key={key}
              label={key}
              value={value}
              onChange={(nextValue) => onUpdateSetting(key, nextValue)}
              onChangeSetting={onUpdateSetting}
            />
          ))}
        </div>

        <div className='grid grid-cols-2 gap-2'>
          <FieldList title='输入' values={node.data.inputs} />
          <FieldList title='输出' values={node.data.outputs} />
        </div>
      </div>
    </section>
  )
}

function NodeCommonConfig({
  node,
  modelChoices,
  groupChoices,
  skillChoices,
  currentGroup,
  currentModel,
  onUpdateNode,
}: {
  node: AgentFlowNode
  modelChoices: ModelChoice[]
  groupChoices: GroupOption[]
  skillChoices: SkillChoice[]
  currentGroup?: string
  currentModel?: string
  onUpdateNode: (patch: Partial<AgentNodeData>) => void
}) {
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
    <div className='border-border/70 bg-muted/15 rounded-[8px] border p-2.5'>
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
            value={node.data.skill}
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

      <div className='mt-2 flex min-w-0 flex-wrap items-center gap-1.5 text-[10px] text-muted-foreground'>
        <span className='bg-background/80 max-w-full truncate rounded-[5px] px-1.5 py-0.5'>
          对话组 {currentGroup || 'default'}
        </span>
        <span className='bg-background/80 max-w-full truncate rounded-[5px] px-1.5 py-0.5'>
          对话模型 {currentModel || '未选择'}
        </span>
        <span
          className='min-w-0 flex-1 truncate px-0.5'
          title={selectedSkill.description}
        >
          {selectedSkill.description}
        </span>
      </div>

      <CompactField label='任务描述' className='mt-2'>
        <Textarea
          value={node.data.description}
          onChange={(event) =>
            onUpdateNode({ description: event.target.value })
          }
          className='min-h-16 rounded-[7px] px-2 py-1.5 text-xs leading-relaxed'
        />
      </CompactField>
    </div>
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
          {executionNodes.length} 个节点 · 闲鱼/鱼小铺日报链路
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
                          selected && 'border-primary/50 bg-primary/5 ring-primary/20 ring-2'
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
                            Skill · {getSkillChoice(node.data.skill, skillChoices).label}
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

const DEFAULT_TEMPLATE_DIR_REPORT = '/templates/reports'
const DEFAULT_TEMPLATE_DIR_WEB = '/templates/web'
const DEFAULT_TRANSFORM_TEMPLATE_PLACEHOLDER =
  '字段: 商品名称, 曝光, 浏览, 想要, 转化建议\n格式: Markdown 表格 + JSON 数组\n排序: 按曝光降序\n说明: 缺失字段保留空值并标注来源'

function SettingControl({
  label,
  value,
  onChange,
  onChangeSetting,
}: {
  label: string
  value: string
  onChange: (value: string) => void
  onChangeSetting?: (key: string, value: string) => void
}) {
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

  // 输出目录 —— 文本输入
  if (label === '输出目录') {
    return (
      <LabeledField label='输出目录'>
        <Input
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder='/output/reports'
          className='h-8 rounded-[8px] text-sm'
        />
        <div className='text-muted-foreground text-[11px] leading-relaxed'>
          生成的文件会写入此目录；标题、格式、语气、归档规则等要求请写在任务描述中。
        </div>
      </LabeledField>
    )
  }

  // 模板目录 —— 文本输入 + 显示内置目录
  if (label === '模板目录') {
    return (
      <LabeledField label='模板目录'>
        <Input
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder={DEFAULT_TEMPLATE_DIR_REPORT}
          className='h-8 rounded-[8px] text-sm'
        />
        <div className='text-muted-foreground text-[11px] leading-relaxed'>
          上传的模板文件存放在此目录中，支持 <code className='bg-muted rounded px-1 text-[10px]'>.md</code> / <code className='bg-muted rounded px-1 text-[10px]'>.html</code> 格式。
        </div>
      </LabeledField>
    )
  }

  // 网页模板 —— 文本输入 + 提示
  if (label === '网页模板') {
    return (
      <LabeledField label='网页模板'>
        <Input
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder='默认模板'
          className='h-8 rounded-[8px] text-sm'
        />
        <div className='text-muted-foreground text-[11px] leading-relaxed'>
          将 HTML 模板上传至 <code className='bg-muted rounded px-1 text-[10px]'>{DEFAULT_TEMPLATE_DIR_WEB}</code> 后在此填写模板文件名。
          数据文件输出至网页同级 <code className='bg-muted rounded px-1 text-[10px]'>/data/</code> 目录。
        </div>
      </LabeledField>
    )
  }

  if (label === '整理模板') {
    const hasTemplate = value.trim() !== '' && value !== '未上传模板'

    const handleTemplateUpload = (event: ChangeEvent<HTMLInputElement>) => {
      const input = event.currentTarget
      const file = input.files?.[0]
      if (!file) return

      const reader = new FileReader()
      reader.onload = () => {
        const content = typeof reader.result === 'string' ? reader.result : ''
        onChange(file.name)
        onChangeSetting?.('模板内容', content.slice(0, 12000))
        input.value = ''
      }
      reader.readAsText(file)
    }

    return (
      <LabeledField label='整理模板'>
        <div className='border-border/70 bg-muted/20 flex items-center gap-2 rounded-[8px] border px-2 py-2'>
          <FileTextIcon className='text-muted-foreground size-4 shrink-0' />
          <div className='min-w-0 flex-1'>
            <div className='truncate text-xs font-medium'>
              {hasTemplate ? value : '未上传模板'}
            </div>
            <div className='text-muted-foreground truncate text-[11px]'>
              {hasTemplate
                ? '将优先按上传模板整理字段和输出格式'
                : '未上传时按数据类型自动推断字段和格式'}
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
              }}
            >
              清除
            </Button>
          )}
          <label className='bg-primary text-primary-foreground hover:bg-primary/90 inline-flex h-7 shrink-0 cursor-pointer items-center rounded-[6px] px-2.5 text-xs font-medium'>
            上传
            <input
              type='file'
              accept='.csv,.json,.md,.txt,text/csv,application/json,text/markdown,text/plain'
              className='sr-only'
              onChange={handleTemplateUpload}
            />
          </label>
        </div>
        <div className='text-muted-foreground text-[11px] leading-relaxed'>
          支持 CSV、JSON、Markdown、TXT 模板。模板可写字段顺序、输出格式、排序规则和字段说明。
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
          留空时会根据“数据类型”和上游数据结构自动整理；填写或上传模板后优先按模板输出。
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

  if (label === '归档位置') {
    return (
      <LabeledField label={label}>
        <NativeSelect
          value={value}
          onChange={(event) => onChange(event.target.value)}
          className='w-full'
        >
          {ARCHIVE_LOCATION_OPTIONS.map((option) => (
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
  managedSkills,
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
      const skill = MARKETPLACE_SKILLS.find((s) => !installedValues.has(s.value))
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
        <div className='text-muted-foreground mb-2 text-[10px] font-semibold uppercase tracking-wider'>
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
                    <div className='truncate text-[11px] font-medium'>{skill.label}</div>
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
          <div className='text-muted-foreground mb-1.5 px-0.5 text-[10px] font-semibold uppercase tracking-wider'>
            已安装 ({userSkills.length})
          </div>
          <div className='flex flex-col gap-1'>
            {userSkills.map((skill) => (
              <div
                key={skill.value}
                className='bg-muted/30 hover:bg-muted/50 group flex items-center gap-2 rounded-[7px] px-2.5 py-2 transition-colors'
              >
                <div className='min-w-0 flex-1'>
                  <div className='truncate text-[11px] font-medium'>{skill.label}</div>
                  <div className='text-muted-foreground flex items-center gap-1 truncate text-[10px]'>
                    <span
                      className={cn(
                        'rounded px-1 py-0 text-[9px] font-medium',
                        skill.source === 'github' && 'bg-purple-500/10 text-purple-600',
                        skill.source === 'marketplace' && 'bg-blue-500/10 text-blue-600',
                        skill.source === 'custom' && 'bg-amber-500/10 text-amber-600'
                      )}
                    >
                      {formatSkillSource(skill.source)}
                    </span>
                    {skill.reference && (
                      <span className='truncate opacity-60'>{skill.reference}</span>
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
          <div className='text-muted-foreground mb-1.5 px-0.5 text-[10px] font-semibold uppercase tracking-wider'>
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
                  className='hover:bg-accent/50 group flex items-center gap-2.5 rounded-[8px] px-2 py-2 transition-colors'
                >
                  <div
                    className={cn(
                      'flex size-7 shrink-0 items-center justify-center rounded-[7px]',
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

function FieldList({ title, values }: { title: string; values: string[] }) {
  return (
    <div className='border-border/70 min-w-0 rounded-[8px] border p-2'>
      <div className='text-muted-foreground mb-1 text-[11px]'>{title}</div>
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
    </div>
  )
}

function buildModelChoices(models: ModelOption[], currentModel?: string) {
  const currentChoices: ModelChoice[] = models.map((model) => ({
    label:
      model.value === currentModel
        ? `${model.label || model.value}（对话当前）`
        : model.label || model.value,
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
    window.localStorage.setItem(AGENT_SKILLS_STORAGE_KEY, JSON.stringify(skills))
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
        const nodes = blueprint.nodes.map((node) => normalizeNode(node))
        return {
          ...blueprint,
          nodes,
          edges: blueprint.edges.map((edge) => normalizeEdge(edge)),
          status: blueprint.status ?? getBlueprintStatus(nodes),
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
  if (nodes.length > 0 && nodes.every((node) => node.data.status === 'done')) {
    return 'ended'
  }
  // paused is set externally via handleStopPreview
  return 'pending'
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
      message:
        typeof candidate.message === 'string' ? candidate.message : '',
      inputSummary: stringArray(candidate.inputSummary),
      outputSummary: stringArray(candidate.outputSummary),
      steps: stringArray(candidate.steps),
      error: typeof candidate.error === 'string' ? candidate.error : undefined,
    }
  }
  return out
}

function isAgentNodeLogStatus(
  status: unknown
): status is AgentNodeLogStatus {
  return (
    status === 'pending' ||
    status === 'running' ||
    status === 'completed' ||
    status === 'stopped'
  )
}

function numberOrUndefined(value: unknown) {
  return typeof value === 'number' && Number.isFinite(value)
    ? value
    : undefined
}

function stringArray(value: unknown) {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === 'string')
    : []
}

function normalizeNode(node: AgentFlowNode): AgentFlowNode {
  const data = node.data ?? ({} as Partial<AgentNodeData>)
  const kind =
    data.kind && NODE_META[data.kind] ? data.kind : ('start' as AgentNodeKind)
  const meta = NODE_META[kind]
  const outputs = Array.isArray(data.outputs) ? data.outputs : meta.outputs

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
      skill: data.skill ?? pickDefaultSkill(kind),
      inputs: Array.isArray(data.inputs) ? data.inputs : meta.inputs,
      outputs: kind === 'report' ? mergeStringList(outputs, meta.outputs) : outputs,
      settings: normalizeNodeSettings(kind, data.settings, meta.settings),
      status: data.status ?? 'ready',
    },
  }
}

function normalizeNodeSettings(
  kind: AgentNodeKind,
  settings: Record<string, string> | undefined,
  defaults: Record<string, string>
) {
  if (kind !== 'report') {
    return { ...defaults, ...(settings ?? {}) }
  }

  return {
    模板目录: settings?.['模板目录'] ?? defaults['模板目录'] ?? '/templates/reports',
    输出目录: settings?.['输出目录'] ?? defaults['输出目录'] ?? '/output/reports',
  }
}

function mergeStringList(current: string[], defaults: string[]) {
  const seen = new Set<string>()
  return [...current, ...defaults].filter((value) => {
    if (seen.has(value)) return false
    seen.add(value)
    return true
  })
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

function pickDefaultSkill(_kind: AgentNodeKind) {
  return NO_SKILL_VALUE
}

function getSkillChoice(value: string, skillChoices: SkillChoice[]) {
  if (!value) return NO_SKILL_CHOICE
  return (
    skillChoices.find((skill) => skill.value === value) ?? {
      label: value,
      value,
      description: '自定义 Skill',
      source: 'custom',
    }
  )
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
        description: '每天 08:30 自动启动鱼小铺经营数据日报。',
        status: 'waiting',
      },
    },
    {
      kind: 'start',
      id: 'start',
      position: { x: -360, y: 90 },
      patch: {
        title: '立即执行一次',
        description: '手动触发当前蓝图，适合临时补跑日报。',
      },
    },
    {
      kind: 'browser',
      id: 'browser',
      position: { x: 0, y: -20 },
      patch: {
        title: '打开鱼小铺后台',
        description:
          '使用当前设备浏览器登录态打开闲鱼/鱼小铺后台，逐个读取商品每日曝光、浏览、想要数据。',
        settings: {
          网址: 'https://seller.goofish.com/',
          登录态: '使用当前设备浏览器',
          权限: '只读采集',
        },
      },
    },
    {
      kind: 'transform',
      id: 'transform',
      position: { x: 350, y: -20 },
      patch: {
        title: '清洗商品指标',
        description:
          '按商品维度合并曝光、浏览、想要数据，计算浏览率和想要率，标记异常波动。',
      },
    },
    {
      kind: 'image',
      id: 'image',
      position: { x: 700, y: -145 },
      patch: {
        title: '生成趋势封面图',
        description: '用今日核心指标生成一张适合日报封面的经营趋势看板图。',
      },
    },
    {
      kind: 'report',
      id: 'report',
      position: { x: 700, y: 95 },
      patch: {
        title: '整理日报文档',
        description:
          '按模板输出 Markdown 日报和 CSV 表格，包含商品排名、关键变化和运营建议。',
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
