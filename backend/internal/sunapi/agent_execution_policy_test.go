package sunapi

import (
	"context"
	"testing"
)

func TestExecuteAgentBlueprintNodeWithPolicyRetriesAndContinues(t *testing.T) {
	node := agentBlueprintGraphNode{ID: "request"}
	node.Data.Kind = "request"
	node.Data.Settings = map[string]string{
		"失败重试": "2 次",
		"重试间隔": "0s",
		"失败处理": "继续执行",
	}

	result := executeAgentBlueprintNodeWithPolicy(
		context.Background(),
		nil,
		nil,
		node,
		nil,
		map[string]interface{}{},
	)
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if result.Output["attempt"] != 3 {
		t.Fatalf("attempt = %#v, want 3", result.Output["attempt"])
	}
	if result.Output["handled_error"] == "" {
		t.Fatalf("handled_error = %#v, want failure detail", result.Output["handled_error"])
	}
}

func TestExecuteAgentBlueprintNodeWithPolicyTimesOut(t *testing.T) {
	node := agentBlueprintGraphNode{ID: "wait"}
	node.Data.Kind = "custom"
	node.Data.ComponentID = "component.wait"
	node.Data.Settings = map[string]string{
		"等待时间": "2s",
		"最大等待": "5s",
		"节点超时": "10ms",
	}

	result := executeAgentBlueprintNodeWithPolicy(
		context.Background(),
		nil,
		nil,
		node,
		map[string]interface{}{"source": map[string]interface{}{"value": true}},
		map[string]interface{}{},
	)
	if result.Status != "failed" {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	if result.Output["timed_out"] != true {
		t.Fatalf("timed_out = %#v, want true", result.Output["timed_out"])
	}
}
