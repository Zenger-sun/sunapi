package sunapi

import "testing"

func TestMergeAgentWebhookInputPreservesBlueprintKnowledgeScope(t *testing.T) {
	vars := map[string]interface{}{
		"knowledge_base_ids": []interface{}{"kb-support", "kb-orders"},
		"knowledge_disabled": false,
		"knowledge_manual":   true,
		"custom_input":       "keep",
		"webhook_body":       "stale",
	}
	payload := map[string]interface{}{
		"body":    map[string]interface{}{"ticket": "T-42"},
		"headers": map[string]string{"X-Request-ID": "req-1"},
		"query":   map[string]string{"source": "crm"},
		"method":  "POST",
		"path":    "/hooks/support",
	}

	merged := mergeAgentWebhookInput(vars, payload, "2026-08-31T12:00:00Z")

	ids, ok := merged["knowledge_base_ids"].([]interface{})
	if !ok || len(ids) != 2 || ids[0] != "kb-support" || ids[1] != "kb-orders" {
		t.Fatalf("knowledge_base_ids were not preserved: %#v", merged["knowledge_base_ids"])
	}
	if merged["knowledge_disabled"] != false || merged["knowledge_manual"] != true {
		t.Fatalf("knowledge scope flags were not preserved: disabled=%v manual=%v", merged["knowledge_disabled"], merged["knowledge_manual"])
	}
	if merged["custom_input"] != "keep" {
		t.Fatalf("custom blueprint input was not preserved: %#v", merged["custom_input"])
	}
	if merged["webhook_body"] == "stale" || merged["webhook_method"] != "POST" || merged["webhook_path"] != "/hooks/support" {
		t.Fatalf("webhook fields were not overlaid: %#v", merged)
	}
}
