package loop_test

import (
	"context"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/process"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/processes"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
	"github.com/matheustavarestrindade/mtt-harness/internal/tools"
)

func TestForegroundProcessesReturnOnlyTheCompleteToolBatch(test *testing.T) {
	testStack := newStack(test, []atom.ResponsePart{
		{ToolCall: &atom.ToolCall{ID: "first-command", Name: "bash", Input: []byte(`{"command":"printf first"}`)}},
		{ToolCall: &atom.ToolCall{ID: "second-command", Name: "bash", Input: []byte(`{"command":"printf second","notify":"interval","interval":1}`)}},
	}, provider.Text("finished"))
	session := testStack.instance(test, 2)
	processManager := processes.New(process.New(4), testStack.database, testStack.harnessRuntime, testStack.bus, testStack.instances)
	test.Cleanup(func() { testutil.RequireNoError(test, processManager.Close(context.Background())) })
	messageQueue := newTestQueue(test, testStack.loop)
	processManager.SetNotifier(func(operationContext context.Context, session atom.Session, content []atom.Content) error {
		_, _, operationError := messageQueue.SubmitRuntimeContent(operationContext, session, content)
		return operationError
	})
	testutil.RequireNoError(test, testStack.registry.Add(tools.NewBash(processManager)))
	turnEnded := make(chan struct{}, 8)
	testStack.harnessRuntime.On(atom.EventTurnEnd, func(context.Context, atom.Event) { turnEnded <- struct{}{} })
	_, _, operationError := messageQueue.Submit(context.Background(), session, "run two foreground commands")
	testutil.RequireNoError(test, operationError)
	select {
	case <-turnEnded:
	case <-time.After(3 * time.Second):
		test.Fatal("foreground batch did not finish")
	}
	// Inspect the policy used by Start, independently of shutdown suppression.
	records, operationError := testStack.database.Processes().List(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	if len(records) != 2 {
		test.Fatalf("expected two process records, got %d", len(records))
	}
	for _, record := range records {
		if record.Spec.Notify.Mode != atom.NotifyNone {
			test.Fatalf("foreground process can wake the session separately: %+v", record.Spec)
		}
	}
	testutil.RequireNoError(test, processManager.Close(context.Background()))
	testutil.RequireNoError(test, messageQueue.Close(context.Background()))
	if len(testStack.provider.Requests) != 2 {
		test.Fatalf("duplicate process updates started extra model turns: %d requests", len(testStack.provider.Requests))
	}
	request := testStack.provider.Requests[1]
	if len(request.Messages) != 4 || request.Messages[0].Role != atom.RoleUser || request.Messages[1].Role != atom.RoleAssistant {
		test.Fatalf("unexpected batch continuation: %+v", request.Messages)
	}
	for index, expected := range []struct{ identifier, output string }{{"first-command", "first"}, {"second-command", "second"}} {
		message := request.Messages[index+2]
		if message.Role != atom.RoleTool || message.ToolCallID != expected.identifier || message.Content[0].Text != expected.output {
			test.Fatalf("process result missing from the shared continuation: %+v", message)
		}
	}
	messages, operationError := testStack.database.Sessions().Messages(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	if len(messages) != 5 {
		test.Fatalf("process result was duplicated in history: %+v", messages)
	}
}
