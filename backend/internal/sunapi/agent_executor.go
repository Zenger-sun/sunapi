package sunapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	agentArchiveDirName      = "agent-archives"
	agentBlueprintRunTimeout = 10 * time.Minute
)

// ── 蓝图执行入口（异步 goroutine 调用）────────────────────────────────────────

func runBlueprintTask(store *Store, bp *AgentBlueprint) {
	ctx, cancel := context.WithTimeout(context.Background(), agentBlueprintRunTimeout)
	defer cancel()

	startedAt := time.Now()

	// 创建执行日志
	execLog, err := store.CreateExecutionLog(ctx, bp.ID, bp.AgentID, bp.UserID)
	if err != nil {
		log.WithError(err).WithField("blueprint_id", bp.ID).
			Warn("agent: failed to create execution log")
		return
	}

	log.WithFields(log.Fields{
		"blueprint_id": bp.ID,
		"agent_id":     bp.AgentID,
		"model":        bp.ModelName,
	}).Info("agent: blueprint execution started")

	// 执行蓝图核心逻辑
	output, archivePath, execErr := executeBlueprintCore(ctx, store, bp)

	// 计算耗时
	durationMs := time.Since(startedAt).Milliseconds()

	// 更新蓝图状态
	finalStatus := "completed"
	failureReason := ""
	if execErr != nil {
		finalStatus = "failed"
		failureReason = execErr.Error()
		log.WithError(execErr).WithField("blueprint_id", bp.ID).
			Warn("agent: blueprint execution failed")
	}

	// 更新蓝图执行次数和时间
	now := time.Now().UnixMilli()
	_ = store.UpdateBlueprintStatus(ctx, bp.ID, finalStatus, failureReason)
	_, _ = store.db.ExecContext(ctx, `
		UPDATE agent_blueprints
		SET execution_count = execution_count + 1,
		    last_executed_time = ?,
		    execution_duration_ms = ?,
		    updated_time = ?
		WHERE id = ?`,
		now, durationMs, now, bp.ID)

	// 更新智能体的最后执行时间
	_, _ = store.db.ExecContext(ctx, `
		UPDATE agents SET last_executed_time = ?, updated_time = ?
		WHERE id = ?`, now, now, bp.AgentID)

	// 完成执行日志
	_ = store.FinishExecutionLog(ctx, execLog.ID, finalStatus, failureReason, archivePath, output)

	log.WithFields(log.Fields{
		"blueprint_id": bp.ID,
		"status":       finalStatus,
		"duration_ms":  durationMs,
	}).Info("agent: blueprint execution finished")
}

// ── 核心执行逻辑 ──────────────────────────────────────────────────────────────

func executeBlueprintCore(ctx context.Context, store *Store, bp *AgentBlueprint) (json.RawMessage, string, error) {
	// 1. 解析归档配置
	var archiveCfg map[string]interface{}
	if len(bp.ArchiveConfig) > 0 {
		_ = json.Unmarshal(bp.ArchiveConfig, &archiveCfg)
	}

	// 2. 解析输入变量
	var inputVars map[string]interface{}
	if len(bp.InputVariables) > 0 {
		_ = json.Unmarshal(bp.InputVariables, &inputVars)
	}

	// 4. 调用 LLM 执行任务
	result, err := callLLMForBlueprint(ctx, store, bp, inputVars)
	if err != nil {
		return nil, "", fmt.Errorf("llm execution failed: %w", err)
	}

	// 5. 构建输出
	outputData := map[string]interface{}{
		"blueprint_id": bp.ID,
		"model":        bp.ModelName,
		"model_group":  bp.ModelGroup,
		"input_vars":   inputVars,
		"result":       result,
		"executed_at":  time.Now().UnixMilli(),
	}
	outputJSON, _ := json.Marshal(outputData)

	// 6. 归档输出（写入 dataDir/agent-archives/YYYYMMDD/）
	archivePath := ""
	archiveType := ""
	if v, ok := archiveCfg["type"].(string); ok {
		archiveType = v
	}
	// 默认或 platform 类型归档到本地
	if archiveType == "" || archiveType == "platform" {
		path, err := persistAgentArchive(store.DataDir(), bp, outputJSON)
		if err != nil {
			log.WithError(err).WithField("blueprint_id", bp.ID).
				Warn("agent: archive write failed, continuing without archive")
		} else {
			archivePath = path
		}
	}

	return json.RawMessage(outputJSON), archivePath, nil
}

