package sunapi

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestKnowledgeBaseLifecycleAndSearch(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	base, err := store.CreateKnowledgeBase(ctx, 7, KnowledgeBaseFormData{
		Name:        "产品手册",
		Description: "用于创作台检索",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !base.Enabled {
		t.Fatal("knowledge bases should be available for explicit selection")
	}
	document, err := store.AddKnowledgeDocument(ctx, base.ID, 7, "manual.md", "text/markdown", 42, "SunAPI 创作台支持知识库检索。\n知识库可以被开启并注入模型上下文。")
	if err != nil {
		t.Fatal(err)
	}
	if document.ChunkCount == 0 {
		t.Fatal("document should be chunked")
	}
	loadedDocument, err := store.GetKnowledgeDocument(ctx, base.ID, document.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if loadedDocument == nil || strings.TrimSpace(loadedDocument.Content) == "" {
		t.Fatal("document content should be available for preview")
	}
	beforeSelection, err := store.SearchKnowledge(ctx, 7, "知识库检索", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(beforeSelection) != 0 {
		t.Fatal("knowledge search without an explicit selection must be empty")
	}
	matches, err := store.SearchKnowledgeForScope(ctx, 7, "开启知识库", 5, KnowledgeSearchScope{
		KnowledgeBaseIDs: []string{base.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].DocumentID != document.ID {
		t.Fatalf("unexpected search matches: %+v", matches)
	}
	if err := store.DeleteKnowledgeDocument(ctx, base.ID, document.ID, 7); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteKnowledgeBase(ctx, base.ID, 7); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListKnowledgeBases(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("expected knowledge base to be deleted, got %d", len(items))
	}
}

func TestUpsertKnowledgeDocumentReplacesGeneratedContentInPlace(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	base, err := store.CreateKnowledgeBase(ctx, 7, KnowledgeBaseFormData{Name: "generated"})
	if err != nil {
		t.Fatal(err)
	}
	const documentID = "doc-stable-generated"
	first, err := store.UpsertKnowledgeDocument(ctx, base.ID, documentID, 7, "generated.md", "text/markdown", 1200, strings.Repeat("old policy ", 160))
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpsertKnowledgeDocument(ctx, base.ID, documentID, 7, "generated.md", "text/markdown", 22, "replacement policy answer")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != updated.ID || updated.ID != documentID {
		t.Fatalf("upsert should preserve the document id: first=%q updated=%q", first.ID, updated.ID)
	}
	loaded, err := store.GetKnowledgeDocument(ctx, base.ID, documentID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.Content != "replacement policy answer" {
		t.Fatalf("upsert did not replace document content: %+v", loaded)
	}
	documents, err := store.ListKnowledgeDocuments(ctx, base.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 1 {
		t.Fatalf("upsert should not append document versions: %+v", documents)
	}
	reloadedBase, err := store.GetKnowledgeBase(ctx, base.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if reloadedBase == nil || reloadedBase.DocumentCount != 1 || reloadedBase.ChunkCount != updated.ChunkCount {
		t.Fatalf("knowledge base counters were not updated: %+v", reloadedBase)
	}
}

func TestKnowledgeSearchExplicitSelectionIsIndependentPerBlueprint(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	first, err := store.CreateKnowledgeBase(ctx, 7, KnowledgeBaseFormData{Name: "first"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateKnowledgeBase(ctx, 7, KnowledgeBaseFormData{Name: "second"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AddKnowledgeDocument(ctx, first.ID, 7, "first.md", "text/markdown", 10, "shared keyword first blueprint"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.AddKnowledgeDocument(ctx, second.ID, 7, "second.md", "text/markdown", 10, "shared keyword second blueprint"); err != nil {
		t.Fatal(err)
	}

	firstMatches, err := store.SearchKnowledgeForScope(ctx, 7, "shared keyword", 5, KnowledgeSearchScope{
		KnowledgeBaseIDs: []string{first.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(firstMatches) != 1 || firstMatches[0].KnowledgeBaseID != first.ID {
		t.Fatalf("expected only the first blueprint selection, got %+v", firstMatches)
	}

	secondMatches, err := store.SearchKnowledgeForScope(ctx, 7, "shared keyword", 5, KnowledgeSearchScope{
		KnowledgeBaseIDs: []string{second.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(secondMatches) != 1 || secondMatches[0].KnowledgeBaseID != second.ID {
		t.Fatalf("expected only the second blueprint selection, got %+v", secondMatches)
	}
}

func TestKnowledgeSearchResolvesBlueprintBindingWithoutGlobalSelection(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	createAgentTestChannel(t, store, "codex", "gpt-5.5")
	agent, err := store.CreateAgent(ctx, 7, AgentFormData{Name: "support"})
	if err != nil {
		t.Fatal(err)
	}
	blueprint, err := store.CreateBlueprint(ctx, agent.ID, 7, BlueprintFormData{
		Name:       "customer service",
		ModelGroup: "codex",
		ModelName:  "gpt-5.5",
	})
	if err != nil {
		t.Fatal(err)
	}
	base, err := store.CreateKnowledgeBase(ctx, 7, KnowledgeBaseFormData{Name: "support faq"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AddKnowledgeDocument(ctx, base.ID, 7, "faq.md", "text/markdown", 10, "refund policy support answer"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SetKnowledgeBindings(ctx, base.ID, 7, []KnowledgeBindingFormData{{
		TargetType: knowledgeBindingBlueprint,
		TargetID:   blueprint.ID,
	}}); err != nil {
		t.Fatal(err)
	}

	matches, err := store.SearchKnowledgeForScope(ctx, 7, "refund policy", 5, KnowledgeSearchScope{
		Module:      "agent",
		AgentID:     agent.ID,
		BlueprintID: blueprint.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].KnowledgeBaseID != base.ID {
		t.Fatalf("expected blueprint binding to select the knowledge base, got %+v", matches)
	}

	contextText := store.BuildKnowledgeContextForScope(ctx, 7, "unknown question", 5, KnowledgeSearchScope{
		Module:      "agent",
		AgentID:     agent.ID,
		BlueprintID: blueprint.ID,
	})
	if !strings.Contains(contextText, "No relevant excerpt was found") {
		t.Fatalf("expected no-match policy, got %q", contextText)
	}
}

func TestKnowledgeFAQLifecycleAndSearch(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	base, err := store.CreateKnowledgeBase(ctx, 7, KnowledgeBaseFormData{Name: "faq"})
	if err != nil {
		t.Fatal(err)
	}
	faq, err := store.CreateKnowledgeFAQ(ctx, base.ID, 7, KnowledgeFAQFormData{
		Question: "How long is shipping?",
		Answer:   "Shipping usually takes two business days.",
		Keywords: []string{"delivery", "shipping"},
	})
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.ListKnowledgeFAQs(ctx, base.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != faq.ID || len(items[0].Keywords) != 2 {
		t.Fatalf("unexpected faq list: %+v", items)
	}
	matches, err := store.SearchKnowledgeForScope(ctx, 7, "delivery", 5, KnowledgeSearchScope{
		KnowledgeBaseIDs: []string{base.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].DocumentID != faq.ID || !strings.Contains(matches[0].Content, "two business days") {
		t.Fatalf("expected faq search match, got %+v", matches)
	}
	if err := store.DeleteKnowledgeFAQ(ctx, base.ID, faq.ID, 7); err != nil {
		t.Fatal(err)
	}
	items, err = store.ListKnowledgeFAQs(ctx, base.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("expected faq to be deleted, got %+v", items)
	}
}

func TestKnowledgeContextDoesNotUseUnselectedBase(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	base, err := store.CreateKnowledgeBase(ctx, 7, KnowledgeBaseFormData{Name: "private"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AddKnowledgeDocument(ctx, base.ID, 7, "private.md", "text/markdown", 1, "private refund answer"); err != nil {
		t.Fatal(err)
	}
	if contextText := store.BuildKnowledgeContextForScope(ctx, 7, "refund", 5, KnowledgeSearchScope{}); contextText != "" {
		t.Fatalf("expected no context without explicit selection or binding, got %q", contextText)
	}
	if contextText := store.BuildKnowledgeContextForScope(ctx, 7, "unrelated", 5, KnowledgeSearchScope{KnowledgeBaseIDs: []string{base.ID}}); !strings.Contains(contextText, "No relevant excerpt was found") {
		t.Fatalf("expected no-match policy for selected base, got %q", contextText)
	}
}

func TestSplitKnowledgeTextKeepsOverlap(t *testing.T) {
	chunks := splitKnowledgeText("abcdefghijklmnopqrstuvwxyz", 10, 2)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[0][8:] != chunks[1][:2] {
		t.Fatalf("expected overlap between chunks: %q / %q", chunks[0], chunks[1])
	}
}

func TestKnowledgeBindingsValidateTargetsAndPersistConversation(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	base, err := store.CreateKnowledgeBase(ctx, 7, KnowledgeBaseFormData{Name: "scope"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetKnowledgeBindings(ctx, base.ID, 7, []KnowledgeBindingFormData{{
		TargetType: knowledgeBindingAgent,
		TargetID:   "missing-agent",
	}}); err == nil {
		t.Fatal("expected missing agent binding to be rejected")
	}

	session, err := store.SavePlaygroundSession(ctx, 7, PlaygroundSessionPayload{
		Title:    "scope conversation",
		Model:    "model",
		Group:    "default",
		Messages: json.RawMessage(`[]`),
		Config:   json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := store.SetKnowledgeBindings(ctx, base.ID, 7, []KnowledgeBindingFormData{{
		TargetType: knowledgeBindingConversation,
		TargetID:   "999999",
	}})
	if err == nil || !strings.Contains(err.Error(), "conversation binding target not found") {
		t.Fatalf("expected missing conversation binding to be rejected, got %v", err)
	}
	bindings, err = store.SetKnowledgeBindings(ctx, base.ID, 7, []KnowledgeBindingFormData{{
		TargetType: knowledgeBindingConversation,
		TargetID:   strconv.FormatInt(session.ID, 10),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].TargetID != strconv.FormatInt(session.ID, 10) {
		t.Fatalf("unexpected persisted bindings: %+v", bindings)
	}
}
