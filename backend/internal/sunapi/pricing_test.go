package sunapi

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"
)

func TestUsageCostForModelUsesCompatRatioAndGroupMultiplier(t *testing.T) {
	cost := UsageCostForModel(Channel{}, Settings{}, 2, "gpt-5.5", 1000, 1000)
	want := 0.07
	if math.Abs(cost-want) > 0.0000001 {
		t.Fatalf("cost = %v, want %v", cost, want)
	}
}

func TestUsageCostForModelPrefersChannelPricing(t *testing.T) {
	channel := Channel{InputPricePer1K: 0.001, OutputPricePer1K: 0.002}
	cost := UsageCostForModel(channel, Settings{}, 3, "gpt-5.5", 1000, 1000)
	want := 0.009
	if math.Abs(cost-want) > 0.0000001 {
		t.Fatalf("cost = %v, want %v", cost, want)
	}
}

func TestUsageCostForModelFallsBackToSettings(t *testing.T) {
	settings := Settings{DefaultInputPricePer1K: 0.003, DefaultOutputPricePer1K: 0.004}
	cost := UsageCostForModel(Channel{}, settings, 1, "unknown-model", 1000, 1000)
	want := 0.007
	if math.Abs(cost-want) > 0.0000001 {
		t.Fatalf("cost = %v, want %v", cost, want)
	}
}

func TestRecalculateUsageCostsUsesModelPricingAndGroupMultiplier(t *testing.T) {
	ctx := context.Background()
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	group, err := store.CreateGroup(ctx, GroupPayload{Name: "vip", PriceMultiplier: 2})
	if err != nil {
		t.Fatal(err)
	}
	channel, err := store.CreateChannel(ctx, ChannelPayload{
		Name:    "codex",
		Group:   group.Name,
		BaseURL: "https://example.com",
		APIKey:  "test-key",
		Models:  "gpt-5.5",
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().Unix()
	err = store.InsertUsageLog(ctx, UsageLog{
		CreatedAt:        now,
		ChannelID:        channel.ID,
		ChannelName:      channel.Name,
		Group:            group.Name,
		Model:            "gpt-5.5",
		Endpoint:         "proxy",
		PromptTokens:     1000,
		CompletionTokens: 1000,
		TotalTokens:      2000,
		Cost:             0,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := store.RecalculateUsageCosts(ctx, now-60, now+60, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Updated != 1 || result.Skipped != 0 {
		t.Fatalf("result = %+v, want total=1 updated=1 skipped=0", result)
	}

	items, err := store.QueryQuotaData(ctx, now-60, now+60, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Quota != 70000 {
		t.Fatalf("quota = %d, want 70000", items[0].Quota)
	}
}
