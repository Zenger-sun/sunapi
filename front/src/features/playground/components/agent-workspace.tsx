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
import {
  useCallback,
  useMemo,
  useRef,
  useState,
  type DragEvent,
  type ReactNode,
} from 'react'
import {
  addEdge,
  applyEdgeChanges,
  applyNodeChanges,
  Background,
  Controls,
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
  type Node,
  type NodeChange,
  type NodeProps,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import {
  BellIcon,
  BotIcon,
  ChartNetworkIcon,
  ClockIcon,
  FileSpreadsheetIcon,
  FileTextIcon,
  GlobeIcon,
  ImageIcon,
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
  ZapIcon,
  type LucideIcon,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import type { ModelOption } from '../types'

type AgentNodeKind =
  | 'start'
  | 'schedule'
  | 'browser'
  | 'request'
  | 'transform'
  | 'image'
  | 'report'
  | 'notify'
  | 'stop'

type AgentNodeStatus = 'ready' | 'waiting' | 'running' | 'done'

type ModelRole = 'system' | 'browser' | 'fast' | 'reasoning' | 'image'

type AgentNodeData = {
  kind: AgentNodeKind
  title: string
  description: string
  model: string
  skill: string
  inputs: string[]
  outputs: string[]
  settings: Record<string, string>
  status: AgentNodeStatus
}

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
}

type SkillChoice = {
  label: string
  value: string
  description: string
}

type NodeTemplate = {
  kind: AgentNodeKind
  title: string
  description: string
  model: string
  skill: string
  inputs: string[]
  outputs: string[]
  settings: Record<string, string>
  status?: AgentNodeStatus
}

type AgentWorkspaceProps = {
  models: ModelOption[]
  currentModel?: string
}

type SavedAgentBlueprint = {
  id: string
  title: string
  nodes: AgentFlowNode[]
  edges: AgentFlowEdge[]
  createdAt: number
  updatedAt: number
}

const DRAG_NODE_TYPE = 'application/sunapi-agent-node'
const BLUEPRINT_STORAGE_KEY = 'sunapi.playground.agent.blueprints.v1'

const SKILL_CHOICES: SkillChoice[] = [
  {
    label: '系统编排 Skill',
    value: 'skill.system-orchestration',
    description: '处理开始、停止、状态归档和执行守护。',
  },
  {
    label: '定时守护 Skill',
    value: 'skill.schedule-guard',
    description: '校验定时规则、重试窗口和循环停止条件。',
  },
  {
    label: '浏览器取数 Skill',
    value: 'skill.browser-reading',
    description: '优化浏览器页面读取、登录态复用和字段识别。',
  },
  {
    label: '网络请求 Skill',
    value: 'skill.network-fetch',
    description: '优化接口请求、分页、重试和 JSON 结构解析。',
  },
  {
    label: '数据整理 Skill',
    value: 'skill.data-cleaning',
    description: '优化清洗、去重、指标计算和异常标注。',
  },
  {
    label: '图片生成 Skill',
    value: 'skill.image-generation',
    description: '优化封面图、趋势图和数据看板提示词。',
  },
  {
    label: '日报写作 Skill',
    value: 'skill.report-writing',
    description: '优化日报结构、表格说明和运营建议。',
  },
  {
    label: '通知分发 Skill',
    value: 'skill.notification-routing',
    description: '优化发送渠道、确认点和失败补偿。',
  },
]

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
    description: '调用接口或网页请求，获取结构化原始数据。',
    inputs: ['网址', '请求参数', '任务描述'],
    outputs: ['JSON 数据', '请求日志'],
    settings: {
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
    description: '清洗、归并、排序数据，并按模板输出表格。',
    inputs: ['原始数据', '模板', '任务描述'],
    outputs: ['标准表格', '关键结论'],
    settings: {
      表格字段: '商品名, 曝光, 浏览, 想要, 转化建议',
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
    description: '把表格、结论和图片整理成日报文档。',
    inputs: ['标准表格', '关键结论', '图片 URL'],
    outputs: ['Markdown 日报', 'CSV 表格'],
    settings: {
      标题: '闲鱼每日经营日报',
      格式: 'Markdown + CSV',
      语气: '运营复盘',
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
  stop: {
    label: '停止',
    group: '触发',
    role: 'system',
    icon: StopCircleIcon,
    accent: 'bg-slate-500/12 text-slate-700 dark:text-slate-300',
    description: '结束循环或归档本轮执行记录。',
    inputs: ['执行结果'],
    outputs: ['归档状态'],
    settings: {
      结束方式: '本轮完成后停止',
      归档: '保存执行日志',
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
  { title: '输出', kinds: ['report', 'notify'] },
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

const nodeTypes = {
  agentNode: BlueprintNode,
}

export function AgentWorkspace({ models, currentModel }: AgentWorkspaceProps) {
  const modelChoices = useMemo(() => buildModelChoices(models), [models])
  const templates = useMemo(
    () => buildTemplates(models, currentModel),
    [currentModel, models]
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
  const [selectedNodeId, setSelectedNodeId] = useState<string>(
    initialBlueprint?.nodes[0]?.id ?? 'browser'
  )
  const [search, setSearch] = useState('')
  const addCounterRef = useRef(0)

  const selectedNode = useMemo(
    () => nodes.find((node) => node.id === selectedNodeId) ?? null,
    [nodes, selectedNodeId]
  )

  const executionNodes = useMemo(() => orderNodes(nodes, edges), [edges, nodes])

  const onNodesChange = useCallback((changes: NodeChange<AgentFlowNode>[]) => {
    setNodes((current) => applyNodeChanges(changes, current))
  }, [])

  const onEdgesChange = useCallback((changes: EdgeChange<AgentFlowEdge>[]) => {
    setEdges((current) => applyEdgeChanges(changes, current))
  }, [])

  const onConnect = useCallback((connection: Connection) => {
    setEdges((current) =>
      addEdge(decorateEdge({ ...connection, id: `e-${Date.now()}` }), current)
    )
  }, [])

  const handleReset = useCallback(() => {
    setNodes(createInitialNodes(templates))
    setEdges(INITIAL_EDGES.map((edge) => decorateEdge(edge)))
    setSelectedNodeId('browser')
  }, [templates])

  const handleNewBlueprint = useCallback(() => {
    setBlueprintTitle('新的智能体蓝图')
    setCurrentBlueprintId(null)
    setLastSavedAt(null)
    setNodes(createInitialNodes(templates))
    setEdges(INITIAL_EDGES.map((edge) => decorateEdge(edge)))
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
  }, [blueprintTitle, blueprints, currentBlueprintId, edges, nodes])

  const handleLoadBlueprint = useCallback((blueprint: SavedAgentBlueprint) => {
    setCurrentBlueprintId(blueprint.id)
    setBlueprintTitle(blueprint.title)
    setLastSavedAt(blueprint.updatedAt)
    setNodes(blueprint.nodes)
    setEdges(blueprint.edges)
    setSelectedNodeId(blueprint.nodes[0]?.id ?? '')
  }, [])

  const handleRunPreview = useCallback(() => {
    const orderedIds = executionNodes.map((node) => node.id)
    setNodes((current) =>
      current.map((node) => {
        const index = orderedIds.indexOf(node.id)
        if (index === -1) return node
        const status: AgentNodeStatus =
          index === 0 ? 'running' : index < 4 ? 'done' : 'waiting'
        return { ...node, data: { ...node.data, status } }
      })
    )
  }, [executionNodes])

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

  return (
    <ReactFlowProvider>
      <BlueprintWorkspace
        blueprints={blueprints}
        blueprintTitle={blueprintTitle}
        currentBlueprintId={currentBlueprintId}
        edges={edges}
        executionNodes={executionNodes}
        lastSavedAt={lastSavedAt}
        modelChoices={modelChoices}
        nodes={nodes}
        onAddNode={handleAddNode}
        onConnect={onConnect}
        onEdgesChange={onEdgesChange}
        onLoadBlueprint={handleLoadBlueprint}
        onNewBlueprint={handleNewBlueprint}
        onNodesChange={onNodesChange}
        onReset={handleReset}
        onRunPreview={handleRunPreview}
        onSaveBlueprint={handleSaveBlueprint}
        onSearchChange={setSearch}
        onSelectNode={setSelectedNodeId}
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
  modelChoices: ModelChoice[]
  blueprints: SavedAgentBlueprint[]
  blueprintTitle: string
  currentBlueprintId: string | null
  lastSavedAt: number | null
  search: string
  onSearchChange: (value: string) => void
  onNodesChange: (changes: NodeChange<AgentFlowNode>[]) => void
  onEdgesChange: (changes: EdgeChange<AgentFlowEdge>[]) => void
  onConnect: (connection: Connection) => void
  onSelectNode: (nodeId: string) => void
  onAddNode: (kind: AgentNodeKind, position?: { x: number; y: number }) => void
  onTitleChange: (value: string) => void
  onSaveBlueprint: () => void
  onLoadBlueprint: (blueprint: SavedAgentBlueprint) => void
  onNewBlueprint: () => void
  onUpdateNode: (patch: Partial<AgentNodeData>) => void
  onUpdateSetting: (key: string, value: string) => void
  onReset: () => void
  onRunPreview: () => void
}

function BlueprintWorkspace({
  nodes,
  edges,
  selectedNode,
  executionNodes,
  modelChoices,
  blueprints,
  blueprintTitle,
  currentBlueprintId,
  lastSavedAt,
  search,
  onSearchChange,
  onNodesChange,
  onEdgesChange,
  onConnect,
  onSelectNode,
  onAddNode,
  onTitleChange,
  onSaveBlueprint,
  onLoadBlueprint,
  onNewBlueprint,
  onUpdateNode,
  onUpdateSetting,
  onReset,
  onRunPreview,
}: BlueprintWorkspaceProps) {
  const reactFlow = useReactFlow<AgentFlowNode, AgentFlowEdge>()
  const filteredGroups = useMemo(() => filterPaletteGroups(search), [search])

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
          blueprints={blueprints}
          currentBlueprintId={currentBlueprintId}
          groups={filteredGroups}
          onAddNode={onAddNode}
          onLoadBlueprint={onLoadBlueprint}
          onNewBlueprint={onNewBlueprint}
          onSearchChange={onSearchChange}
          search={search}
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
              <Button
                type='button'
                size='sm'
                className='h-8 gap-1.5 rounded-[6px] px-2'
                onClick={onRunPreview}
              >
                <ZapIcon className='size-3.5' />
                试运行
              </Button>
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
            edges={edges}
            fitView
            fitViewOptions={{ padding: 0.22 }}
            maxZoom={1.25}
            minZoom={0.35}
            nodeTypes={nodeTypes}
            nodes={nodes}
            onConnect={onConnect}
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
              className='rounded-[8px] border shadow-none'
              position='bottom-left'
            />
            <MiniMap
              className='border-border/70 bg-background/90 overflow-hidden rounded-[8px] border shadow-sm'
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
      </div>

      <aside className='border-border/70 bg-background hidden w-82 shrink-0 flex-col border-l xl:flex'>
        <Inspector
          executionNodes={executionNodes}
          modelChoices={modelChoices}
          selectedNode={selectedNode}
          onUpdateNode={onUpdateNode}
          onUpdateSetting={onUpdateSetting}
        />
      </aside>
    </div>
  )
}

function Palette({
  groups,
  blueprints,
  currentBlueprintId,
  search,
  onSearchChange,
  onNewBlueprint,
  onLoadBlueprint,
  onAddNode,
}: {
  groups: typeof PALETTE_GROUPS
  blueprints: SavedAgentBlueprint[]
  currentBlueprintId: string | null
  search: string
  onSearchChange: (value: string) => void
  onNewBlueprint: () => void
  onLoadBlueprint: (blueprint: SavedAgentBlueprint) => void
  onAddNode: (kind: AgentNodeKind) => void
}) {
  return (
    <div className='flex min-h-0 flex-1 flex-col'>
      <div className='border-border/70 border-b px-3 py-3'>
        <div className='flex items-center gap-2'>
          <div className='bg-primary text-primary-foreground flex size-8 items-center justify-center rounded-[8px]'>
            <BotIcon className='size-4' />
          </div>
          <div className='min-w-0'>
            <div className='truncate text-sm font-semibold'>组件库</div>
            <div className='text-muted-foreground truncate text-xs'>
              蓝图任务节点
            </div>
          </div>
        </div>
        <div className='relative mt-3'>
          <SearchIcon className='text-muted-foreground absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2' />
          <Input
            value={search}
            onChange={(event) => onSearchChange(event.target.value)}
            placeholder='搜索组件'
            className='h-8 rounded-[8px] pl-8 text-xs'
          />
        </div>
      </div>
      <div className='min-h-0 flex-1 overflow-y-auto px-2 py-2'>
        <BlueprintHistory
          blueprints={blueprints}
          currentBlueprintId={currentBlueprintId}
          onLoadBlueprint={onLoadBlueprint}
          onNewBlueprint={onNewBlueprint}
        />
        {groups.map((group) => (
          <div key={group.title} className='mb-3'>
            <div className='text-muted-foreground px-1.5 pb-1.5 text-[10px] font-medium tracking-wider uppercase'>
              {group.title}
            </div>
            <div className='grid gap-1.5'>
              {group.kinds.map((kind) => (
                <PaletteItem key={kind} kind={kind} onAddNode={onAddNode} />
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

function BlueprintHistory({
  blueprints,
  currentBlueprintId,
  onNewBlueprint,
  onLoadBlueprint,
}: {
  blueprints: SavedAgentBlueprint[]
  currentBlueprintId: string | null
  onNewBlueprint: () => void
  onLoadBlueprint: (blueprint: SavedAgentBlueprint) => void
}) {
  return (
    <div className='mb-3'>
      <div className='mb-1.5 flex items-center justify-between px-1.5'>
        <div className='text-muted-foreground text-[10px] font-medium tracking-wider uppercase'>
          蓝图历史
        </div>
        <button
          type='button'
          className='text-muted-foreground hover:text-foreground inline-flex size-6 items-center justify-center rounded-[6px]'
          onClick={onNewBlueprint}
          title='新建蓝图'
        >
          <PlusIcon className='size-3.5' />
        </button>
      </div>
      <div className='grid gap-1.5'>
        {blueprints.length === 0 ? (
          <div className='border-border/70 bg-background text-muted-foreground rounded-[8px] border px-2 py-3 text-center text-xs'>
            保存后会出现在这里
          </div>
        ) : (
          blueprints.slice(0, 5).map((blueprint) => (
            <button
              key={blueprint.id}
              type='button'
              onClick={() => onLoadBlueprint(blueprint)}
              className={cn(
                'border-border/70 bg-background hover:bg-accent/50 flex min-w-0 flex-col items-start rounded-[8px] border px-2 py-2 text-left transition-colors',
                currentBlueprintId === blueprint.id &&
                  'border-primary/50 bg-primary/5'
              )}
            >
              <span className='w-full truncate text-xs font-medium'>
                {blueprint.title}
              </span>
              <span className='text-muted-foreground mt-0.5 text-[11px]'>
                {blueprint.nodes.length} 组件 ·{' '}
                {formatBlueprintTime(blueprint.updatedAt)}
              </span>
            </button>
          ))
        )}
      </div>
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
  const defaultModel = pickDefaultModel(kind, [], undefined)

  return (
    <button
      type='button'
      className='border-border/70 bg-background hover:bg-accent/50 flex min-h-18 w-full items-start gap-2 rounded-[8px] border p-2 text-left transition-colors'
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
        <span className='flex items-center justify-between gap-2'>
          <span className='truncate text-sm font-medium'>{meta.label}</span>
          <PlusIcon className='text-muted-foreground size-3.5 shrink-0' />
        </span>
        <span className='text-muted-foreground mt-0.5 line-clamp-2 text-xs leading-snug'>
          {meta.description}
        </span>
        <span className='text-muted-foreground mt-1 block truncate text-[11px]'>
          默认: {defaultModel}
        </span>
      </span>
    </button>
  )
}

function BlueprintNode({
  data,
  selected,
  isConnectable,
}: NodeProps<AgentFlowNode>) {
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
        <SkillLinkBadge skill={data.skill} />
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

function SkillLinkBadge({ skill }: { skill: string }) {
  const skillChoice = getSkillChoice(skill)

  return (
    <div className='flex items-center gap-2 rounded-[6px] border border-fuchsia-500/20 bg-fuchsia-500/8 px-2 py-1.5'>
      <span className='flex items-center gap-1.5 text-[11px] font-medium text-fuchsia-700 dark:text-fuchsia-300'>
        <span className='size-2 rounded-full bg-fuchsia-500' />
        Skill
      </span>
      <span className='h-px min-w-5 flex-1 bg-fuchsia-500/40' />
      <span className='min-w-0 truncate text-xs font-medium'>
        {skillChoice.label}
      </span>
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
  executionNodes,
  modelChoices,
  onUpdateNode,
  onUpdateSetting,
}: {
  selectedNode: AgentFlowNode | null
  executionNodes: AgentFlowNode[]
  modelChoices: ModelChoice[]
  onUpdateNode: (patch: Partial<AgentNodeData>) => void
  onUpdateSetting: (key: string, value: string) => void
}) {
  return (
    <div className='flex min-h-0 flex-1 flex-col'>
      <div className='border-border/70 flex items-center gap-2 border-b px-3 py-3'>
        <div className='bg-muted flex size-8 items-center justify-center rounded-[8px]'>
          <Settings2Icon className='size-4' />
        </div>
        <div className='min-w-0'>
          <div className='truncate text-sm font-semibold'>节点配置</div>
          <div className='text-muted-foreground truncate text-xs'>
            组件描述、模型和输入输出
          </div>
        </div>
      </div>

      <div className='min-h-0 flex-1 overflow-y-auto'>
        {selectedNode ? (
          <div className='space-y-4 p-3'>
            <NodeConfigPanel
              modelChoices={modelChoices}
              node={selectedNode}
              onUpdateNode={onUpdateNode}
              onUpdateSetting={onUpdateSetting}
            />
            <ExecutionPanel executionNodes={executionNodes} />
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

function NodeConfigPanel({
  node,
  modelChoices,
  onUpdateNode,
  onUpdateSetting,
}: {
  node: AgentFlowNode
  modelChoices: ModelChoice[]
  onUpdateNode: (patch: Partial<AgentNodeData>) => void
  onUpdateSetting: (key: string, value: string) => void
}) {
  const meta = NODE_META[node.data.kind]
  const Icon = meta.icon
  const modelExists = modelChoices.some(
    (model) => model.value === node.data.model
  )
  const choices = modelExists
    ? modelChoices
    : [{ label: node.data.model, value: node.data.model }, ...modelChoices]
  const selectedSkill = getSkillChoice(node.data.skill)

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
      </div>

      <div className='space-y-3'>
        <LabeledField label='组件名称'>
          <Input
            value={node.data.title}
            onChange={(event) => onUpdateNode({ title: event.target.value })}
            className='h-8 rounded-[8px] text-sm'
          />
        </LabeledField>

        <LabeledField label='执行模型'>
          <NativeSelect
            value={node.data.model}
            onChange={(event) => onUpdateNode({ model: event.target.value })}
            className='w-full'
          >
            {choices.map((model) => (
              <NativeSelectOption key={model.value} value={model.value}>
                {model.label}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </LabeledField>

        <LabeledField label='Skill 优化'>
          <NativeSelect
            value={node.data.skill}
            onChange={(event) => onUpdateNode({ skill: event.target.value })}
            className='w-full'
          >
            {SKILL_CHOICES.map((skill) => (
              <NativeSelectOption key={skill.value} value={skill.value}>
                {skill.label}
              </NativeSelectOption>
            ))}
          </NativeSelect>
          <div className='text-muted-foreground text-[11px] leading-relaxed'>
            {selectedSkill.description}
          </div>
        </LabeledField>

        <LabeledField label='任务描述'>
          <Textarea
            value={node.data.description}
            onChange={(event) =>
              onUpdateNode({ description: event.target.value })
            }
            className='min-h-24 rounded-[8px] text-sm'
          />
        </LabeledField>

        <div className='space-y-2'>
          <div className='text-xs font-medium'>参数</div>
          {Object.entries(node.data.settings).map(([key, value]) => (
            <LabeledField key={key} label={key}>
              <Input
                value={value}
                onChange={(event) => onUpdateSetting(key, event.target.value)}
                className='h-8 rounded-[8px] text-sm'
              />
            </LabeledField>
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

function ExecutionPanel({
  executionNodes,
}: {
  executionNodes: AgentFlowNode[]
}) {
  return (
    <section className='border-border/70 rounded-[8px] border p-3'>
      <div className='mb-3 flex items-center gap-2'>
        <div className='bg-muted flex size-8 items-center justify-center rounded-[8px]'>
          <FileSpreadsheetIcon className='size-4' />
        </div>
        <div className='min-w-0'>
          <div className='truncate text-sm font-semibold'>执行预览</div>
          <div className='text-muted-foreground truncate text-xs'>
            闲鱼/鱼小铺日报链路
          </div>
        </div>
      </div>
      <div className='space-y-2'>
        {executionNodes.map((node, index) => {
          const meta = NODE_META[node.data.kind]
          const Icon = meta.icon
          return (
            <div
              key={node.id}
              className='border-border/70 bg-muted/20 flex items-start gap-2 rounded-[8px] border p-2'
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
                  Skill · {getSkillChoice(node.data.skill).label}
                </div>
              </div>
              <StatusDot status={node.data.status} />
            </div>
          )
        })}
      </div>
    </section>
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

function buildModelChoices(models: ModelOption[]) {
  const choices: ModelChoice[] = [
    ...MODEL_FALLBACKS.system,
    ...models.map((model) => ({
      label: model.label || model.value,
      value: model.value,
      description: model.description,
      supportsImage: model.supportsImage,
    })),
    ...MODEL_FALLBACKS.browser,
    ...MODEL_FALLBACKS.fast,
    ...MODEL_FALLBACKS.reasoning,
    ...MODEL_FALLBACKS.image,
  ]
  const seen = new Set<string>()
  return choices.filter((choice) => {
    if (seen.has(choice.value)) return false
    seen.add(choice.value)
    return true
  })
}

function readSavedBlueprints(): SavedAgentBlueprint[] {
  if (typeof window === 'undefined') return []

  try {
    const raw = window.localStorage.getItem(BLUEPRINT_STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []

    return parsed
      .filter(isSavedBlueprint)
      .map((blueprint) => ({
        ...blueprint,
        nodes: blueprint.nodes.map((node) => normalizeNode(node)),
        edges: blueprint.edges.map((edge) => normalizeEdge(edge)),
      }))
      .sort((a, b) => b.updatedAt - a.updatedAt)
  } catch {
    return []
  }
}

function writeSavedBlueprints(blueprints: SavedAgentBlueprint[]) {
  if (typeof window === 'undefined') return

  try {
    window.localStorage.setItem(
      BLUEPRINT_STORAGE_KEY,
      JSON.stringify(blueprints)
    )
  } catch {
    /* localStorage may be unavailable or full. */
  }
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

function normalizeNode(node: AgentFlowNode): AgentFlowNode {
  const data = node.data ?? ({} as Partial<AgentNodeData>)
  const kind =
    data.kind && NODE_META[data.kind] ? data.kind : ('start' as AgentNodeKind)
  const meta = NODE_META[kind]

  return {
    ...node,
    id: node.id || `${kind}-${Date.now()}`,
    type: 'agentNode',
    position: node.position ?? { x: 0, y: 0 },
    data: {
      kind,
      title: data.title ?? meta.label,
      description: data.description ?? meta.description,
      model:
        data.model ?? MODEL_FALLBACKS[meta.role][0]?.value ?? 'gpt-4.1-mini',
      skill: data.skill ?? pickDefaultSkill(kind),
      inputs: Array.isArray(data.inputs) ? data.inputs : meta.inputs,
      outputs: Array.isArray(data.outputs) ? data.outputs : meta.outputs,
      settings: data.settings ?? meta.settings,
      status: data.status ?? 'ready',
    },
  }
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

function buildTemplates(models: ModelOption[], currentModel?: string) {
  return (Object.keys(NODE_META) as AgentNodeKind[]).map((kind) => {
    const meta = NODE_META[kind]
    return {
      kind,
      title: meta.label,
      description: meta.description,
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
  const skillByKind: Record<AgentNodeKind, string> = {
    start: 'skill.system-orchestration',
    schedule: 'skill.schedule-guard',
    browser: 'skill.browser-reading',
    request: 'skill.network-fetch',
    transform: 'skill.data-cleaning',
    image: 'skill.image-generation',
    report: 'skill.report-writing',
    notify: 'skill.notification-routing',
    stop: 'skill.system-orchestration',
  }

  return skillByKind[kind]
}

function getSkillChoice(value: string) {
  return (
    SKILL_CHOICES.find((skill) => skill.value === value) ?? {
      label: value,
      value,
      description: '自定义 Skill',
    }
  )
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
    {
      kind: 'stop',
      id: 'stop',
      position: { x: 1050, y: -20 },
      patch: {
        title: '归档并停止',
        description: '保存本轮执行日志和日报产物，结束当前运行。',
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
    type: 'smoothstep',
    animated: true,
    markerEnd: { type: MarkerType.ArrowClosed },
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
