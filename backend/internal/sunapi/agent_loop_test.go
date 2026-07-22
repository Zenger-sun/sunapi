package sunapi

import (
	"context"
	"testing"
)

func TestAgentLoopItemsPropagateAndCollect(t *testing.T) {
	loopNode := agentBlueprintGraphNode{ID: "loop"}
	loopNode.Data.Kind = "custom"
	loopNode.Data.ComponentID = "component.loop"
	loopNode.Data.Settings = map[string]string{"最大条目": "10", "输出字段": "items"}
	loopResult := executeAgentLoopGraphNode(
		loopNode,
		map[string]interface{}{
			"source": map[string]interface{}{
				"items": []map[string]interface{}{
					{"id": "a", "score": 2},
					{"id": "b", "score": 1},
				},
			},
		},
		agentBlueprintNodeResult{Status: "completed"},
	)
	if loopResult.Status != "completed" {
		t.Fatalf("loop status = %q, error = %s", loopResult.Status, loopResult.Error)
	}

	edges := []agentBlueprintGraphEdge{{
		Source: "loop", Target: "sort", SourceHandle: agentLoopItemHandle,
	}}
	outputs := map[string]map[string]interface{}{"loop": loopResult.Output}
	items, ok := agentLoopItemsForNode("sort", edges, outputs)
	if !ok || len(items) != 2 {
		t.Fatalf("loop items = %#v, ok = %v, want 2 items", items, ok)
	}

	sortNode := agentBlueprintGraphNode{ID: "sort"}
	sortNode.Data.Kind = "custom"
	sortNode.Data.ComponentID = "component.dedupe_sort"
	sortNode.Data.Settings = map[string]string{"去重字段": "id", "排序字段": "score"}
	sortResult := executeAgentBlueprintNodeForEach(
		context.Background(),
		nil,
		nil,
		sortNode,
		edges,
		outputs,
		items,
		map[string]interface{}{},
		false,
	)
	if sortResult.Status != "completed" {
		t.Fatalf("per-item status = %q, error = %s", sortResult.Status, sortResult.Error)
	}
	if sortResult.Output[agentLoopContextKey] != true {
		t.Fatalf("loop context = %#v, want true", sortResult.Output[agentLoopContextKey])
	}

	collectNode := agentBlueprintGraphNode{ID: "collect"}
	collectResult := executeAgentLoopCollectGraphNode(
		collectNode,
		map[string]interface{}{"sort": sortResult.Output},
		agentBlueprintNodeResult{Status: "completed"},
	)
	if collectResult.Status != "completed" {
		t.Fatalf("collect status = %q, error = %s", collectResult.Status, collectResult.Error)
	}
	if collectResult.Output["count"] != 2 {
		t.Fatalf("collected count = %#v, want 2", collectResult.Output["count"])
	}
}

func TestAgentLoopBranchRoutesMatchingItems(t *testing.T) {
	edges := []agentBlueprintGraphEdge{{
		Source: "loop", Target: "branch", SourceHandle: agentLoopItemHandle,
	}}
	outputs := map[string]map[string]interface{}{
		"loop": {
			"items": []map[string]interface{}{
				{"id": "a", "status": "ok"},
				{"id": "b", "status": "error"},
			},
		},
	}
	items, ok := agentLoopItemsForNode("branch", edges, outputs)
	if !ok {
		t.Fatal("expected loop items")
	}
	branch := agentBlueprintGraphNode{ID: "branch"}
	branch.Data.Kind = "custom"
	branch.Data.ComponentID = "component.branch"
	branch.Data.Settings = map[string]string{"条件": "status=ok"}
	branchResult := executeAgentBlueprintNodeForEach(
		context.Background(), nil, nil, branch, edges, outputs, items,
		map[string]interface{}{}, false,
	)
	if branchResult.Status != "completed" {
		t.Fatalf("branch status = %q, error = %s", branchResult.Status, branchResult.Error)
	}

	routeEdges := []agentBlueprintGraphEdge{{
		Source: "branch", Target: "matched", SourceHandle: agentBranchTrueHandle,
	}}
	routeOutputs := map[string]map[string]interface{}{"branch": branchResult.Output}
	matched, ok := agentLoopItemsForNode("matched", routeEdges, routeOutputs)
	if !ok || len(matched) != 1 {
		t.Fatalf("matched loop items = %#v, ok = %v, want 1", matched, ok)
	}
}
