package llm

import "strings"

// PriceVersion identifies the active price-card table. It is snapshotted
// onto every cost rollup row so a future price change never rewrites
// history (llm-cost-analytics design D1/D3): rows keep the version they
// were priced with.
const PriceVersion = 1

// priceCard holds provider list prices in micro-USD per million tokens
// (1 USD = 1_000_000 micro-USD, so $2.50/M = 2_500_000). Integer rates
// keep the conversion float-free end to end.
type priceCard struct {
	InputPerM  int64
	OutputPerM int64
}

// priceCards maps a normalized model name to its card. Unknown models fall
// back to providerDefault. Rates are the public list prices in effect at
// PriceVersion=1 (2026-09); bump PriceVersion (never edit old rows) when
// they change.
var priceCards = map[string]priceCard{
	"gpt-4o-mini":              {InputPerM: 150_000, OutputPerM: 600_000},
	"gpt-4o":                   {InputPerM: 2_500_000, OutputPerM: 10_000_000},
	"claude-sonnet-4-20250514": {InputPerM: 3_000_000, OutputPerM: 15_000_000},
	"claude-3-5-sonnet-latest": {InputPerM: 3_000_000, OutputPerM: 15_000_000},
}

// providerDefault covers models not in the table, keyed by provider name.
//
// ponytail: opencode routes through the OpenCode gateway, which is free at
// PriceVersion=1 (flat subscription). Ceiling: if the gateway starts
// metering per-token, add the real rate here and bump PriceVersion.
var providerDefault = map[string]priceCard{
	"opencode":  {},
	"openai":    {InputPerM: 150_000, OutputPerM: 600_000}, // gpt-4o-mini floor
	"anthropic": {InputPerM: 3_000_000, OutputPerM: 15_000_000},
}

// PriceFor resolves the card for a provider+model pair. Model lookup is
// case-insensitive and tolerates "vendor/model" prefixes (e.g.
// "openai/gpt-4o-mini") so gateway-style model ids still resolve.
func PriceFor(provider, model string) priceCard {
	m := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndex(m, "/"); i >= 0 {
		m = m[i+1:]
	}
	if c, ok := priceCards[m]; ok {
		return c
	}
	return providerDefault[strings.ToLower(strings.TrimSpace(provider))]
}

// CostMicroUSD converts a token count pair into micro-USD using the card
// for (provider, model), rounding to the nearest micro. Returns 0 when the
// provider reported no usage (opencode responses without a usage object),
// which keeps cost rows strictly truthful rather than estimated.
func CostMicroUSD(provider, model string, usage TokenUsage) int64 {
	c := PriceFor(provider, model)
	num := usage.Input*c.InputPerM + usage.Output*c.OutputPerM
	if num == 0 {
		return 0
	}
	return (num + 500_000) / 1_000_000
}
