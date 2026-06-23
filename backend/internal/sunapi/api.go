package sunapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api/handlers"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api/handlers/openai"
)

var startedAt = time.Now()

func truthyQuery(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func supportsImageGenerationModel(model string) bool {
	base := routeModelBaseName(model)
	if strings.Contains(base, "image") {
		return true
	}
	if base == "gpt-image-2" ||
		base == "grok-imagine-image" ||
		base == "grok-imagine-image-quality" {
		return true
	}
	if info := registry.LookupModelInfo(model); info != nil && info.Type == registry.OpenAIImageModelType {
		return true
	}
	return false
}

func supportsImageGenerationModelForChannel(model string, channel Channel) bool {
	return supportsImageGenerationModel(model)
}

func supportsVideoGenerationModel(model string) bool {
	base := routeModelBaseName(model)
	if base == "grok-imagine-video" || base == "grok-imagine-video-1.5-preview" {
		return true
	}
	if info := registry.LookupStaticModelInfo(base); info != nil {
		text := strings.ToLower(strings.Join([]string{info.ID, info.DisplayName, info.Name, info.Description}, " "))
		return strings.Contains(text, "video")
	}
	return false
}

func routeModelBaseName(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	if idx := strings.LastIndex(model, "/"); idx >= 0 && idx < len(model)-1 {
		return strings.TrimSpace(model[idx+1:])
	}
	return model
}

func RegisterRoutes(engine *gin.Engine, store *Store, writer *ConfigWriter) {
	if engine == nil || store == nil {
		return
	}

	api := engine.Group("/api")
	api.GET("/status", func(c *gin.Context) {
		settings, err := store.Settings(c.Request.Context())
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, gin.H{
			"system_name":                   settings.SystemName,
			"logo":                          "/sun-logo.svg",
			"footer_html":                   "",
			"demo_site_enabled":             false,
			"display_token_stat_enabled":    true,
			"display_in_currency":           true,
			"quota_display_type":            "USD",
			"quota_per_unit":                localQuotaPerUnit,
			"usd_exchange_rate":             1,
			"custom_currency_symbol":        settings.CurrencySymbol,
			"custom_currency_exchange_rate": 1,
			"api_info_enabled":              false,
			"uptime_kuma_enabled":           false,
			"announcements_enabled":         false,
			"faq_enabled":                   false,
			"register_enabled":              false,
			"email_verification_enabled":    false,
			"github_oauth":                  false,
			"oidc_enabled":                  false,
			"server_address":                localServerURL(settings),
			"default_start_page":            settings.DefaultStartPage,
			"show_dashboard":                settings.ShowDashboard,
			"show_api_keys":                 settings.ShowAPIKeys,
			"show_usage_logs":               settings.ShowUsageLogs,
			"show_playground":               settings.ShowPlayground,
			"version":                       "local",
			"start_time":                    startedAt.Unix(),
		})
	})
	api.GET("/notice", func(c *gin.Context) {
		ok(c, "")
	})
	api.GET("/home_page_content", func(c *gin.Context) {
		ok(c, "")
	})
	api.GET("/uptime/status", func(c *gin.Context) {
		ok(c, []any{})
	})
	RegisterAuthRoutes(api, store)
	api.Use(RequireAdmin(store))
	api.GET("/user/self", func(c *gin.Context) {
		usedQuota, requestCount, err := store.UserQuotaSummary(c.Request.Context())
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		remainingQuota := localQuotaPerUnit*localQuotaBalanceUSD - usedQuota
		if remainingQuota < 0 {
			remainingQuota = 0
		}
		ok(c, gin.H{
			"id":            1,
			"username":      "local",
			"display_name":  "SunAPI Local",
			"role":          100,
			"status":        1,
			"group":         "default",
			"quota":         remainingQuota,
			"used_quota":    usedQuota,
			"request_count": requestCount,
			"permissions":   gin.H{"sidebar_settings": false},
		})
	})
	api.GET("/user/models", func(c *gin.Context) {
		groupFilter := strings.TrimSpace(c.Query("group"))
		if groupFilter != "" {
			groupFilter = normalizeGroupName(groupFilter)
		}
		withDetails := truthyQuery(c.Query("details"))
		channels, err := store.ListChannels(c.Request.Context(), false)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		seen := map[string]struct{}{}
		models := make([]string, 0)
		details := make([]gin.H, 0)
		detailIndex := map[string]int{}
		for _, channel := range channels {
			if !channel.Enabled {
				continue
			}
			if groupFilter != "" && normalizeGroupName(channel.Group) != groupFilter {
				continue
			}
			for _, model := range modelList(channel.Models) {
				if _, exists := seen[model]; exists {
					if withDetails {
						if index, ok := detailIndex[model]; ok {
							supportsImage, _ := details[index]["supports_image"].(bool)
							supportsVideo, _ := details[index]["supports_video"].(bool)
							details[index]["supports_image"] = supportsImage || supportsImageGenerationModelForChannel(model, channel)
							details[index]["supports_video"] = supportsVideo || supportsVideoGenerationModel(model)
						}
					}
					continue
				}
				seen[model] = struct{}{}
				models = append(models, model)
				if withDetails {
					detailIndex[model] = len(details)
					details = append(details, gin.H{
						"label":          model,
						"value":          model,
						"supports_image": supportsImageGenerationModelForChannel(model, channel),
						"supports_video": supportsVideoGenerationModel(model),
					})
				}
			}
		}
		if withDetails {
			ok(c, details)
			return
		}
		ok(c, models)
	})
	api.GET("/user/self/groups", func(c *gin.Context) {
		groups, err := store.ListGroups(c.Request.Context())
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		out := make(map[string]gin.H, len(groups))
		for _, group := range groups {
			out[group.Name] = gin.H{
				"desc":  group.Description,
				"ratio": group.PriceMultiplier,
			}
		}
		ok(c, out)
	})
	api.GET("/user/:id", func(c *gin.Context) {
		usedQuota, requestCount, err := store.UserQuotaSummary(c.Request.Context())
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		remainingQuota := localQuotaPerUnit*localQuotaBalanceUSD - usedQuota
		if remainingQuota < 0 {
			remainingQuota = 0
		}
		ok(c, gin.H{
			"id":            1,
			"username":      "local",
			"display_name":  "SunAPI Local",
			"quota":         remainingQuota,
			"used_quota":    usedQuota,
			"request_count": requestCount,
			"group":         "default",
			"remark":        "Local single-user account",
		})
	})

	api.GET("/dashboard", func(c *gin.Context) {
		data, err := store.Dashboard(c.Request.Context())
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, data)
	})
	api.GET("/data", func(c *gin.Context) {
		handleQuotaData(c, store, false)
	})
	api.GET("/data/self", func(c *gin.Context) {
		handleQuotaData(c, store, false)
	})
	api.GET("/data/users", func(c *gin.Context) {
		handleQuotaData(c, store, true)
	})
	api.POST("/data/repair_unknown_groups", func(c *gin.Context) {
		start, end := requestTimeRange(c)
		result, err := store.RepairUnknownQuotaDataGroups(c.Request.Context(), start, end, c.Query("username"))
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, result)
	})
	api.POST("/data/recalculate_quota", func(c *gin.Context) {
		start, end := requestTimeRange(c)
		onlyZero := true
		if raw := strings.TrimSpace(c.Query("only_zero")); raw != "" {
			if parsed, err := strconv.ParseBool(raw); err == nil {
				onlyZero = parsed
			}
		}
		if all, _ := strconv.ParseBool(c.DefaultQuery("all", "false")); all {
			onlyZero = false
		}
		result, err := store.RecalculateUsageCosts(c.Request.Context(), start, end, c.Query("username"), onlyZero)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, result)
	})
	api.GET("/perf-metrics/summary", func(c *gin.Context) {
		hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
		models, err := store.PerfMetricsSummary(c.Request.Context(), hours)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, gin.H{"models": models})
	})
	api.GET("/perf-metrics", func(c *gin.Context) {
		hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
		data, err := store.PerfMetrics(c.Request.Context(), c.Query("model"), hours)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, data)
	})
	api.POST("/channel-metadata/sync", func(c *gin.Context) {
		var payload ChannelMetadataPayload
		if !bindJSON(c, &payload) {
			return
		}
		metadata := syncChannelMetadata(c.Request.Context(), payload)
		ok(c, metadata)
	})

	registerPlaygroundSessionRoutes(api, store)

	api.GET("/channels", func(c *gin.Context) {
		items, err := store.ListChannels(c.Request.Context(), false)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, gin.H{"items": items, "total": len(items)})
	})
	api.POST("/channels", func(c *gin.Context) {
		var payload ChannelPayload
		if !bindJSON(c, &payload) {
			return
		}
		channel, err := store.CreateChannel(c.Request.Context(), payload)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		if err := refreshConfig(c.Request.Context(), writer); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, channel)
	})
	api.PUT("/channels/:id", func(c *gin.Context) {
		id, parsed := routeID(c)
		if !parsed {
			return
		}
		var payload ChannelPayload
		if !bindJSON(c, &payload) {
			return
		}
		channel, err := store.UpdateChannel(c.Request.Context(), id, payload)
		if err != nil {
			failCRUD(c, err)
			return
		}
		if err := refreshConfig(c.Request.Context(), writer); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, channel)
	})
	api.PATCH("/channels/:id/enabled", func(c *gin.Context) {
		id, parsed := routeID(c)
		if !parsed {
			return
		}
		var payload struct {
			Enabled bool `json:"enabled"`
		}
		if !bindJSON(c, &payload) {
			return
		}
		channel, err := store.SetChannelEnabled(c.Request.Context(), id, payload.Enabled)
		if err != nil {
			failCRUD(c, err)
			return
		}
		if err := refreshConfig(c.Request.Context(), writer); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, channel)
	})
	api.DELETE("/channels/:id", func(c *gin.Context) {
		id, parsed := routeID(c)
		if !parsed {
			return
		}
		if err := store.DeleteChannel(c.Request.Context(), id); err != nil {
			failCRUD(c, err)
			return
		}
		if err := refreshConfig(c.Request.Context(), writer); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, gin.H{"id": id})
	})

	api.GET("/channel", func(c *gin.Context) {
		handleChannelList(c, store, false)
	})
	api.GET("/channel/search", func(c *gin.Context) {
		handleChannelList(c, store, true)
	})
	api.GET("/channel/detail/:id", func(c *gin.Context) {
		id, parsed := routeID(c)
		if !parsed {
			return
		}
		channel, err := store.GetChannel(c.Request.Context(), id, false)
		if err != nil {
			failCRUD(c, err)
			return
		}
		ok(c, channel)
	})
	api.POST("/channel", func(c *gin.Context) {
		var request AddChannelRequest
		if !bindJSON(c, &request) {
			return
		}
		channel, err := store.CreateChannel(c.Request.Context(), channelPayloadFromAddRequest(request))
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		if err := refreshConfig(c.Request.Context(), writer); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, gin.H{"id": channel.ID})
	})
	api.PUT("/channel/", func(c *gin.Context) {
		handleChannelPatch(c, store, writer)
	})
	api.PUT("/channel", func(c *gin.Context) {
		handleChannelPatch(c, store, writer)
	})
	api.DELETE("/channel/delete/:id", func(c *gin.Context) {
		id, parsed := routeID(c)
		if !parsed {
			return
		}
		if err := store.DeleteChannel(c.Request.Context(), id); err != nil {
			failCRUD(c, err)
			return
		}
		if err := refreshConfig(c.Request.Context(), writer); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, gin.H{"id": id})
	})
	api.GET("/channel/test/:id", func(c *gin.Context) {
		id, parsed := routeID(c)
		if !parsed {
			return
		}
		isStream, _ := strconv.ParseBool(c.Query("stream"))
		result, err := testLocalChannel(c.Request.Context(), store, id, channelTestOptions{
			Model:        c.Query("model"),
			EndpointType: c.Query("endpoint_type"),
			Stream:       isStream,
		})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
				"data":    nil,
			})
			return
		}
		ok(c, result)
	})
	api.GET("/channel/test", func(c *gin.Context) {
		channels, err := store.ListChannels(c.Request.Context(), false)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		var failures int
		for _, channel := range channels {
			if !channel.Enabled {
				continue
			}
			if _, err := testLocalChannel(c.Request.Context(), store, channel.ID, channelTestOptions{}); err != nil {
				failures++
			}
		}
		if failures > 0 {
			ok(c, gin.H{"failures": failures})
			return
		}
		ok(c, gin.H{"failures": 0})
	})
	api.GET("/channel/fetch_models/:id", func(c *gin.Context) {
		id, parsed := routeID(c)
		if !parsed {
			return
		}
		channel, err := store.GetChannel(c.Request.Context(), id, true)
		if err != nil {
			failCRUD(c, err)
			return
		}
		models, err := fetchChannelModels(c.Request.Context(), ChannelMetadataPayload{
			Type:    channel.Type,
			BaseURL: channel.BaseURL,
			APIKey:  channel.APIKey,
		})
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, models)
	})
	api.POST("/channel/fetch_models", func(c *gin.Context) {
		var payload struct {
			Type    int    `json:"type"`
			BaseURL string `json:"base_url"`
			Key     string `json:"key"`
			APIKey  string `json:"api_key"`
		}
		if !bindJSON(c, &payload) {
			return
		}
		apiKey := payload.APIKey
		if apiKey == "" {
			apiKey = payload.Key
		}
		models, err := fetchChannelModels(c.Request.Context(), ChannelMetadataPayload{
			Type:    payload.Type,
			BaseURL: payload.BaseURL,
			APIKey:  apiKey,
		})
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, models)
	})
	api.GET("/channel/models", func(c *gin.Context) {
		models, err := store.AllModels(c.Request.Context(), false)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		out := make([]gin.H, 0, len(models))
		for _, model := range models {
			out = append(out, gin.H{"id": model})
		}
		ok(c, out)
	})
	api.GET("/channel/models_enabled", func(c *gin.Context) {
		models, err := store.AllModels(c.Request.Context(), true)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, models)
	})
	api.GET("/channel/update_balance/:id", func(c *gin.Context) {
		id, parsed := routeID(c)
		if !parsed {
			return
		}
		channel, err := store.GetChannel(c.Request.Context(), id, false)
		if err != nil {
			failCRUD(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "balance": channel.Balance, "currency": "USD"})
	})
	api.GET("/channel/update_balance", func(c *gin.Context) {
		ok(c, gin.H{})
	})
	api.POST("/channel/reset_used_quota/:id", func(c *gin.Context) {
		id, parsed := routeID(c)
		if !parsed {
			return
		}
		if err := store.ResetChannelUsedQuota(c.Request.Context(), id); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, gin.H{"id": id})
	})
	api.POST("/channel/reset_used_quota", func(c *gin.Context) {
		deleted, err := store.ResetAllChannelUsedQuota(c.Request.Context())
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, deleted)
	})
	api.DELETE("/channel/disabled", func(c *gin.Context) {
		deleted, err := store.DeleteDisabledChannels(c.Request.Context())
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		if err := refreshConfig(c.Request.Context(), writer); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, deleted)
	})
	api.POST("/channel/copy/:id", func(c *gin.Context) {
		id, parsed := routeID(c)
		if !parsed {
			return
		}
		channel, err := store.CloneChannel(c.Request.Context(), id, c.DefaultQuery("suffix", "_copy"))
		if err != nil {
			failCRUD(c, err)
			return
		}
		if err := refreshConfig(c.Request.Context(), writer); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, gin.H{"id": channel.ID})
	})
	api.POST("/channel/fix", func(c *gin.Context) {
		ok(c, gin.H{"success": 0, "fails": 0})
	})
	api.POST("/channel/batch", func(c *gin.Context) {
		var payload struct {
			IDs []int64 `json:"ids"`
		}
		if !bindJSON(c, &payload) {
			return
		}
		var deleted int64
		for _, id := range payload.IDs {
			if id <= 0 {
				continue
			}
			if err := store.DeleteChannel(c.Request.Context(), id); err == nil {
				deleted++
			}
		}
		if err := refreshConfig(c.Request.Context(), writer); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, deleted)
	})
	api.POST("/channel/batch/tag", func(c *gin.Context) {
		ok(c, 0)
	})
	api.PUT("/channel/tag", func(c *gin.Context) {
		ok(c, gin.H{})
	})
	api.GET("/channel/tag/models", func(c *gin.Context) {
		ok(c, "")
	})
	api.POST("/channel/tag/enabled", func(c *gin.Context) {
		ok(c, gin.H{})
	})
	api.POST("/channel/tag/disabled", func(c *gin.Context) {
		ok(c, gin.H{})
	})
	api.POST("/channel/key/:id", func(c *gin.Context) {
		id, parsed := routeID(c)
		if !parsed {
			return
		}
		channel, err := store.GetChannel(c.Request.Context(), id, true)
		if err != nil {
			failCRUD(c, err)
			return
		}
		ok(c, gin.H{"key": channel.APIKey})
	})

	api.GET("/groups", func(c *gin.Context) {
		items, err := store.ListGroups(c.Request.Context())
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, gin.H{"items": items, "total": len(items)})
	})
	api.POST("/groups", func(c *gin.Context) {
		var payload GroupPayload
		if !bindJSON(c, &payload) {
			return
		}
		group, err := store.CreateGroup(c.Request.Context(), payload)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		if err := refreshConfig(c.Request.Context(), writer); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, group)
	})
	api.PUT("/groups/:id", func(c *gin.Context) {
		id, parsed := routeID(c)
		if !parsed {
			return
		}
		var payload GroupPayload
		if !bindJSON(c, &payload) {
			return
		}
		group, err := store.UpdateGroup(c.Request.Context(), id, payload)
		if err != nil {
			failCRUD(c, err)
			return
		}
		if err := refreshConfig(c.Request.Context(), writer); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, group)
	})
	api.DELETE("/groups/:id", func(c *gin.Context) {
		id, parsed := routeID(c)
		if !parsed {
			return
		}
		if err := store.DeleteGroup(c.Request.Context(), id); err != nil {
			failCRUD(c, err)
			return
		}
		if err := refreshConfig(c.Request.Context(), writer); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, gin.H{"id": id})
	})

	api.GET("/group/", func(c *gin.Context) {
		groups, err := store.ListGroups(c.Request.Context())
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, groupNames(groups))
	})
	api.GET("/prefill_group", func(c *gin.Context) {
		ok(c, []gin.H{})
	})
	api.GET("/option/", func(c *gin.Context) {
		settings, err := store.Settings(c.Request.Context())
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		groups, err := store.ListGroups(c.Request.Context())
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, optionRows(settings, groups))
	})
	api.PUT("/option/", func(c *gin.Context) {
		var payload struct {
			Key   string `json:"key"`
			Value any    `json:"value"`
		}
		if !bindJSON(c, &payload) {
			return
		}
		if err := applyLocalOption(c.Request.Context(), store, payload.Key, fmt.Sprint(payload.Value)); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		if err := refreshConfig(c.Request.Context(), writer); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
	})

	api.Any("/token", func(c *gin.Context) {
		handleTokenAPI(c, store)
	})
	api.Any("/token/*path", func(c *gin.Context) {
		handleTokenAPI(c, store)
	})

	api.GET("/log", func(c *gin.Context) {
		handleCompatUsageLogs(c, store)
	})
	api.GET("/log/self", func(c *gin.Context) {
		handleCompatUsageLogs(c, store)
	})
	api.GET("/log/stat", func(c *gin.Context) {
		handleCompatUsageLogStats(c, store)
	})
	api.GET("/log/self/stat", func(c *gin.Context) {
		handleCompatUsageLogStats(c, store)
	})
	api.GET("/mj", handleEmptyCompatLogList)
	api.GET("/mj/self", handleEmptyCompatLogList)
	api.GET("/task", handleEmptyCompatLogList)
	api.GET("/task/self", handleEmptyCompatLogList)

	api.GET("/usage-logs", func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "500"))
		items, err := store.ListUsageLogs(c.Request.Context(), limit)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		total, err := store.CountUsageLogs(c.Request.Context())
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, gin.H{"items": items, "total": total})
	})
	api.DELETE("/usage-logs", func(c *gin.Context) {
		deleted, err := store.ClearUsageLogs(c.Request.Context())
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, gin.H{"deleted": deleted})
	})

	api.GET("/settings", func(c *gin.Context) {
		settings, err := store.Settings(c.Request.Context())
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, settings)
	})
	api.PUT("/settings", func(c *gin.Context) {
		var payload Settings
		if !bindJSON(c, &payload) {
			return
		}
		settings, err := store.UpdateSettings(c.Request.Context(), payload)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		if err := refreshConfig(c.Request.Context(), writer); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, settings)
	})
}

