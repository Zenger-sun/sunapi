package sunapi

import "testing"

func TestBuildAgentReportPlanRecognizesProductRequirements(t *testing.T) {
	var node agentBlueprintGraphNode
	node.Data.Kind = "report"
	node.Data.Title = "生成日报"
	node.Data.Settings = map[string]string{
		"模板文件":   "data/templates/reports/daily.xlsx",
		"模板原文件名": "日报.xlsx",
		"模板预览":   "日报总览 商品明细 商品ID 商品标题 曝光次数 浏览次数 商品排行",
		"输出格式":   "Excel 表格",
	}
	planOutput := buildAgentReportPlan(node, map[string]interface{}{
		"browser": map[string]interface{}{
			"final_url":         "https://seller.example.test/data",
			"browser_session":   "daily-seller",
			"browser_target_id": "target-current-run",
		},
	})
	plan, ok := planOutput["report_plan"].(map[string]interface{})
	if !ok {
		t.Fatalf("report_plan=%T, want map", planOutput["report_plan"])
	}
	if plan["requires_products"] != true {
		t.Fatalf("requires_products=%v, want true", plan["requires_products"])
	}
	if plan["source_url"] != "https://seller.example.test/data" {
		t.Fatalf("source_url=%v", plan["source_url"])
	}
	if plan["browser_session"] != "daily-seller" {
		t.Fatalf("browser_session=%v", plan["browser_session"])
	}
	if plan["browser_target_id"] != "target-current-run" {
		t.Fatalf("browser_target_id=%v", plan["browser_target_id"])
	}
	fields, ok := plan["required_fields"].([]string)
	if !ok || len(fields) < 4 {
		t.Fatalf("required_fields=%#v", plan["required_fields"])
	}
}

func TestAgentNodeHasDownstreamReportData(t *testing.T) {
	nodes := []agentBlueprintGraphNode{{ID: "report"}, {ID: "middle"}, {ID: "process"}}
	nodes[0].Data.Kind = "report"
	nodes[1].Data.Kind = "custom"
	nodes[2].Data.Kind = "report_data"
	edges := []agentBlueprintGraphEdge{
		{Source: "report", Target: "middle"},
		{Source: "middle", Target: "process"},
	}
	if !agentNodeHasDownstreamReportData("report", nodes, edges) {
		t.Fatal("expected report data processor to be discovered through downstream path")
	}
}

func TestBuildAgentProductRankings(t *testing.T) {
	products := []map[string]interface{}{
		{"商品ID": "a", "商品标题": "商品A", "曝光次数": "20", "浏览次数": "4", "支付金额": "10", "支付人数": "1"},
		{"商品ID": "b", "商品标题": "商品B", "曝光次数": "100", "浏览次数": "10", "支付金额": "0", "支付人数": "0"},
	}
	for _, product := range products {
		enrichAgentProductRow(product)
	}
	rankings := buildAgentProductRankings(products)
	if len(rankings) != 6 {
		t.Fatalf("rankings=%d, want 6", len(rankings))
	}
	if rankings[0]["排行类型"] != "曝光TOP" || rankings[0]["商品ID"] != "b" {
		t.Fatalf("first exposure ranking=%#v", rankings[0])
	}
	if products[0]["点击率"] != "20.000%" {
		t.Fatalf("click rate=%v, want 20.000%%", products[0]["点击率"])
	}
}

func TestBestAgentProductTableMergesSplitHeaderAndBody(t *testing.T) {
	tables := []agentBrowserTable{
		{Headers: []string{"商品信息", "商品曝光次数", "商品浏览次数", "支付金额"}},
		{Rows: [][]string{
			{"", "", "", ""},
			{"商品A 商品ID p-1 ¥5.30", "26", "7", "22.89"},
		}},
	}
	table := bestAgentProductTable(tables)
	if len(table.Headers) != 4 || len(table.Rows) != 1 {
		t.Fatalf("normalized table=%#v", table)
	}
	rows := agentProductRowsFromTable(table)
	expandAgentCompositeRow(rows[0], []string{"商品ID", "商品标题", "价格", "曝光次数", "浏览次数", "支付金额"})
	if rows[0]["商品ID"] != "p-1" || rows[0]["商品标题"] != "商品A" || rows[0]["价格"] != "¥5.30" {
		t.Fatalf("expanded row=%#v", rows[0])
	}
}

func TestAgentRowsFromSchemaText(t *testing.T) {
	text := `商品信息
商品曝光次数
商品曝光人数
商品浏览次数
商品浏览人数
询单人数
支付人数
支付订单数
支付金额
浏览支付转化率

商品A
商品ID
p-1
¥5.30
26
25
7
3
2
1
1
22.89
33.33%
0
0

商品B
商品ID
p-2
¥0.57
4
4
0
0
0
0
0
0.00
-`
	fields := []string{
		"商品ID", "商品标题", "价格", "曝光次数", "曝光人数", "浏览次数", "浏览人数",
		"点击率", "询单人数", "支付人数", "支付订单数", "支付金额", "浏览支付转化率",
	}
	rows := agentRowsFromSchemaText(text, fields)
	if len(rows) != 2 {
		t.Fatalf("rows=%#v", rows)
	}
	if rows[0]["商品ID"] != "p-1" || rows[0]["商品标题"] != "商品A" || rows[0]["支付金额"] != "22.89" {
		t.Fatalf("first row=%#v", rows[0])
	}
	if rows[1]["商品ID"] != "p-2" || rows[1]["曝光次数"] != "4" {
		t.Fatalf("second row=%#v", rows[1])
	}
}
