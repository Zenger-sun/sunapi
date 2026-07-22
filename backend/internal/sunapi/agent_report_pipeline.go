package sunapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	agentReportDataDefaultPages = 10
	agentReportDataMaxPages     = 50
)

type agentBrowserTable struct {
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
}

type agentBrowserTablePayload struct {
	Title  string              `json:"title"`
	URL    string              `json:"url"`
	Text   string              `json:"text"`
	Tables []agentBrowserTable `json:"tables"`
}

func buildAgentReportPlan(node agentBlueprintGraphNode, upstream map[string]interface{}) map[string]interface{} {
	preview := strings.TrimSpace(node.Data.Settings["模板预览"])
	requiredFields := make([]string, 0)
	for _, field := range []string{
		"商品ID", "商品标题", "价格", "曝光次数", "曝光人数", "浏览次数", "浏览人数",
		"点击率", "询单人数", "支付人数", "支付订单数", "支付金额", "浏览支付转化率",
	} {
		if strings.Contains(preview, field) {
			requiredFields = append(requiredFields, field)
		}
	}
	requiresProducts := strings.Contains(preview, "商品明细") ||
		strings.Contains(preview, "商品排行") ||
		strings.Contains(preview, "商品ID") ||
		strings.Contains(preview, "商品标题")

	plan := map[string]interface{}{
		"title":                  firstNonEmpty(node.Data.Title, "生成日报"),
		"description":            node.Data.Description,
		"template_file":          node.Data.Settings["模板文件"],
		"template_original_name": node.Data.Settings["模板原文件名"],
		"template_preview":       preview,
		"output_format":          node.Data.Settings["输出格式"],
		"output_directory":       firstNonEmpty(node.Data.Settings["输出目录"], "data/output/reports"),
		"source_url":             findAgentStringRecursive(upstream, "final_url", "url"),
		"browser_session":        findAgentStringRecursive(upstream, "browser_session"),
		"browser_target_id":      findAgentStringRecursive(upstream, "browser_target_id"),
		"required_fields":        requiredFields,
		"requires_products":      requiresProducts,
		"period":                 "近1天",
	}
	return map[string]interface{}{
		"mode":        "report_plan",
		"report_plan": plan,
		"source":      upstream,
	}
}

