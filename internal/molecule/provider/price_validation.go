package provider

import (
	"fmt"
	"math"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func validatePrices(prices atom.Prices) error {
	rates := []float64{prices.Input, prices.Output, prices.CacheRead, prices.CacheWrite}
	if prices.Reasoning != nil {
		rates = append(rates, *prices.Reasoning)
	}
	thresholds := map[int]bool{}
	for _, tier := range prices.Tiers {
		if tier.AboveInputTokens < 0 || thresholds[tier.AboveInputTokens] {
			return fmt.Errorf("price tier thresholds must be unique and non-negative")
		}
		thresholds[tier.AboveInputTokens] = true
		rates = append(rates, tier.Input, tier.Output, tier.CacheRead, tier.CacheWrite)
		if tier.Reasoning != nil {
			rates = append(rates, *tier.Reasoning)
		}
	}
	for _, rate := range rates {
		if rate < 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
			return fmt.Errorf("token prices must be finite and non-negative")
		}
	}
	return nil
}
