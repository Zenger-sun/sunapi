package sunapi

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
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

	// 更新蓝图状态。包含定时触发节点的蓝图代表一个持续运行的计划，
	// 单次执行成功后仍保持 running，直到用户显式暂停。
	finalStatus := "completed"
	failureReason := ""
	if execErr != nil {
		finalStatus = "failed"
		failureReason = execErr.Error()
		log.WithError(execErr).WithField("blueprint_id", bp.ID).
			Warn("agent: blueprint execution failed")
	} else if blueprintHasScheduleNode(bp) {
		finalStatus = "running"
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

func blueprintHasScheduleNode(bp *AgentBlueprint) bool {
	if bp == nil || len(bp.Nodes) == 0 || string(bp.Nodes) == "null" {
		return false
	}
	var nodes []agentBlueprintGraphNode
	if err := json.Unmarshal(bp.Nodes, &nodes); err != nil {
		return false
	}
	for _, node := range nodes {
		if node.Data.Kind == "schedule" || node.Data.ComponentID == "component.schedule" {
			return true
		}
	}
	return false
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
	var graphErr error

	for _, node := range ordered {
		upstream := upstreamAgentNodeOutputs(node.ID, edges, outputsByNodeID)
		result := executeAgentBlueprintNode(ctx, store, bp, node, upstream, inputVars)
		results = append(results, result)
		if result.Status == "failed" {
			warnings = append(warnings, fmt.Sprintf("%s: %s", node.ID, result.Error))
			graphErr = fmt.Errorf("node %s failed: %s", node.ID, firstNonEmpty(result.Error, result.Message))
			outputsByNodeID[node.ID] = result.Output
			break
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

	if graphErr != nil {
		return json.RawMessage(outputJSON), archivePath, graphErr
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
		if node.Data.ComponentID == "component.code" {
			result = executeAgentCodeGraphNode(node, upstream, result)
		} else {
			result = executeAgentTransformGraphNode(node, upstream, result)
		}
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
		result = executeAgentCustomGraphNode(ctx, store, bp, node, upstream, inputVars, result)
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
	retryCount := parseAgentRetryCount(node.Data.Settings["重试"], 0)
	req := AgentHTTPRequest{
		URL:     url,
		Method:  firstNonEmpty(node.Data.Settings["方法"], "GET"),
		Headers: parseAgentHTTPHeaderLines(node.Data.Settings["请求头"]),
		Body:    node.Data.Settings["请求体"],
		Timeout: parseAgentDurationSeconds(node.Data.Settings["超时"], 30),
	}
	resp, attemptLogs, hasResp, lastErr := executeAgentHTTPRequestWithRetry(
		ctx,
		req,
		retryCount,
		executeAgentHTTPRequest,
		waitAgentHTTPRequestRetry,
	)
	if !hasResp && lastErr != nil {
		result.Status = "failed"
		result.Message = "真实 HTTP 请求失败。"
		result.Error = lastErr.Error()
		result.Output = map[string]interface{}{"error": result.Error, "url": url, "attempts": attemptLogs}
		return result
	}
	if hasResp && lastErr != nil {
		result.Status = "failed"
		result.Message = fmt.Sprintf("真实 HTTP 请求返回失败状态码 %d，已尝试 %d 次。", resp.StatusCode, len(attemptLogs))
		result.Error = lastErr.Error()
		result.Output = map[string]interface{}{
			"error":        result.Error,
			"url":          resp.URL,
			"final_url":    resp.FinalURL,
			"status_code":  resp.StatusCode,
			"content_type": resp.ContentType,
			"body":         resp.Body,
			"truncated":    resp.Truncated,
			"duration_ms":  resp.DurationMs,
			"attempts":     attemptLogs,
			"retry_count":  retryCount,
		}
		return result
	}
	result.Message = fmt.Sprintf("真实 HTTP 请求完成，状态码 %d，尝试 %d 次。", resp.StatusCode, len(attemptLogs))
	result.Output = map[string]interface{}{
		"url":          resp.URL,
		"final_url":    resp.FinalURL,
		"status_code":  resp.StatusCode,
		"content_type": resp.ContentType,
		"body":         resp.Body,
		"truncated":    resp.Truncated,
		"duration_ms":  resp.DurationMs,
		"attempts":     attemptLogs,
		"retry_count":  retryCount,
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
		"url":              captured.URL,
		"final_url":        captured.FinalURL,
		"title":            captured.Title,
		"text":             captured.Text,
		"text_length":      len([]rune(captured.Text)),
		"screenshot_path":  captured.ScreenshotPath,
		"screenshot_error": captured.ScreenshotError,
		"captured_at":      captured.CapturedAt,
		"elapsed_ms":       captured.ElapsedMS,
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

func executeAgentCodeGraphNode(
	node agentBlueprintGraphNode,
	upstream map[string]interface{},
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	if len(upstream) == 0 {
		result.Status = "failed"
		result.Message = "代码处理节点没有收到上游真实输出。"
		result.Error = "upstream data is required"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	script := strings.TrimSpace(node.Data.Settings["脚本"])
	if script == "" {
		script = "select *\nlimit 100"
	}
	codeResult, err := runAgentCodeDSL(script, upstream)
	if err != nil {
		result.Status = "failed"
		result.Message = "代码处理脚本执行失败。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error, "script": script}
		return result
	}
	result.Message = fmt.Sprintf("代码处理完成，执行 %d 个步骤。", len(codeResult.Steps))
	result.Output = map[string]interface{}{
		"script":      script,
		"steps":       codeResult.Steps,
		"result":      codeResult.Value,
		"result_type": fmt.Sprintf("%T", codeResult.Value),
		"format":      firstNonEmpty(node.Data.Settings["输出格式"], "JSON 数组"),
		"description": node.Data.Description,
	}
	return result
}

type agentCodeDSLResult struct {
	Value interface{} `json:"value"`
	Steps []string    `json:"steps"`
}

func runAgentCodeDSL(script string, upstream map[string]interface{}) (agentCodeDSLResult, error) {
	value := extractAgentCodeInputValue(upstream)
	steps := []string{}
	for _, rawLine := range strings.Split(script, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		next, step, err := applyAgentCodeDSLLine(value, line)
		if err != nil {
			return agentCodeDSLResult{}, err
		}
		value = next
		steps = append(steps, step)
	}
	if len(steps) == 0 {
		return agentCodeDSLResult{}, errors.New("script has no executable steps")
	}
	return agentCodeDSLResult{Value: value, Steps: steps}, nil
}

func extractAgentCodeInputValue(upstream map[string]interface{}) interface{} {
	if len(upstream) == 1 {
		for _, value := range upstream {
			if child, ok := value.(map[string]interface{}); ok {
				for _, key := range []string{"result", "merged", "matched", "data", "rows", "items", "table"} {
					if candidate, exists := child[key]; exists {
						return candidate
					}
				}
			}
			return value
		}
	}
	rows := make([]map[string]interface{}, 0, len(upstream))
	keys := make([]string, 0, len(upstream))
	for key := range upstream {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		rows = append(rows, map[string]interface{}{
			"node":  key,
			"value": upstream[key],
		})
	}
	return rows
}

func applyAgentCodeDSLLine(value interface{}, line string) (interface{}, string, error) {
	command, arg, _ := strings.Cut(line, " ")
	command = strings.ToLower(strings.TrimSpace(command))
	arg = strings.TrimSpace(arg)
	switch command {
	case "select", "fields":
		fields := parseAgentCodeFields(arg)
		if len(fields) == 0 || (len(fields) == 1 && fields[0] == "*") {
			return value, "select *", nil
		}
		rows, err := agentCodeRows(value)
		if err != nil {
			return nil, "", err
		}
		out := make([]map[string]interface{}, 0, len(rows))
		for _, row := range rows {
			next := map[string]interface{}{}
			for _, field := range fields {
				next[field] = agentCodeLookup(row, field)
			}
			out = append(out, next)
		}
		return out, "select " + strings.Join(fields, ", "), nil
	case "filter", "where":
		condition, err := parseAgentCodeCondition(arg)
		if err != nil {
			return nil, "", err
		}
		rows, err := agentCodeRows(value)
		if err != nil {
			return nil, "", err
		}
		out := make([]map[string]interface{}, 0, len(rows))
		for _, row := range rows {
			if condition.Match(row) {
				out = append(out, row)
			}
		}
		return out, "filter " + condition.String(), nil
	case "sort":
		field := strings.TrimSpace(arg)
		desc := false
		if strings.HasPrefix(field, "-") {
			desc = true
			field = strings.TrimPrefix(field, "-")
		}
		field = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(field, " desc"), " DESC"))
		if field == "" {
			return nil, "", errors.New("sort requires a field")
		}
		rows, err := agentCodeRows(value)
		if err != nil {
			return nil, "", err
		}
		sort.SliceStable(rows, func(i, j int) bool {
			cmp := compareAgentCodeValues(agentCodeLookup(rows[i], field), agentCodeLookup(rows[j], field))
			if desc {
				return cmp > 0
			}
			return cmp < 0
		})
		direction := "asc"
		if desc {
			direction = "desc"
		}
		return rows, "sort " + field + " " + direction, nil
	case "limit":
		n, err := parseAgentCodePositiveInt(arg)
		if err != nil {
			return nil, "", err
		}
		rows, err := agentCodeRows(value)
		if err != nil {
			return nil, "", err
		}
		if len(rows) > n {
			rows = rows[:n]
		}
		return rows, fmt.Sprintf("limit %d", n), nil
	case "count":
		rows, err := agentCodeRows(value)
		if err != nil {
			return nil, "", err
		}
		return map[string]interface{}{"count": len(rows)}, "count rows", nil
	case "sum", "average", "avg":
		field := strings.TrimSpace(arg)
		if field == "" {
			return nil, "", errors.New(command + " requires a field")
		}
		rows, err := agentCodeRows(value)
		if err != nil {
			return nil, "", err
		}
		sum := 0.0
		count := 0
		for _, row := range rows {
			if n, ok := agentCodeFloat(agentCodeLookup(row, field)); ok {
				sum += n
				count++
			}
		}
		if command == "average" || command == "avg" {
			avg := 0.0
			if count > 0 {
				avg = sum / float64(count)
			}
			return map[string]interface{}{"field": field, "average": avg, "count": count}, "average " + field, nil
		}
		return map[string]interface{}{"field": field, "sum": sum, "count": count}, "sum " + field, nil
	case "groupby", "group-by":
		field := strings.TrimSpace(arg)
		if field == "" {
			return nil, "", errors.New("groupBy requires a field")
		}
		rows, err := agentCodeRows(value)
		if err != nil {
			return nil, "", err
		}
		groups := map[string]int{}
		for _, row := range rows {
			key := strings.TrimSpace(fmt.Sprint(agentCodeLookup(row, field)))
			if key == "" {
				key = "(empty)"
			}
			groups[key]++
		}
		out := make([]map[string]interface{}, 0, len(groups))
		for key, count := range groups {
			out = append(out, map[string]interface{}{"key": key, "count": count})
		}
		sort.SliceStable(out, func(i, j int) bool {
			return fmt.Sprint(out[i]["key"]) < fmt.Sprint(out[j]["key"])
		})
		return out, "groupBy " + field, nil
	default:
		return nil, "", fmt.Errorf("unsupported script command %q", command)
	}
}

type agentCodeCondition struct {
	Field string
	Op    string
	Value string
}

func parseAgentCodeCondition(arg string) (agentCodeCondition, error) {
	for _, op := range []string{">=", "<=", "!=", "=", ">", "<", "contains"} {
		if op == "contains" {
			parts := strings.SplitN(arg, " contains ", 2)
			if len(parts) == 2 {
				return agentCodeCondition{Field: strings.TrimSpace(parts[0]), Op: op, Value: strings.Trim(strings.TrimSpace(parts[1]), `"'`)}, nil
			}
			continue
		}
		if left, right, ok := strings.Cut(arg, op); ok {
			field := strings.TrimSpace(left)
			value := strings.Trim(strings.TrimSpace(right), `"'`)
			if field == "" {
				return agentCodeCondition{}, errors.New("filter field is required")
			}
			return agentCodeCondition{Field: field, Op: op, Value: value}, nil
		}
	}
	return agentCodeCondition{}, errors.New("filter requires condition like status=ok or amount>10")
}

func (c agentCodeCondition) Match(row map[string]interface{}) bool {
	actual := agentCodeLookup(row, c.Field)
	switch c.Op {
	case "contains":
		return strings.Contains(strings.ToLower(fmt.Sprint(actual)), strings.ToLower(c.Value))
	case "=":
		return strings.EqualFold(strings.TrimSpace(fmt.Sprint(actual)), c.Value)
	case "!=":
		return !strings.EqualFold(strings.TrimSpace(fmt.Sprint(actual)), c.Value)
	case ">", ">=", "<", "<=":
		left, lok := agentCodeFloat(actual)
		right, rok := agentCodeFloat(c.Value)
		if !lok || !rok {
			cmp := strings.Compare(fmt.Sprint(actual), c.Value)
			switch c.Op {
			case ">":
				return cmp > 0
			case ">=":
				return cmp >= 0
			case "<":
				return cmp < 0
			case "<=":
				return cmp <= 0
			}
		}
		switch c.Op {
		case ">":
			return left > right
		case ">=":
			return left >= right
		case "<":
			return left < right
		case "<=":
			return left <= right
		}
	}
	return false
}

func (c agentCodeCondition) String() string {
	return c.Field + c.Op + c.Value
}

func parseAgentCodeFields(arg string) []string {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return nil
	}
	parts := strings.FieldsFunc(arg, func(r rune) bool {
		return r == ',' || r == '，' || r == '、' || r == '|'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseAgentCodePositiveInt(value string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &n); err != nil || n <= 0 {
		return 0, errors.New("limit requires a positive integer")
	}
	if n > 10000 {
		return 10000, nil
	}
	return n, nil
}

func agentCodeRows(value interface{}) ([]map[string]interface{}, error) {
	switch typed := value.(type) {
	case []map[string]interface{}:
		return append([]map[string]interface{}{}, typed...), nil
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for index, item := range typed {
			if row, ok := item.(map[string]interface{}); ok {
				out = append(out, row)
			} else {
				out = append(out, map[string]interface{}{"index": index, "value": item})
			}
		}
		return out, nil
	case map[string]interface{}:
		if rows, ok := typed["rows"]; ok {
			return agentCodeRows(rows)
		}
		if items, ok := typed["items"]; ok {
			return agentCodeRows(items)
		}
		out := make([]map[string]interface{}, 0, len(typed))
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			out = append(out, map[string]interface{}{"key": key, "value": typed[key]})
		}
		return out, nil
	case string:
		var decoded interface{}
		if err := json.Unmarshal([]byte(typed), &decoded); err == nil {
			return agentCodeRows(decoded)
		}
		lines := strings.Split(typed, "\n")
		out := make([]map[string]interface{}, 0, len(lines))
		for index, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				out = append(out, map[string]interface{}{"index": index, "text": line})
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported input type %T", value)
	}
}

func agentCodeLookup(row map[string]interface{}, field string) interface{} {
	field = strings.TrimSpace(field)
	if field == "" {
		return nil
	}
	if value, ok := row[field]; ok {
		return value
	}
	current := interface{}(row)
	for _, part := range strings.Split(field, ".") {
		obj, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current = obj[strings.TrimSpace(part)]
	}
	return current
}

func agentCodeFloat(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		n, err := typed.Float64()
		return n, err == nil
	case string:
		var n float64
		if _, err := fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(typed, "¥")), "%f", &n); err == nil {
			return n, true
		}
	}
	return 0, false
}

func compareAgentCodeValues(a, b interface{}) int {
	if af, ok := agentCodeFloat(a); ok {
		if bf, ok := agentCodeFloat(b); ok {
			if af < bf {
				return -1
			}
			if af > bf {
				return 1
			}
			return 0
		}
	}
	return strings.Compare(fmt.Sprint(a), fmt.Sprint(b))
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
	title := firstNonEmpty(node.Data.Title, "智能体网页输出")
	rows := flattenAgentUpstreamRows(upstream)
	html := rowsToAgentHTML(title, rows, upstream)
	dataJSON, _ := json.MarshalIndent(rows, "", "  ")
	files := []AgentReportOutputFile{{
		Label:     "HTML 网页",
		Extension: "html",
		Content:   html,
	}, {
		Label:     "JSON 数据",
		Extension: "json",
		Content:   string(dataJSON),
	}}
	if strings.Contains(node.Data.Settings["输出模式"], "仅输出数据") {
		files = files[1:]
	}
	writeResult, err := writeAgentReportFiles(store.DataDir(), AgentReportWriteRequest{
		Title:           title,
		OutputDirectory: firstNonEmpty(node.Data.Settings["输出目录"], "data/output/web-reports"),
		Files:           files,
	})
	if err != nil {
		result.Status = "failed"
		result.Message = "网页文件写入失败。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	result.Message = fmt.Sprintf("网页输出已写入输出目录，包含 %d 行数据。", len(rows))
	result.ArchivePath = writeResult.OutputDirectory
	result.Output = map[string]interface{}{
		"html":             html,
		"rows":             rows,
		"row_count":        len(rows),
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
	webhook := firstNonEmpty(node.Data.Settings["Webhook"], node.Data.Settings["通知目标"], node.Data.Settings["网址"])
	if strings.TrimSpace(webhook) == "" {
		result.Status = "failed"
		result.Message = fmt.Sprintf("%s 通知未配置目标 URL。", channel)
		result.Error = "notification target url is required"
		result.Output = map[string]interface{}{"error": result.Error, "channel": channel}
		return result
	}
	bodyBytes, contentType, err := buildAgentNotificationPayload(channel, node, upstream)
	if err != nil {
		result.Status = "failed"
		result.Message = "通知内容构建失败。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error, "channel": channel}
		return result
	}
	resp, err := executeAgentHTTPRequest(ctx, AgentHTTPRequest{
		URL:     webhook,
		Method:  http.MethodPost,
		Headers: map[string]string{"Content-Type": contentType},
		Body:    string(bodyBytes),
		Timeout: parseAgentDurationSeconds(node.Data.Settings["超时"], 30),
	})
	if err != nil {
		result.Status = "failed"
		result.Message = fmt.Sprintf("%s 通知请求失败。", channel)
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error, "channel": channel, "url": webhook}
		return result
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		result.Status = "failed"
		result.Message = fmt.Sprintf("%s 通知返回失败状态码 %d。", channel, resp.StatusCode)
		result.Error = truncateRunes(resp.Body, 1000)
		result.Output = map[string]interface{}{"status_code": resp.StatusCode, "body": resp.Body, "channel": channel}
		return result
	}
	result.Message = fmt.Sprintf("%s 通知已真实请求，状态码 %d。", channel, resp.StatusCode)
	result.Output = map[string]interface{}{"status_code": resp.StatusCode, "body": resp.Body, "channel": channel, "url": resp.FinalURL}
	return result
}

func buildAgentNotificationPayload(
	channel string,
	node agentBlueprintGraphNode,
	upstream map[string]interface{},
) ([]byte, string, error) {
	title := firstNonEmpty(node.Data.Title, "智能体通知")
	description := strings.TrimSpace(node.Data.Description)
	text := buildAgentNotificationText(title, description, upstream)
	switch strings.ToLower(strings.TrimSpace(channel)) {
	case "webhook":
		body, err := json.Marshal(map[string]interface{}{
			"title":       title,
			"description": description,
			"upstream":    upstream,
			"created_at":  time.Now().Format(time.RFC3339),
		})
		return body, "application/json", err
	case "slack":
		body, err := json.Marshal(map[string]interface{}{"text": text})
		return body, "application/json", err
	case "飞书", "feishu", "lark":
		body, err := json.Marshal(map[string]interface{}{
			"msg_type": "text",
			"content":  map[string]string{"text": text},
		})
		return body, "application/json", err
	case "企业微信", "wecom", "wechat-work":
		body, err := json.Marshal(map[string]interface{}{
			"msgtype": "text",
			"text":    map[string]string{"content": text},
		})
		return body, "application/json", err
	case "钉钉", "dingtalk":
		body, err := json.Marshal(map[string]interface{}{
			"msgtype": "text",
			"text":    map[string]string{"content": text},
		})
		return body, "application/json", err
	case "邮件", "email", "mail":
		return nil, "", errors.New("email notification requires an SMTP adapter; use Webhook or a chat robot URL")
	default:
		body, err := json.Marshal(map[string]interface{}{
			"title":       title,
			"description": description,
			"text":        text,
			"upstream":    upstream,
			"channel":     channel,
		})
		return body, "application/json", err
	}
}

func buildAgentNotificationText(title, description string, upstream map[string]interface{}) string {
	parts := []string{title}
	if description != "" {
		parts = append(parts, description)
	}
	if len(upstream) > 0 {
		parts = append(parts, agentSummarizeUpstream(upstream, 2000))
	}
	return truncateRunes(strings.Join(parts, "\n\n"), 3500)
}

func executeAgentCustomGraphNode(
	ctx context.Context,
	store *Store,
	bp *AgentBlueprint,
	node agentBlueprintGraphNode,
	upstream map[string]interface{},
	inputVars map[string]interface{},
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	if len(upstream) == 0 {
		result.Status = "failed"
		result.Message = "自定义组件没有收到上游真实输出。"
		result.Error = "upstream data is required"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	if store == nil || bp == nil {
		result.Status = "failed"
		result.Message = "自定义组件缺少模型执行环境。"
		result.Error = "model execution context is required"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	output, err := callLLMForAgentNode(ctx, store, bp, node, upstream, inputVars)
	if err != nil {
		result.Status = "failed"
		result.Message = "自定义组件模型执行失败。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error, "summary": agentSummarizeUpstream(upstream, 4000)}
		return result
	}
	result.Message = "自定义组件已按当前描述和上游真实输出完成模型执行。"
	result.Output = map[string]interface{}{
		"description": node.Data.Description,
		"model":       firstNonEmpty(node.Data.Model, bp.ModelName),
		"model_group": firstNonEmpty(node.Data.Group, bp.ModelGroup),
		"result":      output,
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
	case "component.branch":
		return true, executeAgentBranchGraphNode(node, upstream, result)
	case "component.wait":
		return true, executeAgentWaitGraphNode(ctx, node, upstream, result)
	case "component.loop":
		return true, executeAgentLoopGraphNode(node, upstream, result)
	case "component.filter":
		return true, executeAgentFilterGraphNode(node, upstream, result)
	case "component.merge":
		return true, executeAgentMergeGraphNode(node, upstream, result)
	case "component.dedupe_sort":
		return true, executeAgentDedupeSortGraphNode(node, upstream, result)
	case "component.database":
		return true, executeAgentDatabaseGraphNode(ctx, store, node, result)
	case "component.file":
		return true, executeAgentFileGraphNode(store, node, upstream, result)
	default:
		_ = ctx
		_ = bp
		return false, result
	}
}

func executeAgentBranchGraphNode(
	node agentBlueprintGraphNode,
	upstream map[string]interface{},
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	if len(upstream) == 0 {
		result.Status = "failed"
		result.Message = "条件分支节点没有收到上游真实输出。"
		result.Error = "upstream data is required"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	condition := strings.TrimSpace(firstNonEmpty(node.Data.Settings["条件"], node.Data.Settings["表达式"]))
	var matched []map[string]interface{}
	var err error
	if condition == "" {
		for key, value := range filterNonEmptyUpstream(upstream) {
			matched = append(matched, map[string]interface{}{"source_node": key, "value": value})
		}
	} else {
		matched, err = filterAgentUpstreamRows(upstream, condition)
	}
	if err != nil {
		result.Status = "failed"
		result.Message = "条件分支判断失败。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error, "condition": condition}
		return result
	}
	passed := len(matched) > 0
	branch := firstNonEmpty(node.Data.Settings["假分支"], "否")
	if passed {
		branch = firstNonEmpty(node.Data.Settings["真分支"], "是")
	}
	result.Message = fmt.Sprintf("条件分支完成：%s，命中 %d 行。", branch, len(matched))
	result.Output = map[string]interface{}{
		"passed":        passed,
		"branch":        branch,
		"condition":     condition,
		"matched":       matched,
		"matched_count": len(matched),
		"upstream":      upstream,
	}
	return result
}

func executeAgentWaitGraphNode(
	ctx context.Context,
	node agentBlueprintGraphNode,
	upstream map[string]interface{},
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	seconds := parseAgentDurationSeconds(firstNonEmpty(node.Data.Settings["等待时间"], node.Data.Settings["延迟"]), 1)
	maxSeconds := parseAgentDurationSeconds(firstNonEmpty(node.Data.Settings["最大等待"], "60s"), 60)
	if seconds > maxSeconds {
		seconds = maxSeconds
	}
	timer := time.NewTimer(time.Duration(seconds) * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		result.Status = "failed"
		result.Message = "等待延迟被取消。"
		result.Error = ctx.Err().Error()
		result.Output = map[string]interface{}{"error": result.Error, "wait_seconds": seconds}
		return result
	case <-timer.C:
	}
	result.Message = fmt.Sprintf("等待延迟完成，实际等待 %d 秒。", seconds)
	result.Output = map[string]interface{}{
		"wait_seconds": seconds,
		"upstream":     upstream,
	}
	return result
}

func executeAgentLoopGraphNode(
	node agentBlueprintGraphNode,
	upstream map[string]interface{},
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	rows := flattenAgentUpstreamRows(upstream)
	if len(rows) == 0 {
		result.Status = "failed"
		result.Message = "列表循环节点没有收到可迭代的上游数据。"
		result.Error = "iterable upstream data is required"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	limit := parseAgentPositiveSetting(firstNonEmpty(node.Data.Settings["最大条目"], node.Data.Settings["限制"]), 100, 10000)
	truncated := false
	if len(rows) > limit {
		rows = rows[:limit]
		truncated = true
	}
	items := make([]map[string]interface{}, 0, len(rows))
	for index, row := range rows {
		next := copyAgentRow(row)
		next["loop_index"] = index
		items = append(items, next)
	}
	outputField := firstNonEmpty(node.Data.Settings["输出字段"], "items")
	result.Message = fmt.Sprintf("列表循环展开完成：输出 %d 个条目。", len(items))
	result.Output = map[string]interface{}{
		outputField:      items,
		"items":          items,
		"count":          len(items),
		"limit":          limit,
		"truncated":      truncated,
		"source_summary": agentSummarizeUpstream(upstream, 2000),
	}
	return result
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
	matched, err := filterAgentUpstreamRows(upstream, condition)
	if err != nil {
		result.Status = "failed"
		result.Message = "条件过滤失败。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error, "condition": condition}
		return result
	}
	if len(matched) == 0 && strings.Contains(firstNonEmpty(node.Data.Settings["未命中处理"], "跳过"), "失败") {
		result.Status = "failed"
		result.Message = "条件过滤未命中任何数据。"
		result.Error = "no rows matched filter condition"
		result.Output = map[string]interface{}{"condition": condition, "matched": matched, "matched_count": 0}
		return result
	}
	result.Message = fmt.Sprintf("条件过滤完成：命中 %d 行真实数据。", len(matched))
	result.Output = map[string]interface{}{
		"matched":   matched,
		"condition": condition,
		"rows":      matched,
		"count":     len(matched),
	}
	return result
}

func executeAgentMergeGraphNode(
	node agentBlueprintGraphNode,
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
	mode := firstNonEmpty(node.Data.Settings["合并方式"], "按字段")
	field := firstNonEmpty(node.Data.Settings["合并字段"], node.Data.Settings["字段"], "id")
	conflict := firstNonEmpty(node.Data.Settings["冲突处理"], "保留最新")
	merged, mergeMode, conflicts, err := mergeAgentUpstreamRows(upstream, mode, field, conflict)
	if err != nil {
		result.Status = "failed"
		result.Message = "合并数据失败。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error, "mode": mode, "field": field}
		return result
	}
	result.Message = fmt.Sprintf("合并数据完成：%s，输出 %d 行真实数据。", mergeMode, len(merged))
	result.Output = map[string]interface{}{
		"merged":         merged,
		"rows":           merged,
		"count":          len(merged),
		"mode":           mergeMode,
		"field":          field,
		"conflict_count": conflicts,
		"summary":        agentSummarizeUpstream(upstream, 4000),
	}
	return result
}

func executeAgentDedupeSortGraphNode(
	node agentBlueprintGraphNode,
	upstream map[string]interface{},
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	rows := flattenAgentUpstreamRows(upstream)
	if len(rows) == 0 {
		result.Status = "failed"
		result.Message = "去重排序节点没有收到可处理的上游行数据。"
		result.Error = "row upstream data is required"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	dedupeField := strings.TrimSpace(firstNonEmpty(node.Data.Settings["去重字段"], node.Data.Settings["字段"]))
	deduped := rows
	removed := 0
	if dedupeField != "" && dedupeField != "*" {
		seen := map[string]bool{}
		deduped = make([]map[string]interface{}, 0, len(rows))
		for _, row := range rows {
			key := strings.TrimSpace(fmt.Sprint(agentCodeLookup(row, dedupeField)))
			if key == "" || key == "<nil>" {
				key = "__missing__" + fmt.Sprint(len(deduped)+removed)
			}
			if seen[key] {
				removed++
				continue
			}
			seen[key] = true
			deduped = append(deduped, copyAgentRow(row))
		}
	}
	sortField := strings.TrimSpace(firstNonEmpty(node.Data.Settings["排序字段"], node.Data.Settings["排序"]))
	sortDirection := strings.TrimSpace(firstNonEmpty(node.Data.Settings["排序方向"], "升序"))
	desc := strings.Contains(sortDirection, "降") || strings.EqualFold(sortDirection, "desc")
	if sortField != "" {
		sort.SliceStable(deduped, func(i, j int) bool {
			cmp := compareAgentCodeValues(agentCodeLookup(deduped[i], sortField), agentCodeLookup(deduped[j], sortField))
			if desc {
				return cmp > 0
			}
			return cmp < 0
		})
	}
	limit := parseAgentPositiveSetting(firstNonEmpty(node.Data.Settings["限制"], node.Data.Settings["最大条目"]), len(deduped), 10000)
	truncated := false
	if len(deduped) > limit {
		deduped = deduped[:limit]
		truncated = true
	}
	result.Message = fmt.Sprintf("去重排序完成：输出 %d 行，去重移除 %d 行。", len(deduped), removed)
	result.Output = map[string]interface{}{
		"rows":           deduped,
		"count":          len(deduped),
		"dedupe_field":   dedupeField,
		"sort_field":     sortField,
		"sort_direction": sortDirection,
		"removed_count":  removed,
		"truncated":      truncated,
	}
	return result
}

func executeAgentDatabaseGraphNode(
	ctx context.Context,
	store *Store,
	node agentBlueprintGraphNode,
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	query, err := normalizeAgentReadOnlySQL(firstNonEmpty(node.Data.Settings["SQL"], node.Data.Settings["查询"]))
	if err != nil {
		result.Status = "failed"
		result.Message = "数据库查询 SQL 不合法。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	db, cleanup, databaseLabel, err := agentDatabaseForNode(store, firstNonEmpty(node.Data.Settings["数据库路径"], node.Data.Settings["数据库"]))
	if err != nil {
		result.Status = "failed"
		result.Message = "数据库查询无法打开数据库。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	if cleanup != nil {
		defer cleanup()
	}
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		result.Status = "failed"
		result.Message = "数据库只读查询执行失败。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error, "sql": query, "database": databaseLabel}
		return result
	}
	defer rows.Close()
	limit := parseAgentPositiveSetting(firstNonEmpty(node.Data.Settings["限制"], node.Data.Settings["最大行数"]), 100, 10000)
	items, columns, truncated, err := scanAgentSQLRows(rows, limit)
	if err != nil {
		result.Status = "failed"
		result.Message = "数据库查询结果读取失败。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error, "sql": query, "database": databaseLabel}
		return result
	}
	result.Message = fmt.Sprintf("数据库只读查询完成：返回 %d 行。", len(items))
	result.Output = map[string]interface{}{
		"database":  databaseLabel,
		"sql":       query,
		"columns":   columns,
		"rows":      items,
		"row_count": len(items),
		"limit":     limit,
		"truncated": truncated,
	}
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
		fileResult, err := readAgentDataFile(store, path)
		if err != nil {
			result.Status = "failed"
			result.Message = "文件读取失败。"
			result.Error = err.Error()
			result.Output = map[string]interface{}{"error": result.Error, "path": path}
			return result
		}
		result.Message = fmt.Sprintf("文件读取完成，识别为 %s。", fileResult.Format)
		result.Output = map[string]interface{}{
			"path":        fileResult.Path,
			"format":      fileResult.Format,
			"extension":   fileResult.Extension,
			"content":     fileResult.Content,
			"rows":        fileResult.Rows,
			"row_count":   len(fileResult.Rows),
			"size":        fileResult.Size,
			"truncated":   fileResult.Truncated,
			"description": node.Data.Description,
		}
		return result
	}
	if len(upstream) == 0 {
		result.Status = "failed"
		result.Message = "写入文件节点没有收到上游真实输出。"
		result.Error = "upstream data is required"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	format := firstNonEmpty(node.Data.Settings["格式"], "自动识别")
	content, extension := formatAgentFileContent(format, upstream)
	outputDir := firstNonEmpty(node.Data.Settings["输出目录"], "data/output/agent-files")
	writeResult, err := writeAgentReportFiles(store.DataDir(), AgentReportWriteRequest{
		Title:           firstNonEmpty(node.Data.Title, "文件输出"),
		OutputDirectory: outputDir,
		Files: []AgentReportOutputFile{{
			Label:     fileFormatLabel(format, extension),
			Extension: extension,
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
	result.Output = map[string]interface{}{
		"files":            writeResult.Files,
		"output_directory": writeResult.OutputDirectory,
		"format":           fileFormatLabel(format, extension),
		"extension":        extension,
	}
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

func filterAgentUpstreamRows(upstream map[string]interface{}, condition string) ([]map[string]interface{}, error) {
	parsed, err := parseAgentCodeCondition(condition)
	if err != nil {
		return nil, err
	}
	rows := flattenAgentUpstreamRows(upstream)
	out := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		if parsed.Match(row) {
			out = append(out, row)
		}
	}
	return out, nil
}

func mergeAgentUpstreamRows(
	upstream map[string]interface{},
	mode string,
	field string,
	conflict string,
) ([]map[string]interface{}, string, int, error) {
	rows := flattenAgentUpstreamRows(upstream)
	if len(rows) == 0 {
		return nil, "", 0, errors.New("no mergeable upstream rows")
	}
	mode = strings.TrimSpace(mode)
	switch {
	case strings.Contains(mode, "追加") || strings.EqualFold(mode, "append"):
		return rows, "追加合并", 0, nil
	case strings.Contains(mode, "索引") || strings.EqualFold(mode, "index"):
		merged := mergeAgentRowsByIndex(rows)
		return merged, "按索引合并", 0, nil
	default:
		field = strings.TrimSpace(field)
		if field == "" {
			return nil, "", 0, errors.New("merge field is required")
		}
		merged, conflicts := mergeAgentRowsByField(rows, field, conflict)
		return merged, "按字段合并", conflicts, nil
	}
}

func flattenAgentUpstreamRows(upstream map[string]interface{}) []map[string]interface{} {
	keys := make([]string, 0, len(upstream))
	for key := range upstream {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]map[string]interface{}, 0, len(keys))
	for _, key := range keys {
		rows, err := agentCodeRows(upstream[key])
		if err != nil || len(rows) == 0 {
			out = append(out, map[string]interface{}{"source_node": key, "value": upstream[key]})
			continue
		}
		for index, row := range rows {
			next := copyAgentRow(row)
			if _, exists := next["source_node"]; !exists {
				next["source_node"] = key
			}
			if _, exists := next["source_index"]; !exists {
				next["source_index"] = index
			}
			out = append(out, next)
		}
	}
	return out
}

func mergeAgentRowsByField(rows []map[string]interface{}, field string, conflict string) ([]map[string]interface{}, int) {
	byKey := map[string]map[string]interface{}{}
	order := []string{}
	conflicts := 0
	for _, row := range rows {
		key := strings.TrimSpace(fmt.Sprint(agentCodeLookup(row, field)))
		if key == "" || key == "<nil>" {
			key = fmt.Sprintf("__missing_%d", len(order))
		}
		current, exists := byKey[key]
		if !exists {
			byKey[key] = copyAgentRow(row)
			order = append(order, key)
			continue
		}
		conflicts++
		byKey[key] = mergeAgentRowValues(current, row, conflict)
	}
	out := make([]map[string]interface{}, 0, len(order))
	for _, key := range order {
		out = append(out, byKey[key])
	}
	return out, conflicts
}

func mergeAgentRowsByIndex(rows []map[string]interface{}) []map[string]interface{} {
	byIndex := map[int]map[string]interface{}{}
	indexes := []int{}
	for position, row := range rows {
		index := position
		if n, ok := agentCodeInt(agentCodeLookup(row, "source_index")); ok {
			index = n
		}
		current, exists := byIndex[index]
		if !exists {
			byIndex[index] = copyAgentRow(row)
			indexes = append(indexes, index)
			continue
		}
		byIndex[index] = mergeAgentRowValues(current, row, "保留最新")
	}
	sort.Ints(indexes)
	out := make([]map[string]interface{}, 0, len(indexes))
	for _, index := range indexes {
		out = append(out, byIndex[index])
	}
	return out
}

func mergeAgentRowValues(left, right map[string]interface{}, conflict string) map[string]interface{} {
	out := copyAgentRow(left)
	keepOld := strings.Contains(conflict, "旧") || strings.Contains(conflict, "最早") || strings.Contains(conflict, "保留已有")
	for key, value := range right {
		if key == "" {
			continue
		}
		existing, exists := out[key]
		if !exists || existing == nil || !keepOld {
			out[key] = value
		}
	}
	return out
}

func copyAgentRow(row map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(row))
	for key, value := range row {
		out[key] = value
	}
	return out
}

func parseAgentPositiveSetting(value string, fallback int, maxValue int) int {
	value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(value, "条"), "行"))
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed <= 0 {
		parsed = fallback
	}
	if parsed <= 0 {
		parsed = 1
	}
	if maxValue > 0 && parsed > maxValue {
		return maxValue
	}
	return parsed
}

func normalizeAgentReadOnlySQL(query string) (string, error) {
	query = strings.TrimSpace(query)
	query = strings.TrimSuffix(query, ";")
	query = strings.TrimSpace(query)
	if query == "" {
		return "", errors.New("SQL is required")
	}
	if strings.Contains(query, ";") {
		return "", errors.New("only one SQL statement is allowed")
	}
	lower := strings.ToLower(strings.TrimSpace(query))
	allowed := strings.HasPrefix(lower, "select ") ||
		strings.HasPrefix(lower, "with ") ||
		strings.HasPrefix(lower, "pragma ")
	if !allowed {
		return "", errors.New("only read-only SELECT/WITH/PRAGMA statements are allowed")
	}
	blockedKeywords := []string{" insert ", " update ", " delete ", " drop ", " alter ", " create ", " replace ", " attach ", " detach ", " vacuum "}
	padded := " " + lower + " "
	for _, keyword := range blockedKeywords {
		if strings.Contains(padded, keyword) {
			return "", fmt.Errorf("read-only SQL cannot contain %s", strings.TrimSpace(keyword))
		}
	}
	return query, nil
}

func agentDatabaseForNode(store *Store, databaseSetting string) (*sql.DB, func(), string, error) {
	databaseSetting = strings.TrimSpace(databaseSetting)
	if databaseSetting == "" || strings.Contains(databaseSetting, "当前") || strings.Contains(strings.ToLower(databaseSetting), "sunapi") {
		if store == nil || store.db == nil {
			return nil, nil, "", errors.New("current SunAPI database is unavailable")
		}
		return store.db, nil, "当前 SunAPI 数据库", nil
	}
	if store == nil || strings.TrimSpace(store.DataDir()) == "" {
		return nil, nil, "", errors.New("data directory is not configured")
	}
	pathValue := strings.Trim(strings.TrimSpace(filepath.ToSlash(databaseSetting)), "/")
	if strings.HasPrefix(pathValue, "data/") {
		pathValue = strings.TrimPrefix(pathValue, "data/")
	}
	if pathValue == "" || strings.Contains(pathValue, "..") {
		return nil, nil, "", errors.New("invalid database path")
	}
	fullPath := filepath.Clean(filepath.Join(store.DataDir(), filepath.FromSlash(pathValue)))
	root := filepath.Clean(store.DataDir())
	if fullPath != root && !strings.HasPrefix(fullPath, root+string(filepath.Separator)) {
		return nil, nil, "", errors.New("database path must stay inside data directory")
	}
	db, err := sql.Open("sqlite", fullPath)
	if err != nil {
		return nil, nil, "", err
	}
	cleanup := func() { _ = db.Close() }
	return db, cleanup, "data/" + filepath.ToSlash(pathValue), nil
}

func scanAgentSQLRows(rows *sql.Rows, limit int) ([]map[string]interface{}, []string, bool, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, false, err
	}
	items := make([]map[string]interface{}, 0, limit)
	truncated := false
	for rows.Next() {
		values := make([]interface{}, len(columns))
		scanTargets := make([]interface{}, len(columns))
		for i := range values {
			scanTargets[i] = &values[i]
		}
		if err := rows.Scan(scanTargets...); err != nil {
			return nil, nil, false, err
		}
		if len(items) >= limit {
			truncated = true
			continue
		}
		item := make(map[string]interface{}, len(columns))
		for i, column := range columns {
			value := values[i]
			if bytesValue, ok := value.([]byte); ok {
				value = string(bytesValue)
			}
			item[column] = value
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, false, err
	}
	return items, columns, truncated, nil
}

func agentCodeInt(value interface{}) (int, bool) {
	if n, ok := agentCodeFloat(value); ok {
		return int(n), true
	}
	return 0, false
}

type agentHTTPRequestExecutor func(context.Context, AgentHTTPRequest) (AgentHTTPResponse, error)

type agentHTTPRequestRetryWaiter func(context.Context, int) bool

func executeAgentHTTPRequestWithRetry(
	ctx context.Context,
	req AgentHTTPRequest,
	retryCount int,
	executor agentHTTPRequestExecutor,
	wait agentHTTPRequestRetryWaiter,
) (AgentHTTPResponse, []map[string]interface{}, bool, error) {
	if executor == nil {
		executor = executeAgentHTTPRequest
	}
	if wait == nil {
		wait = waitAgentHTTPRequestRetry
	}
	attemptLogs := make([]map[string]interface{}, 0, retryCount+1)
	var resp AgentHTTPResponse
	var hasResp bool
	var lastErr error
	for attempt := 1; attempt <= retryCount+1; attempt++ {
		currentResp, err := executor(ctx, req)
		if err != nil {
			lastErr = err
			attemptLogs = append(attemptLogs, map[string]interface{}{
				"attempt": attempt,
				"ok":      false,
				"error":   err.Error(),
			})
			if attempt <= retryCount && wait(ctx, attempt) {
				continue
			}
			break
		}
		resp = currentResp
		hasResp = true
		attemptLogs = append(attemptLogs, map[string]interface{}{
			"attempt":     attempt,
			"ok":          true,
			"status_code": currentResp.StatusCode,
			"duration_ms": currentResp.DurationMs,
			"executed_at": currentResp.ExecutedAt,
		})
		if !shouldRetryAgentHTTPStatus(currentResp.StatusCode) {
			lastErr = nil
			break
		}
		lastErr = fmt.Errorf("HTTP status %d", currentResp.StatusCode)
		if attempt <= retryCount && wait(ctx, attempt) {
			continue
		}
		break
	}
	return resp, attemptLogs, hasResp, lastErr
}

func parseAgentRetryCount(value string, fallback int) int {
	value = strings.TrimSpace(strings.TrimSuffix(value, "次"))
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed < 0 {
		parsed = fallback
	}
	if parsed > 5 {
		return 5
	}
	return parsed
}

func shouldRetryAgentHTTPStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= 500
}

func waitAgentHTTPRequestRetry(ctx context.Context, attempt int) bool {
	delay := time.Duration(attempt) * 200 * time.Millisecond
	if delay > time.Second {
		delay = time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
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

type AgentFileReadResult struct {
	Path      string                   `json:"path"`
	Format    string                   `json:"format"`
	Extension string                   `json:"extension"`
	Content   string                   `json:"content"`
	Rows      []map[string]interface{} `json:"rows,omitempty"`
	Size      int                      `json:"size"`
	Truncated bool                     `json:"truncated"`
}

func readAgentDataFile(store *Store, displayPath string) (AgentFileReadResult, error) {
	if store == nil || strings.TrimSpace(store.DataDir()) == "" {
		return AgentFileReadResult{}, errors.New("data directory is not configured")
	}
	path := strings.Trim(strings.TrimSpace(filepath.ToSlash(displayPath)), "/")
	if path == "" || strings.Contains(path, "..") {
		return AgentFileReadResult{}, errors.New("invalid file path")
	}
	if strings.HasPrefix(path, "data/") {
		path = strings.TrimPrefix(path, "data/")
	}
	fullPath := filepath.Clean(filepath.Join(store.DataDir(), filepath.FromSlash(path)))
	root := filepath.Clean(store.DataDir())
	if fullPath != root && !strings.HasPrefix(fullPath, root+string(filepath.Separator)) {
		return AgentFileReadResult{}, errors.New("invalid file path")
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return AgentFileReadResult{}, err
	}
	originalSize := len(data)
	truncated := false
	if len(data) > 1024*1024 {
		data = data[:1024*1024]
		truncated = true
	}
	ext := strings.ToLower(filepath.Ext(fullPath))
	content, rows, format, err := parseAgentDataFileContent(fullPath, ext, data)
	if err != nil {
		return AgentFileReadResult{}, err
	}
	return AgentFileReadResult{
		Path:      "data/" + filepath.ToSlash(path),
		Format:    format,
		Extension: strings.TrimPrefix(ext, "."),
		Content:   content,
		Rows:      rows,
		Size:      originalSize,
		Truncated: truncated,
	}, nil
}

func parseAgentDataFileContent(absPath, ext string, data []byte) (string, []map[string]interface{}, string, error) {
	switch ext {
	case ".csv":
		text := string(data)
		rows := agentRowsFromTable(parseSimpleCSVRows(text))
		return truncateRunes(text, 12000), rows, "CSV 表格", nil
	case ".json":
		text := string(data)
		rows := agentRowsFromJSON(text)
		return truncateRunes(text, 12000), rows, "JSON 数据", nil
	case ".md", ".markdown":
		text := string(data)
		rows := agentRowsFromTable(parseMarkdownTableRows(text))
		return truncateRunes(text, 12000), rows, "Markdown 文档", nil
	case ".html", ".htm":
		text := stripAgentHTMLTags(string(data))
		return truncateRunes(normalizeAgentTemplatePreview(text), 12000), nil, "HTML 网页", nil
	case ".txt":
		text := string(data)
		return truncateRunes(text, 12000), agentRowsFromLines(text), "TXT 文本", nil
	case ".xlsx":
		content, rows, err := readAgentXLSXContent(absPath)
		if err != nil {
			preview, previewErr := agentTemplateFilePreview(absPath, filepath.Base(absPath))
			if previewErr != nil {
				return "", nil, "", err
			}
			return truncateRunes(preview, 12000), nil, "Excel 表格", nil
		}
		return truncateRunes(content, 12000), rows, "Excel 表格", nil
	case ".docx":
		preview, err := agentTemplateFilePreview(absPath, filepath.Base(absPath))
		if err != nil {
			return "", nil, "", err
		}
		return truncateRunes(preview, 12000), agentRowsFromLines(preview), "Word 文档", nil
	case ".pdf", ".doc", ".xls":
		preview, err := agentTemplateFilePreview(absPath, filepath.Base(absPath))
		if err != nil {
			return "", nil, "", err
		}
		return truncateRunes(preview, 12000), nil, strings.ToUpper(strings.TrimPrefix(ext, ".")) + " 文件", nil
	default:
		return "", nil, "", fmt.Errorf("unsupported file extension %q", ext)
	}
}

func parseSimpleCSVRows(text string) [][]string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	rows := make([][]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		row := make([]string, 0, len(parts))
		for _, part := range parts {
			row = append(row, strings.Trim(strings.TrimSpace(part), `"`))
		}
		rows = append(rows, row)
	}
	return rows
}

func agentRowsFromTable(table [][]string) []map[string]interface{} {
	if len(table) == 0 {
		return nil
	}
	headers := table[0]
	out := make([]map[string]interface{}, 0, maxInt(0, len(table)-1))
	for i, values := range table[1:] {
		row := map[string]interface{}{"index": i}
		for col, value := range values {
			header := fmt.Sprintf("col_%d", col+1)
			if col < len(headers) && strings.TrimSpace(headers[col]) != "" {
				header = headers[col]
			}
			row[header] = value
		}
		out = append(out, row)
	}
	return out
}

func agentRowsFromJSON(text string) []map[string]interface{} {
	var decoded interface{}
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		return nil
	}
	rows, err := agentCodeRows(decoded)
	if err != nil {
		return nil
	}
	return rows
}

func agentRowsFromLines(text string) []map[string]interface{} {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]map[string]interface{}, 0, len(lines))
	for index, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, map[string]interface{}{"index": index, "text": line})
	}
	return out
}

func readAgentXLSXContent(absPath string) (string, []map[string]interface{}, error) {
	reader, err := zip.OpenReader(absPath)
	if err != nil {
		return "", nil, err
	}
	defer reader.Close()
	sharedStrings := []string{}
	sheets := make([][]string, 0)
	for _, file := range reader.File {
		name := strings.ToLower(filepath.ToSlash(file.Name))
		if name != "xl/sharedstrings.xml" && !(strings.HasPrefix(name, "xl/worksheets/sheet") && strings.HasSuffix(name, ".xml")) {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(rc, 512*1024))
		_ = rc.Close()
		if readErr != nil {
			continue
		}
		if name == "xl/sharedstrings.xml" {
			sharedStrings = parseAgentXLSXSharedStrings(data)
			continue
		}
		if rows := parseAgentXLSXWorksheetRows(data, sharedStrings); len(rows) > 0 {
			for _, row := range rows {
				sheets = append(sheets, row)
			}
		}
	}
	if len(sheets) == 0 {
		return "", nil, errors.New("no readable worksheet rows")
	}
	var b strings.Builder
	for _, row := range sheets {
		b.WriteString(strings.Join(row, "\t"))
		b.WriteByte('\n')
	}
	return b.String(), agentRowsFromTable(sheets), nil
}

func parseAgentXLSXSharedStrings(data []byte) []string {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	out := []string{}
	var b strings.Builder
	inItem := false
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := token.(type) {
		case xml.StartElement:
			if strings.EqualFold(t.Name.Local, "si") {
				inItem = true
				b.Reset()
			}
		case xml.EndElement:
			if strings.EqualFold(t.Name.Local, "si") && inItem {
				out = append(out, b.String())
				inItem = false
			}
		case xml.CharData:
			if inItem {
				b.WriteString(string(t))
			}
		}
	}
	return out
}

func parseAgentXLSXWorksheetRows(data []byte, sharedStrings []string) [][]string {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	rows := [][]string{}
	current := []string{}
	cellType := ""
	cellValue := ""
	inValue := false
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := token.(type) {
		case xml.StartElement:
			switch strings.ToLower(t.Name.Local) {
			case "row":
				current = []string{}
			case "c":
				cellType = ""
				cellValue = ""
				for _, attr := range t.Attr {
					if strings.EqualFold(attr.Name.Local, "t") {
						cellType = attr.Value
						break
					}
				}
			case "v", "t":
				inValue = true
			}
		case xml.EndElement:
			switch strings.ToLower(t.Name.Local) {
			case "v", "t":
				inValue = false
			case "c":
				current = append(current, resolveAgentXLSXCellValue(cellValue, cellType, sharedStrings))
			case "row":
				if len(current) > 0 {
					rows = append(rows, current)
				}
			}
		case xml.CharData:
			if inValue {
				cellValue += string(t)
			}
		}
	}
	return rows
}

func resolveAgentXLSXCellValue(value, cellType string, sharedStrings []string) string {
	value = strings.TrimSpace(value)
	if cellType == "s" {
		var idx int
		if _, err := fmt.Sscanf(value, "%d", &idx); err == nil && idx >= 0 && idx < len(sharedStrings) {
			return sharedStrings[idx]
		}
	}
	return value
}

func formatAgentFileContent(format string, upstream map[string]interface{}) (string, string) {
	label := fileFormatLabel(format, agentReportExtensionForLabel(format))
	extension := agentReportExtensionForLabel(label)
	rows := flattenAgentUpstreamRows(upstream)
	switch extension {
	case "json":
		data, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			return agentSummarizeUpstream(upstream, 12000), extension
		}
		return string(data), extension
	case "csv":
		return rowsToAgentCSV(rows), extension
	case "html":
		return rowsToAgentHTML(firstNonEmpty(label, "HTML 网页"), rows, upstream), extension
	case "xlsx":
		return rowsToAgentMarkdownTable(rows), extension
	case "txt":
		return agentSummarizeUpstream(upstream, 12000), extension
	default:
		return rowsToAgentMarkdownTable(rows), extension
	}
}

