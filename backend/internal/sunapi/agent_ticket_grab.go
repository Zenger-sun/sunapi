package sunapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	agentTicketDefaultInterval = 5 * time.Second
	agentTicketMinInterval     = 3 * time.Second
	agentTicketMaxInterval     = 60 * time.Second
	agentTicketDefaultDuration = 8 * time.Minute
	agentTicketMaxDuration     = 30 * time.Minute
	agentTicketDefaultAttempts = 96
	agentTicketMaxAttempts     = 600
)

type agentTicketGrabConfig struct {
	AvailabilityKeywords []string
	UnavailableKeywords  []string
	FilterKeywords       []string
	ClickLabels          []string
	Action               string
	Interval             time.Duration
	MaxDuration          time.Duration
	MaxAttempts          int
}

type agentTicketPagePayload struct {
	Title   string   `json:"title"`
	URL     string   `json:"url"`
	Text    string   `json:"text"`
	Actions []string `json:"actions"`
}

type agentTicketMatch struct {
	Found   bool
	Keyword string
	Action  string
}

type agentTicketClickResult struct {
	Clicked bool   `json:"clicked"`
	Label   string `json:"label"`
}

func executeAgentTicketGrabGraphNode(
	ctx context.Context,
	store *Store,
	node agentBlueprintGraphNode,
	upstream map[string]interface{},
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	if store == nil || strings.TrimSpace(store.DataDir()) == "" {
		return failedAgentTicketResult(result, "智能抢票节点无法访问浏览器数据目录。", "data directory is not configured")
	}

	settings := node.Data.Settings
	sourceURL := firstNonEmpty(
		settings["抢票页面"],
		findAgentStringRecursive(upstream, "final_url", "url"),
	)
	if strings.TrimSpace(sourceURL) == "" {
		return failedAgentTicketResult(result, "智能抢票节点没有收到抢票页面地址。", "ticket page url is required")
	}
	if _, err := normalizeAgentBrowserURL(sourceURL); err != nil {
		return failedAgentTicketResult(result, "智能抢票页面地址无效。", err.Error())
	}

	config := parseAgentTicketGrabConfig(settings)
	sessionName := firstNonEmpty(
		settings["会话名称"],
		findAgentStringRecursive(upstream, "browser_session"),
	)
	session := resolveAgentBrowserSessionPaths(store.DataDir(), sessionName)
	endpoint, err := ensureAgentBrowserDevtoolsForSession(ctx, session)
	if err != nil {
		return failedAgentTicketResult(result, "智能抢票节点启动浏览器失败。", err.Error())
	}

	target, err := resolveAgentTicketBrowserTarget(
		ctx,
		endpoint,
		sourceURL,
		findAgentStringRecursive(upstream, "browser_target_id"),
	)
	if err != nil {
		return failedAgentTicketResult(result, "智能抢票节点无法连接登录浏览器。", err.Error())
	}

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, target.WebSocketDebuggerURL, nil)
	if err != nil {
		return failedAgentTicketResult(result, "智能抢票节点连接浏览器页面失败。", err.Error())
	}
	defer conn.Close()

	if _, err := agentBrowserCDPCall(ctx, conn, 1, "Page.enable", nil); err != nil {
		return failedAgentTicketResult(result, "智能抢票节点初始化页面失败。", err.Error())
	}
	if _, err := agentBrowserCDPCall(ctx, conn, 2, "Runtime.enable", nil); err != nil {
		return failedAgentTicketResult(result, "智能抢票节点初始化脚本失败。", err.Error())
	}
	if _, err := agentBrowserCDPCall(ctx, conn, 3, "Page.navigate", map[string]interface{}{"url": sourceURL}); err != nil {
		return failedAgentTicketResult(result, "智能抢票节点打开目标页面失败。", err.Error())
	}
	if err := waitAgentTicketGrab(ctx, minAgentTicketDuration(config.Interval, 3*time.Second)); err != nil {
		return failedAgentTicketResult(result, "智能抢票节点已停止。", err.Error())
	}

	started := time.Now()
	deadline := started.Add(config.MaxDuration)
	queueDetected := false
	attempts := 0
	lastPage := agentTicketPagePayload{URL: sourceURL}
	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		attempts = attempt
		page, err := evaluateAgentTicketPage(ctx, conn, 100+attempt*4)
		if err != nil {
			return failedAgentTicketResult(result, "智能抢票节点读取页面失败。", err.Error())
		}
		lastPage = page
		if looksLikeAgentBrowserLogin(agentBrowserTextPayload{Title: page.Title, URL: page.URL, Text: page.Text}) {
			return completedAgentTicketResult(result, map[string]interface{}{
				"status":                "login_required",
				"found":                 false,
				"manual_action":         true,
				"attempts":              attempt,
				"final_url":             page.URL,
				"browser_session":       session.Name,
				"browser_target_id":     target.ID,
				"requires_confirmation": true,
			}, "登录会话已失效，请在持久化浏览器中重新登录后再运行。")
		}
		if agentTicketPageNeedsManualVerification(page) {
			return completedAgentTicketResult(result, map[string]interface{}{
				"status":                "manual_verification_required",
				"found":                 false,
				"manual_action":         true,
				"attempts":              attempt,
				"final_url":             page.URL,
				"browser_session":       session.Name,
				"browser_target_id":     target.ID,
				"requires_confirmation": true,
			}, "页面要求人工完成验证码或安全验证，智能抢票已暂停。")
		}

		match := matchAgentTicketPage(page, config)
		if match.Found {
			clicked := agentTicketClickResult{}
			if strings.Contains(config.Action, "进入确认页") {
				clicked, err = clickAgentTicketEntry(ctx, conn, 101+attempt*4, config.ClickLabels)
				if err != nil {
					return failedAgentTicketResult(result, "检测到票源，但点击购票入口失败。", err.Error())
				}
				if clicked.Clicked {
					if err := waitAgentTicketGrab(ctx, 2*time.Second); err != nil {
						return failedAgentTicketResult(result, "智能抢票节点已停止。", err.Error())
					}
					if confirmationPage, captureErr := evaluateAgentTicketPage(ctx, conn, 102+attempt*4); captureErr == nil {
						lastPage = confirmationPage
					}
				}
			}

			screenshotPath := captureAgentTicketScreenshot(ctx, store.DataDir(), target, lastPage.URL)
			message := "检测到可购买票源，已停止轮询并等待人工确认。"
			if clicked.Clicked {
				message = "检测到可购买票源，已进入购票确认阶段；提交订单和支付需人工完成。"
			}
			return completedAgentTicketResult(result, map[string]interface{}{
				"status":                "confirmation_required",
				"found":                 true,
				"matched_keyword":       match.Keyword,
				"matched_action":        match.Action,
				"clicked":               clicked.Clicked,
				"clicked_label":         clicked.Label,
				"attempts":              attempt,
				"elapsed_ms":            time.Since(started).Milliseconds(),
				"final_url":             lastPage.URL,
				"title":                 lastPage.Title,
				"screenshot_path":       screenshotPath,
				"browser_session":       session.Name,
				"browser_target_id":     target.ID,
				"requires_confirmation": true,
				"final_submit_clicked":  false,
				"payment_attempted":     false,
			}, message)
		}

		queued := agentTicketPageIsQueued(page)
		queueDetected = queueDetected || queued
		if attempt >= config.MaxAttempts || time.Now().Add(config.Interval).After(deadline) {
			break
		}
		if !queued {
			if _, err := agentBrowserCDPCall(ctx, conn, 103+attempt*4, "Page.reload", map[string]interface{}{"ignoreCache": true}); err != nil {
				return failedAgentTicketResult(result, "智能抢票节点刷新页面失败。", err.Error())
			}
		}
		if err := waitAgentTicketGrab(ctx, config.Interval); err != nil {
			return failedAgentTicketResult(result, "智能抢票节点已停止。", err.Error())
		}
	}

	return completedAgentTicketResult(result, map[string]interface{}{
		"status":                "not_found",
		"found":                 false,
		"attempts":              attempts,
		"elapsed_ms":            time.Since(started).Milliseconds(),
		"final_url":             lastPage.URL,
		"title":                 lastPage.Title,
		"queue_detected":        queueDetected,
		"browser_session":       session.Name,
		"browser_target_id":     target.ID,
		"requires_confirmation": true,
		"final_submit_clicked":  false,
		"payment_attempted":     false,
	}, "达到轮询次数或运行时长上限，暂未检测到票源，流程已退出。")
}

