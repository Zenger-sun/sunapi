package sunapi

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteAgentCustomGraphNodeRequiresRealModelExecution(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	node := agentBlueprintGraphNode{ID: "custom-1"}
	node.Data.Kind = "custom"
	node.Data.Title = "自定义处理"
	node.Data.Description = "把上游数据整理成摘要。"
	node.Data.Model = "gpt-4.1-mini"
	node.Data.Group = "default"

	result := executeAgentCustomGraphNode(
		context.Background(),
		store,
		&AgentBlueprint{ID: "bp-1", UserID: 1, ModelName: "gpt-4.1-mini", ModelGroup: "default"},
		node,
		map[string]interface{}{"source": map[string]interface{}{"value": 42}},
		map[string]interface{}{"task": "test"},
		agentBlueprintNodeResult{NodeID: node.ID, Kind: node.Data.Kind, Status: "completed"},
	)

	if result.Status != "failed" {
		t.Fatalf("status=%q want failed; output=%v", result.Status, result.Output)
	}
	if !strings.Contains(result.Message, "模型执行失败") {
		t.Fatalf("message=%q want model execution failure", result.Message)
	}
	if result.Error == "" {
		t.Fatalf("expected concrete model execution error")
	}
}

func TestBuildAgentNodeSystemPromptUsesOnlyNodeSkill(t *testing.T) {
	blueprintSkills, err := json.Marshal([]string{"skill.file-document"})
	if err != nil {
		t.Fatal(err)
	}
	node := agentBlueprintGraphNode{ID: "custom-1"}
	node.Data.Title = "API 处理"
	node.Data.Description = "根据上游数据调用接口"
	node.Data.Skill = "skill.api-function"

	prompt := buildAgentNodeSystemPrompt(
		context.Background(),
		nil,
		&AgentBlueprint{ID: "bp-1", UserID: 1, Skills: blueprintSkills},
		node,
	)

	if !strings.Contains(prompt, "API 函数 Skill") {
		t.Fatalf("prompt should include current node skill, got: %s", prompt)
	}
	if strings.Contains(prompt, "文件文档 Skill") || strings.Contains(prompt, "Excel") {
		t.Fatalf("prompt should not include blueprint-level skill context, got: %s", prompt)
	}
}
