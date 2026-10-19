package contextplugin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/contextbuilder"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestReportedContextDoesNotForceCheckpointOnCiphertextSize(test *testing.T) {
	fixture := newPluginFixture(test)
	modelProvider := provider.New(atom.ProviderSpec{Name: "synthetic", Protocol: "responses"})
	model := atom.ModelInfo{ID: "test-model", ContextMax: 872000}
	request := atom.Request{Model: model.ID, Messages: []atom.Message{testMessage("user", atom.RoleUser, "Continue the work.")}}
	response := testMessage("assistant", atom.RoleAssistant, "Working on the task.")
	response.Usage = &atom.Usage{Input: 106625, CacheRead: 8192, Output: 582, Reasoning: 516}
	response.ProviderState = &atom.ProviderState{Provider: "synthetic", Reasoning: []json.RawMessage{json.RawMessage(`{"type":"reasoning","encrypted_content":"` + strings.Repeat("A", 710000) + `"}`)}}
	testutil.RequireNoError(test, contextbuilder.CaptureReportedUsage(context.Background(), request, modelProvider, &response))
	request.Messages = append(request.Messages, response, testMessage("new-user", atom.RoleUser, "Proceed."))
	input := harness.ContextRequest{Session: fixture.session, Request: request, Model: model, ModelID: "synthetic/test-model", Measure: func(operationContext context.Context, candidate atom.Request) (harness.RequestBudget, error) {
		return contextbuilder.MeasureRequest(operationContext, candidate, model, modelProvider)
	}}
	var operationError error
	input.Budget, operationError = input.Measure(context.Background(), request)
	testutil.RequireNoError(test, operationError)
	selection, operationError := fixture.plugin.PrepareContext(context.Background(), input)
	testutil.RequireNoError(test, operationError)
	if input.Budget.InputTokens < 115399 || input.Budget.InputTokens > 116000 || !selection.Managed {
		test.Fatalf("provider report did not control admission: %+v", input.Budget)
	}
	view := fixture.view(test)
	if len(view.Removed) != 0 || view.InputTokens >= selection.InputCeiling || view.NoticeLevel != 0 {
		test.Fatal("opaque bytes forced a reduction or context warning")
	}
	jobs, operationError := fixture.plugin.database.List(context.Background(), fixture.session.InstanceID, "job", "", 20)
	testutil.RequireNoError(test, operationError)
	if len(jobs) != 0 {
		test.Fatal("accounting error scheduled memory preparation")
	}
	// Older saved responses lack an anchor. Their provider-aware fallback must
	// still avoid interpreting the encrypted JSON envelope as input tokens.
	request.Messages[1].ProviderState.ContextUsage = nil
	input.Request = request
	input.Budget, operationError = input.Measure(context.Background(), request)
	testutil.RequireNoError(test, operationError)
	_, operationError = fixture.plugin.PrepareContext(context.Background(), input)
	testutil.RequireNoError(test, operationError)
	if input.Budget.InputTokens > 2000 {
		test.Fatalf("legacy continuation still inflated the estimate: %+v", input.Budget)
	}
}
