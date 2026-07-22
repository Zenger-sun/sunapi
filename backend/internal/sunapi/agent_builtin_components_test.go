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

func TestExecuteBlueprintNodeGraphFollowsBranchHandles(t *testing.T) {
	start := agentBlueprintGraphNode{ID: "start"}
	start.Data.Kind = "start"
	start.Data.Title = "Start"

	branch := agentBlueprintGraphNode{ID: "branch"}
	branch.Data.Kind = "custom"
	branch.Data.ComponentID = "component.branch"
	branch.Data.Title = "Branch"
	branch.Data.Settings = map[string]string{}

	trueNode := agentBlueprintGraphNode{ID: "true-node"}
	trueNode.Data.Kind = "start"
	trueNode.Data.Title = "True path"

	falseNode := agentBlueprintGraphNode{ID: "false-node"}
	falseNode.Data.Kind = "start"
	falseNode.Data.Title = "False path"

	nodes, err := json.Marshal([]agentBlueprintGraphNode{start, branch, trueNode, falseNode})
	if err != nil {
		t.Fatal(err)
	}
	edges, err := json.Marshal([]agentBlueprintGraphEdge{
		{Source: "start", Target: "branch"},
		{Source: "branch", Target: "true-node", SourceHandle: agentBranchTrueHandle},
		{Source: "branch", Target: "false-node", SourceHandle: agentBranchFalseHandle},
	})
	if err != nil {
		t.Fatal(err)
	}

	output, _, err := executeBlueprintNodeGraph(
		context.Background(),
		nil,
		&AgentBlueprint{ID: "branch-routing", Nodes: nodes, Edges: edges},
		nil,
		map[string]interface{}{"type": "disabled"},
	)
	if err != nil {
		t.Fatalf("executeBlueprintNodeGraph error = %v", err)
	}

	var graph agentBlueprintGraphExecution
	if err := json.Unmarshal(output, &graph); err != nil {
		t.Fatalf("failed to parse graph output: %v", err)
	}
	if len(graph.Nodes) != 4 {
		t.Fatalf("graph nodes = %d, want 4", len(graph.Nodes))
	}
	if graph.Nodes[2].NodeID != "true-node" || graph.Nodes[2].Output["skipped"] == true {
		t.Fatalf("true branch result = %#v", graph.Nodes[2])
	}
	if graph.Nodes[3].NodeID != "false-node" || graph.Nodes[3].Output["skipped"] != true {
		t.Fatalf("false branch result = %#v", graph.Nodes[3])
	}
}

