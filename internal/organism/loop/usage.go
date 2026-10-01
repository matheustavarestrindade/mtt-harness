package loop

import "github.com/matheustavarestrindade/mtt-harness/atom"

func calculateUsageCost(modelInfo atom.ModelInfo, usage *atom.Usage) *atom.Cost {
	if modelInfo.Prices == nil || usage == nil {
		return nil
	}
	prices := modelInfo.Prices
	value := (float64(usage.Input)*prices.Input +
		float64(usage.Output)*prices.Output +
		float64(usage.CacheRead)*prices.CacheRead +
		float64(usage.CacheWrite)*prices.CacheWrite) / 1_000_000
	return &atom.Cost{Currency: prices.Currency, Value: value}
}
