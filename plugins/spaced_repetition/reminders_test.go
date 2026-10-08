package spacedrepetition

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestRemindersStayAnchoredWithoutAdvancingBeforeDispatch(test *testing.T) {
	fixture := newFixture(test)
	messages := fixture.services.messages
	operationContext, release, reminder := fixture.prepare(test, 32768, messages)
	if len(reminder.Messages) != 1 || !reminder.Messages[0].InContext || !reminder.Messages[0].Ephemeral || reminder.Messages[0].Role != atom.RoleSystem {
		test.Fatalf("invalid reminder: %+v", reminder.Messages)
	}
	state, operationError := fixture.database.ReadSession(operationContext, "workspace", "session")
	testutil.RequireNoError(test, operationError)
	if state.Schedule.Cursor != 0 || len(state.Events) != 0 {
		test.Fatal("preparation consumed a checkpoint")
	}
	testutil.RequireNoError(test, reminder.Deferred(operationContext))
	release()
	operationContext, release, reminder = fixture.prepare(test, 32768, messages)
	testutil.RequireNoError(test, reminder.Commit(operationContext, harness.ReminderDelivery{Tokens: 24, Estimated: true}))
	testutil.RequireNoError(test, reminder.Commit(operationContext, harness.ReminderDelivery{Tokens: 24, Estimated: true}))
	release()
	messages = append(append([]atom.Message(nil), messages...), atom.Message{ID: "answer", Role: atom.RoleAssistant, Content: []atom.Content{{Type: atom.Text, Text: "Working."}}})
	operationContext, release, next := fixture.prepare(test, 34000, messages)
	defer release()
	if len(next.Messages) != 0 {
		test.Fatal("checkpoint repeated at unchanged milestone")
	}
	projected, operationError := fixture.plugin.ProjectReminders(operationContext, fixture.session, atom.Request{Messages: messages})
	testutil.RequireNoError(test, operationError)
	if len(projected.Messages) != 3 || projected.Messages[0].ID != "user" || projected.Messages[2].ID != "answer" || projected.Messages[1].Content[0].Text != reminder.Messages[0].Content[0].Text {
		test.Fatalf("reminder moved or changed: %+v", projected.Messages)
	}
	again, operationError := fixture.plugin.ProjectReminders(operationContext, fixture.session, projected)
	testutil.RequireNoError(test, operationError)
	if !reflect.DeepEqual(again, projected) {
		test.Fatal("projection duplicated an existing reminder")
	}
	if len(fixture.services.calls) != 0 {
		test.Fatal("periodic repetition invoked a model")
	}
	state, operationError = fixture.database.ReadSession(operationContext, "workspace", "session")
	testutil.RequireNoError(test, operationError)
	if state.Schedule.Cursor != 1 || len(state.Events) != 1 {
		test.Fatalf("dispatch was not idempotent: %+v", state)
	}
}

func TestRecoveryReadsMediumMemoryOnceAndPublishesHigh(test *testing.T) {
	fixture := newFixture(test)
	tool := &instructionTool{plugin: fixture.plugin}
	operationContext := harness.WithSession(context.Background(), fixture.session)
	call := atom.ToolCall{ID: "recovery", Name: tool.Name(), Input: []byte(`{"reason":"The user keeps correcting database choice and writing style."}`)}
	result, operationError := tool.Run(operationContext, call)
	testutil.RequireNoError(test, operationError)
	if result.Content[0].Text != `{"status":"ready"}` {
		test.Fatalf("unexpected tool result: %+v", result)
	}
	_, operationError = tool.Run(operationContext, call)
	testutil.RequireNoError(test, operationError)
	if len(fixture.services.calls) != 2 || len(fixture.services.queries) != 2 {
		test.Fatal("replayed call duplicated model or retrieval work")
	}
	for _, query := range fixture.services.queries {
		if query.Compression != "medium" || query.WorkspaceID != "workspace" || query.Agent != recoveryAgent {
			test.Fatalf("wrong retrieval policy: %+v", query)
		}
	}
	for _, request := range fixture.services.calls {
		if request.Agent != recoveryAgent || request.SourceSessionID != fixture.session.ID || request.RunID == "" || request.RequestID == "" {
			test.Fatalf("missing worker accounting: %+v", request)
		}
	}
	operationContext, release, reminder := fixture.prepare(test, 131072, fixture.services.messages)
	defer release()
	if len(reminder.Messages) != 1 || !strings.Contains(reminder.Messages[0].Content[0].Text, "spaced_repetition.high") || !strings.Contains(reminder.Messages[0].Content[0].Text, "prescribed database") {
		test.Fatalf("recovery not applied: %+v", reminder)
	}
	testutil.RequireNoError(test, reminder.Commit(operationContext, harness.ReminderDelivery{Tokens: 100, Estimated: true}))
	state, operationError := fixture.database.ReadSession(operationContext, "workspace", "session")
	testutil.RequireNoError(test, operationError)
	if state.Pending != nil || state.Schedule.Cursor != 4 || state.Events[0].Level != "high" {
		test.Fatalf("high did not cover due checkpoints: %+v", state)
	}
}

func TestRecoveredInstructionsAreFencedByNewUserAndHistory(test *testing.T) {
	fixture := newFixture(test)
	testutil.RequireNoError(test, fixture.plugin.recoverInstructions(context.Background(), fixture.session, "first", "Repeated instruction drift."))
	newMessages := append(append([]atom.Message(nil), fixture.services.messages...), atom.Message{ID: "new-user", Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Use a different approach now."}}})
	_, release, reminder := fixture.prepare(test, 100, newMessages)
	release()
	if len(reminder.Messages) != 0 {
		test.Fatal("old recovery was applied to changed instructions")
	}
	complete, operationError := fixture.plugin.BeginHistoryChange(context.Background(), harness.HistoryChange{Kind: harness.HistoryDelete, Snapshots: []harness.HistorySnapshot{{Session: fixture.session, Messages: newMessages}}})
	testutil.RequireNoError(test, operationError)
	testutil.RequireNoError(test, complete(context.Background(), true))
	if operationError := fixture.plugin.recoverInstructions(context.Background(), fixture.session, "late", "old worker"); operationError == nil {
		test.Fatal("deleted session accepted a late recovery")
	}
}
