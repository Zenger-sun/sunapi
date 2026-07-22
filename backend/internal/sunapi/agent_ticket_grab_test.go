package sunapi

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestParseAgentTicketGrabConfigClampsPollingAndBlocksPaymentLabels(t *testing.T) {
	config := parseAgentTicketGrabConfig(map[string]string{
		"轮询间隔": "100ms",
		"最大轮询": "9999",
		"最长运行": "2h",
		"购票按钮": "立即购买,提交订单,确认支付,去购票",
	})
	if config.Interval != agentTicketMinInterval {
		t.Fatalf("interval = %s, want %s", config.Interval, agentTicketMinInterval)
	}
	if config.MaxAttempts != agentTicketMaxAttempts {
		t.Fatalf("max attempts = %d, want %d", config.MaxAttempts, agentTicketMaxAttempts)
	}
	if config.MaxDuration != agentTicketMaxDuration {
		t.Fatalf("max duration = %s, want %s", config.MaxDuration, agentTicketMaxDuration)
	}
	if strings.Join(config.ClickLabels, ",") != "立即购买,去购票" {
		t.Fatalf("safe click labels = %#v", config.ClickLabels)
	}
}

func TestMatchAgentTicketPageUsesFiltersAndVisibleActions(t *testing.T) {
	config := parseAgentTicketGrabConfig(map[string]string{
		"场次/票档关键词": "周六,内场",
		"有票关键词":    "立即购买,有票",
		"无票关键词":    "售罄,无票",
	})

	missingFilter := matchAgentTicketPage(agentTicketPagePayload{
		Text:    "周日 内场",
		Actions: []string{"立即购买"},
	}, config)
	if missingFilter.Found {
		t.Fatal("page without all requested filters was matched")
	}

	soldOut := matchAgentTicketPage(agentTicketPagePayload{
		Text: "周六 内场 暂时无票",
	}, config)
	if soldOut.Found {
		t.Fatal("sold-out page was matched")
	}

	available := matchAgentTicketPage(agentTicketPagePayload{
		Text:    "周六 内场；其他票档已售罄",
		Actions: []string{"周六内场 立即购买"},
	}, config)
	if !available.Found || available.Keyword != "立即购买" {
		t.Fatalf("available match = %#v", available)
	}
}

