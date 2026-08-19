package sunapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// registerAgentRoutes 注册所有智能体相关的管理 API 路由
func registerAgentRoutes(engine *gin.Engine, store *Store) {
	if engine == nil || store == nil {
		return
	}
	api := engine.Group("/api")
	api.Use(RequireAdmin(store))

	// ── 智能体 CRUD ─────────────────────────────────────────────────────
	api.GET("/agents", handleListAgents(store))
	api.POST("/agents", handleCreateAgent(store))
	api.PUT("/agents/:id", handleUpdateAgent(store))
	api.DELETE("/agents/:id", handleDeleteAgent(store))

	// ── 蓝图 CRUD ────────────────────────────────────────────────────────
	api.GET("/agents/:id/blueprints", handleListBlueprints(store))
	api.POST("/agents/:id/blueprints", handleCreateBlueprint(store))
	api.PUT("/agents/:id/blueprints/:bpID", handleUpdateBlueprint(store))
	api.DELETE("/agents/:id/blueprints/:bpID", handleDeleteBlueprint(store))

	// ── 蓝图操作（暂停/恢复/重试/立即执行）────────────────────────────────
	api.POST("/agents/:id/blueprints/:bpID/pause", handlePauseBlueprint(store))
	api.POST("/agents/:id/blueprints/:bpID/resume", handleResumeBlueprint(store))
	api.POST("/agents/:id/blueprints/:bpID/retry", handleRetryBlueprint(store))
	api.POST("/agents/:id/blueprints/:bpID/run", handleRunBlueprint(store))

	// ── 执行日志 ────────────────────────────────────────────────────────
	api.GET("/agents/:id/blueprints/:bpID/logs", handleListExecutionLogs(store))

	// ── 归档文件下载 ────────────────────────────────────────────────────
	api.GET("/agents/archives/files/*path", handleServeArchiveFile(store))

	// ── 创作台文件 ──────────────────────────────────────────────────────
	api.GET("/agents/files", handleListAgentFiles(store))
	api.POST("/agents/files", handleUploadAgentFile(store))
	api.POST("/agents/reports", handleWriteAgentReport(store))
	api.POST("/agents/browser-capture", handleCaptureAgentBrowserPage(store))
	api.POST("/agents/http-request", handleAgentHTTPRequest(store))

	// ── 创作台知识库 ────────────────────────────────────────────────────
	api.GET("/knowledge-bases", handleListKnowledgeBases(store))
	api.POST("/knowledge-bases", handleCreateKnowledgeBase(store))
	api.POST("/knowledge-bases/search", handleSearchKnowledge(store))
	api.PUT("/knowledge-bases/:id", handleUpdateKnowledgeBase(store))
	api.PUT("/knowledge-bases/:id/toggle", handleToggleKnowledgeBase(store))
	api.GET("/knowledge-bases/:id/bindings", handleListKnowledgeBindings(store))
	api.PUT("/knowledge-bases/:id/bindings", handleSetKnowledgeBindings(store))
	api.DELETE("/knowledge-bases/:id", handleDeleteKnowledgeBase(store))
	api.GET("/knowledge-bases/:id/documents", handleListKnowledgeDocuments(store))
	api.POST("/knowledge-bases/:id/documents", handleUploadKnowledgeDocument(store))
	api.DELETE("/knowledge-bases/:id/documents/:docID", handleDeleteKnowledgeDocument(store))

	// ── Skill CRUD ────────────────────────────────────────────────────────
	api.GET("/agent-skills", handleListSkills(store))
	api.GET("/agent-skills/marketplace", handleListSkillMarketplace(store))
	api.POST("/agent-skills", handleCreateSkill(store))
	api.PUT("/agent-skills/:id/toggle", handleToggleSkill(store))
	api.DELETE("/agent-skills/:id", handleDeleteSkill(store))

	// ── Component CRUD ────────────────────────────────────────────────────
	api.GET("/agent-components", handleListComponents(store))
	api.POST("/agent-components", handleCreateComponent(store))
	api.DELETE("/agent-components/:id", handleDeleteComponent(store))

	webhooks := engine.Group("/api/agent-webhooks")
	webhooks.Any("/:bpID", handleAgentWebhookTrigger(store))
}