func resolveAgentTicketBrowserTarget(ctx context.Context, endpoint, sourceURL, targetID string) (agentBrowserTarget, error) {
	if strings.TrimSpace(targetID) != "" {
		target, found, err := findAgentBrowserTargetByID(ctx, endpoint, targetID)
		if err != nil {
			return agentBrowserTarget{}, err
		}
		if found && probeAgentBrowserTarget(ctx, target) == nil {
			return target, nil
		}
	}
	target, _, err := openAgentBrowserTarget(ctx, endpoint, sourceURL)
	return target, err
}

func parseAgentTicketGrabConfig(settings map[string]string) agentTicketGrabConfig {
	interval := parseAgentTicketDuration(settings["轮询间隔"], agentTicketDefaultInterval, agentTicketMinInterval, agentTicketMaxInterval)
	duration := parseAgentTicketDuration(settings["最长运行"], agentTicketDefaultDuration, 10*time.Second, agentTicketMaxDuration)
	return agentTicketGrabConfig{
		AvailabilityKeywords: splitAgentTicketKeywords(firstNonEmpty(settings["有票关键词"], "立即购买,立即预订,马上抢,选座购买,有票")),
		UnavailableKeywords:  splitAgentTicketKeywords(firstNonEmpty(settings["无票关键词"], "售罄,无票,缺票,暂时无票,已结束")),
		FilterKeywords:       splitAgentTicketKeywords(settings["场次/票档关键词"]),
		ClickLabels:          safeAgentTicketClickLabels(splitAgentTicketKeywords(firstNonEmpty(settings["购票按钮"], "立即购买,立即预订,马上抢,选座购买,去购票"))),
		Action:               firstNonEmpty(settings["找到后动作"], "进入确认页后停止"),
		Interval:             interval,
		MaxDuration:          duration,
		MaxAttempts:          parseAgentPositiveSetting(settings["最大轮询"], agentTicketDefaultAttempts, agentTicketMaxAttempts),
	}
}

