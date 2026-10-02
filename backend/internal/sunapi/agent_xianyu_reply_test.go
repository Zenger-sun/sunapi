package sunapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestXianyuReplyReconnectBackoffIsCapped(t *testing.T) {
	tests := []struct {
		failures int
		want     time.Duration
	}{
		{failures: 1, want: 5 * time.Second},
		{failures: 2, want: 10 * time.Second},
		{failures: 3, want: 20 * time.Second},
		{failures: 4, want: 40 * time.Second},
		{failures: 5, want: time.Minute},
		{failures: 20, want: time.Minute},
	}
	for _, test := range tests {
		if got := agentXianyuReplyReconnectDelayAfter(test.failures); got != test.want {
			t.Errorf("reconnect delay after %d failures = %s, want %s", test.failures, got, test.want)
		}
	}
}

func TestXianyuReplyComponentDefaultsToRealtimeDirectSend(t *testing.T) {
	for _, component := range customAgentComponents {
		if component.ID != "component.xianyu-reply" {
			continue
		}
		if component.Kind != "xianyu_reply" || component.Group != "process" || component.Source != "custom" {
			t.Fatalf("unexpected component routing: %#v", component)
		}
		var config componentConfigPayload
		if err := json.Unmarshal(component.Config, &config); err != nil {
			t.Fatal(err)
		}
		if config.Settings["消息轮询间隔"] != "2s" || config.Settings["消息合并等待"] != "10s" || config.Settings["监听范围"] != "未读列表 + 当前已打开会话" || config.Settings["单次未清楚补答"] != "最多1次" || config.Settings["发送方式"] != "检测到买家消息静默后自动发送" || config.Settings["默认回复"] != "开启" || config.Settings["无知识库命中"] != "安全澄清回复" || config.Settings["节点超时"] != "不限制" {
			t.Fatalf("unexpected realtime defaults: %#v", config.Settings)
		}
		return
	}
	t.Fatal("xianyu realtime reply component is missing")
}

func TestParseAgentBlueprintNodesNormalizesXianyuReplyComponent(t *testing.T) {
	raw := json.RawMessage(`[{"id":"reply","data":{"kind":"custom","componentId":"component.xianyu-reply"}}]`)
	nodes, err := parseAgentBlueprintNodes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].Data.Kind != "xianyu_reply" {
		t.Fatalf("unexpected nodes: %#v", nodes)
	}
	if got := effectiveAgentNodeTimeout(nodes[0]); got != 0 {
		t.Fatalf("realtime listener must not inherit a fixed node timeout: %s", got)
	}
}

