package sunapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	agentBrowserProfileDir        = "agent-browser-profile"
	agentBrowserPortFile          = "agent-browser-devtools-port"
	agentBrowserCaptureDir        = "agent-browser-captures"
	agentBrowserCaptureMaxRunes   = 200000
	agentBrowserCaptureMaxWait    = 30
	agentBrowserCaptureMinWait    = 1
	agentBrowserCaptureDefault    = 6
	agentBrowserDevtoolsWaitTime  = 12 * time.Second
	agentBrowserProbeTimeout      = 3 * time.Second
	agentBrowserScreenshotTimeout = 8 * time.Second
)

type AgentBrowserCaptureRequest struct {
	URL              string `json:"url"`
	LoginState       string `json:"login_state,omitempty"`
	WaitSeconds      int    `json:"wait_seconds,omitempty"`
	LoginWaitSeconds int    `json:"login_wait_seconds,omitempty"`
}

type AgentBrowserCaptureResult struct {
	URL             string `json:"url"`
	FinalURL        string `json:"final_url"`
	Title           string `json:"title"`
	Text            string `json:"text"`
	ScreenshotPath  string `json:"screenshot_path,omitempty"`
	ScreenshotError string `json:"screenshot_error,omitempty"`
	CapturedAt      int64  `json:"captured_at"`
	ElapsedMS       int64  `json:"elapsed_ms"`
	BrowserTargetID string `json:"browser_target_id,omitempty"`
}

