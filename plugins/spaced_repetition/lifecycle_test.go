package spacedrepetition

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestRecoveryCancellationAndShutdownJoinTheWorker(test *testing.T) {
	fixture := newFixture(test)
	entered := make(chan struct{})
	finish := make(chan struct{})
	fixture.services.run = func(operationContext context.Context, input harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error) {
		close(entered)
		<-operationContext.Done()
		<-finish
		return harness.WorkspaceAgentResponse{}, operationContext.Err()
	}
	done := make(chan error, 1)
	go func() {
		done <- fixture.plugin.recoverInstructions(context.Background(), fixture.session, "slow", "Repeated drift")
	}()
	<-entered
	closed := make(chan error, 1)
	go func() { closed <- fixture.plugin.Close(context.Background()) }()
	select {
	case <-closed:
		test.Fatal("close returned before the worker joined")
	default:
	}
	close(finish)
	if operationError := <-done; !errors.Is(operationError, context.Canceled) {
		test.Fatalf("worker cancellation = %v", operationError)
	}
	testutil.RequireNoError(test, <-closed)
	state, operationError := fixture.database.ReadSession(context.Background(), "workspace", "session")
	testutil.RequireNoError(test, operationError)
	if state.Pending != nil {
		test.Fatal("cancelled worker published a report")
	}
}

func TestChildLeaseDoesNotWaitForPendingParentConfiguration(test *testing.T) {
	fixture := newFixture(test)
	parentContext, release, operationError := fixture.plugin.BeginRequest(context.Background(), fixture.session)
	testutil.RequireNoError(test, operationError)
	state, operationError := fixture.plugin.UpdateConfiguration(context.Background(), "workspace", json.RawMessage(`{"enabled":false}`))
	testutil.RequireNoError(test, operationError)
	if !state.Pending || !state.Enabled || state.RequestedEnabled {
		test.Fatalf("wrong pending state: %+v", state)
	}
	child := fixture.session
	child.ID = "child"
	child.Parent = fixture.session.ID
	fixture.services.mutex.Lock()
	fixture.services.session = child
	fixture.services.mutex.Unlock()
	deadline, cancel := context.WithTimeout(parentContext, time.Second)
	defer cancel()
	childContext, releaseChild, operationError := fixture.plugin.BeginRequest(deadline, child)
	testutil.RequireNoError(test, operationError)
	value, operationError := fixture.plugin.requestConfiguration(childContext, "workspace")
	testutil.RequireNoError(test, operationError)
	if !value.Enabled {
		test.Fatal("child did not inherit the accepted tool-group lease")
	}
	releaseChild()
	release()
	state, operationError = fixture.plugin.ReadState(context.Background(), "workspace")
	testutil.RequireNoError(test, operationError)
	if state.Pending || state.Enabled {
		test.Fatalf("configuration did not settle: %+v", state)
	}
}

func TestRevertKeepsRetainedReminderPhaseAndRejectsStaleCommit(test *testing.T) {
	fixture := newFixture(test)
	operationContext, release, first := fixture.prepare(test, 32768, fixture.services.messages)
	testutil.RequireNoError(test, first.Commit(operationContext, harness.ReminderDelivery{}))
	release()
	messages := append(append([]atom.Message(nil), fixture.services.messages...), atom.Message{ID: "later", Role: atom.RoleUser})
	operationContext, release, later := fixture.prepare(test, 65536, messages)
	complete, operationError := fixture.plugin.BeginHistoryChange(context.Background(), harness.HistoryChange{Kind: harness.HistoryRevert, KeepMessageID: "user", Snapshots: []harness.HistorySnapshot{{Session: fixture.session, Messages: messages}}})
	testutil.RequireNoError(test, operationError)
	testutil.RequireNoError(test, complete(context.Background(), true))
	if operationError := later.Commit(operationContext, harness.ReminderDelivery{}); operationError == nil {
		test.Fatal("stale request committed after revert")
	}
	release()
	_, release, replayed := fixture.prepare(test, 32768, fixture.services.messages)
	defer release()
	if len(replayed.Messages) != 0 {
		test.Fatal("revert replayed old thresholds")
	}
	state, operationError := fixture.database.ReadSession(context.Background(), "workspace", "session")
	testutil.RequireNoError(test, operationError)
	if state.Schedule.Cursor != 1 || len(state.Events) != 1 {
		test.Fatalf("retained phase lost: %+v", state)
	}
}

func TestPostgresReminderStateSurvivesRestartAndDeletion(test *testing.T) {
	databaseURL := os.Getenv("MTT_TEST_DATABASE_URL")
	if databaseURL == "" {
		test.Skip("Postgres test database is not configured")
	}
	fixture := newFixture(test)
	identifier, operationError := newIdentifier()
	testutil.RequireNoError(test, operationError)
	fixture.session.ID = atom.SessionID(identifier)
	fixture.session.InstanceID = identifier
	fixture.services.session = fixture.session
	testutil.RequireNoError(test, fixture.settings.Save(context.Background(), identifier, settingsKey, `{"enabled":true,"interval":{"mode":"tokens","tokens":32768}}`))
	database, operationError := openRepository(context.Background(), databaseURL)
	testutil.RequireNoError(test, operationError)
	fixture.plugin.database = database
	operationContext, release, reminder := fixture.prepare(test, 131072, fixture.services.messages)
	testutil.RequireNoError(test, reminder.Commit(operationContext, harness.ReminderDelivery{Tokens: 32, Estimated: true}))
	release()
	database.Close()
	reopened, operationError := openRepository(context.Background(), databaseURL)
	testutil.RequireNoError(test, operationError)
	fixture.plugin.database = reopened
	operationContext, release, next := fixture.prepare(test, 132000, fixture.services.messages)
	if len(next.Messages) != 0 {
		test.Fatal("restart replayed a checkpoint")
	}
	projected, operationError := fixture.plugin.ProjectReminders(operationContext, fixture.session, atom.Request{Messages: fixture.services.messages})
	testutil.RequireNoError(test, operationError)
	if len(projected.Messages) != 2 {
		test.Fatal("restart lost the anchored reminder")
	}
	release()
	complete, operationError := fixture.plugin.BeginHistoryChange(context.Background(), harness.HistoryChange{Kind: harness.HistoryDelete, Snapshots: []harness.HistorySnapshot{{Session: fixture.session, Messages: fixture.services.messages}}})
	testutil.RequireNoError(test, operationError)
	testutil.RequireNoError(test, complete(context.Background(), true))
	state, operationError := reopened.ReadSession(context.Background(), identifier, identifier)
	testutil.RequireNoError(test, operationError)
	if !state.Deleted || len(state.Events) != 0 || state.Pending != nil {
		test.Fatalf("deletion retained private context: %+v", state)
	}
	operationError = reopened.SaveRecovery(context.Background(), recoveryRun{ID: "late", WorkspaceID: identifier, SessionID: identifier, Epoch: 0})
	if !errors.Is(operationError, errSessionRetired) {
		test.Fatalf("late writer resurrected a recovery: %v", operationError)
	}
}
