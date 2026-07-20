package sunapi

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateBlueprintRequiresNodeModelInSelectedGroup(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	createAgentTestChannel(t, store, "codex", "gpt-5.5,gpt-5.4")
	createAgentTestChannel(t, store, "backup", "gpt-4.1")

	agent, err := store.CreateAgent(context.Background(), 1, AgentFormData{Name: "model validation"})
	if err != nil {
		t.Fatal(err)
	}
	validNodes, _ := json.Marshal([]agentBlueprintGraphNode{
		newAgentModelValidationNode("start", "start", "codex", "gpt-5.5"),
	})
	if _, err := store.CreateBlueprint(context.Background(), agent.ID, 1, BlueprintFormData{
		Name:       "valid",
		ModelGroup: "codex",
		ModelName:  "gpt-5.5",
		Nodes:      validNodes,
	}); err != nil {
		t.Fatalf("create valid blueprint: %v", err)
	}

	invalidNodes, _ := json.Marshal([]agentBlueprintGraphNode{
		newAgentModelValidationNode("start", "start", "codex", "gpt-4.1"),
	})
	_, err = store.CreateBlueprint(context.Background(), agent.ID, 1, BlueprintFormData{
		Name:       "invalid",
		ModelGroup: "codex",
		ModelName:  "gpt-5.5",
		Nodes:      invalidNodes,
	})
	if err == nil || !strings.Contains(err.Error(), "not available in group \"codex\"") {
		t.Fatalf("invalid node model error = %v", err)
	}
}

func createAgentTestChannel(t *testing.T, store *Store, group, models string) {
	t.Helper()
	_, err := store.CreateChannel(context.Background(), ChannelPayload{
		Name:    "test-" + group,
		Group:   group,
		BaseURL: "https://example.com",
		APIKey:  "test-key",
		Models:  models,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create test channel: %v", err)
	}
}

func newAgentModelValidationNode(id, kind, group, model string) agentBlueprintGraphNode {
	node := agentBlueprintGraphNode{ID: id}
	node.Data.Kind = kind
	node.Data.Title = id
	node.Data.Group = group
	node.Data.Model = model
	return node
}