// ── 智能体处理器 ──────────────────────────────────────────────────────────────

func handleListAgents(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := playgroundUserID(c)
		agents, err := store.ListAgents(c.Request.Context(), userID)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, agents)
	}
}

func handleCreateAgent(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var f AgentFormData
		if !bindJSON(c, &f) {
			return
		}
		userID := playgroundUserID(c)
		agent, err := store.CreateAgent(c.Request.Context(), userID, f)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, agent)
	}
}

func handleUpdateAgent(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		var f AgentFormData
		if !bindJSON(c, &f) {
			return
		}
		userID := playgroundUserID(c)
		agent, err := store.UpdateAgent(c.Request.Context(), id, userID, f)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, agent)
	}
}

func handleDeleteAgent(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		userID := playgroundUserID(c)
		if err := store.DeleteAgent(c.Request.Context(), id, userID); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, gin.H{"deleted": true})
	}
}

// ── 蓝图处理器 ────────────────────────────────────────────────────────────────

func handleListBlueprints(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := strings.TrimSpace(c.Param("id"))
		userID := playgroundUserID(c)
		bps, err := store.ListBlueprints(c.Request.Context(), agentID, userID)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, bps)
	}
}

func handleCreateBlueprint(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := strings.TrimSpace(c.Param("id"))
		var f BlueprintFormData
		if !bindJSON(c, &f) {
			return
		}
		userID := playgroundUserID(c)
		bp, err := store.CreateBlueprint(c.Request.Context(), agentID, userID, f)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, bp)
	}
}

func handleUpdateBlueprint(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		bpID := strings.TrimSpace(c.Param("bpID"))
		var f BlueprintFormData
		if !bindJSON(c, &f) {
			return
		}
		userID := playgroundUserID(c)
		bp, err := store.UpdateBlueprint(c.Request.Context(), bpID, userID, f)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, bp)
	}
}

func handleDeleteBlueprint(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		bpID := strings.TrimSpace(c.Param("bpID"))
		userID := playgroundUserID(c)
		if err := store.DeleteBlueprint(c.Request.Context(), bpID, userID); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, gin.H{"deleted": true})
	}
}

// ── 蓝图状态操作 ──────────────────────────────────────────────────────────────

func handlePauseBlueprint(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		bpID := strings.TrimSpace(c.Param("bpID"))
		userID := playgroundUserID(c)
		bp, err := store.GetBlueprint(c.Request.Context(), bpID, userID)
		if err != nil || bp == nil {
			fail(c, http.StatusNotFound, errNotFound("blueprint"))
			return
		}
		if err := store.UpdateBlueprintScheduleState(c.Request.Context(), bpID, "paused", "", 0); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		bp.Status = "paused"
		ok(c, bp)
	}
}

func handleResumeBlueprint(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		bpID := strings.TrimSpace(c.Param("bpID"))
		userID := playgroundUserID(c)
		bp, err := store.GetBlueprint(c.Request.Context(), bpID, userID)
		if err != nil || bp == nil {
			fail(c, http.StatusNotFound, errNotFound("blueprint"))
			return
		}
		status := "pending"
		nextExecutionTime := int64(0)
		if isAgentScheduledBlueprint(bp) {
			status = "running"
			var errNext error
			nextExecutionTime, errNext = nextAgentScheduleTimeFromBlueprint(bp, time.Now())
			if errNext != nil {
				fail(c, http.StatusBadRequest, errNext)
				return
			}
		}
		if err := store.UpdateBlueprintScheduleState(c.Request.Context(), bpID, status, "", nextExecutionTime); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		bp.Status = status
		bp.NextExecutionTime = nextExecutionTime
		ok(c, bp)
	}
}

func handleRetryBlueprint(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		bpID := strings.TrimSpace(c.Param("bpID"))
		userID := playgroundUserID(c)
		bp, err := store.GetBlueprint(c.Request.Context(), bpID, userID)
		if err != nil || bp == nil {
			fail(c, http.StatusNotFound, errNotFound("blueprint"))
			return
		}
		if err := store.UpdateBlueprintStatus(c.Request.Context(), bpID, "running", ""); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		// 异步执行蓝图
		go runBlueprintTask(store, bp)
		bp.Status = "running"
		ok(c, bp)
	}
}

