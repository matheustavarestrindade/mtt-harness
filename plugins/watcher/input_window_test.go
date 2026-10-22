package watcher

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/contextbuilder"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
)

type inputWindowCounter struct {
	harness.SessionWorkers
	provider harness.Provider
}

func (counter inputWindowCounter) Measure(operationContext context.Context, _ string, model, effort, text string, maximum int) (harness.RequestBudget, error) {
	request := atom.Request{Model: model, ReasoningEffort: effort, Params: map[string]any{"max_output_tokens": maximum}, Messages: []atom.Message{{Role: atom.RoleSystem, Content: []atom.Content{{Type: atom.Text, Text: "Fixed reviewer instruction."}}}, {Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: text}}}}}
	return contextbuilder.MeasureRequest(operationContext, request, atom.ModelInfo{ContextMax: 128000}, counter.provider)
}

func TestReviewInputWindowCountsOnlyInjectedText(testingHandle *testing.T) {
	operationContext := context.Background()
	modelProvider := provider.New(atom.ProviderSpec{Name: "synthetic", Protocol: "responses"})
	defer modelProvider.Close()
	plugin := &Plugin{services: harness.PluginServices{Sessions: inputWindowCounter{provider: modelProvider}}}
	definition := definition{Model: "synthetic/model", ContextTokens: 8192, Prompt: strings.Repeat("Fixed review instruction.\n", 600) + "<conversation_to_analize>{last_context}</conversation_to_analize>"}
	applyDefaults(&definition)
	source := harness.ReviewSource{Workspace: atom.InstanceSpec{ID: "workspace"}, Messages: []atom.Message{{ID: "old", Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: strings.Repeat("old context ", 4000)}}}, {ID: "visible", Role: atom.RoleAssistant, Content: []atom.Content{{Type: atom.Text, Text: "VISIBLE CONTEXT\n" + strings.Repeat("actual text ", 2000)}}}, {ID: "last", Role: atom.RoleTool, Content: []atom.Content{{Type: atom.Text, Text: "Final visible result."}}}}}
	before, budget, _, operationError := plugin.renderInput(operationContext, definition, source, observation{Trigger: "end_turn"})
	if operationError != nil {
		testingHandle.Fatal(operationError)
	}
	for index := range source.Messages {
		source.Messages[index].Reasoning = strings.Repeat("THINKING-NOT-INJECTED ", 10000)
		source.Messages[index].Usage = &atom.Usage{Input: 900000, Output: 800000, Reasoning: 700000}
		source.Messages[index].ProviderState = &atom.ProviderState{Provider: "synthetic", Reasoning: []json.RawMessage{json.RawMessage(`{"encrypted_content":"private"}`)}, ContextUsage: &atom.ContextUsage{InputTokens: 900000, OutputTokens: 800000}}
	}
	after, afterBudget, truncated, operationError := plugin.renderInput(operationContext, definition, source, observation{Trigger: "end_turn"})
	if operationError != nil {
		testingHandle.Fatal(operationError)
	}
	if before != after || budget.InputTokens != afterBudget.InputTokens || strings.Contains(after, "THINKING-NOT-INJECTED") {
		testingHandle.Fatal("excluded reasoning affected the injected window or its measurement")
	}
	if !truncated || !strings.Contains(after, "VISIBLE CONTEXT") || !strings.Contains(after, "Final visible result.") {
		testingHandle.Fatal("visible conversation was not selected")
	}
	preview := boundedText(after, 16384)
	if len(preview) != 16384 || len(after) <= 32000 || strings.Contains(preview, "Final visible result.") {
		testingHandle.Fatal("preview limit was confused with the full selected input")
	}
	definition.IncludeReasoning = true
	included, _, _, operationError := plugin.renderInput(operationContext, definition, source, observation{Trigger: "end_turn"})
	if operationError != nil {
		testingHandle.Fatal(operationError)
	}
	if !strings.Contains(included, "THINKING-NOT-INJECTED") {
		testingHandle.Fatal("explicit reasoning reference option was ignored")
	}
}
