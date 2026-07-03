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
	agentArchiveDirName     = "agent-archives"
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
		"blueprint_id":  bp.ID,
		"model":         bp.ModelName,
		"model_group":   bp.ModelGroup,
		"input_vars":    inputVars,
		"result":        result,
		"executed_at":   time.Now().UnixMilli(),
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
	// 构建系统 Prompt
	systemPrompt := buildBlueprintSystemPrompt(bp, inputVars)

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

func buildBlueprintSystemPrompt(bp *AgentBlueprint, inputVars map[string]interface{}) string {
	var sb strings.Builder
	sb.WriteString("你是一个自动化任务执行智能体。\n")
	sb.WriteString("蓝图名称：" + bp.Name + "\n")
	if bp.Description != "" {
		sb.WriteString("任务描述：" + bp.Description + "\n")
	}
	if len(inputVars) > 0 {
		sb.WriteString("输入变量：\n")
		for k, v := range inputVars {
			sb.WriteString(fmt.Sprintf("  - %s: %v\n", k, v))
		}
	}
	sb.WriteString("\n请根据以上任务要求，完成分析并输出结构化的报告。")
	return sb.String()
}

func buildBlueprintUserPrompt(bp *AgentBlueprint, inputVars map[string]interface{}) string {
	if bp.Description != "" {
		return bp.Description
	}
	return "请执行蓝图任务：" + bp.Name
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
