package sunapi

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestExecuteAgentSubflowGraphNode(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	createAgentTestChannel(t, store, "default", "model")

	start := agentBlueprintGraphNode{ID: "start"}
	start.Data.Kind = "start"
	stop := agentBlueprintGraphNode{ID: "stop"}
	stop.Data.Kind = "stop"
	nodes, _ := json.Marshal([]agentBlueprintGraphNode{start, stop})
	edges, _ := json.Marshal([]agentBlueprintGraphEdge{{Source: "start", Target: "stop"}})
	now := time.Now().UnixMilli()
	_, err = store.db.ExecContext(context.Background(), `
		INSERT INTO agent_blueprints
		(id, agent_id, user_id, name, description, status,
		 trigger_config, model_group, model_name, skills,
		 input_variables, archive_config, nodes, edges,
		 created_time, updated_time, last_executed_time, next_execution_time,
		 execution_count, failure_reason, execution_duration_ms)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,0,0,'',0)`,
		"target", "agent", 1, "Target flow", "", "pending",
		json.RawMessage(`{"type":"manual"}`), "default", "model", json.RawMessage(`[]`),
		json.RawMessage(`{"preset":"target"}`), json.RawMessage(`{"type":"disabled"}`), nodes, edges,
		now, now,
	)
	if err != nil {
		t.Fatal(err)
	}

	node := agentBlueprintGraphNode{ID: "subflow"}
	node.Data.Kind = "custom"
	node.Data.ComponentID = "component.subflow"
	node.Data.Settings = map[string]string{
		"蓝图ID": "target",
		"输入映射": "mapped=$parent_value",
	}
	result := executeAgentSubflowGraphNode(
		context.Background(),
		store,
		&AgentBlueprint{ID: "parent", UserID: 1},
		node,
		map[string]interface{}{"source": map[string]interface{}{"value": 42}},
		map[string]interface{}{"parent_value": "ok"},
		agentBlueprintNodeResult{Status: "completed"},
	)
	if result.Status != "completed" {
		t.Fatalf("subflow status = %q, error = %s", result.Status, result.Error)
	}
	if result.Output["blueprint_id"] != "target" {
		t.Fatalf("subflow blueprint = %#v, want target", result.Output["blueprint_id"])
	}
}

func TestExecuteAgentSubflowGraphNodeRejectsCycle(t *testing.T) {
	node := agentBlueprintGraphNode{ID: "subflow"}
	node.Data.ComponentID = "component.subflow"
	node.Data.Settings = map[string]string{"蓝图ID": "parent"}
	result := executeAgentSubflowGraphNode(
		context.Background(),
		&Store{},
		&AgentBlueprint{ID: "parent", UserID: 1},
		node,
		nil,
		nil,
		agentBlueprintNodeResult{Status: "completed"},
	)
	if result.Status != "failed" || result.Error != "subflow cycle detected" {
		t.Fatalf("cycle result = %#v", result)
	}
}