func handleRunBlueprint(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		bpID := strings.TrimSpace(c.Param("bpID"))
		userID := playgroundUserID(c)
		bp, err := store.GetBlueprint(c.Request.Context(), bpID, userID)
		if err != nil || bp == nil {
			fail(c, http.StatusNotFound, errNotFound("blueprint"))
			return
		}
		if bp.Status == "running" || store.isAgentScheduledRunActive(bp.ID) {
			fail(c, http.StatusConflict, errConflict("blueprint is already running"))
			return
		}
		if err := store.UpdateBlueprintStatus(c.Request.Context(), bpID, "running", ""); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		go runBlueprintTask(store, bp)
		bp.Status = "running"
		ok(c, bp)
	}
}

func handleAgentWebhookTrigger(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		bpID := strings.TrimSpace(c.Param("bpID"))
		if bpID == "" {
			fail(c, http.StatusBadRequest, errBadRequest("blueprint id is required"))
			return
		}
		bp, err := store.GetBlueprintByID(c.Request.Context(), bpID)
		if err != nil || bp == nil {
			fail(c, http.StatusNotFound, errNotFound("blueprint"))
			return
		}
		if bp.Status == "paused" {
			fail(c, http.StatusConflict, errConflict("blueprint is paused"))
			return
		}
		if !blueprintHasWebhookTrigger(bp) {
			fail(c, http.StatusBadRequest, errBadRequest("blueprint does not contain a webhook trigger"))
			return
		}
		if err := validateAgentWebhookSecret(c, bp); err != nil {
			fail(c, http.StatusUnauthorized, err)
			return
		}

		payload, err := buildAgentWebhookInput(c)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		inputVars, _ := json.Marshal(map[string]interface{}{
			"webhook":         payload,
			"webhook_body":    payload["body"],
			"webhook_headers": payload["headers"],
			"webhook_query":   payload["query"],
			"webhook_method":  payload["method"],
			"webhook_path":    payload["path"],
			"triggered_at":    time.Now().Format(time.RFC3339),
		})
		runBP := *bp
		runBP.InputVariables = inputVars

		if err := store.UpdateBlueprintStatus(c.Request.Context(), bp.ID, "running", ""); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		go runBlueprintTask(store, &runBP)
		ok(c, gin.H{
			"accepted":     true,
			"blueprint_id": bp.ID,
			"agent_id":     bp.AgentID,
			"status":       "running",
		})
	}
}

func buildAgentWebhookInput(c *gin.Context) (map[string]interface{}, error) {
	bodyBytes, err := io.ReadAll(io.LimitReader(c.Request.Body, 2*1024*1024))
	if err != nil {
		return nil, err
	}
	var body interface{} = string(bodyBytes)
	if len(strings.TrimSpace(string(bodyBytes))) > 0 {
		var decoded interface{}
		if err := json.Unmarshal(bodyBytes, &decoded); err == nil {
			body = decoded
		}
	}
	headers := map[string]string{}
	for key, values := range c.Request.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	query := map[string]string{}
	for key, values := range c.Request.URL.Query() {
		if len(values) > 0 {
			query[key] = values[0]
		}
	}
	return map[string]interface{}{
		"method":       c.Request.Method,
		"path":         c.Request.URL.Path,
		"query":        query,
		"headers":      headers,
		"body":         body,
		"content_type": c.GetHeader("Content-Type"),
		"remote_addr":  c.ClientIP(),
	}, nil
}

func validateAgentWebhookSecret(c *gin.Context, bp *AgentBlueprint) error {
	secret := strings.TrimSpace(agentWebhookSecret(bp))
	if secret == "" {
		return nil
	}
	provided := firstNonEmpty(
		c.GetHeader("X-SunAPI-Webhook-Secret"),
		c.GetHeader("X-Webhook-Secret"),
		c.Query("secret"),
	)
	if provided != secret {
		return errUnauthorized("invalid webhook secret")
	}
	return nil
}

func blueprintHasWebhookTrigger(bp *AgentBlueprint) bool {
	nodes, err := parseAgentBlueprintNodes(bp.Nodes)
	if err != nil {
		return false
	}
	for _, node := range nodes {
		if node.Data.ComponentID == "component.webhook" {
			return true
		}
		if node.Data.Kind == "custom" && strings.Contains(strings.ToLower(node.Data.Title), "webhook") {
			return true
		}
	}
	return false
}

