package sunapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	agentXianyuReviewStateDir         = "agent-xianyu-review-state"
	agentXianyuReviewDefaultURL       = "https://seller.goofish.com/?site=COMMONPRO#/seller-trade/evaluation-manage"
	agentXianyuReviewRoute            = "/seller-trade/evaluation-manage"
	agentXianyuReviewDefaultText      = "沟通顺畅，交易愉快"
	agentXianyuReviewDefaultWait      = 3
	agentXianyuReviewMaxWait          = 30
	agentXianyuReviewDefaultMaxOrders = 2000
	agentXianyuReviewMaxOrders        = 5000
	agentXianyuReviewConfirmAttempts  = 6
	agentXianyuReviewMaxAttempts      = 2
)

type agentXianyuReviewState struct {
	Reviewed  map[string]int64 `json:"reviewed"`
	UpdatedAt int64            `json:"updated_at"`
}

type agentXianyuReviewStats struct {
	Scanned   int `json:"scanned"`
	Eligible  int `json:"eligible"`
	Submitted int `json:"submitted"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`
}

type agentXianyuReviewPendingTabPayload struct {
	Found   bool   `json:"found"`
	Clicked bool   `json:"clicked"`
	Active  bool   `json:"active"`
	Value   string `json:"value"`
	Reason  string `json:"reason"`
}

type agentXianyuReviewPagePayload struct {
	Found             bool     `json:"found"`
	Selected          int      `json:"selected"`
	Pending           int      `json:"pending"`
	OrderKeys         []string `json:"order_keys"`
	ReviewedOrderKeys []string `json:"reviewed_order_keys"`
	Signature         string   `json:"signature"`
	BatchClicked      bool     `json:"batch_clicked"`
	Reason            string   `json:"reason"`
}

type agentXianyuReviewPageSignaturePayload struct {
	Signature string `json:"signature"`
}

type agentXianyuReviewBatchConfirmationPayload struct {
	Confirmed        bool     `json:"confirmed"`
	Failed           bool     `json:"failed"`
	SkippedOrderKeys []string `json:"skipped_order_keys"`
	Reason           string   `json:"reason"`
}

type agentXianyuReviewBatchButtonPayload struct {
	Clicked bool   `json:"clicked"`
	Reason  string `json:"reason"`
}

type agentXianyuReviewCandidatePayload struct {
	Found             bool     `json:"found"`
	OrderKey          string   `json:"order_key"`
	VisibleCount      int      `json:"visible_count"`
	Signature         string   `json:"signature"`
	ReviewedOrderKeys []string `json:"reviewed_order_keys"`
	Reason            string   `json:"reason"`
}

type agentXianyuReviewActionPayload struct {
	Submitted bool   `json:"submitted"`
	Reason    string `json:"reason"`
}

type agentXianyuReviewConfirmationPayload struct {
	Confirmed bool   `json:"confirmed"`
	Failed    bool   `json:"failed"`
	Reason    string `json:"reason"`
}

type agentXianyuReviewNextPagePayload struct {
	Clicked bool   `json:"clicked"`
	AtEnd   bool   `json:"at_end"`
	Reason  string `json:"reason"`
}

