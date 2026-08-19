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