func fileFormatLabel(format, extension string) string {
	format = strings.TrimSpace(format)
	if format != "" && format != "自动识别" {
		return format
	}
	switch extension {
	case "json":
		return "JSON 数据"
	case "csv":
		return "CSV 表格"
	case "html":
		return "HTML 网页"
	case "xlsx":
		return "Excel 表格"
	case "txt":
		return "TXT 文本"
	default:
		return "Markdown 文档"
	}
}

func rowsToAgentCSV(rows []map[string]interface{}) string {
	headers := collectAgentRowHeaders(rows)
	var b strings.Builder
	b.WriteString(joinAgentCSVRow(headers))
	b.WriteByte('\n')
	for _, row := range rows {
		values := make([]string, 0, len(headers))
		for _, header := range headers {
			values = append(values, stringFromInterface(row[header]))
		}
		b.WriteString(joinAgentCSVRow(values))
		b.WriteByte('\n')
	}
	return b.String()
}

func joinAgentCSVRow(values []string) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ReplaceAll(value, `"`, `""`)
		if strings.ContainsAny(value, ",\"\r\n") {
			value = `"` + value + `"`
		}
		out = append(out, value)
	}
	return strings.Join(out, ",")
}

func rowsToAgentMarkdownTable(rows []map[string]interface{}) string {
	headers := collectAgentRowHeaders(rows)
	if len(headers) == 0 {
		return "暂无数据。"
	}
	var b strings.Builder
	b.WriteString("| ")
	b.WriteString(strings.Join(headers, " | "))
	b.WriteString(" |\n| ")
	for i := range headers {
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString("---")
	}
	b.WriteString(" |\n")
	for _, row := range rows {
		b.WriteString("| ")
		for i, header := range headers {
			if i > 0 {
				b.WriteString(" | ")
			}
			b.WriteString(escapeMarkdownCell(stringFromInterface(row[header])))
		}
		b.WriteString(" |\n")
	}
	return b.String()
}