func executeAgentXianyuReviewGraphNode(ctx context.Context, store *Store, bp *AgentBlueprint, node agentBlueprintGraphNode, inputVars map[string]interface{}, result agentBlueprintNodeResult) agentBlueprintNodeResult {
	if store == nil || strings.TrimSpace(store.DataDir()) == "" {
		return failAgentXianyuNode(result, "闲鱼自动评价无法执行，数据目录未配置。", "data directory is not configured")
	}
	if bp == nil {
		return failAgentXianyuNode(result, "闲鱼自动评价缺少当前智能体上下文。", "agent blueprint is required")
	}

	settings := node.Data.Settings
	reviewText := normalizeAgentXianyuReviewText(settings["评价内容"])
	statePath := agentXianyuReviewStatePath(store.DataDir(), bp.ID, node.ID)
	state, err := loadAgentXianyuReviewState(statePath)
	if err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价状态读取失败。", err.Error())
	}
	result.Input = map[string]interface{}{
		"source":      "闲鱼评价管理",
		"mode":        "pending_seller_reviews",
		"review_text": reviewText,
	}
	reportAgentExecutionProgress(ctx, map[string]interface{}{
		"phase":      "collecting",
		"node_id":    node.ID,
		"node_kind":  node.Data.Kind,
		"node_title": node.Data.Title,
		"message":    "正在打开闲鱼评价管理并读取待卖家评价订单…",
	})

	targetURL, err := normalizeAgentXianyuReviewURL(firstNonEmpty(settings["网址"], settings["URL"], agentXianyuReviewDefaultURL))
	if err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价网址无效。", err.Error())
	}
	session := firstNonEmpty(settings["会话名称"], agentXianyuDefaultSession)
	paths := resolveAgentBrowserSessionPaths(store.DataDir(), session)
	endpoint, err := ensureAgentBrowserDevtoolsForSession(ctx, paths)
	if err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价无法连接浏览器。", err.Error())
	}
	target, reusedTarget, err := openAgentBrowserTarget(ctx, endpoint, targetURL)
	if err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价无法打开评价管理。", err.Error())
	}
	defer makeAgentBrowserTargetCleanup(endpoint, target, !reusedTarget)()
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, target.WebSocketDebuggerURL, nil)
	if err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价无法连接浏览器调试通道。", err.Error())
	}
	defer conn.Close()
	if _, err = agentBrowserCDPCall(ctx, conn, 1, "Page.enable", nil); err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价启用页面通道失败。", err.Error())
	}
	if _, err = agentBrowserCDPCall(ctx, conn, 2, "Runtime.enable", nil); err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价启用脚本通道失败。", err.Error())
	}
	// A reusable seller-console target may currently show the chat page. Always
	// navigate it to evaluation management before looking for pending reviews.
	if _, err = agentBrowserCDPCall(ctx, conn, 3, "Page.navigate", map[string]any{"url": targetURL}); err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价无法进入评价管理。", err.Error())
	}
	loginWait := parseAgentPositiveSetting(firstNonEmpty(settings["登录等待"], "180s"), agentXianyuDefaultLoginWait, 300)
	if loginWait > 0 && !strings.Contains(settings["登录态"], "无需登录") {
		if err = waitAgentBrowserLogin(ctx, conn, loginWait); err != nil {
			return failAgentXianyuNode(result, "闲鱼自动评价登录等待失败。", err.Error())
		}
	}
	if err = waitAgentXianyu(ctx, settings["列表等待"], agentXianyuDefaultListWait); err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价等待评价管理页面失败。", err.Error())
	}

	callID := 1000
	nextID := func() int { callID++; return callID }
	pendingTab, err := ensureAgentXianyuReviewPendingTab(ctx, conn, nextID)
	if err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价无法读取评价管理页签。", err.Error())
	}
	if !pendingTab.Found || !pendingTab.Active {
		return failAgentXianyuNode(result, "闲鱼自动评价未找到“待卖家评价”页签。", firstNonEmpty(pendingTab.Reason, "pending_seller_review_tab_not_found"))
	}

	listSelector := settings["订单列表选择器"]
	actionSelector := settings["评价按钮选择器"]
	dialogSelector := settings["评价弹窗选择器"]
	inputSelector := settings["评价输入框选择器"]
	ratingSelector := settings["好评选项选择器"]
	submitSelector := settings["评价提交按钮选择器"]
	ceiling := parseAgentPositiveSetting(settings["最大评价保护上限"], agentXianyuReviewDefaultMaxOrders, agentXianyuReviewMaxOrders)
	limit, _ := parseAgentXianyuReviewLimit(settings["评价数量"], ceiling)
	reviewWait := parseAgentPositiveSetting(settings["单次评价等待"], agentXianyuReviewDefaultWait, agentXianyuReviewMaxWait)

	stats := agentXianyuReviewStats{}
	scannedOrders := map[string]struct{}{}
	lastEmptyPageSignature := ""
	hasLastEmptyPage := false
	emptyPageRepeats := 0
	report := func(message string) {
		reportAgentExecutionProgress(ctx, map[string]interface{}{
			"phase":      "reviewing",
			"node_id":    node.ID,
			"node_kind":  node.Data.Kind,
			"node_title": node.Data.Title,
			"message":    message,
			"scanned":    stats.Scanned,
			"eligible":   stats.Eligible,
			"submitted":  stats.Submitted,
			"skipped":    stats.Skipped,
			"failed":     stats.Failed,
		})
	}

	for stats.Scanned < limit {
		if err := ctx.Err(); err != nil {
			return failAgentXianyuReviewWithStats(result, "闲鱼自动评价已中断。", err.Error(), stats)
		}
		// The seller console keeps both "all" and "pending seller review" on
		// this route. Verify the actual radio state immediately before every
		// batch so pagination or a stale React render can never select all orders.
		pendingTab, tabErr := ensureAgentXianyuReviewPendingTab(ctx, conn, nextID)
		if tabErr != nil {
			return failAgentXianyuReviewWithStats(result, "闲鱼自动评价无法确认待卖家评价页签。", tabErr.Error(), stats)
		}
		if !pendingTab.Found || !pendingTab.Active {
			return failAgentXianyuReviewWithStats(result, "闲鱼自动评价未进入待卖家评价页签。", firstNonEmpty(pendingTab.Reason, "pending_seller_review_tab_not_active"), stats)
		}
		page, inspectErr := evaluateAgentXianyuReviewSelectPage(ctx, conn, nextID(), listSelector, actionSelector, state.Reviewed)
		if inspectErr != nil {
			return failAgentXianyuReviewWithStats(result, "闲鱼自动评价读取待评价订单失败。", inspectErr.Error(), stats)
		}
		if page.Pending == 0 || !page.Found {
			// A processed page can keep the same order signature after its
			// pending actions disappear. It still needs one pagination click;
			// only stop when the same empty page is observed twice in a row.
			if hasLastEmptyPage && page.Signature == lastEmptyPageSignature {
				emptyPageRepeats++
				if emptyPageRepeats >= 3 {
					break
				}
			} else {
				emptyPageRepeats = 0
			}
			lastEmptyPageSignature = page.Signature
			hasLastEmptyPage = true
			previousSignature := page.Signature
			nextPage, pageErr := evaluateAgentXianyuReviewNextPage(ctx, conn, nextID())
			if pageErr != nil {
				return failAgentXianyuReviewWithStats(result, "闲鱼自动评价翻页失败。", pageErr.Error(), stats)
			}
			if !nextPage.Clicked || nextPage.AtEnd {
				break
			}
			pageChanged := false
			for attempt := 0; attempt < 12; attempt++ {
				if err := waitAgentXianyuDuration(ctx, agentXianyuDOMWait); err != nil {
					return failAgentXianyuReviewWithStats(result, "闲鱼自动评价等待下一页失败。", err.Error(), stats)
				}
				probe, probeErr := evaluateAgentXianyuReviewPageSignature(ctx, conn, nextID(), listSelector)
				if probeErr != nil {
					return failAgentXianyuReviewWithStats(result, "闲鱼自动评价读取下一页失败。", probeErr.Error(), stats)
				}
				if probe.Signature != previousSignature {
					pageChanged = true
					break
				}
			}
			if !pageChanged {
				return failAgentXianyuReviewWithStats(result, "闲鱼自动评价等待下一页超时。", "review_next_page_content_not_changed", stats)
			}
			continue
		}
		if page.Selected == 0 {
			return failAgentXianyuReviewWithStats(result, "闲鱼自动评价未能选中当前页待评价商品或打开批量评价。", firstNonEmpty(page.Reason, "batch_review_selection_failed"), stats)
		}
		if !page.BatchClicked {
			batchButton, buttonErr := clickAgentXianyuReviewBatchWithRetry(ctx, conn, nextID)
			if buttonErr != nil || !batchButton.Clicked {
				reason := batchButton.Reason
				if buttonErr != nil {
					reason = buttonErr.Error()
				}
				return failAgentXianyuReviewWithStats(result, "闲鱼自动评价未能打开批量评价。", firstNonEmpty(reason, "batch_review_button_not_found"), stats)
			}
		}

		batchKeys := make([]string, 0, len(page.OrderKeys))
		for _, orderKey := range page.OrderKeys {
			orderKey = strings.TrimSpace(orderKey)
			if orderKey == "" {
				continue
			}
			if _, alreadyScanned := scannedOrders[orderKey]; !alreadyScanned {
				scannedOrders[orderKey] = struct{}{}
				batchKeys = append(batchKeys, orderKey)
			}
		}
		if len(batchKeys) == 0 {
			return failAgentXianyuReviewWithStats(result, "闲鱼自动评价未能确认当前页订单编号。", "review_order_keys_not_found", stats)
		}
		stats.Scanned += len(batchKeys)
		stats.Eligible += len(batchKeys)
		lastEmptyPageSignature = ""
		hasLastEmptyPage = false
		emptyPageRepeats = 0
		report(fmt.Sprintf("当前页已全选 %d 个待评价商品，正在提交好评。", len(batchKeys)))

		batchSucceeded := false
		var batchSkippedKeys []string
		lastReason := "batch_review_not_confirmed"
		publishAttempted := false
		for attempt := 1; attempt <= agentXianyuReviewMaxAttempts; attempt++ {
			// Once the publish button has been clicked, never open another batch
			// dialog while waiting for the table to refresh. The seller console can
			// take several seconds to remove the reviewed rows; reopening the dialog
			// during that window creates duplicate input boxes and may submit twice.
			if publishAttempted {
				break
			}
			if attempt > 1 {
				if err := waitAgentXianyuDuration(ctx, agentXianyuDOMWait); err != nil {
					return failAgentXianyuReviewWithStats(result, "闲鱼自动评价等待重试失败。", err.Error(), stats)
				}
				retryPage, retryErr := evaluateAgentXianyuReviewSelectPage(ctx, conn, nextID(), listSelector, actionSelector, state.Reviewed)
				if retryErr != nil || retryPage.Selected == 0 {
					if retryErr != nil {
						lastReason = retryErr.Error()
					} else {
						lastReason = firstNonEmpty(retryPage.Reason, "batch_review_retry_selection_failed")
					}
					continue
				}
				if !retryPage.BatchClicked {
					batchButton, buttonErr := clickAgentXianyuReviewBatchWithRetry(ctx, conn, nextID)
					if buttonErr != nil || !batchButton.Clicked {
						if buttonErr != nil {
							lastReason = buttonErr.Error()
						} else {
							lastReason = firstNonEmpty(batchButton.Reason, "batch_review_retry_button_not_found")
						}
						continue
					}
				}
			}
			submit, submitErr := submitAgentXianyuReviewWithRetry(ctx, conn, nextID, dialogSelector, inputSelector, ratingSelector, submitSelector, reviewText, reviewWait)
			if submitErr != nil || !submit.Submitted {
				lastReason = submit.Reason
				if submitErr != nil {
					lastReason = submitErr.Error()
				}
				_ = closeAgentXianyuReviewDialog(ctx, conn, nextID())
				continue
			}
			publishAttempted = true
			confirmation, confirmErr := confirmAgentXianyuReviewBatch(ctx, conn, nextID, inputSelector, batchKeys, reviewWait)
			if confirmErr != nil {
				lastReason = confirmErr.Error()
			} else if len(confirmation.SkippedOrderKeys) == len(batchKeys) && len(batchKeys) > 0 {
				batchSkippedKeys = append(batchSkippedKeys, confirmation.SkippedOrderKeys...)
				lastReason = firstNonEmpty(confirmation.Reason, "batch_review_orders_already_processed")
				break
			} else if confirmation.Failed || !confirmation.Confirmed {
				lastReason = firstNonEmpty(confirmation.Reason, "batch_review_confirmation_timeout")
			} else {
				batchSucceeded = true
				break
			}
			_ = closeAgentXianyuReviewDialog(ctx, conn, nextID())
		}
		if len(batchSkippedKeys) > 0 {
			_ = closeAgentXianyuReviewDialog(ctx, conn, nextID())
			now := time.Now().UnixMilli()
			for _, orderKey := range batchSkippedKeys {
				state.Reviewed[orderKey] = now
			}
			state.UpdatedAt = now
			stats.Skipped += len(batchSkippedKeys)
			if err := saveAgentXianyuReviewState(statePath, state); err != nil {
				return failAgentXianyuReviewWithStats(result, "闲鱼评价跳过已处理订单，但本地防重复状态保存失败。", err.Error(), stats)
			}
			report(fmt.Sprintf("当前页 %d 个订单已存在首评，已跳过并继续下一页。", len(batchSkippedKeys)))
			if err := waitAgentXianyuDOM(ctx); err != nil {
				return failAgentXianyuReviewWithStats(result, "闲鱼自动评价等待继续扫描失败。", err.Error(), stats)
			}
			continue
		}
		if !batchSucceeded {
			stats.Failed += len(batchKeys)
			report(fmt.Sprintf("当前页 %d 个商品评价失败：%s。", len(batchKeys), firstNonEmpty(lastReason, "batch_review_confirmation_timeout")))
			return failAgentXianyuReviewWithStats(result, "闲鱼自动评价未确认当前页批量评价成功。", lastReason, stats)
		}

		now := time.Now().UnixMilli()
		for _, orderKey := range batchKeys {
			state.Reviewed[orderKey] = now
		}
		state.UpdatedAt = now
		stats.Submitted += len(batchKeys)
		if err := saveAgentXianyuReviewState(statePath, state); err != nil {
			return failAgentXianyuReviewWithStats(result, "闲鱼评价已提交，但本地防重复状态保存失败。", err.Error(), stats)
		}
		report(fmt.Sprintf("当前页 %d 个商品已完成好评，准备继续扫描。", len(batchKeys)))
		if err := waitAgentXianyuDOM(ctx); err != nil {
			return failAgentXianyuReviewWithStats(result, "闲鱼自动评价等待列表刷新失败。", err.Error(), stats)
		}
	}

	result.Message = fmt.Sprintf("闲鱼自动评价完成，从评价管理扫描 %d 个待卖家评价订单，成功评价 %d 个，跳过 %d 个，失败 %d 个。", stats.Scanned, stats.Submitted, stats.Skipped, stats.Failed)
	if stats.Failed > 0 {
		result.Status = "failed"
		result.Error = fmt.Sprintf("仍有 %d 个订单未确认评价成功", stats.Failed)
	}
	result.Output = map[string]interface{}{
		"source":                 "闲鱼评价管理",
		"scanned_order_count":    stats.Scanned,
		"eligible_order_count":   stats.Eligible,
		"submitted_review_count": stats.Submitted,
		"skipped_count":          stats.Skipped,
		"failed_count":           stats.Failed,
		"review_text":            reviewText,
		"review_rating":          "好评",
	}
	return result
}

