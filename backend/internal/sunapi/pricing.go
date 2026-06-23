package sunapi

import "strings"

const compatBasePricePer1K = 0.002
const openAICompactModelSuffix = "-openai-compact"

type modelTokenPricing struct {
	InputPer1K  float64
	OutputPer1K float64
}

var modelRatioPricing = map[string]modelTokenPricing{
	"gpt-5-pro":                               ratioPricing(7.5, 8),
	"gpt-5-search-api":                        ratioPricing(0.625, 8),
	"gpt-5.1":                                 ratioPricing(0.625, 8),
	"gpt-5.1-chat-latest":                     ratioPricing(0.625, 8),
	"gpt-5.1-codex":                           ratioPricing(0.625, 8),
	"gpt-5.1-codex-mini":                      ratioPricing(0.125, 8),
	"gpt-5.1-codex-max":                       ratioPricing(0.625, 8),
	"gpt-5.2":                                 ratioPricing(0.625, 8),
	"gpt-5.2-chat-latest":                     ratioPricing(0.625, 8),
	"gpt-5.2-pro":                             ratioPricing(7.5, 8),
	"gpt-5.2-codex":                           ratioPricing(0.625, 8),
	"gpt-5.3-chat-latest":                     ratioPricing(0.625, 8),
	"gpt-5.3-codex":                           ratioPricing(0.625, 8),
	"gpt-5.4":                                 ratioPricing(1.25, 6),
	"gpt-5.4-mini":                            ratioPricing(0.375, 6),
	"gpt-5.4-nano":                            ratioPricing(0.1, 6.25),
	"gpt-5.4-pro":                             ratioPricing(15, 6),
	"gpt-5.5":                                 ratioPricing(2.5, 6),
	"gpt-5.5-pro":                             ratioPricing(15, 6),
	"claude-haiku-4-5-20251001":               ratioPricing(0.5, 5),
	"claude-sonnet-4-6":                       ratioPricing(1.5, 5),
	"claude-fable-5":                          ratioPricing(2.5, 5),
	"claude-opus-4-5-20251101":                ratioPricing(2.5, 5),
	"claude-opus-4-6":                         ratioPricing(2.5, 5),
	"claude-opus-4-7":                         ratioPricing(2.5, 5),
	"claude-opus-4-8":                         ratioPricing(2.5, 5),
	"gemini-3-pro-preview":                    ratioPricing(1, 6),
	"gemini-3.1-pro-preview":                  ratioPricing(1, 6),
	"gemini-3-flash-preview":                  ratioPricing(0.15, 4),
	"gemini-3.1-flash-lite-preview":           ratioPricing(0.05, 4),
	"gemini-3-pro-image-preview":              ratioPricing(1, 60),
	"nano-banana-pro-preview":                 ratioPricing(1, 4),
	"gemini-3.1-flash-image-preview":          ratioPricing(0.15, 4),
	"gemini-robotics-er-1.5-preview":          ratioPricing(0.15, 2.5/0.3),
	"gemini-2.5-computer-use-preview-10-2025": ratioPricing(0.15, 4),
	"deep-research-pro-preview-12-2025":       ratioPricing(1, 4),
	"gemini-2.5-flash-native-audio-preview-12-2025": ratioPricing(0.15, 4),
	"gemini-embedding-2-preview":                    ratioPricing(0.075, 4),
	"deepseek-v4-flash":                             ratioPricing(0.07, 1),
	"deepseek-v4-pro":                               ratioPricing(0.2175, 1),
	"qwen3.7-max":                                   ratioPricing(1.25, 4),
	"qwen3.7-plus":                                  ratioPricing(0.25, 4),
	"qwen3.7-coder":                                 ratioPricing(0.25, 4),
	"qwen3.6-flash":                                 ratioPricing(0.02, 4),
	"kimi-k2.6":                                     ratioPricing(0.8, 1),
	"glm-5.1":                                       ratioPricing(0.8, 1),
	"minimax-m2.7":                                  ratioPricing(0.8, 1),
	"mimo-v2.5-pro":                                 ratioPricing(0.8, 1),
	"text-embedding-v4":                             ratioPricing(0.01, 1),
	"qwen3-rerank":                                  ratioPricing(0.05, 1),
	"command-a-plus-05-2026":                        ratioPricing(2.5, 4),
	"embed-v4.0":                                    ratioPricing(0.06, 1),
	"rerank-v4.0-pro":                               ratioPricing(1, 1),
	"rerank-v4.0-fast":                              ratioPricing(0.25, 1),
	"sonar":                                         ratioPricing(0.5, 4),
	"sonar-pro":                                     ratioPricing(1.5, 4),
	"sonar-reasoning":                               ratioPricing(0.5, 4),
	"sonar-reasoning-pro":                           ratioPricing(1.5, 4),
	"sonar-deep-research":                           ratioPricing(1.5, 4),
	"grok-4.3":                                      ratioPricing(2.5, 2),
	"grok-4.3-latest":                               ratioPricing(2.5, 2),
	"grok-latest":                                   ratioPricing(2.5, 2),
	"grok-build-0.1":                                ratioPricing(0.5, 2),
	"grok-4-1-fast-reasoning":                       ratioPricing(0.3, 2),
	"grok-4-1-fast-non-reasoning":                   ratioPricing(0.3, 2),
	"grok-code-fast-1":                              ratioPricing(0.1, 2),
	"grok-4-fast-reasoning":                         ratioPricing(0.3, 2),
	"grok-4-fast-non-reasoning":                     ratioPricing(0.3, 2),
	"grok-imagine-image-pro":                        ratioPricing(1, 1),
	"grok-imagine-image":                            ratioPricing(0.5, 1),
	"grok-imagine-video":                            ratioPricing(2, 1),
}