// ── LLM 调用封装 ──────────────────────────────────────────────────────────────

// callLLMForBlueprint 通过本地 OpenAI 兼容接口调用模型执行任务。
// 它直接复用服务自身的 /v1/chat/completions 接口，避免重复实现认证逻辑。
func callLLMForBlueprint(ctx context.Context, store *Store, bp *AgentBlueprint, inputVars map[string]interface{}) (string, error) {
	templateContext := buildBlueprintTemplatePromptContext(store, inputVars)
	skillContext := buildBlueprintSkillPromptContext(bp)

	// 构建系统 Prompt
	systemPrompt := buildBlueprintSystemPrompt(bp, inputVars, templateContext, skillContext)

	// 构建请求体
	type chatMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	reqBody := map[string]interface{}{
		"model": bp.ModelName,
		"messages": []chatMsg{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: buildBlueprintUserPrompt(bp, inputVars)},
		},
		"stream": false,
	}
	if bp.ModelGroup != "" {
		reqBody["group"] = bp.ModelGroup
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// 获取服务自身配置中的管理员 API Key
	apiKey, err := store.getInternalAPIKey(ctx)
	if err != nil {
		return "", fmt.Errorf("no available api key: %w", err)
	}

	// 读取服务监听地址
	settings, err := store.Settings(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to read settings: %w", err)
	}
	host := settings.ListenHost
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	port := settings.ListenPort
	if port <= 0 {
		port = 3000
	}
	baseURL := fmt.Sprintf("http://%s:%d", host, port)

	// 发送请求（带超时）
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		baseURL+"/v1/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm api returned %d: %s", resp.StatusCode, string(respBody))
	}

	// 解析响应内容
	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("empty response from model")
	}
	return chatResp.Choices[0].Message.Content, nil
}

// getInternalAPIKey 获取一个可用的内部 API Key（状态为启用且额度足够）
func (s *Store) getInternalAPIKey(ctx context.Context) (string, error) {
	var key string
	err := s.db.QueryRowContext(ctx, `
		SELECT token_key FROM api_keys
		WHERE status = 1 AND (unlimited_quota = 1 OR remain_quota > 0)
		ORDER BY id LIMIT 1`).Scan(&key)
	if err != nil {
		return "", err
	}
	return key, nil
}

// ── Prompt 构建 ───────────────────────────────────────────────────────────────

func buildBlueprintSystemPrompt(bp *AgentBlueprint, inputVars map[string]interface{}, templateContext string, skillContext string) string {
	var sb strings.Builder
	sb.WriteString("你是一个自动化任务中的独立功能模块。\n")
	sb.WriteString("只根据显式输入变量和当前任务描述生成结果，不要从蓝图名称、历史示例、默认模板或其他外部上下文补充业务背景。\n")
	sb.WriteString("如果显式输入中包含当前上传模板文件，请只按该模板文件、上一节点数据和当前任务描述控制格式、标题、语气和输出结构。\n")
	sb.WriteString("如果输入不足，请说明缺少哪些输入，不要自行假设行业、平台、商品或经营场景。\n")
	if strings.TrimSpace(skillContext) != "" {
		sb.WriteString(skillContext)
	}
	if bp.Description != "" {
		sb.WriteString("当前任务描述：" + bp.Description + "\n")
	}
	if len(inputVars) > 0 {
		sb.WriteString("显式输入变量：\n")
		for k, v := range inputVars {
			sb.WriteString(fmt.Sprintf("  - %s: %v\n", k, v))
		}
	}
	if strings.TrimSpace(templateContext) != "" {
		sb.WriteString(templateContext)
	}
	sb.WriteString("\n请严格基于以上内容完成当前模块输出。")
	return sb.String()
}

func buildBlueprintUserPrompt(bp *AgentBlueprint, inputVars map[string]interface{}) string {
	if bp.Description != "" {
		return bp.Description
	}
	return "请根据显式输入执行当前模块任务；若缺少任务描述，请返回需要补充的输入。"
}

