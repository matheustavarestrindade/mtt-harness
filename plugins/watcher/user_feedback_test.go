package watcher

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestUserFeedbackDoesNotContinueAndIsContextForTheUserReply(testingHandle *testing.T) {
	stack := newIntegration(testingHandle, provider.Text("A decision is needed"), provider.Call("interfere", `{"message":"Which environment should be updated?","continue_chat":false}`), provider.Text("Using staging"), provider.Call("continue", `{}`))
	enableDefinition(testingHandle, stack, "end_turn")
	_, _, operationError := stack.queue.Submit(context.Background(), stack.session, "prepare the deployment")
	testutil.RequireNoError(testingHandle, operationError)
	runs := waitRuns(testingHandle, stack, 1)
	if runs[0].Status != "user_input_requested" || runs[0].ContinueChat == nil || *runs[0].ContinueChat || stack.provider.Calls() != 2 {
		testingHandle.Fatalf("user question caused automatic work: %+v", runs)
	}
	state, operationError := stack.plugin.database.Session(context.Background(), stack.session.InstanceID, string(stack.session.ID))
	testutil.RequireNoError(testingHandle, operationError)
	if !state.WaitingForUser || len(state.Notices) != 1 || state.Schedules["review"].Continuations != 0 {
		testingHandle.Fatalf("question state: %+v", state)
	}
	messages, operationError := stack.database.Sessions().Messages(context.Background(), stack.session.ID)
	testutil.RequireNoError(testingHandle, operationError)
	for _, message := range messages {
		if message.Role == atom.RoleRuntime {
			testingHandle.Fatal("question entered the automatic queue/history")
		}
	}
	status, operationError := stack.queue.Status(context.Background(), stack.session.ID)
	testutil.RequireNoError(testingHandle, operationError)
	if status.Running || len(status.Messages) != 0 {
		testingHandle.Fatalf("main conversation did not stay idle: %+v", status)
	}
	testutil.RequireNoError(testingHandle, stack.plugin.EndTurn(context.Background(), stack.session, "completed"))
	time.Sleep(30 * time.Millisecond)
	if stack.provider.Calls() != 2 {
		testingHandle.Fatal("same-user completion notification repeated the watcher")
	}
	activity, operationError := stack.plugin.SessionActivity(context.Background(), stack.session, harness.PluginActivityQuery{Limit: 50})
	testutil.RequireNoError(testingHandle, operationError)
	if len(activity.Entries) != 1 || activity.Entries[0].ContinueChat == nil || *activity.Entries[0].ContinueChat {
		testingHandle.Fatalf("question disposition missing from public activity: %+v", activity)
	}
	_, _, operationError = stack.queue.Submit(context.Background(), stack.session, "Use staging.")
	testutil.RequireNoError(testingHandle, operationError)
	waitRuns(testingHandle, stack, 2)
	requests := stack.provider.Requests
	if len(requests) != 4 {
		testingHandle.Fatalf("unexpected requests after user reply: %d", len(requests))
	}
	definitionFound := false
	for _, tool := range requests[1].Tools {
		if tool.Name == "interfere" {
			encoded, _ := json.Marshal(tool.InputSchema)
			definitionFound = strings.Contains(string(encoded), `"continue_chat"`) && strings.Contains(string(encoded), `"default":true`)
		}
	}
	if !definitionFound {
		testingHandle.Fatal("the actual watcher model request lacks the continuation option")
	}
	for _, message := range requests[1].Messages {
		if message.Role == atom.RoleUser {
			if !strings.Contains(message.Content[0].Text, "User:\nprepare the deployment") || strings.Contains(message.Content[0].Text, `"CreatedAt"`) {
				testingHandle.Fatal("watcher request did not use the readable conversation transcript")
			}
		}
	}
	questionIndex, replyIndex := -1, -1
	for index, message := range requests[2].Messages {
		for _, content := range message.Content {
			if message.Role == atom.RoleRuntime && strings.Contains(content.Text, "Which environment should be updated?") {
				questionIndex = index
			}
			if message.Role == atom.RoleUser && content.Text == "Use staging." {
				replyIndex = index
			}
		}
	}
	if questionIndex < 0 || replyIndex <= questionIndex {
		testingHandle.Fatalf("reply lost its question context: question=%d reply=%d", questionIndex, replyIndex)
	}
	state, operationError = stack.plugin.database.Session(context.Background(), stack.session.InstanceID, string(stack.session.ID))
	testutil.RequireNoError(testingHandle, operationError)
	if state.WaitingForUser {
		testingHandle.Fatal("new user input did not release the watcher wait")
	}
}