var modelPriceAliases = map[string]string{
	"gpt-5-pro-2025-10-06":               "gpt-5-pro",
	"gpt-5-search-api-2025-10-14":        "gpt-5-search-api",
	"gpt-5.1-2025-11-13":                 "gpt-5.1",
	"gpt-5.2-2025-12-11":                 "gpt-5.2",
	"gpt-5.2-pro-2025-12-11":             "gpt-5.2-pro",
	"gpt-5.4-2026-03-05":                 "gpt-5.4",
	"gpt-5.4-pro-2026-03-05":             "gpt-5.4-pro",
	"claude-opus-4-6-max":                "claude-opus-4-6",
	"claude-opus-4-6-high":               "claude-opus-4-6",
	"claude-opus-4-6-medium":             "claude-opus-4-6",
	"claude-opus-4-6-low":                "claude-opus-4-6",
	"claude-opus-4-7-max":                "claude-opus-4-7",
	"claude-opus-4-7-xhigh":              "claude-opus-4-7",
	"claude-opus-4-7-high":               "claude-opus-4-7",
	"claude-opus-4-7-medium":             "claude-opus-4-7",
	"claude-opus-4-7-low":                "claude-opus-4-7",
	"claude-opus-4-8-max":                "claude-opus-4-8",
	"claude-opus-4-8-xhigh":              "claude-opus-4-8",
	"claude-opus-4-8-high":               "claude-opus-4-8",
	"claude-opus-4-8-medium":             "claude-opus-4-8",
	"claude-opus-4-8-low":                "claude-opus-4-8",
	"gemini-3.1-pro-preview-customtools": "gemini-3.1-pro-preview",
	"deepseek-v4-flash-none":             "deepseek-v4-flash",
	"deepseek-v4-flash-max":              "deepseek-v4-flash",
	"deepseek-v4-pro-none":               "deepseek-v4-pro",
	"deepseek-v4-pro-max":                "deepseek-v4-pro",
	"MiniMax-M2.7":                       "minimax-m2.7",
	"Qwen/Qwen3.7-Max":                   "qwen3.7-max",
	"Qwen/Qwen3.7-Coder":                 "qwen3.7-coder",
	"zai-org/GLM-5.1-FP8":                "glm-5.1",
	"deepseek-ai/DeepSeek-V4-Flash":      "deepseek-v4-flash",
	"deepseek-ai/DeepSeek-V4-Pro":        "deepseek-v4-pro",
	"moonshotai/kimi-k2.6":               "kimi-k2.6",
	"MiniMaxAI/MiniMax-M2.7":             "minimax-m2.7",
}