type builtinAgentSkillDefinition struct {
	Label        string
	Description  string
	Capabilities []string
	Instructions []string
}

var builtinAgentSkillOrder = []string{
	"skill.web-research",
	"skill.file-document",
	"skill.browser-automation",
	"skill.code-interpreter",
	"skill.api-function",
	"skill.rag-file-search",
	"skill.database-sql",
	"skill.mcp-workflow",
}

var builtinAgentSkillDefinitions = map[string]builtinAgentSkillDefinition{
	"skill.web-research": {
		Label:        "Web Research Skill",
		Description:  "面向检索、网页阅读和事实核验的通用研究能力，适合多数资料收集任务。",
		Capabilities: []string{"关键词规划", "来源交叉验证", "引用摘要"},
		Instructions: []string{
			"先把任务拆成 2-5 个可检索问题，再围绕当前节点输入进行查找和摘要。",
			"优先使用权威、官方、近期或一手来源；多个来源冲突时明确说明差异。",
			"输出结论、依据和不确定点，不要把未验证内容当作事实。",
		},
	},
	"skill.file-document": {
		Label:        "File & Document Skill",
		Description:  "读取和整理 Excel、CSV、PDF、Word、Markdown、HTML、TXT 等常见文件。",
		Capabilities: []string{"多格式解析", "模板遵循", "结构保真"},
		Instructions: []string{
			"只使用当前上传或上游显式提供的文件内容，不猜测未提供的表格、标题或业务背景。",
			"保留文件中的关键字段、工作表、表头、段落层级和格式要求。",
			"输出格式必须跟模板或节点配置一致；无法读取时说明具体缺失文件或格式限制。",
		},
	},
	"skill.browser-automation": {
		Label:        "Browser Automation Skill",
		Description:  "面向登录态页面、表单、动态网页和截图证据的浏览器自动化能力。",
		Capabilities: []string{"页面导航", "DOM 抽取", "操作安全"},
		Instructions: []string{
			"按节点给出的 URL、登录态和任务描述执行导航、读取、截图和字段抽取。",
			"遇到弹窗、登录、分页、懒加载或动态内容时记录处理步骤和剩余限制。",
			"默认只读；购买、删除、提交、发布、转账等高风险动作必须停下并等待确认。",
		},
	},
	"skill.code-interpreter": {
		Label:        "Code Interpreter Skill",
		Description:  "用于数据清洗、计算、图表、文件转换和可复现脚本化分析。",
		Capabilities: []string{"数据清洗", "计算分析", "文件生成"},
		Instructions: []string{
			"优先用确定性的表格/脚本处理计算、清洗、去重、排序、转换和统计任务。",
			"输出关键步骤、计算口径、生成文件路径或可复用结果，避免只给泛泛建议。",
			"如果数据不足或格式不一致，先列出阻塞字段和建议修复方式。",
		},
	},
	"skill.api-function": {
		Label:        "API & Function Skill",
		Description:  "组织 HTTP/API 调用、函数参数、重试策略和结构化响应解析。",
		Capabilities: []string{"参数校验", "JSON 解析", "错误恢复"},
		Instructions: []string{
			"根据节点中的 endpoint、method、headers、body、鉴权和超时配置组织请求。",
			"把响应解析成稳定 JSON、表格或日志摘要，保留状态码、错误信息和关键字段。",
			"遇到分页、限流、空响应或非 JSON 响应时说明处理策略，不伪造结果。",
		},
	},
	"skill.rag-file-search": {
		Label:        "RAG & File Search Skill",
		Description:  "面向知识库、文档检索、片段引用和问答总结的常用检索增强能力。",
		Capabilities: []string{"语义检索", "片段引用", "答案合成"},
		Instructions: []string{
			"围绕问题检索最相关的文档片段，并基于片段回答，不使用未命中的外部背景补全。",
			"保留来源文件、页码、段落或片段标识，区分事实、推断和待确认内容。",
			"当检索结果不足时，明确说明没有足够依据，而不是生成看似完整的答案。",
		},
	},
	"skill.database-sql": {
		Label:        "Database & SQL Skill",
		Description:  "面向结构化数据查询、SQL 生成、指标口径和结果解释的能力。",
		Capabilities: []string{"Schema 理解", "SQL 生成", "结果校验"},
		Instructions: []string{
			"先确认表结构、字段含义、过滤条件和时间范围，再生成查询或解释结果。",
			"优先使用只读查询；避免破坏性 SQL，涉及写入、删除、更新时必须请求确认。",
			"输出 SQL、参数、结果摘要和可能的数据质量风险。",
		},
	},
	"skill.mcp-workflow": {
		Label:        "MCP & Workflow Skill",
		Description:  "适配 MCP/GitHub/文件系统等外部工具编排，强调权限边界和可追踪步骤。",
		Capabilities: []string{"工具选择", "权限边界", "步骤追踪"},
		Instructions: []string{
			"根据任务选择最少必要工具，说明每一步输入、输出和失败补偿方案。",
			"严格遵守工具权限边界，不越权读取、写入或调用未配置的外部系统。",
			"对 GitHub、文件系统、任务管理等工具操作保留可审计摘要。",
		},
	},
}