func normalizeAgentXianyuReviewURL(raw string) (string, error) {
	value, err := normalizeAgentXianyuURL(firstNonEmpty(raw, agentXianyuReviewDefaultURL))
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("site", "COMMONPRO")
	parsed.RawQuery = query.Encode()
	parsed.Fragment = agentXianyuReviewRoute
	return parsed.String(), nil
}

func parseAgentXianyuReviewLimit(value string, ceiling int) (limit int, all bool) {
	if ceiling <= 0 {
		ceiling = agentXianyuReviewDefaultMaxOrders
	}
	raw := strings.ToLower(strings.TrimSpace(value))
	switch raw {
	case "", "0", "all", "unlimited", "全部", "不限", "所有", "全部评价", "所有评价":
		return ceiling, true
	default:
		return parseAgentPositiveSetting(raw, 20, ceiling), false
	}
}

func normalizeAgentXianyuReviewText(_ string) string {
	return agentXianyuReviewDefaultText
}

func evaluateAgentXianyuReviewPendingTab(ctx context.Context, conn *websocket.Conn, id int) (agentXianyuReviewPendingTabPayload, error) {
	return evaluateAgentXianyuJSON(ctx, conn, id, buildAgentXianyuReviewPendingTabExpression(), agentXianyuReviewPendingTabPayload{})
}

func ensureAgentXianyuReviewPendingTab(ctx context.Context, conn *websocket.Conn, nextID func() int) (agentXianyuReviewPendingTabPayload, error) {
	var last agentXianyuReviewPendingTabPayload
	clicked := false
	for attempt := 0; attempt < agentXianyuReviewConfirmAttempts; attempt++ {
		payload, err := evaluateAgentXianyuReviewPendingTab(ctx, conn, nextID())
		if err != nil {
			return last, err
		}
		last = payload
		clicked = clicked || payload.Clicked
		if payload.Found && payload.Active {
			if clicked {
				// Checking the radio is synchronous, but its filtered table is loaded
				// asynchronously. Let the old "all orders" rows leave the DOM before
				// the caller is allowed to use the header select-all checkbox.
				if err := waitAgentXianyuDuration(ctx, 2*agentXianyuDOMWait); err != nil {
					return last, err
				}
			}
			return payload, nil
		}
		if !payload.Found {
			return payload, nil
		}
		if err := waitAgentXianyuDOM(ctx); err != nil {
			return last, err
		}
	}
	return last, nil
}

func evaluateAgentXianyuReviewSelectPage(ctx context.Context, conn *websocket.Conn, id int, listSelector, actionSelector string, reviewed map[string]int64) (agentXianyuReviewPagePayload, error) {
	expression := buildAgentXianyuReviewSelectPageExpression(listSelector, actionSelector, agentXianyuReviewMapKeys(reviewed))
	return evaluateAgentXianyuJSON(ctx, conn, id, expression, agentXianyuReviewPagePayload{})
}

func evaluateAgentXianyuReviewPageSignature(ctx context.Context, conn *websocket.Conn, id int, listSelector string) (agentXianyuReviewPageSignaturePayload, error) {
	return evaluateAgentXianyuJSON(ctx, conn, id, buildAgentXianyuReviewPageSignatureExpression(listSelector), agentXianyuReviewPageSignaturePayload{})
}

func evaluateAgentXianyuReviewClickBatch(ctx context.Context, conn *websocket.Conn, id int) (agentXianyuReviewBatchButtonPayload, error) {
	return evaluateAgentXianyuJSON(ctx, conn, id, buildAgentXianyuReviewClickBatchExpression(), agentXianyuReviewBatchButtonPayload{})
}

func clickAgentXianyuReviewBatchWithRetry(ctx context.Context, conn *websocket.Conn, nextID func() int) (agentXianyuReviewBatchButtonPayload, error) {
	var last agentXianyuReviewBatchButtonPayload
	for attempt := 0; attempt < agentXianyuReviewMaxAttempts; attempt++ {
		// Selecting the header checkbox updates the table through React. Give
		// that state update a moment to settle before querying the batch button;
		// otherwise the button may still be rendered for the previous selection
		// and the workflow advances to the next page without opening the dialog.
		if err := waitAgentXianyuDuration(ctx, agentXianyuDOMWait); err != nil {
			return last, err
		}
		payload, err := evaluateAgentXianyuReviewClickBatch(ctx, conn, nextID())
		if err != nil {
			return last, err
		}
		last = payload
		if payload.Clicked {
			return payload, nil
		}
	}
	return last, nil
}

func confirmAgentXianyuReviewBatch(ctx context.Context, conn *websocket.Conn, nextID func() int, inputSelector string, orderKeys []string, waitSeconds int) (agentXianyuReviewBatchConfirmationPayload, error) {
	// Poll immediately after publish. The seller console displays batch failure
	// toasts briefly, so waiting for the configured per-review delay can hide the
	// actual business error behind the still-open editor.
	waitDuration := agentXianyuDOMWait
	if waitSeconds > 0 && waitSeconds < 1 {
		waitDuration = time.Duration(waitSeconds) * time.Second
	}
	var last agentXianyuReviewBatchConfirmationPayload
	for attempt := 0; attempt < agentXianyuReviewConfirmAttempts; attempt++ {
		if err := waitAgentXianyuDuration(ctx, waitDuration); err != nil {
			return last, err
		}
		payload, err := evaluateAgentXianyuJSON(ctx, conn, nextID(), buildAgentXianyuReviewBatchConfirmationExpression(inputSelector, orderKeys), agentXianyuReviewBatchConfirmationPayload{})
		if err != nil {
			return last, err
		}
		last = payload
		if payload.Confirmed || payload.Failed {
			return payload, nil
		}
		// After the first probe, poll quickly so transient seller-console error
		// toasts such as "成功评价 0 个订单" are captured before disappearing.
		waitDuration = agentXianyuDOMWait
	}
	return last, nil
}