func TestPermissionQuestionLeavesThePendingDecisionForTheUser(testingHandle *testing.T) {
	path := filepath.Join(testingHandle.TempDir(), "reference.txt")
	testutil.RequireNoError(testingHandle, os.WriteFile(path, []byte("reference"), 0600))
	input, _ := json.Marshal(map[string]any{"path": path, "actions": []map[string]string{{"op": "read"}}})
	stack := newIntegration(testingHandle, provider.Call("file_actions", string(input)), provider.Call("interfere", `{"message":"May this external reference be read?","continue_chat":false}`), provider.Text("Read approved reference"))
	enableDefinition(testingHandle, stack, "permission_request")
	permissions := make(chan atom.PermissionRequest, 1)
	stack.runtime.On(atom.EventPermissionRequest, func(_ context.Context, event atom.Event) {
		var request atom.PermissionRequest
		if json.Unmarshal(event.Payload, &request) == nil {
			select {
			case permissions <- request:
			default:
			}
		}
	})
	harness.Decide(stack.runtime, atom.StageToolInput, func(context.Context, atom.ToolCall) (atom.Verdict, error) {
		return atom.Verdict{Kind: atom.VerdictAsk, Target: path, Why: "outside workspace"}, nil
	})
	_, _, operationError := stack.queue.Submit(context.Background(), stack.session, "read the reference")
	testutil.RequireNoError(testingHandle, operationError)
	runs := waitRuns(testingHandle, stack, 1)
	if runs[0].Status != "user_input_requested" || stack.provider.Calls() != 2 {
		testingHandle.Fatalf("question resolved the permission automatically: %+v", runs)
	}
	var pending atom.PermissionRequest
	select {
	case pending = <-permissions:
	case <-time.After(time.Second):
		testingHandle.Fatal("permission request was not emitted")
	}
	request, operationError := stack.plugin.services.Sessions.Permission(context.Background(), stack.session.ID, pending.ID)
	testutil.RequireNoError(testingHandle, operationError)
	if request == nil {
		testingHandle.Fatal("permission question was automatically denied or approved")
	}
	source, operationError := stack.plugin.services.Sessions.Source(context.Background(), stack.session.ID, 4096, false)
	testutil.RequireNoError(testingHandle, operationError)
	applied, operationError := stack.plugin.services.Sessions.ResolvePermission(context.Background(), source, pending.ID, true, "user", "")
	testutil.RequireNoError(testingHandle, operationError)
	if !applied {
		testingHandle.Fatal("user could not resolve the pending permission")
	}
	deadline := time.Now().Add(2 * time.Second)
	for stack.provider.Calls() < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if stack.provider.Calls() != 3 {
		testingHandle.Fatal("normal work did not resume after the user decision")
	}
	state, operationError := stack.plugin.database.Session(context.Background(), stack.session.InstanceID, string(stack.session.ID))
	testutil.RequireNoError(testingHandle, operationError)
	if state.WaitingForUser {
		testingHandle.Fatal("a resolved permission left the watcher waiting for a text reply")
	}
}

func TestUserFeedbackWinsOverAutomaticCorrectionsInTheSameReviewGroup(testingHandle *testing.T) {
	stack := newIntegration(testingHandle, provider.Text("Finished"), provider.Call("interfere", `{"message":"Check another file","continue_chat":true}`), provider.Call("interfere", `{"message":"Please confirm the required account","continue_chat":false}`))
	enableDefinition(testingHandle, stack, "end_turn")
	configuration, _, operationError := stack.plugin.loadConfiguration(context.Background(), stack.session.InstanceID)
	testutil.RequireNoError(testingHandle, operationError)
	second := configuration.Watchers[0]
	second.ID = "question"
	second.Name = "Question"
	configuration.Watchers = append(configuration.Watchers, second)
	data, _ := json.Marshal(configuration)
	_, operationError = stack.plugin.UpdateConfiguration(context.Background(), stack.session.InstanceID, data)
	testutil.RequireNoError(testingHandle, operationError)
	_, _, operationError = stack.queue.Submit(context.Background(), stack.session, "task")
	testutil.RequireNoError(testingHandle, operationError)
	runs := waitRuns(testingHandle, stack, 2)
	statuses := map[string]string{}
	for _, run := range runs {
		statuses[run.DefinitionID] = run.Status
	}
	if statuses["review"] != "deferred_for_user" || statuses["question"] != "user_input_requested" || stack.provider.Calls() != 3 {
		testingHandle.Fatalf("automatic correction bypassed a user question: %+v", runs)
	}
}

func TestQuestionProjectionSurvivesRestartAndRetiresWithItsAnchor(testingHandle *testing.T) {
	stack := newIntegration(testingHandle, provider.Text("Done"), provider.Call("interfere", `{"message":"Need your answer","continue_chat":false}`))
	enableDefinition(testingHandle, stack, "end_turn")
	_, _, operationError := stack.queue.Submit(context.Background(), stack.session, "task")
	testutil.RequireNoError(testingHandle, operationError)
	waitRuns(testingHandle, stack, 1)
	messages, operationError := stack.database.Sessions().Messages(context.Background(), stack.session.ID)
	testutil.RequireNoError(testingHandle, operationError)
	// A new plugin instance over the same repository models a restart without
	// replaying a model call or relying on process-local queued work.
	replacement := &Plugin{database: stack.plugin.database, services: stack.plugin.services}
	requestContext, release, operationError := replacement.BeginRequest(context.Background(), stack.session)
	testutil.RequireNoError(testingHandle, operationError)
	defer release()
	projected, operationError := replacement.ProjectReminders(requestContext, stack.session, atom.Request{Messages: messages})
	testutil.RequireNoError(testingHandle, operationError)
	if len(projected.Messages) != len(messages)+1 {
		testingHandle.Fatal("saved question was not projected after restart")
	}
	replay, operationError := replacement.ProjectReminders(requestContext, stack.session, projected)
	testutil.RequireNoError(testingHandle, operationError)
	if len(replay.Messages) != len(projected.Messages) {
		testingHandle.Fatal("question projection duplicated the reference")
	}
	testutil.RequireNoError(testingHandle, stack.plugin.database.RetireHistory(context.Background(), stack.session.InstanceID, string(stack.session.ID), false, []string{messages[0].ID}))
	state, operationError := stack.plugin.database.Session(context.Background(), stack.session.InstanceID, string(stack.session.ID))
	testutil.RequireNoError(testingHandle, operationError)
	if len(state.Notices) != 0 || state.WaitingForUser {
		testingHandle.Fatalf("revert retained obsolete question state: %+v", state)
	}
}