func parseAgentTicketDuration(value string, fallback, minimum, maximum time.Duration) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return fallback
	}
	if duration < minimum {
		return minimum
	}
	if duration > maximum {
		return maximum
	}
	return duration
}

func splitAgentTicketKeywords(value string) []string {
	parts := strings.FieldsFunc(value, func(char rune) bool {
		switch char {
		case ',', '，', '、', ';', '；', '|', '\n', '\r':
			return true
		default:
			return false
		}
	})
	seen := map[string]bool{}
	keywords := make([]string, 0, len(parts))
	for _, part := range parts {
		keyword := strings.TrimSpace(part)
		key := strings.ToLower(keyword)
		if keyword == "" || seen[key] {
			continue
		}
		seen[key] = true
		keywords = append(keywords, keyword)
	}
	return keywords
}

func safeAgentTicketClickLabels(labels []string) []string {
	forbidden := []string{"提交订单", "确认订单", "确认支付", "立即支付", "付款", "支付订单", "确认付款"}
	safe := make([]string, 0, len(labels))
	for _, label := range labels {
		if containsAnyAgentTicketKeyword(label, forbidden) {
			continue
		}
		safe = append(safe, label)
	}
	return safe
}

func matchAgentTicketPage(page agentTicketPagePayload, config agentTicketGrabConfig) agentTicketMatch {
	pageText := strings.Join([]string{page.Title, page.Text}, "\n")
	for _, filter := range config.FilterKeywords {
		if !containsAgentTicketKeyword(pageText, filter) {
			return agentTicketMatch{}
		}
	}
	for _, action := range page.Actions {
		for _, keyword := range config.AvailabilityKeywords {
			if containsAgentTicketKeyword(action, keyword) {
				return agentTicketMatch{Found: true, Keyword: keyword, Action: action}
			}
		}
	}
	if containsAnyAgentTicketKeyword(pageText, config.UnavailableKeywords) {
		return agentTicketMatch{}
	}
	for _, keyword := range config.AvailabilityKeywords {
		if containsAgentTicketKeyword(pageText, keyword) {
			return agentTicketMatch{Found: true, Keyword: keyword}
		}
	}
	return agentTicketMatch{}
}

func containsAgentTicketKeyword(value, keyword string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(strings.TrimSpace(keyword))) && strings.TrimSpace(keyword) != ""
}

func containsAnyAgentTicketKeyword(value string, keywords []string) bool {
	for _, keyword := range keywords {
		if containsAgentTicketKeyword(value, keyword) {
			return true
		}
	}
	return false
}

