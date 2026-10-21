package contextbuilder

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

// Fit removes complete oldest input turns, never individual tool messages.
// Persisted history is unchanged. System instructions and the newest turn stay.
func (builder *Builder) Fit(operationContext context.Context, request atom.Request, model atom.ModelInfo, provider harness.Provider) (atom.Request, error) {
	if model.ContextMax <= 0 {
		return request, nil
	}
	return builder.FitMeasured(operationContext, request, func(measurementContext context.Context, candidate atom.Request) (harness.RequestBudget, error) {
		return MeasureRequest(measurementContext, candidate, model, provider)
	})
}

// FitMeasured counts projected request-local data without making it a pruning
// boundary. Each candidate retains exactly the overlays whose anchors survive.
func (builder *Builder) FitMeasured(operationContext context.Context, request atom.Request, measure func(context.Context, atom.Request) (harness.RequestBudget, error)) (atom.Request, error) {
	request.Messages = append([]atom.Message(nil), request.Messages...)
	if operationError := operationContext.Err(); operationError != nil {
		return request, operationError
	}
	measurement, operationError := measure(operationContext, request)
	if operationError != nil {
		return request, operationError
	}
	if measurement.InputLimit <= 0 || measurement.InputTokens <= measurement.InputLimit {
		return request, nil
	}
	var turns []int
	var protected []atom.Message
	var protectedCounts []int
	for index, message := range request.Messages {
		if !message.Ephemeral && (message.Role == atom.RoleUser || message.Role == atom.RoleRuntime) {
			turns = append(turns, index)
			protectedCounts = append(protectedCounts, len(protected))
		}
		if len(turns) == 0 || message.Role == atom.RoleSystem || message.Ephemeral {
			protected = append(protected, message)
		}
	}
	if len(turns) < 2 {
		return request, fmt.Errorf("newest turn and tool schemas exceed the context budget of %d tokens", measurement.InputLimit)
	}
	// Probe whole-turn suffixes rather than repeatedly copying and recounting one
	// shrinking turn at a time. Each measurement still projects anchored reminders.
	// Keep an actually measured fitting candidate even for a non-monotone custom
	// counter; fitting never depends on a token estimate extrapolated from a probe.
	candidateAt := func(turn int) atom.Request {
		candidate := request
		candidate.Messages = make([]atom.Message, 0, protectedCounts[turn]+len(request.Messages)-turns[turn])
		candidate.Messages = append(candidate.Messages, protected[:protectedCounts[turn]]...)
		candidate.Messages = append(candidate.Messages, request.Messages[turns[turn]:]...)
		return candidate
	}
	lower, upper := 1, len(turns)-1
	best := candidateAt(upper)
	measurement, operationError = measure(operationContext, best)
	if operationError != nil {
		return request, operationError
	}
	if measurement.InputLimit > 0 && measurement.InputTokens > measurement.InputLimit {
		// TokenCounter and projection callbacks can be non-monotone. An
		// over-budget newest suffix does not prove every longer suffix fails.
		for turnIndex := 1; turnIndex < len(turns)-1; turnIndex++ {
			if operationError := operationContext.Err(); operationError != nil {
				return request, operationError
			}
			candidate := candidateAt(turnIndex)
			candidateBudget, measurementError := measure(operationContext, candidate)
			if measurementError != nil {
				return request, measurementError
			}
			if candidateBudget.InputLimit <= 0 || candidateBudget.InputTokens <= candidateBudget.InputLimit {
				return candidate, nil
			}
		}
		return request, fmt.Errorf("newest turn and tool schemas exceed the context budget of %d tokens", measurement.InputLimit)
	}
	for lower < upper {
		if operationError := operationContext.Err(); operationError != nil {
			return request, operationError
		}
		middle := lower + (upper-lower)/2
		candidate := candidateAt(middle)
		measurement, operationError = measure(operationContext, candidate)
		if operationError != nil {
			return request, operationError
		}
		if measurement.InputLimit <= 0 || measurement.InputTokens <= measurement.InputLimit {
			best = candidate
			upper = middle
			continue
		}
		lower = middle + 1
	}
	return best, nil
}

// MeasureRequest is shared by core fitting and public plugin policies. It counts
// all provider-bound content and identifies estimates instead of claiming exact
// utilization when the provider has no tokenizer.
func MeasureRequest(operationContext context.Context, request atom.Request, model atom.ModelInfo, provider harness.Provider) (harness.RequestBudget, error) {
	measurement := harness.RequestBudget{ContextLimit: model.ContextMax}
	_, exact := provider.(harness.TokenCounter)
	measurement.Estimated = !exact
	count, operationError := countTokens(operationContext, request, provider)
	if operationError != nil {
		return measurement, operationError
	}
	if count < 0 {
		return measurement, fmt.Errorf("provider token count is negative")
	}
	measurement.InputTokens = count
	if model.ContextMax <= 0 {
		return measurement, nil
	}
	reserve := min(1024, model.ContextMax/4)
	for _, key := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens"} {
		if value, found := request.Params[key]; found {
			data, operationError := json.Marshal(value)
			if operationError != nil {
				return measurement, operationError
			}
			var limit int
			if operationError := json.Unmarshal(data, &limit); operationError != nil || limit <= 0 {
				return measurement, fmt.Errorf("%s must be a positive integer", key)
			}
			reserve = limit
		}
	}
	budget := model.ContextMax - reserve
	if budget <= 0 {
		return measurement, fmt.Errorf("output reservation exhausts model context")
	}
	measurement.OutputReserve, measurement.InputLimit = reserve, budget
	return measurement, nil
}

func countTokens(operationContext context.Context, request atom.Request, provider harness.Provider) (int, error) {
	if counter, supported := provider.(harness.TokenCounter); supported {
		return counter.CountTokens(operationContext, request)
	}
	if estimator, supported := provider.(harness.TokenEstimator); supported {
		estimate, operationError := estimator.EstimateTokens(operationContext, request)
		if operationError != nil {
			return 0, operationError
		}
		if observed, found, operationError := estimateFromReportedUsage(operationContext, request, provider.Name(), estimator, estimate.Identity); found || operationError != nil {
			return observed, operationError
		}
		return estimate.InputTokens, nil
	}
	// Count prompt data, not storage IDs, timestamps, or historical usage. A
	// text-byte estimate is deliberately conservative. Media has a separate
	// allowance; adapters with model-specific tokenizers implement TokenCounter.
	tools, operationError := json.Marshal(request.Tools)
	if operationError != nil {
		return 0, operationError
	}
	count := 16 + len(tools)
	for _, message := range request.Messages {
		count += 8
		if message.ProviderState != nil && message.ProviderState.Provider == provider.Name() {
			count += len(message.ProviderState.ChatReasoning)
			for _, reasoning := range message.ProviderState.Reasoning {
				count += len(reasoning)
			}
		}
		for _, content := range message.Content {
			if content.Type == atom.Text {
				count += len(content.Text)
				continue
			}
			count += 16384
		}
		for _, call := range message.ToolCalls {
			arguments, operationError := call.ArgumentsText()
			if operationError != nil {
				arguments = string(call.Input)
			}
			count += len(call.Name) + len(arguments) + 8
		}
	}
	return count, operationContext.Err()
}
