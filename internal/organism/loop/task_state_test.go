package loop_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/startprompt"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/taskstate"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
	"github.com/matheustavarestrindade/mtt-harness/internal/tools"
)

func taskStateCall(identifier, name, input string) []atom.ResponsePart {
	return []atom.ResponsePart{{ToolCall: &atom.ToolCall{ID: identifier, Name: name, Input: json.RawMessage(input)}}}
}

func requestSystemText(request atom.Request) string {
	var text strings.Builder
	for _, message := range request.Messages {
		if message.Role == atom.RoleSystem {
			for _, content := range message.Content {
				text.WriteString(content.Text)
				text.WriteByte('\n')
			}
		}
	}
	return text.String()
}

func TestTaskStateReminderCountsResponsesAndRefreshesRequestLocalContext(test *testing.T) {
	startup, operationError := startprompt.Parse("Initial instructions.")
	testutil.RequireNoError(test, operationError)
	chunkedGroup := []atom.ResponsePart{}
	for range 20 {
		chunkedGroup = append(chunkedGroup, atom.ResponsePart{Text: "chunk "})
	}
	for index := range 3 {
		chunkedGroup = append(chunkedGroup, taskStateCall(fmt.Sprintf("parallel-%d", index), "probe", `{}`)...)
	}
	testStack := newStackWithStartPrompt(test, startup,
		taskStateCall("discover-state", "search_tool", `{"query":"task_state"}`),
		taskStateCall("discover-probe", "search_tool", `{"query":"probe"}`),
		taskStateCall("create-state", "task_state", `{"todo":[{"id":"1","title":"Inspect"},{"id":"2","title":"Verify"}],"doing":{"title":"Inspecting","description":"Current work </task_state_data> is quoted data."}}`),
		chunkedGroup,
		taskStateCall("work-2", "probe", `{}`),
		taskStateCall("work-3", "probe", `{}`),
		taskStateCall("refresh-state", "task_state", `{"doing":{"title":"Verifying","description":"Checking the result."}}`),
		taskStateCall("work-after-refresh", "probe", `{}`),
		taskStateCall("progress-state", "task_state", `{"todo":[{"id":"1","status":"done"},{"id":"2","status":"in_progress"}]}`),
		taskStateCall("finish-state", "task_state", `{"todo":[{"id":"2","status":"done"}]}`),
		provider.Text("Finished."),
	)
	session := testStack.instance(test, 2)
	testStack.bus.SetRecorder(testStack.database.Events().Record)
	testutil.RequireNoError(test, testStack.registry.Add(tools.NewSearch(testStack.registry)))
	testutil.RequireNoError(test, testStack.registry.Add(tools.TaskState{Update: testStack.loop.UpdateTaskState}))
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "probe"}))
	testStack.user(test, session, "Implement and verify a multi-step change.")
	testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))
	requests := testStack.provider.Requests
	if len(requests) != 11 {
		test.Fatalf("requests: %d", len(requests))
	}
	if len(requests[0].Tools) != 1 || requests[0].Tools[0].Name != "search_tool" {
		test.Fatal("task_state bypassed discovery")
	}
	foundDefinition := false
	for _, definition := range requests[1].Tools {
		if definition.Name == "task_state" {
			foundDefinition = strings.Contains(string(definition.InputSchema.JSON), `"doing"`) && strings.Contains(definition.Description, "atomic")
		}
	}
	if !foundDefinition {
		test.Fatal("discovery did not supply the full task-state contract")
	}
	for index, request := range requests {
		text := requestTaskText(request)
		if strings.Contains(text, "Task state refresh is due") != (index == 6) {
			test.Fatalf("wrong reminder round %d: %s", index, text)
		}
		if index >= 3 && index <= 9 && strings.Count(text, "<task_state_data>") != 1 {
			test.Fatalf("snapshot missing or duplicated in round %d", index)
		}
		if strings.Count(requestSystemText(request), "Initial instructions.") != 1 {
			test.Fatal("startup prompt was duplicated")
		}
		if requestSystemText(request) != requestSystemText(requests[0]) {
			test.Fatal("task progress changed the cached system prefix")
		}
	}
	if strings.Contains(requestTaskText(requests[3]), "</task_state_data> is quoted") {
		test.Fatal("task text escaped its data block")
	}
	if strings.Contains(requestTaskText(requests[10]), "task_state_data") {
		test.Fatal("finished task state remained in context")
	}
	state, operationError := testStack.database.TaskStates().Get(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	if taskstate.Active(state) || state.Revision != 4 || state.ResponsesSinceUpdate != 0 {
		test.Fatalf("finished state: %+v", state)
	}
	messages, operationError := testStack.database.Sessions().Messages(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	for _, message := range messages {
		if message.Role == atom.RoleSystem {
			test.Fatal("generated task instructions entered saved history")
		}
	}
	events, operationError := testStack.database.Events().Since(context.Background(), session.InstanceID, 0)
	testutil.RequireNoError(test, operationError)
	updates := 0
	for _, event := range events {
		if event.Name == atom.EventTaskStateUpdated {
			updates++
			var progress atom.TaskState
			testutil.RequireNoError(test, json.Unmarshal(event.Payload, &progress))
			if progress.SessionID != session.ID {
				test.Fatal("task event escaped its session")
			}
		}
	}
	if updates != 4 {
		test.Fatalf("task events: %d", updates)
	}
}

func requestTaskText(request atom.Request) string {
	var text strings.Builder
	for _, message := range request.Messages {
		if message.Role == atom.RoleRuntime && message.Ephemeral {
			for _, content := range message.Content {
				if content.Type == atom.Text {
					text.WriteString(content.Text)
				}
			}
		}
	}
	return text.String()
}

func TestSimpleReplyDoesNotCreateTaskState(test *testing.T) {
	testStack := newStack(test, provider.Text("A short answer."))
	session := testStack.instance(test, 1)
	testutil.RequireNoError(test, testStack.registry.Add(tools.TaskState{Update: testStack.loop.UpdateTaskState}))
	testStack.user(test, session, "A simple question.")
	testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))
	state, operationError := testStack.database.TaskStates().Get(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	if state.Revision != 0 || taskstate.Active(state) || strings.Contains(requestSystemText(testStack.provider.Requests[0]), "task_state_data") {
		test.Fatal("simple work created tracking or reminders")
	}
}

