package sidekick

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestEventRetrievalDeliveryAndDuplicateSuppression(test *testing.T) {
	fixture := newFixture(test, false)
	operationContext, release := fixture.begin(test)
	fixture.waitState(test, func(state sessionState) bool { return state.Pending != nil })
	fixture.services.mutex.Lock()
	if len(fixture.services.calls) != 1 || len(fixture.services.queries) != 1 || fixture.services.queries[0].Compression != "medium" {
		test.Fatalf("unexpected worker work: %+v %+v", fixture.services.calls, fixture.services.queries)
	}
	fixture.services.mutex.Unlock()
	request := fixture.request()
	contribution, operationError := fixture.plugin.PrepareReminder(operationContext, request)
	testutil.RequireNoError(test, operationError)
	if len(contribution.Messages) != 1 || contribution.Messages[0].Role != atom.RoleRuntime || !strings.Contains(messageText(contribution.Messages[0]), "<sidekick>") {
		test.Fatalf("incorrect hint: %+v", contribution)
	}
	testutil.RequireNoError(test, contribution.Commit(operationContext, harness.ReminderDelivery{Tokens: 100, Estimated: true}))
	release()
	operationContext, release = fixture.begin(test)
	defer release()
	projected, operationError := fixture.plugin.ProjectReminders(operationContext, fixture.session, request.Request)
	testutil.RequireNoError(test, operationError)
	again, operationError := fixture.plugin.ProjectReminders(operationContext, fixture.session, projected)
	testutil.RequireNoError(test, operationError)
	if !reflect.DeepEqual(projected, again) || len(projected.Messages) != 2 {
		test.Fatal("projection duplicated or changed an anchor")
	}
	fixture.event(fixture.services.updateDoing("Inspect persistence"))
	time.Sleep(50 * time.Millisecond)
	fixture.services.mutex.Lock()
	calls := len(fixture.services.calls)
	fixture.services.mutex.Unlock()
	if calls != 1 {
		test.Fatalf("unchanged DOING called worker %d times", calls)
	}
	fixture.event(fixture.services.updateDoing("Check persistence again"))
	fixture.waitState(test, func(state sessionState) bool {
		return state.TaskKey == doingFingerprint(&atom.DoingState{Title: "Check persistence again", Description: "Check database ownership and persistence."}) && state.LastStatus == "skipped"
	})
	fixture.services.mutex.Lock()
	calls = len(fixture.services.calls)
	fixture.services.mutex.Unlock()
	if calls != 1 {
		test.Fatal("already delivered sources caused another model call")
	}
}

func TestEmptyFilterResponseAndWithdrawnMemoryDoNotInject(test *testing.T) {
	fixture := newFixture(test, false)
	fixture.services.run = func(context.Context, harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error) {
		return harness.WorkspaceAgentResponse{Text: `{"notes":[]}`}, nil
	}
	operationContext, release := fixture.begin(test)
	defer release()
	fixture.waitState(test, func(state sessionState) bool { return state.LastStatus == "skipped" })
	contribution, operationError := fixture.plugin.PrepareReminder(operationContext, fixture.request())
	testutil.RequireNoError(test, operationError)
	if len(contribution.Messages) != 0 {
		test.Fatal("empty result was injected")
	}
	fixture.services.mutex.Lock()
	fixture.services.run = nil
	fixture.services.mutex.Unlock()
	fixture.event(fixture.services.updateDoing("Review the data layer"))
	fixture.waitState(test, func(state sessionState) bool { return state.Pending != nil })
	fixture.services.mutex.Lock()
	fixture.services.memoryVersion++
	fixture.services.mutex.Unlock()
	contribution, operationError = fixture.plugin.PrepareReminder(operationContext, fixture.request())
	testutil.RequireNoError(test, operationError)
	if len(contribution.Messages) > 0 {
		test.Fatal("superseded memory reached the model")
	}
}

func TestClearedDoingCancelsBackgroundWorkWithoutBlockingTheEvent(test *testing.T) {
	fixture := newFixture(test, false)
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	fixture.services.run = func(operationContext context.Context, input harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error) {
		close(entered)
		<-operationContext.Done()
		close(cancelled)
		return harness.WorkspaceAgentResponse{}, operationContext.Err()
	}
	_, release := fixture.begin(test)
	defer release()
	select {
	case <-entered:
	case <-time.After(time.Second):
		test.Fatal("worker did not start")
	}
	fixture.event(fixture.services.updateDoing(""))
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		test.Fatal("cleared DOING did not cancel")
	}
	fixture.waitState(test, func(state sessionState) bool { return state.LastStatus == "cancelled" })
	operationContext, finish := fixture.begin(test)
	defer finish()
	contribution, operationError := fixture.plugin.PrepareReminder(operationContext, fixture.request())
	testutil.RequireNoError(test, operationError)
	if len(contribution.Messages) > 0 {
		test.Fatal("cancelled hint reached the model")
	}
}

func TestPostgresPendingHintSurvivesRestartAndDeletionRejectsLatePublication(test *testing.T) {
	fixture := newFixture(test, true)
	_, release := fixture.begin(test)
	state := fixture.waitState(test, func(state sessionState) bool { return state.Pending != nil })
	release()
	database := fixture.plugin.database.(*postgresRepository)
	reopened, operationError := openRepository(context.Background(), database.pool.Config().ConnConfig.ConnString())
	testutil.RequireNoError(test, operationError)
	defer reopened.Close()
	restored, operationError := reopened.ReadSession(context.Background(), fixture.session.InstanceID, string(fixture.session.ID))
	testutil.RequireNoError(test, operationError)
	if !reflect.DeepEqual(state.Pending, restored.Pending) {
		test.Fatal("prepared hint changed across restart")
	}
	messages, _ := fixture.services.Messages(context.Background(), fixture.session.ID)
	complete, operationError := fixture.plugin.BeginHistoryChange(context.Background(), harness.HistoryChange{Kind: harness.HistoryDelete, WorkspaceID: fixture.session.InstanceID, Snapshots: []harness.HistorySnapshot{{Session: fixture.session, Messages: messages}}})
	testutil.RequireNoError(test, operationError)
	testutil.RequireNoError(test, complete(context.Background(), true))
	_, operationError = reopened.UpdateSession(context.Background(), fixture.session.InstanceID, string(fixture.session.ID), func(current *sessionState) (mutation, error) {
		if current.Deleted || current.Epoch != state.Epoch {
			return mutation{}, errRetired
		}
		current.Pending = state.Pending
		return mutation{}, nil
	})
	if operationError == nil {
		test.Fatal("late publication restored a deleted session")
	}
}
