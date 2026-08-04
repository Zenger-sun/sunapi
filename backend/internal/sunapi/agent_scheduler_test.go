package sunapi

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestNextAgentScheduleTimeDailyUsesConfiguredTimezone(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	after := time.Date(2026, time.July, 14, 8, 31, 0, 0, location)
	next, err := nextAgentScheduleTime(agentScheduleConfig{
		Frequency: "daily",
		Time:      "08:30",
		Timezone:  "Asia/Shanghai",
	}, after, after)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, time.July, 15, 8, 30, 0, 0, location)
	if !next.Equal(want) {
		t.Fatalf("next schedule = %s, want %s", next, want)
	}
}

func TestNextAgentScheduleTimeWeeklyUsesSelectedWeekday(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	after := time.Date(2026, time.July, 14, 9, 0, 0, 0, location) // Tuesday
	next, err := nextAgentScheduleTime(agentScheduleConfig{
		Frequency: "weekly",
		Time:      "10:15",
		Timezone:  "Asia/Shanghai",
		Weekdays:  []int{4},
	}, after, after)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, time.July, 16, 10, 15, 0, 0, location)
	if !next.Equal(want) {
		t.Fatalf("next schedule = %s, want %s", next, want)
	}
}

func TestNextAgentScheduleTimeWeekdaysSkipsWeekend(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	friday := time.Date(2026, time.July, 24, 18, 0, 0, 0, location)
	next, err := nextAgentScheduleTime(agentScheduleConfig{
		Frequency: "weekdays",
		Time:      "09:00",
		Timezone:  "Asia/Shanghai",
	}, friday, friday)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, time.July, 27, 9, 0, 0, 0, location)
	if !next.Equal(want) {
		t.Fatalf("next schedule = %s, want %s", next, want)
	}
}

func TestBuildAgentTriggeredBlueprintKeepsOnlySchedulePath(t *testing.T) {
	nodes := []agentBlueprintGraphNode{
		newAgentSchedulerTestNode("manual", "start"),
		newAgentSchedulerTestNode("schedule", "schedule"),
		newAgentSchedulerTestNode("shared", "transform"),
		newAgentSchedulerTestNode("manual-only", "notify"),
	}
	edges := []agentBlueprintGraphEdge{
		{Source: "manual", Target: "shared"},
		{Source: "schedule", Target: "shared"},
		{Source: "manual", Target: "manual-only"},
	}
	rawNodes, _ := json.Marshal(nodes)
	rawEdges, _ := json.Marshal(edges)
	run := buildAgentTriggeredBlueprint(&AgentBlueprint{Nodes: rawNodes, Edges: rawEdges}, "schedule")
	gotNodes, err := parseAgentBlueprintNodes(run.Nodes)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotNodes) != 2 || gotNodes[0].ID != "schedule" || gotNodes[1].ID != "shared" {
		t.Fatalf("scheduled path nodes = %#v", gotNodes)
	}
}

func TestScheduledBlueprintPersistsActiveScheduleState(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	createAgentTestChannel(t, store, "default", "gpt-test")

	agent, err := store.CreateAgent(context.Background(), 1, AgentFormData{Name: "scheduled agent"})
	if err != nil {
		t.Fatal(err)
	}
	trigger, _ := json.Marshal(agentScheduleConfig{
		Type:      "scheduled",
		Frequency: "daily",
		Time:      "08:30",
		Timezone:  "Asia/Shanghai",
	})
	bp, err := store.CreateBlueprint(context.Background(), agent.ID, 1, BlueprintFormData{
		Name:          "daily report",
		ModelName:     "gpt-test",
		TriggerConfig: trigger,
	})
	if err != nil {
		t.Fatal(err)
	}
	if bp.Status != "running" {
		t.Fatalf("status = %q, want running", bp.Status)
	}
	if bp.NextExecutionTime <= time.Now().UnixMilli() {
		t.Fatalf("next execution time = %d, want a future timestamp", bp.NextExecutionTime)
	}
}

