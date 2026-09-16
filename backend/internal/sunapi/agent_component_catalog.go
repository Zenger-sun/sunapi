package sunapi

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

type componentConfigPayload struct {
	Tags         []string          `json:"tags,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Inputs       []string          `json:"inputs,omitempty"`
	Outputs      []string          `json:"outputs,omitempty"`
	Settings     map[string]string `json:"settings,omitempty"`
	ModelGroup   string            `json:"model_group,omitempty"`
	ModelName    string            `json:"model_name,omitempty"`
	Skill        string            `json:"skill,omitempty"`
	Role         string            `json:"role,omitempty"`
	Icon         string            `json:"icon,omitempty"`
	Accent       string            `json:"accent,omitempty"`
}

var builtinAgentComponents = []AgentComponent{
	builtinComponent("component.start", "开始", "手动启动一次蓝图任务。", "start", "trigger", "system.runner", "skill.mcp-workflow", []string{}, []string{"执行信号"}, map[string]string{"触发方式": "手动", "运行模式": "单次执行"}, []string{"trigger"}),
	builtinComponent("component.schedule", "定时", "按固定时间自动触发任务。", "schedule", "trigger", "system.scheduler", "skill.mcp-workflow", []string{}, []string{"定时信号"}, map[string]string{"执行频率": "每天", "执行时间": "08:30", "时区": "Asia/Shanghai"}, []string{"trigger", "schedule"}),
	builtinComponent("component.stop", "停止", "用于中断流程、循环控制或标记流程结束。", "stop", "control", "system.runner", "skill.mcp-workflow", []string{"执行结果"}, []string{"结束状态"}, map[string]string{"结束方式": "本轮完成后停止"}, []string{"control"}),
	builtinComponent("component.browser", "打开浏览器", "打开指定网页并采集页面内容；需要登录时可复用已保存的浏览器会话。", "browser", "collect", "gpt-4.1-mini", "skill.browser-automation", []string{"网址", "登录要求"}, []string{"浏览器会话", "登录状态"}, map[string]string{"网址": "", "登录态": "持久化登录会话（首次登录后复用）", "会话名称": "默认", "登录等待": "120s", "权限": "只读采集", "采集等待": "6s"}, []string{"browser", "session", "data"}),
	builtinComponent("component.request", "API / 网页请求", "发起 HTTP 请求，获取接口或页面的结构化原始数据。", "request", "collect", "gpt-4.1-mini", "skill.api-function", []string{"任务描述"}, []string{"JSON 数据", "请求日志"}, map[string]string{"网址": "", "方法": "GET", "超时": "30s", "重试": "1 次"}, []string{"http", "api"}),
	builtinComponent("component.webhook", "Webhook 触发", "接收外部系统回调并把请求体作为蓝图输入。", "custom", "trigger", "system.runner", "skill.api-function", []string{"Webhook 请求"}, []string{"请求体", "请求头"}, map[string]string{"路径": "/api/agent-webhooks/{blueprint_id}", "方法": "POST", "鉴权": "可选密钥", "密钥": ""}, []string{"webhook", "trigger"}),
	builtinComponent("component.branch", "条件分支", "根据上游数据和条件表达式输出是/否分支结果。", "custom", "control", "gpt-4.1-mini", "skill.mcp-workflow", []string{"上游数据", "条件"}, []string{"分支结果", "命中数据"}, map[string]string{"条件": "status=ok", "真分支": "通过", "假分支": "未通过"}, []string{"branch", "condition", "control"}),
	builtinComponent("component.switch", "多路判断", "按字段值选择多个命名分支，并在没有匹配时走默认分支。", "custom", "control", "gpt-4.1-mini", "skill.mcp-workflow", []string{"上游数据", "判断字段"}, []string{"选中分支", "判断值"}, map[string]string{"判断字段": "status", "分支配置": "ok=正常\nerror=异常", "默认分支": "其他"}, []string{"switch", "condition", "control"}),
	builtinComponent("component.set_variable", "设置变量", "把固定值或上游结果写入蓝图变量，供后续节点和条件使用。", "custom", "control", "system.runner", "skill.mcp-workflow", []string{"上游数据", "变量名", "变量值"}, []string{"变量结果", "变量快照"}, map[string]string{"变量名": "result", "变量值": "", "为空时": "使用上游数据"}, []string{"variable", "state", "control"}),
	builtinComponent("component.subflow", "调用子蓝图", "调用另一个蓝图作为可复用子流程，并把执行结果传递给当前蓝图。", "custom", "control", "system.runner", "skill.mcp-workflow", []string{"上游数据", "输入变量"}, []string{"子蓝图结果", "子蓝图状态"}, map[string]string{"蓝图ID": "", "输入映射": "", "递归限制": "8 层"}, []string{"subflow", "workflow", "control"}),
	builtinComponent("component.wait", "等待延迟", "按配置等待一段时间后继续传递上游数据。", "custom", "control", "system.runner", "skill.mcp-workflow", []string{"上游数据"}, []string{"等待结果", "上游数据"}, map[string]string{"等待时间": "5s", "最大等待": "60s"}, []string{"wait", "delay", "control"}),
	builtinComponent("component.loop", "列表循环", "把上游数组、表格或对象展开为可迭代条目。", "custom", "control", "gpt-4.1-mini", "skill.code-interpreter", []string{"列表数据"}, []string{"迭代条目", "条目数量"}, map[string]string{"最大条目": "100", "输出字段": "items"}, []string{"loop", "iteration", "data"}),
	builtinComponent("component.loop_collect", "汇总循环", "收集逐项处理结果并结束循环上下文，继续执行后续节点。", "custom", "control", "system.runner", "skill.mcp-workflow", []string{"逐项结果"}, []string{"汇总数据", "处理数量"}, map[string]string{"结果字段": "items", "汇总失败项": "保留并标记"}, []string{"loop", "collect", "control"}),
	builtinComponent("component.filter", "条件过滤", "按字段、表达式或状态筛选上游数据。", "custom", "process", "gpt-4.1-mini", "skill.code-interpreter", []string{"上游数据", "过滤条件"}, []string{"命中数据", "过滤日志"}, map[string]string{"条件": "字段存在", "未命中处理": "跳过"}, []string{"filter", "data"}),
	builtinComponent("component.merge", "合并数据", "把多个上游分支按索引、字段或追加方式合并。", "custom", "process", "gpt-4.1-mini", "skill.code-interpreter", []string{"数据 A", "数据 B"}, []string{"合并数据", "冲突记录"}, map[string]string{"合并方式": "按字段", "冲突处理": "保留最新"}, []string{"merge", "data"}),
	builtinComponent("component.dedupe_sort", "去重排序", "按字段对上游行数据去重、排序和限制数量。", "custom", "process", "gpt-4.1-mini", "skill.code-interpreter", []string{"行数据"}, []string{"处理后数据", "处理日志"}, map[string]string{"去重字段": "id", "排序字段": "updated_at", "排序方向": "降序", "限制": "100"}, []string{"dedupe", "sort", "data"}),
	builtinComponent("component.code", "代码处理", "用脚本完成数据清洗、计算、转换和可复现处理。", "transform", "process", "gpt-4.1-mini", "skill.code-interpreter", []string{"原始数据", "处理规则"}, []string{"处理结果", "运行日志"}, map[string]string{"数据类型": "自动识别", "输出格式": "JSON 数组", "脚本": "select *\nlimit 100"}, []string{"code", "transform"}),
	builtinComponent("component.database", "数据库查询", "对 SQLite 数据库执行只读 SQL 查询并输出结构化结果。", "custom", "collect", "gpt-4.1-mini", "skill.database-sql", []string{"SQL"}, []string{"查询结果", "查询日志"}, map[string]string{"数据库": "当前 SunAPI 数据库", "SQL": "select name from sqlite_master where type='table' order by name", "限制": "100"}, []string{"database", "sql", "data"}),
	builtinComponent("component.file", "读写文件", "读取或生成 Excel、CSV、Markdown、HTML、TXT 等文件。", "custom", "process", "gpt-4.1-mini", "skill.file-document", []string{"文件路径", "文件内容"}, []string{"文件内容", "保存路径"}, map[string]string{"模式": "读取", "格式": "自动识别"}, []string{"file", "document"}),
	builtinComponent("component.image", "生成图片", "根据数据和提示词生成封面图、趋势图或运营图片。", "image", "process", "gpt-image-1", "skill.code-interpreter", []string{"提示词", "参考图"}, []string{"图片 URL"}, map[string]string{"尺寸": "16:9", "风格": "清爽数据看板", "数量": "1"}, []string{"image"}),
	builtinComponent("component.report", "生成报告", "按上游数据、当前描述和模板生成结构化报告文档。", "report", "output", "gpt-4.1", "skill.file-document", []string{"标准表格", "关键结论", "图片 URL"}, []string{"Markdown 文档", "CSV 表格"}, map[string]string{"输出目录": "data/output/reports", "输出格式": "Markdown 文档、CSV 表格"}, []string{"report", "document"}),
	builtinComponent("component.webpage", "输出网页", "将结果数据渲染成可访问网页，支持自定义模板。", "webpage", "output", "gpt-4.1", "skill.file-document", []string{"Markdown 文档", "CSV 表格"}, []string{"网页 URL", "数据文件"}, map[string]string{"输出目录": "data/output/web-reports", "输出模式": "输出网页 + 数据"}, []string{"html", "report"}),
	builtinComponent("component.notify", "发送通知", "将任务结果发送到邮件、飞书、企业微信、Slack 或 Webhook。", "notify", "output", "gpt-4.1-mini", "skill.api-function", []string{"消息内容", "通知目标"}, []string{"发送结果"}, map[string]string{"渠道": "Webhook", "通知目标": "", "Webhook": "", "确认": "发送前确认", "超时": "30s"}, []string{"notification", "webhook"}),
}

// Specialized business workflows stay outside the generic built-in catalog.
// They are exposed as custom components so the built-in palette remains reusable.
var customAgentComponents = []AgentComponent{
	customComponent("component.xianyu", "闲鱼客服归档", "复用闲鱼卖家后台登录态，只读阅读客服对话和当前商品信息，脱敏总结后逐会话写入当前智能体选择的知识库。默认读取列表中的全部会话，每条成功后立即保存，任务中断后可从未完成会话继续。", "xianyu", "collect", "gpt-5.6-terra", "skill.browser-automation", []string{"闲鱼客服会话", "当前智能体知识库"}, []string{"知识库文档", "会话统计"}, map[string]string{"网址": "https://seller.goofish.com/?site=COMM", "登录态": "持久化登录会话（首次登录后复用）", "会话名称": "闲鱼客服", "登录等待": "180s", "列表等待": "8s", "会话数量": "全部", "最大会话保护上限": "5000", "同步范围": "仅归档未归档或有新消息的会话", "单会话等待": "1s", "历史加载轮数": "6", "对话列表选择器": "", "消息区域选择器": "", "消息滚动容器选择器": "", "商品详情选择器": "", "权限": "只读采集", "节点超时": "不限制"}, []string{"xianyu", "customer-service", "knowledge"}),
	customComponent("component.xianyu-reply", "闲鱼实时智能客服", "监听已打开的闲鱼卖家工作台页面。新买家消息到达时读取当前商品和最近对话，检索当前智能体知识库并直接自动回复；无匹配或模型拒答时结合商品描述和状态发送回复。", "xianyu_reply", "process", "gpt-5.6-terra", "skill.browser-automation", []string{"闲鱼新消息", "当前智能体知识库"}, []string{"自动回复统计", "监听状态"}, map[string]string{"网址": "https://seller.goofish.com/?site=COMM", "登录态": "持久化登录会话（首次登录后复用）", "会话名称": "闲鱼客服", "登录等待": "180s", "列表等待": "8s", "消息轮询间隔": "2s", "每轮最大会话数": "20", "单会话等待": "1s", "启动时处理现有未读": "是", "最大回复字数": "300", "默认回复": "开启", "无知识库命中": "商品描述和状态回复", "发送方式": "检测到买家消息立即自动发送", "对话列表选择器": "", "消息区域选择器": "", "消息滚动容器选择器": "", "商品详情选择器": "", "消息输入框选择器": "", "发送按钮选择器": "", "节点超时": "不限制"}, []string{"xianyu", "customer-service", "knowledge", "auto-reply", "realtime"}),
	customComponent("component.xianyu-review", "闲鱼自动评价", "复用闲鱼卖家后台登录态，扫描全部客服会话和订单区域，仅对明确显示可评价入口的已完成交易填写评价并提交；已评价、无订单或无法确认状态的会话自动跳过。", "xianyu_review", "process", "system.runner", "skill.browser-automation", []string{"闲鱼客服会话", "订单信息"}, []string{"评价统计", "操作日志"}, map[string]string{"网址": "https://seller.goofish.com/?site=COMM", "登录态": "持久化登录会话（首次登录后复用）", "会话名称": "闲鱼客服", "登录等待": "180s", "列表等待": "8s", "评价内容": "沟通顺畅，交易愉快。", "评价星级": "5", "会话数量": "全部", "单会话等待": "1s", "对话列表选择器": "", "订单区域选择器": "", "评价入口选择器": "", "评价输入框选择器": "", "评价提交按钮选择器": "", "权限": "读写操作", "节点超时": "不限制"}, []string{"xianyu", "review", "order", "write-operation", "customer-service"}),
}

func builtinComponent(id, name, description, kind, group, model, skill string, inputs, outputs []string, settings map[string]string, tags []string) AgentComponent {
	config := mustComponentConfig(componentConfigPayload{
		Tags:         normalizeTags(tags),
		Capabilities: inferComponentCapabilities(name + "\n" + description + "\n" + strings.Join(tags, " ")),
		Inputs:       inputs,
		Outputs:      outputs,
		Settings:     settings,
		ModelName:    model,
		Skill:        skill,
	})
	return AgentComponent{
		ID:          id,
		UserID:      0,
		Name:        name,
		Description: description,
		Kind:        kind,
		Group:       group,
		Source:      "builtin",
		Enabled:     true,
		Config:      config,
		CreatedTime: 0,
		UpdatedTime: 0,
	}
}

func customComponent(id, name, description, kind, group, model, skill string, inputs, outputs []string, settings map[string]string, tags []string) AgentComponent {
	component := builtinComponent(id, name, description, kind, group, model, skill, inputs, outputs, settings, append([]string{"custom-component"}, tags...))
	component.Source = "custom"
	return component
}

func mustComponentConfig(payload componentConfigPayload) json.RawMessage {
	raw, err := json.Marshal(payload)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

func mergeAgentComponents(userID int64, custom []AgentComponent) []AgentComponent {
	out := make([]AgentComponent, 0, len(builtinAgentComponents)+len(customAgentComponents)+len(custom))
	seen := map[string]bool{}
	for _, component := range builtinAgentComponents {
		component.UserID = userID
		out = append(out, component)
		seen[component.ID] = true
	}
	for _, component := range customAgentComponents {
		component.UserID = userID
		out = append(out, component)
		seen[component.ID] = true
	}
	for _, component := range custom {
		if seen[component.ID] {
			continue
		}
		out = append(out, component)
	}
	return out
}

func prepareComponentForm(f ComponentFormData) (ComponentFormData, json.RawMessage, error) {
	f.Name = strings.TrimSpace(f.Name)
	f.Description = strings.TrimSpace(f.Description)
	if f.Name == "" {
		return f, nil, errors.New("component name is required")
	}
	if f.Description == "" {
		return f, nil, errors.New("component description is required")
	}
	group := normalizeComponentGroup(f.Group)
	if group == "" {
		group = "process"
	}
	f.Group = group
	inputs := uniqueNonEmptyStrings(f.Inputs)
	outputs := uniqueNonEmptyStrings(f.Outputs)
	if len(outputs) == 0 {
		outputs = []string{"执行结果"}
	}
	settings := map[string]string{}
	if len(f.Settings) > 0 && string(f.Settings) != "null" {
		_ = json.Unmarshal(f.Settings, &settings)
	}
	tags := normalizeTags(append(f.Tags, inferComponentTags(f.Description)...))
	config := mustComponentConfig(componentConfigPayload{
		Tags:         tags,
		Capabilities: inferComponentCapabilities(f.Description + "\n" + strings.Join(tags, " ")),
		Inputs:       inputs,
		Outputs:      outputs,
		Settings:     settings,
		ModelGroup:   strings.TrimSpace(f.ModelGroup),
		ModelName:    strings.TrimSpace(f.ModelName),
		Skill:        strings.TrimSpace(f.Skill),
	})
	return f, config, nil
}

func normalizeComponentGroup(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "触发", "trigger", "triggers":
		return "trigger"
	case "采集", "collect", "collection", "source":
		return "collect"
	case "处理", "智能处理", "process", "processor", "transform", "default":
		return "process"
	case "控制", "控制流", "control", "flow", "workflow":
		return "control"
	case "输出", "output", "sink":
		return "output"
	default:
		return ""
	}
}

func inferComponentTags(description string) []string {
	lower := strings.ToLower(description)
	tags := []string{"custom-component"}
	keywords := map[string]string{
		"定时": "schedule", "webhook": "webhook", "浏览器": "browser", "网页": "web",
		"http": "api", "api": "api", "接口": "api", "过滤": "filter", "条件": "filter",
		"合并": "merge", "代码": "code", "脚本": "code", "文件": "file", "excel": "spreadsheet",
		"csv": "spreadsheet", "图片": "image", "报告": "report",
		"通知": "notification", "数据库": "database", "sql": "database",
		"流程": "workflow", "分支": "workflow", "循环": "workflow", "等待": "workflow",
		"变量": "state", "多路": "workflow",
		"去重": "dedupe", "排序": "sort",
	}
	for keyword, tag := range keywords {
		if strings.Contains(lower, strings.ToLower(keyword)) {
			tags = append(tags, tag)
		}
	}
	return tags
}

func inferComponentCapabilities(description string) []string {
	lower := strings.ToLower(description)
	caps := []string{"节点输入校验", "结构化输出"}
	if strings.Contains(lower, "browser") || strings.Contains(lower, "浏览器") || strings.Contains(lower, "网页") {
		caps = append(caps, "页面读取")
	}
	if strings.Contains(lower, "api") || strings.Contains(lower, "http") || strings.Contains(lower, "接口") {
		caps = append(caps, "接口调用")
	}
	if strings.Contains(lower, "data") || strings.Contains(lower, "数据") || strings.Contains(lower, "表格") {
		caps = append(caps, "数据处理")
	}
	if strings.Contains(lower, "file") || strings.Contains(lower, "文件") || strings.Contains(lower, "文档") {
		caps = append(caps, "文件读写")
	}
	if strings.Contains(lower, "通知") || strings.Contains(lower, "webhook") {
		caps = append(caps, "结果投递")
	}
	if strings.Contains(lower, "流程") || strings.Contains(lower, "分支") || strings.Contains(lower, "循环") || strings.Contains(lower, "等待") || strings.Contains(lower, "变量") {
		caps = append(caps, "流程控制")
	}
	if strings.Contains(lower, "去重") || strings.Contains(lower, "排序") {
		caps = append(caps, "集合处理")
	}
	out := uniqueNonEmptyStrings(caps)
	sort.Strings(out)
	return out
}