func executeAgentReportDataGraphNode(
	ctx context.Context,
	store *Store,
	node agentBlueprintGraphNode,
	upstream map[string]interface{},
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	plan := findAgentReportPlan(upstream)
	if plan == nil {
		result.Status = "failed"
		result.Message = "处理数据节点没有收到生成日报节点输出的模板计划。"
		result.Error = "report plan is required"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}
	if store == nil || strings.TrimSpace(store.DataDir()) == "" {
		result.Status = "failed"
		result.Message = "处理数据节点无法访问浏览器数据目录。"
		result.Error = "data directory is not configured"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}

	sourceURL := stringFromInterface(plan["source_url"])
	if sourceURL == "" {
		sourceURL = findAgentStringRecursive(upstream, "final_url", "url")
	}
	if sourceURL == "" {
		result.Status = "failed"
		result.Message = "处理数据节点没有找到浏览器会话的目标网址。"
		result.Error = "browser source url is required"
		result.Output = map[string]interface{}{"error": result.Error}
		return result
	}

	waitSeconds := parseAgentDurationSeconds(node.Data.Settings["页面等待"], 4)
	maxPages := parseAgentPositiveSetting(node.Data.Settings["最大页数"], agentReportDataDefaultPages, agentReportDataMaxPages)
	period := firstNonEmpty(node.Data.Settings["采集周期"], stringFromInterface(plan["period"]), "近1天")
	requiredFields := agentStringSlice(plan["required_fields"])
	browserSession := stringFromInterface(plan["browser_session"])
	browserTargetID := stringFromInterface(plan["browser_target_id"])
	dataset, err := collectAgentReportBrowserData(ctx, store.DataDir(), sourceURL, period, waitSeconds, maxPages, requiredFields, browserSession, browserTargetID)
	if err != nil {
		result.Status = "failed"
		result.Message = "处理数据节点重新采集浏览器数据失败。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error, "source_url": sourceURL}
		return result
	}

	requiresProducts, _ := plan["requires_products"].(bool)
	products, _ := dataset["products"].([]interface{})
	if requiresProducts && len(products) == 0 {
		result.Status = "failed"
		result.Message = "日报模板需要商品明细，但商品数据页没有采集到可解析商品行。"
		result.Error = "required product rows were not collected"
		result.Output = map[string]interface{}{
			"error":      result.Error,
			"source_url": sourceURL,
			"dataset":    dataset,
		}
		return result
	}

	reportNode := reportNodeFromPlan(plan)
	content := buildAgentGraphReportContent(reportNode, map[string]interface{}{"dataset": dataset})
	files := agentGraphReportFiles(reportNode, content, dataset)
	writeResult, err := writeAgentReportFiles(store.DataDir(), AgentReportWriteRequest{
		Title:           firstNonEmpty(reportNode.Data.Title, "生成日报"),
		OutputDirectory: firstNonEmpty(reportNode.Data.Settings["输出目录"], "data/output/reports"),
		Files:           files,
	})
	if err != nil {
		result.Status = "failed"
		result.Message = "处理数据完成，但日报文件写入失败。"
		result.Error = err.Error()
		result.Output = map[string]interface{}{"error": result.Error, "dataset": dataset}
		return result
	}

	result.Message = fmt.Sprintf("重新采集并处理 %d 条商品数据，日报文件已写入。", len(products))
	result.ArchivePath = writeResult.OutputDirectory
	result.Output = map[string]interface{}{
		"dataset":          dataset,
		"summary":          dataset["summary"],
		"products":         dataset["products"],
		"rankings":         dataset["rankings"],
		"content":          content,
		"output_directory": writeResult.OutputDirectory,
		"files":            writeResult.Files,
	}
	return result
}

func reportNodeFromPlan(plan map[string]interface{}) agentBlueprintGraphNode {
	var node agentBlueprintGraphNode
	node.ID = "report-plan"
	node.Data.Kind = "report"
	node.Data.ComponentID = "component.report"
	node.Data.Title = firstNonEmpty(stringFromInterface(plan["title"]), "生成日报")
	node.Data.Description = stringFromInterface(plan["description"])
	node.Data.Settings = map[string]string{
		"模板文件":   stringFromInterface(plan["template_file"]),
		"模板原文件名": stringFromInterface(plan["template_original_name"]),
		"模板预览":   stringFromInterface(plan["template_preview"]),
		"输出格式":   stringFromInterface(plan["output_format"]),
		"输出目录":   firstNonEmpty(stringFromInterface(plan["output_directory"]), "data/output/reports"),
	}
	return node
}

func findAgentReportPlan(value interface{}) map[string]interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		if plan, ok := typed["report_plan"].(map[string]interface{}); ok {
			return plan
		}
		for _, child := range typed {
			if plan := findAgentReportPlan(child); plan != nil {
				return plan
			}
		}
	case []interface{}:
		for _, child := range typed {
			if plan := findAgentReportPlan(child); plan != nil {
				return plan
			}
		}
	}
	return nil
}

func findAgentStringRecursive(value interface{}, keys ...string) string {
	wanted := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		wanted[key] = struct{}{}
	}
	var walk func(interface{}) string
	walk = func(current interface{}) string {
		switch typed := current.(type) {
		case map[string]interface{}:
			for _, key := range keys {
				if raw, ok := typed[key]; ok {
					if value := stringFromInterface(raw); value != "" && value != "<nil>" {
						return value
					}
				}
			}
			for key, child := range typed {
				if _, skip := wanted[key]; skip {
					continue
				}
				if value := walk(child); value != "" {
					return value
				}
			}
		case []interface{}:
			for _, child := range typed {
				if value := walk(child); value != "" {
					return value
				}
			}
		}
		return ""
	}
	return walk(value)
}

