package atom

import (
	"fmt"
	"slices"
	"strings"
)

// ValidateReasoningEffort accepts an empty selection as the model/provider
// default. Explicit values must come from that model's declared capability.
func (modelInfo ModelInfo) ValidateReasoningEffort(effort string) error {
	if effort == "" {
		return nil
	}
	if !modelInfo.Reasoning || !slices.Contains(modelInfo.ReasoningEfforts, effort) {
		return fmt.Errorf("model %q does not support reasoning effort %q; supported choices: %s; use an empty effort for the model default", modelInfo.ID, effort, strings.Join(modelInfo.ReasoningEfforts, ", "))
	}
	return nil
}
