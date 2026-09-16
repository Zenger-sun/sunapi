package sunapi

import "testing"

func TestAgentReportNeedsProductRows(t *testing.T) {
	var node agentBlueprintGraphNode
	node.Data.Settings = map[string]string{"模板预览": "日报总览 商品明细 商品ID 商品标题"}
	if !agentReportNeedsProductRows(node) {
		t.Fatal("expected product rows to be required")
	}
	node.Data.Settings["模板预览"] = "核心指标 数值"
	if agentReportNeedsProductRows(node) {
		t.Fatal("did not expect product rows to be required")
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
	rows := agentRowsFromSchemaText(text, agentReportProductFields)
	if len(rows) != 2 {
		t.Fatalf("rows=%#v, want two rows", rows)
	}
	if rows[0]["商品ID"] != "p-1" || rows[0]["商品标题"] != "商品A" || rows[0]["支付金额"] != "22.89" {
		t.Fatalf("first row=%#v", rows[0])
	}
	if rows[1]["商品ID"] != "p-2" || rows[1]["曝光次数"] != "4" {
		t.Fatalf("second row=%#v", rows[1])
	}
}

func TestPrepareAgentReportTemplateDataKeepsExistingProducts(t *testing.T) {
	var node agentBlueprintGraphNode
	node.Data.Settings = map[string]string{"模板预览": "商品明细 商品ID 商品标题"}
	upstream := map[string]interface{}{
		"collector": map[string]interface{}{
			"products": []interface{}{
				map[string]interface{}{"商品ID": "p-1", "商品标题": "商品A"},
			},
		},
	}
	data, count, autoCollected, err := prepareAgentReportTemplateData(t.Context(), nil, node, upstream)
	if err != nil {
		t.Fatalf("prepareAgentReportTemplateData error=%v", err)
	}
	if count != 1 || autoCollected || len(data) != len(upstream) {
		t.Fatalf("count=%d autoCollected=%v data=%#v", count, autoCollected, data)
	}
}

func TestBuildAgentReportTemplateContextUsesPeriodDate(t *testing.T) {
	context := buildAgentReportTemplateContext(map[string]interface{}{
		"period": "2026-08-26",
	})
	if context.date != "2026-08-26" {
		t.Fatalf("date=%q, want period date", context.date)
	}
}