func TestRemovingScheduleResetsBlueprintToPending(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	createAgentTestChannel(t, store, "default", "gpt-test")

	ctx := context.Background()
	bp := createScheduledBlueprintForTest(t, store, ctx, "schedule removal")
	if err := store.UpdateBlueprintStatus(ctx, bp.ID, "running", "stale schedule failure"); err != nil {
		t.Fatal(err)
	}

	updated, err := store.UpdateBlueprint(ctx, bp.ID, 1, BlueprintFormData{
		Name:          bp.Name,
		ModelName:     bp.ModelName,
		TriggerConfig: json.RawMessage(`{"type":"manual","timezone":"Asia/Shanghai"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "pending" {
		t.Fatalf("status = %q, want pending", updated.Status)
	}
	if updated.NextExecutionTime != 0 {
		t.Fatalf("next execution time = %d, want 0", updated.NextExecutionTime)
	}
	if updated.FailureReason != "" {
		t.Fatalf("failure reason = %q, want empty", updated.FailureReason)
	}

	persisted, err := store.GetBlueprint(ctx, bp.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if persisted == nil || persisted.Status != "pending" || persisted.NextExecutionTime != 0 {
		t.Fatalf("persisted blueprint = %#v, want pending with no next execution", persisted)
	}
	if persisted.FailureReason != "" {
		t.Fatalf("persisted failure reason = %q, want empty", persisted.FailureReason)
	}
}

func TestScheduledRunCompletionUsesLatestTriggerConfig(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	createAgentTestChannel(t, store, "default", "gpt-test")

	ctx := context.Background()
	bp := createScheduledBlueprintForTest(t, store, ctx, "active schedule removal")
	if !store.beginAgentScheduledRun(bp.ID) {
		t.Fatal("failed to mark scheduled run active")
	}
	defer store.endAgentScheduledRun(bp.ID)

	updated, err := store.UpdateBlueprint(ctx, bp.ID, 1, BlueprintFormData{
		Name:          bp.Name,
		ModelName:     bp.ModelName,
		TriggerConfig: json.RawMessage(`{"type":"manual","timezone":"Asia/Shanghai"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "pending" {
		t.Fatalf("status after schedule removal = %q, want pending", updated.Status)
	}
	if !store.isAgentScheduledRunActive(bp.ID) {
		t.Fatal("scheduled run claim was lost while updating the blueprint")
	}

	if err := store.UpdateBlueprintAfterRun(ctx, bp.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	persisted, err := store.GetBlueprint(ctx, bp.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if persisted == nil || persisted.Status != "completed" || persisted.NextExecutionTime != 0 {
		t.Fatalf("persisted blueprint = %#v, want completed with no next execution", persisted)
	}
}

func TestReconcileResetsStaleManualBlueprintState(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	createAgentTestChannel(t, store, "default", "gpt-test")

	ctx := context.Background()
	agent, err := store.CreateAgent(ctx, 1, AgentFormData{Name: "stale manual agent"})
	if err != nil {
		t.Fatal(err)
	}
	bp, err := store.CreateBlueprint(ctx, agent.ID, 1, BlueprintFormData{
		Name:      "stale manual",
		ModelName: "gpt-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateBlueprintStatus(ctx, bp.ID, "running", ""); err != nil {
		t.Fatal(err)
	}

	if err := store.reconcileAgentSchedules(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	persisted, err := store.GetBlueprint(ctx, bp.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if persisted == nil || persisted.Status != "pending" || persisted.NextExecutionTime != 0 {
		t.Fatalf("reconciled blueprint = %#v, want pending with no next execution", persisted)
	}
}

func TestDispatchDueAgentBlueprintRunsSchedulePath(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	createAgentTestChannel(t, store, "default", "gpt-test")

	ctx := context.Background()
	agent, err := store.CreateAgent(ctx, 1, AgentFormData{Name: "dispatch agent"})
	if err != nil {
		t.Fatal(err)
	}
	trigger, _ := json.Marshal(agentScheduleConfig{
		Type:      "scheduled",
		Frequency: "daily",
		Time:      "08:30",
		Timezone:  "Asia/Shanghai",
	})
	nodes, _ := json.Marshal([]agentBlueprintGraphNode{
		newAgentSchedulerTestNode("schedule", "schedule"),
		newAgentSchedulerTestNode("stop", "stop"),
	})
	edges, _ := json.Marshal([]agentBlueprintGraphEdge{{Source: "schedule", Target: "stop"}})
	bp, err := store.CreateBlueprint(ctx, agent.ID, 1, BlueprintFormData{
		Name:          "scheduled dispatch",
		ModelName:     "gpt-test",
		TriggerConfig: trigger,
		ArchiveConfig: json.RawMessage(`{"type":"disabled"}`),
		Nodes:         nodes,
		Edges:         edges,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := store.updateAgentNextExecutionTime(ctx, bp.ID, now.Add(-time.Second).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err := store.dispatchDueAgentBlueprints(ctx, now); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		logs, err := store.ListExecutionLogs(ctx, bp.ID, 1, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(logs) > 0 && logs[0].Status == "completed" {
			updated, err := store.GetBlueprint(ctx, bp.ID, 1)
			if err != nil {
				t.Fatal(err)
			}
			if updated == nil || updated.Status != "running" {
				t.Fatalf("scheduled blueprint status = %#v, want running", updated)
			}
			if updated.NextExecutionTime <= now.UnixMilli() {
				t.Fatalf("next execution time = %d, want a future timestamp", updated.NextExecutionTime)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("scheduled blueprint did not complete before the deadline")
}

func createScheduledBlueprintForTest(t *testing.T, store *Store, ctx context.Context, name string) *AgentBlueprint {
	t.Helper()
	agent, err := store.CreateAgent(ctx, 1, AgentFormData{Name: name + " agent"})
	if err != nil {
		t.Fatal(err)
	}
	trigger, err := json.Marshal(agentScheduleConfig{
		Type:      "scheduled",
		Frequency: "daily",
		Time:      "08:30",
		Timezone:  "Asia/Shanghai",
	})
	if err != nil {
		t.Fatal(err)
	}
	bp, err := store.CreateBlueprint(ctx, agent.ID, 1, BlueprintFormData{
		Name:          name,
		ModelName:     "gpt-test",
		TriggerConfig: trigger,
	})
	if err != nil {
		t.Fatal(err)
	}
	return bp
}

func newAgentSchedulerTestNode(id, kind string) agentBlueprintGraphNode {
	node := agentBlueprintGraphNode{ID: id}
	node.Data.Kind = kind
	return node
}
