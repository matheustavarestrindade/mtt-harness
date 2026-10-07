package loop

import "github.com/matheustavarestrindade/mtt-harness/atom"

func calculateUsageCost(modelInfo atom.ModelInfo, usage *atom.Usage) *atom.Cost {
	return atom.CalculateUsageCost(modelInfo, usage)
}
