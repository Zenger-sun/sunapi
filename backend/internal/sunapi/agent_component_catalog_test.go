package sunapi

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestPrepareAndCreateCustomComponentPersistsModelSkillTags(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	prepared, config, err := prepareComponentForm(ComponentFormData{
		Name:        "库存分支循环",
		Description: "按条件分支循环处理库存数据，去重排序后输出异常商品。",
		Group:       "process",
		ModelGroup:  "ops",
		ModelName:   "gpt-4.1-mini",
		Skill:       "skill.mcp-workflow",
		Inputs:      []string{"库存数据"},
		Outputs:     []string{"异常商品"},
		Settings:    json.RawMessage(`{"条件":"stock<3","排序字段":"stock"}`),
		Tags:        []string{"inventory"},
	})
	if err != nil {
		t.Fatal(err)
	}

	component, err := store.CreateComponent(context.Background(), 1, prepared, config)
	if err != nil {
		t.Fatal(err)
	}
	if component.Source != "custom" || component.Kind != "custom" || component.Group != "process" {
		t.Fatalf("unexpected component routing fields: %#v", component)
	}

	var cfg componentConfigPayload
	if err := json.Unmarshal(component.Config, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.ModelGroup != "ops" || cfg.ModelName != "gpt-4.1-mini" {
		t.Fatalf("model config missing: %#v", cfg)
	}
	if cfg.Skill != "skill.mcp-workflow" {
		t.Fatalf("skill=%q want skill.mcp-workflow", cfg.Skill)
	}
	if cfg.Settings["条件"] != "stock<3" || cfg.Settings["排序字段"] != "stock" {
		t.Fatalf("settings not persisted: %#v", cfg.Settings)
	}
	for _, tag := range []string{"custom-component", "inventory", "workflow", "dedupe", "sort"} {
		if !stringSliceContains(cfg.Tags, tag) {
			t.Fatalf("expected tag %q in %#v", tag, cfg.Tags)
		}
	}
	for _, capability := range []string{"流程控制", "集合处理", "数据处理"} {
		if !stringSliceContains(cfg.Capabilities, capability) {
			t.Fatalf("expected capability %q in %#v", capability, cfg.Capabilities)
		}
	}
}

func TestBuiltinAgentComponentsDoesNotExposeBusinessTemplates(t *testing.T) {
	hidden := map[string]bool{
		"component.report-data":       true,
		"component.order-report-data": true,
		"component.ticket_grab":       true,
		"component.xianyu":            true,
	}
	for _, component := range builtinAgentComponents {
		if hidden[component.ID] {
			t.Fatalf("business template must not be exposed in the generic catalog: %#v", component)
		}
	}
}

func TestXianyuComponentIsCustom(t *testing.T) {
	var found bool
	for _, component := range customAgentComponents {
		if component.ID != "component.xianyu" {
			continue
		}
		found = true
		if component.Source != "custom" || component.Kind != "xianyu" {
			t.Fatalf("unexpected xianyu component source: %#v", component)
		}
		var config componentConfigPayload
		if err := json.Unmarshal(component.Config, &config); err != nil {
			t.Fatal(err)
		}
		if config.ModelName != "gpt-5.6-terra" || config.Settings["会话数量"] != "全部" || config.Settings["最大会话保护上限"] != "5000" || config.Settings["节点超时"] != "不限制" {
			t.Fatalf("xianyu defaults are not configured for full scan: %#v", config)
		}
	}
	if !found {
		t.Fatal("xianyu component is missing from custom catalog")
	}
}

func TestNormalizeComponentGroupRejectsTemplateGroups(t *testing.T) {
	for _, group := range []string{"模板", "业务模板", "template", "recipe"} {
		if got := normalizeComponentGroup(group); got != "" {
			t.Fatalf("normalizeComponentGroup(%q)=%q want empty", group, got)
		}
	}
}

func TestBuiltinCollectorComponentsDoNotPresetBusinessURLs(t *testing.T) {
	collectorIDs := map[string]bool{
		"component.browser": true,
		"component.request": true,
	}
	for _, component := range builtinAgentComponents {
		if !collectorIDs[component.ID] {
			continue
		}
		var cfg componentConfigPayload
		if err := json.Unmarshal(component.Config, &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg.Settings["网址"] != "" {
			t.Fatalf("collector %q must not preset a URL: %#v", component.ID, cfg.Settings)
		}
	}
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