func evaluateAgentXianyuReviewOpenNext(ctx context.Context, conn *websocket.Conn, id int, listSelector, actionSelector string, reviewed map[string]int64, attempted map[string]struct{}) (agentXianyuReviewCandidatePayload, error) {
	expression := buildAgentXianyuReviewOpenNextExpression(listSelector, actionSelector, agentXianyuReviewMapKeys(reviewed), agentXianyuReviewSetKeys(attempted))
	return evaluateAgentXianyuJSON(ctx, conn, id, expression, agentXianyuReviewCandidatePayload{})
}

func evaluateAgentXianyuReviewSubmit(ctx context.Context, conn *websocket.Conn, id int, dialogSelector, inputSelector, ratingSelector, submitSelector, text string) (agentXianyuReviewActionPayload, error) {
	expression := buildAgentXianyuReviewSubmitExpression(dialogSelector, inputSelector, ratingSelector, submitSelector, text)
	return evaluateAgentXianyuJSON(ctx, conn, id, expression, agentXianyuReviewActionPayload{})
}

func submitAgentXianyuReviewWithRetry(ctx context.Context, conn *websocket.Conn, nextID func() int, dialogSelector, inputSelector, ratingSelector, submitSelector, text string, waitSeconds int) (agentXianyuReviewActionPayload, error) {
	var last agentXianyuReviewActionPayload
	if waitSeconds <= 0 {
		waitSeconds = agentXianyuReviewDefaultWait
	}
	if waitSeconds > agentXianyuReviewMaxWait {
		waitSeconds = agentXianyuReviewMaxWait
	}
	// The seller console renders the evaluation drawer asynchronously. Wait
	// before querying it so a visible drawer is not mistaken for a missing form.
	if err := waitAgentXianyuDuration(ctx, time.Duration(waitSeconds)*time.Second); err != nil {
		return last, err
	}
	for attempt := 0; attempt < agentXianyuReviewMaxAttempts; attempt++ {
		payload, err := evaluateAgentXianyuReviewSubmit(ctx, conn, nextID(), dialogSelector, inputSelector, ratingSelector, submitSelector, text)
		if err != nil {
			return last, err
		}
		last = payload
		if payload.Submitted {
			return payload, nil
		}
		if attempt+1 < agentXianyuReviewMaxAttempts {
			if err := waitAgentXianyuDuration(ctx, agentXianyuDOMWait); err != nil {
				return last, err
			}
		}
	}
	return last, nil
}

func evaluateAgentXianyuReviewConfirmation(ctx context.Context, conn *websocket.Conn, id int, inputSelector, orderKey string) (agentXianyuReviewConfirmationPayload, error) {
	expression := buildAgentXianyuReviewConfirmationExpression(inputSelector, orderKey)
	return evaluateAgentXianyuJSON(ctx, conn, id, expression, agentXianyuReviewConfirmationPayload{})
}

func closeAgentXianyuReviewDialog(ctx context.Context, conn *websocket.Conn, id int) error {
	_, err := evaluateAgentXianyuJSON(ctx, conn, id, buildAgentXianyuReviewCloseExpression(), struct {
		Closed bool `json:"closed"`
	}{})
	return err
}

func confirmAgentXianyuReviewSubmission(ctx context.Context, conn *websocket.Conn, nextID func() int, inputSelector, orderKey string, waitSeconds int) (agentXianyuReviewConfirmationPayload, error) {
	waitDuration := agentXianyuDOMWait
	if waitSeconds > 0 {
		waitDuration = time.Duration(waitSeconds) * time.Second
	}
	var last agentXianyuReviewConfirmationPayload
	for attempt := 0; attempt < agentXianyuReviewConfirmAttempts; attempt++ {
		if err := waitAgentXianyuDuration(ctx, waitDuration); err != nil {
			return last, err
		}
		payload, err := evaluateAgentXianyuReviewConfirmation(ctx, conn, nextID(), inputSelector, orderKey)
		if err != nil {
			return last, err
		}
		last = payload
		if payload.Confirmed || payload.Failed {
			return payload, nil
		}
		waitDuration = agentXianyuDOMWait
	}
	return last, nil
}

func evaluateAgentXianyuReviewNextPage(ctx context.Context, conn *websocket.Conn, id int) (agentXianyuReviewNextPagePayload, error) {
	return evaluateAgentXianyuJSON(ctx, conn, id, buildAgentXianyuReviewNextPageExpression(), agentXianyuReviewNextPagePayload{})
}

func buildAgentXianyuReviewPendingTabExpression() string {
	return `(() => {
  const clean = value => String(value || '').replace(/\s+/g, ' ').trim();
  const displayed = element => { if (!element) return false; const rect = element.getBoundingClientRect(); const style = getComputedStyle(element); return rect.width > 0 && rect.height > 0 && style.display !== 'none' && style.visibility !== 'hidden' && style.opacity !== '0'; };
  const labels = [...document.querySelectorAll('label,[role="tab"],button,a,[role="button"],div,span')].filter(element => displayed(element) && /^待卖家评价\s*\d*$/.test(clean(element.innerText || element.textContent)));
  if (!labels.length) return JSON.stringify({found:false,clicked:false,active:false,value:'',reason:'pending_seller_review_tab_not_found'});
  let target = labels.find(element => element.matches('label') && element.querySelector('input[type="radio"]')) || null;
  if (!target) {
    const label = labels[0];
    target = label.closest('label,[role="tab"],button,a,[role="button"]') || label;
  }
  const input = target.querySelector('input[type="radio"]') || (target.matches('input[type="radio"]') ? target : null);
  const value = input ? String(input.value || '') : '';
  const active = !!(input && input.checked) || target.getAttribute('aria-selected') === 'true' || /(^|[-_\s])(checked|active|selected|is-active|is-selected)([-_\s]|$)/i.test(String(target.className || ''));
  target.scrollIntoView({block:'center',inline:'nearest'});
  if (!active) target.click();
  return JSON.stringify({found:true,clicked:!active,active,value,reason:active?'pending_seller_review_tab_active':'pending_seller_review_tab_clicked'});
})()`
}

func buildAgentXianyuReviewPageSignatureExpression(listSelector string) string {
	listJSON := quoteAgentXianyuJS(listSelector)
	return fmt.Sprintf(`(() => {
  const customList=%s;
  const clean=value=>String(value||'').replace(/\s+/g,' ').trim();
  const displayed=element=>{if(!element)return false;const rect=element.getBoundingClientRect(),style=getComputedStyle(element);return rect.width>0&&rect.height>0&&style.display!=='none'&&style.visibility!=='hidden'&&style.opacity!=='0';};
  let scope=document;
  if(customList){try{scope=document.querySelector(customList)||document;}catch(_){scope=document;}}
  const keys=[...scope.querySelectorAll('tr[data-row-key]')].filter(displayed).map(row=>row.getAttribute('data-row-key')).filter(Boolean);
  const fallback=keys.length?keys:([...scope.querySelectorAll('tr,[role="row"]')].filter(displayed).map(row=>clean(row.innerText||row.textContent)).filter(Boolean));
  return JSON.stringify({signature:fallback.join('|')});
})()`, listJSON)
}

