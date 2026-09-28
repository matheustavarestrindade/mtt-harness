package contextbuilder

import (
	"context"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestTrimmingPreservesSystemAndCompleteToolTurns(test *testing.T) {
	model := atom.ModelInfo{ContextMax: 120}
	request := atom.Request{Messages: []atom.Message{
		{Role: atom.RoleSystem, Content: []atom.Content{{Type: atom.Text, Text: "system"}}},
		{Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: strings.Repeat("old ", 100)}}},
		{Role: atom.RoleAssistant, ToolCalls: []atom.ToolCall{{ID: "tool", Name: "read", Input: []byte(`{}`)}}},
		{Role: atom.RoleTool, ToolCallID: "tool", Content: []atom.Content{{Type: atom.Text, Text: "result"}}},
		{Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "new"}}},
	}}
	fitted, operationError := (&Builder{}).Fit(context.Background(), request, model, provider.NewTest("test"))
	testutil.RequireNoError(test, operationError)
	if len(fitted.Messages) != 2 || fitted.Messages[0].Role != atom.RoleSystem || fitted.Messages[1].Content[0].Text != "new" {
		test.Fatalf("trim left an orphaned tool exchange: %+v", fitted.Messages)
	}
	if len(request.Messages) != 5 {
		test.Fatal("context trim mutated stored history")
	}
	request.Messages = request.Messages[:2]
	if _, operationError := (&Builder{}).Fit(context.Background(), request, model, provider.NewTest("test")); operationError == nil {
		test.Fatal("oversized newest turn was silently discarded")
	}
}
