package contextbuilder

import (
	"context"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestRuntimeUpdateStartsANewPrunableTurn(test *testing.T) {
	request := atom.Request{Messages: []atom.Message{
		{ID: "system", Role: atom.RoleSystem, Content: []atom.Content{{Type: atom.Text, Text: "instructions"}}},
		{ID: "old-user", Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: strings.Repeat("old", 80)}}},
		{ID: "old-plan", Role: atom.RoleAssistant, ToolCalls: []atom.ToolCall{{ID: "old-call", Name: "bash", Input: []byte(`{}`)}}},
		{ID: "old-result", Role: atom.RoleTool, ToolCallID: "old-call", Content: []atom.Content{{Type: atom.Text, Text: "old output"}}},
		{ID: "runtime", Role: atom.RoleRuntime, Content: []atom.Content{{Type: atom.Text, Text: "background finished"}}},
	}}
	builder := &Builder{}
	fitted, operationError := builder.Fit(context.Background(), request, atom.ModelInfo{ContextMax: 128}, provider.NewTest("fixture"))
	testutil.RequireNoError(test, operationError)
	if len(fitted.Messages) != 2 || fitted.Messages[0].ID != "system" || fitted.Messages[1].ID != "runtime" {
		test.Fatalf("runtime update was pinned as instructions or dropped with old tool results: %+v", fitted.Messages)
	}
	if len(request.Messages) != 5 {
		test.Fatal("context fitting changed persisted history")
	}
}