func TestXianyuReplyExpressionsStayMessageScoped(t *testing.T) {
	keepalive := buildAgentXianyuConnectionKeepaliveExpression()
	for _, required := range []string{"reconnected", "reconnecting", "connection", "\\u8fde\\u63a5\\u4e2d\\u65ad", "\\u91cd\\u8fde", "reconnect", "setInterval", "__sunapiXianyuReconnectWatchdog", "window.location.reload"} {
		if !strings.Contains(keepalive, required) {
			t.Fatalf("connection keepalive expression is missing %q: %s", required, keepalive)
		}
	}
	observer := buildAgentXianyuReplyObserverInstallExpression(".conversation-list")
	if !strings.Contains(observer, "MutationObserver") || !strings.Contains(observer, "__sunapiXianyuDirty") {
		t.Fatalf("observer expression does not install a page change listener: %s", observer)
	}
	status := buildAgentXianyuReplyObserverStatusExpression()
	for _, required := range []string{"unread_count", "unread_signature", "data-unread-count", "__sunapiXianyuDirty"} {
		if !strings.Contains(status, required) {
			t.Fatalf("observer status expression is missing unread-only signal %q: %s", required, status)
		}
	}
	unreadCandidates := buildAgentXianyuCandidatesExpressionWithUnread(".conversation-list", 20, true)
	for _, required := range []string{"const unreadOnly = true", "if (unreadOnly && !unread) return", "const scope = custom", "const selectors = ['[data-conversation-id]'", "[class*=\"Item\"]", "isUnreadBadge", "::after", "systemTitle"} {
		if !strings.Contains(unreadCandidates, required) {
			t.Fatalf("candidate expression is missing unread robustness marker %q: %s", required, unreadCandidates)
		}
	}
	if !strings.Contains(status, "systemTitle") || !strings.Contains(status, "const seen = new Set()") {
		t.Fatalf("observer status must exclude notices and deduplicate rows: %s", status)
	}
	latest := buildAgentXianyuReplyLatestMessageExpression(".message-list")
	for _, required := range []string{"data-message-id", "direction", "messages.slice(-50)", "container_found", "isSystemMessage", "systemText"} {
		if !strings.Contains(latest, required) {
			t.Fatalf("latest-message expression is missing %q", required)
		}
	}
	current := buildAgentXianyuCurrentConversationExpression(".conversation-list")
	for _, required := range []string{"activeSignal", "aria-current", "aria-selected", "data-conversation-id", "JSON.stringify({key:"} {
		if !strings.Contains(current, required) {
			t.Fatalf("current-conversation expression is missing %q", required)
		}
	}
	product := buildAgentXianyuProductContextExpression(".current-product")
	for _, required := range []string{"当前宝贝", "aria-controls", "title", "status", "text"} {
		if !strings.Contains(product, required) {
			t.Fatalf("product-context expression is missing %q", required)
		}
	}
	send := buildAgentXianyuReplySendExpression(".composer", ".send", "测试回复")
	for _, required := range []string{"composer_not_empty", "InputEvent", "button.click", "测试回复"} {
		if !strings.Contains(send, required) {
			t.Fatalf("send expression is missing %q", required)
		}
	}
}

func TestAgentXianyuReplySkipsSystemMessages(t *testing.T) {
	systemText := "\u6b64\u6d88\u606f\u4e0d\u652f\u6301\u5728PC\u7aef\u6d4f\u89c8\uff0c\u8bf7\u5230APP\u4e0a\u67e5\u770b"
	if !isAgentXianyuSystemMessage(systemText) {
		t.Fatal("PC-only system notice should be recognized")
	}
	if isAgentXianyuSystemMessage("\u5728\u7684\uff0c\u60a8\u5177\u4f53\u60f3\u95ee\u54ea\u65b9\u9762\uff1f") {
		t.Fatal("ordinary buyer message must not be classified as a system notice")
	}
	messages := []agentXianyuReplyMessage{
		{Key: "system", Text: systemText, Direction: "customer"},
		{Key: "buyer", Text: "\u5728\u7684\uff0c\u60a8\u5177\u4f53\u60f3\u95ee\u54ea\u65b9\u9762\uff1f", Direction: "customer"},
	}
	got := agentXianyuReplyUnansweredCustomerMessages(messages, true)
	if len(got) != 1 || got[0].Key != "buyer" {
		t.Fatalf("system notice should be skipped while preserving buyer messages: %#v", got)
	}
}

func TestXianyuReplyTargetReservationsPreventDuplicateWorkers(t *testing.T) {
	endpoint := "http://127.0.0.1:12345"
	target := agentBrowserTarget{ID: "target-1"}
	if !reserveAgentXianyuReplyTarget(endpoint, target) {
		t.Fatal("first target reservation should succeed")
	}
	if reserveAgentXianyuReplyTarget(endpoint, target) {
		t.Fatal("the same target must not be reserved twice")
	}
	releaseAgentXianyuReplyTarget(endpoint, target)
	if !reserveAgentXianyuReplyTarget(endpoint, target) {
		t.Fatal("released target should be reusable")
	}
	releaseAgentXianyuReplyTarget(endpoint, target)
}

func TestGenericBrowserTargetSkipsReplyReservation(t *testing.T) {
	endpoint := "http://127.0.0.1:12346"
	target := agentBrowserTarget{ID: "reply-page", Type: "page", WebSocketDebuggerURL: "ws://reply"}
	if !reserveAgentXianyuReplyTarget(endpoint, target) {
		t.Fatal("reply target reservation should succeed")
	}
	defer releaseAgentXianyuReplyTarget(endpoint, target)
	if !isAgentXianyuReplyTargetReserved(endpoint, target) {
		t.Fatal("generic browser nodes must observe the reply target reservation")
	}
}

