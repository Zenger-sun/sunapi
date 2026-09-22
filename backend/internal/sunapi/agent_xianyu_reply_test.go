package sunapi

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
		if config.Settings["消息轮询间隔"] != "2s" || config.Settings["监听范围"] != "仅未读消息" || config.Settings["单次未清楚补答"] != "最多1次" || config.Settings["发送方式"] != "检测到买家消息立即自动发送" || config.Settings["默认回复"] != "开启" || config.Settings["无知识库命中"] != "安全澄清回复" || config.Settings["节点超时"] != "不限制" {
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
	if !strings.Contains(unreadCandidates, "const unreadOnly = true") || !strings.Contains(unreadCandidates, "if (unreadOnly && !unread) return") {
		t.Fatalf("candidate expression must filter non-unread rows: %s", unreadCandidates)
	}
	latest := buildAgentXianyuReplyLatestMessageExpression(".message-list")
	for _, required := range []string{"data-message-id", "direction", "messages.slice(-12)", "container_found"} {
		if !strings.Contains(latest, required) {
			t.Fatalf("latest-message expression is missing %q", required)
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
	for _, status := range []string{"sent", "sent_fallback", "skipped_self", "model_error"} {
		if shouldResumeAgentXianyuReply(status) {
			t.Fatalf("status %q must not be replayed", status)
		}
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
