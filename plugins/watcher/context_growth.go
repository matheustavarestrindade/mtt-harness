package watcher

import (
	"context"
	"fmt"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

const contextGrowthBasis = "visible_input_without_reasoning_v1"

// measureContextGrowth is an observation, not context fitting or billing. Use a
// request-local copy: the real request must retain its provider continuation and
// accounting anchors. An aggregate usage anchor includes thinking and therefore
// cannot establish the size of just the visible messages, tools, and media.
func measureContextGrowth(operationContext context.Context, input harness.ContextRequest) (harness.RequestBudget, []string, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return harness.RequestBudget{}, nil, operationError
	}
	if input.Measure == nil {
		return harness.RequestBudget{}, nil, fmt.Errorf("watcher context measurement is unavailable")
	}
	request := input.Request
	request.Messages = make([]atom.Message, 0, len(input.Request.Messages))
	var visible []string
	for _, message := range input.Request.Messages {
		if operationError := operationContext.Err(); operationError != nil {
			return harness.RequestBudget{}, nil, operationError
		}
		message.Reasoning = ""
		message.ProviderState = nil
		message.Usage = nil
		// Thinking-only replies must not add message overhead or make removing
		// that thinking look like a visible-context compaction to the scheduler.
		if message.Role == atom.RoleAssistant && !hasVisibleMessageInput(message) {
			continue
		}
		request.Messages = append(request.Messages, message)
		if message.ID != "" && !message.Ephemeral {
			visible = append(visible, message.ID)
		}
	}
	measurement, operationError := input.Measure(operationContext, request)
	if operationError != nil {
		return harness.RequestBudget{}, nil, fmt.Errorf("measure watcher context: %w", operationError)
	}
	if measurement.InputTokens < 0 {
		return harness.RequestBudget{}, nil, fmt.Errorf("watcher context token count is negative")
	}
	return measurement, visible, operationContext.Err()
}

func hasVisibleMessageInput(message atom.Message) bool {
	if len(message.ToolCalls) > 0 || message.ToolCallID != "" {
		return true
	}
	for _, content := range message.Content {
		if content.Type != atom.Text || strings.TrimSpace(content.Text) != "" {
			return true
		}
	}
	return false
}
