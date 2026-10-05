package loop_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
	"github.com/matheustavarestrindade/mtt-harness/internal/tools"
)

type switchingTestProvider struct{ *provider.Test }

func (modelProvider switchingTestProvider) Models() []atom.ModelInfo {
	large := modelProvider.Test.Models()[0]
	large.ContextMax = 16000
	small := large
	small.ID, small.ContextMax = "small-model", 4000
	return []atom.ModelInfo{large, small}
}

func TestModelSwitchUsesNewRequestBudgetAndPreservesHistory(test *testing.T) {
	testStack := newStack(test, provider.Call("search_tool", `{"category":"file"}`), provider.Text("done"))
	testStack.harnessRuntime.Provider(switchingTestProvider{testStack.provider})
	testutil.RequireNoError(test, testStack.registry.Add(tools.NewSearch(testStack.registry)))
	session := testStack.instance(test, 2)
	oldMessage := atom.Message{ID: "older-turn", SessionID: session.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: strings.Repeat("previous ", 750)}}, CreatedAt: time.Now()}
	testutil.RequireNoError(test, testStack.database.Sessions().Append(context.Background(), oldMessage))
	testStack.user(test, session, "Continue with the current task")
	harness.Pipe(testStack.harnessRuntime, atom.StageToolResult, func(operationContext context.Context, result atom.ToolResult) (atom.ToolResult, error) {
		return result, testStack.database.Sessions().SetModelSelection(operationContext, session.ID, session.ModelSelection(), atom.SessionModelSelection{Model: "test/small-model"})
	})
	testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))
	requests := testStack.provider.Requests
	if len(requests) != 2 || requests[0].Model != "test-model" || requests[1].Model != "small-model" {
		test.Fatalf("model switch did not reach the next request: %+v", requests)
	}
	foundOld, foundCurrent, foundTool := false, false, false
	for _, message := range requests[1].Messages {
		foundOld = foundOld || message.ID == oldMessage.ID
		foundCurrent = foundCurrent || message.ID == "user-1"
		foundTool = foundTool || message.Role == atom.RoleTool
	}
	if foundOld || !foundCurrent || !foundTool {
		test.Fatal("context compaction lost the current turn or retained the oversized old turn")
	}
	messages, operationError := testStack.database.Sessions().Messages(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	if messages[0].ID != oldMessage.ID || len(messages[0].Content[0].Text) != len(oldMessage.Content[0].Text) {
		test.Fatal("request compaction changed saved history")
	}
}