func RegisterPlaygroundRoutes(engine *gin.Engine, store *Store, base *handlers.BaseAPIHandler) {
	if engine == nil || store == nil || base == nil {
		return
	}

	openaiHandlers := openai.NewOpenAIAPIHandler(base)
	registerPlaygroundImageTaskRoutes(engine, store, openaiHandlers)

	pg := engine.Group("/pg")
	pg.Use(RequireAdmin(store))
	pg.Use(playgroundAccessMetadata())
	pg.Use(playgroundResolveAttachments(store))
	{
		pg.POST("/chat/completions", openaiHandlers.ChatCompletions)
		pg.POST("/images/generations", openaiHandlers.ImagesGenerations)
		pg.POST("/images/edits", openaiHandlers.ImagesEdits)
		pg.POST("/videos", openaiHandlers.XAIVideosGenerations)
		pg.GET("/videos/:request_id", openaiHandlers.XAIVideosRetrieve)
	}
}

func registerPlaygroundSessionRoutes(api *gin.RouterGroup, store *Store) {
	api.POST("/playground/attachments", func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		if file.Size <= 0 {
			fail(c, http.StatusBadRequest, errors.New("attachment is empty"))
			return
		}
		if file.Size > 12*1024*1024 {
			fail(c, http.StatusBadRequest, errors.New("attachment is too large"))
			return
		}

		src, err := file.Open()
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		defer src.Close()

		data, err := io.ReadAll(io.LimitReader(src, 12*1024*1024+1))
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		if len(data) > 12*1024*1024 {
			fail(c, http.StatusBadRequest, errors.New("attachment is too large"))
			return
		}

		mediaType := strings.TrimSpace(file.Header.Get("Content-Type"))
		if mediaType == "" {
			mediaType = http.DetectContentType(data)
		}
		if parsed, _, err := mime.ParseMediaType(mediaType); err == nil {
			mediaType = parsed
		}
		if !strings.HasPrefix(strings.ToLower(mediaType), "image/") {
			fail(c, http.StatusBadRequest, errors.New("only image attachments are supported"))
			return
		}

		attachment, err := store.SavePlaygroundAttachment(c.Request.Context(), playgroundUserID(c), mediaType, file.Filename, data)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, attachment)
	})
	api.GET("/playground/attachments/:id", func(c *gin.Context) {
		attachment, err := store.GetPlaygroundAttachment(c.Request.Context(), playgroundUserID(c), c.Param("id"))
		if err != nil {
			failCRUD(c, err)
			return
		}
		c.Header("Cache-Control", "private, max-age=86400")
		if attachment.Filename != "" {
			c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", attachment.Filename))
		}
		c.Data(http.StatusOK, attachment.MediaType, attachment.Data)
	})
	api.GET("/playground/image-history/files/*path", func(c *gin.Context) {
		path, err := playgroundImageHistoryFilePath(store.DataDir(), c.Param("path"))
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		c.Header("Cache-Control", "private, max-age=86400")
		c.File(path)
	})
	api.GET("/playground/image-history", func(c *gin.Context) {
		_, size := pageParams(c, 50)
		items, err := store.ListPlaygroundImageGenerations(c.Request.Context(), playgroundUserID(c), size)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, gin.H{"items": items, "total": len(items)})
	})
	api.POST("/playground/image-history", func(c *gin.Context) {
		var payload PlaygroundImageGeneration
		if !bindJSON(c, &payload) {
			return
		}
		if len(payload.URLs) > 0 {
			payload.URLs = persistPlaygroundImageURLs(c.Request.Context(), store.DataDir(), payload.ID, payload.URLs, payload.CreatedAt)
		}
		item, err := store.SavePlaygroundImageGeneration(c.Request.Context(), playgroundUserID(c), payload)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, item)
	})
	api.DELETE("/playground/image-history/:id", func(c *gin.Context) {
		urls, err := store.DeletePlaygroundImageGeneration(c.Request.Context(), playgroundUserID(c), c.Param("id"))
		if err != nil {
			failCRUD(c, err)
			return
		}
		deleteStoredPlaygroundImageURLs(store.DataDir(), urls)
		ok(c, gin.H{"id": c.Param("id")})
	})

	api.GET("/playground/sessions", func(c *gin.Context) {
		page, size := pageParams(c, 50)
		items, total, err := store.ListPlaygroundSessions(c.Request.Context(), playgroundUserID(c), page, size)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, gin.H{
			"items":     items,
			"total":     total,
			"page":      page,
			"page_size": size,
		})
	})
	api.POST("/playground/sessions", func(c *gin.Context) {
		var payload PlaygroundSessionPayload
		if !bindJSON(c, &payload) {
			return
		}
		session, err := store.SavePlaygroundSession(c.Request.Context(), playgroundUserID(c), payload)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, summarizePlaygroundSession(session))
	})
	api.DELETE("/playground/sessions", func(c *gin.Context) {
		if err := store.DeleteAllPlaygroundSessions(c.Request.Context(), playgroundUserID(c)); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, nil)
	})
	api.GET("/playground/sessions/:id", func(c *gin.Context) {
		id, parsed := routeID(c)
		if !parsed {
			return
		}
		session, err := store.GetPlaygroundSession(c.Request.Context(), playgroundUserID(c), id)
		if err != nil {
			failCRUD(c, err)
			return
		}
		ok(c, session)
	})
	api.PUT("/playground/sessions/:id", func(c *gin.Context) {
		id, parsed := routeID(c)
		if !parsed {
			return
		}
		var payload PlaygroundSessionPayload
		if !bindJSON(c, &payload) {
			return
		}
		payload.ID = id
		session, err := store.SavePlaygroundSession(c.Request.Context(), playgroundUserID(c), payload)
		if err != nil {
			failCRUD(c, err)
			return
		}
		ok(c, summarizePlaygroundSession(session))
	})
	api.PATCH("/playground/sessions/:id", func(c *gin.Context) {
		id, parsed := routeID(c)
		if !parsed {
			return
		}
		var payload PlaygroundSessionMetaPayload
		if !bindJSON(c, &payload) {
			return
		}
		session, err := store.PatchPlaygroundSession(c.Request.Context(), playgroundUserID(c), id, payload)
		if err != nil {
			failCRUD(c, err)
			return
		}
		ok(c, summarizePlaygroundSession(session))
	})
	api.DELETE("/playground/sessions/:id", func(c *gin.Context) {
		id, parsed := routeID(c)
		if !parsed {
			return
		}
		if err := store.DeletePlaygroundSession(c.Request.Context(), playgroundUserID(c), id); err != nil {
			failCRUD(c, err)
			return
		}
		ok(c, nil)
	})
}

func bindJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		fail(c, http.StatusBadRequest, err)
		return false
	}
	return true
}

func pageParams(c *gin.Context, defaultSize int) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("p", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", c.DefaultQuery("size", strconv.Itoa(defaultSize))))
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = defaultSize
	}
	return page, size
}

func playgroundUserID(c *gin.Context) int64 {
	if value, ok := c.Get("sunapi_admin_user"); ok {
		if user, ok := value.(AdminUser); ok && user.ID > 0 {
			return user.ID
		}
	}
	return 1
}

func summarizePlaygroundSession(session PlaygroundSession) PlaygroundSession {
	session.Messages = nil
	return session
}

func routeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		fail(c, http.StatusBadRequest, errors.New("invalid id"))
		return 0, false
	}
	return id, true
}

func requestTimeRange(c *gin.Context) (int64, int64) {
	start, _ := strconv.ParseInt(strings.TrimSpace(c.Query("start_timestamp")), 10, 64)
	end, _ := strconv.ParseInt(strings.TrimSpace(c.Query("end_timestamp")), 10, 64)
	return normalizeTimeRange(start, end)
}

func handleQuotaData(c *gin.Context, store *Store, byUser bool) {
	start, end := requestTimeRange(c)
	username := strings.TrimSpace(c.Query("username"))
	repairUsername := username
	if !byUser && strings.EqualFold(c.FullPath(), "/api/data/self") {
		repairUsername = "local"
	}
	_, _ = store.RecalculateUsageCosts(c.Request.Context(), start, end, repairUsername, true)
	var (
		items []QuotaData
		err   error
	)
	if byUser {
		items, err = store.QueryQuotaData(c.Request.Context(), start, end, username, true)
	} else if strings.EqualFold(c.FullPath(), "/api/data/self") {
		items, err = store.QueryQuotaDataByUsername(c.Request.Context(), start, end, "local")
	} else {
		items, err = store.QueryQuotaData(c.Request.Context(), start, end, username, false)
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	ok(c, items)
}

func handleCompatUsageLogs(c *gin.Context, store *Store) {
	page, _ := strconv.Atoi(c.DefaultQuery("p", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", c.DefaultQuery("size", "20")))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 20
	}
	where, args := buildUsageLogWhere(c)

	var total int64
	if err := store.db.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM usage_logs`+where, args...).Scan(&total); err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}

	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := store.db.QueryContext(c.Request.Context(), `SELECT id, created_at, channel_id, channel_name,
		username, group_name, model, endpoint, prompt_tokens, completion_tokens, total_tokens, cost, duration_ms,
		status_code, error FROM usage_logs`+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()

	items := make([]gin.H, 0)
	for rows.Next() {
		var log UsageLog
		if err := rows.Scan(
			&log.ID,
			&log.CreatedAt,
			&log.ChannelID,
			&log.ChannelName,
			&log.Username,
			&log.Group,
			&log.Model,
			&log.Endpoint,
			&log.PromptTokens,
			&log.CompletionTokens,
			&log.TotalTokens,
			&log.Cost,
			&log.DurationMS,
			&log.StatusCode,
			&log.Error,
		); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		items = append(items, compatUsageLogRow(log))
	}
	if err := rows.Err(); err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}

	ok(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func handleCompatUsageLogStats(c *gin.Context, store *Store) {
	where, args := buildUsageLogWhere(c)
	var cost float64
	var tokens, requests int64
	if err := store.db.QueryRowContext(c.Request.Context(), `SELECT COALESCE(SUM(cost), 0),
		COALESCE(SUM(total_tokens), 0), COUNT(*) FROM usage_logs`+where, args...).Scan(&cost, &tokens, &requests); err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	start, end := requestTimeRange(c)
	minutes := float64(end-start) / 60
	if minutes <= 0 {
		minutes = 1
	}
	ok(c, gin.H{
		"quota": quotaFromCost(cost),
		"rpm":   float64(requests) / minutes,
		"tpm":   float64(tokens) / minutes,
	})
}

func handleEmptyCompatLogList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("p", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", c.DefaultQuery("size", "20")))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	ok(c, gin.H{
		"items":     []gin.H{},
		"total":     0,
		"page":      page,
		"page_size": pageSize,
	})
}

func buildUsageLogWhere(c *gin.Context) (string, []any) {
	start, end := requestTimeRange(c)
	clauses := []string{"created_at >= ?", "created_at <= ?"}
	args := []any{start, end}

	if model := strings.TrimSpace(c.Query("model_name")); model != "" {
		clauses = append(clauses, "model LIKE ?")
		args = append(args, "%"+model+"%")
	}
	if group := strings.TrimSpace(c.Query("group")); group != "" {
		clauses = append(clauses, "group_name = ?")
		args = append(args, group)
	}
	if username := strings.TrimSpace(c.Query("username")); username != "" {
		clauses = append(clauses, "COALESCE(NULLIF(TRIM(username), ''), 'local') = ?")
		args = append(args, username)
	}
	if channelID, _ := strconv.ParseInt(strings.TrimSpace(c.Query("channel")), 10, 64); channelID > 0 {
		clauses = append(clauses, "channel_id = ?")
		args = append(args, channelID)
	}

	logType, _ := strconv.Atoi(strings.TrimSpace(c.Query("type")))
	switch logType {
	case 0:
	case 2:
		clauses = append(clauses, "status_code < 400 AND TRIM(COALESCE(error, '')) = ''")
	case 5:
		clauses = append(clauses, "(status_code >= 400 OR TRIM(COALESCE(error, '')) != '')")
	default:
		clauses = append(clauses, "1 = 0")
	}

	if strings.TrimSpace(c.Query("token_name")) != "" ||
		strings.TrimSpace(c.Query("request_id")) != "" ||
		strings.TrimSpace(c.Query("upstream_request_id")) != "" {
		clauses = append(clauses, "1 = 0")
	}

	return " WHERE " + strings.Join(clauses, " AND "), args
}

func compatUsageLogRow(log UsageLog) gin.H {
	logType := 2
	if log.StatusCode >= 400 || strings.TrimSpace(log.Error) != "" {
		logType = 5
	}
	content := strings.TrimSpace(log.Error)
	if content == "" {
		content = log.Endpoint
	}
	useTime := float64(log.DurationMS) / 1000
	if useTime < 0 {
		useTime = 0
	}
	username := strings.TrimSpace(log.Username)
	if username == "" {
		username = "local"
	}
	return gin.H{
		"id":                  log.ID,
		"user_id":             1,
		"created_at":          log.CreatedAt,
		"type":                logType,
		"content":             content,
		"username":            username,
		"token_name":          "local",
		"model_name":          log.Model,
		"quota":               quotaFromCost(log.Cost),
		"prompt_tokens":       log.PromptTokens,
		"completion_tokens":   log.CompletionTokens,
		"use_time":            useTime,
		"is_stream":           false,
		"channel":             log.ChannelID,
		"channel_name":        log.ChannelName,
		"token_id":            0,
		"group":               log.Group,
		"ip":                  "127.0.0.1",
		"other":               "{}",
		"request_id":          "",
		"upstream_request_id": "",
	}
}

func handleChannelList(c *gin.Context, store *Store, allowKeyword bool) {
	page, _ := strconv.Atoi(c.DefaultQuery("p", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", c.DefaultQuery("size", "20")))
	channelType, _ := strconv.Atoi(c.Query("type"))
	opts := ChannelListOptions{
		Page:      page,
		PageSize:  pageSize,
		Group:     strings.TrimSpace(c.Query("group")),
		Status:    strings.TrimSpace(c.Query("status")),
		Type:      channelType,
		Model:     strings.TrimSpace(c.Query("model")),
		SortBy:    strings.TrimSpace(c.Query("sort_by")),
		SortOrder: strings.TrimSpace(c.Query("sort_order")),
		IDSort:    strings.EqualFold(c.Query("id_sort"), "true"),
	}
	if allowKeyword {
		opts.Keyword = strings.TrimSpace(c.Query("keyword"))
	} else {
		opts.Keyword = strings.TrimSpace(c.Query("filter"))
	}
	items, total, typeCounts, err := store.ListChannelsPage(c.Request.Context(), opts, false)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	ok(c, gin.H{
		"items":       items,
		"total":       total,
		"page":        opts.Page,
		"page_size":   opts.PageSize,
		"type_counts": typeCounts,
	})
}

func channelPayloadFromAddRequest(request AddChannelRequest) ChannelPayload {
	payload := request.Channel
	mode := strings.TrimSpace(request.Mode)
	if mode == "batch" || mode == "multi_to_single" {
		keys := modelList(payload.APIKey)
		if len(keys) == 0 {
			keys = modelList(payload.Key)
		}
		if len(keys) > 0 {
			payload.APIKey = keys[0]
			payload.Key = keys[0]
		}
	}
	return payload
}

func handleChannelPatch(c *gin.Context, store *Store, writer *ConfigWriter) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		fail(c, http.StatusBadRequest, errors.New("empty request body"))
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	var payload struct {
		ID int64 `json:"id"`
		ChannelPayload
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	if payload.ID <= 0 {
		fail(c, http.StatusBadRequest, errors.New("invalid channel id"))
		return
	}
	delete(raw, "id")
	channel, err := store.PatchChannel(c.Request.Context(), payload.ID, payload.ChannelPayload, raw)
	if err != nil {
		failCRUD(c, err)
		return
	}
	if err := refreshConfig(c.Request.Context(), writer); err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	ok(c, channel)
}

func fetchChannelModels(ctx context.Context, payload ChannelMetadataPayload) ([]string, error) {
	payload.Type = normalizeChannelType(payload.Type)
	baseURL := strings.TrimRight(strings.TrimSpace(payload.BaseURL), "/")
	apiKey := firstAPIKey(payload.APIKey)
	if baseURL == "" {
		baseURL = defaultBaseURLForType(payload.Type)
	}
	if baseURL == "" {
		return nil, errors.New("base_url is required")
	}
	if apiKey == "" && payload.Type != 4 {
		return nil, errors.New("api key is required")
	}
	models, err := fetchProviderModels(ctx, payload.Type, baseURL, apiKey)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, errors.New("no models returned from upstream")
	}
	return models, nil
}

type channelTestOptions struct {
	Model        string
	EndpointType string
	Stream       bool
}

func testLocalChannel(ctx context.Context, store *Store, id int64, opts channelTestOptions) (gin.H, error) {
	channel, err := store.GetChannel(ctx, id, true)
	if err != nil {
		return nil, err
	}
	baseURL := strings.TrimRight(strings.TrimSpace(channel.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURLForType(channel.Type)
	}
	if baseURL == "" {
		return nil, errors.New("base_url is required")
	}
	apiKey := firstAPIKey(channel.APIKey)
	if apiKey == "" && normalizeChannelType(channel.Type) != 4 {
		return nil, errors.New("api key is required")
	}
	model := channelTestModel(channel, opts.Model)
	endpointType := normalizeChannelTestEndpoint(channel, model, opts.EndpointType)
	start := time.Now()
	err = runChannelTestRequest(ctx, channel.Type, baseURL, apiKey, model, endpointType, opts.Stream)
	responseTime := time.Since(start).Milliseconds()
	if responseTime <= 0 {
		responseTime = 1
	}
	if err != nil {
		return nil, err
	}
	if _, updateErr := store.UpdateChannelTestResult(ctx, id, responseTime); updateErr != nil {
		return nil, updateErr
	}
	return gin.H{"response_time": responseTime}, nil
}

func channelTestModel(channel Channel, requested string) string {
	if model := strings.TrimSpace(requested); model != "" {
		return model
	}
	if model := strings.TrimSpace(channel.TestModel); model != "" {
		return model
	}
	if models := modelList(channel.Models); len(models) > 0 {
		return models[0]
	}
	return "gpt-4o-mini"
}

func normalizeChannelTestEndpoint(channel Channel, model, endpointType string) string {
	normalized := strings.TrimSpace(endpointType)
	if normalized != "" && normalized != "auto" {
		return normalized
	}
	modelLower := strings.ToLower(strings.TrimSpace(model))
	switch normalizeChannelType(channel.Type) {
	case 14:
		return "anthropic"
	case 24:
		return "gemini"
	case 57:
		return "openai-response"
	}
	if strings.Contains(modelLower, "claude") {
		return "anthropic"
	}
	if strings.Contains(modelLower, "gemini") {
		return "gemini"
	}
	if strings.Contains(modelLower, "codex") {
		return "openai-response"
	}
	return "openai"
}

func runChannelTestRequest(ctx context.Context, channelType int, baseURL, apiKey, model, endpointType string, stream bool) error {
	switch normalizeChannelType(channelType) {
	case 4:
		return testOllamaChannel(ctx, baseURL, model)
	case 14:
		return testAnthropicChannel(ctx, baseURL, apiKey, model, stream)
	case 24:
		return testGeminiChannel(ctx, baseURL, apiKey, model)
	default:
		switch endpointType {
		case "anthropic":
			return testAnthropicChannel(ctx, baseURL, apiKey, model, stream)
		case "gemini":
			return testGeminiChannel(ctx, baseURL, apiKey, model)
		case "openai-response", "openai-response-compact":
			return testOpenAIResponsesChannel(ctx, channelType, baseURL, apiKey, model, stream)
		default:
			return testOpenAIChatChannel(ctx, channelType, baseURL, apiKey, model, stream)
		}
	}
}

func testOpenAIChatChannel(ctx context.Context, channelType int, baseURL, apiKey, model string, stream bool) error {
	endpoint, err := buildOpenAICompatibleURL(channelType, baseURL, "v1", "chat", "completions")
	if err != nil {
		return err
	}
	payload := gin.H{
		"model": model,
		"messages": []gin.H{
			{"role": "user", "content": "hi"},
		},
		"stream": stream,
	}
	if isReasoningModel(model) {
		payload["max_completion_tokens"] = 16
	} else {
		payload["max_tokens"] = 16
	}
	req, err := newJSONRequest(ctx, endpoint, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	return doChannelTestRequest(req)
}

func testOpenAIResponsesChannel(ctx context.Context, channelType int, baseURL, apiKey, model string, stream bool) error {
	endpoint, err := buildOpenAICompatibleURL(channelType, baseURL, "v1", "responses")
	if err != nil {
		return err
	}
	req, err := newJSONRequest(ctx, endpoint, gin.H{
		"model":  model,
		"input":  []gin.H{{"role": "user", "content": "hi"}},
		"stream": stream,
	})
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	return doChannelTestRequest(req)
}

func testAnthropicChannel(ctx context.Context, baseURL, apiKey, model string, stream bool) error {
	endpoint, err := buildVersionedActionURL(baseURL, "v1", "messages")
	if err != nil {
		return err
	}
	req, err := newJSONRequest(ctx, endpoint, gin.H{
		"model":      model,
		"max_tokens": 16,
		"messages": []gin.H{
			{"role": "user", "content": "hi"},
		},
		"stream": stream,
	})
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	return doChannelTestRequest(req)
}

func testGeminiChannel(ctx context.Context, baseURL, apiKey, model string) error {
	endpoint, err := buildGeminiGenerateURL(baseURL, model, apiKey)
	if err != nil {
		return err
	}
	req, err := newJSONRequest(ctx, endpoint, gin.H{
		"contents": []gin.H{
			{
				"role":  "user",
				"parts": []gin.H{{"text": "hi"}},
			},
		},
		"generationConfig": gin.H{"maxOutputTokens": 16},
	})
	if err != nil {
		return err
	}
	return doChannelTestRequest(req)
}

func testOllamaChannel(ctx context.Context, baseURL, model string) error {
	endpoint, err := url.JoinPath(baseURL, "api", "chat")
	if err != nil {
		return err
	}
	req, err := newJSONRequest(ctx, endpoint, gin.H{
		"model": model,
		"messages": []gin.H{
			{"role": "user", "content": "hi"},
		},
		"stream": false,
	})
	if err != nil {
		return err
	}
	return doChannelTestRequest(req)
}

func newJSONRequest(ctx context.Context, endpoint string, payload any) (*http.Request, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func doChannelTestRequest(req *http.Request) error {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("upstream returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if err := validateChannelTestResponse(body); err != nil {
		return err
	}
	return nil
}

func validateChannelTestResponse(body []byte) error {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil
	}
	if bytes.Contains(trimmed, []byte(`"error"`)) {
		var data struct {
			Error any `json:"error"`
		}
		if json.Unmarshal(trimmed, &data) == nil && data.Error != nil {
			message, _ := json.Marshal(data.Error)
			return fmt.Errorf("upstream error: %s", strings.TrimSpace(string(message)))
		}
	}
	return nil
}

func isReasoningModel(model string) bool {
	lower := strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(lower, "o1") ||
		strings.HasPrefix(lower, "o3") ||
		strings.HasPrefix(lower, "o4") ||
		strings.Contains(lower, "reasoning")
}

func groupNames(groups []Group) []string {
	out := make([]string, 0, len(groups))
	for _, group := range groups {
		out = append(out, group.Name)
	}
	return out
}

func optionRows(settings Settings, groups []Group) []gin.H {
	groupRatio := map[string]float64{}
	userUsableGroups := map[string]string{}
	for _, group := range groups {
		groupRatio[group.Name] = group.PriceMultiplier
		userUsableGroups[group.Name] = group.Description
	}
	groupRatioJSON, _ := json.Marshal(groupRatio)
	userUsableGroupsJSON, _ := json.Marshal(userUsableGroups)
	return []gin.H{
		{"key": "SystemName", "value": settings.SystemName},
		{"key": "ServerAddress", "value": localServerURL(settings)},
		{"key": "GroupRatio", "value": string(groupRatioJSON)},
		{"key": "UserUsableGroups", "value": string(userUsableGroupsJSON)},
		{"key": "ModelRatio", "value": "{}"},
		{"key": "ModelPrice", "value": "{}"},
		{"key": "CompletionRatio", "value": "{}"},
		{"key": "DisplayInCurrencyEnabled", "value": "true"},
		{"key": "DisplayTokenStatEnabled", "value": "true"},
		{"key": "QuotaPerUnit", "value": strconv.FormatInt(localQuotaPerUnit, 10)},
		{"key": "USDExchangeRate", "value": "1"},
		{"key": "general_setting.custom_currency_symbol", "value": settings.CurrencySymbol},
		{"key": "general_setting.custom_currency_exchange_rate", "value": "1"},
		{"key": "general_setting.quota_display_type", "value": "USD"},
	}
}

func applyLocalOption(ctx context.Context, store *Store, key, value string) error {
	key = strings.TrimSpace(key)
	switch key {
	case "GroupRatio":
		ratios := map[string]float64{}
		if strings.TrimSpace(value) != "" {
			if err := json.Unmarshal([]byte(value), &ratios); err != nil {
				return err
			}
		}
		for name, ratio := range ratios {
			if err := store.UpsertGroupByName(ctx, GroupPayload{
				Name:            name,
				PriceMultiplier: ratio,
			}); err != nil {
				return err
			}
		}
	case "UserUsableGroups":
		descriptions := map[string]string{}
		if strings.TrimSpace(value) != "" {
			if err := json.Unmarshal([]byte(value), &descriptions); err != nil {
				return err
			}
		}
		for name, description := range descriptions {
			if err := store.UpsertGroupByName(ctx, GroupPayload{
				Name:        name,
				Description: description,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func syncChannelMetadata(ctx context.Context, payload ChannelMetadataPayload) ChannelMetadata {
	payload.Type = normalizeChannelType(payload.Type)
	baseURL := strings.TrimRight(strings.TrimSpace(payload.BaseURL), "/")
	apiKey := firstAPIKey(payload.APIKey)
	fallback := channelMetadataPreset(payload.Type)

	if baseURL == "" {
		baseURL = defaultBaseURLForType(payload.Type)
	}
	if baseURL == "" || (apiKey == "" && payload.Type != 4) {
		return fallback
	}

	models, err := fetchProviderModels(ctx, payload.Type, baseURL, apiKey)
	if err != nil || len(models) == 0 {
		return fallback
	}

	fallback.Models = models
	fallback.Source = "upstream"
	return fallback
}

func fetchProviderModels(ctx context.Context, channelType int, baseURL, apiKey string) ([]string, error) {
	switch normalizeChannelType(channelType) {
	case 4:
		return fetchOllamaModels(ctx, baseURL, apiKey)
	case 24:
		return fetchGeminiModels(ctx, baseURL, apiKey)
	case 14:
		return fetchAnthropicModels(ctx, baseURL, apiKey)
	default:
		return fetchOpenAICompatibleModels(ctx, channelType, baseURL, apiKey)
	}
}

func fetchOpenAICompatibleModels(ctx context.Context, channelType int, baseURL, apiKey string) ([]string, error) {
	modelsURL, err := buildOpenAICompatibleModelsURL(channelType, baseURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	return fetchModelsFromRequest(req)
}

func fetchOllamaModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	modelsURL, err := url.JoinPath(baseURL, "api", "tags")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Accept", "application/json")
	return fetchModelsFromRequest(req)
}

func fetchAnthropicModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	modelsURL, err := buildVersionedModelsURL(baseURL, "v1")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Accept", "application/json")
	return fetchModelsFromRequest(req)
}

func fetchGeminiModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	modelsURL, err := buildVersionedModelsURL(baseURL, "v1beta")
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(modelsURL)
	if err != nil {
		return nil, err
	}
	query := parsed.Query()
	query.Set("key", apiKey)
	parsed.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	return fetchModelsFromRequest(req)
}

func fetchModelsFromRequest(req *http.Request) ([]string, error) {
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
		return nil, fmt.Errorf("upstream returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	return parseModelsResponse(body)
}

func parseModelsResponse(body []byte) ([]string, error) {
	var data struct {
		Data []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
		Models []struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
			ID          string `json:"id"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	models := make([]string, 0, len(data.Data)+len(data.Models))
	add := func(value string) {
		model := strings.TrimPrefix(strings.TrimSpace(value), "models/")
		if model == "" {
			return
		}
		if _, ok := seen[model]; ok {
			return
		}
		seen[model] = struct{}{}
		models = append(models, model)
	}
	for _, item := range data.Data {
		if item.ID != "" {
			add(item.ID)
			continue
		}
		if item.Name != "" {
			add(item.Name)
			continue
		}
		add(item.DisplayName)
	}
	for _, item := range data.Models {
		if item.Name != "" {
			add(item.Name)
			continue
		}
		if item.ID != "" {
			add(item.ID)
			continue
		}
		add(item.DisplayName)
	}
	return models, nil
}

func firstAPIKey(apiKey string) string {
	for _, line := range strings.Split(apiKey, "\n") {
		if key := strings.TrimSpace(line); key != "" {
			return key
		}
	}
	return ""
}

func buildVersionedModelsURL(baseURL, version string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", errors.New("base_url is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(parsed.EscapedPath(), "/")
	if path == "/"+version || strings.HasSuffix(path, "/"+version) {
		return url.JoinPath(baseURL, "models")
	}
	return url.JoinPath(baseURL, version, "models")
}

func buildOpenAICompatibleModelsURL(channelType int, baseURL string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", errors.New("base_url is required")
	}
	if specialBase, ok := channelSpecialOpenAIBaseURL(baseURL); ok {
		switch normalizeChannelType(channelType) {
		case 25, 26:
			return url.JoinPath(specialBase, "models")
		case 45:
			return buildVersionedModelsURL(specialBase, "v1")
		}
	}
	switch normalizeChannelType(channelType) {
	case 17:
		return buildModelsURLWithPath(baseURL, "compatible-mode", "v1")
	case 26:
		return buildModelsURLWithPath(baseURL, "api", "paas", "v4")
	default:
		return buildVersionedModelsURL(baseURL, "v1")
	}
}

func buildOpenAICompatibleURL(channelType int, baseURL string, pathParts ...string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", errors.New("base_url is required")
	}
	if len(pathParts) == 0 {
		return "", errors.New("path is required")
	}
	actionParts := pathParts
	if pathParts[0] == "v1" {
		actionParts = pathParts[1:]
	}
	if specialBase, ok := channelSpecialOpenAIBaseURL(baseURL); ok {
		switch normalizeChannelType(channelType) {
		case 25, 26:
			return buildActionURL(specialBase, actionParts...)
		case 45:
			return buildVersionedActionURL(specialBase, "v1", actionParts...)
		}
	}
	switch normalizeChannelType(channelType) {
	case 17:
		return buildActionURLWithPath(baseURL, append([]string{"compatible-mode", "v1"}, actionParts...)...)
	case 26:
		return buildActionURLWithPath(baseURL, append([]string{"api", "paas", "v4"}, actionParts...)...)
	default:
		return buildVersionedActionURL(baseURL, "v1", actionParts...)
	}
}

