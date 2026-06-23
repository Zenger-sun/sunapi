package sunapi

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestNormalizePlaygroundImageTaskRequestUsesSavedTaskBoundary(t *testing.T) {
	req := normalizePlaygroundImageTaskRequest([]byte(`{"model":"wrong-image","group":"default","prompt":"draw"}`), PlaygroundImageGeneration{
		Model: "gpt-image-2",
		Group: "image2",
	})

	if got := gjson.GetBytes(req, "model").String(); got != "gpt-image-2" {
		t.Fatalf("model = %q, want gpt-image-2; body=%s", got, string(req))
	}
	if got := gjson.GetBytes(req, "group").String(); got != "image2" {
		t.Fatalf("group = %q, want image2; body=%s", got, string(req))
	}
}