type agentBrowserTarget struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	Title                string `json:"title"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

func handleCaptureAgentBrowserPage(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req AgentBrowserCaptureRequest
		if !bindJSON(c, &req) {
			return
		}
		if store == nil || strings.TrimSpace(store.DataDir()) == "" {
			fail(c, http.StatusInternalServerError, errors.New("data directory is not configured"))
			return
		}

		timeout := 75 * time.Second
		if req.LoginWaitSeconds > 0 {
			timeout += time.Duration(req.LoginWaitSeconds) * time.Second
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		result, err := captureAgentBrowserPage(ctx, store.DataDir(), req)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, result)
	}
}

func captureAgentBrowserPage(ctx context.Context, dataDir string, req AgentBrowserCaptureRequest) (AgentBrowserCaptureResult, error) {
	started := time.Now()
	targetURL, err := normalizeAgentBrowserURL(req.URL)
	if err != nil {
		return AgentBrowserCaptureResult{}, err
	}

	endpoint, err := ensureAgentBrowserDevtools(ctx, dataDir)
	if err != nil {
		return AgentBrowserCaptureResult{}, err
	}

	target, err := openAgentBrowserTarget(ctx, endpoint, targetURL)
	if err != nil {
		return AgentBrowserCaptureResult{}, err
	}

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, target.WebSocketDebuggerURL, nil)
	if err != nil {
		return AgentBrowserCaptureResult{}, fmt.Errorf("连接浏览器调试通道失败: %w", err)
	}
	defer conn.Close()

	if _, err := agentBrowserCDPCall(ctx, conn, 1, "Page.enable", nil); err != nil {
		return AgentBrowserCaptureResult{}, err
	}
	if _, err := agentBrowserCDPCall(ctx, conn, 2, "Runtime.enable", nil); err != nil {
		return AgentBrowserCaptureResult{}, err
	}
	if _, err := agentBrowserCDPCall(ctx, conn, 3, "Page.navigate", map[string]any{"url": targetURL}); err != nil {
		return AgentBrowserCaptureResult{}, err
	}
	if req.LoginWaitSeconds > 0 && !strings.Contains(req.LoginState, "无需登录") {
		if err := waitAgentBrowserLogin(ctx, conn, req.LoginWaitSeconds); err != nil {
			return AgentBrowserCaptureResult{}, err
		}
	}

	waitSeconds := normalizeAgentBrowserWaitSeconds(req.WaitSeconds)
	select {
	case <-time.After(time.Duration(waitSeconds) * time.Second):
	case <-ctx.Done():
		return AgentBrowserCaptureResult{}, ctx.Err()
	}

	payload, err := evaluateAgentBrowserText(ctx, conn, 4)
	if err != nil {
		return AgentBrowserCaptureResult{}, err
	}

	text := truncateRunes(strings.TrimSpace(payload.Text), agentBrowserCaptureMaxRunes)
	if text == "" {
		return AgentBrowserCaptureResult{}, errors.New("浏览器已打开页面，但页面正文为空；请确认页面已登录并完成加载")
	}

	capturedAt := time.Now()
	finalURL := strings.TrimSpace(payload.URL)
	if finalURL == "" {
		finalURL = targetURL
	}
	screenshotPath := ""
	screenshotError := ""
	if screenshotData, err := captureAgentBrowserScreenshot(ctx, conn, 5); err != nil {
		screenshotError = err.Error()
	} else if path, err := writeAgentBrowserScreenshot(dataDir, finalURL, capturedAt, screenshotData); err != nil {
		screenshotError = err.Error()
	} else {
		screenshotPath = path
	}

	return AgentBrowserCaptureResult{
		URL:             targetURL,
		FinalURL:        finalURL,
		Title:           payload.Title,
		Text:            text,
		ScreenshotPath:  screenshotPath,
		ScreenshotError: screenshotError,
		CapturedAt:      capturedAt.Unix(),
		ElapsedMS:       time.Since(started).Milliseconds(),
		BrowserTargetID: target.ID,
	}, nil
}

func normalizeAgentBrowserURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("网址不能为空")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("网址格式不正确")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("浏览器取数仅支持 http/https 页面")
	}
	return parsed.String(), nil
}

func reserveAgentBrowserPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok || addr.Port <= 0 {
		return 0, errors.New("无法分配浏览器调试端口")
	}
	return addr.Port, nil
}

func ensureAgentBrowserDevtools(ctx context.Context, dataDir string) (string, error) {
	portPath := filepath.Join(dataDir, agentBrowserPortFile)
	if raw, err := os.ReadFile(portPath); err == nil {
		if port := strings.TrimSpace(string(raw)); port != "" {
			endpoint := "http://127.0.0.1:" + port
			if waitAgentBrowserDevtoolsQuick(ctx, endpoint) == nil {
				return endpoint, nil
			}
		}
	}

	port, err := reserveAgentBrowserPort()
	if err != nil {
		return "", err
	}
	endpoint := fmt.Sprintf("http://127.0.0.1:%d", port)

	browserPath, err := findAgentBrowserExecutable()
	if err != nil {
		return "", err
	}

	profileDir := filepath.Join(dataDir, agentBrowserProfileDir)
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		return "", err
	}

	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--user-data-dir=" + profileDir,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
		"--window-size=1280,900",
		"about:blank",
	}
	cmd := exec.Command(browserPath, args...)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("启动浏览器失败: %w", err)
	}

	if err := waitAgentBrowserDevtools(ctx, endpoint); err != nil {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
		return "", err
	}
	_ = cmd.Process.Release()
	_ = os.WriteFile(portPath, []byte(fmt.Sprintf("%d", port)), 0644)
	return endpoint, nil
}

func waitAgentBrowserDevtoolsQuick(ctx context.Context, endpoint string) error {
	quickCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	return waitAgentBrowserDevtools(quickCtx, endpoint)
}

func findAgentBrowserExecutable() (string, error) {
	candidates := []string{}
	if runtime.GOOS == "windows" {
		for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LocalAppData"} {
			if base := strings.TrimSpace(os.Getenv(env)); base != "" {
				candidates = append(candidates, filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"))
				candidates = append(candidates, filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"))
			}
		}
	}
	candidates = append(candidates, "msedge", "msedge.exe", "chrome", "chrome.exe", "google-chrome", "chromium")

	for _, candidate := range candidates {
		if strings.ContainsAny(candidate, `/\`) {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", errors.New("未找到 Edge 或 Chrome 浏览器，请先安装浏览器后重试")
}

func waitAgentBrowserDevtools(ctx context.Context, endpoint string) error {
	deadline := time.Now().Add(agentBrowserDevtoolsWaitTime)
	client := http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/json/version", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		select {
		case <-time.After(250 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return errors.New("浏览器调试端口启动超时")
}

func openAgentBrowserTarget(ctx context.Context, endpoint, targetURL string) (agentBrowserTarget, error) {
	return createAgentBrowserTarget(ctx, endpoint, targetURL)
}

func findAgentBrowserTarget(ctx context.Context, endpoint, targetURL string) (agentBrowserTarget, bool, error) {
	targets, err := listAgentBrowserTargets(ctx, endpoint)
	if err != nil {
		return agentBrowserTarget{}, false, err
	}
	wantedHost := ""
	if parsed, err := url.Parse(targetURL); err == nil {
		wantedHost = strings.ToLower(parsed.Hostname())
	}
	for _, target := range targets {
		if target.Type != "page" || target.WebSocketDebuggerURL == "" {
			continue
		}
		if strings.TrimSpace(target.URL) == strings.TrimSpace(targetURL) {
			return target, true, nil
		}
		if wantedHost != "" {
			if parsed, err := url.Parse(target.URL); err == nil && strings.EqualFold(parsed.Hostname(), wantedHost) {
				return target, true, nil
			}
		}
	}
	return agentBrowserTarget{}, false, nil
}

func findAgentBrowserTargetByID(ctx context.Context, endpoint, targetID string) (agentBrowserTarget, bool, error) {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return agentBrowserTarget{}, false, nil
	}
	targets, err := listAgentBrowserTargets(ctx, endpoint)
	if err != nil {
		return agentBrowserTarget{}, false, err
	}
	for _, target := range targets {
		if target.ID == targetID && target.Type == "page" && target.WebSocketDebuggerURL != "" {
			return target, true, nil
		}
	}
	return agentBrowserTarget{}, false, nil
}

func listAgentBrowserTargets(ctx context.Context, endpoint string) ([]agentBrowserTarget, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/json/list", nil)
	if err != nil {
		return nil, err
	}
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("读取浏览器页面列表失败: %s", resp.Status)
	}
	var targets []agentBrowserTarget
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, err
	}
	return targets, nil
}

func probeAgentBrowserTarget(ctx context.Context, target agentBrowserTarget) error {
	probeCtx, cancel := context.WithTimeout(ctx, agentBrowserProbeTimeout)
	defer cancel()
	conn, _, err := websocket.DefaultDialer.DialContext(probeCtx, target.WebSocketDebuggerURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = agentBrowserCDPCall(probeCtx, conn, 9001, "Runtime.evaluate", map[string]any{
		"expression":    "document.readyState",
		"returnByValue": true,
	})
	return err
}

func closeAgentBrowserTarget(ctx context.Context, endpoint, targetID string) error {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/json/close/"+url.PathEscape(targetID), nil)
	if err != nil {
		return err
	}
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("关闭浏览器页面失败: %s", resp.Status)
	}
	return nil
}

func captureAgentBrowserTargetScreenshot(ctx context.Context, target agentBrowserTarget) (string, error) {
	screenshotCtx, cancel := context.WithTimeout(ctx, agentBrowserScreenshotTimeout)
	defer cancel()
	conn, _, err := websocket.DefaultDialer.DialContext(screenshotCtx, target.WebSocketDebuggerURL, nil)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if _, err := agentBrowserCDPCall(screenshotCtx, conn, 9100, "Page.enable", nil); err != nil {
		return "", err
	}
	return captureAgentBrowserScreenshot(screenshotCtx, conn, 9101)
}

func createAgentBrowserTarget(ctx context.Context, endpoint, targetURL string) (agentBrowserTarget, error) {
	client := http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint+"/json/new?"+url.QueryEscape(targetURL), nil)
	if err != nil {
		return agentBrowserTarget{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return agentBrowserTarget{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return agentBrowserTarget{}, fmt.Errorf("创建浏览器页面失败: %s", resp.Status)
	}
	var target agentBrowserTarget
	if err := json.NewDecoder(resp.Body).Decode(&target); err != nil {
		return agentBrowserTarget{}, err
	}
	if target.WebSocketDebuggerURL == "" {
		return agentBrowserTarget{}, errors.New("浏览器页面缺少调试地址")
	}
	return target, nil
}

func agentBrowserCDPCall(ctx context.Context, conn *websocket.Conn, id int, method string, params map[string]any) (json.RawMessage, error) {
	deadline := time.Now().Add(30 * time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetWriteDeadline(deadline)
	_ = conn.SetReadDeadline(deadline)

	request := map[string]any{
		"id":     id,
		"method": method,
	}
	if params != nil {
		request["params"] = params
	}
	if err := conn.WriteJSON(request); err != nil {
		return nil, fmt.Errorf("发送浏览器指令失败: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		var response struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := conn.ReadJSON(&response); err != nil {
			return nil, fmt.Errorf("读取浏览器返回失败: %w", err)
		}
		if response.ID != id {
			continue
		}
		if response.Error != nil {
			return nil, fmt.Errorf("浏览器指令失败: %s", response.Error.Message)
		}
		return response.Result, nil
	}
}

type agentBrowserTextPayload struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Text  string `json:"text"`
}

func evaluateAgentBrowserText(ctx context.Context, conn *websocket.Conn, id int) (agentBrowserTextPayload, error) {
	expression := `JSON.stringify({title: document.title || "", url: location.href || "", text: document.body ? document.body.innerText : ""})`
	raw, err := agentBrowserCDPCall(ctx, conn, id, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
	})
	if err != nil {
		return agentBrowserTextPayload{}, err
	}
	var response struct {
		Result struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return agentBrowserTextPayload{}, err
	}
	if len(response.ExceptionDetails) > 0 {
		return agentBrowserTextPayload{}, errors.New("浏览器执行页面文本采集脚本失败")
	}
	var payload agentBrowserTextPayload
	if err := json.Unmarshal([]byte(response.Result.Value), &payload); err != nil {
		return agentBrowserTextPayload{}, err
	}
	return payload, nil
}

func waitAgentBrowserLogin(ctx context.Context, conn *websocket.Conn, maxWaitSeconds int) error {
	if maxWaitSeconds <= 0 {
		return nil
	}
	if maxWaitSeconds > 300 {
		maxWaitSeconds = 300
	}
	deadline := time.Now().Add(time.Duration(maxWaitSeconds) * time.Second)
	callID := 50
	for {
		payload, err := evaluateAgentBrowserText(ctx, conn, callID)
		callID++
		if err == nil && strings.TrimSpace(payload.Text) != "" && !looksLikeAgentBrowserLogin(payload) {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("等待浏览器登录超时；请在打开的小窗中完成登录后重试")
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
	}
}

func looksLikeAgentBrowserLogin(payload agentBrowserTextPayload) bool {
	location := strings.ToLower(strings.TrimSpace(payload.URL))
	for _, marker := range []string{"/login", "login.", "signin", "passport", "/auth"} {
		if strings.Contains(location, marker) {
			return true
		}
	}
	text := strings.Join([]string{payload.Title, payload.Text}, "\n")
	if !strings.Contains(text, "登录") {
		return false
	}
	for _, marker := range []string{"手机号", "验证码", "扫码登录", "密码登录", "立即登录"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func captureAgentBrowserScreenshot(ctx context.Context, conn *websocket.Conn, id int) (string, error) {
	raw, err := agentBrowserCDPCall(ctx, conn, id, "Page.captureScreenshot", map[string]any{
		"format":                "png",
		"fromSurface":           true,
		"captureBeyondViewport": true,
	})
	if err != nil {
		raw, err = agentBrowserCDPCall(ctx, conn, id+1, "Page.captureScreenshot", map[string]any{
			"format":      "png",
			"fromSurface": true,
		})
	}
	if err != nil {
		return "", err
	}
	var response struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.Data) == "" {
		return "", errors.New("浏览器未返回截图数据")
	}
	return response.Data, nil
}

func writeAgentBrowserScreenshot(dataDir, targetURL string, capturedAt time.Time, encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", fmt.Errorf("解析浏览器截图失败: %w", err)
	}
	if len(data) == 0 {
		return "", errors.New("浏览器截图为空")
	}
	host := "page"
	if parsed, err := url.Parse(targetURL); err == nil && strings.TrimSpace(parsed.Hostname()) != "" {
		host = parsed.Hostname()
	}
	filename := fmt.Sprintf(
		"%s-%s.png",
		sanitizeAgentBrowserFilenamePart(host),
		capturedAt.UTC().Format("20060102-150405-000000000"),
	)
	relPath := filepath.Join(agentBrowserCaptureDir, filename)
	absPath := filepath.Join(dataDir, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(absPath, data, 0644); err != nil {
		return "", err
	}
	return "data/" + filepath.ToSlash(relPath), nil
}

func sanitizeAgentBrowserFilenamePart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if valid {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(builder.String(), "-")
	if len(result) > 60 {
		result = strings.Trim(result[:60], "-")
	}
	if result == "" {
		return "page"
	}
	return result
}

func normalizeAgentBrowserWaitSeconds(value int) int {
	if value <= 0 {
		return agentBrowserCaptureDefault
	}
	if value < agentBrowserCaptureMinWait {
		return agentBrowserCaptureMinWait
	}
	if value > agentBrowserCaptureMaxWait {
		return agentBrowserCaptureMaxWait
	}
	return value
}