func collectAgentReportBrowserData(
	ctx context.Context,
	dataDir string,
	sourceURL string,
	period string,
	waitSeconds int,
	maxPages int,
	requiredFields []string,
	browserSession string,
	browserTargetID string,
) (map[string]interface{}, error) {
	endpoint, err := ensureAgentBrowserDevtoolsForSession(ctx, resolveAgentBrowserSessionPaths(dataDir, browserSession))
	if err != nil {
		return nil, err
	}
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
		var reusedTarget bool
		target, reusedTarget, err = openAgentBrowserTarget(ctx, endpoint, sourceURL)
		if err != nil {
			return nil, err
		}
		createdTarget = !reusedTarget
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
		return nil, fmt.Errorf("连接商品数据浏览器页面失败: %w", err)
	}
	defer conn.Close()

	if _, err := agentBrowserCDPCall(ctx, conn, 1, "Page.enable", nil); err != nil {
		return nil, err
	}
	if _, err := agentBrowserCDPCall(ctx, conn, 2, "Runtime.enable", nil); err != nil {
		return nil, err
	}
	if _, err := agentBrowserCDPCall(ctx, conn, 3, "Page.navigate", map[string]any{"url": sourceURL}); err != nil {
		return nil, err
	}
	if err := waitAgentReportBrowser(ctx, waitSeconds); err != nil {
		return nil, err
	}
	overview, err := evaluateAgentBrowserText(ctx, conn, 4)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(overview.Text) == "" || looksLikeAgentBrowserLogin(overview) {
		return nil, errors.New("浏览器登录会话无效；请在打开的浏览器窗口完成登录后重试")
	}
	capturedAt := time.Now()
	screenshotPath := ""
	if screenshot, screenshotErr := captureAgentBrowserTargetScreenshot(ctx, target); screenshotErr == nil {
		screenshotPath, _ = writeAgentBrowserScreenshot(dataDir, overview.URL, capturedAt, screenshot)
	}

	clicked, err := clickAgentBrowserLabel(ctx, conn, 10, []string{"商品数据"})
	if err != nil {
		return nil, err
	}
	if !clicked {
		return nil, errors.New("当前登录页面未找到“商品数据”入口")
	}
	if err := waitAgentReportBrowser(ctx, waitSeconds); err != nil {
		return nil, err
	}
	_, _ = clickAgentBrowserLabel(ctx, conn, 11, []string{period})
	if err := waitAgentReportBrowser(ctx, 1); err != nil {
		return nil, err
	}

	products := make([]map[string]interface{}, 0)
	seen := map[string]bool{}
	extractionStrategies := map[string]bool{}
	productPageURL := ""
	productPageText := ""
	productScreenshotPath := ""
	for page := 0; page < maxPages; page++ {
		payload, captureErr := evaluateAgentBrowserTables(ctx, conn, 100+page*3)
		if captureErr != nil {
			return nil, captureErr
		}
		productPageURL = payload.URL
		productPageText = payload.Text
		table := bestAgentProductTable(payload.Tables)
		pageRows := agentProductRowsFromTable(table)
		if len(pageRows) > 0 {
			extractionStrategies["dom_table"] = true
		} else {
			pageRows = agentRowsFromSchemaText(payload.Text, requiredFields)
			if len(pageRows) > 0 {
				extractionStrategies["schema_text"] = true
			}
		}
		for _, row := range pageRows {
			expandAgentCompositeRow(row, requiredFields)
			key := agentProductRowKey(row)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			products = append(products, row)
		}
		if productScreenshotPath == "" {
			if screenshot, screenshotErr := captureAgentBrowserTargetScreenshot(ctx, target); screenshotErr == nil {
				productScreenshotPath, _ = writeAgentBrowserScreenshot(dataDir, payload.URL, time.Now(), screenshot)
			}
		}

		next, nextErr := clickAgentBrowserNextPage(ctx, conn, 101+page*3)
		if nextErr != nil {
			return nil, nextErr
		}
		if !next {
			break
		}
		if err := waitAgentReportBrowser(ctx, 2); err != nil {
			return nil, err
		}
	}

	productValues := make([]interface{}, 0, len(products))
	for _, product := range products {
		enrichAgentProductRow(product)
		productValues = append(productValues, product)
	}
	rankingRows := buildAgentProductRankings(products)
	rankingValues := make([]interface{}, 0, len(rankingRows))
	for _, ranking := range rankingRows {
		rankingValues = append(rankingValues, ranking)
	}
	strategies := make([]string, 0, len(extractionStrategies))
	for strategy := range extractionStrategies {
		strategies = append(strategies, strategy)
	}
	sort.Strings(strategies)

	return map[string]interface{}{
		"summary": map[string]interface{}{
			"url":             sourceURL,
			"final_url":       overview.URL,
			"title":           overview.Title,
			"text":            overview.Text,
			"screenshot_path": screenshotPath,
			"captured_at":     capturedAt.Unix(),
		},
		"products":                     productValues,
		"rankings":                     rankingValues,
		"source_url":                   sourceURL,
		"period":                       period,
		"product_page_url":             productPageURL,
		"product_page_text":            productPageText,
		"product_page_screenshot_path": productScreenshotPath,
		"extraction_strategies":        strategies,
		"captured_at":                  time.Now().Unix(),
	}, nil
}

