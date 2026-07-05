package sunapi

import "testing"

func TestFilterAgentUpstreamRowsByCondition(t *testing.T) {
	upstream := map[string]interface{}{
		"collector": map[string]interface{}{
			"rows": []interface{}{
				map[string]interface{}{"name": "A", "status": "ok", "amount": 2.0},
				map[string]interface{}{"name": "B", "status": "draft", "amount": 8.0},
				map[string]interface{}{"name": "C", "status": "ok", "amount": 11.0},
			},
		},
	}
	rows, err := filterAgentUpstreamRows(upstream, "amount>=8")
	if err != nil {
		t.Fatalf("filterAgentUpstreamRows returned error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows)=%d want 2", len(rows))
	}
	if got := rows[0]["name"]; got != "B" {
		t.Fatalf("first row name=%v want B", got)
	}
	if got := rows[1]["source_node"]; got != "collector" {
		t.Fatalf("source_node=%v want collector", got)
	}
}

func TestMergeAgentUpstreamRowsByFieldKeepsLatest(t *testing.T) {
	upstream := map[string]interface{}{
		"a": []interface{}{
			map[string]interface{}{"id": "1", "name": "old", "score": 1.0},
			map[string]interface{}{"id": "2", "name": "two", "score": 2.0},
		},
		"b": []interface{}{
			map[string]interface{}{"id": "1", "name": "new", "extra": true},
		},
	}
	rows, mode, conflicts, err := mergeAgentUpstreamRows(upstream, "按字段", "id", "保留最新")
	if err != nil {
		t.Fatalf("mergeAgentUpstreamRows returned error: %v", err)
	}
	if mode != "按字段合并" {
		t.Fatalf("mode=%q want 按字段合并", mode)
	}
	if conflicts != 1 {
		t.Fatalf("conflicts=%d want 1", conflicts)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows)=%d want 2", len(rows))
	}
	if got := rows[0]["name"]; got != "new" {
		t.Fatalf("merged name=%v want new", got)
	}
	if got := rows[0]["extra"]; got != true {
		t.Fatalf("merged extra=%v want true", got)
	}
}

func TestMergeAgentUpstreamRowsAppend(t *testing.T) {
	upstream := map[string]interface{}{
		"a": []interface{}{map[string]interface{}{"id": "1"}},
		"b": []interface{}{map[string]interface{}{"id": "1"}},
	}
	rows, mode, conflicts, err := mergeAgentUpstreamRows(upstream, "追加", "id", "保留最新")
	if err != nil {
		t.Fatalf("mergeAgentUpstreamRows returned error: %v", err)
	}
	if mode != "追加合并" {
		t.Fatalf("mode=%q want 追加合并", mode)
	}
	if conflicts != 0 {
		t.Fatalf("conflicts=%d want 0", conflicts)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows)=%d want 2", len(rows))
	}
}
