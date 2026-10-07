package atom

import "math"

// CalculateUsageCost preserves provider-reported cost and computes an estimate
// only when token prices are known. Reasoning tokens are a subset of output.
func CalculateUsageCost(modelInfo ModelInfo, usage *Usage) *Cost {
	if usage == nil {
		return nil
	}
	if usage.Cost != nil {
		return usage.Cost
	}
	if modelInfo.Prices == nil || modelInfo.Billing == "subscription" {
		return nil
	}
	prices := *modelInfo.Prices
	inputTokens := usage.Input + usage.CacheRead + usage.CacheWrite
	selectedThreshold := -1
	for _, tier := range prices.Tiers {
		if inputTokens > tier.AboveInputTokens && tier.AboveInputTokens > selectedThreshold {
			selectedThreshold = tier.AboveInputTokens
			prices.Input, prices.Output = tier.Input, tier.Output
			prices.CacheRead, prices.CacheWrite, prices.Reasoning = tier.CacheRead, tier.CacheWrite, tier.Reasoning
			prices.CacheReadUnknown, prices.CacheWriteUnknown = tier.CacheReadUnknown, tier.CacheWriteUnknown
		}
	}
	if usage.CacheRead > 0 && prices.CacheReadUnknown || usage.CacheWrite > 0 && prices.CacheWriteUnknown {
		return nil
	}
	value := (float64(usage.Input)*prices.Input + float64(usage.Output)*prices.Output + float64(usage.CacheRead)*prices.CacheRead + float64(usage.CacheWrite)*prices.CacheWrite) / 1_000_000
	if prices.Reasoning != nil {
		value += float64(usage.Reasoning) * (*prices.Reasoning - prices.Output) / 1_000_000
	}
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	return &Cost{Currency: prices.Currency, Value: value, Estimated: true}
}
