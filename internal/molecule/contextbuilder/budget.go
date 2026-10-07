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
	measurement, operationError := MeasureRequest(operationContext, request, model, provider)
	if operationError != nil {
		return request, operationError
	}
	budget := measurement.InputLimit
	request.Messages = append([]atom.Message(nil), request.Messages...)
	for {
		count, operationError := countTokens(operationContext, request, provider)
		if operationError != nil {
			return request, operationError
		}
		if count <= budget {
			return request, nil
		}
		firstTurnIndex, nextTurnIndex := -1, -1
		for index, message := range request.Messages {
			if message.Ephemeral || (message.Role != atom.RoleUser && message.Role != atom.RoleRuntime) {
				continue
			}
			if firstTurnIndex < 0 {
				firstTurnIndex = index
				continue
			}
			nextTurnIndex = index
			break
		}
		if nextTurnIndex < 0 {
			return request, fmt.Errorf("newest turn and tool schemas exceed the context budget of %d tokens", budget)
		}
		kept := append([]atom.Message(nil), request.Messages[:firstTurnIndex]...)
		for _, message := range request.Messages[firstTurnIndex:nextTurnIndex] {
			if message.Role == atom.RoleSystem || message.Ephemeral {
				kept = append(kept, message)
			}
		}
		request.Messages = append(kept, request.Messages[nextTurnIndex:]...)
	}
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
		if message.ProviderState != nil {
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
			count += len(call.Name) + len(call.Input) + 8
		}
	}
	return count, operationContext.Err()
}
