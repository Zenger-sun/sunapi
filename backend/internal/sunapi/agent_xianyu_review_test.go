package sunapi

import (
	"strings"
	"testing"
)

func TestNormalizeAgentXianyuReviewURLForcesEvaluationManagement(t *testing.T) {
	got, err := normalizeAgentXianyuReviewURL("https://seller.goofish.com/?site=COMM#/messages")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "site=COMMONPRO") {
		t.Fatalf("normalized URL must use seller console site: %q", got)
	}
	if !strings.Contains(got, "#/seller-trade/evaluation-manage") {
		t.Fatalf("normalized URL must use evaluation management route: %q", got)
	}
	if strings.Contains(got, "#/messages") {
		t.Fatalf("normalized URL must not retain chat route: %q", got)
	}
}

func TestParseAgentXianyuReviewLimit(t *testing.T) {
	if limit, all := parseAgentXianyuReviewLimit("全部", 1200); !all || limit != 1200 {
		t.Fatalf("全部 should use the protection ceiling, got limit=%d all=%v", limit, all)
	}
	if limit, all := parseAgentXianyuReviewLimit("25", 1200); all || limit != 25 {
		t.Fatalf("explicit review count should be respected, got limit=%d all=%v", limit, all)
	}
	if limit, all := parseAgentXianyuReviewLimit("5000", 1200); all || limit != 1200 {
		t.Fatalf("review count should be capped by the protection ceiling, got limit=%d all=%v", limit, all)
	}
}

func TestNormalizeAgentXianyuReviewTextMigratesPreviousDefault(t *testing.T) {
	for _, value := range []string{"", "沟通顺畅，交易愉快。", "沟通顺畅，交易愉快"} {
		if got := normalizeAgentXianyuReviewText(value); got != agentXianyuReviewDefaultText {
			t.Fatalf("legacy review text %q migrated to %q", value, got)
		}
	}
	if got := normalizeAgentXianyuReviewText("自定义评价"); got != agentXianyuReviewDefaultText {
		t.Fatalf("review text must remain fixed, got %q", got)
	}
}

func TestXianyuReviewExpressionsUseEvaluationManagementFlow(t *testing.T) {
	pending := buildAgentXianyuReviewPendingTabExpression()
	if !strings.Contains(pending, "待卖家评价") {
		t.Fatalf("pending tab expression must target seller-pending reviews: %s", pending)
	}

	openNext := buildAgentXianyuReviewOpenNextExpression("", "", nil, nil)
	for _, marker := range []string{"订单编号", "去评价", "立即评价"} {
		if !strings.Contains(openNext, marker) {
			t.Fatalf("order review expression is missing %q: %s", marker, openNext)
		}
	}
	for _, marker := range []string{"scan_all_conversations", "会话数量", "对话列表选择器"} {
		if strings.Contains(openNext, marker) {
			t.Fatalf("review expression must not contain chat-list logic %q", marker)
		}
	}

	submit := buildAgentXianyuReviewSubmitExpression("", "", "", "", agentXianyuReviewDefaultText)
	if !strings.Contains(submit, agentXianyuReviewDefaultText) || !strings.Contains(submit, "好评") {
		t.Fatalf("submit expression must use the fixed review and positive rating: %s", submit)
	}
}

func TestXianyuReviewComponentDefaults(t *testing.T) {
	for _, component := range customAgentComponents {
		if component.ID != "component.xianyu-review" {
			continue
		}
		if !strings.Contains(string(component.Config), agentXianyuReviewDefaultText) {
			t.Fatalf("component must expose the fixed review text: %s", component.Config)
		}
		if !strings.Contains(string(component.Config), "待卖家评价") || strings.Contains(string(component.Config), "会话数量") {
			t.Fatalf("component must use evaluation management settings only: %s", component.Config)
		}
		return
	}
	t.Fatal("xianyu review component is missing from custom catalog")
}