func TestExecuteAgentTicketGrabStopsBeforeFinalSubmission(t *testing.T) {
	var pageEvaluations atomic.Int32
	upgrader := websocket.Upgrader{}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json/version":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"Browser": "test"})
		case "/json/list":
			_ = json.NewEncoder(w).Encode([]agentBrowserTarget{{
				ID:                   "ticket-page",
				Type:                 "page",
				URL:                  "https://tickets.example.test/show",
				WebSocketDebuggerURL: "ws" + strings.TrimPrefix(server.URL, "http") + "/devtools/page/ticket",
			}})
		case "/devtools/page/ticket":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			for {
				var request struct {
					ID     int                    `json:"id"`
					Method string                 `json:"method"`
					Params map[string]interface{} `json:"params"`
				}
				if err := conn.ReadJSON(&request); err != nil {
					return
				}
				result := map[string]interface{}{}
				switch request.Method {
				case "Runtime.evaluate":
					expression, _ := request.Params["expression"].(string)
					value := "complete"
					if strings.Contains(expression, "actions:Array.from") {
						if pageEvaluations.Add(1) == 1 {
							value = `{"title":"演出详情","url":"https://tickets.example.test/show","text":"周六 内场","actions":["立即购买"]}`
						} else {
							value = `{"title":"确认订单","url":"https://tickets.example.test/order/confirm","text":"请核对购票人和票档","actions":["提交订单"]}`
						}
					} else if strings.Contains(expression, "candidates.find") {
						if strings.Contains(expression, "确认支付") {
							value = `{"clicked":true,"label":"立即购买"}`
						}
					}
					result = map[string]interface{}{"result": map[string]interface{}{"value": value}}
				case "Page.captureScreenshot":
					result = map[string]interface{}{"data": base64.StdEncoding.EncodeToString([]byte{0x89, 'P', 'N', 'G'})}
				}
				if err := conn.WriteJSON(map[string]interface{}{"id": request.ID, "result": result}); err != nil {
					return
				}
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(parsedURL.Host)
	if err != nil {
		t.Fatal(err)
	}
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, agentBrowserPortFile), []byte(port), 0644); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(filepath.Join(dataDir, "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	node := agentBlueprintGraphNode{ID: "ticket-grab"}
	node.Data.ComponentID = "component.ticket_grab"
	node.Data.Settings = map[string]string{
		"场次/票档关键词": "周六,内场",
		"轮询间隔":     "3s",
		"最大轮询":     "1",
		"最长运行":     "10s",
		"找到后动作":    "进入确认页后停止",
	}
	result := executeAgentTicketGrabGraphNode(
		t.Context(),
		store,
		node,
		map[string]interface{}{"browser": map[string]interface{}{
			"final_url":         "https://tickets.example.test/show",
			"browser_session":   "default",
			"browser_target_id": "ticket-page",
		}},
		agentBlueprintNodeResult{Status: "completed"},
	)
	if result.Status != "completed" {
		t.Fatalf("status = %q, error = %s", result.Status, result.Error)
	}
	if result.Output["found"] != true || result.Output["clicked"] != true {
		t.Fatalf("ticket result = %#v", result.Output)
	}
	if result.Output["requires_confirmation"] != true || result.Output["final_submit_clicked"] != false {
		t.Fatalf("confirmation boundary = %#v", result.Output)
	}
	if result.Output["payment_attempted"] != false {
		t.Fatalf("payment boundary = %#v", result.Output)
	}
}

func TestAgentTicketPagePausesForVerificationAndQueue(t *testing.T) {
	if !agentTicketPageNeedsManualVerification(agentTicketPagePayload{Text: "请完成滑块验证"}) {
		t.Fatal("expected manual verification to be detected")
	}
	if !agentTicketPageIsQueued(agentTicketPagePayload{Text: "正在排队，请稍候"}) {
		t.Fatal("expected queue page to be detected")
	}
	if agentTicketPageNeedsManualVerification(agentTicketPagePayload{Text: "周六场次可以购买"}) {
		t.Fatal("normal ticket page was treated as a verification page")
	}
}

func TestParseAgentTicketDurationUsesFallback(t *testing.T) {
	if got := parseAgentTicketDuration("invalid", 5*time.Second, time.Second, time.Minute); got != 5*time.Second {
		t.Fatalf("duration = %s", got)
	}
}

func TestAgentBlueprintExecutionTimeoutIncludesTicketRunDuration(t *testing.T) {
	node := agentBlueprintGraphNode{ID: "ticket"}
	node.Data.Kind = "custom"
	node.Data.ComponentID = "component.ticket_grab"
	node.Data.Settings = map[string]string{"最长运行": "30m"}
	nodes, err := json.Marshal([]agentBlueprintGraphNode{node})
	if err != nil {
		t.Fatal(err)
	}
	if got := agentBlueprintExecutionTimeout(&AgentBlueprint{Nodes: nodes}); got != 32*time.Minute {
		t.Fatalf("blueprint timeout = %s, want 32m", got)
	}
	if got := agentBlueprintExecutionTimeout(nil); got != agentBlueprintRunTimeout {
		t.Fatalf("default timeout = %s, want %s", got, agentBlueprintRunTimeout)
	}
}

func TestBuiltinAgentComponentsIncludesTicketGrabTemplate(t *testing.T) {
	for _, component := range builtinAgentComponents {
		if component.ID != "component.ticket_grab" {
			continue
		}
		if component.Group != "template" || component.Kind != "custom" {
			t.Fatalf("ticket component = %#v", component)
		}
		return
	}
	t.Fatal("component.ticket_grab is missing from the built-in catalog")
}
