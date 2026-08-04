package sunapi

import (
	"strings"
	"testing"
	"time"
)

func TestParseAgentOrderPageText(t *testing.T) {
	text := `已发货
订单编号
ABC20260731001
添加备注
下单时间 2026-07-31 12:34:56
蓝色收纳盒
已发货
¥
6.03
×2
¥
12.06
包邮
买家甲
联系ta
查看详情
交易成功
订单编号
XYZ20260730002
添加备注
下单时间 2026-07-30 08:01:02
白色挂钩
交易成功
¥ 10.00
×1
¥ 10.00
包邮
买家乙
联系ta
查看详情`

	orders := parseAgentOrderPageText(text)
	if len(orders) != 2 {
		t.Fatalf("orders=%d, want 2: %#v", len(orders), orders)
	}
	if orders[0].OrderID != "ABC20260731001" || orders[0].Status != "已发货" {
		t.Fatalf("first order identity/status=%#v", orders[0])
	}
	if orders[0].ProductName != "蓝色收纳盒" || orders[0].Quantity != 2 || orders[0].Amount != 12.06 || orders[0].BuyerNickname != "买家甲" {
		t.Fatalf("first order fields=%#v", orders[0])
	}
	if orders[1].Status != "交易成功" || orders[1].UnitPrice != 10 || orders[1].Amount != 10 {
		t.Fatalf("second order fields=%#v", orders[1])
	}
}

func TestParseAgentOrderDateRangeDefaultsToPreviousMonth(t *testing.T) {
	now := time.Date(2026, time.August, 3, 21, 0, 0, 0, time.Local)
	rangeValue, err := parseAgentOrderDateRange(map[string]string{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if rangeValue.StartDate != "2026-07-01" || rangeValue.EndDate != "2026-07-31" {
		t.Fatalf("range=%+v, want July 2026", rangeValue)
	}
	if rangeValue.End.Hour() != 23 || rangeValue.End.Minute() != 59 || rangeValue.End.Second() != 59 {
		t.Fatalf("end=%v, want end of day", rangeValue.End)
	}
}

func TestAgentOrderAccumulatorFiltersAndDeduplicates(t *testing.T) {
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.Local)
	rangeValue, err := parseAgentOrderDateRange(map[string]string{"开始日期": "2026-07-01", "结束日期": "2026-07-31"}, now)
	if err != nil {
		t.Fatal(err)
	}
	parseTime := func(value string) time.Time {
		parsed, _ := time.ParseInLocation("2006-01-02 15:04:05", value, time.Local)
		return parsed
	}
	records := []agentOrderRecord{
		{OrderID: "order-1", OrderedAt: parseTime("2026-07-20 10:00:00"), Status: "已发货", Amount: 8},
		{OrderID: "order-1", OrderedAt: parseTime("2026-07-20 10:00:00"), Status: "已发货", Amount: 8},
		{OrderID: "order-2", OrderedAt: parseTime("2026-06-30 10:00:00"), Status: "已发货", Amount: 9},
		{OrderID: "order-3", OrderedAt: parseTime("2026-07-21 10:00:00"), Status: "待付款", Amount: 10},
	}
	acc := newAgentOrderAccumulator()
	acc.AddPage(records, 1, rangeValue, []string{"已发货"})
	if len(acc.Orders) != 1 || acc.Orders[0].OrderID != "order-1" {
		t.Fatalf("orders=%#v", acc.Orders)
	}
	if acc.RawCount != 4 || acc.DuplicateCount != 1 || acc.OutOfRangeCount != 1 || acc.StatusFilteredCount != 1 {
		t.Fatalf("counts raw=%d duplicate=%d out=%d status=%d", acc.RawCount, acc.DuplicateCount, acc.OutOfRangeCount, acc.StatusFilteredCount)
	}
}

func TestBuildAgentOrderReportFilesUsesMultipleWorksheets(t *testing.T) {
	node := agentBlueprintGraphNode{}
	node.Data.Title = "七月订单报告"
	dataset := map[string]interface{}{
		"summary": map[string]interface{}{
			"开始日期": "2026-07-01", "结束日期": "2026-07-31", "订单总数": 1,
			"订单成交价合计": "12.06", "可计营业额": "12.06", "退款成功订单数": 0,
			"退款关联成交额": "0.00", "原始订单数": 1, "重复订单数": 0,
			"日期范围外订单数": 0, "状态过滤订单数": 0, "采集页数": 1,
			"停止原因": "no_next_page", "数据来源": "https://www.goofish.com/",
			"退款金额口径": "参考值",
		},
		"orders": []interface{}{map[string]interface{}{
			"订单编号": "order-1", "下单时间": "2026-07-20 10:00:00", "订单状态": "已发货", "成交价": "12.06",
		}},
		"order_status_summary": []interface{}{map[string]interface{}{"订单状态": "已发货", "订单数": 1, "成交价合计": "12.06"}},
	}
	files := buildAgentOrderReportFiles(node, "# report", dataset)
	if len(files) != 2 || len(files[0].Worksheets) != 4 {
		t.Fatalf("files=%#v", files)
	}
	if files[0].Worksheets[1].Name != "订单明细" || files[0].Worksheets[1].Rows[1][0] != "order-1" {
		t.Fatalf("order worksheet=%#v", files[0].Worksheets[1])
	}
	if !strings.Contains(files[1].Content, "order-1") {
		t.Fatalf("csv=%q", files[1].Content)
	}
}

func TestAgentNodeHasDownstreamOrderReportData(t *testing.T) {
	nodes := []agentBlueprintGraphNode{{ID: "report"}, {ID: "order"}}
	nodes[0].Data.Kind = "report"
	nodes[1].Data.Kind = "custom"
	nodes[1].Data.ComponentID = "component.order-report-data"
	if !agentNodeHasDownstreamReportData("report", nodes, []agentBlueprintGraphEdge{{Source: "report", Target: "order"}}) {
		t.Fatal("expected order report data processor to be discovered")
	}
}