func waitAgentReportBrowser(ctx context.Context, seconds int) error {
	if seconds < 1 {
		seconds = 1
	}
	timer := time.NewTimer(time.Duration(seconds) * time.Second)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func evaluateAgentBrowserJSON(ctx context.Context, conn *websocket.Conn, id int, expression string, target interface{}) error {
	raw, err := agentBrowserCDPCall(ctx, conn, id, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
	})
	if err != nil {
		return err
	}
	var response struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return err
	}
	if len(response.ExceptionDetails) > 0 {
		return errors.New("浏览器页面数据脚本执行失败")
	}
	if strings.TrimSpace(response.Result.Value) == "" {
		return errors.New("浏览器页面数据脚本没有返回结果")
	}
	return json.Unmarshal([]byte(response.Result.Value), target)
}

func clickAgentBrowserLabel(ctx context.Context, conn *websocket.Conn, id int, labels []string) (bool, error) {
	labelsJSON, _ := json.Marshal(labels)
	expression := `(function(labels){
		const clean = value => (value || '').replace(/\s+/g, ' ').trim();
		const visible = el => { const r = el.getBoundingClientRect(); const s = getComputedStyle(el); return r.width > 0 && r.height > 0 && s.visibility !== 'hidden' && s.display !== 'none'; };
		const selectors = 'a,button,[role="button"],[role="tab"],[role="menuitem"],li,span,div';
		const candidates = Array.from(document.querySelectorAll(selectors)).filter(visible);
		for (const label of labels) {
			const exact = candidates.filter(el => clean(el.innerText || el.textContent) === label);
			for (const match of exact) {
				const target = match.closest('a,button,[role="button"],[role="tab"],[role="menuitem"],li') || match;
				if (target && visible(target)) { target.click(); return JSON.stringify({clicked:true,label}); }
			}
		}
		return JSON.stringify({clicked:false});
	})(` + string(labelsJSON) + `)`
	var response struct {
		Clicked bool `json:"clicked"`
	}
	if err := evaluateAgentBrowserJSON(ctx, conn, id, expression, &response); err != nil {
		return false, err
	}
	return response.Clicked, nil
}