func TestExecuteBlueprintNodeGraphRoutesSwitchFromBlueprintVariable(t *testing.T) {
	start := agentBlueprintGraphNode{ID: "start"}
	start.Data.Kind = "start"

	setVariable := agentBlueprintGraphNode{ID: "set-variable"}
	setVariable.Data.Kind = "custom"
	setVariable.Data.ComponentID = "component.set_variable"
	setVariable.Data.Settings = map[string]string{
		"变量名": "status",
		"变量值": "ok",
	}

	switchNode := agentBlueprintGraphNode{ID: "switch"}
	switchNode.Data.Kind = "custom"
	switchNode.Data.ComponentID = "component.switch"
	switchNode.Data.Settings = map[string]string{
		"判断字段": "variables.status",
		"分支配置": "ok=正常\nerror=异常",
		"默认分支": "其他",
	}

	matchedNode := agentBlueprintGraphNode{ID: "matched"}
	matchedNode.Data.Kind = "start"
	defaultNode := agentBlueprintGraphNode{ID: "default"}
	defaultNode.Data.Kind = "start"

	nodes, err := json.Marshal([]agentBlueprintGraphNode{
		start, setVariable, switchNode, matchedNode, defaultNode,
	})
	if err != nil {
		t.Fatal(err)
	}
	edges, err := json.Marshal([]agentBlueprintGraphEdge{
		{Source: "start", Target: "set-variable"},
		{Source: "set-variable", Target: "switch"},
		{Source: "switch", Target: "matched", SourceHandle: "switch-0"},
		{Source: "switch", Target: "default", SourceHandle: agentSwitchDefaultHandle},
	})
	if err != nil {
		t.Fatal(err)
	}

	output, _, err := executeBlueprintNodeGraph(
		context.Background(),
		nil,
		&AgentBlueprint{ID: "switch-routing", Nodes: nodes, Edges: edges},
		nil,
		map[string]interface{}{"type": "disabled"},
	)
	if err != nil {
		t.Fatalf("executeBlueprintNodeGraph error = %v", err)
	}

	var graph agentBlueprintGraphExecution
	if err := json.Unmarshal(output, &graph); err != nil {
		t.Fatalf("failed to parse graph output: %v", err)
	}
	if len(graph.Nodes) != 5 {
		t.Fatalf("graph nodes = %d, want 5", len(graph.Nodes))
	}
	if graph.InputVars["status"] != "ok" {
		t.Fatalf("blueprint status variable = %#v, want ok", graph.InputVars["status"])
	}
	if graph.Nodes[3].NodeID != "matched" || graph.Nodes[3].Output["skipped"] == true {
		t.Fatalf("matched route result = %#v", graph.Nodes[3])
	}
	if graph.Nodes[4].NodeID != "default" || graph.Nodes[4].Output["skipped"] != true {
		t.Fatalf("default route result = %#v", graph.Nodes[4])
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

func TestExecuteBlueprintNodeGraphStopsAfterStopNode(t *testing.T) {
	start := agentBlueprintGraphNode{ID: "start"}
	start.Data.Kind = "start"
	stop := agentBlueprintGraphNode{ID: "stop"}
	stop.Data.Kind = "stop"
	after := agentBlueprintGraphNode{ID: "after"}
	after.Data.Kind = "start"
	nodes, _ := json.Marshal([]agentBlueprintGraphNode{start, stop, after})
	edges, _ := json.Marshal([]agentBlueprintGraphEdge{
		{Source: "start", Target: "stop"},
		{Source: "stop", Target: "after"},
	})

	output, _, err := executeBlueprintNodeGraph(
		context.Background(),
		nil,
		&AgentBlueprint{ID: "stop-flow", Nodes: nodes, Edges: edges},
		nil,
		map[string]interface{}{"type": "disabled"},
	)
	if err != nil {
		t.Fatal(err)
	}
	var graph agentBlueprintGraphExecution
	if err := json.Unmarshal(output, &graph); err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 2 || graph.Nodes[1].NodeID != "stop" {
		t.Fatalf("executed nodes = %#v, want start and stop", graph.Nodes)
	}
}

func TestExecuteBlueprintNodeGraphFansOutOneOutputToMultipleNodes(t *testing.T) {
	start := agentBlueprintGraphNode{ID: "start"}
	start.Data.Kind = "start"
	left := agentBlueprintGraphNode{ID: "left"}
	left.Data.Kind = "start"
	right := agentBlueprintGraphNode{ID: "right"}
	right.Data.Kind = "start"
	nodes, _ := json.Marshal([]agentBlueprintGraphNode{start, left, right})
	edges, _ := json.Marshal([]agentBlueprintGraphEdge{
		{Source: "start", Target: "left"},
		{Source: "start", Target: "right"},
	})

	output, _, err := executeBlueprintNodeGraph(
		context.Background(),
		nil,
		&AgentBlueprint{ID: "fan-out", Nodes: nodes, Edges: edges},
		nil,
		map[string]interface{}{"type": "disabled"},
	)
	if err != nil {
		t.Fatal(err)
	}
	var graph agentBlueprintGraphExecution
	if err := json.Unmarshal(output, &graph); err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 3 {
		t.Fatalf("executed nodes = %d, want 3", len(graph.Nodes))
	}
	executed := map[string]bool{}
	for _, node := range graph.Nodes {
		executed[node.NodeID] = true
	}
	if !executed["left"] || !executed["right"] {
		t.Fatalf("fan-out execution = %#v", executed)
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

func TestValidateAgentBlueprintGraphRejectsCycle(t *testing.T) {
	first := agentBlueprintGraphNode{ID: "first"}
	second := agentBlueprintGraphNode{ID: "second"}
	err := validateAgentBlueprintGraph(
		[]agentBlueprintGraphNode{first, second},
		[]agentBlueprintGraphEdge{
			{ID: "a", Source: "first", Target: "second"},
			{ID: "b", Source: "second", Target: "first"},
		},
	)
	if err == nil {
		t.Fatal("expected cycle validation error")
	}
}

func TestAgentCodeConditionSupportsFieldExists(t *testing.T) {
	condition, err := parseAgentCodeCondition("status exists")
	if err != nil {
		t.Fatal(err)
	}
	if !condition.Match(map[string]interface{}{"status": "ok"}) {
		t.Fatal("expected existing status field to match")
	}
	if condition.Match(map[string]interface{}{"status": ""}) {
		t.Fatal("did not expect empty status field to match")
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