func buildAgentXianyuReviewClickBatchExpression() string {
	return `(() => {
  const clean=value=>String(value||'').replace(/\s+/g,' ').trim();
  const displayed=element=>{if(!element)return false;const rect=element.getBoundingClientRect(),style=getComputedStyle(element);return rect.width>0&&rect.height>0&&style.display!=='none'&&style.visibility!=='hidden'&&style.opacity!=='0';};
  const disabled=element=>!element||element.disabled||element.getAttribute('aria-disabled')==='true'||/(^|[-_\s])(disabled|is-disabled)([-_\s]|$)/i.test(String(element.className||''));
  const exact=/^(批量评价|批量评估|批量操作|评价选中)$/;
  const textOf=element=>clean(element.innerText||element.textContent||element.getAttribute('aria-label')||element.getAttribute('title'));
  let labels=[...document.querySelectorAll('*')].filter(element=>displayed(element)&&exact.test(textOf(element)));
  if(!labels.length)labels=[...document.querySelectorAll('*')].filter(element=>displayed(element)&&/(批量.*评价|评价.*选中)/.test(textOf(element)));
  if(!labels.length)return JSON.stringify({clicked:false,reason:'batch_review_button_not_found'});
  // Prefer the smallest text node, then climb to the actual clickable
  // ancestor. This covers div/span based buttons used by the seller console.
  labels.sort((left,right)=>left.childElementCount-right.childElementCount||textOf(left).length-textOf(right).length);
  let button=labels[0];
  for(let depth=0;button&&depth<6;depth+=1,button=button.parentElement){
    if(button.matches('button,a,[role="button"]')||typeof button.onclick==='function'||/button|btn|action|operate/i.test(String(button.className||'')))break;
  }
  if(!button)return JSON.stringify({clicked:false,reason:'batch_review_button_not_clickable'});
  if(disabled(button))return JSON.stringify({clicked:false,reason:'batch_review_button_disabled'});
  button.scrollIntoView({block:'center',inline:'nearest'});
  // One DOM click is sufficient. Dispatching pointer/mouse events and then
  // calling click() can invoke custom seller-console handlers twice.
  button.click();
  return JSON.stringify({clicked:true,reason:'batch_review_clicked'});
})()`
}

func buildAgentXianyuReviewSelectPageExpression(listSelector, actionSelector string, reviewedKeys []string) string {
	listJSON := quoteAgentXianyuJS(listSelector)
	_ = actionSelector
	reviewedJSON, _ := json.Marshal(reviewedKeys)
	return fmt.Sprintf(`(() => {
  const customList=%s,reviewed=new Set(%s);
  const clean=value=>String(value||'').replace(/\s+/g,' ').trim();
  const displayed=element=>{if(!element)return false;const rect=element.getBoundingClientRect(),style=getComputedStyle(element);return rect.width>0&&rect.height>0&&style.display!=='none'&&style.visibility!=='hidden'&&style.opacity!=='0';};
  const disabled=element=>!element||element.disabled||element.getAttribute('aria-disabled')==='true'||/(^|[-_\s])(disabled|is-disabled)([-_\s]|$)/i.test(String(element.className||''));
  // Hard guard: bulk selection is allowed only while the actual Ant radio for
  // "pending seller review" is checked. Never trust route text or CSS alone.
  const pendingLabels=[...document.querySelectorAll('label')].filter(element=>displayed(element)&&/^待卖家评价\s*\d*$/.test(clean(element.innerText||element.textContent)));
  const pendingLabel=pendingLabels.find(element=>element.querySelector('input[type="radio"]'));
  const pendingInput=pendingLabel&&pendingLabel.querySelector('input[type="radio"]');
  if(!pendingInput||!pendingInput.checked||String(pendingInput.value||'').toUpperCase()==='ALL')return JSON.stringify({found:false,pending:0,selected:0,order_keys:[],reviewed_order_keys:[],signature:'',batch_clicked:false,reason:'pending_seller_review_tab_not_active'});
  let scope=document;
  if(customList){try{scope=document.querySelector(customList)||document;}catch(_){return JSON.stringify({found:false,pending:0,selected:0,order_keys:[],reviewed_order_keys:[],signature:'',batch_clicked:false,reason:'invalid_order_list_selector'});}}
  // The pending-seller-review tab is already the server-side filter. Every
  // visible keyed row on this page belongs to the current batch; do not infer
  // eligibility from per-row text or click row checkboxes one by one.
  const orderKeys=[...scope.querySelectorAll('tr[data-row-key]')]
    .filter(displayed)
    .map(row=>String(row.getAttribute('data-row-key')||'').trim())
    .filter((key,index,items)=>key&&items.indexOf(key)===index);
  const reviewedOnPage=orderKeys.filter(key=>reviewed.has(key));
  const signature=orderKeys.join('|');
  if(!orderKeys.length)return JSON.stringify({found:false,pending:0,selected:0,order_keys:[],reviewed_order_keys:[],signature,batch_clicked:false,reason:'no_pending_review_rows'});
  // A successful batch can leave the old page mounted briefly. Skip the
  // entire stale page, but never fall back to per-row selection.
  if(reviewedOnPage.length===orderKeys.length)return JSON.stringify({found:false,pending:0,selected:0,order_keys:[],reviewed_order_keys:reviewedOnPage,signature,batch_clicked:false,reason:'current_page_has_no_pending_reviews'});

  const selectAll=[...scope.querySelectorAll('thead input[type="checkbox"],thead [role="checkbox"]')]
    .find(element=>!disabled(element));
  if(!selectAll)return JSON.stringify({found:true,pending:orderKeys.length,selected:0,order_keys:orderKeys,reviewed_order_keys:[],signature,batch_clicked:false,reason:'pending_review_select_all_not_found'});
  const input=selectAll.matches('input[type="checkbox"]')?selectAll:selectAll.querySelector('input[type="checkbox"]');
  const checked=input?input.checked:selectAll.getAttribute('aria-checked')==='true';
  if(!checked){
    const target=selectAll.closest('label')||selectAll;
    target.scrollIntoView({block:'nearest',inline:'nearest'});
    target.click();
  }
  // Let the caller wait for the controlled checkbox state to settle before
  // clicking the bulk action. Clicking it in this same task races React and
  // leaves the button disabled even though the boxes look selected.
  return JSON.stringify({found:true,pending:orderKeys.length,selected:orderKeys.length,order_keys:orderKeys,reviewed_order_keys:[],signature,batch_clicked:false,reason:checked?'page_already_selected_waiting_batch_button':'page_selected_waiting_batch_button'});
})()`, listJSON, string(reviewedJSON))
}

func buildAgentXianyuReviewOpenNextExpression(listSelector, actionSelector string, reviewedKeys, attemptedKeys []string) string {
	listJSON := quoteAgentXianyuJS(listSelector)
	actionJSON := quoteAgentXianyuJS(actionSelector)
	reviewedJSON, _ := json.Marshal(reviewedKeys)
	attemptedJSON, _ := json.Marshal(attemptedKeys)
	return fmt.Sprintf(`(() => {
 const customList=%s,customAction=%s,reviewed=new Set(%s),attempted=new Set(%s);
 const clean=value=>String(value||'').replace(/\s+/g,' ').trim();
 const displayed=element=>{if(!element)return false;const rect=element.getBoundingClientRect(),style=getComputedStyle(element);return rect.width>0&&rect.height>0&&style.display!=='none'&&style.visibility!=='hidden'&&style.opacity!=='0';};
  const disabled=element=>!element||element.disabled||element.getAttribute('aria-disabled')==='true'||/(^|[-_\s])(disabled|is-disabled)([-_\s]|$)/i.test(String(element.className||''));
 const extractOrder=text=>{const match=clean(text).match(/(?:订单编号|订单号)\s*[:：]?\s*([A-Za-z0-9_-]{8,})/);return match?match[1]:'';};
 let scope=document;
 if(customList){try{scope=document.querySelector(customList)||document;}catch(_){return JSON.stringify({found:false,visible_count:0,signature:'',reviewed_order_keys:[],reason:'invalid_order_list_selector'});}}
 let rawActions=[];
 if(customAction){try{rawActions=[...scope.querySelectorAll(customAction)];}catch(_){return JSON.stringify({found:false,visible_count:0,signature:'',reviewed_order_keys:[],reason:'invalid_review_action_selector'});}}
 if(!rawActions.length){
   rawActions=[...scope.querySelectorAll('button,a,[role="button"]')].filter(element=>/^(去评价|立即评价|评价商品|评价)$/.test(clean(element.innerText||element.textContent||element.getAttribute('aria-label'))));
   const textActions=[...scope.querySelectorAll('span,div')].filter(element=>element.childElementCount===0&&/^(去评价|立即评价|评价商品|评价)$/.test(clean(element.textContent)));
   rawActions.push(...textActions.map(element=>element.closest('button,a,[role="button"]')||element));
 }
 const actions=[...new Set(rawActions)].filter(element=>displayed(element)&&!disabled(element));
 const markerCandidates=[...scope.querySelectorAll('div,span,p')].filter(element=>{const text=clean(element.innerText||element.textContent);return displayed(element)&&text.length<160&&/(?:订单编号|订单号)\s*[:：]?\s*[A-Za-z0-9_-]{8,}/.test(text);});
 const orderForAction=action=>{
   let current=action;
   for(let depth=0;current&&depth<12;depth+=1,current=current.parentElement){
     const text=clean(current.innerText||current.textContent);
     const matches=[...text.matchAll(/(?:订单编号|订单号)\s*[:：]?\s*([A-Za-z0-9_-]{8,})/g)].map(match=>match[1]);
     const unique=[...new Set(matches)];
     if(unique.length===1&&text.length<8000)return unique[0];
     if(current===scope)break;
   }
   const actionRect=action.getBoundingClientRect();
   const nearby=markerCandidates.map(element=>({key:extractOrder(element.innerText||element.textContent),rect:element.getBoundingClientRect()})).filter(item=>item.key&&item.rect.top<=actionRect.bottom+40).sort((left,right)=>Math.abs(actionRect.top-left.rect.top)-Math.abs(actionRect.top-right.rect.top));
   return nearby.length?nearby[0].key:'';
 };
 const items=[];
 for(const action of actions){const key=orderForAction(action);if(key&&!items.some(item=>item.key===key))items.push({key,action});}
 const keys=items.map(item=>item.key);
 const reviewedOnPage=keys.filter(key=>reviewed.has(key));
 const candidate=items.find(item=>!reviewed.has(item.key)&&!attempted.has(item.key));
 const signature=keys.join('|');
 if(!candidate)return JSON.stringify({found:false,order_key:'',visible_count:items.length,signature,reviewed_order_keys:reviewedOnPage,reason:items.length?'all_visible_orders_already_processed':'no_pending_review_action'});
 candidate.action.scrollIntoView({block:'center',inline:'nearest'});
 candidate.action.click();
 return JSON.stringify({found:true,order_key:candidate.key,visible_count:items.length,signature,reviewed_order_keys:reviewedOnPage,reason:''});
})()`, listJSON, actionJSON, string(reviewedJSON), string(attemptedJSON))
}

