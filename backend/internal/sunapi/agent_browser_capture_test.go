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

	"github.com/gorilla/websocket"
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

func TestResolveAgentBrowserSessionPathsKeepsLegacyDefaultProfile(t *testing.T) {
	dataDir := t.TempDir()
	session := resolveAgentBrowserSessionPaths(dataDir, agentBrowserProfileDir)
	if session.Name != agentBrowserDefaultSession {
		t.Fatalf("session name = %q, want %q", session.Name, agentBrowserDefaultSession)
	}
	if session.ProfileDir != filepath.Join(dataDir, agentBrowserProfileDir) {
		t.Fatalf("profile path = %q", session.ProfileDir)
	}
	if session.PortPath != filepath.Join(dataDir, agentBrowserPortFile) {
		t.Fatalf("port path = %q", session.PortPath)
	}
}

func TestResolveAgentBrowserSessionPathsSeparatesNamedSessions(t *testing.T) {
	dataDir := t.TempDir()
	session := resolveAgentBrowserSessionPaths(dataDir, "日报 账号/一")
	if session.Name != "日报-账号-一" {
		t.Fatalf("session name = %q", session.Name)
	}
	if session.ProfileDir != filepath.Join(dataDir, agentBrowserSessionsDir, session.Name, "profile") {
		t.Fatalf("profile path = %q", session.ProfileDir)
	}
	if session.PortPath != filepath.Join(dataDir, agentBrowserSessionsDir, session.Name, "devtools-port") {
		t.Fatalf("port path = %q", session.PortPath)
	}
}

func TestAgentBrowserProfileLockExists(t *testing.T) {
	profileDir := t.TempDir()
	if agentBrowserProfileLockExists(profileDir) {
		t.Fatal("empty profile should not be considered locked")
	}
	if err := os.WriteFile(filepath.Join(profileDir, "SingletonLock"), []byte("locked"), 0600); err != nil {
		t.Fatalf("write profile lock: %v", err)
	}
	if !agentBrowserProfileLockExists(profileDir) {
		t.Fatal("profile lock should be detected")
	}
}

func TestOpenAgentBrowserTargetCreatesPageWhenNoReusablePage(t *testing.T) {
	listCalls := 0
	newCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json/list":
			listCalls++
			http.Error(w, "temporary target list failure", http.StatusInternalServerError)
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

	target, reused, err := openAgentBrowserTarget(t.Context(), server.URL, "https://example.com")
	if err != nil {
		t.Fatalf("openAgentBrowserTarget error = %v", err)
	}
	if target.ID != "new-page" {
		t.Fatalf("target ID = %q, want new-page", target.ID)
	}
	if reused {
		t.Fatal("newly created page was reported as reused")
	}
	if listCalls != 1 || newCalls != 1 {
		t.Fatalf("list calls = %d, new calls = %d; want 1 and 1", listCalls, newCalls)
	}
}

func TestOpenAgentBrowserTargetReusesHealthyMatchingPage(t *testing.T) {
	upgrader := websocket.Upgrader{}
	browser := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			var request struct {
				ID int `json:"id"`
			}
			if err := conn.ReadJSON(&request); err != nil {
				return
			}
			if err := conn.WriteJSON(map[string]interface{}{
				"id":     request.ID,
				"result": map[string]interface{}{},
			}); err != nil {
				return
			}
		}
	}))
	t.Cleanup(browser.Close)
	websocketURL := "ws" + strings.TrimPrefix(browser.URL, "http") + "/devtools/page/session"

	newCalls := 0
	devtools := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json/list":
			_ = json.NewEncoder(w).Encode([]agentBrowserTarget{{
				ID:                   "persisted-page",
				Type:                 "page",
				URL:                  "https://seller.example.test/data",
				WebSocketDebuggerURL: websocketURL,
			}})
		case "/json/new":
			newCalls++
			http.Error(w, "should not create a page", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(devtools.Close)

	target, reused, err := openAgentBrowserTarget(t.Context(), devtools.URL, "https://seller.example.test/data")
	if err != nil {
		t.Fatalf("openAgentBrowserTarget error = %v", err)
	}
	if target.ID != "persisted-page" {
		t.Fatalf("target ID = %q, want persisted-page", target.ID)
	}
	if !reused {
		t.Fatal("persisted page was reported as newly created")
	}
	if newCalls != 0 {
		t.Fatalf("new target calls = %d, want 0", newCalls)
	}
}

func TestFindAgentBrowserTargetRequiresMatchingURLFamily(t *testing.T) {
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
		t.Fatalf("target=%+v found=%v, want seller target from the same seller console", target, found)
	}
}

func TestAgentBrowserTargetURLMatchesSellerSiteVariants(t *testing.T) {
	if !agentBrowserTargetURLMatches(
		"https://seller.goofish.com/?site=COMMONPRO&spm=tracking#/im",
		"https://seller.goofish.com/?site=COMM",
	) {
		t.Fatal("expected seller console site variants to reuse the current page")
	}
	if !agentBrowserTargetURLMatches(
		"https://seller.goofish.com/messages?site=COMMONPRO",
		"https://seller.goofish.com/?site=COMM",
	) {
		t.Fatal("expected different seller console routes to reuse the current page")
	}
	if agentBrowserTargetURLMatches(
		"https://seller.example.com/messages?site=COMMONPRO",
		"https://seller.goofish.com/?site=COMM",
	) {
		t.Fatal("did not expect a different seller host to match")
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

func TestMakeAgentBrowserTargetCleanupIsOwnedAndIdempotent(t *testing.T) {
	closeCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/close/owned-page" {
			http.NotFound(w, r)
			return
		}
		closeCalls++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	cleanup := makeAgentBrowserTargetCleanup(server.URL, agentBrowserTarget{ID: "owned-page"}, true)
	cleanup()
	cleanup()
	if closeCalls != 1 {
		t.Fatalf("close calls=%d, want 1", closeCalls)
	}
}

func TestMakeAgentBrowserTargetCleanupKeepsReusedPage(t *testing.T) {
	closeCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		closeCalls++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	cleanup := makeAgentBrowserTargetCleanup(server.URL, agentBrowserTarget{ID: "reused-page"}, false)
	cleanup()
	if closeCalls != 0 {
		t.Fatalf("close calls=%d, want 0", closeCalls)
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
