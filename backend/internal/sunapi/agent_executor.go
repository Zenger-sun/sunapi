package sunapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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

	if hasBlueprintNodeGraph(bp) {
		return executeBlueprintNodeGraph(ctx, store, bp, inputVars, archiveCfg)
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

type agentBlueprintGraphNode struct {
	ID   string `json:"id"`
	Data struct {
		Kind        string            `json:"kind"`
		ComponentID string            `json:"componentId"`
		Title       string            `json:"title"`
		Description string            `json:"description"`
		Group       string            `json:"group"`
		Model       string            `json:"model"`
		Skill       string            `json:"skill"`
		Inputs      []string          `json:"inputs"`
		Outputs     []string          `json:"outputs"`
		Settings    map[string]string `json:"settings"`
	} `json:"data"`
}

type agentBlueprintGraphEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type agentBlueprintNodeResult struct {
	NodeID      string                 `json:"node_id"`
	Kind        string                 `json:"kind"`
	Title       string                 `json:"title"`
	Status      string                 `json:"status"`
	Message     string                 `json:"message"`
	Input       map[string]interface{} `json:"input,omitempty"`
	Output      map[string]interface{} `json:"output,omitempty"`
	StartedAt   int64                  `json:"started_at"`
	EndedAt     int64                  `json:"ended_at"`
	DurationMS  int64                  `json:"duration_ms"`
	Error       string                 `json:"error,omitempty"`
	ArchivePath string                 `json:"archive_path,omitempty"`
}

type agentBlueprintGraphExecution struct {
	BlueprintID string                     `json:"blueprint_id"`
	Mode        string                     `json:"mode"`
	Model       string                     `json:"model"`
	ModelGroup  string                     `json:"model_group"`
	InputVars   map[string]interface{}     `json:"input_vars"`
	Nodes       []agentBlueprintNodeResult `json:"nodes"`
	FinalOutput map[string]interface{}     `json:"final_output,omitempty"`
	ExecutedAt  int64                      `json:"executed_at"`
	Warnings    []string                   `json:"warnings,omitempty"`
}

func hasBlueprintNodeGraph(bp *AgentBlueprint) bool {
	nodes, err := parseAgentBlueprintNodes(bp.Nodes)
	return err == nil && len(nodes) > 0
}

func executeBlueprintNodeGraph(
	ctx context.Context,
	store *Store,
	bp *AgentBlueprint,
	inputVars map[string]interface{},
	archiveCfg map[string]interface{},
) (json.RawMessage, string, error) {
	nodes, err := parseAgentBlueprintNodes(bp.Nodes)
	if err != nil {
		return nil, "", err
	}
	edges, err := parseAgentBlueprintEdges(bp.Edges)
	if err != nil {
		return nil, "", err
	}
	ordered := orderAgentBlueprintNodes(nodes, edges)
	results := make([]agentBlueprintNodeResult, 0, len(ordered))
	outputsByNodeID := make(map[string]map[string]interface{}, len(ordered))
	var warnings []string

	for _, node := range ordered {
		upstream := upstreamAgentNodeOutputs(node.ID, edges, outputsByNodeID)
		result := executeAgentBlueprintNode(ctx, store, bp, node, upstream, inputVars)
		results = append(results, result)
		if result.Status == "failed" {
			warnings = append(warnings, fmt.Sprintf("%s: %s", node.ID, result.Error))
		}
		outputsByNodeID[node.ID] = result.Output
	}

	finalOutput := map[string]interface{}{}
	if len(results) > 0 {
		finalOutput = results[len(results)-1].Output
	}
	outputData := agentBlueprintGraphExecution{
		BlueprintID: bp.ID,
		Mode:        "node_graph",
		Model:       bp.ModelName,
		ModelGroup:  bp.ModelGroup,
		InputVars:   inputVars,
		Nodes:       results,
		FinalOutput: finalOutput,
		ExecutedAt:  time.Now().UnixMilli(),
		Warnings:    warnings,
	}
	outputJSON, _ := json.Marshal(outputData)

	archivePath := ""
	archiveType := ""
	if v, ok := archiveCfg["type"].(string); ok {
		archiveType = v
	}
	if archiveType == "" || archiveType == "platform" {
		path, err := persistAgentArchive(store.DataDir(), bp, outputJSON)
		if err != nil {
			log.WithError(err).WithField("blueprint_id", bp.ID).
				Warn("agent: node graph archive write failed, continuing")
		} else {
			archivePath = path
		}
	}

	return json.RawMessage(outputJSON), archivePath, nil
}

func parseAgentBlueprintNodes(raw json.RawMessage) ([]agentBlueprintGraphNode, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil, nil
	}
	var nodes []agentBlueprintGraphNode
	if err := json.Unmarshal(raw, &nodes); err != nil {
		return nil, fmt.Errorf("invalid blueprint nodes: %w", err)
	}
	out := make([]agentBlueprintGraphNode, 0, len(nodes))
	for _, node := range nodes {
		node.ID = strings.TrimSpace(node.ID)
		node.Data.Kind = strings.TrimSpace(node.Data.Kind)
		if node.ID == "" || node.Data.Kind == "" {
			continue
		}
		if node.Data.Settings == nil {
			node.Data.Settings = map[string]string{}
		}
		out = append(out, node)
	}
	return out, nil
}

