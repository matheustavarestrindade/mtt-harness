package loop_test

import (
	"context"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
	"github.com/matheustavarestrindade/mtt-harness/internal/tools"
)

func TestAgentGivesResultToParent(test *testing.T) {
	testStack := newStack(test,
		provider.Call("agent", `{"task":"find the answer"}`),
		provider.Call("finish", `{"result":"the child answer"}`),
		provider.Text("the parent is done"),
	)
	testutil.RequireNoError(test, testStack.registry.Add(&agentTool{runner: testStack.loop}))
	testutil.RequireNoError(test, testStack.registry.Add(tools.Finish{}))

	session := testStack.instance(test, 2)
	testStack.user(test, session, "start an agent")
	testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))

	messages, _ := testStack.database.Sessions().Messages(context.Background(), session.ID)
	found := false
	for _, message := range messages {
		if message.Role == atom.RoleTool && strings.Contains(message.Content[0].Text, "the child answer") {
			found = true
		}
	}
	if !found {
		test.Fatalf("the parent does not have the child result: %+v", messages)
	}
}

func TestAgentDepthLimit(test *testing.T) {
	testStack := newStack(test, provider.Text("done"))
	session := testStack.instance(test, 2)
	session.Depth = 2
	operationContext := harness.WithSession(context.Background(), session)
	result, operationError := testStack.loop.RunAgentTask(operationContext, atom.AgentTask{Task: "work"})
	testutil.RequireNoError(test, operationError)

	if !strings.Contains(result.Error, "depth limit") {
		test.Fatalf("the depth limit is not applied: %+v", result)
	}
}
