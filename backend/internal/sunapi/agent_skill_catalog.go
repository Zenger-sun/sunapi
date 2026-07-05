package sunapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

const maxSkillManifestBytes = 512 * 1024

type skillConfigPayload struct {
	Tags         []string `json:"tags,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Instructions []string `json:"instructions,omitempty"`
	Inputs       []string `json:"inputs,omitempty"`
	Outputs      []string `json:"outputs,omitempty"`
	ModelGroup   string   `json:"model_group,omitempty"`
	ModelName    string   `json:"model_name,omitempty"`
	InstallMode  string   `json:"install_mode,omitempty"`
	Source       string   `json:"source,omitempty"`
	EntryFile    string   `json:"entry_file,omitempty"`
}

type skillManifestPayload struct {
	Name         string             `json:"name"`
	Description  string             `json:"description"`
	Version      string             `json:"version"`
	Tags         []string           `json:"tags"`
	Capabilities []string           `json:"capabilities"`
	Instructions []string           `json:"instructions"`
	Permissions  []string           `json:"permissions"`
	Config       skillConfigPayload `json:"config"`
}

var marketplaceSkillCatalog = []AgentSkillMarketplaceItem{
	{
		ID:          "marketplace.web-research-brief",
		Name:        "网页研究简报 Skill",
		Description: "围绕网页资料收集、来源核验和引用摘要生成研究简报。",
		Version:     "1.0.0",
		SourceURL:   "https://docs.n8n.io/integrations/builtin/core-nodes/",
		Tags:        []string{"research", "web", "summary"},
		Capabilities: []string{
			"搜索计划",
			"网页摘要",
			"来源核验",
		},
		Permissions: []string{"network:read"},
		Config: mustSkillConfig(skillConfigPayload{
			Tags:         []string{"research", "web", "summary"},
			Capabilities: []string{"搜索计划", "网页摘要", "来源核验"},
			Instructions: []string{
				"先把问题拆成可验证检索点，再按来源可信度整理结论。",
				"保留来源标题、URL、发布时间或访问时间，区分事实和推断。",
				"当网页不可访问或证据不足时，返回缺口，不补造事实。",
			},
			Inputs:      []string{"问题", "URL", "关键词"},
			Outputs:     []string{"研究摘要", "来源列表", "待确认点"},
			InstallMode: "marketplace",
		}),
	},
	{
		ID:          "marketplace.browser-operator",
		Name:        "浏览器自动化 Skill",
		Description: "处理登录态页面读取、动态 DOM 抽取、截图证据和安全操作边界。",
		Version:     "1.0.0",
		SourceURL:   "https://docs.n8n.io/integrations/builtin/core-nodes/",
		Tags:        []string{"browser", "automation", "dom"},
		Capabilities: []string{
			"页面导航",
			"DOM 抽取",
			"截图证据",
		},
		Permissions: []string{"browser:read", "file:write"},
		Config: mustSkillConfig(skillConfigPayload{
			Tags:         []string{"browser", "automation", "dom"},
			Capabilities: []string{"页面导航", "DOM 抽取", "截图证据"},
			Instructions: []string{
				"只按节点给出的 URL、登录态和任务描述读取页面。",
				"对分页、弹窗、懒加载和动态图表记录处理步骤。",
				"默认只读；涉及提交、购买、删除、转账等动作必须暂停并请求确认。",
			},
			Inputs:      []string{"URL", "任务描述", "登录态"},
			Outputs:     []string{"页面文本", "结构化数据", "截图路径"},
			InstallMode: "marketplace",
		}),
	},
	{
		ID:          "marketplace.http-api-tool",
		Name:        "HTTP API 调用 Skill",
		Description: "组织 HTTP 请求、鉴权、分页、重试和 JSON/文本响应解析。",
		Version:     "1.0.0",
		SourceURL:   "https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.httprequest/",
		Tags:        []string{"api", "http", "integration"},
		Capabilities: []string{
			"参数校验",
			"分页重试",
			"响应解析",
		},
		Permissions: []string{"network:read"},
		Config: mustSkillConfig(skillConfigPayload{
			Tags:         []string{"api", "http", "integration"},
			Capabilities: []string{"参数校验", "分页重试", "响应解析"},
			Instructions: []string{
				"根据 method、URL、headers、body、timeout 和鉴权配置发起请求。",
				"保留状态码、耗时、响应摘要和关键字段，不吞掉错误。",
				"分页、限流、非 JSON 响应要给出处理方式和剩余限制。",
			},
			Inputs:      []string{"URL", "方法", "请求体", "请求头"},
			Outputs:     []string{"响应数据", "状态码", "请求日志"},
			InstallMode: "marketplace",
		}),
	},
	{
		ID:          "marketplace.file-transformer",
		Name:        "文件转换与表格 Skill",
		Description: "读取 Excel、CSV、PDF、Word、HTML、Markdown 和 TXT 并进行结构化转换。",
		Version:     "1.0.0",
		SourceURL:   "https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.readwritefile/",
		Tags:        []string{"file", "spreadsheet", "document"},
		Capabilities: []string{
			"多格式读取",
			"表格整理",
			"文件生成",
		},
		Permissions: []string{"file:read", "file:write"},
		Config: mustSkillConfig(skillConfigPayload{
			Tags:         []string{"file", "spreadsheet", "document"},
			Capabilities: []string{"多格式读取", "表格整理", "文件生成"},
			Instructions: []string{
				"优先保留模板文件的工作表、表头、格式和命名规则。",
				"读取失败时指出具体文件、格式或字段缺口。",
				"转换结果要写明生成文件路径和格式。",
			},
			Inputs:      []string{"模板文件", "源数据", "输出格式"},
			Outputs:     []string{"转换文件", "字段映射", "处理日志"},
			InstallMode: "marketplace",
		}),
	},
	{
		ID:          "marketplace.data-cleaning",
		Name:        "数据清洗分析 Skill",
		Description: "用于去重、排序、统计、字段映射、指标口径和可复现数据处理。",
		Version:     "1.0.0",
		SourceURL:   "https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.code/",
		Tags:        []string{"data", "code", "analysis"},
		Capabilities: []string{
			"数据清洗",
			"指标计算",
			"口径校验",
		},
		Permissions: []string{"code:execute"},
		Config: mustSkillConfig(skillConfigPayload{
			Tags:         []string{"data", "code", "analysis"},
			Capabilities: []string{"数据清洗", "指标计算", "口径校验"},
			Instructions: []string{
				"优先用确定性计算处理清洗、聚合、排序和转换。",
				"输出字段映射、计算口径、异常值和数据质量提示。",
				"禁止用模型臆测缺失数据；缺字段时返回明确缺口。",
			},
			Inputs:      []string{"表格", "JSON", "指标口径"},
			Outputs:     []string{"清洗数据", "统计摘要", "异常清单"},
			InstallMode: "marketplace",
		}),
	},
	{
		ID:          "marketplace.workflow-control",
		Name:        "流程控制 Skill",
		Description: "处理条件分支、等待延迟、列表循环、批处理和控制流记录。",
		Version:     "1.0.0",
		SourceURL:   "https://docs.n8n.io/integrations/builtin/core-nodes/",
		Tags:        []string{"workflow", "control", "branch", "loop"},
		Capabilities: []string{
			"条件分支",
			"等待延迟",
			"列表循环",
		},
		Permissions: []string{"workflow:execute"},
		Config: mustSkillConfig(skillConfigPayload{
			Tags:         []string{"workflow", "control", "branch", "loop"},
			Capabilities: []string{"条件分支", "等待延迟", "列表循环"},
			Instructions: []string{
				"只根据当前节点条件、等待时间、循环数量和上游输出决定控制结果。",
				"条件表达式必须输出命中分支、命中数据和未命中原因。",
				"循环和等待必须保留限制、截断、耗时和取消状态，避免无限执行。",
			},
			Inputs:      []string{"上游输出", "条件表达式", "循环列表"},
			Outputs:     []string{"控制结果", "命中数据", "执行日志"},
			InstallMode: "marketplace",
		}),
	},
	{
		ID:          "marketplace.notification-router",
		Name:        "通知路由 Skill",
		Description: "把任务结果投递到邮件、Webhook、飞书、企业微信、Slack 等通道。",
		Version:     "1.0.0",
		SourceURL:   "https://platform.zapier.com/docs/actions",
		Tags:        []string{"notification", "webhook", "ops"},
		Capabilities: []string{
			"通道选择",
			"消息格式化",
			"失败重试",
		},
		Permissions: []string{"network:write"},
		Config: mustSkillConfig(skillConfigPayload{
			Tags:         []string{"notification", "webhook", "ops"},
			Capabilities: []string{"通道选择", "消息格式化", "失败重试"},
			Instructions: []string{
				"根据节点配置的通道、收件人、Webhook 和确认策略发送。",
				"保留投递状态、响应码和失败原因。",
				"未配置目标或涉及外发敏感数据时停止并要求补充配置。",
			},
			Inputs:      []string{"消息", "通道", "收件人"},
			Outputs:     []string{"投递状态", "回执", "错误信息"},
			InstallMode: "marketplace",
		}),
	},
	{
		ID:          "marketplace.sql-analyst",
		Name:        "数据库 SQL 分析 Skill",
		Description: "生成只读 SQL、理解 Schema、解释查询结果并检查指标口径。",
		Version:     "1.0.0",
		SourceURL:   "https://python.langchain.com/docs/integrations/tools/sql_database/",
		Tags:        []string{"database", "sql", "analysis"},
		Capabilities: []string{
			"Schema 理解",
			"只读查询",
			"指标解释",
		},
		Permissions: []string{"database:read"},
		Config: mustSkillConfig(skillConfigPayload{
			Tags:         []string{"database", "sql", "analysis"},
			Capabilities: []string{"Schema 理解", "只读查询", "指标解释"},
			Instructions: []string{
				"先确认表、字段、时间范围和过滤条件，再生成 SQL 或解释结果。",
				"默认只读；UPDATE、DELETE、INSERT、DROP 等破坏性语句必须拒绝并要求确认。",
				"输出 SQL、参数、结果摘要、口径说明和数据质量风险。",
			},
			Inputs:      []string{"表结构", "业务问题", "查询结果"},
			Outputs:     []string{"SQL", "结果摘要", "口径风险"},
			InstallMode: "marketplace",
		}),
	},
	{
		ID:          "marketplace.github-actions",
		Name:        "GitHub 工作流 Skill",
		Description: "处理 GitHub Issue、PR、Actions、发布说明和仓库自动化任务。",
		Version:     "1.0.0",
		SourceURL:   "https://docs.github.com/actions",
		Tags:        []string{"github", "workflow", "ci"},
		Capabilities: []string{
			"Issue/PR 摘要",
			"Actions 编排",
			"发布说明",
		},
		Permissions: []string{"network:read", "network:write"},
		Config: mustSkillConfig(skillConfigPayload{
			Tags:         []string{"github", "workflow", "ci"},
			Capabilities: []string{"Issue/PR 摘要", "Actions 编排", "发布说明"},
			Instructions: []string{
				"只基于提供的仓库、Issue、PR、commit、workflow 日志执行。",
				"涉及写入仓库、关闭 issue、合并 PR 或触发发布时，必须说明风险并要求确认。",
				"输出变更摘要、检查结果、后续命令和可追踪链接。",
			},
			Inputs:      []string{"仓库", "Issue/PR", "日志"},
			Outputs:     []string{"处理摘要", "检查清单", "后续动作"},
			InstallMode: "marketplace",
		}),
	},
	{
		ID:          "marketplace.rag-knowledge",
		Name:        "知识库检索 Skill",
		Description: "面向文档、知识库和片段引用的检索增强问答与总结。",
		Version:     "1.0.0",
		SourceURL:   "https://python.langchain.com/docs/integrations/document_loaders/",
		Tags:        []string{"rag", "retrieval", "document"},
		Capabilities: []string{
			"文档切片",
			"片段引用",
			"答案合成",
		},
		Permissions: []string{"file:read"},
		Config: mustSkillConfig(skillConfigPayload{
			Tags:         []string{"rag", "retrieval", "document"},
			Capabilities: []string{"文档切片", "片段引用", "答案合成"},
			Instructions: []string{
				"基于命中的文档片段回答，保留来源文件、页码、标题或片段标识。",
				"区分事实、推断和待确认点；没有命中依据时明确说明不足。",
				"不要用模型常识补全知识库未提供的结论。",
			},
			Inputs:      []string{"文档", "问题", "检索条件"},
			Outputs:     []string{"答案", "引用片段", "不足证据"},
			InstallMode: "marketplace",
		}),
	},
	{
		ID:          "marketplace.form-extractor",
		Name:        "表单票据抽取 Skill",
		Description: "从截图、PDF、HTML 或表格中抽取字段，校验必填项和格式。",
		Version:     "1.0.0",
		SourceURL:   "https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.extractfromfile/",
		Tags:        []string{"document", "extraction", "ocr"},
		Capabilities: []string{
			"字段抽取",
			"格式校验",
			"缺失项检查",
		},
		Permissions: []string{"file:read"},
		Config: mustSkillConfig(skillConfigPayload{
			Tags:         []string{"document", "extraction", "ocr"},
			Capabilities: []string{"字段抽取", "格式校验", "缺失项检查"},
			Instructions: []string{
				"按节点描述中的字段清单抽取值，保留来源片段或坐标线索。",
				"对日期、金额、身份证、邮箱、URL 等格式做确定性校验。",
				"未识别字段必须标记为空并给出原因，不补造值。",
			},
			Inputs:      []string{"文件", "字段清单", "校验规则"},
			Outputs:     []string{"字段 JSON", "校验结果", "缺失项"},
			InstallMode: "marketplace",
		}),
	},
	{
		ID:          "marketplace.image-generation",
		Name:        "图像生成提示词 Skill",
		Description: "把业务数据、参考图和风格要求转成可控图像生成提示词。",
		Version:     "1.0.0",
		SourceURL:   "https://platform.openai.com/docs/guides/images",
		Tags:        []string{"image", "prompt", "creative"},
		Capabilities: []string{
			"提示词结构化",
			"参考图约束",
			"安全检查",
		},
		Permissions: []string{"image:generate"},
		Config: mustSkillConfig(skillConfigPayload{
			Tags:         []string{"image", "prompt", "creative"},
			Capabilities: []string{"提示词结构化", "参考图约束", "安全检查"},
			Instructions: []string{
				"根据上游真实数据和当前节点描述生成图像提示词，不加入无来源的商品或品牌信息。",
				"明确画面主体、构图、风格、尺寸、禁止元素和参考图使用方式。",
				"输出最终提示词、参数建议和风险提示。",
			},
			Inputs:      []string{"业务数据", "参考图", "风格要求"},
			Outputs:     []string{"图像提示词", "参数建议", "风险提示"},
			InstallMode: "marketplace",
		}),
	},
	{
		ID:          "marketplace.video-generation",
		Name:        "视频生成提示词 Skill",
		Description: "把脚本、分镜、参考图和平台规格转成视频生成任务描述。",
		Version:     "1.0.0",
		SourceURL:   "https://docs.x.ai/docs/guides/video-generations",
		Tags:        []string{"video", "prompt", "creative"},
		Capabilities: []string{
			"分镜规划",
			"镜头约束",
			"规格适配",
		},
		Permissions: []string{"video:generate"},
		Config: mustSkillConfig(skillConfigPayload{
			Tags:         []string{"video", "prompt", "creative"},
			Capabilities: []string{"分镜规划", "镜头约束", "规格适配"},
			Instructions: []string{
				"根据显式脚本、参考图和节点描述生成视频任务，不补造未提供的产品卖点。",
				"输出镜头顺序、运动方式、时长、比例、分辨率和负面约束。",
				"如果缺少脚本或素材，先列出缺失项和可执行的最小任务。",
			},
			Inputs:      []string{"脚本", "参考图", "规格"},
			Outputs:     []string{"视频提示词", "分镜", "参数建议"},
			InstallMode: "marketplace",
		}),
	},
}

func mustSkillConfig(payload skillConfigPayload) json.RawMessage {
	raw, err := json.Marshal(payload)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

func listMarketplaceSkills(installed []AgentSkill) []AgentSkillMarketplaceItem {
	installedKeys := map[string]bool{}
	for _, skill := range installed {
		installedKeys[strings.TrimSpace(skill.SourceURL)] = true
		if skill.Config != nil {
			var cfg map[string]any
			if err := json.Unmarshal(skill.Config, &cfg); err == nil {
				if id, ok := cfg["marketplace_id"].(string); ok {
					installedKeys[id] = true
				}
			}
		}
	}
	out := make([]AgentSkillMarketplaceItem, 0, len(marketplaceSkillCatalog))
	for _, item := range marketplaceSkillCatalog {
		copied := item
		copied.Installed = installedKeys[item.ID] || installedKeys[item.SourceURL]
		out = append(out, copied)
	}
	return out
}

func prepareSkillForm(ctx context.Context, f SkillFormData) (SkillFormData, json.RawMessage, json.RawMessage, string, error) {
	source := strings.TrimSpace(f.SourceType)
	if source == "" {
		source = "custom"
	}
	f.SourceType = source

	switch source {
	case "marketplace":
		return prepareMarketplaceSkillForm(f)
	case "github":
		return prepareGitHubSkillForm(ctx, f)
	case "custom":
		return prepareCustomSkillForm(f)
	case "builtin":
		return f, nil, nil, "", errors.New("built-in skills are managed by system")
	default:
		return f, nil, nil, "", fmt.Errorf("unsupported skill source type %q", source)
	}
}

func prepareMarketplaceSkillForm(f SkillFormData) (SkillFormData, json.RawMessage, json.RawMessage, string, error) {
	id := strings.TrimSpace(f.MarketplaceID)
	if id == "" && len(f.Config) > 0 {
		var cfg map[string]any
		if err := json.Unmarshal(f.Config, &cfg); err == nil {
			if raw, ok := cfg["marketplace_id"].(string); ok {
				id = raw
			}
		}
	}
	for _, item := range marketplaceSkillCatalog {
		if item.ID != id {
			continue
		}
		f.Name = item.Name
		f.Description = item.Description
		f.SourceURL = item.SourceURL
		config := mergeSkillConfig(item.Config, map[string]any{
			"marketplace_id": item.ID,
			"install_mode":   "marketplace",
			"source":         item.SourceURL,
		})
		permissions := marshalStringSlice(item.Permissions)
		return f, config, permissions, item.Version, nil
	}
	return f, nil, nil, "", fmt.Errorf("marketplace skill %q not found", id)
}

func prepareGitHubSkillForm(ctx context.Context, f SkillFormData) (SkillFormData, json.RawMessage, json.RawMessage, string, error) {
	sourceURL := strings.TrimSpace(f.SourceURL)
	if sourceURL == "" {
		return f, nil, nil, "", errors.New("GitHub URL is required")
	}
	manifest, entryFile, err := fetchGitHubSkillManifest(ctx, sourceURL)
	if err != nil {
		return f, nil, nil, "", err
	}
	if strings.TrimSpace(f.Name) == "" {
		f.Name = manifest.Name
	}
	if strings.TrimSpace(f.Description) == "" {
		f.Description = manifest.Description
	}
	if strings.TrimSpace(f.Name) == "" {
		f.Name = repoNameFromURL(sourceURL) + " Skill"
	}
	if strings.TrimSpace(f.Description) == "" {
		f.Description = "从 GitHub 仓库导入的 Skill。"
	}
	tags := normalizeTags(append(manifest.Tags, f.Tags...))
	config := mergeSkillConfig(mustSkillConfig(skillConfigPayload{
		Tags:         tags,
		Capabilities: manifest.Capabilities,
		Instructions: manifest.Instructions,
		InstallMode:  "github",
		Source:       sourceURL,
		EntryFile:    entryFile,
	}), map[string]any{
		"github_url": sourceURL,
	})
	version := manifest.Version
	if strings.TrimSpace(version) == "" {
		version = "1.0.0"
	}
	permissions := marshalStringSlice(manifest.Permissions)
	return f, config, permissions, version, nil
}

func prepareCustomSkillForm(f SkillFormData) (SkillFormData, json.RawMessage, json.RawMessage, string, error) {
	if strings.TrimSpace(f.Name) == "" {
		return f, nil, nil, "", errors.New("skill name is required")
	}
	if strings.TrimSpace(f.Description) == "" {
		return f, nil, nil, "", errors.New("skill description is required")
	}
	tags := normalizeTags(append(f.Tags, inferSkillTags(f.Description)...))
	capabilities := inferSkillCapabilities(f.Description)
	instructions := buildCustomSkillInstructions(f.Description, capabilities)
	config := mergeSkillConfig(f.Config, map[string]any{
		"tags":         tags,
		"capabilities": capabilities,
		"instructions": instructions,
		"model_group":  strings.TrimSpace(f.ModelGroup),
		"model_name":   strings.TrimSpace(f.ModelName),
		"install_mode": "custom",
	})
	return f, config, json.RawMessage(`[]`), "1.0.0", nil
}

func mergeSkillConfig(base json.RawMessage, values map[string]any) json.RawMessage {
	cfg := map[string]any{}
	if len(base) > 0 && string(base) != "null" {
		_ = json.Unmarshal(base, &cfg)
	}
	for k, v := range values {
		if v == nil {
			continue
		}
		switch typed := v.(type) {
		case string:
			if strings.TrimSpace(typed) == "" {
				continue
			}
		case []string:
			if len(typed) == 0 {
				continue
			}
		}
		cfg[k] = v
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

func marshalStringSlice(values []string) json.RawMessage {
	raw, err := json.Marshal(uniqueNonEmptyStrings(values))
	if err != nil {
		return json.RawMessage(`[]`)
	}
	return raw
}

func normalizeTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, tag := range tags {
		normalized := strings.ToLower(strings.TrimSpace(tag))
		normalized = strings.ReplaceAll(normalized, " ", "-")
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func inferSkillTags(description string) []string {
	lower := strings.ToLower(description)
	tags := []string{"custom"}
	keywords := map[string]string{
		"浏览器": "browser", "网页": "web", "搜索": "research", "http": "api",
		"api": "api", "接口": "api", "表格": "spreadsheet", "excel": "spreadsheet",
		"csv": "spreadsheet", "文档": "document", "pdf": "document", "图片": "image",
		"视频": "video", "数据": "data", "sql": "database", "数据库": "database",
		"通知": "notification", "webhook": "webhook", "日报": "report", "报告": "report",
		"流程": "workflow", "条件": "workflow", "分支": "workflow", "循环": "workflow",
	}
	for keyword, tag := range keywords {
		if strings.Contains(lower, strings.ToLower(keyword)) {
			tags = append(tags, tag)
		}
	}
	return tags
}

func inferSkillCapabilities(description string) []string {
	tagSet := map[string]bool{}
	for _, tag := range inferSkillTags(description) {
		tagSet[tag] = true
	}
	capabilities := []string{"任务拆解", "结构化输出", "结果校验"}
	if tagSet["browser"] || tagSet["web"] {
		capabilities = append(capabilities, "网页读取", "来源核验")
	}
	if tagSet["api"] {
		capabilities = append(capabilities, "接口调用", "响应解析")
	}
	if tagSet["spreadsheet"] || tagSet["data"] {
		capabilities = append(capabilities, "数据清洗", "表格生成")
	}
	if tagSet["document"] || tagSet["report"] {
		capabilities = append(capabilities, "模板遵循", "文档生成")
	}
	if tagSet["notification"] || tagSet["webhook"] {
		capabilities = append(capabilities, "消息投递", "失败重试")
	}
	if tagSet["workflow"] {
		capabilities = append(capabilities, "流程控制", "分支循环")
	}
	return uniqueNonEmptyStrings(capabilities)
}

func buildCustomSkillInstructions(description string, capabilities []string) []string {
	return []string{
		"只根据当前节点输入、上游显式输出和 Skill 描述执行，不扩大业务上下文。",
		"Skill 描述：" + strings.TrimSpace(description),
		"核心能力：" + strings.Join(capabilities, "、"),
		"输出必须包含执行结果、关键依据和无法完成时的缺失输入。",
	}
}

func fetchGitHubSkillManifest(ctx context.Context, repoURL string) (skillManifestPayload, string, error) {
	owner, repo, branch, subdir, err := parseGitHubRepoURL(repoURL)
	if err != nil {
		return skillManifestPayload{}, "", err
	}
	branches := uniqueNonEmptyStrings([]string{branch, "main", "master"})
	candidates := []string{
		"skill.json",
		"SKILL.json",
		"skill.jsonc",
		"SKILL.md",
		".codex/skill.json",
		".codex/SKILL.md",
	}
	var lastErr error
	for _, b := range branches {
		for _, candidate := range candidates {
			filePath := path.Join(subdir, candidate)
			rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", owner, repo, b, filePath)
			body, err := fetchLimitedText(ctx, rawURL)
			if err != nil {
				lastErr = err
				continue
			}
			manifest, err := parseSkillManifest(candidate, body)
			if err != nil {
				lastErr = err
				continue
			}
			return manifest, filePath, nil
		}
	}
	if lastErr != nil {
		return skillManifestPayload{}, "", fmt.Errorf("failed to read skill manifest: %w", lastErr)
	}
	return skillManifestPayload{}, "", errors.New("repository does not contain skill.json or SKILL.md")
}

func parseGitHubRepoURL(raw string) (owner, repo, branch, subdir string, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "", "", "", "", errors.New("invalid GitHub URL")
	}
	host := strings.ToLower(u.Host)
	if host != "github.com" && host != "www.github.com" {
		return "", "", "", "", errors.New("only github.com repositories are supported")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", "", "", "", errors.New("GitHub URL must include owner and repository")
	}
	owner = parts[0]
	repo = strings.TrimSuffix(parts[1], ".git")
	branch = ""
	if len(parts) >= 4 && (parts[2] == "tree" || parts[2] == "blob") {
		branch = parts[3]
		if len(parts) > 4 {
			subdir = path.Join(parts[4:]...)
		}
	}
	return owner, repo, branch, subdir, nil
}

func fetchLimitedText(ctx context.Context, rawURL string) ([]byte, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.raw, text/plain, application/json")
	req.Header.Set("User-Agent", "SunAPI-Skill-Installer")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned %d", rawURL, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxSkillManifestBytes))
}

func parseSkillManifest(filename string, body []byte) (skillManifestPayload, error) {
	if strings.HasSuffix(strings.ToLower(filename), ".md") {
		return parseSkillMarkdown(body)
	}
	var manifest skillManifestPayload
	if err := json.Unmarshal(body, &manifest); err != nil {
		return manifest, err
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return manifest, errors.New("skill manifest missing name")
	}
	if len(manifest.Capabilities) == 0 && len(manifest.Config.Capabilities) > 0 {
		manifest.Capabilities = manifest.Config.Capabilities
	}
	if len(manifest.Instructions) == 0 && len(manifest.Config.Instructions) > 0 {
		manifest.Instructions = manifest.Config.Instructions
	}
	return manifest, nil
}

func parseSkillMarkdown(body []byte) (skillManifestPayload, error) {
	text := string(body)
	manifest := skillManifestPayload{}
	if strings.HasPrefix(text, "---") {
		parts := strings.SplitN(text, "---", 3)
		if len(parts) >= 3 {
			for _, line := range strings.Split(parts[1], "\n") {
				key, value, ok := strings.Cut(line, ":")
				if !ok {
					continue
				}
				value = strings.Trim(strings.TrimSpace(value), `"'`)
				switch strings.TrimSpace(strings.ToLower(key)) {
				case "name":
					manifest.Name = value
				case "description":
					manifest.Description = value
				case "version":
					manifest.Version = value
				case "tags":
					manifest.Tags = splitManifestList(value)
				}
			}
			text = parts[2]
		}
	}
	if strings.TrimSpace(manifest.Name) == "" {
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "# ") {
				manifest.Name = strings.TrimSpace(strings.TrimPrefix(line, "# "))
				break
			}
		}
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return manifest, errors.New("SKILL.md missing name heading or frontmatter")
	}
	if strings.TrimSpace(manifest.Description) == "" {
		manifest.Description = firstParagraph(text)
	}
	manifest.Instructions = []string{strings.TrimSpace(text)}
	manifest.Capabilities = inferSkillCapabilities(manifest.Description + "\n" + manifest.Name)
	manifest.Tags = normalizeTags(append(manifest.Tags, inferSkillTags(manifest.Description+"\n"+manifest.Name)...))
	return manifest, nil
}

func splitManifestList(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "[]")
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(strings.TrimSpace(part), `"'`)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func firstParagraph(text string) string {
	lines := strings.Split(text, "\n")
	var paragraph []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "---") {
			if len(paragraph) > 0 {
				break
			}
			continue
		}
		paragraph = append(paragraph, line)
		if len(strings.Join(paragraph, " ")) > 160 {
			break
		}
	}
	return strings.TrimSpace(strings.Join(paragraph, " "))
}

func repoNameFromURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "GitHub"
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 {
		return strings.TrimSuffix(parts[1], ".git")
	}
	return "GitHub"
}

var nonWordForSkillID = regexp.MustCompile(`[^a-z0-9]+`)

func stableSkillInstallKey(sourceType, sourceURL, name string) string {
	base := strings.ToLower(strings.TrimSpace(sourceURL))
	if base == "" {
		base = strings.ToLower(strings.TrimSpace(name))
	}
	base = nonWordForSkillID.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if len(base) > 52 {
		base = base[:52]
		base = strings.Trim(base, "-")
	}
	if base == "" {
		base = "skill"
	}
	return "sk-" + strings.TrimSpace(sourceType) + "-" + base
}