func buildGeminiGenerateURL(baseURL, model, apiKey string) (string, error) {
	model = strings.TrimPrefix(strings.TrimSpace(model), "models/")
	if model == "" {
		return "", errors.New("model is required")
	}
	endpoint, err := buildVersionedActionURL(baseURL, "v1beta", "models", model+":generateContent")
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("key", apiKey)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func buildVersionedActionURL(baseURL, version string, pathParts ...string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", errors.New("base_url is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(parsed.EscapedPath(), "/")
	if path == "/"+version || strings.HasSuffix(path, "/"+version) {
		return url.JoinPath(baseURL, pathParts...)
	}
	parts := append([]string{version}, pathParts...)
	return url.JoinPath(baseURL, parts...)
}

func buildActionURL(baseURL string, pathParts ...string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", errors.New("base_url is required")
	}
	return url.JoinPath(baseURL, pathParts...)
}

func buildActionURLWithPath(baseURL string, pathParts ...string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	currentPath := strings.TrimRight(strings.Trim(parsed.EscapedPath(), "/"), "/")
	targetPath := strings.Join(pathParts, "/")
	if currentPath == targetPath || strings.HasSuffix(currentPath, "/"+targetPath) {
		return baseURL, nil
	}
	for i := len(pathParts); i > 0; i-- {
		prefix := strings.Join(pathParts[:i], "/")
		if currentPath == prefix || strings.HasSuffix(currentPath, "/"+prefix) {
			return url.JoinPath(baseURL, pathParts[i:]...)
		}
	}
	return url.JoinPath(baseURL, pathParts...)
}

