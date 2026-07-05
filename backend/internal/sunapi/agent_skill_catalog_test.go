package sunapi

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareMarketplaceSkillFormInstallsExpandedCatalogItem(t *testing.T) {
	form, config, permissions, version, err := prepareMarketplaceSkillForm(SkillFormData{
		SourceType:    "marketplace",
		MarketplaceID: "marketplace.github-actions",
	})
	if err != nil {
		t.Fatalf("prepareMarketplaceSkillForm returned error: %v", err)
	}
	if form.Name != "GitHub 工作流 Skill" {
		t.Fatalf("name=%q", form.Name)
	}
	if version != "1.0.0" {
		t.Fatalf("version=%q", version)
	}
	var cfg map[string]any
	if err := json.Unmarshal(config, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["marketplace_id"] != "marketplace.github-actions" {
		t.Fatalf("marketplace_id=%v", cfg["marketplace_id"])
	}
	var perms []string
	if err := json.Unmarshal(permissions, &perms); err != nil {
		t.Fatal(err)
	}
	if len(perms) == 0 {
		t.Fatalf("expected permissions")
	}
}

func TestBuildBlueprintSkillPromptContextIncludesInstalledSkill(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	form, config, permissions, version, err := prepareMarketplaceSkillForm(SkillFormData{
		SourceType:    "marketplace",
		MarketplaceID: "marketplace.github-actions",
	})
	if err != nil {
		t.Fatal(err)
	}
	skill, err := store.CreateSkill(context.Background(), 1, form, config, permissions, version)
	if err != nil {
		t.Fatal(err)
	}
	rawSkills, _ := json.Marshal([]string{skill.ID})
	ctx := buildBlueprintSkillPromptContext(context.Background(), store, &AgentBlueprint{
		UserID: 1,
		Skills: rawSkills,
	})
	if !strings.Contains(ctx, "GitHub 工作流 Skill") {
		t.Fatalf("prompt context missing installed skill: %s", ctx)
	}
	if !strings.Contains(ctx, "Issue、PR") {
		t.Fatalf("prompt context missing installed instructions: %s", ctx)
	}
}

func TestPrepareCustomSkillFormIncludesModelAndTags(t *testing.T) {
	_, config, _, _, err := prepareCustomSkillForm(SkillFormData{
		Name:        "财务日报",
		Description: "读取 Excel 数据，生成日报并发送通知。",
		ModelGroup:  "finance",
		ModelName:   "gpt-4.1",
		Tags:        []string{"finance"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(config, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["model_group"] != "finance" || cfg["model_name"] != "gpt-4.1" {
		t.Fatalf("model fields missing: %#v", cfg)
	}
	tags, _ := cfg["tags"].([]any)
	if len(tags) == 0 {
		t.Fatalf("expected tags in config: %#v", cfg)
	}
}
