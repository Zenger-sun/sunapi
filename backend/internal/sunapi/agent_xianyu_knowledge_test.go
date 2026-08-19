package sunapi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeAgentXianyuURLRestrictsOfficialHosts(t *testing.T) {
	accepted := []string{
		"https://seller.goofish.com/?site=COMM",
		"https://www.xianyu.com/messages",
	}
	for _, value := range accepted {
		if _, err := normalizeAgentXianyuURL(value); err != nil {
			t.Fatalf("expected %q to be accepted: %v", value, err)
		}
	}
	for _, value := range []string{
		"https://example.com/seller.goofish.com",
		"https://seller.goofish.com.example.com/",
		"file:///tmp/messages.html",
	} {
		if _, err := normalizeAgentXianyuURL(value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}

func TestResolveAgentXianyuKnowledgeBasesUsesBlueprintSelection(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	base, err := store.CreateKnowledgeBase(ctx, 7, KnowledgeBaseFormData{Name: "闲鱼客服"})
	if err != nil {
		t.Fatal(err)
	}
	bp := &AgentBlueprint{ID: "bp-xianyu", AgentID: "ag-xianyu", UserID: 7}

	if _, _, err := resolveAgentXianyuKnowledgeBases(ctx, store, bp, map[string]interface{}{
		"knowledge_disabled": true,
	}); err == nil {
		t.Fatal("expected a disabled knowledge scope to be rejected")
	}
	bases, ids, err := resolveAgentXianyuKnowledgeBases(ctx, store, bp, map[string]interface{}{
		"knowledge_base_ids": []interface{}{base.ID, base.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bases) != 1 || len(ids) != 1 || bases[0].ID != base.ID {
		t.Fatalf("unexpected resolved knowledge bases: %+v / %+v", bases, ids)
	}
}

func TestAgentXianyuIncrementalStateStoresOnlyHashes(t *testing.T) {
	items := []agentXianyuConversation{{
		Key:        "buyer-1",
		Title:      "会话 1",
		Transcript: "买家：订单号 12345678901 怎么处理？",
	}}
	changed, hashes := filterAgentXianyuChangedConversations(items, agentXianyuSyncState{}, false)
	if len(changed) != 1 || len(hashes) != 1 {
		t.Fatalf("unexpected first incremental result: %+v / %+v", changed, hashes)
	}
	state := agentXianyuSyncState{Conversations: hashes}
	changed, _ = filterAgentXianyuChangedConversations(items, state, false)
	if len(changed) != 0 {
		t.Fatalf("unchanged conversation should not be archived again: %+v", changed)
	}
	items[0].Transcript += "\n客服：已处理。"
	changed, hashes = filterAgentXianyuChangedConversations(items, state, false)
	if len(changed) != 1 {
		t.Fatal("updated conversation should be archived")
	}

	path := agentXianyuStatePath(t.TempDir(), "bp-1", "node-1")
	if err := saveAgentXianyuSyncState(path, agentXianyuSyncState{Conversations: hashes}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "订单号") || strings.Contains(string(raw), "已处理") {
		t.Fatalf("sync state must not contain raw conversation text: %s", raw)
	}
	var persisted agentXianyuSyncState
	if err := json.Unmarshal(raw, &persisted); err != nil || len(persisted.Conversations) != 1 {
		t.Fatalf("unexpected persisted state: %+v, %v", persisted, err)
	}
}

func TestWriteAgentXianyuKnowledgeDocumentsCreatesPreviewableDocument(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	first, err := store.CreateKnowledgeBase(ctx, 7, KnowledgeBaseFormData{Name: "客服知识一"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateKnowledgeBase(ctx, 7, KnowledgeBaseFormData{Name: "客服知识二"})
	if err != nil {
		t.Fatal(err)
	}
	content := buildAgentXianyuKnowledgeDocument(
		agentXianyuDefaultURL,
		agentXianyuDefaultSession,
		[]agentXianyuConversation{{Key: "buyer-1", Transcript: "raw message"}},
		"## 常见问题\n\n退货申请应先核对商品状态。联系电话 13800138000。",
	)
	if strings.Contains(content, "raw message") || strings.Contains(content, "13800138000") {
		t.Fatalf("knowledge document should contain only a redacted summary: %q", content)
	}
	documents, err := writeAgentXianyuKnowledgeDocuments(ctx, store, []*KnowledgeBase{first, second}, "闲鱼客服知识.md", content)
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 2 {
		t.Fatalf("expected one document in each selected knowledge base, got %+v", documents)
	}
	loaded, err := store.GetKnowledgeDocument(ctx, first.ID, documents[0].DocumentID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || !strings.Contains(loaded.Content, "退货申请") {
		t.Fatalf("expected generated knowledge to be previewable: %+v", loaded)
	}
}

func TestBuildAgentXianyuExpressionsQuoteCustomSelectors(t *testing.T) {
	selector := `[data-name="customer's chat"]`
	for _, expression := range []string{
		buildAgentXianyuCandidatesExpression(selector, 20),
		buildAgentXianyuClickExpression(selector, agentXianyuConversationCandidate{Key: `key"'`, Title: "买家"}),
		buildAgentXianyuTranscriptExpression(selector),
		buildAgentXianyuScrollExpression(selector),
	} {
		if !strings.Contains(expression, `customer's chat`) {
			t.Fatalf("expected selector in expression: %s", expression)
		}
	}
}