func parseAgentBlueprintEdges(raw json.RawMessage) ([]agentBlueprintGraphEdge, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil, nil
	}
	var edges []agentBlueprintGraphEdge
	if err := json.Unmarshal(raw, &edges); err != nil {
		return nil, fmt.Errorf("invalid blueprint edges: %w", err)
	}
	out := make([]agentBlueprintGraphEdge, 0, len(edges))
	for _, edge := range edges {
		edge.Source = strings.TrimSpace(edge.Source)
		edge.Target = strings.TrimSpace(edge.Target)
		if edge.Source == "" || edge.Target == "" {
			continue
		}
		out = append(out, edge)
	}
	return out, nil
}

func orderAgentBlueprintNodes(nodes []agentBlueprintGraphNode, edges []agentBlueprintGraphEdge) []agentBlueprintGraphNode {
	byID := make(map[string]agentBlueprintGraphNode, len(nodes))
	incoming := make(map[string]int, len(nodes))
	outgoing := make(map[string][]string, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
		incoming[node.ID] = 0
	}
	for _, edge := range edges {
		if _, ok := byID[edge.Source]; !ok {
			continue
		}
		if _, ok := byID[edge.Target]; !ok {
			continue
		}
		incoming[edge.Target]++
		outgoing[edge.Source] = append(outgoing[edge.Source], edge.Target)
	}
	queue := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if incoming[node.ID] == 0 {
			queue = append(queue, node.ID)
		}
	}
	sort.SliceStable(queue, func(i, j int) bool {
		return nodeOrderIndex(nodes, queue[i]) < nodeOrderIndex(nodes, queue[j])
	})
	ordered := make([]agentBlueprintGraphNode, 0, len(nodes))
	seen := map[string]bool{}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		ordered = append(ordered, byID[id])
		for _, target := range outgoing[id] {
			incoming[target]--
			if incoming[target] == 0 {
				queue = append(queue, target)
			}
		}
	}
	for _, node := range nodes {
		if !seen[node.ID] {
			ordered = append(ordered, node)
		}
	}
	return ordered
}

func nodeOrderIndex(nodes []agentBlueprintGraphNode, id string) int {
	for index, node := range nodes {
		if node.ID == id {
			return index
		}
	}
	return len(nodes)
}

func upstreamAgentNodeOutputs(
	nodeID string,
	edges []agentBlueprintGraphEdge,
	outputsByNodeID map[string]map[string]interface{},
) map[string]interface{} {
	upstream := map[string]interface{}{}
	for _, edge := range edges {
		if edge.Target != nodeID {
			continue
		}
		if output, ok := outputsByNodeID[edge.Source]; ok {
			upstream[edge.Source] = output
		}
	}
	return upstream
}

func executeAgentBlueprintNode(
	ctx context.Context,
	store *Store,
	bp *AgentBlueprint,
	node agentBlueprintGraphNode,
	upstream map[string]interface{},
	inputVars map[string]interface{},
) agentBlueprintNodeResult {
	started := time.Now()
	result := agentBlueprintNodeResult{
		NodeID:    node.ID,
		Kind:      node.Data.Kind,
		Title:     firstNonEmpty(node.Data.Title, node.Data.Kind),
		Status:    "completed",
		StartedAt: started.UnixMilli(),
		Input: map[string]interface{}{
			"settings": node.Data.Settings,
			"upstream": upstream,
			"vars":     inputVars,
		},
		Output: map[string]interface{}{},
	}
	defer func() {
		result.EndedAt = time.Now().UnixMilli()
		result.DurationMS = time.Since(started).Milliseconds()
	}()

	switch node.Data.Kind {
	case "start":
		result.Message = "开始节点已触发。"
		result.Output = map[string]interface{}{"signal": "started", "input_vars": inputVars}
	case "schedule":
		result.Message = "定时节点条件已记录；实际调度由蓝图调度器负责。"
		result.Output = map[string]interface{}{
			"signal":    "scheduled",
			"frequency": node.Data.Settings["执行频率"],
			"time":      node.Data.Settings["执行时间"],
			"timezone":  node.Data.Settings["时区"],
		}
	case "browser":
		result = executeAgentBrowserGraphNode(ctx, store, node, result)
	case "request":
		result = executeAgentRequestGraphNode(ctx, node, upstream, result)
	case "transform":
		result = executeAgentTransformGraphNode(node, upstream, result)
	case "image":
		result = executeAgentImageGraphNode(ctx, store, bp, node, upstream, result)
	case "report":
		result = executeAgentReportGraphNode(store, node, upstream, result)
	case "webpage":
		result = executeAgentWebpageGraphNode(store, node, upstream, result)
	case "notify":
		result = executeAgentNotifyGraphNode(ctx, node, upstream, result)
	case "stop":
		result.Message = "结束节点已执行。"
		result.Output = map[string]interface{}{"status": "stopped", "upstream": upstream}
	default:
		if handled, handledResult := executeAgentBuiltinCustomComponent(ctx, store, bp, node, upstream, inputVars, result); handled {
			return handledResult
		}
		result = executeAgentCustomGraphNode(node, upstream, result)
	}
	return result
}

