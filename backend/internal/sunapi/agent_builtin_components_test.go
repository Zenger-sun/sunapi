package sunapi

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestExecuteAgentBranchLoopAndDedupeSortComponents(t *testing.T) {
	upstream := map[string]interface{}{
		"source": map[string]interface{}{
			"rows": []interface{}{
				map[string]interface{}{"id": "a", "status": "ok", "score": 3},
				map[string]interface{}{"id": "b", "status": "bad", "score": 5},
				map[string]interface{}{"id": "a", "status": "ok", "score": 9},
			},
		},
	}

	branchNode := agentBlueprintGraphNode{ID: "branch-1"}
	branchNode.Data.ComponentID = "component.branch"
	branchNode.Data.Settings = map[string]string{"条件": "status=ok", "真分支": "通过", "假分支": "拦截"}
	branchResult := executeAgentBranchGraphNode(branchNode, upstream, agentBlueprintNodeResult{Status: "completed"})
	if branchResult.Status != "completed" {
		t.Fatalf("branch status=%q error=%s", branchResult.Status, branchResult.Error)
	}
	if branchResult.Output["passed"] != true {
		t.Fatalf("branch passed=%v want true", branchResult.Output["passed"])
	}
	if branchResult.Output["matched_count"] != 2 {
		t.Fatalf("branch matched_count=%v want 2", branchResult.Output["matched_count"])
	}

	loopNode := agentBlueprintGraphNode{ID: "loop-1"}
	loopNode.Data.ComponentID = "component.loop"
	loopNode.Data.Settings = map[string]string{"最大条目": "2", "输出字段": "items"}
	loopResult := executeAgentLoopGraphNode(loopNode, upstream, agentBlueprintNodeResult{Status: "completed"})
	if loopResult.Status != "completed" {
		t.Fatalf("loop status=%q error=%s", loopResult.Status, loopResult.Error)
	}
	if loopResult.Output["count"] != 2 {
		t.Fatalf("loop count=%v want 2", loopResult.Output["count"])
	}
	if loopResult.Output["truncated"] != true {
		t.Fatalf("loop truncated=%v want true", loopResult.Output["truncated"])
	}

	sortNode := agentBlueprintGraphNode{ID: "sort-1"}
	sortNode.Data.ComponentID = "component.dedupe_sort"
	sortNode.Data.Settings = map[string]string{"去重字段": "id", "排序字段": "score", "排序方向": "降序"}
	sortResult := executeAgentDedupeSortGraphNode(sortNode, upstream, agentBlueprintNodeResult{Status: "completed"})
	if sortResult.Status != "completed" {
		t.Fatalf("dedupe_sort status=%q error=%s", sortResult.Status, sortResult.Error)
	}
	if sortResult.Output["removed_count"] != 1 {
		t.Fatalf("removed_count=%v want 1", sortResult.Output["removed_count"])
	}
	rows, ok := sortResult.Output["rows"].([]map[string]interface{})
	if !ok || len(rows) != 2 {
		t.Fatalf("rows=%T %#v", sortResult.Output["rows"], sortResult.Output["rows"])
	}
	if rows[0]["id"] != "b" {
		t.Fatalf("first row id=%v want b", rows[0]["id"])
	}
}

func TestExecuteBlueprintNodeGraphStopsOnFailedNode(t *testing.T) {
	start := agentBlueprintGraphNode{ID: "start-1"}
	start.Data.Kind = "start"
	start.Data.Title = "开始"

	browser := agentBlueprintGraphNode{ID: "browser-1"}
	browser.Data.Kind = "browser"
	browser.Data.Title = "浏览器取数"
	browser.Data.Settings = map[string]string{}

	stop := agentBlueprintGraphNode{ID: "stop-1"}
	stop.Data.Kind = "stop"
	stop.Data.Title = "停止"

	nodes, err := json.Marshal([]agentBlueprintGraphNode{start, browser, stop})
	if err != nil {
		t.Fatal(err)
	}
	edges, err := json.Marshal([]agentBlueprintGraphEdge{
		{Source: "start-1", Target: "browser-1"},
		{Source: "browser-1", Target: "stop-1"},
	})
	if err != nil {
		t.Fatal(err)
	}

	output, _, err := executeBlueprintNodeGraph(
		context.Background(),
		nil,
		&AgentBlueprint{ID: "bp-test", Nodes: nodes, Edges: edges},
		nil,
		map[string]interface{}{"type": "disabled"},
	)
	if err == nil {
		t.Fatal("expected graph execution to fail when a node fails")
	}

	var graph agentBlueprintGraphExecution
	if err := json.Unmarshal(output, &graph); err != nil {
		t.Fatalf("failed to parse graph output: %v", err)
	}
	if len(graph.Nodes) != 2 {
		t.Fatalf("executed nodes=%d want 2", len(graph.Nodes))
	}
	if graph.Nodes[1].NodeID != "browser-1" || graph.Nodes[1].Status != "failed" {
		t.Fatalf("failed node=%+v", graph.Nodes[1])
	}
	if len(graph.Warnings) == 0 {
		t.Fatal("expected graph warnings to include failed node")
	}
}

func TestExecuteAgentWaitComponentCanBeCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	node := agentBlueprintGraphNode{ID: "wait-1"}
	node.Data.ComponentID = "component.wait"
	node.Data.Settings = map[string]string{"等待时间": "60s"}

	result := executeAgentWaitGraphNode(ctx, node, nil, agentBlueprintNodeResult{Status: "completed"})
	if result.Status != "failed" {
		t.Fatalf("wait status=%q want failed", result.Status)
	}
	if result.Error == "" {
		t.Fatalf("expected cancellation error")
	}
}

func TestExecuteAgentDatabaseComponentRunsReadOnlySQL(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	node := agentBlueprintGraphNode{ID: "db-1"}
	node.Data.ComponentID = "component.database"
	node.Data.Settings = map[string]string{"SQL": "select 42 as answer", "限制": "10"}

	result := executeAgentDatabaseGraphNode(context.Background(), store, node, agentBlueprintNodeResult{Status: "completed"})
	if result.Status != "completed" {
		t.Fatalf("database status=%q error=%s", result.Status, result.Error)
	}
	rows, ok := result.Output["rows"].([]map[string]interface{})
	if !ok || len(rows) != 1 {
		t.Fatalf("rows=%T %#v", result.Output["rows"], result.Output["rows"])
	}
	if rows[0]["answer"] == nil {
		t.Fatalf("answer missing from row: %#v", rows[0])
	}

	node.Data.Settings["SQL"] = "delete from agents"
	blocked := executeAgentDatabaseGraphNode(context.Background(), store, node, agentBlueprintNodeResult{Status: "completed"})
	if blocked.Status != "failed" {
		t.Fatalf("write SQL status=%q want failed", blocked.Status)
	}
}
