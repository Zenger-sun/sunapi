package sunapi

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestListExecutionLogsReadsPendingTextOutput(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	created, err := store.CreateExecutionLog(ctx, "blueprint-1", "agent-1", 1)
	if err != nil {
		t.Fatalf("create execution log: %v", err)
	}

	logs, err := store.ListExecutionLogs(ctx, "blueprint-1", 1, 5)
	if err != nil {
		t.Fatalf("list execution logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("execution log count = %d, want 1", len(logs))
	}
	if logs[0].ID != created.ID {
		t.Fatalf("execution log ID = %q, want %q", logs[0].ID, created.ID)
	}
	if got := string(logs[0].Output); got != "{}" {
		t.Fatalf("execution log output = %q, want %q", got, "{}")
	}

	now := time.Now().Unix()
	page, total, err := store.ListExecutionLogsPage(ctx, 1, now-60, now+60, "", 1, 20)
	if err != nil {
		t.Fatalf("list execution logs page: %v", err)
	}
	if total != 1 || len(page) != 1 {
		t.Fatalf("execution log page total = %d, count = %d, want 1 and 1", total, len(page))
	}
	if got := string(page[0].Output); got != "{}" {
		t.Fatalf("paged execution log output = %q, want %q", got, "{}")
	}
}