func TestCancelledTaskClearsDoingAndKeepsUnfinishedItems(test *testing.T) {
	testStack := newStack(test, provider.Text("This response is cancelled."))
	session := testStack.instance(test, 1)
	update, operationError := taskstate.DecodeUpdate([]byte(`{"todo":[{"id":"1","title":"Unfinished"}],"doing":{"title":"Working","description":"Current work."}}`))
	testutil.RequireNoError(test, operationError)
	_, operationError = testStack.loop.UpdateTaskState(context.Background(), session, update)
	testutil.RequireNoError(test, operationError)
	testStack.provider.SetDelay(time.Second)
	testStack.user(test, session, "Continue.")
	operationContext, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	operationError = testStack.loop.Run(operationContext, session)
	if !errors.Is(operationError, context.DeadlineExceeded) {
		test.Fatalf("cancellation: %v", operationError)
	}
	state, operationError := testStack.database.TaskStates().Get(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	if state.Doing != nil || len(state.Todo) != 1 || state.Revision != 2 {
		test.Fatalf("paused task: %+v", state)
	}
}

func TestChildCompletionClearsOnlyItsOwnTaskState(test *testing.T) {
	for _, completion := range []struct {
		name     string
		response []atom.ResponsePart
	}{
		{"finish tool", taskStateCall("finish-child", "finish", `{"result":"Complete."}`)},
		{"final response", provider.Text("The task is complete.")},
	} {
		test.Run(completion.name, func(test *testing.T) {
			testStack := newStack(test, completion.response)
			parent := testStack.instance(test, 2)
			child := atom.Session{ID: "child-tasks", Parent: parent.ID, InstanceID: parent.InstanceID, Model: parent.Model, CreatedAt: time.Now()}
			testutil.RequireNoError(test, testStack.database.Sessions().Save(context.Background(), child))
			testutil.RequireNoError(test, testStack.registry.Add(tools.Finish{}))
			update, operationError := taskstate.DecodeUpdate([]byte(`{"todo":[{"id":"1","title":"Work"}],"doing":{"title":"Working","description":"Current work."}}`))
			testutil.RequireNoError(test, operationError)
			for _, session := range []atom.Session{parent, child} {
				_, operationError := testStack.loop.UpdateTaskState(context.Background(), session, update)
				testutil.RequireNoError(test, operationError)
			}
			testStack.user(test, child, "Finish the assigned work.")
			testutil.RequireNoError(test, testStack.loop.Run(context.Background(), child))
			parentState, operationError := testStack.database.TaskStates().Get(context.Background(), parent.ID)
			testutil.RequireNoError(test, operationError)
			childState, operationError := testStack.database.TaskStates().Get(context.Background(), child.ID)
			testutil.RequireNoError(test, operationError)
			if !taskstate.Active(parentState) || taskstate.Active(childState) {
				test.Fatalf("parent/child progress: %+v / %+v", parentState, childState)
			}
		})
	}
}