func buildModelsURLWithPath(baseURL string, pathParts ...string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	currentPath := strings.TrimRight(strings.Trim(parsed.EscapedPath(), "/"), "/")
	targetPath := strings.Join(pathParts, "/")
	if currentPath == targetPath || strings.HasSuffix(currentPath, "/"+targetPath) {
		return url.JoinPath(baseURL, "models")
	}
	parts := append(pathParts, "models")
	return url.JoinPath(baseURL, parts...)
}

func channelSpecialOpenAIBaseURL(baseURL string) (string, bool) {
	switch strings.TrimRight(strings.TrimSpace(baseURL), "/") {
	case "https://open.bigmodel.cn/api/coding/paas":
		return "https://open.bigmodel.cn/api/coding/paas/v4", true
	case "https://api.z.ai/api/coding/paas":
		return "https://api.z.ai/api/coding/paas/v4", true
	case "https://api.kimi.com/coding":
		return "https://api.kimi.com/coding/v1", true
	case "https://ark.cn-beijing.volces.com/api/coding":
		return "https://ark.cn-beijing.volces.com/api/coding/v3", true
	default:
		return "", false
	}
}

func defaultBaseURLForType(channelType int) string {
	switch normalizeChannelType(channelType) {
	case 1:
		return "https://api.openai.com"
	case 4:
		return "http://localhost:11434"
	case 14:
		return "https://api.anthropic.com"
	case 17:
		return "https://dashscope.aliyuncs.com"
	case 20:
		return "https://openrouter.ai/api"
	case 23:
		return "https://hunyuan.tencentcloudapi.com"
	case 24:
		return "https://generativelanguage.googleapis.com"
	case 25:
		return "https://api.moonshot.cn"
	case 26:
		return "https://open.bigmodel.cn"
	case 27:
		return "https://api.perplexity.ai"
	case 31:
		return "https://api.lingyiwanwu.com"
	case 34:
		return "https://api.cohere.ai"
	case 35:
		return "https://api.minimax.chat"
	case 40:
		return "https://api.siliconflow.cn"
	case 42:
		return "https://api.mistral.ai"
	case 43:
		return "https://api.deepseek.com"
	case 48:
		return "https://api.x.ai"
	default:
		return ""
	}
}

