package sunapi

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
	log "github.com/sirupsen/logrus"
)

type UsagePlugin struct {
	store *Store
}

func NewUsagePlugin(store *Store) *UsagePlugin {
	return &UsagePlugin{store: store}
}

func (p *UsagePlugin) HandleUsage(ctx context.Context, record coreusage.Record) {
	if p == nil || p.store == nil {
		return
	}
	usageCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ctx = usageCtx

	inputTokens := record.Detail.InputTokens
	outputTokens := record.Detail.OutputTokens + record.Detail.ReasoningTokens
	totalTokens := record.Detail.TotalTokens
	if totalTokens == 0 {
		totalTokens = inputTokens + outputTokens
	}

	statusCode := 200
	errText := ""
	if record.Failed {
		statusCode = record.Fail.StatusCode
		if statusCode == 0 {
			statusCode = 500
		}
		errText = trimLogText(record.Fail.Body, 2000)
	}

	createdAt := record.RequestedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	model := strings.TrimSpace(record.Alias)
	if model == "" {
		model = strings.TrimSpace(record.Model)
	}
	if model == "" {
		model = "unknown"
	}

	endpoint := strings.TrimSpace(record.ExecutorType)
	if endpoint == "" {
		endpoint = strings.TrimSpace(record.Provider)
	}
	if endpoint == "" {
		endpoint = "proxy"
	}

	channel, multiplier, err := p.store.ChannelForUsage(ctx, record.Source, "", record.Provider)
	usageLog := UsageLog{
		CreatedAt:        createdAt.Unix(),
		Model:            model,
		Endpoint:         endpoint,
		PromptTokens:     inputTokens,
		CompletionTokens: outputTokens,
		TotalTokens:      totalTokens,
		DurationMS:       record.Latency.Milliseconds(),
		StatusCode:       statusCode,
		Error:            errText,
	}
	if err == nil {
		usageLog.ChannelID = channel.ID
		usageLog.ChannelName = channel.Name
		usageLog.Group = channel.Group
		settings, settingsErr := p.store.Settings(ctx)
		if settingsErr != nil {
			log.Warnf("sunapi usage: failed to load pricing settings: %v", settingsErr)
		}
		usageLog.Cost = UsageCostForModel(channel, settings, multiplier, model, inputTokens, outputTokens)
	} else {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Warnf("sunapi usage: failed to resolve channel for provider %q: %v", record.Provider, err)
		}
		usageLog.ChannelName = fallbackChannelName(record)
		usageLog.Group = "unknown"
	}

	if err := p.store.InsertUsageLog(ctx, usageLog); err != nil {
		log.Warnf("sunapi usage: failed to store usage record: %v", err)
	}
}

func fallbackChannelName(record coreusage.Record) string {
	for _, value := range []string{record.Provider, record.AuthID, record.AuthIndex, record.ExecutorType} {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return "unknown"
}

func trimLogText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}