func rowsToAgentHTML(title string, rows []map[string]interface{}, upstream map[string]interface{}) string {
	headers := collectAgentRowHeaders(rows)
	var b strings.Builder
	b.WriteString("<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"><title>")
	b.WriteString(escapeAgentHTML(title))
	b.WriteString("</title></head><body><h1>")
	b.WriteString(escapeAgentHTML(title))
	b.WriteString("</h1>")
	if len(headers) > 0 {
		b.WriteString("<table><thead><tr>")
		for _, header := range headers {
			b.WriteString("<th>")
			b.WriteString(escapeAgentHTML(header))
			b.WriteString("</th>")
		}
		b.WriteString("</tr></thead><tbody>")
		for _, row := range rows {
			b.WriteString("<tr>")
			for _, header := range headers {
				b.WriteString("<td>")
				b.WriteString(escapeAgentHTML(stringFromInterface(row[header])))
				b.WriteString("</td>")
			}
			b.WriteString("</tr>")
		}
		b.WriteString("</tbody></table>")
	} else {
		b.WriteString("<pre>")
		b.WriteString(escapeAgentHTML(agentSummarizeUpstream(upstream, 12000)))
		b.WriteString("</pre>")
	}
	b.WriteString("</body></html>")
	return b.String()
}

func collectAgentRowHeaders(rows []map[string]interface{}) []string {
	seen := map[string]bool{}
	headers := []string{}
	preferred := []string{"index", "source_node", "source_index"}
	for _, header := range preferred {
		for _, row := range rows {
			if _, ok := row[header]; ok && !seen[header] {
				seen[header] = true
				headers = append(headers, header)
			}
		}
	}
	for _, row := range rows {
		keys := make([]string, 0, len(row))
		for key := range row {
			if !seen[key] {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			seen[key] = true
			headers = append(headers, key)
		}
	}
	return headers
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
	templateBindings := parseAgentReportTemplateBindings(
		firstNonEmpty(node.Data.Settings["模板文件"], node.Data.Settings["网页模板"]),
		node.Data.Settings["模板原文件名"],
	)
	if len(templateBindings) > 0 {
		files := make([]AgentReportOutputFile, 0, len(templateBindings))
		for _, binding := range templateBindings {
			label := agentReportLabelForTemplatePath(binding.Path)
			if label == "" {
				label = firstNonEmpty(node.Data.Settings["输出格式"], "Markdown 日报")
			}
			files = append(files, AgentReportOutputFile{
				Label:                label,
				Extension:            agentReportExtensionForLabel(label),
				Content:              content,
				TemplatePath:         binding.Path,
				TemplateOriginalName: binding.OriginalName,
			})
		}
		return files
	}

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

type agentReportTemplateBinding struct {
	Path         string `json:"path"`
	OriginalName string `json:"originalName"`
}

func parseAgentReportTemplateBindings(value, originalName string) []agentReportTemplateBinding {
	value = strings.TrimSpace(value)
	if value == "" || value == "未上传模板" {
		return nil
	}
	if strings.HasPrefix(value, "[") {
		var rawItems []interface{}
		if err := json.Unmarshal([]byte(value), &rawItems); err == nil {
			out := make([]agentReportTemplateBinding, 0, len(rawItems))
			for _, item := range rawItems {
				switch typed := item.(type) {
				case string:
					if strings.TrimSpace(typed) != "" {
						out = append(out, agentReportTemplateBinding{
							Path:         strings.TrimSpace(typed),
							OriginalName: filepath.Base(filepath.FromSlash(typed)),
						})
					}
				case map[string]interface{}:
					pathValue := strings.TrimSpace(fmt.Sprint(typed["path"]))
					if pathValue == "" || pathValue == "<nil>" {
						continue
					}
					name := strings.TrimSpace(fmt.Sprint(typed["originalName"]))
					if name == "" || name == "<nil>" {
						name = filepath.Base(filepath.FromSlash(pathValue))
					}
					out = append(out, agentReportTemplateBinding{Path: pathValue, OriginalName: name})
				}
			}
			return out
		}
	}
	return []agentReportTemplateBinding{{
		Path:         value,
		OriginalName: firstNonEmpty(originalName, filepath.Base(filepath.FromSlash(value))),
	}}
}

func agentReportLabelForTemplatePath(pathValue string) string {
	switch strings.ToLower(filepath.Ext(filepath.FromSlash(pathValue))) {
	case ".xlsx", ".xls":
		return "Excel 表格"
	case ".csv":
		return "CSV 表格"
	case ".md", ".markdown":
		return "Markdown 日报"
	case ".doc", ".docx":
		return "Word 文档"
	case ".pdf":
		return "PDF 文档"
	case ".html", ".htm":
		return "HTML 网页"
	case ".txt":
		return "TXT 文本"
	case ".json":
		return "JSON 数据"
	default:
		return ""
	}
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
	skillContext := buildBlueprintSkillPromptContext(ctx, store, bp)

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

func callLLMForAgentNode(
	ctx context.Context,
	store *Store,
	bp *AgentBlueprint,
	node agentBlueprintGraphNode,
	upstream map[string]interface{},
	inputVars map[string]interface{},
) (string, error) {
	if store == nil || bp == nil {
		return "", errors.New("agent node model context is required")
	}
	type chatMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	model := firstNonEmpty(node.Data.Model, bp.ModelName)
	if model == "" {
		return "", errors.New("model name is required")
	}
	reqBody := map[string]interface{}{
		"model": model,
		"messages": []chatMsg{
			{Role: "system", Content: buildAgentNodeSystemPrompt(ctx, store, bp, node)},
			{Role: "user", Content: buildAgentNodeUserPrompt(node, upstream, inputVars)},
		},
		"stream": false,
	}
	if group := firstNonEmpty(node.Data.Group, bp.ModelGroup); group != "" {
		reqBody["group"] = group
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}
	apiKey, err := store.getInternalAPIKey(ctx)
	if err != nil {
		return "", fmt.Errorf("no available api key: %w", err)
	}
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
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(
		reqCtx,
		http.MethodPost,
		fmt.Sprintf("http://%s:%d/v1/chat/completions", host, port),
		bytes.NewReader(bodyBytes),
	)
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
		return "", errors.New("empty response from model")
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

func buildAgentNodeSystemPrompt(ctx context.Context, store *Store, bp *AgentBlueprint, node agentBlueprintGraphNode) string {
	var sb strings.Builder
	sb.WriteString("你是自动化蓝图中的一个独立自定义组件。\n")
	sb.WriteString("只根据当前节点描述、当前节点配置、上游节点输出和显式输入变量执行；不要引用蓝图标题、历史样例或未提供的业务背景。\n")
	sb.WriteString("如果输入不足，请输出缺失项和可执行的最小处理结果，不要编造数据。\n")
	sb.WriteString("输出必须可被下游节点继续使用，优先给出结构化 JSON 或清晰字段列表。\n")
	if skillContext := buildAgentNodeSkillPromptContext(ctx, store, bp, node); strings.TrimSpace(skillContext) != "" {
		sb.WriteString(skillContext)
	}
	sb.WriteString("当前节点：\n")
	sb.WriteString(fmt.Sprintf("  - 名称: %s\n", firstNonEmpty(node.Data.Title, node.ID)))
	sb.WriteString(fmt.Sprintf("  - 描述: %s\n", firstNonEmpty(node.Data.Description, "未填写描述")))
	if len(node.Data.Inputs) > 0 {
		sb.WriteString("  - 输入字段: " + strings.Join(node.Data.Inputs, "、") + "\n")
	}
	if len(node.Data.Outputs) > 0 {
		sb.WriteString("  - 输出字段: " + strings.Join(node.Data.Outputs, "、") + "\n")
	}
	return sb.String()
}

func buildAgentNodeUserPrompt(node agentBlueprintGraphNode, upstream map[string]interface{}, inputVars map[string]interface{}) string {
	payload := map[string]interface{}{
		"node_description": node.Data.Description,
		"node_settings":    node.Data.Settings,
		"upstream":         upstream,
		"input_vars":       inputVars,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return firstNonEmpty(node.Data.Description, "请根据当前节点输入执行任务。")
	}
	return "请执行当前自定义组件。输入如下：\n```json\n" + string(data) + "\n```"
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
		Description:  "适配 MCP/GitHub/文件系统等外部工具编排和本地流程控制，强调权限边界和可追踪步骤。",
		Capabilities: []string{"工具选择", "流程控制", "分支循环", "步骤追踪"},
		Instructions: []string{
			"根据任务选择最少必要工具，说明每一步输入、输出和失败补偿方案。",
			"处理条件分支、等待延迟、列表循环和批处理时，必须保留条件、限制、截断、耗时和取消状态。",
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

func buildBlueprintSkillPromptContext(ctx context.Context, store *Store, bp *AgentBlueprint) string {
	skillIDs := extractBlueprintSkillIDs(bp)
	userID := int64(0)
	if bp != nil {
		userID = bp.UserID
	}
	return buildAgentSkillPromptContextForIDs(ctx, store, userID, skillIDs)
}

func buildAgentNodeSkillPromptContext(ctx context.Context, store *Store, bp *AgentBlueprint, node agentBlueprintGraphNode) string {
	userID := int64(0)
	if bp != nil {
		userID = bp.UserID
	}
	return buildAgentSkillPromptContextForIDs(ctx, store, userID, []string{node.Data.Skill})
}

func buildAgentSkillPromptContextForIDs(ctx context.Context, store *Store, userID int64, skillIDs []string) string {
	skillIDs = uniqueNonEmptyStrings(skillIDs)
	if len(skillIDs) == 0 {
		return ""
	}

	installedSkills := map[string]AgentSkill{}
	if store != nil && userID > 0 {
		if skills, err := store.ListSkills(ctx, userID); err == nil {
			for _, skill := range skills {
				if skill.Enabled {
					installedSkills[skill.ID] = skill
					for _, alias := range installedSkillAliases(skill) {
						installedSkills[alias] = skill
					}
				}
			}
		}
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
		if skill, ok := installedSkills[skillID]; ok {
			if written == 0 {
				sb.WriteString("启用 Skill 约束：\n")
			}
			sb.WriteString(fmt.Sprintf("- %s：%s\n", skill.Name, skill.Description))
			for _, instruction := range installedSkillInstructions(skill) {
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

func installedSkillAliases(skill AgentSkill) []string {
	var cfg struct {
		MarketplaceID string `json:"marketplace_id"`
		GitHubURL     string `json:"github_url"`
		Source        string `json:"source"`
	}
	if len(skill.Config) > 0 {
		_ = json.Unmarshal(skill.Config, &cfg)
	}
	return uniqueNonEmptyStrings([]string{
		cfg.MarketplaceID,
		cfg.GitHubURL,
		cfg.Source,
		skill.SourceURL,
	})
}

func installedSkillInstructions(skill AgentSkill) []string {
	var cfg struct {
		Capabilities []string `json:"capabilities"`
		Instructions []string `json:"instructions"`
		Tags         []string `json:"tags"`
		ModelGroup   string   `json:"model_group"`
		ModelName    string   `json:"model_name"`
	}
	if len(skill.Config) > 0 {
		_ = json.Unmarshal(skill.Config, &cfg)
	}
	instructions := uniqueNonEmptyStrings(cfg.Instructions)
	if len(instructions) == 0 {
		if len(cfg.Capabilities) > 0 {
			instructions = append(instructions, "核心能力："+strings.Join(cfg.Capabilities, "、"))
		}
		instructions = append(instructions, "只作为当前节点的补充执行约束，不扩大未显式提供的业务上下文。")
	}
	if strings.TrimSpace(cfg.ModelName) != "" {
		instructions = append(instructions, "推荐模型："+strings.TrimSpace(cfg.ModelName))
	}
	return instructions
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
