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
		if config.Settings["消息轮询间隔"] != "5s" || config.Settings["发送方式"] != "直接自动发送" || config.Settings["默认回复"] != "开启" || config.Settings["无知识库命中"] != "安全澄清回复" || config.Settings["节点超时"] != "不限制" {
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
		agentXianyuProductContext{Found: true, Title: "移动话费充值 100 元", Status: "在线"},
	)
	for _, expected := range []string{"移动话费充值 100 元", "在线", "现在还有货吗"} {
		if !strings.Contains(query, expected) {
			t.Fatalf("knowledge query is missing %q: %s", expected, query)
		}
	}
	if strings.Count(query, "现在还有货吗") != 1 {
		t.Fatalf("knowledge query should de-duplicate the list preview: %s", query)
	}
}

func TestBuildAgentXianyuFallbackReplyAlwaysReturnsSafeText(t *testing.T) {
	available := buildAgentXianyuFallbackReply("老板，现在还有货吗", agentXianyuProductContext{Found: true, Title: "话费充值", Text: "话费充值\n在线"}, 300)
	if available == "" || !strings.Contains(available, "规格") {
		t.Fatalf("stock fallback should ask for the product specification: %q", available)
	}
	unavailable := buildAgentXianyuFallbackReply("还有货吗", agentXianyuProductContext{Found: true, Status: "已删除", Text: "已删除\n话费充值"}, 300)
	if !strings.Contains(unavailable, "不可拍") {
		t.Fatalf("unavailable product fallback should explain the page status: %q", unavailable)
	}
	generic := buildAgentXianyuFallbackReply("你好", agentXianyuProductContext{}, 12)
	if generic == "" || len([]rune(generic)) > 12 {
		t.Fatalf("generic fallback must be non-empty and bounded: %q", generic)
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
			"conversation": {PreviewHash: "preview", LastIncomingHash: "message", Status: "sent", UpdatedAt: 123},
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
	if got.Conversations["conversation"].LastIncomingHash != "message" || got.UpdatedAt != 123 {
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
