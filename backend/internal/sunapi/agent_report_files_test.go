package sunapi

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestAgentGraphReportFilesUsesMultipleTemplateBindings(t *testing.T) {
	node := agentBlueprintGraphNode{ID: "report-1"}
	node.Data.Kind = "report"
	node.Data.Title = "生成日报"
	node.Data.Settings = map[string]string{
		"模板文件": `[{"path":"data/templates/reports/a.xlsx","originalName":"a.xlsx"},"data/templates/reports/b.md"]`,
	}

	files := agentGraphReportFiles(node, "report content")
	if len(files) != 2 {
		t.Fatalf("files=%d want 2", len(files))
	}
	if files[0].Label != "Excel 表格" || files[0].TemplateOriginalName != "a.xlsx" {
		t.Fatalf("first file=%+v", files[0])
	}
	if files[1].Label != "Markdown 日报" || files[1].TemplateOriginalName != "b.md" {
		t.Fatalf("second file=%+v", files[1])
	}
}

func TestBuildAgentReportXLSXFromTemplatePreservesTemplateWithoutData(t *testing.T) {
	templateBytes, err := buildAgentReportXLSX("template", "Excel", "old-template-data")
	if err != nil {
		t.Fatalf("build template xlsx: %v", err)
	}

	output, err := buildAgentReportXLSXFromTemplate(templateBytes, "report", "Excel", "generated-data")
	if err != nil {
		t.Fatalf("build xlsx from template: %v", err)
	}
	sheetXML := readXLSXEntryForTest(t, output, "xl/worksheets/sheet1.xml")
	if !strings.Contains(sheetXML, "old-template-data") {
		t.Fatalf("expected original template layout to be preserved: %s", sheetXML)
	}
	if strings.Contains(sheetXML, "generated-data") {
		t.Fatalf("did not expect generic content to replace the template: %s", sheetXML)
	}
}

func TestBuildAgentReportXLSXFromTemplateReplacesPlaceholders(t *testing.T) {
	templateBytes, err := buildAgentReportXLSX("模板", "Excel 表格", "{{content}}")
	if err != nil {
		t.Fatalf("build template xlsx: %v", err)
	}

	output, err := buildAgentReportXLSXFromTemplate(templateBytes, "日报", "Excel 表格", "生成内容")
	if err != nil {
		t.Fatalf("build xlsx from template: %v", err)
	}
	sheetXML := readXLSXEntryForTest(t, output, "xl/worksheets/sheet1.xml")
	if strings.Contains(sheetXML, "{{content}}") {
		t.Fatalf("expected placeholder to be replaced: %s", sheetXML)
	}
	if !strings.Contains(sheetXML, "生成内容") {
		t.Fatalf("expected generated content to replace placeholder: %s", sheetXML)
	}
}

func TestBuildAgentReportXLSXFromTemplateFillsSummaryFields(t *testing.T) {
	templateContent := strings.Join([]string{
		"| 核心指标 | 数值 | 说明 | 健康判断 | 建议优先级 |",
		"| --- | --- | --- | --- | --- |",
		"| 商品访问次数 | 66 | 当前访问次数 | 参考 | 中 |",
		"| 支付金额 | 0.00 | 当前支付金额 | 无成交 | 高 |",
		"| 运营建议 |  |  |  |  |",
		"| 1 | 旧模板建议 |  |  |  |",
	}, "\n")
	templateBytes, err := buildAgentReportXLSX("template", "Excel", templateContent)
	if err != nil {
		t.Fatalf("build template xlsx: %v", err)
	}

	output, err := buildAgentReportXLSXFromTemplate(
		templateBytes,
		"report",
		"Excel",
		"generic content",
		map[string]interface{}{
			"browser": map[string]interface{}{
				"text":      "商品访问次数\n92\n支付金额（元）\n¥22.89\n2026-07-13",
				"final_url": "https://seller.goofish.com/dashboard",
			},
		},
	)
	if err != nil {
		t.Fatalf("build xlsx from template: %v", err)
	}
	cells := parseAgentReportWorksheetCells(
		[]byte(readXLSXEntryForTest(t, output, "xl/worksheets/sheet1.xml")),
		nil,
	)
	values := make(map[string]string, len(cells))
	for _, cell := range cells {
		values[cell.Reference] = cell.Value
	}
	if values["A1"] != "核心指标" {
		t.Fatalf("template header = %q, want preserved header", values["A1"])
	}
	if values["B2"] != "92" {
		t.Fatalf("visit count = %q, want 92", values["B2"])
	}
	if values["B3"] != "22.89" {
		t.Fatalf("payment amount = %q, want 22.89", values["B3"])
	}
	if values["D3"] != "正常" {
		t.Fatalf("payment health = %q, want 正常", values["D3"])
	}
	if strings.Contains(values["B5"], "旧模板建议") {
		t.Fatalf("old template advice was not cleared: %q", values["B5"])
	}
}

