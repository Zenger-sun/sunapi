package sunapi

import (
	"encoding/json"
	"testing"
)

func TestBlueprintHasScheduleNode(t *testing.T) {
	node := agentBlueprintGraphNode{ID: "schedule-1"}
	node.Data.Kind = "schedule"
	nodes, err := json.Marshal([]agentBlueprintGraphNode{node})
	if err != nil {
		t.Fatal(err)
	}
	if !blueprintHasScheduleNode(&AgentBlueprint{Nodes: nodes}) {
		t.Fatalf("expected schedule node to be detected")
	}
}

func TestBlueprintHasScheduleNodeReturnsFalseForManualBlueprint(t *testing.T) {
	node := agentBlueprintGraphNode{ID: "start-1"}
	node.Data.Kind = "start"
	nodes, err := json.Marshal([]agentBlueprintGraphNode{node})
	if err != nil {
		t.Fatal(err)
	}
	if blueprintHasScheduleNode(&AgentBlueprint{Nodes: nodes}) {
		t.Fatalf("did not expect manual blueprint to be scheduled")
	}
}
