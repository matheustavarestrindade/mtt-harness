package loop

import (
	"math"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func TestCostUsesInputContextTiersWithoutDoubleBillingReasoning(test *testing.T) {
	model := atom.ModelInfo{Prices: &atom.Prices{Currency: "USD", Input: 1, Output: 4, CacheRead: 0.2, Tiers: []atom.PriceTier{{AboveInputTokens: 100, Input: 2, Output: 6, CacheRead: 0.4}}}}
	for _, scenario := range []struct {
		input    int
		expected float64
	}{{80, 244.0 / 1e6}, {81, 410.0 / 1e6}} {
		usage := &atom.Usage{Input: scenario.input, CacheRead: 20, Output: 40, Reasoning: 10}
		cost := calculateUsageCost(model, usage)
		if cost == nil || !cost.Estimated || math.Abs(cost.Value-scenario.expected) > 1e-12 {
			test.Fatalf("incorrect context-tier cost: %+v, want %g", cost, scenario.expected)
		}
	}
	separateRate := 8.0
	model.Prices.Reasoning = &separateRate
	cost := calculateUsageCost(model, &atom.Usage{Input: 10, Output: 40, Reasoning: 10})
	if cost == nil || math.Abs(cost.Value-210.0/1e6) > 1e-12 {
		test.Fatalf("reasoning charged twice: %+v", cost)
	}
	actual := &atom.Cost{Currency: "EUR", Value: 1.25}
	if calculateUsageCost(model, &atom.Usage{Cost: actual}) != actual {
		test.Fatal("catalog estimate replaced provider cost")
	}
	model.Billing = "subscription"
	if calculateUsageCost(model, &atom.Usage{Input: 10}) != nil {
		test.Fatal("subscription was charged at API rates")
	}
	model.Billing = "tokens"
	model.Prices.CacheWriteUnknown = true
	if calculateUsageCost(model, &atom.Usage{CacheWrite: 10}) != nil {
		test.Fatal("unknown cache pricing became a free write")
	}
}