func TestXianyuReplyDoesNotCreateTabWhenExistingTargetProbeFails(t *testing.T) {
	newCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json/list":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":"existing","type":"page","url":"https://seller.goofish.com/?site=COMMONPRO&spm=im","webSocketDebuggerUrl":"ws://127.0.0.1:1/devtools/page/existing"}]`))
		case "/json/new":
			newCalls++
			http.Error(w, "unexpected tab creation", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	target, reused, err := openAgentXianyuReplyTarget(context.Background(), server.URL, "https://seller.goofish.com/?site=COMMONPRO")
	if err == nil {
		t.Fatal("expected a transient probe error")
	}
	if reused || target.ID != "" {
		t.Fatalf("failed probe must not return a target: %#v, reused=%v", target, reused)
	}
	if newCalls != 0 {
		t.Fatalf("probe failure must not create another tab, got %d /json/new calls", newCalls)
	}
	// The failed probe must release its reservation so the same target can be
	// claimed after the browser's websocket becomes available again.
	listed, listErr := listAgentBrowserTargets(context.Background(), server.URL)
	if listErr != nil || len(listed) != 1 || !reserveAgentXianyuReplyTarget(server.URL, listed[0]) {
		t.Fatalf("failed probe left a stale reservation: targets=%#v err=%v", listed, listErr)
	}
	releaseAgentXianyuReplyTarget(server.URL, listed[0])
}

func TestXianyuReplySessionLockSerializesSharedProfile(t *testing.T) {
	dataDir := t.TempDir()
	settings := map[string]string{"会话名称": "shared-session"}
	release, err := acquireAgentXianyuReplySession(context.Background(), dataDir, settings)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if _, err := acquireAgentXianyuReplySession(ctx, dataDir, settings); err == nil {
		t.Fatal("expected a second reply session to wait for the shared profile")
	}

	release()
	nextRelease, err := acquireAgentXianyuReplySession(context.Background(), dataDir, settings)
	if err != nil {
		t.Fatal(err)
	}
	nextRelease()
}

func TestAgentXianyuReplyUsesAllUnansweredBuyerMessages(t *testing.T) {
	messages := []agentXianyuReplyMessage{
		{Key: "1", Text: "之前的问题", Direction: "customer"},
		{Key: "2", Text: "卖家已回复", Direction: "self"},
		{Key: "3", Text: "现在多少钱", Direction: "customer"},
		{Key: "4", Text: "还在吗", Direction: "customer"},
	}
	got := agentXianyuReplyUnansweredCustomerMessages(messages, false)
	if len(got) != 2 || got[0].Text != "现在多少钱" || got[1].Text != "还在吗" {
		t.Fatalf("unexpected pending buyer batch: %#v", got)
	}
	if got := agentXianyuReplyUnansweredCustomerMessages(messages[:2], false); len(got) != 0 {
		t.Fatalf("seller reply should close the previous buyer batch: %#v", got)
	}
}

func TestAgentXianyuReplyKeepsUnknownRowsAfterSellerBoundary(t *testing.T) {
	messages := []agentXianyuReplyMessage{
		{Key: "1", Text: "seller", Direction: "self"},
		{Key: "2", Text: "first buyer message", Direction: "unknown"},
		{Key: "3", Text: "second buyer message", Direction: "unknown"},
	}
	got := agentXianyuReplyUnansweredCustomerMessages(messages, true)
	if len(got) != 2 || got[0].Key != "2" || got[1].Key != "3" {
		t.Fatalf("unknown buyer burst should preserve all rows after seller boundary: %#v", got)
	}
	if got[0].Direction != "customer" || got[1].Direction != "customer" {
		t.Fatalf("unknown buyer rows should be normalized: %#v", got)
	}
}

func TestAgentXianyuReplyAggregatesUnknownTailWithoutSellerBoundary(t *testing.T) {
	messages := []agentXianyuReplyMessage{
		{Key: "old", Text: "historical row", Direction: "unknown"},
		{Key: "known", Text: "buyer context", Direction: "customer"},
		{Key: "new-1", Text: "first new row", Direction: "unknown"},
		{Key: "new-2", Text: "second new row", Direction: "unknown"},
	}
	got := agentXianyuReplyUnansweredCustomerMessages(messages, true)
	if len(got) != 3 || got[0].Key != "known" || got[1].Key != "new-1" || got[2].Key != "new-2" {
		t.Fatalf("unknown tail should be aggregated without including earlier unknown history: %#v", got)
	}
}

func TestAgentXianyuReplyBatchHashChangesWhenBuyerAddsMessage(t *testing.T) {
	one := []agentXianyuReplyMessage{{Key: "1", Text: "多少钱", Direction: "customer"}}
	two := append(one, agentXianyuReplyMessage{Key: "2", Text: "还在吗", Direction: "customer"})
	if agentXianyuReplyBatchHash("conversation", one) == agentXianyuReplyBatchHash("conversation", two) {
		t.Fatal("buyer message batch hash must change when a new message arrives")
	}
}

func TestAgentXianyuReplyDetectsHumanSellerMessage(t *testing.T) {
	baseline := agentXianyuReplyMessagePayload{Messages: []agentXianyuReplyMessage{
		{Key: "buyer-1", Text: "还有吗", Direction: "customer"},
	}}
	if agentXianyuReplyHumanMessageAppeared(baseline, baseline) {
		t.Fatal("unchanged conversation must not look like a human takeover")
	}
	human := agentXianyuReplyMessagePayload{Messages: append(append([]agentXianyuReplyMessage{}, baseline.Messages...), agentXianyuReplyMessage{
		Key: "seller-1", Text: "在的，可以拍", Direction: "self",
	})}
	if !agentXianyuReplyHumanMessageAppeared(baseline, human) {
		t.Fatal("new seller message must interrupt the auto reply")
	}
	buyerFollowUp := agentXianyuReplyMessagePayload{Messages: append(append([]agentXianyuReplyMessage{}, baseline.Messages...), agentXianyuReplyMessage{
		Key: "buyer-2", Text: "那我拍了", Direction: "customer",
	})}
	if agentXianyuReplyHumanMessageAppeared(baseline, buyerFollowUp) {
		t.Fatal("a buyer follow-up alone must not be classified as a human takeover")
	}
	duplicateHuman := agentXianyuReplyMessagePayload{Messages: append(append([]agentXianyuReplyMessage{}, baseline.Messages...),
		agentXianyuReplyMessage{Text: "same seller reply", Direction: "self"},
		agentXianyuReplyMessage{Text: "same seller reply", Direction: "self"},
	)}
	previousDuplicate := agentXianyuReplyMessagePayload{Messages: duplicateHuman.Messages[:len(duplicateHuman.Messages)-1]}
	if !agentXianyuReplyHumanMessageAppeared(previousDuplicate, duplicateHuman) {
		t.Fatal("a repeated seller reply without message ids must interrupt the auto reply")
	}
}

func TestAgentXianyuReplyProcessesNewOrChangedConversationPreview(t *testing.T) {
	baseline := agentXianyuReplyConversationState{PreviewHash: "same", Status: "baseline"}
	if !shouldProcessAgentXianyuCandidate(agentXianyuConversationCandidate{Unread: true}, baseline, true, "same", true) {
		t.Fatal("unread candidate must reach the worker even when its preview hash is unchanged")
	}
	if !shouldProcessAgentXianyuCandidate(agentXianyuConversationCandidate{}, baseline, true, "changed", true) {
		t.Fatal("changed preview should process a follow-up even when the unread badge is gone")
	}
	if shouldProcessAgentXianyuCandidate(agentXianyuConversationCandidate{}, agentXianyuReplyConversationState{}, false, "new", true) {
		t.Fatal("new read conversation should not be opened during inbox scanning")
	}
	if !shouldProcessAgentXianyuCandidate(agentXianyuConversationCandidate{Unread: true}, agentXianyuReplyConversationState{}, false, "new", true) {
		t.Fatal("new unread conversation should be processed")
	}
	if shouldProcessAgentXianyuCandidate(agentXianyuConversationCandidate{Unread: true}, agentXianyuReplyConversationState{}, false, "new", false) {
		t.Fatal("disabled default reply must not process unread conversations")
	}
	if shouldProcessAgentXianyuCandidate(agentXianyuConversationCandidate{}, agentXianyuReplyConversationState{}, true, "changed", false) {
		t.Fatal("disabled default reply must not process changed conversations")
	}
}

func TestNormalizeAgentXianyuReplyHonorsDeclineAndLength(t *testing.T) {
	if got := normalizeAgentXianyuReply("```\nSUNAPI_NO_REPLY\n```", 100); got != "" {
		t.Fatalf("declined reply should be empty, got %q", got)
	}
	if got := normalizeAgentXianyuReply("这是可以直接发送的回复", 4); got != "这是可以" {
		t.Fatalf("reply should be rune bounded, got %q", got)
	}
}

func TestBuildAgentXianyuReplyKnowledgeQueryIncludesProduct(t *testing.T) {
	query := buildAgentXianyuReplyKnowledgeQuery(
		"现在还有货吗",
		agentXianyuConversationCandidate{Preview: "现在还有货吗"},
		agentXianyuProductContext{Found: true, Title: "移动话费充值 100 元", Status: "在线", Description: strings.Repeat("很长的商品介绍", 300)},
	)
	for _, expected := range []string{"移动话费充值 100 元", "现在还有货吗"} {
		if !strings.Contains(query, expected) {
			t.Fatalf("knowledge query is missing %q: %s", expected, query)
		}
	}
	if strings.Contains(query, "在线") {
		t.Fatalf("generic product status must not create cross-product knowledge matches: %s", query)
	}
	if strings.Count(query, "现在还有货吗") != 1 {
		t.Fatalf("knowledge query should de-duplicate the list preview: %s", query)
	}
	if !strings.HasPrefix(query, "现在还有货吗") || strings.Contains(query, "很长的商品介绍") || len([]rune(query)) > 1200 {
		t.Fatalf("knowledge query should prioritize buyer intent and exclude verbose product description: %s", query)
	}
}

func TestFilterAgentXianyuReplyKnowledgeMatchesRejectsWeakOverlap(t *testing.T) {
	matches := []KnowledgeSearchMatch{
		{DocumentID: "weak", Filename: "闲鱼客服知识-other.md", Content: "其他商品当前有货"},
		{DocumentID: "buyer", Filename: "faq", Content: "买家问现在还有货吗时，应先确认规格"},
		{DocumentID: "product", Filename: "闲鱼客服知识-current.md", Content: "移动话费充值 100 元支持自动到账"},
	}
	filtered := filterAgentXianyuReplyKnowledgeMatches(matches, "现在还有货吗", "移动话费充值 100 元")
	if len(filtered) != 2 || filtered[0].DocumentID != "buyer" || filtered[1].DocumentID != "product" {
		t.Fatalf("weak single-term overlap should not count as a knowledge hit: %#v", filtered)
	}

	short := filterAgentXianyuReplyKnowledgeMatches([]KnowledgeSearchMatch{{DocumentID: "refund", Content: "退款规则"}}, "退款", "")
	if len(short) != 1 {
		t.Fatalf("an exact two-rune buyer question should remain searchable: %#v", short)
	}
}

func TestFilterAgentXianyuReplyKnowledgeMatchesScopesProductDocuments(t *testing.T) {
	matches := []KnowledgeSearchMatch{
		{DocumentID: "other-product", Filename: agentXianyuProductFilenamePrefix + "other.md", Content: "其他充值商品 当前有货"},
		{DocumentID: "current-product", Filename: agentXianyuProductFilenamePrefix + "current.md", Content: "移动话费充值 100 元 当前有货"},
	}
	filtered := filterAgentXianyuReplyKnowledgeMatches(matches, "现在还有货吗", "移动话费充值 100 元")
	if len(filtered) != 1 || filtered[0].DocumentID != "current-product" {
		t.Fatalf("product knowledge must be scoped to the current product: %#v", filtered)
	}
}

func TestSplitAgentXianyuReplyKnowledgeMatchesDoesNotTreatStyleAsFact(t *testing.T) {
	facts, styles := splitAgentXianyuReplyKnowledgeMatches([]KnowledgeSearchMatch{
		{DocumentID: "style", Filename: agentXianyuStyleDocumentFilename, Content: "绠€鐭嚜鐒剁殑鍥炲"},
		{DocumentID: "product", Filename: "闂查奔商品知识-current.md", Content: "鍦ㄥ敭"},
	})
	if len(facts) != 1 || facts[0].DocumentID != "product" {
		t.Fatalf("style document must not count as a product fact: %#v", facts)
	}
	if len(styles) != 1 || styles[0].DocumentID != "style" {
		t.Fatalf("style document should remain available as context: %#v", styles)
	}
}

func TestBuildAgentXianyuReplyKnowledgeContextUsesFilteredMatches(t *testing.T) {
	context := buildAgentXianyuReplyKnowledgeContext([]KnowledgeSearchMatch{{
		KnowledgeBaseName: "客服规则",
		Filename:          "faq",
		ChunkIndex:        1,
		Content:           "充值失败时请先核对订单状态\n曝光：1589\n浏览：28\n想要：4",
	}})
	for _, expected := range []string{"Knowledge-base answer policy", "知识片段 2", "充值失败时请先核对订单状态"} {
		if !strings.Contains(context, expected) {
			t.Fatalf("filtered knowledge context is missing %q: %s", expected, context)
		}
	}
	for _, private := range []string{"客服规则", "faq", "曝光", "1589", "浏览：28", "想要：4"} {
		if strings.Contains(context, private) {
			t.Fatalf("filtered knowledge context leaked private metadata %q: %s", private, context)
		}
	}
}

func TestBuildAgentXianyuFallbackReplyAlwaysReturnsSafeText(t *testing.T) {
	for _, question := range []string{"老板，现在还有货吗", "这个还有没有", "现在能买吗", "可以下单吗"} {
		available := buildAgentXianyuFallbackReply(question, agentXianyuProductContext{Found: true, Title: "话费充值", Status: "已上架", Description: "无货时支持退款"}, 300)
		if available == "" || !strings.Contains(available, "可以拍的") || strings.Contains(available, "有货，可以") {
			t.Fatalf("stock fallback should safely answer %q: %q", question, available)
		}
	}
	unavailable := buildAgentXianyuFallbackReply("还有货吗", agentXianyuProductContext{Found: true, Status: "已删除", Text: "已删除\n话费充值"}, 300)
	if !strings.Contains(unavailable, "不可拍") {
		t.Fatalf("unavailable product fallback should explain the page status: %q", unavailable)
	}
	greeting := buildAgentXianyuFallbackReply("老板，在吗", agentXianyuProductContext{Found: true, Title: "话费充值", Availability: "available", Description: "自动充值服务\n曝光：1589"}, 300)
	if strings.Contains(greeting, "直接下单") || strings.Contains(greeting, "自动充值服务") || strings.Contains(greeting, "曝光") || greeting != "在的，您具体想问哪方面？" {
		t.Fatalf("a greeting should get a short natural reply without copying product data: %q", greeting)
	}
	generic := buildAgentXianyuFallbackReply("你好", agentXianyuProductContext{}, 12)
	if generic == "" || len([]rune(generic)) > 12 {
		t.Fatalf("generic fallback must be non-empty and bounded: %q", generic)
	}
}

func TestAgentXianyuReplyContainsPrivateData(t *testing.T) {
	for _, value := range []string{
		"这个商品曝光：1589，浏览：28",
		"这个商品浏览有 28 人",
		"目前有 4 人想要",
		"本周购买 6 次，本周消费 36.28 元",
		"订单号：3312858145451060750",
		"手机号 138-0013-8000",
		"根据知识库和浏览器会话显示可以拍",
	} {
		if !agentXianyuReplyContainsPrivateData(value) {
			t.Fatalf("private reply should be blocked: %q", value)
		}
	}
	for _, value := range []string{"可以拍的，直接下单就行。", "月卡 6.98 元，年卡 123.77 元。", "您想要哪个规格？"} {
		if agentXianyuReplyContainsPrivateData(value) {
			t.Fatalf("public reply should remain sendable: %q", value)
		}
	}
}

func TestShouldResumeAgentXianyuReplyMigratesSilentTerminalStates(t *testing.T) {
	for _, status := range []string{"model_declined", "no_knowledge_match"} {
		if !shouldResumeAgentXianyuReply(status) {
			t.Fatalf("legacy silent status %q must be retried when default reply is enabled", status)
		}
	}
	for _, status := range []string{"sent", "sent_fallback", "skipped_self", "model_error", "interrupted_by_human", "send_unverified_exhausted"} {
		if shouldResumeAgentXianyuReply(status) {
			t.Fatalf("status %q must not be replayed", status)
		}
	}
	for _, status := range []string{"send_error", "send_input_not_found", "send_button_disabled"} {
		if !shouldResumeAgentXianyuReply(status) {
			t.Fatalf("transient send status %q must be replayed after its retry delay", status)
		}
	}
}

func TestScheduleAgentXianyuReplyUnverifiedRetryIsBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state := agentXianyuReplyState{Conversations: map[string]agentXianyuReplyConversationState{}}
	var mu sync.Mutex
	entry := agentXianyuReplyConversationState{}
	for attempt := 1; attempt <= agentXianyuReplyUnverifiedMaxAttempts; attempt++ {
		if err := scheduleAgentXianyuReplyUnverifiedRetry(&mu, path, &state, "conversation", entry, "preview", "incoming"); err != nil {
			t.Fatal(err)
		}
		entry = state.Conversations["conversation"]
		if entry.UnverifiedAttempts != attempt {
			t.Fatalf("attempt %d was not persisted: %#v", attempt, entry)
		}
	}
	if entry.Status != "send_unverified_exhausted" {
		t.Fatalf("expected exhausted status after bounded retries, got %q", entry.Status)
	}
	if entry.LastIncomingHash != "" {
		t.Fatalf("unverified send must not mark incoming hash handled: %#v", entry)
	}
	if err := scheduleAgentXianyuReplyUnverifiedRetry(&mu, path, &state, "conversation", entry, "preview", "new-incoming"); err != nil {
		t.Fatal(err)
	}
	if got := state.Conversations["conversation"].UnverifiedAttempts; got != 1 {
		t.Fatalf("new incoming batch should reset verification attempts, got %d", got)
	}
}

func TestXianyuReplyStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	want := agentXianyuReplyState{
		Conversations: map[string]agentXianyuReplyConversationState{
			"conversation": {PreviewHash: "preview", LastIncomingHash: "message", LastReplyHash: "reply", FollowUpUsed: true, Status: "sent_with_follow_up", UpdatedAt: 123},
		},
		UpdatedAt: 123,
	}
	if err := saveAgentXianyuReplyState(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadAgentXianyuReplyState(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Conversations["conversation"].LastIncomingHash != "message" || !got.Conversations["conversation"].FollowUpUsed || got.UpdatedAt != 123 {
		t.Fatalf("unexpected state: %#v", got)
	}
}

func TestCancelAgentRunsStopsLongListener(t *testing.T) {
	store := &Store{agentActiveRuns: map[string]map[string]context.CancelFunc{}}
	ctx, cancel := context.WithCancel(context.Background())
	store.registerAgentRun("blueprint", "run", cancel)
	store.cancelAgentRuns("blueprint")
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("active listener was not canceled")
	}
	store.unregisterAgentRun("blueprint", "run")
	if store.isAgentRunActive("blueprint") {
		t.Fatal("listener remained registered after completion")
	}
}

func TestStopAllAgentRunsWaitsBeforeShutdown(t *testing.T) {
	store := &Store{agentActiveRuns: map[string]map[string]context.CancelFunc{}}
	release := make(chan struct{})
	canceled := make(chan struct{})
	store.registerAgentRun("blueprint", "run", func() { close(canceled) })
	go func() {
		<-canceled
		<-release
		store.unregisterAgentRun("blueprint", "run")
	}()
	done := make(chan struct{})
	go func() {
		store.stopAllAgentRuns()
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("store shutdown returned before active run cleanup")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("store shutdown did not wait for active run cleanup")
	}
}