func evaluateAgentBrowserTables(ctx context.Context, conn *websocket.Conn, id int) (agentBrowserTablePayload, error) {
	expression := `(function(){
		const clean = value => (value || '').replace(/\s+/g, ' ').trim();
		const tables = Array.from(document.querySelectorAll('table')).map(table => {
			let headers = Array.from(table.querySelectorAll('thead th')).map(cell => clean(cell.innerText || cell.textContent));
			const rows = Array.from(table.querySelectorAll('tbody tr')).map(row => Array.from(row.querySelectorAll('td')).map(cell => clean(cell.innerText || cell.textContent))).filter(row => row.some(Boolean));
			if (!headers.length) {
				const first = table.querySelector('tr');
				headers = first ? Array.from(first.querySelectorAll('th,td')).map(cell => clean(cell.innerText || cell.textContent)) : [];
			}
			return {headers,rows};
		}).filter(table => table.headers.length || table.rows.length);
		if (!tables.some(table => table.rows.length)) {
			const roleRows = Array.from(document.querySelectorAll('[role="row"]'));
			const headerRow = roleRows.find(row => row.querySelector('[role="columnheader"]'));
			if (headerRow) {
				const headers = Array.from(headerRow.querySelectorAll('[role="columnheader"]')).map(cell => clean(cell.innerText || cell.textContent));
				const rows = roleRows.filter(row => row !== headerRow).map(row => Array.from(row.querySelectorAll('[role="cell"],[role="gridcell"]')).map(cell => clean(cell.innerText || cell.textContent))).filter(row => row.length);
				if (rows.length) tables.push({headers,rows});
			}
		}
		return JSON.stringify({title:document.title || '',url:location.href || '',text:document.body ? document.body.innerText : '',tables});
	})()`
	var payload agentBrowserTablePayload
	if err := evaluateAgentBrowserJSON(ctx, conn, id, expression, &payload); err != nil {
		return agentBrowserTablePayload{}, err
	}
	return payload, nil
}

func clickAgentBrowserNextPage(ctx context.Context, conn *websocket.Conn, id int) (bool, error) {
	expression := `(function(){
		const visible = el => { const r = el.getBoundingClientRect(); const s = getComputedStyle(el); return r.width > 0 && r.height > 0 && s.visibility !== 'hidden' && s.display !== 'none'; };
		const disabled = el => el.disabled || el.getAttribute('aria-disabled') === 'true' || /disabled/.test(el.className || '') || /disabled/.test((el.parentElement && el.parentElement.className) || '');
		const selectors = ['button[aria-label*="下一"]','button[title*="下一"]','.ant-pagination-next button','.ant-pagination-next','[class*="pagination-next"] button','[class*="pagination-next"]'];
		for (const selector of selectors) {
			const el = document.querySelector(selector);
			if (el && visible(el) && !disabled(el)) { el.click(); return JSON.stringify({clicked:true}); }
		}
		const textMatch = Array.from(document.querySelectorAll('button,a,[role="button"]')).find(el => visible(el) && !disabled(el) && /^(下一页|下页|next|›|>)$/i.test((el.innerText || el.textContent || '').trim()));
		if (textMatch) { textMatch.click(); return JSON.stringify({clicked:true}); }
		return JSON.stringify({clicked:false});
	})()`
	var response struct {
		Clicked bool `json:"clicked"`
	}
	if err := evaluateAgentBrowserJSON(ctx, conn, id, expression, &response); err != nil {
		return false, err
	}
	return response.Clicked, nil
}

func bestAgentProductTable(tables []agentBrowserTable) agentBrowserTable {
	tables = normalizeAgentBrowserTables(tables)
	bestScore := 0
	best := agentBrowserTable{}
	for _, table := range tables {
		headers := strings.Join(table.Headers, " ")
		score := 0
		for _, marker := range []string{"商品", "标题", "曝光", "浏览", "支付", "价格", "询单"} {
			if strings.Contains(headers, marker) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			best = table
		}
	}
	if bestScore < 2 {
		return agentBrowserTable{}
	}
	return best
}

