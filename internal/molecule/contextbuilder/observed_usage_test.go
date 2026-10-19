package contextbuilder

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func reportedContextFixture(test *testing.T) (*provider.Standard, atom.Request, atom.Message) {
	test.Helper()
	modelProvider := provider.New(atom.ProviderSpec{Name: "synthetic", Protocol: "responses"})
	request := atom.Request{Model: "test-model", Messages: []atom.Message{
		{Role: atom.RoleSystem, Ephemeral: true, Content: []atom.Content{{Type: atom.Text, Text: "startup instructions"}}},
		{Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "work request"}}},
		{Role: atom.RoleRuntime, Ephemeral: true, Content: []atom.Content{{Type: atom.Text, Text: "task revision 1"}}},
	}}
	response := atom.Message{ID: "response", Role: atom.RoleAssistant, Content: []atom.Content{{Type: atom.Text, Text: "progress"}},
		Usage:         &atom.Usage{Input: 106625, CacheRead: 8192, Output: 582, Reasoning: 516},
		ProviderState: &atom.ProviderState{Provider: "synthetic", Reasoning: []json.RawMessage{json.RawMessage(`{"type":"reasoning","encrypted_content":"` + strings.Repeat("A", 700000) + `"}`)}}}
	testutil.RequireNoError(test, CaptureReportedUsage(context.Background(), request, modelProvider, &response))
	return modelProvider, request, response
}

func TestReportedUsageAnchorsCurrentInputInsteadOfCumulativeOrEncryptedBytes(test *testing.T) {
	modelProvider, request, response := reportedContextFixture(test)
	request.Messages = append(request.Messages, response)
	budget, operationError := MeasureRequest(context.Background(), request, atom.ModelInfo{ContextMax: 872000}, modelProvider)
	testutil.RequireNoError(test, operationError)
	if budget.InputTokens != 115399 || !budget.Estimated || budget.InputLimit != 870976 {
		test.Fatalf("reported input/cache/output accounting: %+v", budget)
	}
	tool := atom.Message{Role: atom.RoleTool, ToolCallID: "call", Content: []atom.Content{{Type: atom.Text, Text: strings.Repeat("result ", 400)}}}
	request.Messages = append(request.Messages, tool)
	additional, operationError := modelProvider.EstimateTokens(context.Background(), atom.Request{Model: request.Model, Messages: []atom.Message{tool}})
	testutil.RequireNoError(test, operationError)
	budget, operationError = MeasureRequest(context.Background(), request, atom.ModelInfo{ContextMax: 872000}, modelProvider)
	testutil.RequireNoError(test, operationError)
	if budget.InputTokens != 115399+additional.InputTokens {
		test.Fatalf("new tool output was omitted or counted twice: %+v", budget)
	}
	public, operationError := json.Marshal(response)
	testutil.RequireNoError(test, operationError)
	if strings.Contains(string(public), "ContextUsage") || strings.Contains(string(public), "PrefixFingerprint") || strings.Contains(string(public), "encrypted_content") {
		test.Fatal("private accounting or continuation entered public history")
	}
}

func TestUsageAnchorInvalidationAndConservativeOverlayChanges(test *testing.T) {
	for _, variation := range []string{"edited", "removed", "reordered", "model", "provider", "effort", "response", "unknown latest usage"} {
		test.Run(variation, func(test *testing.T) {
			modelProvider, request, response := reportedContextFixture(test)
			request.Messages = append(request.Messages, response)
			switch variation {
			case "edited":
				request.Messages[1].Content = []atom.Content{{Type: atom.Text, Text: "changed"}}
			case "removed":
				request.Messages = append(request.Messages[:1], request.Messages[2:]...)
			case "reordered":
				request.Messages = append([]atom.Message{{Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "another turn"}}}}, request.Messages...)
			case "model":
				request.Model = "other-model"
			case "provider":
				modelProvider = provider.New(atom.ProviderSpec{Name: "other", Protocol: "responses"})
			case "effort":
				request.ReasoningEffort = "high"
			case "response":
				request.Messages[3].Content = []atom.Content{{Type: atom.Text, Text: "edited response"}}
			case "unknown latest usage":
				request.Messages = append(request.Messages, atom.Message{Role: atom.RoleAssistant, Content: []atom.Content{{Type: atom.Text, Text: "unknown usage"}}})
			}
			raw, operationError := modelProvider.EstimateTokens(context.Background(), request)
			testutil.RequireNoError(test, operationError)
			budget, operationError := MeasureRequest(context.Background(), request, atom.ModelInfo{ContextMax: 872000}, modelProvider)
			testutil.RequireNoError(test, operationError)
			if budget.InputTokens != raw.InputTokens {
				test.Fatalf("stale report survived %s: %+v vs %+v", variation, budget, raw)
			}
		})
	}
	modelProvider, request, response := reportedContextFixture(test)
	request.Messages = append(request.Messages, response)
	request.Messages[2].Content = []atom.Content{{Type: atom.Text, Text: strings.Repeat("new reminder ", 100)}}
	request.Tools = []atom.ToolSpec{{Name: "new_tool", Description: "New discovered capability", InputSchema: atom.Schema{JSON: json.RawMessage(`{"type":"object"}`)}}}
	budget, operationError := MeasureRequest(context.Background(), request, atom.ModelInfo{ContextMax: 872000}, modelProvider)
	testutil.RequireNoError(test, operationError)
	if budget.InputTokens <= 115399 {
		test.Fatal("changed reminders or tools were not charged")
	}
}
