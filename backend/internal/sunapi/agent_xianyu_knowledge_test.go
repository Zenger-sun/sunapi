package sunapi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	for _, private := range []string{"来源网址", "浏览器会话", "会话归档标识"} {
		if strings.Contains(content, private) {
			t.Fatalf("knowledge document should not persist internal metadata %q: %q", private, content)
		}
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

func TestCommitAgentXianyuConversationKnowledgeCheckpointsAndReplaces(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	base, err := store.CreateKnowledgeBase(ctx, 7, KnowledgeBaseFormData{Name: "xianyu"})
	if err != nil {
		t.Fatal(err)
	}
	statePath := agentXianyuStatePath(t.TempDir(), "bp-resume", "node-resume")
	state := agentXianyuSyncState{Conversations: map[string]string{}}
	conversation := agentXianyuConversation{
		Key:        "buyer-private-key",
		Title:      "private buyer name",
		Transcript: "private raw transcript",
	}
	identity := agentXianyuConversationIdentity(conversation.Key)
	firstHash := agentXianyuConversationHash(conversation)
	firstDocs, err := commitAgentXianyuConversationKnowledge(
		ctx, store, []*KnowledgeBase{base}, "bp-resume", "node-resume", statePath,
		&state, conversation, firstHash, agentXianyuDefaultURL, agentXianyuDefaultSession, "first support answer",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstDocs) != 1 || state.Conversations[identity] != firstHash {
		t.Fatalf("first checkpoint was not committed: docs=%+v state=%+v", firstDocs, state)
	}
	persisted, err := loadAgentXianyuSyncState(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Conversations[identity] != firstHash {
		t.Fatalf("conversation hash was not persisted: %+v", persisted)
	}

	conversation.Transcript += "\nnew message"
	secondHash := agentXianyuConversationHash(conversation)
	secondDocs, err := commitAgentXianyuConversationKnowledge(
		ctx, store, []*KnowledgeBase{base}, "bp-resume", "node-resume", statePath,
		&state, conversation, secondHash, agentXianyuDefaultURL, agentXianyuDefaultSession, "replacement support answer",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(secondDocs) != 1 || firstDocs[0].DocumentID != secondDocs[0].DocumentID {
		t.Fatalf("conversation update should replace the stable document: first=%+v second=%+v", firstDocs, secondDocs)
	}
	documents, err := store.ListKnowledgeDocuments(ctx, base.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 1 {
		t.Fatalf("conversation update appended a stale document: %+v", documents)
	}
	loaded, err := store.GetKnowledgeDocument(ctx, base.ID, secondDocs[0].DocumentID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || !strings.Contains(loaded.Content, "replacement support answer") || strings.Contains(loaded.Content, "first support answer") {
		t.Fatalf("stable document was not replaced: %+v", loaded)
	}
	for _, privateValue := range []string{conversation.Key, conversation.Title, conversation.Transcript} {
		if strings.Contains(loaded.Content, privateValue) {
			t.Fatalf("knowledge document leaked raw conversation data %q", privateValue)
		}
	}
	persisted, err = loadAgentXianyuSyncState(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Conversations[identity] != secondHash {
		t.Fatalf("updated conversation hash was not checkpointed: %+v", persisted)
	}
}

func TestBuildAgentXianyuExpressionsQuoteCustomSelectors(t *testing.T) {
	selector := `[data-name="customer's chat"]`
	for _, expression := range []string{
		buildAgentXianyuCandidatesExpression(selector, 20),
		buildAgentXianyuClickExpression(selector, agentXianyuConversationCandidate{Key: `key"'`, Title: "买家"}),
		buildAgentXianyuTranscriptExpression(selector),
		buildAgentXianyuProductContextExpression(selector),
		buildAgentXianyuScrollExpression(selector),
		buildAgentXianyuScrollToLatestExpression(selector),
		buildAgentXianyuConversationListScrollExpression(selector),
		buildAgentXianyuConversationListResetExpression(selector),
	} {
		if !strings.Contains(expression, `customer's chat`) {
			t.Fatalf("expected selector in expression: %s", expression)
		}
	}
}

func TestAgentXianyuScrollExpressionsReportBoundaries(t *testing.T) {
	up := buildAgentXianyuScrollExpression("")
	if !strings.Contains(up, "dispatchEvent") || !strings.Contains(up, "at_start") || !strings.Contains(up, "moved") {
		t.Fatalf("history scroll expression must report movement and start boundary: %s", up)
	}
	down := buildAgentXianyuScrollToLatestExpression("")
	if !strings.Contains(down, "scroll.scrollHeight") || !strings.Contains(down, "at_end") {
		t.Fatalf("latest scroll expression must move to the end and report boundary: %s", down)
	}
	list := buildAgentXianyuConversationListScrollExpression("")
	if !strings.Contains(list, "client_height") || !strings.Contains(list, "at_end") || !strings.Contains(list, "before + step") || !strings.Contains(list, "Math.min(maxTop") {
		t.Fatalf("conversation list scroll expression must report boundary: %s", list)
	}
	reset := buildAgentXianyuConversationListResetExpression("")
	if !strings.Contains(reset, "scrollTop=0") || !strings.Contains(reset, "at_start") {
		t.Fatalf("conversation list reset expression must return to the start: %s", reset)
	}
}

func TestAgentXianyuConversationListRestoreKeepsMonotonicPosition(t *testing.T) {
	restore := buildAgentXianyuConversationListRestoreExpression("", 2400)
	if !strings.Contains(restore, "requestedTop=2400") || !strings.Contains(restore, "const targetTop=Math.min(Math.max(0,requestedTop),maxTop)") {
		t.Fatalf("restore expression must clamp to the requested list position: %s", restore)
	}
	if !strings.Contains(restore, "el.scrollTop=Math.max(before,targetTop)") {
		t.Fatalf("restore expression must never move the list backwards: %s", restore)
	}
	if strings.Contains(restore, "el.scrollTop=0;") {
		t.Fatalf("restore expression must not reset the list to page one: %s", restore)
	}
}

func TestAgentXianyuConversationListScrollReportsContainerState(t *testing.T) {
	for name, expression := range map[string]string{
		"scroll":  buildAgentXianyuConversationListScrollExpression(""),
		"reset":   buildAgentXianyuConversationListResetExpression(""),
		"restore": buildAgentXianyuConversationListRestoreExpression("", 1200),
	} {
		if !strings.Contains(expression, "container_found") {
			t.Fatalf("%s expression must report whether the list container was found: %s", name, expression)
		}
	}
}

func TestAgentXianyuHistoryFastPathConfiguration(t *testing.T) {
	if agentXianyuHistoryWait >= time.Second {
		t.Fatalf("history polling should be shorter than the per-conversation delay: %s", agentXianyuHistoryWait)
	}
	if agentXianyuHistoryQuietPeriod >= 2*time.Second {
		t.Fatalf("history quiet period is too slow: %s", agentXianyuHistoryQuietPeriod)
	}
	if agentXianyuHistoryMaxDuration <= agentXianyuHistoryQuietPeriod {
		t.Fatalf("history max duration must bound the quiet period")
	}
	if agentXianyuHistoryMaxDuration > 20*time.Second {
		t.Fatalf("history max duration must keep a stalled conversation bounded: %s", agentXianyuHistoryMaxDuration)
	}
}

func TestAgentXianyuCandidateKeyFallsBackToTitle(t *testing.T) {
	if got := agentXianyuCandidateKey(agentXianyuConversationCandidate{Key: " buyer-7 "}); got != "buyer-7" {
		t.Fatalf("expected trimmed candidate key, got %q", got)
	}
	if got := agentXianyuCandidateKey(agentXianyuConversationCandidate{Title: " Buyer title "}); got != "Buyer title" {
		t.Fatalf("expected title fallback, got %q", got)
	}
	if got := agentXianyuCandidateKey(agentXianyuConversationCandidate{}); got != "" {
		t.Fatalf("expected empty key for empty candidate, got %q", got)
	}
	if !agentXianyuCandidateBatchContains([]agentXianyuConversationCandidate{{Key: "buyer-7", Title: "Buyer"}}, agentXianyuConversationCandidate{Key: " buyer-7 "}) {
		t.Fatal("expected visible candidate batch to contain the target key")
	}
	if !agentXianyuCandidateBatchContains([]agentXianyuConversationCandidate{{Title: "Buyer"}}, agentXianyuConversationCandidate{Title: "Buyer"}) {
		t.Fatal("expected visible candidate batch to contain the target title")
	}
	if agentXianyuCandidateBatchContains([]agentXianyuConversationCandidate{{Key: "buyer-8", Title: "Other"}}, agentXianyuConversationCandidate{Key: "buyer-7", Title: "Buyer"}) {
		t.Fatal("did not expect an unrelated candidate to match")
	}
}

func TestBuildAgentXianyuCandidatesPreservesDocumentOrder(t *testing.T) {
	expression := buildAgentXianyuCandidatesExpression("", 20)
	if !strings.Contains(expression, "compareDocumentPosition") || !strings.Contains(expression, "r.bottom > 0") || !strings.Contains(expression, "r.top < innerHeight") {
		t.Fatalf("candidate extraction must preserve document order: %s", expression)
	}
	if strings.Contains(expression, "items.sort") {
		t.Fatalf("candidate extraction must not reorder conversations by heuristic score: %s", expression)
	}
	if strings.Contains(expression, "firstLine + '|'") || strings.Contains(expression, "text.slice(0, 180)") {
		t.Fatalf("conversation identity must not depend on mutable preview text: %s", expression)
	}
	click := buildAgentXianyuClickExpression("", agentXianyuConversationCandidate{Key: "buyer-1", Title: "Buyer"})
	if !strings.Contains(click, "querySelector('button,a,[role=\"button\"]')") || !strings.Contains(click, "mousedown") || !strings.Contains(click, "scrollIntoView") {
		t.Fatalf("conversation click expression must click the interactive descendant: %s", click)
	}
}

func TestMergeAndTruncateAgentXianyuTranscriptSnapshots(t *testing.T) {
	merged := mergeAgentXianyuTranscriptSnapshots([]string{
		"old message\nshared message tail",
		"shared message tail\nnew message",
		"new message",
	})
	if merged != "old message\nshared message tail\nnew message" {
		t.Fatalf("unexpected merged transcript: %q", merged)
	}
	const maxLength = 80
	long := "HEAD-END\n" + strings.Repeat("M", 120) + strings.Repeat("T", 40) + "\nTAIL-BEGIN"
	trimmed := truncateAgentXianyuTranscript(long, maxLength)
	if len([]rune(trimmed)) > maxLength || !strings.Contains(trimmed, "HEAD-END") || !strings.Contains(trimmed, "TAIL-BEGIN") {
		t.Fatalf("truncated transcript lost head/tail or exceeded limit: len=%d text=%q", len([]rune(trimmed)), trimmed)
	}
}

func TestSplitAgentXianyuTextPreservesCompleteHistory(t *testing.T) {
	input := "第一条消息\n第二条消息\n第三条消息\n第四条消息"
	chunks := splitAgentXianyuText(input, 8)
	if len(chunks) < 2 {
		t.Fatalf("expected the history to be split into multiple chunks: %+v", chunks)
	}
	if strings.Join(chunks, "\n") != input {
		t.Fatalf("splitting must preserve all history text: %+v", chunks)
	}
	if expression := buildAgentXianyuTranscriptExpression(""); strings.Contains(expression, "slice(0, 20000)") {
		t.Fatalf("transcript extraction must not truncate history before archival: %s", expression)
	}
}

func TestAgentXianyuTranscriptExpressionExtractsOnlyMessageText(t *testing.T) {
	expression := buildAgentXianyuTranscriptExpression("")
	for _, expected := range []string{
		"document.createTreeWalker",
		"NodeFilter.SHOW_TEXT",
		"data-message-id",
		"message_count",
		"excludedSelector",
		"container_found: false",
	} {
		if !strings.Contains(expression, expected) {
			t.Fatalf("transcript extraction must contain %q: %s", expected, expression)
		}
	}
	if strings.Contains(expression, "document.querySelectorAll('main,section,article,div')") {
		t.Fatalf("transcript extraction must not fall back to arbitrary page containers: %s", expression)
	}
	if strings.Contains(expression, "best.innerText || best.textContent") {
		t.Fatalf("transcript extraction must not send an unfiltered container text dump: %s", expression)
	}
}

func TestAgentXianyuProductContextExpressionStaysInCurrentProductPanel(t *testing.T) {
	expression := buildAgentXianyuProductContextExpression("")
	for _, expected := range []string{"[role=\"tab\"]", "当前宝贝", "aria-controls", "NodeFilter.SHOW_TEXT", "found:false", "已上架", "出售中", "有货", "privateLine", "publicLines", "本周购买", "成本价"} {
		if !strings.Contains(expression, expected) {
			t.Fatalf("product extraction must contain %q: %s", expected, expression)
		}
	}
	if strings.Contains(expression, "status+'\\n'+pageText") {
		t.Fatalf("product availability must not be inferred from description text: %s", expression)
	}
	if strings.Contains(expression, "document.body.innerText") {
		t.Fatalf("product extraction must not capture the whole page: %s", expression)
	}
}

func TestGroupAgentXianyuSummaryPartsBoundsMergeInput(t *testing.T) {
	groups := groupAgentXianyuSummaryParts([]string{
		strings.Repeat("甲", 12000),
		strings.Repeat("乙", 12000),
		strings.Repeat("丙", 12000),
	}, agentXianyuSummaryMergeRunes)
	if len(groups) != 2 || len(groups[0]) != 2 || len(groups[1]) != 1 {
		t.Fatalf("expected summaries to be reduced in bounded batches, got %#v", groups)
	}
	for index, group := range groups {
		total := 0
		for _, part := range group {
			total += len([]rune(part))
		}
		if total > agentXianyuSummaryMergeRunes {
			t.Fatalf("merge group %d exceeds input bound: %d", index, total)
		}
	}
}

func TestParseAgentLLMResponseSupportsContentParts(t *testing.T) {
	response := []byte(`{"choices":[{"message":{"content":[{"type":"text","text":"第一段"},{"type":"text","text":"第二段"}]}}]}`)
	got, err := parseAgentLLMResponse(response)
	if err != nil {
		t.Fatal(err)
	}
	if got != "第一段第二段" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestParseAgentLLMResponseRejectsReasoningOnly(t *testing.T) {
	response := []byte(`{"choices":[{"message":{"content":"","reasoning_content":"internal reasoning"}}]}`)
	if _, err := parseAgentLLMResponse(response); err == nil || !strings.Contains(err.Error(), "reasoning-only") {
		t.Fatalf("expected reasoning-only response to be rejected, got %v", err)
	}
}

func TestAgentXianyuSummaryRequestIsBounded(t *testing.T) {
	if agentXianyuSummaryRequestTimeout < 2*time.Minute || agentXianyuSummaryRequestTimeout >= 5*time.Minute {
		t.Fatalf("summary request timeout should leave headroom for normal responses without inheriting the five-minute call timeout: %s", agentXianyuSummaryRequestTimeout)
	}
}

func TestEffectiveAgentNodeTimeoutPreservesXianyuFullScan(t *testing.T) {
	xianyu := agentBlueprintGraphNode{}
	xianyu.Data.Kind = "xianyu"
	xianyu.Data.Settings = map[string]string{"节点超时": "10m"}
	if got := effectiveAgentNodeTimeout(xianyu); got != 0 {
		t.Fatalf("stale Xianyu timeout should not cancel a resumable full scan, got %s", got)
	}
	ordinary := agentBlueprintGraphNode{}
	ordinary.Data.Kind = "browser"
	ordinary.Data.Settings = map[string]string{"节点超时": "10m"}
	if got := effectiveAgentNodeTimeout(ordinary); got != 10*time.Minute {
		t.Fatalf("ordinary node timeout should remain unchanged, got %s", got)
	}
	unlimited := agentBlueprintGraphNode{}
	unlimited.Data.Kind = "xianyu"
	unlimited.Data.Settings = map[string]string{"节点超时": "不限制"}
	if got := effectiveAgentNodeTimeout(unlimited); got != 0 {
		t.Fatalf("unlimited Xianyu timeout should remain unlimited, got %s", got)
	}
}

func TestEffectiveAgentBlueprintRunTimeoutAllowsXianyuCompletion(t *testing.T) {
	xianyu := agentBlueprintGraphNode{ID: "xianyu"}
	xianyu.Data.Kind = "xianyu"
	raw, err := json.Marshal([]agentBlueprintGraphNode{xianyu})
	if err != nil {
		t.Fatal(err)
	}
	if got := effectiveAgentBlueprintRunTimeout(&AgentBlueprint{Nodes: raw}); got != 0 {
		t.Fatalf("Xianyu blueprint should not inherit the aggregate one-hour deadline, got %s", got)
	}
	ordinary := agentBlueprintGraphNode{ID: "browser"}
	ordinary.Data.Kind = "browser"
	raw, err = json.Marshal([]agentBlueprintGraphNode{ordinary})
	if err != nil {
		t.Fatal(err)
	}
	if got := effectiveAgentBlueprintRunTimeout(&AgentBlueprint{Nodes: raw}); got != agentBlueprintRunTimeout {
		t.Fatalf("ordinary blueprint timeout changed: %s", got)
	}
}

func TestParseAgentXianyuConversationLimitSupportsAll(t *testing.T) {
	for _, value := range []string{"", "全部", "不限", "all", "0"} {
		limit, all := parseAgentXianyuConversationLimit(value)
		if !all || limit != agentXianyuMaxConversations {
			t.Fatalf("%q should select all conversations, got limit=%d all=%v", value, limit, all)
		}
	}
	if limit, all := parseAgentXianyuConversationLimit("20"); all || limit != 20 {
		t.Fatalf("explicit numeric limit should be preserved, got limit=%d all=%v", limit, all)
	}
}

func TestParseAgentXianyuConversationLimitUsesConfiguredCeiling(t *testing.T) {
	if limit, all := parseAgentXianyuConversationLimitWithCeiling("全部", 1200); !all || limit != 1200 {
		t.Fatalf("configured all-conversation ceiling should be used, got limit=%d all=%v", limit, all)
	}
	if limit, all := parseAgentXianyuConversationLimitWithCeiling("50", 40); all || limit != 40 {
		t.Fatalf("numeric limits should still respect the configured ceiling, got limit=%d all=%v", limit, all)
	}
}

func TestAgentXianyuDefaultSummaryInstructionIsStructured(t *testing.T) {
	for _, heading := range []string{"商品/服务信息", "用户问题/场景", "用户意图与约束", "回复口吻与表达习惯", "推荐客服回复", "业务规则与例外", "需要确认或转人工", "禁止承诺"} {
		if !strings.Contains(agentXianyuDefaultSummaryInstruction, heading) {
			t.Fatalf("summary instruction is missing heading %q", heading)
		}
	}
	for _, rule := range []string{"只分析卖家已发送消息", "避免通用 AI 客服套话", "曝光", "成本", "不得在任何面向买家的示例中出现"} {
		if !strings.Contains(agentXianyuDefaultSummaryInstruction, rule) {
			t.Fatalf("summary instruction is missing privacy/style rule %q", rule)
		}
	}
}

func TestRedactAgentXianyuTextRemovesPrivateSellerData(t *testing.T) {
	value := strings.Join([]string{
		"支持自动充值服务",
		"曝光：1589",
		"浏览：28",
		"想要：4",
		"浏览有 28 人",
		"有 4 人想要",
		"本周购买：6次",
		"订单号：3312858145451060750",
		"联系方式：138-0013-8000",
	}, "\n")
	redacted := redactAgentXianyuText(value)
	if !strings.Contains(redacted, "支持自动充值服务") {
		t.Fatalf("public product information should remain: %q", redacted)
	}
	for _, private := range []string{"曝光", "1589", "浏览", "28", "想要", "本周购买", "3312858145451060750", "138-0013-8000"} {
		if strings.Contains(redacted, private) {
			t.Fatalf("redaction leaked %q: %q", private, redacted)
		}
	}
}

func TestShouldRetryAgentLLMStatus(t *testing.T) {
	for _, status := range []int{429, 500, 502, 503} {
		if !shouldRetryAgentLLMStatus(status) {
			t.Fatalf("status %d should be retried", status)
		}
	}
	for _, status := range []int{200, 400, 401, 404} {
		if shouldRetryAgentLLMStatus(status) {
			t.Fatalf("status %d should not be retried", status)
		}
	}
}

func TestAgentLLMRetryDelayUsesBoundedExponentialBackoff(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 1, want: 2 * time.Second},
		{attempt: 2, want: 4 * time.Second},
		{attempt: 3, want: 8 * time.Second},
		{attempt: 4, want: 16 * time.Second},
		{attempt: 5, want: 16 * time.Second},
	}
	for _, test := range tests {
		if got := agentLLMRetryDelay(test.attempt); got != test.want {
			t.Fatalf("attempt %d: expected %s, got %s", test.attempt, test.want, got)
		}
	}
	if agentLLMMaxAttempts < 5 {
		t.Fatalf("temporary model failures need more than the previous three rapid attempts")
	}
}

func TestGetInternalAPIKeysPrefersMatchingModelGroups(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	createAgentTestChannel(t, store, "codex-flow", "gpt-5.6-sol,gpt-5.6-luna")
	createAgentTestChannel(t, store, "codex-backup", "gpt-5.6-sol")
	createAgentTestChannel(t, store, "claude", "claude-sonnet")
	if _, err := store.CreateAPIKey(ctx, APIKeyPayload{Name: "flow", Group: "codex-flow", UnlimitedQuota: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateAPIKey(ctx, APIKeyPayload{Name: "backup", Group: "codex-backup", UnlimitedQuota: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateAPIKey(ctx, APIKeyPayload{Name: "claude", Group: "claude", UnlimitedQuota: true}); err != nil {
		t.Fatal(err)
	}

	keys, err := store.getInternalAPIKeys(ctx, "codex-flow", "gpt-5.6-sol")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0].Group != "codex-flow" || keys[1].Group != "codex-backup" {
		t.Fatalf("unexpected model routes: %+v", keys)
	}
	keys, err = store.getInternalAPIKeys(ctx, "codex-flow", "claude-sonnet")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].Group != "claude" {
		t.Fatalf("unexpected claude routes: %+v", keys)
	}
}
