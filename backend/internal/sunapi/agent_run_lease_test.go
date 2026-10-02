package sunapi

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestAgentRunLeaseHeartbeatNormalizesNilContext(t *testing.T) {
	heartbeatCtx, stop := startAgentRunLeaseHeartbeat(nil, nil, agentRunLease{})
	defer stop()
	if heartbeatCtx == nil {
		t.Fatal("heartbeat helper must return a usable context")
	}
}

func TestStoreCloseIsIdempotent(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	store.StartAgentScheduler()
	if err := store.Close(); err != nil {
		t.Fatalf("first Close error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second Close error = %v", err)
	}
	store.StartAgentScheduler()
	if !store.agentSchedulerClosed {
		t.Fatal("closed store must reject scheduler restart")
	}
}

func TestAgentRunLeaseSerializesBlueprintRuns(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	first := agentRunLease{BlueprintID: "bp-1", RunID: "run-1", Kind: "manual"}
	acquired, err := store.acquireAgentRunLease(ctx, first.BlueprintID, first.RunID, first.Kind)
	if err != nil || !acquired {
		t.Fatalf("first lease acquired=%v err=%v", acquired, err)
	}
	second, err := store.acquireAgentRunLease(ctx, "bp-1", "run-2", "webhook")
	if err != nil {
		t.Fatalf("second acquisition error = %v", err)
	}
	if second {
		t.Fatal("a second run acquired the same blueprint lease")
	}
	if err := store.releaseAgentRunLease(ctx, first); err != nil {
		t.Fatalf("release lease error = %v", err)
	}
	second, err = store.acquireAgentRunLease(ctx, "bp-1", "run-2", "webhook")
	if err != nil || !second {
		t.Fatalf("lease was not reusable after release acquired=%v err=%v", second, err)
	}
}

func TestAgentRunLeaseWaitsForPausedRunCleanup(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	defer store.Close()
	createAgentTestChannel(t, store, "default", "gpt-test")

	ctx := context.Background()
	agent, err := store.CreateAgent(ctx, 1, AgentFormData{Name: "paused lease test agent"})
	if err != nil {
		t.Fatalf("CreateAgent error = %v", err)
	}
	bp, err := store.CreateBlueprint(ctx, agent.ID, 1, BlueprintFormData{
		Name:          "paused lease test blueprint",
		ModelName:     "gpt-test",
		TriggerConfig: json.RawMessage(`{"type":"manual"}`),
	})
	if err != nil {
		t.Fatalf("CreateBlueprint error = %v", err)
	}
	if err := store.UpdateBlueprintScheduleState(ctx, bp.ID, "paused", "", 0); err != nil {
		t.Fatalf("pause blueprint error = %v", err)
	}
	oldRun := agentRunLease{BlueprintID: bp.ID, RunID: "old-run", Kind: "manual"}
	acquired, err := store.acquireAgentRunLease(ctx, oldRun.BlueprintID, oldRun.RunID, oldRun.Kind)
	if err != nil || !acquired {
		t.Fatalf("old run lease acquired=%v err=%v", acquired, err)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = store.releaseAgentRunLease(ctx, oldRun)
	}()

	newRunID := "new-run"
	acquired, err = store.acquireAgentRunLeaseAfterPause(ctx, bp.ID, newRunID, "manual")
	if err != nil || !acquired {
		t.Fatalf("new run lease acquired after pause=%v err=%v", acquired, err)
	}
}

func TestAgentRunLeaseOwnershipCheckRejectsReplacedRun(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	first := agentRunLease{BlueprintID: "bp-ownership", RunID: "run-1", Kind: "manual"}
	acquired, err := store.acquireAgentRunLease(ctx, first.BlueprintID, first.RunID, first.Kind)
	if err != nil || !acquired {
		t.Fatalf("first lease acquired=%v err=%v", acquired, err)
	}
	owned, err := store.ownsAgentRunLease(ctx, first)
	if err != nil || !owned {
		t.Fatalf("expected first lease to be owned=%v err=%v", owned, err)
	}
	if err := store.releaseAgentRunLease(ctx, first); err != nil {
		t.Fatalf("release first lease error = %v", err)
	}
	second := agentRunLease{BlueprintID: first.BlueprintID, RunID: "run-2", Kind: "manual"}
	acquired, err = store.acquireAgentRunLease(ctx, second.BlueprintID, second.RunID, second.Kind)
	if err != nil || !acquired {
		t.Fatalf("second lease acquired=%v err=%v", acquired, err)
	}
	owned, err = store.ownsAgentRunLease(ctx, first)
	if err != nil {
		t.Fatalf("stale ownership check error = %v", err)
	}
	if owned {
		t.Fatal("replaced lease must not remain owned by the old run")
	}
}

func TestAgentRunLeasePreventsStaleFinalStateWrite(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	defer store.Close()
	createAgentTestChannel(t, store, "default", "gpt-test")

	ctx := context.Background()
	agent, err := store.CreateAgent(ctx, 1, AgentFormData{Name: "lease test agent"})
	if err != nil {
		t.Fatalf("CreateAgent error = %v", err)
	}
	bp, err := store.CreateBlueprint(ctx, agent.ID, 1, BlueprintFormData{
		Name:          "lease test blueprint",
		ModelName:     "gpt-test",
		TriggerConfig: json.RawMessage(`{"type":"manual"}`),
	})
	if err != nil {
		t.Fatalf("CreateBlueprint error = %v", err)
	}
	first := agentRunLease{BlueprintID: bp.ID, RunID: "run-1", Kind: "manual"}
	acquired, err := store.acquireAgentRunLease(ctx, first.BlueprintID, first.RunID, first.Kind)
	if err != nil || !acquired {
		t.Fatalf("first lease acquired=%v err=%v", acquired, err)
	}
	if err := store.releaseAgentRunLease(ctx, first); err != nil {
		t.Fatalf("release first lease error = %v", err)
	}
	second := agentRunLease{BlueprintID: bp.ID, RunID: "run-2", Kind: "manual"}
	acquired, err = store.acquireAgentRunLease(ctx, second.BlueprintID, second.RunID, second.Kind)
	if err != nil || !acquired {
		t.Fatalf("second lease acquired=%v err=%v", acquired, err)
	}
	updated, err := store.updateAgentBlueprintAfterRunForLease(ctx, first, "completed", "")
	if err != nil {
		t.Fatalf("stale final-state update error = %v", err)
	}
	if updated {
		t.Fatal("stale lease updated the replacement run's blueprint")
	}
	persisted, err := store.GetBlueprintByID(ctx, bp.ID)
	if err != nil {
		t.Fatalf("GetBlueprintByID error = %v", err)
	}
	if persisted.Status != "pending" {
		t.Fatalf("status after stale update = %q, want pending", persisted.Status)
	}
	updated, err = store.updateAgentBlueprintAfterRunForLease(ctx, second, "completed", "")
	if err != nil || !updated {
		t.Fatalf("replacement final-state update updated=%v err=%v", updated, err)
	}
	persisted, err = store.GetBlueprintByID(ctx, bp.ID)
	if err != nil {
		t.Fatalf("GetBlueprintByID after replacement update error = %v", err)
	}
	if persisted.Status != "completed" {
		t.Fatalf("status after replacement update = %q, want completed", persisted.Status)
	}
}