func ratioPricing(modelRatio float64, completionRatio float64) modelTokenPricing {
	input := modelRatio * compatBasePricePer1K
	return modelTokenPricing{
		InputPer1K:  input,
		OutputPer1K: input * completionRatio,
	}
}

func modelPricing(model string) (modelTokenPricing, bool) {
	model = normalizePricingModelName(model)
	if model == "" {
		return modelTokenPricing{}, false
	}
	for _, candidate := range pricingModelCandidates(model) {
		if pricing, ok := modelRatioPricing[candidate]; ok {
			return pricing, true
		}
		if alias, ok := modelPriceAliases[candidate]; ok {
			pricing, exists := modelRatioPricing[alias]
			return pricing, exists
		}
		for suffix := strings.LastIndex(candidate, "-"); suffix > 0; suffix = strings.LastIndex(candidate[:suffix], "-") {
			base := candidate[:suffix]
			if pricing, ok := modelRatioPricing[base]; ok {
				return pricing, true
			}
		}
	}
	return modelTokenPricing{}, false
}

func normalizePricingModelName(model string) string {
	model = strings.TrimSpace(model)
	model = strings.TrimPrefix(model, "models/")
	model = strings.TrimSuffix(model, openAICompactModelSuffix)
	return model
}

func pricingModelCandidates(model string) []string {
	candidates := []string{model}
	if idx := strings.LastIndex(model, "/"); idx >= 0 && idx+1 < len(model) {
		candidates = append(candidates, model[idx+1:])
	}
	lower := strings.ToLower(model)
	if lower != model {
		candidates = append(candidates, lower)
	}
	if idx := strings.LastIndex(lower, "/"); idx >= 0 && idx+1 < len(lower) {
		candidates = append(candidates, lower[idx+1:])
	}
	return candidates
}

func resolvedUsagePrices(channel Channel, settings Settings, model string) (float64, float64) {
	inputPrice := channel.InputPricePer1K
	outputPrice := channel.OutputPricePer1K

	if inputPrice <= 0 || outputPrice <= 0 {
		if pricing, ok := modelPricing(model); ok {
			if inputPrice <= 0 {
				inputPrice = pricing.InputPer1K
			}
			if outputPrice <= 0 {
				outputPrice = pricing.OutputPer1K
			}
		}
	}
	if inputPrice <= 0 && settings.DefaultInputPricePer1K > 0 {
		inputPrice = settings.DefaultInputPricePer1K
	}
	if outputPrice <= 0 && settings.DefaultOutputPricePer1K > 0 {
		outputPrice = settings.DefaultOutputPricePer1K
	}
	if inputPrice < 0 {
		inputPrice = 0
	}
	if outputPrice < 0 {
		outputPrice = 0
	}
	return inputPrice, outputPrice
}

func UsageCostForModel(channel Channel, settings Settings, multiplier float64, model string, inputTokens, outputTokens int64) float64 {
	if multiplier <= 0 {
		multiplier = 1
	}
	inputPrice, outputPrice := resolvedUsagePrices(channel, settings, model)
	inputCost := (float64(inputTokens) / 1000) * inputPrice
	outputCost := (float64(outputTokens) / 1000) * outputPrice
	return (inputCost + outputCost) * multiplier
}