func buildAgentXianyuReviewSubmitExpression(dialogSelector, inputSelector, ratingSelector, submitSelector, text string) string {
	dialogJSON := quoteAgentXianyuJS(dialogSelector)
	inputJSON := quoteAgentXianyuJS(inputSelector)
	ratingJSON := quoteAgentXianyuJS(ratingSelector)
	submitJSON := quoteAgentXianyuJS(submitSelector)
	textJSON := quoteAgentXianyuJS(text)
	return fmt.Sprintf(`(() => {
 const customDialog=%s,customInput=%s,customRating=%s,customSubmit=%s,review=%s;
 const clean=value=>String(value||'').replace(/\s+/g,' ').trim();
 const displayed=element=>{if(!element)return false;const rect=element.getBoundingClientRect(),style=getComputedStyle(element);return rect.width>0&&rect.height>0&&style.display!=='none'&&style.visibility!=='hidden'&&style.opacity!=='0';};
 const disabled=element=>!element||element.disabled||element.getAttribute('aria-disabled')==='true'||/(^|[-_\s])(disabled|is-disabled)([-_\s]|$)/i.test(String(element.className||''));
 let root=null;
 if(customDialog){try{root=[...document.querySelectorAll(customDialog)].filter(displayed).pop()||null;}catch(_){return JSON.stringify({submitted:false,reason:'invalid_review_dialog_selector'});}}
 if(!root){const dialogs=[...document.querySelectorAll('[role="dialog"],[aria-modal="true"],[class*="modal" i],[class*="dialog" i],[class*="drawer" i],[class*="popup" i],[class*="overlay" i]')].filter(displayed);root=dialogs.pop()||document;}
 let inputs=[];
 if(customInput){try{inputs=[...root.querySelectorAll(customInput)];if(!inputs.length)inputs=[...document.querySelectorAll(customInput)];}catch(_){return JSON.stringify({submitted:false,reason:'invalid_review_input_selector'});}}
 if(!inputs.length){
   const candidates=[...root.querySelectorAll('textarea,[contenteditable="true"],input:not([type="hidden"]):not([type="checkbox"]):not([type="radio"])')];
   inputs=candidates.filter(element=>{const hint=clean(element.getAttribute('placeholder')||element.getAttribute('aria-label')||element.getAttribute('name')||element.getAttribute('title'));return element.tagName==='TEXTAREA'||element.isContentEditable||/(评价|感受|内容|留言|心得)/.test(hint);});
 }
 const input=inputs.find(displayed);
 if(!input)return JSON.stringify({submitted:false,reason:'review_input_not_found'});
 let rating=null;
 if(customRating){try{rating=[...root.querySelectorAll(customRating)].find(displayed)||[...document.querySelectorAll(customRating)].find(displayed)||null;}catch(_){return JSON.stringify({submitted:false,reason:'invalid_positive_rating_selector'});}}
 if(!rating){
   const labels=[...root.querySelectorAll('label,button,[role="radio"],[role="button"],span,div')].filter(element=>displayed(element)&&element.childElementCount<4&&/^(好评|满意|五星|5星)$/.test(clean(element.innerText||element.textContent||element.getAttribute('aria-label'))));
   rating=labels[0]||root.querySelector('[data-score="5"],[data-rating="5"],[data-value="5"],[aria-label*="5星"],[title*="5星"]');
   if(!rating){const stars=[...root.querySelectorAll('[role="radio"],[class*="star" i],[class*="rate" i] [class*="item" i]')].filter(displayed);if(stars.length>=5)rating=stars[4];}
 }
 if(!rating)return JSON.stringify({submitted:false,reason:'positive_rating_not_found'});
 const ratingTarget=rating.closest('label,button,[role="radio"],[role="button"]')||rating;ratingTarget.click();
 input.focus();
 if(input.isContentEditable){
   input.textContent=review;
   input.dispatchEvent(new InputEvent('input',{bubbles:true,inputType:'insertText',data:review}));
   input.dispatchEvent(new Event('change',{bubbles:true}));
 }else{
   const proto=input.tagName==='TEXTAREA'?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;
   const descriptor=Object.getOwnPropertyDescriptor(proto,'value');
   if(descriptor&&descriptor.set)descriptor.set.call(input,review);else input.value=review;
   input.dispatchEvent(new InputEvent('input',{bubbles:true,inputType:'insertText',data:review}));
   input.dispatchEvent(new Event('change',{bubbles:true}));
   input.dispatchEvent(new Event('blur',{bubbles:true}));
 }
 let buttons=[];
 if(customSubmit){try{buttons=[...root.querySelectorAll(customSubmit)];if(!buttons.length)buttons=[...document.querySelectorAll(customSubmit)];}catch(_){return JSON.stringify({submitted:false,reason:'invalid_review_submit_selector'});}}
 if(!buttons.length)buttons=[...root.querySelectorAll('button,a,[role="button"],input[type="submit"]')].filter(element=>/^(确认评价|提交评价|发布评价|发表评价|确认提交|提交|发布|确定|完成|评价)$/.test(clean(element.innerText||element.textContent||element.getAttribute('aria-label')||element.getAttribute('value'))));
 const button=buttons.find(element=>displayed(element)&&!disabled(element));
 if(!button)return JSON.stringify({submitted:false,reason:'review_submit_button_not_found'});
 button.scrollIntoView({block:'center',inline:'nearest'});
 // Ant Design/React updates the controlled textarea and radio state after the
 // input event returns. Defer the actual publish click by one macrotask so the
 // batch request receives the text and rating instead of submitting stale form
 // state (which the seller console reports as all orders failed).
 const publish=()=>button.click();
 setTimeout(publish,120);
 return JSON.stringify({submitted:true,reason:'review_submit_scheduled'});
})()`, dialogJSON, inputJSON, ratingJSON, submitJSON, textJSON)
}