func agentTicketPageNeedsManualVerification(page agentTicketPagePayload) bool {
	text := strings.Join([]string{page.Title, page.URL, page.Text}, "\n")
	return containsAnyAgentTicketKeyword(text, []string{"验证码", "人机验证", "安全验证", "滑块验证", "captcha"})
}

func agentTicketPageIsQueued(page agentTicketPagePayload) bool {
	text := strings.Join([]string{page.Title, page.Text}, "\n")
	return containsAnyAgentTicketKeyword(text, []string{"排队中", "正在排队", "队列中", "queue"})
}

func evaluateAgentTicketPage(ctx context.Context, conn *websocket.Conn, id int) (agentTicketPagePayload, error) {
	expression := `(function(){
		const clean = value => (value || '').replace(/\s+/g, ' ').trim();
		const visible = el => { const r = el.getBoundingClientRect(); const s = getComputedStyle(el); return r.width > 0 && r.height > 0 && s.visibility !== 'hidden' && s.display !== 'none'; };
		const disabled = el => el.disabled || el.getAttribute('aria-disabled') === 'true' || /disabled/i.test(String(el.className || ''));
		const actions = Array.from(document.querySelectorAll('a,button,[role="button"]')).filter(el => visible(el) && !disabled(el)).map(el => clean(el.innerText || el.textContent)).filter(Boolean);
		return JSON.stringify({title:document.title || '',url:location.href || '',text:document.body ? document.body.innerText : '',actions:Array.from(new Set(actions)).slice(0,200)});
	})()`
	var payload agentTicketPagePayload
	if err := evaluateAgentBrowserJSON(ctx, conn, id, expression, &payload); err != nil {
		return agentTicketPagePayload{}, err
	}
	return payload, nil
}

func clickAgentTicketEntry(ctx context.Context, conn *websocket.Conn, id int, labels []string) (agentTicketClickResult, error) {
	if len(labels) == 0 {
		return agentTicketClickResult{}, errors.New("没有可安全点击的购票入口关键词")
	}
	labelsJSON, _ := json.Marshal(labels)
	forbiddenJSON, _ := json.Marshal([]string{"提交订单", "确认订单", "确认支付", "立即支付", "付款", "支付订单", "确认付款"})
	expression := `(function(labels,forbidden){
		const clean = value => (value || '').replace(/\s+/g, ' ').trim();
		const visible = el => { const r = el.getBoundingClientRect(); const s = getComputedStyle(el); return r.width > 0 && r.height > 0 && s.visibility !== 'hidden' && s.display !== 'none'; };
		const disabled = el => el.disabled || el.getAttribute('aria-disabled') === 'true' || /disabled/i.test(String(el.className || ''));
		const candidates = Array.from(document.querySelectorAll('a,button,[role="button"]')).filter(el => visible(el) && !disabled(el));
		for (const label of labels) {
			const target = candidates.find(el => { const text = clean(el.innerText || el.textContent); return text.includes(label) && !forbidden.some(item => text.includes(item)); });
			if (target) { const text = clean(target.innerText || target.textContent); target.click(); return JSON.stringify({clicked:true,label:text}); }
		}
		return JSON.stringify({clicked:false,label:''});
	})(` + string(labelsJSON) + `,` + string(forbiddenJSON) + `)`
	var clicked agentTicketClickResult
	if err := evaluateAgentBrowserJSON(ctx, conn, id, expression, &clicked); err != nil {
		return agentTicketClickResult{}, err
	}
	return clicked, nil
}

func captureAgentTicketScreenshot(ctx context.Context, dataDir string, target agentBrowserTarget, pageURL string) string {
	screenshot, err := captureAgentBrowserTargetScreenshot(ctx, target)
	if err != nil {
		return ""
	}
	path, err := writeAgentBrowserScreenshot(dataDir, pageURL, time.Now(), screenshot)
	if err != nil {
		return ""
	}
	return path
}

func waitAgentTicketGrab(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func minAgentTicketDuration(left, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}

func failedAgentTicketResult(result agentBlueprintNodeResult, message, detail string) agentBlueprintNodeResult {
	result.Status = "failed"
	result.Message = message
	result.Error = detail
	result.Output = map[string]interface{}{"error": detail, "final_submit_clicked": false, "payment_attempted": false}
	return result
}

func completedAgentTicketResult(result agentBlueprintNodeResult, output map[string]interface{}, message string) agentBlueprintNodeResult {
	result.Status = "completed"
	result.Message = message
	result.Output = output
	return result
}
