package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type usageDetails struct {
	Cached     int `json:"cached_tokens"`
	CacheWrite int `json:"cache_write_tokens"`
	Reasoning  int `json:"reasoning_tokens"`
}

type chatUsage struct {
	PromptTokens            int             `json:"prompt_tokens"`
	CompletionTokens        int             `json:"completion_tokens"`
	PromptTokensDetails     usageDetails    `json:"prompt_tokens_details"`
	CompletionTokensDetails usageDetails    `json:"completion_tokens_details"`
	Cost                    json.RawMessage `json:"cost"`
}

type responsesUsage struct {
	Input         int             `json:"input_tokens"`
	Output        int             `json:"output_tokens"`
	InputDetails  usageDetails    `json:"input_tokens_details"`
	OutputDetails usageDetails    `json:"output_tokens_details"`
	Cost          json.RawMessage `json:"cost"`
}

// Both protocols report total input including cache reads/writes. Reasoning is
// part of output, not an additional token charge.
func normalizeProviderUsage(input, output int, inputDetails, outputDetails usageDetails, reportedCost json.RawMessage) (*atom.Usage, error) {
	if input < 0 || output < 0 || inputDetails.Cached < 0 || inputDetails.CacheWrite < 0 || inputDetails.Cached > input || inputDetails.CacheWrite > input-inputDetails.Cached || outputDetails.Reasoning < 0 || outputDetails.Reasoning > output {
		return nil, fmt.Errorf("invalid provider token usage")
	}
	cost, operationError := decodeReportedCost(reportedCost)
	if operationError != nil {
		return nil, operationError
	}
	return &atom.Usage{Input: input - inputDetails.Cached - inputDetails.CacheWrite, Output: output, CacheRead: inputDetails.Cached, CacheWrite: inputDetails.CacheWrite, Reasoning: outputDetails.Reasoning, Cost: cost}, nil
}

// Compatible endpoints such as OpenRouter report numeric usage.cost in USD.
// A structured cost retains its explicit currency and estimated status. Missing
// or null cost stays unknown; numeric zero is a reported zero charge.
func decodeReportedCost(encoded json.RawMessage) (*atom.Cost, error) {
	encoded = bytes.TrimSpace(encoded)
	if len(encoded) == 0 || bytes.Equal(encoded, []byte("null")) {
		return nil, nil
	}
	var cost atom.Cost
	if encoded[0] == '{' {
		var fields struct {
			Currency  string
			Value     *float64
			Estimated bool
		}
		if operationError := json.Unmarshal(encoded, &fields); operationError != nil {
			return nil, fmt.Errorf("invalid reported cost: %w", operationError)
		}
		if fields.Value == nil || strings.TrimSpace(fields.Currency) == "" {
			return nil, fmt.Errorf("reported cost requires currency and value")
		}
		cost = atom.Cost{Currency: fields.Currency, Value: *fields.Value, Estimated: fields.Estimated}
	} else {
		cost.Currency = "USD"
		if operationError := json.Unmarshal(encoded, &cost.Value); operationError != nil {
			return nil, fmt.Errorf("invalid reported cost: %w", operationError)
		}
	}
	if cost.Value < 0 || math.IsNaN(cost.Value) || math.IsInf(cost.Value, 0) {
		return nil, fmt.Errorf("reported cost must be finite and non-negative")
	}
	return &cost, nil
}

func chatCompletionTermination(providerName, reason string) error {
	switch reason {
	case "", "stop", "tool_calls", "function_call":
		return nil
	case "length", "content_filter":
		return fmt.Errorf("%s generation incomplete: finish_reason=%s", providerName, reason)
	default:
		return fmt.Errorf("%s generation interrupted: finish_reason=%s", providerName, reason)
	}
}
