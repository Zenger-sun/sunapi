package sunapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	maxAgentHTTPResponseBytes = 1024 * 1024
	agentHTTPRequestTimeout   = 30 * time.Second
)

type AgentHTTPRequest struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
	Timeout int               `json:"timeout_seconds,omitempty"`
}

type AgentHTTPResponse struct {
	URL         string            `json:"url"`
	FinalURL    string            `json:"final_url"`
	Method      string            `json:"method"`
	StatusCode  int               `json:"status_code"`
	Headers     map[string]string `json:"headers"`
	Body        string            `json:"body"`
	BodyBytes   int               `json:"body_bytes"`
	Truncated   bool              `json:"truncated"`
	ContentType string            `json:"content_type,omitempty"`
	DurationMs  int64             `json:"duration_ms"`
	ExecutedAt  int64             `json:"executed_at"`
}

func handleAgentHTTPRequest(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req AgentHTTPRequest
		if !bindJSON(c, &req) {
			return
		}
		result, err := executeAgentHTTPRequest(c.Request.Context(), req)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, result)
	}
}

func executeAgentHTTPRequest(ctx context.Context, req AgentHTTPRequest) (AgentHTTPResponse, error) {
	targetURL := strings.TrimSpace(req.URL)
	if targetURL == "" {
		return AgentHTTPResponse{}, errors.New("url is required")
	}
	parsed, err := url.Parse(targetURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return AgentHTTPResponse{}, errors.New("invalid url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return AgentHTTPResponse{}, errors.New("only http and https URLs are supported")
	}
	if err := rejectPrivateAgentURL(parsed); err != nil {
		return AgentHTTPResponse{}, err
	}

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return AgentHTTPResponse{}, fmt.Errorf("unsupported method %s", method)
	}
	timeout := req.Timeout
	if timeout <= 0 || timeout > 60 {
		timeout = int(agentHTTPRequestTimeout.Seconds())
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	startedAt := time.Now()
	httpReq, err := http.NewRequestWithContext(ctx, method, parsed.String(), strings.NewReader(req.Body))
	if err != nil {
		return AgentHTTPResponse{}, err
	}
	httpReq.Header.Set("User-Agent", "SunAPI-Agent-HTTP/1.0")
	if req.Body != "" && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	for key, value := range req.Headers {
		key = strings.TrimSpace(key)
		if key == "" || strings.EqualFold(key, "host") {
			continue
		}
		httpReq.Header.Set(key, value)
	}

	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("stopped after 5 redirects")
			}
			if err := rejectPrivateAgentURL(next.URL); err != nil {
				return err
			}
			return nil
		},
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return AgentHTTPResponse{}, err
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxAgentHTTPResponseBytes+1)
	bodyBytes, err := io.ReadAll(limited)
	if err != nil {
		return AgentHTTPResponse{}, err
	}
	truncated := len(bodyBytes) > maxAgentHTTPResponseBytes
	if truncated {
		bodyBytes = bodyBytes[:maxAgentHTTPResponseBytes]
	}
	headers := make(map[string]string, len(resp.Header))
	for key, values := range resp.Header {
		headers[key] = strings.Join(values, ", ")
	}
	return AgentHTTPResponse{
		URL:         parsed.String(),
		FinalURL:    resp.Request.URL.String(),
		Method:      method,
		StatusCode:  resp.StatusCode,
		Headers:     headers,
		Body:        string(bodyBytes),
		BodyBytes:   len(bodyBytes),
		Truncated:   truncated,
		ContentType: resp.Header.Get("Content-Type"),
		DurationMs:  time.Since(startedAt).Milliseconds(),
		ExecutedAt:  time.Now().UnixMilli(),
	}, nil
}

func rejectPrivateAgentURL(parsed *url.URL) error {
	host := parsed.Hostname()
	if host == "" {
		return errors.New("url host is required")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("failed to resolve host: %w", err)
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return errors.New("private or local network URLs are not allowed")
		}
	}
	return nil
}