func buildAgentXianyuReviewBatchConfirmationExpression(inputSelector string, orderKeys []string) string {
	inputJSON := quoteAgentXianyuJS(inputSelector)
	keysJSON, _ := json.Marshal(orderKeys)
	return fmt.Sprintf(`(() => {
  const customInput=%s,expected=new Set(%s);
  const clean=value=>String(value||'').replace(/\\s+/g,' ').trim();
  const displayed=element=>{if(!element)return false;const rect=element.getBoundingClientRect(),style=getComputedStyle(element);return rect.width>0&&rect.height>0&&style.display!=='none'&&style.visibility!=='hidden'&&style.opacity!=='0';};
  const dialogs=[...document.querySelectorAll('[role="dialog"],[aria-modal="true"],[class*="modal" i],[class*="dialog" i]')].filter(displayed);
  const failureDialog=dialogs.find(element=>/评价失败的订单/.test(clean(element.innerText||element.textContent)));
  if(failureDialog){
    const failures=[];
    for(const row of failureDialog.querySelectorAll('tbody tr,tr[data-row-key],[role="row"]')){
      if(!displayed(row))continue;
      const cells=[...row.querySelectorAll('td,[role="cell"]')].map(cell=>clean(cell.innerText||cell.textContent)).filter(Boolean);
      const orderId=clean(row.getAttribute('data-row-key')||cells[0]);
      const failReason=clean(cells.slice(1).join(' '));
      if(orderId&&failReason&&orderId!=='订单')failures.push({orderId,failReason});
    }
    if(!failures.length)return JSON.stringify({confirmed:false,failed:false,skipped_order_keys:[],reason:'batch_review_failure_detail_loading'});
    const matching=failures.filter(failure=>expected.has(failure.orderId));
    if(!matching.length){
      // A failure-detail dialog can survive from an earlier run. Never apply
      // those order ids to the newly selected batch; close it and probe again.
      const close=[...failureDialog.querySelectorAll('button,a,[role="button"]')].find(element=>displayed(element)&&/^(我知道了|知道了|关闭|取消|×|X)$/.test(clean(element.innerText||element.textContent||element.getAttribute('aria-label')||element.getAttribute('title'))));
      if(close)close.click();
      return JSON.stringify({confirmed:false,failed:false,skipped_order_keys:[],reason:'stale_batch_failure_detail_closed'});
    }
    const grouped=new Map();
    for(const failure of matching){
      const ids=grouped.get(failure.failReason)||[];
      ids.push(failure.orderId);
      grouped.set(failure.failReason,ids);
    }
    const detail=[...grouped.entries()].map(([reason,ids])=>reason+'（'+ids.length+'单：'+ids.slice(0,3).join('、')+(ids.length>3?'等':'')+'）').join('；');
    const alreadyProcessed=matching.filter(failure=>/(24\s*小时内只能追评一次|评价未生效[^，。]*不能追加评价|评价等级仅对初次评价生效)/.test(failure.failReason));
    if(alreadyProcessed.length===expected.size){
      return JSON.stringify({confirmed:true,failed:false,skipped_order_keys:alreadyProcessed.map(failure=>failure.orderId),reason:'订单已有首评，跳过重复评价：'+detail});
    }
    return JSON.stringify({confirmed:false,failed:true,skipped_order_keys:[],reason:'闲鱼批量评价失败：'+detail});
  }
  const noticeElements=[...document.querySelectorAll('[role="alert"],[class*="message" i],[class*="toast" i],[class*="notification" i],[class*="notice" i],[class*="alert" i]')].filter(element=>displayed(element)&&clean(element.innerText||element.textContent).length<260);
  const notices=noticeElements.map(element=>clean(element.innerText||element.textContent));
  const pageText=clean(document.body&&document.body.innerText);
  const failed=notices.find(text=>/(评价失败|提交失败|发布失败|发表失败|请重试|操作失败|成功评价\s*0\s*个订单|其他\s*\d+\s*个订单评价失败)/.test(text))||((/成功评价\s*0\s*个订单/.test(pageText)&&/评价失败/.test(pageText))?'batch_review_failed_notice':'');
  if(failed){
    const failedNotice=noticeElements.find(element=>clean(element.innerText||element.textContent)===failed);
    const view=failedNotice&&[...failedNotice.querySelectorAll('button,a,[role="button"],span')].find(element=>displayed(element)&&/^查看$/.test(clean(element.innerText||element.textContent)));
    if(view){
      view.click();
      return JSON.stringify({confirmed:false,failed:false,reason:'batch_review_failure_detail_opening'});
    }
    return JSON.stringify({confirmed:false,failed:true,reason:failed});
  }
  const success=notices.find(text=>/(评价成功|提交成功|发布成功|发表成功|评价完成|已评价)/.test(text));
  if(success)return JSON.stringify({confirmed:true,failed:false,reason:success});
  let editors=[];
  if(customInput){try{editors=[...document.querySelectorAll(customInput)];}catch(_) {}}
  if(!editors.length)editors=[...document.querySelectorAll('[role="dialog"] textarea,[aria-modal="true"] textarea,[class*="modal" i] textarea,[class*="drawer" i] textarea,[role="dialog"] [contenteditable="true"],[aria-modal="true"] [contenteditable="true"],[class*="modal" i] [contenteditable="true"],[class*="drawer" i] [contenteditable="true"]')];
  if(editors.some(displayed))return JSON.stringify({confirmed:false,failed:false,reason:'review_editor_still_open'});
  const orderPattern=/(?:订单编号|订单号)\\s*[:：]?\\s*([A-Za-z0-9_-]{8,})/g;
  const actionPattern=/^(去评价|立即评价|评价商品|评价)$/;
  const hasReviewAction=row=>[...row.querySelectorAll('button,a,[role="button"],span,div')].some(element=>displayed(element)&&actionPattern.test(clean(element.innerText||element.textContent||element.getAttribute('aria-label')))&&(element.childElementCount===0||element.matches('button,a,[role="button"]')));
  const unresolved=[];
  let matchedCount=0;
  for(const key of expected){
    // Current seller-console rows expose the order key directly on the
    // content row. The checkbox/order-number row is its previous sibling, so
    // do not require the text "订单编号" to be repeated in the content row.
    const keyedRow=[...document.querySelectorAll('tr[data-row-key]')].find(element=>element.getAttribute('data-row-key')===key&&displayed(element));
    if(!keyedRow){
      // A successful batch removes the reviewed order from the pending list.
      matchedCount+=1;
      continue;
    }
    const keyedText=clean(keyedRow.innerText||keyedRow.textContent);
    if(keyedText.includes('买家未评价')&&hasReviewAction(keyedRow))unresolved.push(key);
    else matchedCount+=1;
  }
  if(unresolved.length)return JSON.stringify({confirmed:false,failed:false,reason:'review_orders_still_pending:'+unresolved.join(',')});
  if(matchedCount===expected.size)return JSON.stringify({confirmed:true,failed:false,reason:'batch_review_action_removed'});
  if(matchedCount===0)return JSON.stringify({confirmed:false,failed:false,reason:'review_orders_not_found'});
  /* Keep the legacy text-based fallback for seller-console variants that do
     not expose data-row-key on their rendered rows. */
  for(const key of expected){
    if([...document.querySelectorAll('tr[data-row-key]')].some(element=>element.getAttribute('data-row-key')===key))continue;
    const nodes=[...document.querySelectorAll('tr,[role="row"],li,[class*="item" i],[class*="order" i],[class*="card" i],[class*="row" i],div')].filter(element=>displayed(element)&&clean(element.innerText||element.textContent).includes(key));
    let matched=false;
    for(const node of nodes){
      let current=node;
      for(let depth=0;current&&depth<10;depth+=1,current=current.parentElement){
        const keys=[...clean(current.innerText||current.textContent).matchAll(orderPattern)].map(match=>match[1]);
        if(keys.length===1&&keys[0]===key){
          matched=true;
          matchedCount+=1;
          if(hasReviewAction(current))unresolved.push(key);
          break;
        }
      }
      if(matched)break;
    }
  }
  if(unresolved.length)return JSON.stringify({confirmed:false,failed:false,reason:'review_orders_still_pending:'+unresolved.join(',')});
  if(matchedCount>=expected.size)return JSON.stringify({confirmed:true,failed:false,reason:'batch_review_action_removed'});
  return JSON.stringify({confirmed:false,failed:false,reason:'review_orders_not_found'});
})()`, inputJSON, string(keysJSON))
}