var legacyBuiltinAgentSkillAliases = map[string]string{
	"skill.system-orchestration": "skill.mcp-workflow",
	"skill.schedule-guard":       "skill.mcp-workflow",
	"skill.browser-reading":      "skill.browser-automation",
	"skill.network-fetch":        "skill.api-function",
	"skill.data-cleaning":        "skill.code-interpreter",
	"skill.image-generation":     "skill.code-interpreter",
	"skill.report-writing":       "skill.file-document",
	"skill.notification-routing": "skill.mcp-workflow",
}

func builtinAgentSkillsForUser(userID int64) []AgentSkill {
	out := make([]AgentSkill, 0, len(builtinAgentSkillOrder))
	for _, id := range builtinAgentSkillOrder {
		def, ok := builtinAgentSkillDefinitions[id]
		if !ok {
			continue
		}
		config, err := json.Marshal(map[string]interface{}{
			"capabilities": def.Capabilities,
			"instructions": def.Instructions,
		})
		if err != nil {
			config = []byte(`{}`)
		}
		out = append(out, AgentSkill{
			ID:          id,
			UserID:      userID,
			Name:        def.Label,
			Description: def.Description,
			Version:     "1.0.0",
			Source:      "builtin",
			Enabled:     true,
			Config:      json.RawMessage(config),
			Permissions: json.RawMessage(`[]`),
			CreatedTime: 0,
			UpdatedTime: 0,
		})
	}
	return out
}

func mergeBuiltinAgentSkills(userID int64, stored []AgentSkill) []AgentSkill {
	builtins := builtinAgentSkillsForUser(userID)
	out := make([]AgentSkill, 0, len(builtins)+len(stored))
	seen := make(map[string]bool, len(builtins)+len(stored))
	for _, skill := range builtins {
		seen[skill.ID] = true
		out = append(out, skill)
	}
	for _, skill := range stored {
		if seen[skill.ID] {
			continue
		}
		seen[skill.ID] = true
		out = append(out, skill)
	}
	return out
}

func isBuiltinAgentSkillID(id string) bool {
	normalized := normalizeBuiltinAgentSkillID(id)
	_, ok := builtinAgentSkillDefinitions[normalized]
	return ok
}

func normalizeBuiltinAgentSkillID(id string) string {
	id = strings.TrimSpace(id)
	if replacement, ok := legacyBuiltinAgentSkillAliases[id]; ok {
		return replacement
	}
	return id
}

func buildBlueprintSkillPromptContext(bp *AgentBlueprint) string {
	skillIDs := extractBlueprintSkillIDs(bp)
	if len(skillIDs) == 0 {
		return ""
	}

	var sb strings.Builder
	written := 0
	for _, skillID := range skillIDs {
		normalizedSkillID := normalizeBuiltinAgentSkillID(skillID)
		if def, ok := builtinAgentSkillDefinitions[normalizedSkillID]; ok {
			if written == 0 {
				sb.WriteString("启用 Skill 约束：\n")
			}
			sb.WriteString(fmt.Sprintf("- %s：%s\n", def.Label, strings.Join(def.Capabilities, "、")))
			for _, instruction := range def.Instructions {
				sb.WriteString("  * " + instruction + "\n")
			}
			written++
			continue
		}
		if strings.TrimSpace(skillID) != "" {
			if written == 0 {
				sb.WriteString("启用 Skill 约束：\n")
			}
			sb.WriteString(fmt.Sprintf("- 自定义 Skill %s：仅作为当前模块的补充执行约束，不允许扩展未显式提供的业务上下文。\n", skillID))
			written++
		}
	}
	if written == 0 {
		return ""
	}
	return sb.String()
}