func normalizeAgentBrowserTables(tables []agentBrowserTable) []agentBrowserTable {
	normalized := make([]agentBrowserTable, 0, len(tables))
	for index, table := range tables {
		rows := nonEmptyAgentBrowserRows(table.Rows)
		if len(rows) == 0 {
			continue
		}
		headers := append([]string(nil), table.Headers...)
		if len(headers) == 0 {
			width := widestAgentBrowserRow(rows)
			bestIndex := -1
			bestDistance := len(tables) + 1
			for candidateIndex, candidate := range tables {
				if len(candidate.Headers) != width {
					continue
				}
				distance := index - candidateIndex
				if distance < 0 {
					distance = -distance
				}
				if distance < bestDistance {
					bestIndex = candidateIndex
					bestDistance = distance
				}
			}
			if bestIndex >= 0 {
				headers = append([]string(nil), tables[bestIndex].Headers...)
			}
		}
		if len(headers) == 0 {
			continue
		}
		normalized = append(normalized, agentBrowserTable{Headers: headers, Rows: rows})
	}
	return normalized
}

func nonEmptyAgentBrowserRows(rows [][]string) [][]string {
	result := make([][]string, 0, len(rows))
	for _, row := range rows {
		nonEmpty := false
		for _, value := range row {
			if strings.TrimSpace(strings.ReplaceAll(value, "\u00a0", " ")) != "" {
				nonEmpty = true
				break
			}
		}
		if nonEmpty {
			result = append(result, row)
		}
	}
	return result
}

func widestAgentBrowserRow(rows [][]string) int {
	width := 0
	for _, row := range rows {
		if len(row) > width {
			width = len(row)
		}
	}
	return width
}

func agentProductRowsFromTable(table agentBrowserTable) []map[string]interface{} {
	if len(table.Headers) == 0 {
		return nil
	}
	rows := make([]map[string]interface{}, 0, len(table.Rows))
	for _, values := range table.Rows {
		row := map[string]interface{}{}
		for index, header := range table.Headers {
			header = strings.TrimSpace(header)
			if header == "" || index >= len(values) {
				continue
			}
			row[header] = strings.TrimSpace(values[index])
		}
		if len(row) > 0 {
			rows = append(rows, row)
		}
	}
	return rows
}