func executeAgentRequestGraphNode(
	ctx context.Context,
	node agentBlueprintGraphNode,
	upstream map[string]interface{},
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	url := firstNonEmpty(node.Data.Settings["网址"], node.Data.Settings["请求URL"], stringFromMap(upstream, "url"))
	if strings.TrimSpace(url) == "" {
		result.Status = "failed"
		result.Message = "网页请求节点未配置网址。"
		result.Error = "request url is required"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	resp, err := executeAgentHTTPRequest(ctx, AgentHTTPRequest{
		URL:     url,
		Method:  firstNonEmpty(node.Data.Settings["方法"], "GET"),
		Headers: parseAgentHTTPHeaderLines(node.Data.Settings["请求头"]),
		Body:    node.Data.Settings["请求体"],
		Timeout: parseAgentDurationSeconds(node.Data.Settings["超时"], 30),
	})
	if err != nil {
		result.Status = "failed"
		result.Message = "真实 HTTP 请求失败。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error, "url": url}
		return result
	}
	result.Message = fmt.Sprintf("真实 HTTP 请求完成，状态码 %d。", resp.StatusCode)
	result.Output = map[string]interface{}{
		"url":          resp.URL,
		"final_url":    resp.FinalURL,
		"status_code":  resp.StatusCode,
		"content_type": resp.ContentType,
		"body":         resp.Body,
		"truncated":    resp.Truncated,
		"duration_ms":  resp.DurationMs,
	}
	return result
}

func executeAgentBrowserGraphNode(
	ctx context.Context,
	store *Store,
	node agentBlueprintGraphNode,
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	url := firstNonEmpty(node.Data.Settings["网址"], node.Data.Settings["URL"])
	if strings.TrimSpace(url) == "" {
		result.Status = "failed"
		result.Message = "浏览器取数节点未配置网址。"
		result.Error = "browser url is required"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	if store == nil || strings.TrimSpace(store.DataDir()) == "" {
		result.Status = "failed"
		result.Message = "浏览器取数无法执行，数据目录未配置。"
		result.Error = "data directory is not configured"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	captureCtx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	captured, err := captureAgentBrowserPage(captureCtx, store.DataDir(), AgentBrowserCaptureRequest{
		URL:         url,
		LoginState:  node.Data.Settings["登录态"],
		WaitSeconds: parseAgentDurationSeconds(node.Data.Settings["采集等待"], agentBrowserCaptureDefault),
	})
	if err != nil {
		result.Status = "failed"
		result.Message = "浏览器真实采集失败。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error, "url": url}
		return result
	}
	result.Message = fmt.Sprintf("浏览器真实采集完成，获取 %d 个字符。", len([]rune(captured.Text)))
	result.Output = map[string]interface{}{
		"url":         captured.URL,
		"final_url":   captured.FinalURL,
		"title":       captured.Title,
		"text":        captured.Text,
		"text_length": len([]rune(captured.Text)),
		"captured_at": captured.CapturedAt,
		"elapsed_ms":  captured.ElapsedMS,
	}
	return result
}

func executeAgentTransformGraphNode(
	node agentBlueprintGraphNode,
	upstream map[string]interface{},
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	if len(upstream) == 0 {
		result.Status = "failed"
		result.Message = "整理数据节点没有收到上游真实输出。"
		result.Error = "upstream data is required"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	markdown := agentUpstreamMarkdownTable(upstream)
	result.Message = "已按上游真实输出生成确定性整理结果。"
	result.Output = map[string]interface{}{
		"format":      firstNonEmpty(node.Data.Settings["输出格式"], "Markdown 表格"),
		"table":       markdown,
		"summary":     agentSummarizeUpstream(upstream, 2000),
		"description": node.Data.Description,
	}
	return result
}

func executeAgentImageGraphNode(
	ctx context.Context,
	store *Store,
	bp *AgentBlueprint,
	node agentBlueprintGraphNode,
	upstream map[string]interface{},
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	if store == nil || store.PlaygroundImageHandlers() == nil {
		result.Status = "failed"
		result.Message = "图片节点无法执行，图片生成处理器未注册。"
		result.Error = "image handler is unavailable"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}

	prompt := buildAgentImagePrompt(node, upstream)
	params := buildAgentImageParams(node)
	paramsJSON, _ := json.Marshal(params)

	model := firstNonEmpty(node.Data.Model, bp.ModelName, "gpt-image-1")
	group := normalizeGroupName(firstNonEmpty(node.Data.Group, bp.ModelGroup, "default"))
	createdAt := time.Now().UnixMilli()
	taskID := fmt.Sprintf("agent-image-%s-%d", safeImageHistorySegment(node.ID), createdAt)
	item := PlaygroundImageGeneration{
		ID:              taskID,
		Prompt:          prompt,
		NegativePrompt:  stringFromInterface(params["negativePrompt"]),
		Params:          paramsJSON,
		Model:           model,
		Group:           group,
		ReferenceImages: json.RawMessage("[]"),
		URLs:            []string{},
		Status:          "running",
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}
	requestBody, _ := json.Marshal(buildAgentImageRequest(model, group, prompt, params))
	saved, err := startPlaygroundImageTask(ctx, store, store.PlaygroundImageHandlers(), bp.UserID, item, requestBody)
	if err != nil {
		result.Status = "failed"
		result.Message = "图片任务提交失败。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error, "task_id": taskID}
		return result
	}

	result.Message = "图片任务已提交到创作台真实图片生成队列。"
	result.Output = map[string]interface{}{
		"task_id":         saved.ID,
		"status":          saved.Status,
		"prompt":          prompt,
		"model":           saved.Model,
		"group":           saved.Group,
		"params":          params,
		"urls":            saved.URLs,
		"image_history":   "/api/playground/image-history",
		"submitted_at":    saved.CreatedAt,
		"async_execution": true,
	}
	return result
}

func buildAgentImagePrompt(node agentBlueprintGraphNode, upstream map[string]interface{}) string {
	explicitPrompt := strings.TrimSpace(node.Data.Settings["图片提示词"])
	style := firstNonEmpty(node.Data.Settings["风格"], "auto")
	upstreamSummary := agentSummarizeUpstream(upstream, 2400)
	parts := []string{
		strings.TrimSpace(node.Data.Description),
	}
	if explicitPrompt != "" {
		parts = append(parts, "图片提示词: "+explicitPrompt)
	}
	parts = append(parts, "风格: "+style)
	if upstreamSummary != "" && upstreamSummary != "{}" {
		parts = append(parts, "上游真实输入:\n"+upstreamSummary)
	} else {
		parts = append(parts, "上游真实输入: 未提供，按当前节点描述生成通用图片。")
	}
	parts = append(parts, "要求: 只依据上游真实输入和当前节点描述生成，不使用演示图片或内置样例。")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			out = append(out, strings.TrimSpace(part))
		}
	}
	return truncateRunes(strings.Join(out, "\n\n"), 4000)
}

func buildAgentImageParams(node agentBlueprintGraphNode) map[string]interface{} {
	aspectRatio := normalizeAgentImageAspectRatio(node.Data.Settings["尺寸"])
	return map[string]interface{}{
		"resolution":     "auto",
		"aspectRatio":    aspectRatio,
		"quality":        "auto",
		"style":          agentImageStyleForSetting(node.Data.Settings["风格"]),
		"n":              normalizeAgentImageCount(node.Data.Settings["数量"]),
		"seed":           nil,
		"negativePrompt": strings.TrimSpace(node.Data.Settings["负面提示词"]),
	}
}

func buildAgentImageRequest(model, group, prompt string, params map[string]interface{}) map[string]interface{} {
	resolution := stringFromInterface(params["resolution"])
	aspectRatio := stringFromInterface(params["aspectRatio"])
	request := map[string]interface{}{
		"model":           model,
		"group":           group,
		"prompt":          prompt,
		"quality":         stringFromInterface(params["quality"]),
		"style":           stringFromInterface(params["style"]),
		"n":               params["n"],
		"seed":            params["seed"],
		"response_format": "url",
	}
	if negativePrompt := stringFromInterface(params["negativePrompt"]); negativePrompt != "" {
		request["negative_prompt"] = negativePrompt
	}
	if resolution != "" && resolution != "auto" {
		request["resolution"] = resolution
	}
	if aspectRatio != "" && aspectRatio != "auto" {
		request["aspect_ratio"] = aspectRatio
	}
	if size := resolveAgentImageSize(resolution, aspectRatio); size != "" {
		request["size"] = size
	}
	return request
}

func normalizeAgentImageAspectRatio(value string) string {
	switch strings.TrimSpace(value) {
	case "1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3":
		return strings.TrimSpace(value)
	default:
		return "auto"
	}
}

func agentImageStyleForSetting(value string) string {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	if value == "" || lower == "auto" {
		return "auto"
	}
	if strings.Contains(value, "自然") || strings.Contains(value, "写实") || strings.Contains(lower, "natural") {
		return "natural"
	}
	if strings.Contains(value, "鲜明") || strings.Contains(value, "营销") || strings.Contains(lower, "vivid") {
		return "vivid"
	}
	return "auto"
}

func normalizeAgentImageCount(value string) int {
	value = strings.TrimSpace(value)
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed <= 0 {
		return 1
	}
	if parsed > 4 {
		return 4
	}
	return parsed
}

func resolveAgentImageSize(resolution, aspectRatio string) string {
	if resolution == "auto" || aspectRatio == "auto" || resolution == "" || aspectRatio == "" {
		return ""
	}
	sizes := map[string]map[string]string{
		"1k": {
			"1:1":  "1024x1024",
			"16:9": "1792x1024",
			"9:16": "1024x1792",
			"4:3":  "1344x1024",
			"3:4":  "1024x1344",
			"3:2":  "1536x1024",
			"2:3":  "1024x1536",
		},
		"2k": {
			"1:1":  "2048x2048",
			"16:9": "2048x1152",
			"9:16": "1152x2048",
			"4:3":  "2048x1536",
			"3:4":  "1536x2048",
			"3:2":  "2048x1365",
			"2:3":  "1365x2048",
		},
		"4k": {
			"1:1":  "4096x4096",
			"16:9": "4096x2304",
			"9:16": "2304x4096",
			"4:3":  "4096x3072",
			"3:4":  "3072x4096",
			"3:2":  "4096x2731",
			"2:3":  "2731x4096",
		},
	}
	if byRatio, ok := sizes[resolution]; ok {
		return byRatio[aspectRatio]
	}
	return ""
}

func executeAgentReportGraphNode(
	store *Store,
	node agentBlueprintGraphNode,
	upstream map[string]interface{},
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	if len(upstream) == 0 {
		result.Status = "failed"
		result.Message = "生成日报节点没有收到上游真实输出。"
		result.Error = "upstream data is required"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	content := buildAgentGraphReportContent(node, upstream)
	files := agentGraphReportFiles(node, content)
	writeResult, err := writeAgentReportFiles(store.DataDir(), AgentReportWriteRequest{
		Title:           firstNonEmpty(node.Data.Title, "生成日报"),
		OutputDirectory: firstNonEmpty(node.Data.Settings["输出目录"], "data/output/reports"),
		Files:           files,
	})
	if err != nil {
		result.Status = "failed"
		result.Message = "日报文件写入失败。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	result.Message = "日报文件已按生成日报节点配置写入。"
	result.ArchivePath = writeResult.OutputDirectory
	result.Output = map[string]interface{}{
		"content":          content,
		"output_directory": writeResult.OutputDirectory,
		"files":            writeResult.Files,
	}
	return result
}

func executeAgentWebpageGraphNode(
	store *Store,
	node agentBlueprintGraphNode,
	upstream map[string]interface{},
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	if len(upstream) == 0 {
		result.Status = "failed"
		result.Message = "输出网页节点没有收到上游真实输出。"
		result.Error = "upstream data is required"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	html := "<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"><title>" +
		escapeAgentHTML(firstNonEmpty(node.Data.Title, "智能体网页输出")) +
		"</title></head><body><h1>" + escapeAgentHTML(firstNonEmpty(node.Data.Title, "智能体网页输出")) +
		"</h1><pre>" + escapeAgentHTML(agentSummarizeUpstream(upstream, 8000)) + "</pre></body></html>"
	writeResult, err := writeAgentReportFiles(store.DataDir(), AgentReportWriteRequest{
		Title:           firstNonEmpty(node.Data.Title, "网页输出"),
		OutputDirectory: firstNonEmpty(node.Data.Settings["输出目录"], "data/output/web-reports"),
		Files: []AgentReportOutputFile{{
			Label:     "HTML 网页",
			Extension: "html",
			Content:   html,
		}},
	})
	if err != nil {
		result.Status = "failed"
		result.Message = "网页文件写入失败。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	result.Message = "网页文件已写入输出目录。"
	result.ArchivePath = writeResult.OutputDirectory
	result.Output = map[string]interface{}{
		"html":             html,
		"output_directory": writeResult.OutputDirectory,
		"files":            writeResult.Files,
	}
	return result
}

func executeAgentNotifyGraphNode(
	ctx context.Context,
	node agentBlueprintGraphNode,
	upstream map[string]interface{},
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	channel := firstNonEmpty(node.Data.Settings["渠道"], "Webhook")
	if channel != "Webhook" {
		result.Message = "当前通知渠道未配置真实后端适配器，已生成待发送记录。"
		result.Output = map[string]interface{}{"channel": channel, "pending": true, "upstream": upstream}
		return result
	}
	webhook := firstNonEmpty(node.Data.Settings["Webhook"], node.Data.Settings["网址"])
	if strings.TrimSpace(webhook) == "" {
		result.Status = "failed"
		result.Message = "Webhook 通知未配置 URL。"
		result.Error = "webhook url is required"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	bodyBytes, _ := json.Marshal(map[string]interface{}{
		"title":       firstNonEmpty(node.Data.Title, "智能体通知"),
		"description": node.Data.Description,
		"upstream":    upstream,
		"created_at":  time.Now().Format(time.RFC3339),
	})
	resp, err := executeAgentHTTPRequest(ctx, AgentHTTPRequest{
		URL:     webhook,
		Method:  http.MethodPost,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    string(bodyBytes),
		Timeout: parseAgentDurationSeconds(node.Data.Settings["超时"], 30),
	})
	if err != nil {
		result.Status = "failed"
		result.Message = "Webhook 通知请求失败。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	result.Message = fmt.Sprintf("Webhook 已真实请求，状态码 %d。", resp.StatusCode)
	result.Output = map[string]interface{}{"status_code": resp.StatusCode, "body": resp.Body}
	return result
}

func executeAgentCustomGraphNode(
	node agentBlueprintGraphNode,
	upstream map[string]interface{},
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	if len(upstream) == 0 {
		result.Status = "failed"
		result.Message = "自定义组件没有收到上游真实输出。"
		result.Error = "upstream data is required"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	result.Message = "自定义组件已基于上游真实输出生成确定性结果。"
	result.Output = map[string]interface{}{
		"description": node.Data.Description,
		"summary":     agentSummarizeUpstream(upstream, 4000),
	}
	return result
}

func executeAgentBuiltinCustomComponent(
	ctx context.Context,
	store *Store,
	bp *AgentBlueprint,
	node agentBlueprintGraphNode,
	upstream map[string]interface{},
	inputVars map[string]interface{},
	result agentBlueprintNodeResult,
) (bool, agentBlueprintNodeResult) {
	switch node.Data.ComponentID {
	case "component.webhook":
		result.Message = "Webhook 触发节点已读取显式输入变量。"
		result.Output = map[string]interface{}{
			"body":    firstInputValue(inputVars, "webhook_body", "body", "payload"),
			"headers": firstInputValue(inputVars, "webhook_headers", "headers"),
			"raw":     inputVars,
		}
		return true, result
	case "component.filter":
		return true, executeAgentFilterGraphNode(node, upstream, result)
	case "component.merge":
		return true, executeAgentMergeGraphNode(upstream, result)
	case "component.file":
		return true, executeAgentFileGraphNode(store, node, upstream, result)
	default:
		_ = ctx
		_ = bp
		return false, result
	}
}

func executeAgentFilterGraphNode(
	node agentBlueprintGraphNode,
	upstream map[string]interface{},
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	if len(upstream) == 0 {
		result.Status = "failed"
		result.Message = "条件过滤节点没有收到上游真实输出。"
		result.Error = "upstream data is required"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	condition := strings.TrimSpace(node.Data.Settings["条件"])
	if condition == "" || condition == "字段存在" {
		result.Message = "条件过滤完成：保留非空上游输出。"
		result.Output = map[string]interface{}{"matched": filterNonEmptyUpstream(upstream), "condition": condition}
		return result
	}
	result.Message = "条件过滤完成：当前仅支持确定性非空/字段存在过滤。"
	result.Output = map[string]interface{}{
		"matched":   filterNonEmptyUpstream(upstream),
		"condition": condition,
		"warning":   "复杂表达式需要代码处理组件或自定义组件执行。",
	}
	return result
}

func executeAgentMergeGraphNode(
	upstream map[string]interface{},
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	if len(upstream) == 0 {
		result.Status = "failed"
		result.Message = "合并数据节点没有收到上游真实输出。"
		result.Error = "upstream data is required"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	result.Message = "合并数据完成：按上游节点 ID 保留全部真实输出。"
	result.Output = map[string]interface{}{"merged": upstream, "summary": agentSummarizeUpstream(upstream, 4000)}
	return result
}

func executeAgentFileGraphNode(
	store *Store,
	node agentBlueprintGraphNode,
	upstream map[string]interface{},
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	mode := firstNonEmpty(node.Data.Settings["模式"], "读取")
	path := firstNonEmpty(node.Data.Settings["文件路径"], node.Data.Settings["路径"])
	if strings.Contains(mode, "读取") {
		if path == "" {
			result.Status = "failed"
			result.Message = "读写文件节点未配置文件路径。"
			result.Error = "file path is required"
			result.Output = map[string]interface{}{"error": result.Error}
			return result
		}
		content, err := readAgentDataFile(store, path)
		if err != nil {
			result.Status = "failed"
			result.Message = "文件读取失败。"
			result.Error = err.Error()
			result.Output = map[string]interface{}{"error": result.Error, "path": path}
			return result
		}
		result.Message = "文件读取完成。"
		result.Output = map[string]interface{}{"path": path, "content": content, "size": len(content)}
		return result
	}
	if len(upstream) == 0 {
		result.Status = "failed"
		result.Message = "写入文件节点没有收到上游真实输出。"
		result.Error = "upstream data is required"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	content := agentSummarizeUpstream(upstream, 12000)
	outputDir := firstNonEmpty(node.Data.Settings["输出目录"], "data/output/agent-files")
	writeResult, err := writeAgentReportFiles(store.DataDir(), AgentReportWriteRequest{
		Title:           firstNonEmpty(node.Data.Title, "文件输出"),
		OutputDirectory: outputDir,
		Files: []AgentReportOutputFile{{
			Label:     firstNonEmpty(node.Data.Settings["格式"], "TXT 文本"),
			Extension: agentReportExtensionForLabel(firstNonEmpty(node.Data.Settings["格式"], "TXT 文本")),
			Content:   content,
		}},
	})
	if err != nil {
		result.Status = "failed"
		result.Message = "文件写入失败。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	result.Message = "文件写入完成。"
	result.Output = map[string]interface{}{"files": writeResult.Files, "output_directory": writeResult.OutputDirectory}
	return result
}

func parseAgentHTTPHeaderLines(value string) map[string]string {
	headers := map[string]string{}
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			continue
		}
		headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return headers
}

func firstInputValue(inputVars map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if value, ok := inputVars[key]; ok {
			return value
		}
	}
	return nil
}

func filterNonEmptyUpstream(upstream map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for key, value := range upstream {
		if value == nil {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func readAgentDataFile(store *Store, displayPath string) (string, error) {
	if store == nil || strings.TrimSpace(store.DataDir()) == "" {
		return "", errors.New("data directory is not configured")
	}
	path := strings.Trim(strings.TrimSpace(filepath.ToSlash(displayPath)), "/")
	if path == "" || strings.Contains(path, "..") {
		return "", errors.New("invalid file path")
	}
	if strings.HasPrefix(path, "data/") {
		path = strings.TrimPrefix(path, "data/")
	}
	fullPath := filepath.Clean(filepath.Join(store.DataDir(), filepath.FromSlash(path)))
	root := filepath.Clean(store.DataDir())
	if fullPath != root && !strings.HasPrefix(fullPath, root+string(filepath.Separator)) {
		return "", errors.New("invalid file path")
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	if len(data) > 1024*1024 {
		data = data[:1024*1024]
	}
	return string(data), nil
}

func parseAgentDurationSeconds(value string, fallback int) int {
	value = strings.TrimSpace(strings.TrimSuffix(value, "秒"))
	value = strings.TrimSuffix(value, "s")
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed <= 0 {
		return fallback
	}
	if parsed > 60 {
		return 60
	}
	return parsed
}

func stringFromMap(value map[string]interface{}, key string) string {
	for _, raw := range value {
		if child, ok := raw.(map[string]interface{}); ok {
			if s, ok := child[key].(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

func stringFromInterface(value interface{}) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func agentUpstreamMarkdownTable(upstream map[string]interface{}) string {
	return "| 上游节点 | 内容摘要 |\n| --- | --- |\n" + agentUpstreamRows(upstream)
}

func agentUpstreamRows(upstream map[string]interface{}) string {
	keys := make([]string, 0, len(upstream))
	for key := range upstream {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, key := range keys {
		sb.WriteString("| ")
		sb.WriteString(escapeMarkdownCell(key))
		sb.WriteString(" | ")
		sb.WriteString(escapeMarkdownCell(agentValueExcerpt(upstream[key], 500)))
		sb.WriteString(" |\n")
	}
	return sb.String()
}

func agentSummarizeUpstream(upstream map[string]interface{}, maxLength int) string {
	data, err := json.MarshalIndent(upstream, "", "  ")
	if err != nil {
		return ""
	}
	text := string(data)
	if len(text) > maxLength {
		return text[:maxLength] + "\n...（已截断）"
	}
	return text
}

func agentValueExcerpt(value interface{}, maxLength int) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	text := string(data)
	if len(text) > maxLength {
		return text[:maxLength] + "..."
	}
	return text
}

func escapeMarkdownCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", "<br />")
	return value
}

func escapeAgentHTML(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, "\"", "&quot;")
	value = strings.ReplaceAll(value, "'", "&#39;")
	return value
}

func buildAgentGraphReportContent(node agentBlueprintGraphNode, upstream map[string]interface{}) string {
	return strings.Join([]string{
		"# " + firstNonEmpty(node.Data.Title, "日报"),
		"",
		"## 节点描述",
		firstNonEmpty(node.Data.Description, "未填写描述。"),
		"",
		"## 上游真实输出",
		agentUpstreamMarkdownTable(upstream),
		"",
		"## 原始数据",
		"```json",
		agentSummarizeUpstream(upstream, 12000),
		"```",
	}, "\n")
}

func agentGraphReportFiles(node agentBlueprintGraphNode, content string) []AgentReportOutputFile {
	formats := splitAgentOutputFormats(node.Data.Settings["输出格式"])
	if len(formats) == 0 {
		formats = []string{"Markdown 日报"}
	}
	files := make([]AgentReportOutputFile, 0, len(formats))
	for _, format := range formats {
		files = append(files, AgentReportOutputFile{
			Label:                format,
			Extension:            agentReportExtensionForLabel(format),
			Content:              content,
			TemplatePath:         firstNonEmpty(node.Data.Settings["模板文件"], node.Data.Settings["网页模板"]),
			TemplateOriginalName: node.Data.Settings["模板原文件名"],
		})
	}
	return files
}

func splitAgentOutputFormats(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '、' || r == ',' || r == '，' || r == ';' || r == '；' || r == '|' || r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return uniqueNonEmptyStrings(out)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
		Label:        "网页研究 Skill",
		Description:  "面向检索、网页阅读和事实核验的通用研究能力，适合多数资料收集任务。",
		Capabilities: []string{"关键词规划", "来源交叉验证", "引用摘要"},
		Instructions: []string{
			"先把任务拆成 2-5 个可检索问题，再围绕当前节点输入进行查找和摘要。",
			"优先使用权威、官方、近期或一手来源；多个来源冲突时明确说明差异。",
			"输出结论、依据和不确定点，不要把未验证内容当作事实。",
		},
	},
	"skill.file-document": {
		Label:        "文件文档 Skill",
		Description:  "读取和整理 Excel、CSV、PDF、Word、Markdown、HTML、TXT 等常见文件。",
		Capabilities: []string{"多格式解析", "模板遵循", "结构保真"},
		Instructions: []string{
			"只使用当前上传或上游显式提供的文件内容，不猜测未提供的表格、标题或业务背景。",
			"保留文件中的关键字段、工作表、表头、段落层级和格式要求。",
			"输出格式必须跟模板或节点配置一致；无法读取时说明具体缺失文件或格式限制。",
		},
	},
	"skill.browser-automation": {
		Label:        "浏览器自动化 Skill",
		Description:  "面向登录态页面、表单、动态网页和截图证据的浏览器自动化能力。",
		Capabilities: []string{"页面导航", "DOM 抽取", "操作安全"},
		Instructions: []string{
			"按节点给出的 URL、登录态和任务描述执行导航、读取、截图和字段抽取。",
			"遇到弹窗、登录、分页、懒加载或动态内容时记录处理步骤和剩余限制。",
			"默认只读；购买、删除、提交、发布、转账等高风险动作必须停下并等待确认。",
		},
	},
	"skill.code-interpreter": {
		Label:        "代码解释器 Skill",
		Description:  "用于数据清洗、计算、图表、文件转换和可复现脚本化分析。",
		Capabilities: []string{"数据清洗", "计算分析", "文件生成"},
		Instructions: []string{
			"优先用确定性的表格/脚本处理计算、清洗、去重、排序、转换和统计任务。",
			"输出关键步骤、计算口径、生成文件路径或可复用结果，避免只给泛泛建议。",
			"如果数据不足或格式不一致，先列出阻塞字段和建议修复方式。",
		},
	},
	"skill.api-function": {
		Label:        "API 函数 Skill",
		Description:  "组织 HTTP/API 调用、函数参数、重试策略和结构化响应解析。",
		Capabilities: []string{"参数校验", "JSON 解析", "错误恢复"},
		Instructions: []string{
			"根据节点中的 endpoint、method、headers、body、鉴权和超时配置组织请求。",
			"把响应解析成稳定 JSON、表格或日志摘要，保留状态码、错误信息和关键字段。",
			"遇到分页、限流、空响应或非 JSON 响应时说明处理策略，不伪造结果。",
		},
	},
	"skill.rag-file-search": {
		Label:        "知识检索 Skill",
		Description:  "面向知识库、文档检索、片段引用和问答总结的常用检索增强能力。",
		Capabilities: []string{"语义检索", "片段引用", "答案合成"},
		Instructions: []string{
			"围绕问题检索最相关的文档片段，并基于片段回答，不使用未命中的外部背景补全。",
			"保留来源文件、页码、段落或片段标识，区分事实、推断和待确认内容。",
			"当检索结果不足时，明确说明没有足够依据，而不是生成看似完整的答案。",
		},
	},
	"skill.database-sql": {
		Label:        "数据库 SQL Skill",
		Description:  "面向结构化数据查询、SQL 生成、指标口径和结果解释的能力。",
		Capabilities: []string{"Schema 理解", "SQL 生成", "结果校验"},
		Instructions: []string{
			"先确认表结构、字段含义、过滤条件和时间范围，再生成查询或解释结果。",
			"优先使用只读查询；避免破坏性 SQL，涉及写入、删除、更新时必须请求确认。",
			"输出 SQL、参数、结果摘要和可能的数据质量风险。",
		},
	},
	"skill.mcp-workflow": {
		Label:        "MCP 工作流 Skill",
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
