package sunapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadAgentDataFileParsesCSVRows(t *testing.T) {
	store := &Store{dataDir: t.TempDir()}
	path := filepath.Join(store.DataDir(), "input.csv")
	if err := os.WriteFile(path, []byte("name,amount\nA,3\nB,5\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := readAgentDataFile(store, "data/input.csv")
	if err != nil {
		t.Fatalf("readAgentDataFile returned error: %v", err)
	}
	if result.Format != "CSV 表格" {
		t.Fatalf("format=%q want CSV 表格", result.Format)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("len(rows)=%d want 2", len(result.Rows))
	}
	if got := result.Rows[1]["name"]; got != "B" {
		t.Fatalf("row name=%v want B", got)
	}
}

func TestReadAgentDataFileParsesXLSXRows(t *testing.T) {
	store := &Store{dataDir: t.TempDir()}
	data, err := buildAgentReportXLSX("测试表格", "Excel 表格", "| name | amount |\n| --- | --- |\n| A | 3 |\n| B | 5 |")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.DataDir(), "input.xlsx")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	result, err := readAgentDataFile(store, "data/input.xlsx")
	if err != nil {
		t.Fatalf("readAgentDataFile returned error: %v", err)
	}
	if result.Format != "Excel 表格" {
		t.Fatalf("format=%q want Excel 表格", result.Format)
	}
	if len(result.Rows) < 1 {
		t.Fatalf("expected parsed xlsx rows")
	}
}

func TestExecuteAgentFileGraphNodeWritesExcelFile(t *testing.T) {
	store := &Store{dataDir: t.TempDir()}
	node := agentBlueprintGraphNode{ID: "file-1"}
	node.Data.Title = "导出结果"
	node.Data.Settings = map[string]string{
		"模式":   "写入",
		"格式":   "Excel 表格",
		"输出目录": "data/output/agent-files",
	}
	upstream := map[string]interface{}{
		"filter": map[string]interface{}{
			"rows": []interface{}{
				map[string]interface{}{"name": "A", "amount": 3.0},
				map[string]interface{}{"name": "B", "amount": 5.0},
			},
		},
	}
	result := executeAgentFileGraphNode(store, node, upstream, agentBlueprintNodeResult{Status: "completed"})
	if result.Status != "completed" {
		t.Fatalf("status=%q error=%s", result.Status, result.Error)
	}
	files, ok := result.Output["files"].([]AgentReportWrittenFile)
	if !ok || len(files) != 1 {
		t.Fatalf("files=%T %#v", result.Output["files"], result.Output["files"])
	}
	if files[0].Extension != "xlsx" {
		t.Fatalf("extension=%q want xlsx", files[0].Extension)
	}
	if _, err := os.Stat(filepath.Join(store.DataDir(), filepath.FromSlash(strings.TrimPrefix(files[0].Path, "data/")))); err != nil {
		t.Fatalf("expected written file: %v", err)
	}
}

func TestComponentInputFilesUseDedicatedDirectory(t *testing.T) {
	directory, outputDirectory := agentUploadDirectories("component-input")
	if directory != agentDataInputDir || outputDirectory != "" {
		t.Fatalf("component input directories = (%q, %q)", directory, outputDirectory)
	}

	dataDir := t.TempDir()
	path := filepath.Join(dataDir, agentDataInputDir, "source.csv")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("name\nSunAPI\n"), 0600); err != nil {
		t.Fatal(err)
	}
	resolved, err := agentUploadedTemplateAbsPath(dataDir, "data/inputs/source.csv")
	if err != nil {
		t.Fatalf("component input path rejected: %v", err)
	}
	if resolved != path {
		t.Fatalf("resolved=%q want %q", resolved, path)
	}
}

func TestResolveAgentNodeImportedFilesReadsConfiguredFile(t *testing.T) {
	store := &Store{dataDir: t.TempDir()}
	path := filepath.Join(store.DataDir(), agentDataInputDir, "source.csv")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("name,amount\nSunAPI,1\n"), 0600); err != nil {
		t.Fatal(err)
	}

	node := agentBlueprintGraphNode{ID: "custom-file"}
	node.Data.Inputs = []string{"导入文件"}
	node.Data.Settings = map[string]string{"导入文件": "data/inputs/source.csv"}
	files, err := resolveAgentNodeImportedFiles(store, node)
	if err != nil {
		t.Fatalf("resolveAgentNodeImportedFiles returned error: %v", err)
	}
	file, ok := files["导入文件"]
	if !ok {
		t.Fatalf("files=%#v", files)
	}
	if file.Format != "CSV 表格" || len(file.Rows) != 1 {
		t.Fatalf("file=%#v", file)
	}
}
