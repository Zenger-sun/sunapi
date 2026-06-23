package util

import (
	"slices"
	"testing"
)

func TestGetProviderNameFallbackForStaticCodexModel(t *testing.T) {
	got := GetProviderName("gpt-5.4-mini")
	if !slices.Contains(got, "codex") {
		t.Fatalf("GetProviderName(gpt-5.4-mini) = %v, want codex fallback", got)
	}
}

func TestGetProviderNameFallbackTrimsPrefixAndThinkingSuffix(t *testing.T) {
	got := GetProviderName("team-a/gpt-5.4-mini(high)")
	if !slices.Contains(got, "codex") {
		t.Fatalf("GetProviderName(prefixed suffixed gpt model) = %v, want codex fallback", got)
	}
}
