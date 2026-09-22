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
	if agentXianyuReviewDefaultText != "沟通顺畅，交易愉快" {
		t.Fatalf("default review text changed unexpectedly: %q", agentXianyuReviewDefaultText)
	}
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
	for _, marker := range []string{"input[type=\"radio\"]", "input.checked", "pending_seller_review_tab_active", "value"} {
		if !strings.Contains(pending, marker) {
			t.Fatalf("pending tab expression must verify the real radio state %q: %s", marker, pending)
		}
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
	for _, marker := range []string{"review_submit_scheduled", "setTimeout(publish,120)", "const publish=()=>button.click()", "popup", "contenteditable", "评价完成"} {
		if !strings.Contains(submit+buildAgentXianyuReviewConfirmationExpression("", "order-12345678"), marker) {
			t.Fatalf("review flow is missing robust marker %q", marker)
		}
	}
	if strings.Contains(submit, "pointerdown") || strings.Contains(submit, "mousedown") {
		t.Fatalf("review submit must issue a single click instead of a synthetic mouse sequence: %s", submit)
	}
	close := buildAgentXianyuReviewCloseExpression()
	if !strings.Contains(close, "Escape") || !strings.Contains(close, "关闭") || !strings.Contains(close, "我知道了") {
		t.Fatalf("review close expression must close stale dialogs: %s", close)
	}
}

func TestXianyuReviewBatchExpressionSelectsCurrentPageAndPaginatesAfterConfirmation(t *testing.T) {
	page := buildAgentXianyuReviewSelectPageExpression("", "", nil)
	for _, marker := range []string{"thead input[type=\"checkbox\"]", "tr[data-row-key]", "pending_review_select_all_not_found", "page_selected_waiting_batch_button", "current_page_has_no_pending_reviews", "pending_seller_review_tab_not_active", "pendingInput.checked", "order_keys"} {
		if !strings.Contains(page, marker) {
			t.Fatalf("batch page expression is missing %q: %s", marker, page)
		}
	}
	for _, marker := range []string{"买家未评价", "去评价", "pending_review_checkbox_not_found", "previousElementSibling"} {
		if strings.Contains(page, marker) {
			t.Fatalf("batch page expression must use the header select-all without per-row filtering %q: %s", marker, page)
		}
	}
	batch := buildAgentXianyuReviewClickBatchExpression()
	for _, marker := range []string{"批量评价", "button.click()", "batch_review_button_not_found"} {
		if !strings.Contains(batch, marker) {
			t.Fatalf("batch button expression is missing %q: %s", marker, batch)
		}
	}
	if strings.Contains(batch, "pointerdown") || strings.Contains(batch, "mousedown") {
		t.Fatalf("batch action must issue a single click instead of a synthetic mouse sequence: %s", batch)
	}
	confirmation := buildAgentXianyuReviewBatchConfirmationExpression("", []string{"order-12345678"})
	for _, marker := range []string{"batch_review_action_removed", "review_orders_still_pending", "成功评价\\s*0\\s*个订单", "评价成功", "评价失败的订单", "batch_review_failure_detail_opening", "stale_batch_failure_detail_closed", "skipped_order_keys", "24\\s*小时内只能追评一次", "闲鱼批量评价失败"} {
		if !strings.Contains(confirmation, marker) {
			t.Fatalf("batch confirmation expression is missing %q: %s", marker, confirmation)
		}
	}
	next := buildAgentXianyuReviewNextPageExpression()
	if !strings.Contains(next, "下一页") || !strings.Contains(next, "last_page") {
		t.Fatalf("pagination expression must handle the final page: %s", next)
	}
}

func TestXianyuReviewWaitAndRetryDefaults(t *testing.T) {
	if agentXianyuReviewDefaultWait < 2 {
		t.Fatalf("evaluation form wait should allow the seller console to render: %d", agentXianyuReviewDefaultWait)
	}
	if agentXianyuReviewMaxAttempts < 2 {
		t.Fatalf("evaluation submission should retry transient popup failures: %d", agentXianyuReviewMaxAttempts)
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
