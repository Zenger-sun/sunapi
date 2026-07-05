package sunapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteAgentWebpageGraphNodeWritesHTMLAndJSON(t *testing.T) {
	store := &Store{dataDir: t.TempDir()}
	node := agentBlueprintGraphNode{ID: "webpage-1"}
	node.Data.Title = "运营网页"
	node.Data.Settings = map[string]string{"输出目录": "data/output/web-reports", "输出模式": "输出网页 + 数据"}
	upstream := map[string]interface{}{
		"table": map[string]interface{}{
			"rows": []interface{}{
				map[string]interface{}{"商品": "A", "访问": 10.0},
				map[string]interface{}{"商品": "B", "访问": 20.0},
			},
		},
	}

	result := executeAgentWebpageGraphNode(store, node, upstream, agentBlueprintNodeResult{Status: "completed"})
	if result.Status != "completed" {
		t.Fatalf("status=%q error=%s", result.Status, result.Error)
	}
	files, ok := result.Output["files"].([]AgentReportWrittenFile)
	if !ok || len(files) != 2 {
		t.Fatalf("files=%T %#v", result.Output["files"], result.Output["files"])
	}
	var htmlPath string
	var jsonPath string
	for _, file := range files {
		switch file.Extension {
		case "html":
			htmlPath = file.Path
		case "json":
			jsonPath = file.Path
		}
	}
	if htmlPath == "" || jsonPath == "" {
		t.Fatalf("expected html and json files, got %#v", files)
	}
	htmlBytes, err := os.ReadFile(filepath.Join(store.DataDir(), filepath.FromSlash(strings.TrimPrefix(htmlPath, "data/"))))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(htmlBytes), "商品") || !strings.Contains(string(htmlBytes), "访问") {
		t.Fatalf("html does not include table headers: %s", string(htmlBytes))
	}
}
