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
	builtinComponent("component.stop", "停止", "用于中断流程、循环控制或标记流程结束。", "stop", "trigger", "system.runner", "skill.mcp-workflow", []string{"执行结果"}, []string{"结束状态"}, map[string]string{"结束方式": "本轮完成后停止"}, []string{"control"}),
	builtinComponent("component.browser", "浏览器取数", "打开当前设备浏览器页面，读取登录态下的后台数据。", "browser", "collect", "gpt-4.1-mini", "skill.browser-automation", []string{"网址", "任务描述"}, []string{"页面数据", "截图证据"}, map[string]string{"网址": "https://www.goofish.com/", "登录态": "使用当前设备浏览器", "权限": "只读采集", "采集等待": "6s"}, []string{"browser", "data"}),
	builtinComponent("component.request", "网页请求", "发起 HTTP 请求，获取接口或页面的结构化原始数据。", "request", "collect", "gpt-4.1-mini", "skill.api-function", []string{"任务描述"}, []string{"JSON 数据", "请求日志"}, map[string]string{"网址": "https://example.com/api", "方法": "GET", "超时": "30s", "重试": "1 次"}, []string{"http", "api"}),
	builtinComponent("component.webhook", "Webhook 触发", "接收外部系统回调并把请求体作为蓝图输入。", "custom", "trigger", "system.runner", "skill.api-function", []string{"Webhook 请求"}, []string{"请求体", "请求头"}, map[string]string{"路径": "/api/agent-webhooks/{blueprint_id}", "方法": "POST", "鉴权": "可选密钥", "密钥": ""}, []string{"webhook", "trigger"}),
	builtinComponent("component.filter", "条件过滤", "按字段、表达式或状态筛选上游数据。", "custom", "process", "gpt-4.1-mini", "skill.code-interpreter", []string{"上游数据", "过滤条件"}, []string{"命中数据", "过滤日志"}, map[string]string{"条件": "字段存在", "未命中处理": "跳过"}, []string{"filter", "data"}),
	builtinComponent("component.merge", "合并数据", "把多个上游分支按索引、字段或追加方式合并。", "custom", "process", "gpt-4.1-mini", "skill.code-interpreter", []string{"数据 A", "数据 B"}, []string{"合并数据", "冲突记录"}, map[string]string{"合并方式": "按字段", "冲突处理": "保留最新"}, []string{"merge", "data"}),
	builtinComponent("component.code", "代码处理", "用脚本完成数据清洗、计算、转换和可复现处理。", "transform", "process", "gpt-4.1-mini", "skill.code-interpreter", []string{"原始数据", "处理规则"}, []string{"处理结果", "运行日志"}, map[string]string{"数据类型": "自动识别", "输出格式": "JSON 数组", "脚本": "select *\nlimit 100"}, []string{"code", "transform"}),
	builtinComponent("component.file", "读写文件", "读取或生成 Excel、CSV、Markdown、HTML、TXT 等文件。", "custom", "process", "gpt-4.1-mini", "skill.file-document", []string{"文件路径", "文件内容"}, []string{"文件内容", "保存路径"}, map[string]string{"模式": "读取", "格式": "自动识别"}, []string{"file", "document"}),
	builtinComponent("component.image", "生成图片", "根据数据和提示词生成封面图、趋势图或运营图片。", "image", "process", "gpt-image-1", "skill.code-interpreter", []string{"提示词", "参考图"}, []string{"图片 URL"}, map[string]string{"尺寸": "16:9", "风格": "清爽数据看板", "数量": "1"}, []string{"image"}),
	builtinComponent("component.report", "生成日报", "按上游数据、当前描述和模板生成日报文档。", "report", "output", "gpt-4.1", "skill.file-document", []string{"标准表格", "关键结论", "图片 URL"}, []string{"Markdown 日报", "CSV 表格"}, map[string]string{"输出目录": "data/output/reports", "输出格式": "Markdown 日报、CSV 表格"}, []string{"report", "document"}),
	builtinComponent("component.webpage", "输出网页", "将报告数据渲染成可交互网页，支持自定义模板。", "webpage", "output", "gpt-4.1", "skill.file-document", []string{"Markdown 日报", "CSV 表格"}, []string{"网页 URL", "数据文件"}, map[string]string{"输出目录": "data/output/web-reports", "输出模式": "输出网页 + 数据"}, []string{"html", "report"}),
	builtinComponent("component.notify", "发送通知", "将任务结果发送到邮件、飞书、企业微信、Slack 或 Webhook。", "notify", "output", "gpt-4.1-mini", "skill.api-function", []string{"消息内容", "通知目标"}, []string{"发送结果"}, map[string]string{"渠道": "Webhook", "通知目标": "", "Webhook": "", "确认": "发送前确认", "超时": "30s"}, []string{"notification", "webhook"}),
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

func mustComponentConfig(payload componentConfigPayload) json.RawMessage {
	raw, err := json.Marshal(payload)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

func mergeBuiltinAgentComponents(userID int64, custom []AgentComponent) []AgentComponent {
	out := make([]AgentComponent, 0, len(builtinAgentComponents)+len(custom))
	seen := map[string]bool{}
	for _, component := range builtinAgentComponents {
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
	case "处理", "智能处理", "process", "processor", "transform":
		return "process"
	case "输出", "output", "sink":
		return "output"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func inferComponentTags(description string) []string {
	lower := strings.ToLower(description)
	tags := []string{"custom-component"}
	keywords := map[string]string{
		"定时": "schedule", "webhook": "webhook", "浏览器": "browser", "网页": "web",
		"http": "api", "api": "api", "接口": "api", "过滤": "filter", "条件": "filter",
		"合并": "merge", "代码": "code", "脚本": "code", "文件": "file", "excel": "spreadsheet",
		"csv": "spreadsheet", "图片": "image", "报告": "report", "日报": "report",
		"通知": "notification", "数据库": "database", "sql": "database",
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
	out := uniqueNonEmptyStrings(caps)
	sort.Strings(out)
	return out
}
