package sunapi

import (
	"context"
	"reflect"
	"testing"

	"net/http"
	"net/http/httptest"
)

func TestFetchProviderModelsUsesUpstreamResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer test-key"; got != want {
			t.Fatalf("Authorization = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"upstream-new-model-2026"},{"id":"vendor/latest-preview"}]}`))
	}))
	defer server.Close()

	models, err := fetchProviderModels(context.Background(), 1, server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"upstream-new-model-2026", "vendor/latest-preview"}
	if !reflect.DeepEqual(models, want) {
		t.Fatalf("models = %#v, want %#v", models, want)
	}
}

func TestParseModelsResponseAcceptsProviderShapes(t *testing.T) {
	models, err := parseModelsResponse([]byte(`{
		"data": [{"id":"openai-new-model"}],
		"models": [{"name":"models/gemini-new-model"},{"id":"anthropic-new-model"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"openai-new-model", "gemini-new-model", "anthropic-new-model"}
	if !reflect.DeepEqual(models, want) {
		t.Fatalf("models = %#v, want %#v", models, want)
	}
}

func TestBuildCompatModelsAllowsUnknownModels(t *testing.T) {
	models := buildCompatModels(Channel{
		Models: "upstream-new-model-2026,vendor/latest-preview",
	})
	if len(models) != 2 {
		t.Fatalf("len(models) = %d, want 2", len(models))
	}
	for _, model := range models {
		if model.Name == "" || model.Alias != model.Name {
			t.Fatalf("model = %+v, want name preserved as alias", model)
		}
		if model.Thinking == nil || len(model.Thinking.Levels) == 0 {
			t.Fatalf("model = %+v, want default thinking support for unknown models", model)
		}
	}
}
