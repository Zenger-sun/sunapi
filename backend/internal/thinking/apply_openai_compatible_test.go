package thinking_test

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/thinking/provider/codex"
)

func TestApplyThinkingAllowsOpenAICompatibleXHigh(t *testing.T) {
	const modelID = "sunapi-openai-compatible-xhigh"
	clientID := "test-" + modelID
	reg := registry.GetGlobalRegistry()
	reg.RegisterClient(clientID, "openai-compatibility", []*registry.ModelInfo{{
		ID:       modelID,
		Thinking: &registry.ThinkingSupport{Levels: []string{"low", "medium", "high", "xhigh"}},
	}})
	t.Cleanup(func() {
		reg.UnregisterClient(clientID)
	})

	out, err := thinking.ApplyThinking(
		[]byte(`{"model":"sunapi-openai-compatible-xhigh","reasoning":{"effort":"xhigh"}}`),
		modelID,
		"openai-response",
		"codex",
		"openai-compatibility",
	)
	if err != nil {
		t.Fatalf("ApplyThinking() error = %v", err)
	}
	if got := thinking.ExtractTranslatedReasoningEffort(out, "codex"); got != "xhigh" {
		t.Fatalf("translated effort = %q, want xhigh; body=%s", got, string(out))
	}
}