func TestBuildAgentReportXLSXFromTemplateClearsStaleProductRows(t *testing.T) {
	templateBytes, err := buildAgentReportXLSX(
		"template",
		"Excel",
		"| 商品ID | 商品标题 | 支付金额(元) |\n| --- | --- | --- |\n| old-id | 旧商品 | 99 |",
	)
	if err != nil {
		t.Fatalf("build template xlsx: %v", err)
	}
	output, err := buildAgentReportXLSXFromTemplate(
		templateBytes,
		"report",
		"Excel",
		"generic content",
		map[string]interface{}{"browser": map[string]interface{}{"text": "商品访问次数\n92"}},
	)
	if err != nil {
		t.Fatalf("build xlsx from template: %v", err)
	}
	sheetXML := readXLSXEntryForTest(t, output, "xl/worksheets/sheet1.xml")
	if strings.Contains(sheetXML, "old-id") || strings.Contains(sheetXML, "旧商品") {
		t.Fatalf("stale product rows were not cleared: %s", sheetXML)
	}
}

func TestBuildAgentReportWorksheetFieldUpdatesFillsProductRows(t *testing.T) {
	cells := []agentReportWorksheetCell{
		{Reference: "A1", Value: "商品ID"},
		{Reference: "B1", Value: "商品标题"},
		{Reference: "C1", Value: "曝光次数"},
		{Reference: "A2", Value: "old"},
		{Reference: "B2", Value: "old title"},
		{Reference: "C2", Value: "1"},
		{Reference: "A3", Value: "old-2"},
		{Reference: "B3", Value: "old title 2"},
		{Reference: "C3", Value: "2"},
	}
	updates := buildAgentReportWorksheetFieldUpdates(cells, agentReportTemplateContext{
		fields: map[string]string{},
		products: []map[string]string{
			{"商品ID": "p-1", "商品标题": "新商品", "曝光次数": "88"},
		},
	})
	if updates["A2"] != "p-1" || updates["B2"] != "新商品" || updates["C2"] != "88" {
		t.Fatalf("first product row=%#v", updates)
	}
	if updates["A3"] != "" || updates["B3"] != "" || updates["C3"] != "" {
		t.Fatalf("stale second row was not cleared: %#v", updates)
	}
}

func TestBuildAgentReportWorksheetFieldUpdatesFillsRankingRows(t *testing.T) {
	cells := []agentReportWorksheetCell{
		{Reference: "A1", Value: "排行类型"},
		{Reference: "B1", Value: "排名"},
		{Reference: "C1", Value: "关键指标"},
		{Reference: "D1", Value: "建议"},
		{Reference: "A2", Value: "old"},
		{Reference: "B2", Value: "old"},
		{Reference: "C2", Value: "old"},
		{Reference: "D2", Value: "old"},
	}
	updates := buildAgentReportWorksheetFieldUpdates(cells, agentReportTemplateContext{
		fields: map[string]string{},
		rankings: []map[string]string{
			{"排行类型": "曝光TOP", "排名": "1", "关键指标": "100", "建议": "优化点击"},
		},
	})
	if updates["A2"] != "曝光TOP" || updates["B2"] != "1" || updates["C2"] != "100" || updates["D2"] != "优化点击" {
		t.Fatalf("ranking row=%#v", updates)
	}
}

func TestBuildAgentReportWorksheetFieldUpdatesFillsOrderAliases(t *testing.T) {
	cells := []agentReportWorksheetCell{
		{Reference: "A1", Value: "订单号"},
		{Reference: "B1", Value: "订单日期"},
		{Reference: "C1", Value: "商品"},
		{Reference: "D1", Value: "成交金额"},
		{Reference: "A2", Value: "old"},
		{Reference: "B2", Value: "old"},
		{Reference: "C2", Value: "old"},
		{Reference: "D2", Value: "old"},
	}
	updates := buildAgentReportWorksheetFieldUpdates(cells, agentReportTemplateContext{
		fields: map[string]string{},
		orders: []map[string]string{
			{"订单编号": "order-1", "下单时间": "2026-07-20 10:00:00", "商品名称": "新商品", "成交价": "12.06"},
		},
	})
	if updates["A2"] != "order-1" || updates["B2"] != "2026-07-20 10:00:00" || updates["C2"] != "新商品" || updates["D2"] != "12.06" {
		t.Fatalf("order row=%#v", updates)
	}
}

func TestReplaceAgentReportWorksheetCellValueSupportsSelfClosingCell(t *testing.T) {
	worksheet := []byte(`<worksheet><sheetData><row r="2"><c r="A2" s="1"/></row></sheetData></worksheet>`)
	updated := string(replaceAgentReportWorksheetCellValue(worksheet, "A2", "商品-1"))
	if !strings.Contains(updated, "商品-1") || strings.Contains(updated, `<c r="A2" s="1"/>`) {
		t.Fatalf("self-closing cell was not replaced: %s", updated)
	}
}

func readXLSXEntryForTest(t *testing.T, data []byte, name string) string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open xlsx: %v", err)
	}
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open xlsx entry: %v", err)
		}
		body, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read xlsx entry: %v", err)
		}
		return string(body)
	}
	t.Fatalf("xlsx entry %q not found", name)
	return ""
}
