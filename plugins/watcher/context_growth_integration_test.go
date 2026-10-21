package watcher

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestThinkingDoesNotScheduleContextGrowthReview(testingHandle *testing.T) {
	displayThinking := strings.Repeat("visible thinking ", 2000)
	chatThinking := strings.Repeat("private thinking ", 1200)
	encryptedThinking, operationError := json.Marshal(map[string]string{"type": "reasoning", "encrypted_content": strings.Repeat("ciphertext", 1500)})
	testutil.RequireNoError(testingHandle, operationError)
	response := []atom.ResponsePart{
		{Reasoning: displayThinking},
		{ProviderState: &atom.ProviderState{Provider: "test", ChatReasoning: chatThinking, Reasoning: []json.RawMessage{encryptedThinking}}},
		{ToolCall: &atom.ToolCall{ID: "find-loaded-tool", Name: "search_tool", Input: json.RawMessage(`{"query":"search_tool"}`)}},
		{Usage: &atom.Usage{Input: 20, Output: 6005, Reasoning: 6000}},
	}
	stack := newIntegration(testingHandle, response, provider.Text("Done"))
	enableDefinition(testingHandle, stack, "context_growth")
	configuration, _, operationError := stack.plugin.loadConfiguration(context.Background(), stack.session.InstanceID)
	testutil.RequireNoError(testingHandle, operationError)
	configuration.Watchers[0].IntervalTokens = 8192
	// Reference-context selection must not opt thinking back into the trigger.
	configuration.Watchers[0].IncludeReasoning = true
	encoded, operationError := json.Marshal(configuration)
	testutil.RequireNoError(testingHandle, operationError)
	_, operationError = stack.plugin.UpdateConfiguration(context.Background(), stack.session.InstanceID, encoded)
	testutil.RequireNoError(testingHandle, operationError)
	_, _, operationError = stack.queue.Submit(context.Background(), stack.session, "Inspect the loaded discovery tool, then finish.")
	testutil.RequireNoError(testingHandle, operationError)

	deadline := time.Now().Add(8 * time.Second)
	settled := false
	for time.Now().Before(deadline) {
		status, statusError := stack.queue.Status(context.Background(), stack.session.ID)
		testutil.RequireNoError(testingHandle, statusError)
		if status.Error != "" {
			testingHandle.Fatal(status.Error)
		}
		messages, readError := stack.database.Sessions().Messages(context.Background(), stack.session.ID)
		testutil.RequireNoError(testingHandle, readError)
		var thoughtResponseID string
		for _, message := range messages {
			if len(message.ToolCalls) > 0 {
				thoughtResponseID = message.ID
				break
			}
		}
		state, stateError := stack.plugin.database.Session(context.Background(), stack.session.InstanceID, string(stack.session.ID))
		testutil.RequireNoError(testingHandle, stateError)
		stack.plugin.mutex.Lock()
		idle := len(stack.plugin.active) == 0 && len(stack.plugin.inbox) == 0
		stack.plugin.mutex.Unlock()
		if !status.Running && idle && thoughtResponseID != "" && slices.Contains(state.Schedules["review"].Visible, thoughtResponseID) {
			settled = true
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !settled {
		testingHandle.Fatal("main turn and committed context observation did not settle")
	}
	runs, operationError := stack.plugin.database.Runs(context.Background(), stack.session.InstanceID, string(stack.session.ID), "", 100)
	testutil.RequireNoError(testingHandle, operationError)
	if len(runs.Runs) != 0 {
		testingHandle.Fatalf("thinking started %d context-growth review(s); first status=%s trigger=%s", len(runs.Runs), runs.Runs[0].Status, runs.Runs[0].Trigger)
	}
	if len(stack.provider.Requests) != 2 {
		testingHandle.Fatalf("unexpected inference requests: %d", len(stack.provider.Requests))
	}
	retained := false
	for _, message := range stack.provider.Requests[1].Messages {
		if message.Reasoning == displayThinking && message.ProviderState != nil && message.ProviderState.ChatReasoning == chatThinking && len(message.ProviderState.Reasoning) == 1 && string(message.ProviderState.Reasoning[0]) == string(encryptedThinking) {
			retained = true
		}
	}
	if !retained {
		testingHandle.Fatal("watcher accounting removed provider-bound thinking state")
	}
	statistics, operationError := stack.database.Usage().Session(context.Background(), stack.session.ID)
	testutil.RequireNoError(testingHandle, operationError)
	if statistics.Reasoning != 6000 || statistics.Output != 6005 {
		testingHandle.Fatalf("watcher accounting changed provider usage: %+v", statistics)
	}
}