func normalizeChannelType(channelType int) int {
	if channelType <= 0 {
		return 1
	}
	return channelType
}

func channelMetadataPreset(channelType int) ChannelMetadata {
	switch normalizeChannelType(channelType) {
	case 14:
		return ChannelMetadata{
			Models:           []string{"claude-3-5-sonnet-latest", "claude-3-5-haiku-latest"},
			InputPricePer1K:  0.003,
			OutputPricePer1K: 0.015,
			Source:           "preset",
		}
	case 20:
		return ChannelMetadata{
			Models:           []string{"openai/gpt-4o-mini", "anthropic/claude-3.5-sonnet"},
			InputPricePer1K:  0,
			OutputPricePer1K: 0,
			Source:           "preset",
		}
	case 24:
		return ChannelMetadata{
			Models:           []string{"gemini-1.5-flash", "gemini-1.5-pro"},
			InputPricePer1K:  0.000075,
			OutputPricePer1K: 0.0003,
			Source:           "preset",
		}
	case 43:
		return ChannelMetadata{
			Models:           []string{"deepseek-chat", "deepseek-reasoner"},
			InputPricePer1K:  0.00014,
			OutputPricePer1K: 0.00028,
			Source:           "preset",
		}
	case 57:
		return ChannelMetadata{
			Models:           []string{"codex-mini-latest"},
			InputPricePer1K:  0,
			OutputPricePer1K: 0,
			Source:           "preset",
		}
	default:
		return ChannelMetadata{
			Models:           []string{"gpt-4o-mini", "gpt-4o", "gpt-4.1-mini"},
			InputPricePer1K:  0.00015,
			OutputPricePer1K: 0.0006,
			Source:           "preset",
		}
	}
}

