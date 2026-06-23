package sunapi

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"gopkg.in/yaml.v3"
)

type ConfigWriter struct {
	store      *Store
	configPath string
	authDir    string
}

func NewConfigWriter(store *Store, configPath, authDir string) *ConfigWriter {
	return &ConfigWriter{store: store, configPath: configPath, authDir: authDir}
}

func (w *ConfigWriter) Build(ctx context.Context) (*config.Config, error) {
	settings, err := w.store.Settings(ctx)
	if err != nil {
		return nil, err
	}
	channels, err := w.store.ListChannels(ctx, true)
	if err != nil {
		return nil, err
	}
	cfg := &config.Config{
		Host:                            settings.ListenHost,
		Port:                            settings.ListenPort,
		AuthDir:                         w.authDir,
		LoggingToFile:                   false,
		CommercialMode:                  true,
		UsageStatisticsEnabled:          true,
		RedisUsageQueueRetentionSeconds: 3600,
		DisableCooling:                  true,
		RequestRetry:                    0,
		MaxRetryCredentials:             1,
		MaxRetryInterval:                1,
		SDKConfig: config.SDKConfig{
			RequestLog:         false,
			PassthroughHeaders: false,
			Streaming: config.StreamingConfig{
				KeepAliveSeconds: 2,
			},
		},
	}
	cfg.Plugins.Enabled = false
	cfg.RemoteManagement.AllowRemote = false
	cfg.RemoteManagement.DisableControlPanel = true
	cfg.RemoteManagement.PanelGitHubRepository = config.DefaultPanelGitHubRepository
	for _, channel := range channels {
		if strings.TrimSpace(channel.APIKey) == "" || strings.TrimSpace(channel.BaseURL) == "" {
			continue
		}
		models := buildCompatModels(channel)
		if len(models) == 0 {
			continue
		}
		weight := channel.Weight
		if weight <= 0 {
			weight = 1
		}
		cfg.OpenAICompatibility = append(cfg.OpenAICompatibility, config.OpenAICompatibility{
			Name:     compatName(channel),
			Group:    normalizeGroupName(channel.Group),
			Priority: channel.Priority,
			Weight:   weight,
			BaseURL:  strings.TrimRight(channel.BaseURL, "/"),
			Disabled: !channel.Enabled,
			APIKeyEntries: []config.OpenAICompatibilityAPIKey{
				{APIKey: channel.APIKey},
			},
			Models: models,
		})
	}
	return cfg, nil
}

func (w *ConfigWriter) Write(ctx context.Context) (*config.Config, error) {
	cfg, err := w.Build(ctx)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(w.configPath), 0755); err != nil {
		return nil, err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(w.configPath, data, 0600); err != nil {
		return nil, err
	}
	return cfg, nil
}

func buildCompatModels(channel Channel) []config.OpenAICompatibilityModel {
	models := modelList(channel.Models)
	out := make([]config.OpenAICompatibilityModel, 0, len(models))
	for _, model := range models {
		out = append(out, config.OpenAICompatibilityModel{
			Name:     model,
			Alias:    model,
			Image:    supportsImageGenerationModelForChannel(model, channel),
			Thinking: openAICompatThinkingSupport(model),
		})
	}
	return out
}

func openAICompatThinkingSupport(model string) *registry.ThinkingSupport {
	if upstream := registry.LookupStaticModelInfo(model); upstream != nil && upstream.Thinking != nil {
		return upstream.Thinking
	}
	return &registry.ThinkingSupport{Levels: []string{"low", "medium", "high", "xhigh"}}
}

func compatName(channel Channel) string {
	name := strings.ToLower(strings.TrimSpace(channel.Name))
	if name == "" {
		name = "channel"
	}
	name = regexp.MustCompile(`[^a-z0-9_-]+`).ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if name == "" {
		name = "channel"
	}
	return "sunapi-" + strconv.FormatInt(channel.ID, 10) + "-" + name
}
