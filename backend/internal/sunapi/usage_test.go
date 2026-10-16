package sunapi

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
)

func TestUsagePluginBillsChannelWhenSourceIsNotAPIKey(t *testing.T) {
	ctx := context.Background()
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	channel, err := store.CreateChannel(ctx, ChannelPayload{
		Name:             "codex",
		Group:            "default",
		BaseURL:          "https://example.com",
		APIKey:           "upstream-key",
		Models:           "gpt-5.5",
		InputPricePer1K:  0.001,
		OutputPricePer1K: 0.002,
		Enabled:          true,
	})
	if err != nil {
		t.Fatal(err)
	}

	NewUsagePlugin(store).HandleUsage(ctx, coreusage.Record{
		Provider:    "codex",
		Source:      "codex-user@example.com",
		Model:       "gpt-5.5",
		Alias:       "gpt-5.5",
		RequestedAt: time.Now(),
		Detail: coreusage.Detail{
			InputTokens:  1000,
			OutputTokens: 1000,
			TotalTokens:  2000,
		},
	})

	logs, err := store.ListUsageLogs(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("got %d usage logs, want 1", len(logs))
	}
	wantCost := UsageCostForModel(channel, Settings{}, 1, "gpt-5.5", 1000, 1000)
	if logs[0].ChannelID != channel.ID {
		t.Fatalf("usage log channel ID = %d, want %d", logs[0].ChannelID, channel.ID)
	}
	if math.Abs(logs[0].Cost-wantCost) > 0.0000001 {
		t.Fatalf("usage log cost = %v, want %v", logs[0].Cost, wantCost)
	}

	updated, err := store.GetChannel(ctx, channel.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if updated.UsedQuota != quotaFromCost(wantCost) {
		t.Fatalf("channel used quota = %d, want %d", updated.UsedQuota, quotaFromCost(wantCost))
	}
}