func handleListAPIKeys(c *gin.Context, store *Store, includeToken bool) {
	page, _ := strconv.Atoi(c.DefaultQuery("p", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	keyword := strings.TrimSpace(c.Query("keyword"))
	token := ""
	if includeToken {
		token = c.Query("token")
	}
	items, total, err := store.ListAPIKeys(c.Request.Context(), keyword, token, page, size)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	ok(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": size,
	})
}

func handleTokenAPI(c *gin.Context, store *Store) {
	path := strings.Trim(strings.TrimSpace(c.Param("path")), "/")
	switch c.Request.Method {
	case http.MethodGet:
		switch {
		case path == "" || path == "search":
			handleListAPIKeys(c, store, path == "search")
		case path != "":
			id, err := strconv.ParseInt(strings.TrimSpace(strings.Split(path, "/")[0]), 10, 64)
			if err != nil || id <= 0 {
				fail(c, http.StatusBadRequest, errors.New("invalid id"))
				return
			}
			item, err := store.GetAPIKey(c.Request.Context(), id, false)
			if err != nil {
				failCRUD(c, err)
				return
			}
			ok(c, item)
		default:
			fail(c, http.StatusNotFound, errors.New("not found"))
		}
	case http.MethodPost:
		switch path {
		case "", "/":
			var payload APIKeyPayload
			if !bindJSON(c, &payload) {
				return
			}
			item, err := store.CreateAPIKey(c.Request.Context(), payload)
			if err != nil {
				fail(c, http.StatusBadRequest, err)
				return
			}
			ok(c, item)
		case "batch":
			var payload struct {
				IDs []int64 `json:"ids"`
			}
			if !bindJSON(c, &payload) {
				return
			}
			deleted, err := store.DeleteAPIKeys(c.Request.Context(), payload.IDs)
			if err != nil {
				fail(c, http.StatusBadRequest, err)
				return
			}
			ok(c, deleted)
		default:
			parts := strings.Split(path, "/")
			if len(parts) == 2 && parts[1] == "key" {
				id, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
				if err != nil || id <= 0 {
					fail(c, http.StatusBadRequest, errors.New("invalid id"))
					return
				}
				key, err := store.APIKeySecret(c.Request.Context(), id)
				if err != nil {
					failCRUD(c, err)
					return
				}
				ok(c, gin.H{"key": key})
				return
			}
			if len(parts) == 2 && parts[1] == "configure" {
				id, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
				if err != nil || id <= 0 {
					fail(c, http.StatusBadRequest, errors.New("invalid id"))
					return
				}
				var payload struct {
					Target string `json:"target"`
				}
				if !bindJSON(c, &payload) {
					return
				}
				result, err := configureLocalClientForToken(c.Request.Context(), store, id, payload.Target)
				if err != nil {
					fail(c, http.StatusBadRequest, err)
					return
				}
				ok(c, result)
				return
			}
			if path == "batch/keys" {
				var payload struct {
					IDs []int64 `json:"ids"`
				}
				if !bindJSON(c, &payload) {
					return
				}
				keys, err := store.APIKeySecrets(c.Request.Context(), payload.IDs)
				if err != nil {
					fail(c, http.StatusBadRequest, err)
					return
				}
				ok(c, gin.H{"keys": keys})
				return
			}
			fail(c, http.StatusNotFound, errors.New("not found"))
		}
	case http.MethodPut:
		var payload APIKeyPayload
		if !bindJSON(c, &payload) {
			return
		}
		var (
			item APIKey
			err  error
		)
		if strings.EqualFold(c.Query("status_only"), "true") {
			item, err = store.SetAPIKeyStatus(c.Request.Context(), payload.ID, payload.Status)
		} else {
			item, err = store.UpdateAPIKey(c.Request.Context(), payload)
		}
		if err != nil {
			failCRUD(c, err)
			return
		}
		ok(c, item)
	case http.MethodDelete:
		parts := strings.Split(path, "/")
		if len(parts) == 1 && parts[0] != "" {
			id, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
			if err != nil || id <= 0 {
				fail(c, http.StatusBadRequest, errors.New("invalid id"))
				return
			}
			if err := store.DeleteAPIKey(c.Request.Context(), id); err != nil {
				failCRUD(c, err)
				return
			}
			ok(c, gin.H{"id": id})
			return
		}
		fail(c, http.StatusNotFound, errors.New("not found"))
	default:
		c.Status(http.StatusMethodNotAllowed)
	}
}

func configureLocalClientForToken(ctx context.Context, store *Store, id int64, target string) (gin.H, error) {
	target = strings.ToLower(strings.TrimSpace(target))
	if target != "codex" && target != "claude" {
		return nil, errors.New("unsupported target")
	}
	secret, err := store.APIKeySecret(ctx, id)
	if err != nil {
		return nil, err
	}
	token := localClientToken(secret)
	if token == "" {
		return nil, errors.New("api key is empty")
	}
	settings, err := store.Settings(ctx)
	if err != nil {
		return nil, err
	}
	endpoint := localServerURL(settings)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(homeDir) == "" {
		return nil, errors.New("user home directory not found")
	}

	switch target {
	case "codex":
		files, err := configureCodexClient(homeDir, endpoint, token)
		if err != nil {
			return nil, err
		}
		return gin.H{
			"target":   target,
			"endpoint": endpoint,
			"files":    files,
		}, nil
	case "claude":
		files, env, err := configureClaudeClient(homeDir, endpoint, token)
		if err != nil {
			return nil, err
		}
		return gin.H{
			"target":   target,
			"endpoint": endpoint,
			"files":    files,
			"env":      env,
		}, nil
	default:
		return nil, errors.New("unsupported target")
	}
}

func configureCodexClient(homeDir, endpoint, token string) ([]string, error) {
	codexDir := filepath.Join(homeDir, ".codex")
	authPath := filepath.Join(codexDir, "auth.json")
	configPath := filepath.Join(codexDir, "config.toml")
	endpointV1 := strings.TrimRight(endpoint, "/") + "/v1"

	if err := os.MkdirAll(codexDir, 0755); err != nil {
		return nil, err
	}
	if err := writeJSONFileMap(authPath, map[string]any{
		"OPENAI_API_KEY": token,
	}); err != nil {
		return nil, err
	}

	config := fmt.Sprintf(`model_provider = "OpenAI"
model = "gpt-5.5"
review_model = "gpt-5.5"
model_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true

[model_providers.OpenAI]
name = "OpenAI"
base_url = %s
wire_api = "responses"
requires_openai_auth = true

[features]
goals = true
`, tomlBasicString(endpointV1))
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		return nil, err
	}
	return []string{authPath, configPath}, nil
}