func agentStringSlice(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(stringFromInterface(item)); text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func expandAgentCompositeRow(row map[string]interface{}, expectedFields []string) {
	keyField := agentExpectedField(expectedFields, "商品ID", "商品编号", "SKU", "ID", "编号", "编码")
	if keyField == "" || agentProductValue(row, keyField) != "" {
		return
	}
	titleField := agentExpectedField(expectedFields, "商品标题", "商品名称", "标题", "名称")
	priceField := agentExpectedField(expectedFields, "价格", "售价", "单价")
	markers := uniqueAgentStrings([]string{keyField, "商品ID", "商品编号", "SKU", "ID"})

	for _, raw := range row {
		text := strings.TrimSpace(stringFromInterface(raw))
		for _, marker := range markers {
			position := strings.Index(text, marker)
			if position < 0 {
				continue
			}
			before := strings.TrimSpace(text[:position])
			after := strings.Fields(strings.TrimSpace(text[position+len(marker):]))
			if len(after) == 0 {
				continue
			}
			row[keyField] = after[0]
			if titleField != "" && before != "" {
				row[titleField] = before
			}
			if priceField != "" && len(after) > 1 {
				row[priceField] = after[1]
			}
			return
		}
	}
}

func agentRowsFromSchemaText(text string, expectedFields []string) []map[string]interface{} {
	keyField := agentExpectedField(expectedFields, "商品ID", "商品编号", "SKU", "ID", "编号", "编码")
	if keyField == "" {
		return nil
	}
	titleField := agentExpectedField(expectedFields, "商品标题", "商品名称", "标题", "名称")
	priceField := agentExpectedField(expectedFields, "价格", "售价", "单价")
	markers := uniqueAgentStrings([]string{keyField, "商品ID", "商品编号", "SKU", "ID"})
	lines := agentPageTextLines(text)
	markerIndexes := make([]int, 0)
	for index, line := range lines {
		if agentStringEqualsAny(line, markers) {
			markerIndexes = append(markerIndexes, index)
		}
	}
	if len(markerIndexes) == 0 {
		return nil
	}

	excluded := map[string]bool{
		normalizeAgentReportFieldName(keyField):   true,
		normalizeAgentReportFieldName(titleField): true,
		normalizeAgentReportFieldName(priceField): true,
	}
	metricFieldsReversed := make([]string, 0, len(expectedFields))
	seenFields := map[string]bool{}
	for index := markerIndexes[0] - 1; index >= 0; index-- {
		field := agentMatchingExpectedField(lines[index], expectedFields)
		normalized := normalizeAgentReportFieldName(field)
		if field == "" || excluded[normalized] || seenFields[normalized] {
			continue
		}
		seenFields[normalized] = true
		metricFieldsReversed = append(metricFieldsReversed, field)
	}
	metricFields := make([]string, 0, len(metricFieldsReversed))
	for index := len(metricFieldsReversed) - 1; index >= 0; index-- {
		metricFields = append(metricFields, metricFieldsReversed[index])
	}

	rows := make([]map[string]interface{}, 0, len(markerIndexes))
	for markerOffset, markerIndex := range markerIndexes {
		end := len(lines)
		if markerOffset+1 < len(markerIndexes) {
			end = markerIndexes[markerOffset+1] - 1
		}
		if markerIndex+1 >= end {
			continue
		}
		identifier := strings.TrimSpace(lines[markerIndex+1])
		if identifier == "" || agentMatchingExpectedField(identifier, expectedFields) != "" {
			continue
		}
		row := map[string]interface{}{keyField: identifier}
		if titleField != "" && markerIndex > 0 {
			row[titleField] = lines[markerIndex-1]
		}
		cursor := markerIndex + 2
		if priceField != "" && cursor < end {
			row[priceField] = lines[cursor]
			cursor++
		}
		for _, field := range metricFields {
			if cursor >= end {
				break
			}
			row[field] = lines[cursor]
			cursor++
		}
		rows = append(rows, row)
	}
	return rows
}

func agentPageTextLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	result := make([]string, 0)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.ReplaceAll(line, "\u00a0", " "))
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func agentExpectedField(expectedFields []string, aliases ...string) string {
	for _, alias := range aliases {
		normalizedAlias := normalizeAgentReportFieldName(alias)
		for _, field := range expectedFields {
			if normalizeAgentReportFieldName(field) == normalizedAlias {
				return field
			}
		}
	}
	for _, alias := range aliases {
		if field := agentMatchingExpectedField(alias, expectedFields); field != "" {
			return field
		}
	}
	return ""
}

func agentMatchingExpectedField(label string, expectedFields []string) string {
	normalizedLabel := normalizeAgentReportFieldName(label)
	if normalizedLabel == "" {
		return ""
	}
	best := ""
	bestLength := 0
	for _, field := range expectedFields {
		normalizedField := normalizeAgentReportFieldName(field)
		if normalizedField == "" {
			continue
		}
		if normalizedLabel == normalizedField || strings.HasSuffix(normalizedLabel, normalizedField) || strings.HasSuffix(normalizedField, normalizedLabel) {
			if len(normalizedField) > bestLength {
				best = field
				bestLength = len(normalizedField)
			}
		}
	}
	return best
}

func uniqueAgentStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func agentStringEqualsAny(value string, candidates []string) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}

func agentProductRowKey(row map[string]interface{}) string {
	if value := agentProductValue(row, "商品ID", "商品编号", "ID"); value != "" {
		return "id:" + value
	}
	if value := agentProductValue(row, "商品标题", "标题", "商品"); value != "" {
		return "title:" + value
	}
	raw, _ := json.Marshal(row)
	return string(raw)
}

func enrichAgentProductRow(row map[string]interface{}) {
	if _, ok := row["日期"]; !ok {
		row["日期"] = time.Now().Format("2006-01-02")
	}
	deriveRate := func(target string, numeratorAliases, denominatorAliases []string) {
		if agentProductValue(row, target) != "" {
			return
		}
		numerator, numeratorOK := agentProductNumber(row, numeratorAliases...)
		denominator, denominatorOK := agentProductNumber(row, denominatorAliases...)
		if numeratorOK && denominatorOK && denominator != 0 {
			row[target] = fmt.Sprintf("%.3f%%", numerator/denominator*100)
		}
	}
	deriveRate("点击率", []string{"浏览次数", "商品浏览次数"}, []string{"曝光次数", "商品曝光次数"})
	deriveRate("浏览支付转化率", []string{"支付人数", "支付笔数"}, []string{"浏览人数", "商品浏览人数"})
	if agentProductValue(row, "单品建议", "建议") == "" {
		row["单品建议"] = agentProductAdvice(row)
	}
}

