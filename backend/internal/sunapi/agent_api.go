package sunapi

import (
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

	// ── Skill CRUD ────────────────────────────────────────────────────────
	api.GET("/agent-skills", handleListSkills(store))
	api.POST("/agent-skills", handleCreateSkill(store))
	api.PUT("/agent-skills/:id/toggle", handleToggleSkill(store))
	api.DELETE("/agent-skills/:id", handleDeleteSkill(store))
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
		if err := store.UpdateBlueprintStatus(c.Request.Context(), bpID, "paused", ""); err != nil {
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
		if err := store.UpdateBlueprintStatus(c.Request.Context(), bpID, "pending", ""); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		bp.Status = "pending"
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
		if bp.Status == "running" {
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
		ok(c, skills)
	}
}

func handleCreateSkill(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var f SkillFormData
		if !bindJSON(c, &f) {
			return
		}
		userID := playgroundUserID(c)
		sk, err := store.CreateSkill(c.Request.Context(), userID, f)
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
		if err := store.DeleteSkill(c.Request.Context(), id, userID); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, gin.H{"deleted": true})
	}
}