func configureClaudeClient(homeDir, endpoint, token string) ([]string, []string, error) {
	claudeDir := filepath.Join(homeDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")
	configPath := filepath.Join(claudeDir, "config.json")

	settingsDoc := map[string]any{
		"env": map[string]any{
			"ANTHROPIC_AUTH_TOKEN":                     token,
			"ANTHROPIC_BASE_URL":                       endpoint,
			"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		},
		"permissions": map[string]any{
			"allow": []string{},
			"deny":  []string{},
		},
		"theme":       "auto",
		"model":       "opus[1m]",
		"effortLevel": "max",
	}
	if err := writeJSONFileMap(settingsPath, settingsDoc); err != nil {
		return nil, nil, err
	}

	configDoc := map[string]any{
		"primaryApiKey": token,
	}
	if err := writeJSONFileMap(configPath, configDoc); err != nil {
		return nil, nil, err
	}

	envVars := map[string]string{
		"ANTHROPIC_BASE_URL":                       endpoint,
		"ANTHROPIC_AUTH_TOKEN":                     token,
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
	}
	updatedEnv := make([]string, 0, len(envVars))
	for key, value := range envVars {
		if err := setUserEnvironment(key, value); err != nil {
			return nil, nil, err
		}
		updatedEnv = append(updatedEnv, key)
	}
	return []string{settingsPath, configPath}, updatedEnv, nil
}

func localClientToken(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(secret), "sk-") {
		return secret
	}
	return "sk-" + secret
}

func writeJSONFileMap(path string, doc map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0600)
}

func tomlBasicString(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\b", `\b`, "\t", `\t`, "\n", `\n`, "\f", `\f`, "\r", `\r`)
	return `"` + replacer.Replace(value) + `"`
}

func setUserEnvironment(name, value string) error {
	if err := os.Setenv(name, value); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "windows":
		return setWindowsUserEnvironment(name, value)
	case "darwin":
		return setShellProfileEnvironment(name, value, ".zshrc")
	default:
		return nil
	}
}

func setWindowsUserEnvironment(name, value string) error {
	script := fmt.Sprintf(
		"[System.Environment]::SetEnvironmentVariable(%s, %s, [System.EnvironmentVariableTarget]::User)",
		powerShellString(name),
		powerShellString(value),
	)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("set %s failed: %s", name, message)
	}
	return nil
}

func setShellProfileEnvironment(name, value, profileName string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if strings.TrimSpace(homeDir) == "" {
		return errors.New("user home directory not found")
	}

	profilePath := filepath.Join(homeDir, profileName)
	data, err := os.ReadFile(profilePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s failed: %w", profilePath, err)
	}

	content := string(data)
	start := fmt.Sprintf("# >>> SunAPI %s >>>", name)
	end := fmt.Sprintf("# <<< SunAPI %s <<<", name)
	block := start + "\n" + shellExportLine(name, value) + "\n" + end
	updated := replaceMarkedShellBlock(content, start, end, block)
	return os.WriteFile(profilePath, []byte(updated), 0600)
}

func replaceMarkedShellBlock(content, start, end, block string) string {
	if startIndex := strings.Index(content, start); startIndex >= 0 {
		afterStart := content[startIndex:]
		if endRelativeIndex := strings.Index(afterStart, end); endRelativeIndex >= 0 {
			endIndex := startIndex + endRelativeIndex + len(end)
			for endIndex < len(content) && (content[endIndex] == '\r' || content[endIndex] == '\n') {
				endIndex++
			}
			prefix := strings.TrimRight(content[:startIndex], "\r\n")
			suffix := strings.TrimLeft(content[endIndex:], "\r\n")
			return joinShellProfileParts(prefix, block, suffix)
		}
	}
	return joinShellProfileParts(strings.TrimRight(content, "\r\n"), block, "")
}

func joinShellProfileParts(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, "\r\n")
		if part != "" {
			clean = append(clean, part)
		}
	}
	return strings.Join(clean, "\n\n") + "\n"
}

func shellExportLine(name, value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "`", "\\`")
	return fmt.Sprintf(`export %s="%s"`, name, replacer.Replace(value))
}

func powerShellString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func refreshConfig(ctx context.Context, writer *ConfigWriter) error {
	if writer == nil {
		return nil
	}
	_, err := writer.Write(ctx)
	return err
}

func failCRUD(c *gin.Context, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		fail(c, http.StatusNotFound, err)
		return
	}
	fail(c, http.StatusBadRequest, err)
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": data})
}

func fail(c *gin.Context, status int, err error) {
	message := "request failed"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error()
	}
	c.JSON(status, gin.H{"success": false, "message": message, "data": nil})
}

func localServerURL(settings Settings) string {
	host := strings.TrimSpace(settings.ListenHost)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("http://%s:%d", host, settings.ListenPort)
}
