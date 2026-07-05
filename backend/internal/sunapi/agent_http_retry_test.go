package sunapi

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestExecuteAgentHTTPRequestWithRetryRetriesTransientStatus(t *testing.T) {
	calls := 0
	resp, attempts, hasResp, err := executeAgentHTTPRequestWithRetry(
		context.Background(),
		AgentHTTPRequest{URL: "https://example.com", Method: http.MethodGet},
		2,
		func(context.Context, AgentHTTPRequest) (AgentHTTPResponse, error) {
			calls++
			if calls == 1 {
				return AgentHTTPResponse{StatusCode: http.StatusInternalServerError}, nil
			}
			return AgentHTTPResponse{StatusCode: http.StatusOK, Body: `{"ok":true}`}, nil
		},
		func(context.Context, int) bool { return true },
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasResp {
		t.Fatalf("expected response")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected final status 200, got %d", resp.StatusCode)
	}
	if calls != 2 || len(attempts) != 2 {
		t.Fatalf("expected 2 attempts, calls=%d attempts=%d", calls, len(attempts))
	}
}

func TestExecuteAgentHTTPRequestWithRetryReturnsLastResponse(t *testing.T) {
	calls := 0
	resp, attempts, hasResp, err := executeAgentHTTPRequestWithRetry(
		context.Background(),
		AgentHTTPRequest{URL: "https://example.com", Method: http.MethodGet},
		1,
		func(context.Context, AgentHTTPRequest) (AgentHTTPResponse, error) {
			calls++
			return AgentHTTPResponse{StatusCode: http.StatusServiceUnavailable, Body: "busy"}, nil
		},
		func(context.Context, int) bool { return true },
	)
	if err == nil {
		t.Fatalf("expected last transient status as error")
	}
	if !hasResp {
		t.Fatalf("expected final HTTP response to be available")
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected final status 503, got %d", resp.StatusCode)
	}
	if calls != 2 || len(attempts) != 2 {
		t.Fatalf("expected 2 attempts, calls=%d attempts=%d", calls, len(attempts))
	}
}

func TestExecuteAgentHTTPRequestWithRetryFailsWithoutResponse(t *testing.T) {
	wantErr := errors.New("connection refused")
	_, attempts, hasResp, err := executeAgentHTTPRequestWithRetry(
		context.Background(),
		AgentHTTPRequest{URL: "https://example.com", Method: http.MethodGet},
		1,
		func(context.Context, AgentHTTPRequest) (AgentHTTPResponse, error) {
			return AgentHTTPResponse{}, wantErr
		},
		func(context.Context, int) bool { return true },
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected connection error, got %v", err)
	}
	if hasResp {
		t.Fatalf("did not expect response")
	}
	if len(attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(attempts))
	}
}

func TestParseAgentRetryCount(t *testing.T) {
	tests := map[string]int{
		"":    1,
		"0":   0,
		"3 次": 3,
		"9次":  5,
	}
	for input, want := range tests {
		if got := parseAgentRetryCount(input, 1); got != want {
			t.Fatalf("parseAgentRetryCount(%q)=%d want %d", input, got, want)
		}
	}
}
