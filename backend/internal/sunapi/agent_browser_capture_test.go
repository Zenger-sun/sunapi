package sunapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteAgentBrowserScreenshot(t *testing.T) {
	dataDir := t.TempDir()
	encoded := base64.StdEncoding.EncodeToString([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	capturedAt := time.Date(2026, 7, 5, 10, 11, 12, 13, time.UTC)

	path, err := writeAgentBrowserScreenshot(dataDir, "https://seller.goofish.com/?site=COMM", capturedAt, encoded)
	if err != nil {
		t.Fatalf("writeAgentBrowserScreenshot error=%v", err)
	}
	if !strings.HasPrefix(path, "data/agent-browser-captures/seller-goofish-com-20260705-101112-") {
		t.Fatalf("path=%q does not include expected data-relative capture prefix", path)
	}
	if !strings.HasSuffix(path, ".png") {
		t.Fatalf("path=%q does not end with .png", path)
	}

	relative := strings.TrimPrefix(path, "data/")
	info, err := os.Stat(filepath.Join(dataDir, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatalf("expected screenshot file to exist: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("expected screenshot file to contain bytes")
	}
}

func TestWriteAgentBrowserScreenshotRejectsInvalidData(t *testing.T) {
	_, err := writeAgentBrowserScreenshot(t.TempDir(), "https://example.com", time.Now(), "not base64")
	if err == nil {
		t.Fatal("expected invalid screenshot data to fail")
	}
}

func TestOpenAgentBrowserTargetAlwaysCreatesNewPage(t *testing.T) {
	listCalls := 0
	newCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json/list":
			listCalls++
			http.Error(w, "page reuse is disabled", http.StatusInternalServerError)
		case "/json/new":
			newCalls++
			_ = json.NewEncoder(w).Encode(agentBrowserTarget{
				ID: "new-page", Type: "page", URL: "https://example.com", WebSocketDebuggerURL: "ws://new-page",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	target, err := openAgentBrowserTarget(t.Context(), server.URL, "https://example.com")
	if err != nil {
		t.Fatalf("openAgentBrowserTarget error = %v", err)
	}
	if target.ID != "new-page" {
		t.Fatalf("target ID = %q, want new-page", target.ID)
	}
	if listCalls != 0 || newCalls != 1 {
		t.Fatalf("list calls = %d, new calls = %d; want 0 and 1", listCalls, newCalls)
	}
}

func TestFindAgentBrowserTargetReusesMatchingHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/list" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]agentBrowserTarget{
			{ID: "other", Type: "page", URL: "https://example.com", WebSocketDebuggerURL: "ws://other"},
			{ID: "seller", Type: "page", URL: "https://seller.goofish.com/home", WebSocketDebuggerURL: "ws://seller"},
		})
	}))
	t.Cleanup(server.Close)

	target, found, err := findAgentBrowserTarget(t.Context(), server.URL, "https://seller.goofish.com/data")
	if err != nil {
		t.Fatalf("findAgentBrowserTarget error = %v", err)
	}
	if !found || target.ID != "seller" {
		t.Fatalf("target=%+v found=%v, want seller target", target, found)
	}
}

func TestFindAgentBrowserTargetByID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/list" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]agentBrowserTarget{
			{ID: "old-page", Type: "page", URL: "https://seller.goofish.com/data", WebSocketDebuggerURL: "ws://old"},
			{ID: "current-page", Type: "page", URL: "https://seller.goofish.com/data", WebSocketDebuggerURL: "ws://current"},
		})
	}))
	t.Cleanup(server.Close)

	target, found, err := findAgentBrowserTargetByID(t.Context(), server.URL, "current-page")
	if err != nil {
		t.Fatalf("findAgentBrowserTargetByID error = %v", err)
	}
	if !found || target.ID != "current-page" {
		t.Fatalf("target=%+v found=%v, want current-page", target, found)
	}
}

func TestCloseAgentBrowserTarget(t *testing.T) {
	closedPath := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		closedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	if err := closeAgentBrowserTarget(t.Context(), server.URL, "current-page"); err != nil {
		t.Fatalf("closeAgentBrowserTarget error = %v", err)
	}
	if closedPath != "/json/close/current-page" {
		t.Fatalf("closed path=%q", closedPath)
	}
}

func TestLooksLikeAgentBrowserLogin(t *testing.T) {
	if !looksLikeAgentBrowserLogin(agentBrowserTextPayload{
		URL:  "https://seller.example.test/login",
		Text: "手机号 验证码 登录",
	}) {
		t.Fatal("expected login page to be detected")
	}
	if looksLikeAgentBrowserLogin(agentBrowserTextPayload{
		URL:   "https://seller.example.test/data",
		Title: "数据总览",
		Text:  "商品数据 曝光次数 支付金额",
	}) {
		t.Fatal("did not expect authenticated data page to be detected as login")
	}
}
