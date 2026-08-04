package sunapi

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	agentOrderReportDefaultPages = 20
	agentOrderReportMaxPages     = 50
	agentOrderReportDefaultWait  = 4
)

var (
	agentOrderIDPattern          = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{2,}$`)
	agentOrderTimePattern        = regexp.MustCompile(`(\d{4})[-/.年](\d{1,2})[-/.月](\d{1,2})(?:日)?(?:\s+|T)?(\d{1,2}):?(\d{2}):?(\d{2})?`)
	agentOrderMoneyPattern       = regexp.MustCompile(`^[¥￥]\s*([0-9]+(?:,[0-9]{3})*(?:\.[0-9]+)?)$`)
	agentOrderInlineMoneyPattern = regexp.MustCompile(`[¥￥]\s*([0-9]+(?:,[0-9]{3})*(?:\.[0-9]+)?)`)
	agentOrderPlainMoneyPattern  = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)
	agentOrderQuantityPattern    = regexp.MustCompile(`^[×xX*]\s*(\d+)$`)
)

type agentOrderDateRange struct {
	Start     time.Time
	End       time.Time
	StartDate string
	EndDate   string
}

type agentOrderRecord struct {
	OrderID       string
	OrderedAt     time.Time
	OrderTime     string
	Status        string
	RefundStatus  string
	ProductName   string
	Specification string
	UnitPrice     float64
	Quantity      int
	Amount        float64
	BuyerNickname string
	Page          int
}

type agentOrderAccumulator struct {
	Orders              []agentOrderRecord
	Seen                map[string]bool
	RawCount            int
	DuplicateCount      int
	OutOfRangeCount     int
	StatusFilteredCount int
}

func newAgentOrderAccumulator() *agentOrderAccumulator {
	return &agentOrderAccumulator{Seen: map[string]bool{}, Orders: make([]agentOrderRecord, 0)}
}

func parseAgentOrderDateRange(settings map[string]string, now time.Time) (agentOrderDateRange, error) {
	if now.IsZero() {
		now = time.Now()
	}
	loc := now.Location()
	startRaw := firstNonEmpty(settings["开始日期"], settings["起始日期"], settings["日期开始"])
	endRaw := firstNonEmpty(settings["结束日期"], settings["截止日期"], settings["日期结束"])
	previousMonthStart := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, loc)
	previousMonthEnd := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc).Add(-time.Nanosecond)
	if strings.TrimSpace(startRaw) == "" {
		startRaw = previousMonthStart.Format("2006-01-02")
	}
	if strings.TrimSpace(endRaw) == "" {
		endRaw = previousMonthEnd.Format("2006-01-02")
	}
	start, err := parseAgentOrderDateSettingAt(startRaw, loc, false, now)
	if err != nil {
		return agentOrderDateRange{}, fmt.Errorf("开始日期无效: %w", err)
	}
	end, err := parseAgentOrderDateSettingAt(endRaw, loc, true, now)
	if err != nil {
		return agentOrderDateRange{}, fmt.Errorf("结束日期无效: %w", err)
	}
	if end.Before(start) {
		return agentOrderDateRange{}, errors.New("结束日期不能早于开始日期")
	}
	return agentOrderDateRange{
		Start: start, End: end,
		StartDate: start.Format("2006-01-02"), EndDate: end.Format("2006-01-02"),
	}, nil
}

func parseAgentOrderDateSetting(value string, loc *time.Location, endOfDay bool) (time.Time, error) {
	return parseAgentOrderDateSettingAt(value, loc, endOfDay, time.Now().In(loc))
}

func parseAgentOrderDateSettingAt(value string, loc *time.Location, endOfDay bool, reference time.Time) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, errors.New("日期不能为空")
	}
	now := reference.In(loc)
	switch value {
	case "今天", "今日":
		value = now.Format("2006-01-02")
	case "昨日", "昨天":
		value = now.AddDate(0, 0, -1).Format("2006-01-02")
	case "本月", "本月1日", "本月第一天":
		value = now.Format("2006-01") + "-01"
	}
	formats := []string{"2006-01-02", "2006/01/02", "2006.01.02", "2006-1-2", "2006/1/2", "2006年1月2日"}
	for _, format := range formats {
		if parsed, err := time.ParseInLocation(format, value, loc); err == nil {
			if endOfDay {
				return parsed.Add(24*time.Hour - time.Nanosecond), nil
			}
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("支持 YYYY-MM-DD 日期，收到 %q", value)
}

func parseAgentOrderPositiveSetting(value string, fallback, max int) int {
	parsed := parseAgentPositiveSetting(value, fallback, max)
	if parsed < 1 {
		parsed = fallback
	}
	if max > 0 && parsed > max {
		parsed = max
	}
	return parsed
}

func parseAgentOrderStatuses(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；' || r == '、' || r == '\n' || r == '\r'
	})
	statuses := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "全部" || strings.EqualFold(part, "all") {
			continue
		}
		part = canonicalAgentOrderStatus(part)
		if part != "" && !seen[part] {
			seen[part] = true
			statuses = append(statuses, part)
		}
	}
	return statuses
}

func canonicalAgentOrderStatus(value string) string {
	value = strings.TrimSpace(value)
	for _, status := range []string{"待付款", "待发货", "发货即将超时", "发货已超时", "已发货", "售后中", "交易成功", "交易关闭", "已取消", "退款中", "退款成功"} {
		if value == status || strings.Contains(value, status) {
			return status
		}
	}
	return value
}

func agentOrderStatusMatches(record agentOrderRecord, statuses []string) bool {
	if len(statuses) == 0 {
		return true
	}
	for _, wanted := range statuses {
		if canonicalAgentOrderStatus(record.Status) == wanted || canonicalAgentOrderStatus(record.RefundStatus) == wanted {
			return true
		}
	}
	return false
}

func agentOrderPageReachedBeforeStart(records []agentOrderRecord, start time.Time) bool {
	if start.IsZero() {
		return false
	}
	for _, record := range records {
		if !record.OrderedAt.IsZero() && record.OrderedAt.Before(start) {
			return true
		}
	}
	return false
}

func (acc *agentOrderAccumulator) AddPage(records []agentOrderRecord, page int, dateRange agentOrderDateRange, statuses []string) {
	for _, record := range records {
		acc.RawCount++
		record.Page = page
		if record.OrderID == "" {
			continue
		}
		if acc.Seen[record.OrderID] {
			acc.DuplicateCount++
			continue
		}
		acc.Seen[record.OrderID] = true
		if !record.OrderedAt.IsZero() && (record.OrderedAt.Before(dateRange.Start) || record.OrderedAt.After(dateRange.End)) {
			acc.OutOfRangeCount++
			continue
		}
		if !agentOrderStatusMatches(record, statuses) {
			acc.StatusFilteredCount++
			continue
		}
		acc.Orders = append(acc.Orders, record)
	}
}

func agentOrderRecordToMap(record agentOrderRecord) map[string]interface{} {
	row := map[string]interface{}{
		"订单编号": record.OrderID,
		"下单时间": record.OrderTime,
		"订单状态": record.Status,
		"退款状态": record.RefundStatus,
		"商品名称": record.ProductName,
		"规格":   record.Specification,
		"单价":   formatAgentOrderMoney(record.UnitPrice),
		"数量":   record.Quantity,
		"成交价":  formatAgentOrderMoney(record.Amount),
		"买家昵称": record.BuyerNickname,
	}
	if record.Page > 0 {
		row["采集页码"] = record.Page
	}
	return row
}

func formatAgentOrderMoney(value float64) string {
	if value == 0 {
		return "0.00"
	}
	return fmt.Sprintf("%.2f", value)
}

func buildAgentOrderStatusSummary(records []agentOrderRecord) []map[string]interface{} {
	type statusTotal struct {
		count  int
		amount float64
	}
	totals := map[string]statusTotal{}
	for _, record := range records {
		status := firstNonEmpty(record.Status, "未知")
		item := totals[status]
		item.count++
		item.amount += record.Amount
		totals[status] = item
	}
	order := []string{"交易成功", "已发货", "待发货", "待付款", "售后中", "交易关闭", "已取消", "未知"}
	for status := range totals {
		found := false
		for _, known := range order {
			if known == status {
				found = true
				break
			}
		}
		if !found {
			order = append(order, status)
		}
	}
	rows := make([]map[string]interface{}, 0, len(totals))
	for _, status := range order {
		item, ok := totals[status]
		if !ok {
			continue
		}
		rows = append(rows, map[string]interface{}{"订单状态": status, "订单数": item.count, "成交价合计": formatAgentOrderMoney(item.amount)})
	}
	return rows
}

func buildAgentOrderDataset(acc *agentOrderAccumulator, dateRange agentOrderDateRange, sourceURL, browserSession, stopReason string, pages int) map[string]interface{} {
	sort.SliceStable(acc.Orders, func(i, j int) bool { return acc.Orders[i].OrderedAt.After(acc.Orders[j].OrderedAt) })
	orders := make([]interface{}, 0, len(acc.Orders))
	for _, record := range acc.Orders {
		orders = append(orders, agentOrderRecordToMap(record))
	}
	statusRows := buildAgentOrderStatusSummary(acc.Orders)
	var allAmount, recognizedAmount, refundedOrderAmount float64
	statusCounts := map[string]int{}
	for _, record := range acc.Orders {
		allAmount += record.Amount
		statusCounts[firstNonEmpty(record.Status, "未知")]++
		if record.RefundStatus == "退款成功" {
			refundedOrderAmount += record.Amount
			continue
		}
		if record.Status == "已发货" || record.Status == "交易成功" {
			recognizedAmount += record.Amount
		}
	}
	summary := map[string]interface{}{
		"开始日期":               dateRange.StartDate,
		"结束日期":               dateRange.EndDate,
		"订单总数":               len(acc.Orders),
		"原始订单数":              acc.RawCount,
		"重复订单数":              acc.DuplicateCount,
		"日期范围外订单数":           acc.OutOfRangeCount,
		"状态过滤订单数":            acc.StatusFilteredCount,
		"采集页数":               pages,
		"停止原因":               stopReason,
		"订单成交价合计":            formatAgentOrderMoney(allAmount),
		"可计营业额":              formatAgentOrderMoney(recognizedAmount),
		"退款成功订单数":            countAgentOrderRefunds(acc.Orders),
		"退款关联成交额":            formatAgentOrderMoney(refundedOrderAmount),
		"退款金额口径":             "页面未提供实际退款金额；退款成功订单成交价仅作参考，不等同实际退款金额。",
		"数据来源":               sourceURL,
		"状态数量":               statusCounts,
		"order_count":        len(acc.Orders),
		"pages_collected":    pages,
		"start_date":         dateRange.StartDate,
		"end_date":           dateRange.EndDate,
		"recognized_revenue": formatAgentOrderMoney(recognizedAmount),
	}
	return map[string]interface{}{
		"summary":              summary,
		"orders":               orders,
		"order_status_summary": toAgentOrderInterfaceRows(statusRows),
		"source_url":           sourceURL,
		"browser_session":      browserSession,
		"start_date":           dateRange.StartDate,
		"end_date":             dateRange.EndDate,
		"pages_collected":      pages,
		"stop_reason":          stopReason,
	}
}

func countAgentOrderRefunds(records []agentOrderRecord) int {
	count := 0
	for _, record := range records {
		if record.RefundStatus == "退款成功" {
			count++
		}
	}
	return count
}

func toAgentOrderInterfaceRows(rows []map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	return out
}

func executeAgentOrderReportDataGraphNode(ctx context.Context, store *Store, node agentBlueprintGraphNode, upstream map[string]interface{}, result agentBlueprintNodeResult) agentBlueprintNodeResult {
	plan := findAgentReportPlan(upstream)
	if plan == nil {
		result.Status = "failed"
		result.Message = "订单数据管线没有收到生成报告节点输出的模板计划。"
		result.Error = "report plan is required"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	if store == nil || strings.TrimSpace(store.DataDir()) == "" {
		result.Status = "failed"
		result.Message = "订单数据管线无法访问浏览器数据目录。"
		result.Error = "data directory is not configured"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	dateRange, err := parseAgentOrderDateRange(node.Data.Settings, time.Now())
	if err != nil {
		result.Status = "failed"
		result.Message = "订单日期范围配置无效。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	sourceURL := firstNonEmpty(stringFromInterface(plan["source_url"]), findAgentStringRecursive(upstream, "final_url", "url"))
	if sourceURL == "" {
		result.Status = "failed"
		result.Message = "订单数据管线没有找到浏览器目标网址。"
		result.Error = "browser source url is required"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	waitSeconds := parseAgentDurationSeconds(node.Data.Settings["页面等待"], agentOrderReportDefaultWait)
	maxPages := parseAgentOrderPositiveSetting(node.Data.Settings["最大页数"], agentOrderReportDefaultPages, agentOrderReportMaxPages)
	statuses := parseAgentOrderStatuses(node.Data.Settings["订单状态"])
	browserSession := stringFromInterface(plan["browser_session"])
	browserTargetID := stringFromInterface(plan["browser_target_id"])
	dataset, err := collectAgentOrderBrowserData(ctx, store.DataDir(), sourceURL, dateRange, statuses, waitSeconds, maxPages, browserSession, browserTargetID)
	if err != nil {
		result.Status = "failed"
		result.Message = "订单管理页采集失败。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error, "source_url": sourceURL}
		return result
	}
	reportNode := reportNodeFromPlan(plan)
	content := buildAgentOrderReportContent(reportNode, dataset)
	files := buildAgentOrderReportFiles(reportNode, content, dataset)
	writeResult, err := writeAgentReportFiles(store.DataDir(), AgentReportWriteRequest{
		Title:           firstNonEmpty(reportNode.Data.Title, "订单报告"),
		OutputDirectory: firstNonEmpty(reportNode.Data.Settings["输出目录"], "data/output/reports"),
		Files:           files,
	})
	if err != nil {
		result.Status = "failed"
		result.Message = "订单数据已采集，但报告文件写入失败。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error, "dataset": dataset}
		return result
	}
	orders, _ := dataset["orders"].([]interface{})
	summary, _ := dataset["summary"].(map[string]interface{})
	result.Message = fmt.Sprintf("订单数据管线完成：采集 %d 条订单，报告文件已写入。", len(orders))
	result.ArchivePath = writeResult.OutputDirectory
	result.Output = map[string]interface{}{
		"dataset":          dataset,
		"summary":          summary,
		"orders":           orders,
		"content":          content,
		"output_directory": writeResult.OutputDirectory,
		"files":            writeResult.Files,
	}
	return result
}

func resolveAgentOrderManageURL(sourceURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(sourceURL))
	if err != nil || parsed.Host == "" || !strings.Contains(strings.ToLower(parsed.Host), "goofish.com") {
		return sourceURL
	}
	if strings.Contains(strings.ToLower(parsed.Fragment), "seller-trade/order-manage") {
		return parsed.String()
	}
	if parsed.RawQuery == "" {
		parsed.RawQuery = "site=COMMONPRO"
	}
	parsed.Path = "/"
	parsed.Fragment = "/seller-trade/order-manage"
	return parsed.String()
}

func collectAgentOrderBrowserData(ctx context.Context, dataDir, sourceURL string, dateRange agentOrderDateRange, statuses []string, waitSeconds, maxPages int, browserSession, browserTargetID string) (map[string]interface{}, error) {
	endpoint, err := ensureAgentBrowserDevtoolsForSession(ctx, resolveAgentBrowserSessionPaths(dataDir, browserSession))
	if err != nil {
		return nil, err
	}
	targetURL := resolveAgentOrderManageURL(sourceURL)
	var target agentBrowserTarget
	found := false
	if strings.TrimSpace(browserTargetID) != "" {
		target, found, err = findAgentBrowserTargetByID(ctx, endpoint, browserTargetID)
		if err != nil {
			return nil, err
		}
		if found && probeAgentBrowserTarget(ctx, target) != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = closeAgentBrowserTarget(cleanupCtx, endpoint, target.ID)
			cancel()
			found = false
		}
	}
	createdTarget := false
	if !found {
		var reused bool
		target, reused, err = openAgentBrowserTarget(ctx, endpoint, targetURL)
		if err != nil {
			return nil, err
		}
		createdTarget = !reused
	}
	if createdTarget {
		defer func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = closeAgentBrowserTarget(cleanupCtx, endpoint, target.ID)
		}()
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, target.WebSocketDebuggerURL, nil)
	if err != nil {
		return nil, fmt.Errorf("连接订单管理页浏览器失败: %w", err)
	}
	defer conn.Close()
	if _, err = agentBrowserCDPCall(ctx, conn, 1, "Page.enable", nil); err != nil {
		return nil, err
	}
	if _, err = agentBrowserCDPCall(ctx, conn, 2, "Runtime.enable", nil); err != nil {
		return nil, err
	}
	if _, err = agentBrowserCDPCall(ctx, conn, 3, "Page.navigate", map[string]any{"url": targetURL}); err != nil {
		return nil, err
	}
	if err = waitAgentReportBrowser(ctx, waitSeconds); err != nil {
		return nil, err
	}

	acc := newAgentOrderAccumulator()
	stopReason := "max_pages"
	pages := 0
	var firstPayload agentBrowserTablePayload
	for page := 1; page <= maxPages; page++ {
		payload, captureErr := evaluateAgentBrowserTables(ctx, conn, 1000+page*10)
		if captureErr != nil {
			return nil, captureErr
		}
		if page == 1 {
			firstPayload = payload
		}
		if strings.TrimSpace(payload.Text) == "" || looksLikeAgentBrowserLogin(agentBrowserTextPayload{Title: payload.Title, URL: payload.URL, Text: payload.Text}) {
			return nil, errors.New("浏览器登录会话无效；请在打开的浏览器窗口完成登录后重试")
		}
		pageRecords := parseAgentOrderPageText(payload.Text)
		if len(pageRecords) == 0 {
			pageRecords = parseAgentOrderTables(payload.Tables)
		}
		pages = page
		newOrderCount := countAgentOrderUnseenIDs(pageRecords, acc.Seen)
		acc.AddPage(pageRecords, page, dateRange, statuses)
		if page > 1 && newOrderCount == 0 {
			stopReason = "no_new_orders"
			break
		}
		if agentOrderPageReachedBeforeStart(pageRecords, dateRange.Start) {
			stopReason = "reached_before_start_date"
			break
		}
		if page == maxPages {
			break
		}
		next, nextErr := clickAgentBrowserNextPage(ctx, conn, 1001+page*10)
		if nextErr != nil {
			return nil, nextErr
		}
		if !next {
			stopReason = "no_next_page"
			break
		}
		if err := waitAgentReportBrowser(ctx, waitSeconds); err != nil {
			return nil, err
		}
	}
	if acc.RawCount == 0 {
		return nil, errors.New("订单管理页未找到可解析订单；请确认登录状态、页面地址和页面已加载完成")
	}
	capturedAt := time.Now()
	screenshotPath := ""
	if screenshot, screenshotErr := captureAgentBrowserTargetScreenshot(ctx, target); screenshotErr == nil {
		screenshotPath, _ = writeAgentBrowserScreenshot(dataDir, firstPayload.URL, capturedAt, screenshot)
	}
	dataset := buildAgentOrderDataset(acc, dateRange, targetURL, browserSession, stopReason, pages)
	summary, _ := dataset["summary"].(map[string]interface{})
	summary["final_url"] = firstPayload.URL
	summary["title"] = firstPayload.Title
	summary["screenshot_path"] = screenshotPath
	summary["captured_at"] = capturedAt.Unix()
	dataset["summary"] = summary
	return dataset, nil
}

func countAgentOrderUnseenIDs(records []agentOrderRecord, seen map[string]bool) int {
	if len(records) == 0 {
		return 0
	}
	unique := make(map[string]bool, len(records))
	for _, record := range records {
		orderID := strings.TrimSpace(record.OrderID)
		if orderID == "" || seen[orderID] || unique[orderID] {
			continue
		}
		unique[orderID] = true
	}
	return len(unique)
}

func parseAgentOrderPageText(text string) []agentOrderRecord {
	lines := agentPageTextLines(text)
	markers := make([]int, 0)
	for index, line := range lines {
		if line == "订单编号" {
			markers = append(markers, index)
		}
	}
	orders := make([]agentOrderRecord, 0, len(markers))
	for markerIndex, marker := range markers {
		end := len(lines)
		if markerIndex+1 < len(markers) {
			end = markers[markerIndex+1]
		}
		if marker+1 >= end {
			continue
		}
		orderIDIndex := -1
		for index := marker + 1; index < end && index <= marker+5; index++ {
			if agentOrderIDPattern.MatchString(lines[index]) {
				orderIDIndex = index
				break
			}
		}
		if orderIDIndex < 0 {
			continue
		}
		orderID := lines[orderIDIndex]
		timeIndex := -1
		var orderedAt time.Time
		for index := orderIDIndex + 1; index < end; index++ {
			if parsed, ok := parseAgentOrderTime(lines[index]); ok {
				timeIndex, orderedAt = index, parsed
				break
			}
		}
		if timeIndex < 0 {
			continue
		}
		record := agentOrderRecord{OrderID: orderID, OrderedAt: orderedAt, OrderTime: orderedAt.Format("2006-01-02 15:04:05"), Quantity: 1}
		var money []float64
		for index := timeIndex + 1; index < end; index++ {
			line := strings.TrimSpace(lines[index])
			if status := canonicalAgentOrderStatus(line); isAgentOrderStatus(status) {
				if record.Status == "" && status != "退款成功" {
					record.Status = status
				}
			}
			if refundStatus := canonicalAgentOrderStatus(line); isAgentOrderRefundStatus(refundStatus) {
				record.RefundStatus = firstNonEmpty(record.RefundStatus, refundStatus)
			}
			if strings.HasPrefix(line, "规格") {
				record.Specification = strings.TrimSpace(strings.TrimPrefix(line, "规格"))
			}
			if quantity := parseAgentOrderQuantity(line); quantity > 0 {
				record.Quantity = quantity
			}
			if line == "¥" || line == "￥" {
				if index+1 < end {
					if value, ok := parseAgentOrderMoneyValue(lines[index+1]); ok {
						money = append(money, value)
					}
				}
			} else if value, ok := parseAgentOrderMoney(line); ok {
				money = append(money, value)
			}
			if record.ProductName == "" && isAgentOrderProductLine(line) {
				record.ProductName = line
			}
			if strings.EqualFold(line, "联系ta") || strings.EqualFold(line, "联系TA") {
				if index > timeIndex {
					candidate := strings.TrimSpace(lines[index-1])
					if isAgentOrderBuyerCandidate(candidate) {
						record.BuyerNickname = candidate
					}
				}
			}
		}
		if len(money) > 0 {
			record.UnitPrice = money[0]
			record.Amount = money[len(money)-1]
		}
		if record.Status == "" && marker > 0 {
			record.Status = canonicalAgentOrderStatus(lines[marker-1])
		}
		orders = append(orders, record)
	}
	return orders
}

func parseAgentOrderTables(tables []agentBrowserTable) []agentOrderRecord {
	normalized := normalizeAgentBrowserTables(tables)
	best := agentBrowserTable{}
	bestScore := 0
	for _, table := range normalized {
		headers := strings.Join(table.Headers, " ")
		score := 0
		for _, marker := range []string{"订单", "商品", "状态", "单价", "成交价", "买家", "下单时间"} {
			if strings.Contains(headers, marker) {
				score++
			}
		}
		if score > bestScore {
			bestScore, best = score, table
		}
	}
	if len(best.Rows) == 0 {
		return nil
	}
	orders := make([]agentOrderRecord, 0, len(best.Rows))
	for _, row := range best.Rows {
		text := strings.Join(row, "\n")
		orders = append(orders, parseAgentOrderPageText("订单编号\n"+text)...)
	}
	return orders
}

func parseAgentOrderTime(value string) (time.Time, bool) {
	matches := agentOrderTimePattern.FindStringSubmatch(value)
	if len(matches) < 6 {
		return time.Time{}, false
	}
	second := matches[6]
	if second == "" {
		second = "00"
	}
	parsed, err := time.ParseInLocation("2006-1-2 15:4:5", fmt.Sprintf("%s-%s-%s %s:%s:%s", matches[1], matches[2], matches[3], matches[4], matches[5], second), time.Local)
	return parsed, err == nil
}

func parseAgentOrderMoney(value string) (float64, bool) {
	value = strings.TrimSpace(strings.ReplaceAll(value, ",", ""))
	match := agentOrderMoneyPattern.FindStringSubmatch(value)
	if len(match) < 2 {
		match = agentOrderInlineMoneyPattern.FindStringSubmatch(value)
	}
	if len(match) < 2 {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(match[1], 64)
	return parsed, err == nil
}

func parseAgentOrderMoneyValue(value string) (float64, bool) {
	value = strings.TrimSpace(strings.ReplaceAll(value, ",", ""))
	if parsed, ok := parseAgentOrderMoney(value); ok {
		return parsed, true
	}
	if !agentOrderPlainMoneyPattern.MatchString(value) {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(value, 64)
	return parsed, err == nil
}

func parseAgentOrderQuantity(value string) int {
	match := agentOrderQuantityPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) < 2 {
		return 0
	}
	parsed, _ := strconv.Atoi(match[1])
	return parsed
}

func isAgentOrderStatus(value string) bool {
	for _, status := range []string{"待付款", "待发货", "发货即将超时", "发货已超时", "已发货", "售后中", "交易成功", "交易关闭", "已取消"} {
		if value == status {
			return true
		}
	}
	return false
}

func isAgentOrderRefundStatus(value string) bool {
	return value == "退款中" || value == "退款成功"
}

func isAgentOrderProductLine(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "¥" || value == "￥" || value == "包邮" || value == "添加备注" || value == "订单编号" || value == "联系ta" || value == "联系TA" || isAgentOrderStatus(canonicalAgentOrderStatus(value)) || strings.Contains(value, "退款") || strings.HasPrefix(value, "规格") || strings.HasPrefix(value, "描述") || strings.HasPrefix(value, "下单时间") || strings.HasPrefix(value, "查看") || strings.HasPrefix(value, "提醒") || strings.HasPrefix(value, "修改") || strings.HasPrefix(value, "取消") {
		return false
	}
	if agentOrderIDPattern.MatchString(value) || parseAgentOrderMoneyValueOnly(value) || parseAgentOrderQuantity(value) > 0 {
		return false
	}
	return true
}

func parseAgentOrderMoneyValueOnly(value string) bool {
	_, ok := parseAgentOrderMoneyValue(value)
	return ok
}

func isAgentOrderBuyerCandidate(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "包邮" || strings.Contains(value, "订单") || strings.Contains(value, "查看") || strings.Contains(value, "提醒") || strings.Contains(value, "退款") || isAgentOrderStatus(canonicalAgentOrderStatus(value)) {
		return false
	}
	if agentOrderIDPattern.MatchString(value) || parseAgentOrderMoneyValueOnly(value) {
		return false
	}
	return true
}

func buildAgentOrderReportContent(node agentBlueprintGraphNode, dataset map[string]interface{}) string {
	title := firstNonEmpty(node.Data.Title, "订单报告")
	summary, _ := dataset["summary"].(map[string]interface{})
	orders, _ := dataset["orders"].([]interface{})
	statusRows, _ := dataset["order_status_summary"].([]interface{})
	var sb strings.Builder
	sb.WriteString("# " + title + "\n\n")
	sb.WriteString("## 统计范围\n\n")
	sb.WriteString(fmt.Sprintf("开始日期：%v\n\n结束日期：%v\n\n数据来源：%v\n\n", summary["开始日期"], summary["结束日期"], summary["数据来源"]))
	sb.WriteString("## 汇总\n\n| 指标 | 数值 |\n| --- | --- |\n")
	for _, key := range []string{"订单总数", "原始订单数", "重复订单数", "日期范围外订单数", "状态过滤订单数", "采集页数", "订单成交价合计", "可计营业额", "退款成功订单数", "退款关联成交额"} {
		sb.WriteString(fmt.Sprintf("| %s | %s |\n", key, escapeMarkdownCell(fmt.Sprint(summary[key]))))
	}
	sb.WriteString(fmt.Sprintf("\n口径说明：%s\n\n", summary["退款金额口径"]))
	sb.WriteString("## 状态汇总\n\n| 订单状态 | 订单数 | 成交价合计 |\n| --- | --- | --- |\n")
	for _, raw := range statusRows {
		if row, ok := raw.(map[string]interface{}); ok {
			sb.WriteString(fmt.Sprintf("| %s | %v | %s |\n", escapeMarkdownCell(fmt.Sprint(row["订单状态"])), row["订单数"], escapeMarkdownCell(fmt.Sprint(row["成交价合计"]))))
		}
	}
	sb.WriteString("\n## 订单明细\n\n")
	sb.WriteString(agentOrderDetailsMarkdown(orders))
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

func agentOrderDetailsMarkdown(orders []interface{}) string {
	headers := []string{"订单编号", "下单时间", "订单状态", "退款状态", "商品名称", "规格", "单价", "数量", "成交价", "买家昵称", "采集页码"}
	var sb strings.Builder
	sb.WriteString("| " + strings.Join(headers, " | ") + " |\n| " + strings.Repeat("--- | ", len(headers)-1) + "--- |\n")
	for _, raw := range orders {
		row, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		cells := make([]string, 0, len(headers))
		for _, header := range headers {
			cells = append(cells, escapeMarkdownCell(fmt.Sprint(row[header])))
		}
		sb.WriteString("| " + strings.Join(cells, " | ") + " |\n")
	}
	if len(orders) == 0 {
		sb.WriteString("| 暂无符合条件的订单 | | | | | | | | | | |\n")
	}
	return sb.String()
}

func buildAgentOrderCSV(orders []interface{}) string {
	var buf bytes.Buffer
	buf.WriteString("\ufeff")
	writer := csv.NewWriter(&buf)
	headers := []string{"订单编号", "下单时间", "订单状态", "退款状态", "商品名称", "规格", "单价", "数量", "成交价", "买家昵称", "采集页码"}
	_ = writer.Write(headers)
	for _, raw := range orders {
		row, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		values := make([]string, 0, len(headers))
		for _, header := range headers {
			values = append(values, fmt.Sprint(row[header]))
		}
		_ = writer.Write(values)
	}
	writer.Flush()
	return buf.String()
}

func buildAgentOrderReportFiles(node agentBlueprintGraphNode, content string, dataset map[string]interface{}) []AgentReportOutputFile {
	bindingValue := firstNonEmpty(node.Data.Settings["模板文件"], node.Data.Settings["网页模板"])
	if len(parseAgentReportTemplateBindings(bindingValue, node.Data.Settings["模板原文件名"])) > 0 {
		return agentGraphReportFiles(node, content, dataset)
	}
	orders, _ := dataset["orders"].([]interface{})
	return []AgentReportOutputFile{
		{Label: "Excel 订单报告", Extension: "xlsx", Content: content, Worksheets: buildAgentOrderWorkbookSheets(dataset)},
		{Label: "CSV 订单明细", Extension: "csv", Content: buildAgentOrderCSV(orders)},
	}
}

func buildAgentOrderWorkbookSheets(dataset map[string]interface{}) []AgentReportWorksheet {
	summary := agentOrderMapValue(dataset["summary"])
	orders := agentOrderRowsAsStrings(dataset["orders"])
	statusRows := agentOrderRowsAsStrings(dataset["order_status_summary"])

	summaryRows := [][]string{
		{"指标", "数值"},
		{"统计范围", fmt.Sprintf("%s 至 %s", summary["开始日期"], summary["结束日期"])},
		{"订单总数", summary["订单总数"]},
		{"订单成交价合计", summary["订单成交价合计"]},
		{"可计营业额", summary["可计营业额"]},
		{"退款成功订单数", summary["退款成功订单数"]},
		{"退款关联成交额", firstNonEmpty(summary["退款关联成交额"], summary["退款成功订单成交价参考"])},
		{"原始订单数", summary["原始订单数"]},
		{"重复订单数", summary["重复订单数"]},
		{"日期范围外订单数", summary["日期范围外订单数"]},
		{"状态过滤订单数", summary["状态过滤订单数"]},
		{"采集页数", summary["采集页数"]},
		{"停止原因", summary["停止原因"]},
		{"数据来源", summary["数据来源"]},
		{"退款金额口径", summary["退款金额口径"]},
	}

	orderHeaders := []string{"订单编号", "下单时间", "订单状态", "退款状态", "商品名称", "规格", "单价", "数量", "成交价", "买家昵称", "采集页码"}
	orderSheetRows := [][]string{orderHeaders}
	for _, row := range orders {
		orderSheetRows = append(orderSheetRows, agentOrderRowByHeaders(row, orderHeaders))
	}
	if len(orderSheetRows) == 1 {
		orderSheetRows = append(orderSheetRows, []string{"暂无符合条件的订单"})
	}

	statusHeaders := []string{"订单状态", "订单数", "成交价合计"}
	statusSheetRows := [][]string{statusHeaders}
	for _, row := range statusRows {
		statusSheetRows = append(statusSheetRows, agentOrderRowByHeaders(row, statusHeaders))
	}
	if len(statusSheetRows) == 1 {
		statusSheetRows = append(statusSheetRows, []string{"暂无数据"})
	}

	notesRows := [][]string{
		{"说明", "内容"},
		{"采集方式", "复用已登录浏览器会话，从订单管理页分页采集订单卡片。"},
		{"去重规则", "按订单编号去重；重复订单不会重复计入金额。"},
		{"日期规则", "按下单时间筛选开始日期和结束日期（含当天 23:59:59）。"},
		{"可计营业额", "仅统计订单状态为已发货或交易成功且未标记退款成功的订单成交价。"},
		{"退款金额", "页面未提供实际退款金额；退款成功订单成交价仅作参考，不等同实际退款金额。"},
	}
	return []AgentReportWorksheet{
		{Name: "营业额总览", Rows: summaryRows},
		{Name: "订单明细", Rows: orderSheetRows},
		{Name: "状态汇总", Rows: statusSheetRows},
		{Name: "数据说明", Rows: notesRows},
	}
}

func agentOrderMapValue(value interface{}) map[string]string {
	out := map[string]string{}
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, raw := range typed {
			out[key] = agentOrderCellString(raw)
		}
	case map[string]string:
		for key, raw := range typed {
			out[key] = raw
		}
	}
	return out
}

func agentOrderRowsAsStrings(value interface{}) []map[string]string {
	rows := []map[string]string{}
	switch typed := value.(type) {
	case []interface{}:
		for _, raw := range typed {
			if row := agentOrderMapValue(raw); len(row) > 0 {
				rows = append(rows, row)
			}
		}
	case []map[string]interface{}:
		for _, raw := range typed {
			if row := agentOrderMapValue(raw); len(row) > 0 {
				rows = append(rows, row)
			}
		}
	case []map[string]string:
		rows = append(rows, typed...)
	}
	return rows
}

func agentOrderRowByHeaders(row map[string]string, headers []string) []string {
	values := make([]string, 0, len(headers))
	for _, header := range headers {
		values = append(values, row[header])
	}
	return values
}

func agentOrderCellString(value interface{}) string {
	if value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "<nil>" {
		return ""
	}
	return text
}
