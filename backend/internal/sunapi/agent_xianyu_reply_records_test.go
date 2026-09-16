package sunapi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAgentXianyuReplyRecordsAppendAndListNewestFirst(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	base := time.Now().UnixMilli()
	for index, status := range []string{"sent", "sent_no_match", "send_error"} {
		record := AgentXianyuReplyRecord{
			BlueprintID:         "bp-reply",
			AgentID:             "agent-1",
			NodeID:              "node-reply",
			ConversationKey:     "conversation-" + string(rune('a'+index)),
			IncomingMessage:     "消息",
			KnowledgeMatchCount: index,
			KnowledgeQuery:      "商品\n消息",
			FinalReply:          "回复",
			Status:              status,
			StartedAt:           base + int64(index)*10,
			CompletedAt:         base + int64(index)*10 + 5,
			CreatedAt:           base + int64(index)*10,
		}
		if err := store.AppendAgentXianyuReplyRecord(ctx, record); err != nil {
			t.Fatalf("append record %d: %v", index, err)
		}
	}

	records, err := store.ListAgentXianyuReplyRecords(ctx, "bp-reply", 2)
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("record count = %d, want 2", len(records))
	}
	if records[0].Status != "send_error" || records[1].Status != "sent_no_match" {
		t.Fatalf("records are not newest first: %#v", records)
	}
	if records[0].ID == "" || records[1].ID == "" {
		t.Fatal("record IDs should be generated")
	}
}

func TestAgentXianyuReplyRecordsSkipMalformedLines(t *testing.T) {
	dataDir := t.TempDir()
	store, err := OpenStore(filepath.Join(dataDir, "sunapi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	path := agentXianyuReplyRecordPath(dataDir, "bp-malformed")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("create records directory: %v", err)
	}
	record := AgentXianyuReplyRecord{BlueprintID: "bp-malformed", IncomingMessage: "ok", Status: "sent", CreatedAt: time.Now().UnixMilli()}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	if err := os.WriteFile(path, append([]byte("not-json\n"), append(raw, '\n')...), 0644); err != nil {
		t.Fatalf("write records file: %v", err)
	}

	records, err := store.ListAgentXianyuReplyRecords(context.Background(), "bp-malformed", 10)
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if len(records) != 1 || records[0].IncomingMessage != "ok" {
		t.Fatalf("malformed line should be skipped: %#v", records)
	}
}