func extractBlueprintSkillIDs(bp *AgentBlueprint) []string {
	if bp == nil || len(bp.Skills) == 0 || string(bp.Skills) == "null" {
		return nil
	}

	var ids []string
	if err := json.Unmarshal(bp.Skills, &ids); err == nil {
		return uniqueNonEmptyStrings(ids)
	}

	var objects []struct {
		Value string `json:"value"`
		ID    string `json:"id"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(bp.Skills, &objects); err == nil {
		values := make([]string, 0, len(objects))
		for _, object := range objects {
			switch {
			case strings.TrimSpace(object.Value) != "":
				values = append(values, object.Value)
			case strings.TrimSpace(object.ID) != "":
				values = append(values, object.ID)
			case strings.TrimSpace(object.Name) != "":
				values = append(values, object.Name)
			}
		}
		return uniqueNonEmptyStrings(values)
	}
	return nil
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

type agentTemplateInputRef struct {
	KeyPath string
	Path    string
}

func buildBlueprintTemplatePromptContext(store *Store, inputVars map[string]interface{}) string {
	if store == nil || len(inputVars) == 0 || strings.TrimSpace(store.DataDir()) == "" {
		return ""
	}

	refs := collectAgentTemplateInputRefs(inputVars)
	if len(refs) == 0 {
		return ""
	}

	var sb strings.Builder
	written := 0
	for _, ref := range refs {
		if written >= 4 {
			break
		}
		absPath, err := agentUploadedTemplateAbsPath(store.DataDir(), ref.Path)
		if err != nil {
			continue
		}
		preview, err := agentTemplateFilePreview(absPath, filepath.Base(absPath))
		if err != nil {
			preview = ""
		}
		if written == 0 {
			sb.WriteString("\n当前上传模板文件（显式输入，仅用于当前模块）:\n")
		}
		sb.WriteString(fmt.Sprintf("- %s: %s\n", ref.KeyPath, ref.Path))
		if preview != "" {
			sb.WriteString("  可读模板预览:\n")
			sb.WriteString(indentAgentTemplatePreview(preview))
		}
		written += 1
	}

	if written == 0 {
		return ""
	}
	return sb.String()
}

func collectAgentTemplateInputRefs(inputVars map[string]interface{}) []agentTemplateInputRef {
	refs := make([]agentTemplateInputRef, 0, 4)
	collectAgentTemplateInputRefsFromValue(inputVars, "", &refs, 0)
	return refs
}

func collectAgentTemplateInputRefsFromValue(value interface{}, path string, refs *[]agentTemplateInputRef, depth int) {
	if depth > 8 || len(*refs) >= 8 {
		return
	}

	switch typed := value.(type) {
	case map[string]interface{}:
		for key, child := range typed {
			nextPath := key
			if path != "" {
				nextPath = path + "." + key
			}
			if isAgentTemplateInputKey(key) {
				if templatePath, ok := child.(string); ok {
					displayPaths := extractAgentTemplateDisplayPaths(templatePath)
					for index, displayPath := range displayPaths {
						keyPath := nextPath
						if len(displayPaths) > 1 {
							keyPath = fmt.Sprintf("%s[%d]", nextPath, index)
						}
						*refs = append(*refs, agentTemplateInputRef{
							KeyPath: keyPath,
							Path:    displayPath,
						})
					}
				}
				if templateMap, ok := child.(map[string]interface{}); ok {
					if templatePath, ok := templateMap["path"].(string); ok && isAgentUploadedTemplateDisplayPath(templatePath) {
						*refs = append(*refs, agentTemplateInputRef{
							KeyPath: nextPath,
							Path:    strings.TrimSpace(filepath.ToSlash(templatePath)),
						})
					}
				}
			}
			collectAgentTemplateInputRefsFromValue(child, nextPath, refs, depth+1)
		}
	case []interface{}:
		for index, child := range typed {
			collectAgentTemplateInputRefsFromValue(
				child,
				fmt.Sprintf("%s[%d]", path, index),
				refs,
				depth+1,
			)
		}
	}
}

func extractAgentTemplateDisplayPaths(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if isAgentUploadedTemplateDisplayPath(value) {
		return []string{strings.Trim(strings.TrimSpace(filepath.ToSlash(value)), "/")}
	}

	var bindings []struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(value), &bindings); err == nil && len(bindings) > 0 {
		paths := make([]string, 0, len(bindings))
		for _, binding := range bindings {
			if isAgentUploadedTemplateDisplayPath(binding.Path) {
				paths = append(paths, strings.Trim(strings.TrimSpace(filepath.ToSlash(binding.Path)), "/"))
			}
		}
		return paths
	}

	var pathList []string
	if err := json.Unmarshal([]byte(value), &pathList); err == nil && len(pathList) > 0 {
		paths := make([]string, 0, len(pathList))
		for _, path := range pathList {
			if isAgentUploadedTemplateDisplayPath(path) {
				paths = append(paths, strings.Trim(strings.TrimSpace(filepath.ToSlash(path)), "/"))
			}
		}
		return paths
	}
	return nil
}

func isAgentTemplateInputKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "模板文件",
		"网页模板",
		"整理模板",
		"当前模板文件",
		"上传模板文件",
		"template_file",
		"template_path",
		"templatefile",
		"templatepath",
		"current_template",
		"uploaded_template":
		return true
	default:
		return false
	}
}

func isAgentUploadedTemplateDisplayPath(value string) bool {
	value = strings.Trim(strings.TrimSpace(filepath.ToSlash(value)), "/")
	return strings.HasPrefix(value, "data/templates/")
}

func indentAgentTemplatePreview(preview string) string {
	lines := strings.Split(preview, "\n")
	if len(lines) > 30 {
		lines = lines[:30]
	}

	var sb strings.Builder
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sb.WriteString("    ")
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// ── 归档存储 ──────────────────────────────────────────────────────────────────

// persistAgentArchive 将执行结果写入 <dataDir>/agent-archives/YYYYMMDD/<id>.json
// 返回可通过 /api/agents/archives/files/ 访问的 URL 路径
func persistAgentArchive(dataDir string, bp *AgentBlueprint, content []byte) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", nil
	}

	dateDir := time.Now().Format("20060102")
	dir := filepath.Join(dataDir, agentArchiveDirName, dateDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	// 文件名：<blueprintID>-<timestamp>.json
	ts := time.Now().UnixMilli()
	filename := fmt.Sprintf("%s-%d.json", safeImageHistorySegment(bp.ID), ts)
	fullPath := filepath.Join(dir, filename)

	if err := os.WriteFile(fullPath, content, 0600); err != nil {
		return "", err
	}

	// 返回 URL 路径（与 image-history 保持一致的风格）
	rel := filepath.ToSlash(filepath.Join(agentArchiveDirName, dateDir, filename))
	return "/api/agents/archives/files/" + rel, nil
}

// agentArchiveFilePath 安全地将 URL 相对路径映射到本地文件路径（防路径穿越）
func agentArchiveFilePath(dataDir, relPath string) (string, error) {
	relPath = strings.TrimPrefix(strings.TrimSpace(relPath), "/")
	relPath = filepath.Clean(filepath.FromSlash(relPath))

	if relPath == "." || filepath.IsAbs(relPath) ||
		strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || relPath == ".." {
		return "", errNotFound("invalid archive path")
	}

	root := filepath.Clean(filepath.Join(dataDir, agentArchiveDirName))
	full := filepath.Clean(filepath.Join(dataDir, relPath))

	if full != root && !strings.HasPrefix(full, root+string(filepath.Separator)) {
		return "", errNotFound("invalid archive path")
	}

	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errNotFound("invalid archive path")
	}
	return full, nil
}
