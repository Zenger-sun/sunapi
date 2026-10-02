package sunapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

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

func TestBuildAgentWebhookInputRedactsSensitiveMetadata(t *testing.T) {
	req := httptest.NewRequest("POST", "/hooks/support?source=crm&token=secret&email=user@example.com&mobile=13800138000", strings.NewReader(`{"ticket":"T-42"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "req-1")
	req.Header.Set("X-Customer-Phone", "13800138000")
	req.Header.Set("Authorization", "Bearer should-not-be-captured")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	input, err := buildAgentWebhookInput(c)
	if err != nil {
		t.Fatalf("buildAgentWebhookInput error = %v", err)
	}
	headers := input["headers"].(map[string]string)
	if _, ok := headers["Authorization"]; ok {
		t.Fatal("authorization header was captured")
	}
	if _, ok := headers["X-Customer-Phone"]; ok {
		t.Fatal("customer phone header was captured")
	}
	query := input["query"].(map[string]string)
	for _, key := range []string{"token", "email", "mobile"} {
		if _, ok := query[key]; ok {
			t.Fatalf("sensitive query parameter %q was captured", key)
		}
	}
	if headers["X-Request-Id"] != "req-1" {
		t.Fatalf("safe header missing: %#v", headers)
	}
}
