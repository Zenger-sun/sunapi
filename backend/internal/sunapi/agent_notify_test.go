package sunapi

import (
	"encoding/json"
	"testing"
)

func TestBuildAgentNotificationPayloadForChatRobots(t *testing.T) {
	node := agentBlueprintGraphNode{}
	node.Data.Title = "日报通知"
	node.Data.Description = "今日日报已生成"
	upstream := map[string]interface{}{"report": map[string]interface{}{"path": "data/output/reports/a.md"}}

	cases := []struct {
		channel string
		key     string
	}{
		{channel: "Slack", key: "text"},
		{channel: "飞书", key: "msg_type"},
		{channel: "企业微信", key: "msgtype"},
		{channel: "钉钉", key: "msgtype"},
		{channel: "Webhook", key: "title"},
	}

	for _, tc := range cases {
		body, contentType, err := buildAgentNotificationPayload(tc.channel, node, upstream)
		if err != nil {
			t.Fatalf("%s payload error: %v", tc.channel, err)
		}
		if contentType != "application/json" {
			t.Fatalf("%s contentType = %q, want application/json", tc.channel, contentType)
		}
		var decoded map[string]interface{}
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("%s body is not JSON: %v", tc.channel, err)
		}
		if _, ok := decoded[tc.key]; !ok {
			t.Fatalf("%s body missing key %q: %s", tc.channel, tc.key, string(body))
		}
	}
}

func TestBuildAgentNotificationPayloadRejectsEmailWithoutAdapter(t *testing.T) {
	node := agentBlueprintGraphNode{}
	if _, _, err := buildAgentNotificationPayload("邮件", node, nil); err == nil {
		t.Fatal("expected email payload to fail without SMTP adapter")
	}
}
