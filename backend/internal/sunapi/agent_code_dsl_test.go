package sunapi

import "testing"

func TestRunAgentCodeDSLFiltersSortsAndLimitsRows(t *testing.T) {
	upstream := map[string]interface{}{
		"source": map[string]interface{}{
			"rows": []interface{}{
				map[string]interface{}{"name": "A", "amount": 3.0, "status": "ok"},
				map[string]interface{}{"name": "B", "amount": 9.0, "status": "ok"},
				map[string]interface{}{"name": "C", "amount": 1.0, "status": "draft"},
			},
		},
	}

	result, err := runAgentCodeDSL("filter status=ok\nsort -amount\nselect name, amount\nlimit 1", upstream)
	if err != nil {
		t.Fatalf("runAgentCodeDSL returned error: %v", err)
	}
	rows, ok := result.Value.([]map[string]interface{})
	if !ok {
		t.Fatalf("result.Value = %T, want []map[string]interface{}", result.Value)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if got := rows[0]["name"]; got != "B" {
		t.Fatalf("name = %v, want B", got)
	}
	if got := rows[0]["amount"]; got != 9.0 {
		t.Fatalf("amount = %v, want 9", got)
	}
}

func TestRunAgentCodeDSLSumsNumericField(t *testing.T) {
	upstream := map[string]interface{}{
		"source": []interface{}{
			map[string]interface{}{"amount": "2.5"},
			map[string]interface{}{"amount": 7.5},
			map[string]interface{}{"amount": "ignore"},
		},
	}

	result, err := runAgentCodeDSL("sum amount", upstream)
	if err != nil {
		t.Fatalf("runAgentCodeDSL returned error: %v", err)
	}
	summary, ok := result.Value.(map[string]interface{})
	if !ok {
		t.Fatalf("result.Value = %T, want map[string]interface{}", result.Value)
	}
	if got := summary["sum"]; got != 10.0 {
		t.Fatalf("sum = %v, want 10", got)
	}
	if got := summary["count"]; got != 2 {
		t.Fatalf("count = %v, want 2", got)
	}
}
