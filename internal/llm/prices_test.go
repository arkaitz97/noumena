package llm

import "testing"

func TestCostMicroUSD(t *testing.T) {
	tests := []struct {
		name            string
		provider, model string
		usage           TokenUsage
		want            int64
	}{
		{"gpt-4o-mini batch", "openai", "gpt-4o-mini", TokenUsage{Input: 1000, Output: 500}, 450}, // 1000*0.15 + 500*0.6 micro
		{"vendor-prefixed model resolves", "opencode", "openai/gpt-4o-mini", TokenUsage{Input: 1000, Output: 0}, 150},
		{"anthropic default for unknown model", "anthropic", "claude-future-9", TokenUsage{Input: 1_000_000, Output: 0}, 3_000_000},
		{"opencode gateway free at v1", "opencode", "opencode/default", TokenUsage{Input: 9_999_999, Output: 9_999_999}, 0},
		{"unreported usage is zero", "openai", "gpt-4o", TokenUsage{}, 0},
		{"rounds to nearest micro", "openai", "gpt-4o-mini", TokenUsage{Input: 1, Output: 0}, 0}, // 0.15 → 0
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CostMicroUSD(tc.provider, tc.model, tc.usage); got != tc.want {
				t.Errorf("CostMicroUSD = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestCostMicroRoundingMidpoint(t *testing.T) {
	// 3 input tokens at $0.60/M-ish: 3*150000 = 450000 /1e6 → 0.45 → 0;
	// 7*150000=1_050_000 → 1.05 → 1.
	if got := CostMicroUSD("openai", "gpt-4o-mini", TokenUsage{Input: 7, Output: 0}); got != 1 {
		t.Errorf("want round-to-nearest 1, got %d", got)
	}
}
