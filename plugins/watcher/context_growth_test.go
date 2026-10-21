package watcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/contextbuilder"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func growthRequestSnapshot(testingHandle *testing.T, request atom.Request) []byte {
	testingHandle.Helper()
	var states []*atom.ProviderState
	for _, message := range request.Messages {
		states = append(states, message.ProviderState)
	}
	encoded, operationError := json.Marshal(struct {
		Request atom.Request
		Private []*atom.ProviderState
	}{request, states})
	testutil.RequireNoError(testingHandle, operationError)
	return encoded
}

func TestContextGrowthCountsVisibleContentButNotThinking(testingHandle *testing.T) {
	for _, protocol := range []string{"responses", "openai"} {
		testingHandle.Run(protocol, func(testingHandle *testing.T) {
			modelProvider := provider.New(atom.ProviderSpec{Name: "synthetic", Protocol: protocol})
			measure := func(operationContext context.Context, request atom.Request) (harness.RequestBudget, error) {
				return contextbuilder.MeasureRequest(operationContext, request, atom.ModelInfo{ContextMax: 1000000}, modelProvider)
			}
			request := atom.Request{Model: "synthetic-model", ReasoningEffort: "high", Params: map[string]any{"max_output_tokens": 2048},
				Tools: []atom.ToolSpec{{Name: "test_tool", Description: "Visible tool definition", InputSchema: atom.Schema{JSON: json.RawMessage(`{"type":"object"}`)}}},
				Messages: []atom.Message{
					{ID: "instructions", Role: atom.RoleSystem, Ephemeral: true, Content: []atom.Content{{Type: atom.Text, Text: "Workspace instructions"}}},
					{ID: "user", Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Inspect the project 🌍."}}},
					{ID: "assistant", Role: atom.RoleAssistant, Content: []atom.Content{{Type: atom.Text, Text: "Checking the file."}}, ToolCalls: []atom.ToolCall{{ID: "tool", Name: "test_tool", Input: json.RawMessage(`{"path":"README.md"}`)}}},
					{ID: "result", Role: atom.RoleTool, ToolCallID: "tool", Content: []atom.Content{{Type: atom.Text, Text: "File contents"}}},
					{ID: "media", Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Image, Data: []byte("image bytes"), MIME: "image/png"}}},
				}}
			expected, operationError := measure(context.Background(), request)
			testutil.RequireNoError(testingHandle, operationError)
			for _, multiplier := range []int{1, 10} {
				request.Messages[2].Reasoning = strings.Repeat("display thinking ", multiplier*1000)
				request.Messages[2].Usage = &atom.Usage{Input: 90000, Output: multiplier * 9000, Reasoning: multiplier * 8500}
				request.Messages[2].ProviderState = &atom.ProviderState{Provider: "synthetic", ChatReasoning: strings.Repeat("private thinking ", multiplier*1000),
					Reasoning: []json.RawMessage{json.RawMessage(`{"type":"reasoning","encrypted_content":"` + strings.Repeat("A", multiplier*20000) + `"}`)}}
				original := growthRequestSnapshot(testingHandle, request)
				measurement, visible, operationError := measureContextGrowth(context.Background(), harness.ContextRequest{Request: request, Measure: measure, Budget: harness.RequestBudget{InputTokens: 990000}})
				testutil.RequireNoError(testingHandle, operationError)
				if measurement != expected || !slices.Equal(visible, []string{"user", "assistant", "result", "media"}) {
					testingHandle.Fatalf("thinking changed visible context or discarded other input: %+v / %+v, %v", measurement, expected, visible)
				}
				if !bytes.Equal(original, growthRequestSnapshot(testingHandle, request)) {
					testingHandle.Fatal("measurement mutated the dispatched request")
				}
			}
			request.Messages = append(request.Messages, atom.Message{ID: "thinking-only", Role: atom.RoleAssistant,
				Content: []atom.Content{{Type: atom.Text, Text: "\n  "}}, Reasoning: "Only thinking", ProviderState: request.Messages[2].ProviderState})
			measurement, visible, operationError := measureContextGrowth(context.Background(), harness.ContextRequest{Request: request, Measure: measure})
			testutil.RequireNoError(testingHandle, operationError)
			if measurement != expected || slices.Contains(visible, "thinking-only") {
				testingHandle.Fatal("a thinking-only message advanced context or its history boundary")
			}
			request.Messages = append(request.Messages, atom.Message{ID: "new-text", Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: strings.Repeat("x", 4096)}}})
			measurement, visible, operationError = measureContextGrowth(context.Background(), harness.ContextRequest{Request: request, Measure: measure})
			testutil.RequireNoError(testingHandle, operationError)
			if measurement.InputTokens != expected.InputTokens+1024+8 || !slices.Contains(visible, "new-text") {
				testingHandle.Fatal("ordinary input did not advance context growth")
			}
		})
	}
}