func agentWebhookSecret(bp *AgentBlueprint) string {
	nodes, err := parseAgentBlueprintNodes(bp.Nodes)
	if err == nil {
		for _, node := range nodes {
			if node.Data.ComponentID != "component.webhook" {
				continue
			}
			if secret := firstNonEmpty(node.Data.Settings["密钥"], node.Data.Settings["Secret"], node.Data.Settings["secret"]); secret != "" {
				return secret
			}
		}
	}
	var trigger map[string]interface{}
	if len(bp.TriggerConfig) > 0 {
		_ = json.Unmarshal(bp.TriggerConfig, &trigger)
	}
	if value, ok := trigger["secret"].(string); ok {
		return value
	}
	if value, ok := trigger["密钥"].(string); ok {
		return value
	}
	return ""
}

// ── 执行日志处理器 ────────────────────────────────────────────────────────────

func handleListExecutionLogs(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		bpID := strings.TrimSpace(c.Param("bpID"))
		userID := playgroundUserID(c)
		limit := 20
		if v := c.Query("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		logs, err := store.ListExecutionLogs(c.Request.Context(), bpID, userID, limit)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, logs)
	}
}

// ── 归档文件处理器 ────────────────────────────────────────────────────────────

func handleServeArchiveFile(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		relPath := strings.TrimSpace(c.Param("path"))
		full, err := agentArchiveFilePath(store.DataDir(), relPath)
		if err != nil {
			fail(c, http.StatusNotFound, errNotFound("file"))
			return
		}
		// 防止缓存过期，设置合理的缓存头
		c.Header("Cache-Control", "public, max-age=86400")
		c.Header("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		http.ServeFile(c.Writer, c.Request, full)
	}
}

// ── Skill 处理器 ──────────────────────────────────────────────────────────────

func handleListSkills(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := playgroundUserID(c)
		skills, err := store.ListSkills(c.Request.Context(), userID)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		skills = mergeBuiltinAgentSkills(userID, skills)
		ok(c, skills)
	}
}

func handleListSkillMarketplace(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := playgroundUserID(c)
		skills, err := store.ListSkills(c.Request.Context(), userID)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, listMarketplaceSkills(skills))
	}
}

func handleCreateSkill(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var f SkillFormData
		if !bindJSON(c, &f) {
			return
		}
		prepared, config, permissions, version, err := prepareSkillForm(c.Request.Context(), f)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		userID := playgroundUserID(c)
		sk, err := store.CreateSkill(c.Request.Context(), userID, prepared, config, permissions, version)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, sk)
	}
}

func handleToggleSkill(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		userID := playgroundUserID(c)
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if !bindJSON(c, &body) {
			return
		}
		if isBuiltinAgentSkillID(id) {
			ok(c, gin.H{"enabled": true, "builtin": true})
			return
		}
		if err := store.ToggleSkill(c.Request.Context(), id, userID, body.Enabled); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, gin.H{"enabled": body.Enabled})
	}
}

func handleDeleteSkill(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		userID := playgroundUserID(c)
		if isBuiltinAgentSkillID(id) {
			fail(c, http.StatusBadRequest, errConflict("built-in skills cannot be removed"))
			return
		}
		if err := store.DeleteSkill(c.Request.Context(), id, userID); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, gin.H{"deleted": true})
	}
}

func handleListComponents(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := playgroundUserID(c)
		customComponents, err := store.ListComponents(c.Request.Context(), userID)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, mergeBuiltinAgentComponents(userID, customComponents))
	}
}

func handleCreateComponent(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var f ComponentFormData
		if !bindJSON(c, &f) {
			return
		}
		prepared, config, err := prepareComponentForm(f)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		userID := playgroundUserID(c)
		component, err := store.CreateComponent(c.Request.Context(), userID, prepared, config)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, component)
	}
}

func handleDeleteComponent(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		if strings.HasPrefix(id, "component.") {
			fail(c, http.StatusBadRequest, errConflict("built-in components cannot be removed"))
			return
		}
		userID := playgroundUserID(c)
		if err := store.DeleteComponent(c.Request.Context(), id, userID); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, gin.H{"deleted": true})
	}
}