func buildAgentProductRankings(products []map[string]interface{}) []map[string]interface{} {
	type rankingSpec struct {
		Name    string
		Aliases []string
		Advice  string
	}
	specs := []rankingSpec{
		{Name: "曝光TOP", Aliases: []string{"曝光次数", "商品曝光次数"}, Advice: "曝光高，优先优化点击转化"},
		{Name: "支付TOP", Aliases: []string{"支付金额", "支付金额(元)", "支付订单数", "支付笔数"}, Advice: "有成交，重点补库存和扩相似款"},
		{Name: "转化TOP", Aliases: []string{"浏览支付转化率", "支付转化率", "点击率"}, Advice: "转化较好，可复用标题和主图思路"},
	}
	rows := make([]map[string]interface{}, 0, len(specs)*5)
	for _, spec := range specs {
		sorted := append([]map[string]interface{}(nil), products...)
		sort.SliceStable(sorted, func(i, j int) bool {
			left, _ := agentProductNumber(sorted[i], spec.Aliases...)
			right, _ := agentProductNumber(sorted[j], spec.Aliases...)
			return left > right
		})
		limit := len(sorted)
		if limit > 5 {
			limit = 5
		}
		for index := 0; index < limit; index++ {
			product := sorted[index]
			value := agentProductValue(product, spec.Aliases...)
			rows = append(rows, map[string]interface{}{
				"排行类型": spec.Name,
				"排名":   index + 1,
				"关键指标": value,
				"辅助指标": agentProductValue(product, "浏览次数", "支付人数", "曝光次数"),
				"建议":   spec.Advice,
				"商品ID": agentProductValue(product, "商品ID", "商品编号", "ID"),
				"商品标题": agentProductValue(product, "商品标题", "标题", "商品"),
			})
		}
	}
	return rows
}

func agentProductAdvice(row map[string]interface{}) string {
	exposure, hasExposure := agentProductNumber(row, "曝光次数", "商品曝光次数")
	views, hasViews := agentProductNumber(row, "浏览次数", "商品浏览次数")
	payments, hasPayments := agentProductNumber(row, "支付订单数", "支付笔数", "支付人数")
	switch {
	case hasPayments && payments > 0:
		return "已有成交，保持供给并复用有效标题和主图。"
	case hasExposure && exposure >= 50 && (!hasViews || views == 0):
		return "曝光较高但无浏览，优先优化主图、标题和价格展示。"
	case hasViews && views > 0:
		return "有浏览但未成交，补充详情页信任信息和客服承接。"
	default:
		return "曝光较低，建议优化关键词或调整商品供给。"
	}
}

func agentProductValue(row map[string]interface{}, aliases ...string) string {
	for _, alias := range aliases {
		normalizedAlias := normalizeAgentReportFieldName(alias)
		for key, value := range row {
			normalizedKey := normalizeAgentReportFieldName(key)
			if normalizedKey == normalizedAlias || strings.HasSuffix(normalizedKey, normalizedAlias) || strings.HasSuffix(normalizedAlias, normalizedKey) {
				return stringFromInterface(value)
			}
		}
	}
	return ""
}

func agentProductNumber(row map[string]interface{}, aliases ...string) (float64, bool) {
	value := agentProductValue(row, aliases...)
	value = strings.TrimSpace(strings.TrimSuffix(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(value, ",", ""), "¥", ""), "￥", ""), "%"))
	if value == "" || value == "-" || value == "—" {
		return 0, false
	}
	number, err := strconv.ParseFloat(value, 64)
	return number, err == nil
}