func TestContextGrowthDoesNotReuseAggregateUsageAnchors(testingHandle *testing.T) {
	modelProvider := provider.New(atom.ProviderSpec{Name: "synthetic", Protocol: "responses"})
	model := atom.ModelInfo{ContextMax: 200000}
	request := atom.Request{Model: "synthetic-model", Messages: []atom.Message{{ID: "user", Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Task"}}}}}
	response := atom.Message{ID: "answer", Role: atom.RoleAssistant, Content: []atom.Content{{Type: atom.Text, Text: "Answer"}},
		Usage:         &atom.Usage{Input: 70000, CacheRead: 2000, Output: 50000, Reasoning: 49000},
		ProviderState: &atom.ProviderState{Provider: "synthetic", Reasoning: []json.RawMessage{json.RawMessage(`{"type":"reasoning","encrypted_content":"` + strings.Repeat("A", 200000) + `"}`)}}}
	testutil.RequireNoError(testingHandle, contextbuilder.CaptureReportedUsage(context.Background(), request, modelProvider, &response))
	request.Messages = append(request.Messages, response)
	measure := func(operationContext context.Context, candidate atom.Request) (harness.RequestBudget, error) {
		return contextbuilder.MeasureRequest(operationContext, candidate, model, modelProvider)
	}
	full, operationError := measure(context.Background(), request)
	testutil.RequireNoError(testingHandle, operationError)
	if full.InputTokens != 122000 {
		testingHandle.Fatalf("fixture lost its reported input/output anchor: %+v", full)
	}
	original := growthRequestSnapshot(testingHandle, request)
	measurement, _, operationError := measureContextGrowth(context.Background(), harness.ContextRequest{Request: request, Budget: full, Measure: measure})
	testutil.RequireNoError(testingHandle, operationError)
	if measurement.InputTokens >= 100 || !measurement.Estimated {
		testingHandle.Fatalf("aggregate usage inflated visible-context growth: %+v", measurement)
	}
	after, operationError := measure(context.Background(), request)
	testutil.RequireNoError(testingHandle, operationError)
	if after != full || !bytes.Equal(original, growthRequestSnapshot(testingHandle, request)) {
		testingHandle.Fatal("watcher measurement changed actual context accounting")
	}
}

func TestContextGrowthMeasurementFailuresDoNotUseTheThinkingInclusiveBudget(testingHandle *testing.T) {
	if _, _, operationError := measureContextGrowth(context.Background(), harness.ContextRequest{Budget: harness.RequestBudget{InputTokens: 90000}}); operationError == nil {
		testingHandle.Fatal("missing measurement silently used the full model budget")
	}
	operationContext, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	input := harness.ContextRequest{Measure: func(context.Context, atom.Request) (harness.RequestBudget, error) {
		called = true
		return harness.RequestBudget{InputTokens: 12}, nil
	}}
	if _, _, operationError := measureContextGrowth(operationContext, input); !errors.Is(operationError, context.Canceled) || called {
		testingHandle.Fatal("cancelled observation still measured input")
	}
	failed := errors.New("counter unavailable")
	input.Measure = func(context.Context, atom.Request) (harness.RequestBudget, error) {
		return harness.RequestBudget{}, failed
	}
	if _, _, operationError := measureContextGrowth(context.Background(), input); !errors.Is(operationError, failed) {
		testingHandle.Fatal("measurement failure was not retained")
	}
	input.Measure = func(context.Context, atom.Request) (harness.RequestBudget, error) {
		return harness.RequestBudget{InputTokens: 123, Estimated: false}, nil
	}
	budget, _, operationError := measureContextGrowth(context.Background(), input)
	testutil.RequireNoError(testingHandle, operationError)
	if budget.InputTokens != 123 || budget.Estimated {
		testingHandle.Fatal("exact counter result was changed")
	}
}

func TestExistingContextSchedulesRebaseWhenThinkingIsExcluded(testingHandle *testing.T) {
	value := definition{ID: "review", Enabled: true, Triggers: []string{"context_growth"}, IntervalTokens: 256, MaxContinuations: 3}
	source := harness.ReviewSource{Fence: harness.ReviewFence{UserMessageID: "user"}}
	state := sessionState{UserID: "user", Schedules: map[string]schedule{"review": {Next: 90112, HighWater: 90000, Model: "synthetic/model", Signature: definitionSignature(value), Visible: []string{"content"}, Continuations: 1}}}
	configuration := configuration{Enabled: true, Watchers: []definition{value}}
	observe := func(tokens int) []definition {
		return selectDefinitions(&state, configuration, source, observation{Trigger: "context_growth", Tokens: tokens, Model: "synthetic/model", Visible: []string{"content"}})
	}
	if len(observe(1000)) != 0 || state.Schedules["review"].Next != 1256 || state.Schedules["review"].GrowthBasis != contextGrowthBasis || state.Schedules["review"].Continuations != 1 {
		testingHandle.Fatalf("legacy schedule was not rebased: %+v", state.Schedules["review"])
	}
	if len(observe(1000)) != 0 || len(observe(1255)) != 0 || len(observe(1256)) != 1 {
		testingHandle.Fatal("visible growth did not retain the configured interval after migration")
	}
}
