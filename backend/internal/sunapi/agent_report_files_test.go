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

func TestBuildAgentReportXLSXFromTemplateReplacesTemplateRows(t *testing.T) {
	templateBytes, err := buildAgentReportXLSX("template", "Excel", "old-template-data")
	if err != nil {
		t.Fatalf("build template xlsx: %v", err)
	}

	output, err := buildAgentReportXLSXFromTemplate(templateBytes, "report", "Excel", "generated-data")
	if err != nil {
		t.Fatalf("build xlsx from template: %v", err)
	}
	sheetXML := readXLSXEntryForTest(t, output, "xl/worksheets/sheet1.xml")
	if strings.Contains(sheetXML, "old-template-data") {
		t.Fatalf("expected original template data to be replaced: %s", sheetXML)
	}
	if !strings.Contains(sheetXML, "generated-data") {
		t.Fatalf("expected generated content to be written: %s", sheetXML)
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