func buildAgentXianyuReviewConfirmationExpression(inputSelector, orderKey string) string {
	inputJSON := quoteAgentXianyuJS(inputSelector)
	orderJSON := quoteAgentXianyuJS(orderKey)
	return fmt.Sprintf(`(() => {
 const customInput=%s,orderKey=%s;
 const clean=value=>String(value||'').replace(/\s+/g,' ').trim();
 const displayed=element=>{if(!element)return false;const rect=element.getBoundingClientRect(),style=getComputedStyle(element);return rect.width>0&&rect.height>0&&style.display!=='none'&&style.visibility!=='hidden'&&style.opacity!=='0';};
 const notices=[...document.querySelectorAll('[role="alert"],[class*="message" i],[class*="toast" i],[class*="notification" i]')].filter(element=>displayed(element)&&clean(element.innerText||element.textContent).length<240).map(element=>clean(element.innerText||element.textContent));
 const failed=notices.find(text=>/(评价失败|提交失败|发布失败|发表失败|请重试|操作失败)/.test(text));
 if(failed)return JSON.stringify({confirmed:false,failed:true,reason:failed});
 const success=notices.find(text=>/(评价成功|提交成功|发布成功|发表成功|评价完成|已评价|成功评价\s*\d+\s*个订单)/.test(text));
 if(success)return JSON.stringify({confirmed:true,failed:false,reason:success});
 let editors=[];
 if(customInput){try{editors=[...document.querySelectorAll(customInput)];}catch(_){}}
 if(!editors.length)editors=[...document.querySelectorAll('[role="dialog"] textarea,[aria-modal="true"] textarea,[class*="modal" i] textarea,[class*="drawer" i] textarea,[role="dialog"] [contenteditable="true"],[aria-modal="true"] [contenteditable="true"],[class*="modal" i] [contenteditable="true"],[class*="drawer" i] [contenteditable="true"]')];
 const editorOpen=editors.some(displayed);
 if(editorOpen)return JSON.stringify({confirmed:false,failed:false,reason:'review_editor_still_open'});
 const orderNodes=[...document.querySelectorAll('tr,[role="row"],li,[class*="item" i],[class*="order" i],[class*="card" i],[class*="row" i],div')].filter(element=>displayed(element)&&orderKey&&clean(element.innerText||element.textContent).includes(orderKey)&&clean(element.innerText||element.textContent).length<1600);
 const orderPattern=/(?:订单编号|订单号)\s*[:：]?\s*([A-Za-z0-9_-]{8,})/g;
 for(const node of orderNodes){
   let current=node;
   for(let depth=0;current&&depth<12;depth+=1,current=current.parentElement){
     const text=clean(current.innerText||current.textContent);
     const keys=[...text.matchAll(orderPattern)].map(match=>match[1]);
     if(keys.length===1&&keys[0]===orderKey){
       const actions=[...current.querySelectorAll('button,a,[role="button"]')].filter(element=>displayed(element)&&/^(去评价|立即评价|评价商品|评价)$/.test(clean(element.innerText||element.textContent||element.getAttribute('aria-label'))));
       if(actions.length)return JSON.stringify({confirmed:false,failed:false,reason:'review_order_still_pending'});
       if(current.matches('tr,[role="row"],li,[class*="item" i],[class*="order" i],[class*="card" i],[class*="row" i]'))return JSON.stringify({confirmed:true,failed:false,reason:'review_order_action_removed'});
     }
   }
 }
 if(orderNodes.length)return JSON.stringify({confirmed:false,failed:false,reason:'review_order_scope_not_found'});
 return JSON.stringify({confirmed:false,failed:false,reason:'review_success_not_confirmed'});
})()`, inputJSON, orderJSON)
}

func buildAgentXianyuReviewCloseExpression() string {
	return `(() => {
 const clean=value=>String(value||'').replace(/\s+/g,' ').trim();
 const displayed=element=>{if(!element)return false;const rect=element.getBoundingClientRect(),style=getComputedStyle(element);return rect.width>0&&rect.height>0&&style.display!=='none'&&style.visibility!=='hidden'&&style.opacity!=='0';};
 const dialogs=[...document.querySelectorAll('[role="dialog"],[aria-modal="true"],[class*="modal" i],[class*="dialog" i],[class*="drawer" i],[class*="popup" i]')].filter(displayed);
 const root=dialogs.pop();
 if(!root)return JSON.stringify({closed:false});
 const close=[...root.querySelectorAll('button,a,[role="button"]')].find(element=>displayed(element)&&/^(取消|关闭|返回|稍后|再想想|我知道了|知道了|×|X)$/.test(clean(element.innerText||element.textContent||element.getAttribute('aria-label')||element.getAttribute('title'))));
 if(close){close.click();return JSON.stringify({closed:true});}
 document.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',code:'Escape',keyCode:27,which:27,bubbles:true}));
 return JSON.stringify({closed:true});
})()`
}

func buildAgentXianyuReviewNextPageExpression() string {
	return `(() => {
 const clean=value=>String(value||'').replace(/\s+/g,' ').trim();
 const displayed=element=>{if(!element)return false;const rect=element.getBoundingClientRect(),style=getComputedStyle(element);return rect.width>0&&rect.height>0&&style.display!=='none'&&style.visibility!=='hidden';};
  const disabled=element=>!element||element.disabled||element.getAttribute('aria-disabled')==='true'||/(^|[-_\s])(disabled|is-disabled)([-_\s]|$)/i.test(String(element.className||''));
 const selectors=['.ant-pagination-next button','.ant-pagination-next','.rc-pagination-next button','.rc-pagination-next','.semi-page-next','.semi-pagination-next','[class*="pagination-next"] button','[class*="pagination-next"]','[class*="page-next"]','[aria-label="下一页"]','[title="下一页"]'];
 let next=null;
 for(const selector of selectors){try{const candidate=[...document.querySelectorAll(selector)].find(displayed);if(candidate){next=candidate;break;}}catch(_){}}
 if(!next)next=[...document.querySelectorAll('button,a,[role="button"],li')].find(element=>displayed(element)&&/^(下一页|下页|next|›|>)$/i.test(clean(element.innerText||element.textContent||element.getAttribute('aria-label')||element.getAttribute('title'))));
 if(!next)return JSON.stringify({clicked:false,at_end:true,reason:'next_page_not_found'});
 const target=next.matches('button,a,[role="button"]')?next:(next.querySelector('button,a,[role="button"]')||next);
 if(disabled(next)||disabled(target))return JSON.stringify({clicked:false,at_end:true,reason:'last_page'});
 target.scrollIntoView({block:'center',inline:'nearest'});
 target.click();
 return JSON.stringify({clicked:true,at_end:false,reason:''});
})()`
}

func agentXianyuReviewMapKeys(values map[string]int64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key = strings.TrimSpace(key); key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func agentXianyuReviewSetKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key = strings.TrimSpace(key); key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func failAgentXianyuReviewWithStats(result agentBlueprintNodeResult, message, errText string, stats agentXianyuReviewStats) agentBlueprintNodeResult {
	result.Status = "failed"
	result.Message = message
	result.Error = errText
	result.Output = map[string]interface{}{
		"source":                 "闲鱼评价管理",
		"scanned_order_count":    stats.Scanned,
		"eligible_order_count":   stats.Eligible,
		"submitted_review_count": stats.Submitted,
		"skipped_count":          stats.Skipped,
		"failed_count":           stats.Failed,
		"error":                  errText,
	}
	return result
}

func agentXianyuReviewStatePath(dataDir, blueprintID, nodeID string) string {
	return filepath.Join(dataDir, agentXianyuReviewStateDir, sanitizeAgentBrowserFilenamePart(blueprintID), sanitizeAgentBrowserFilenamePart(nodeID)+".json")
}

func loadAgentXianyuReviewState(path string) (agentXianyuReviewState, error) {
	state := agentXianyuReviewState{Reviewed: map[string]int64{}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err = json.Unmarshal(raw, &state); err != nil {
		return state, err
	}
	if state.Reviewed == nil {
		state.Reviewed = map[string]int64{}
	}
	return state, nil
}

func saveAgentXianyuReviewState(path string, state agentXianyuReviewState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, raw, 0644); err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
