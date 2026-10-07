package contextplugin

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestCancelledTurnFencesWorkerPublication(test *testing.T) {
	fixture := newPluginFixture(test)
	messages := fixtureMessages(fixture.session, 1000)
	fixture.prepare(test, messages, 200000)
	view := fixture.view(test)
	started := make(chan struct{})
	cancelled := make(chan struct{})
	fixture.services.mutex.Lock()
	fixture.services.respond = func(operationContext context.Context, input harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error) {
		close(started)
		<-operationContext.Done()
		close(cancelled)
		return harness.WorkspaceAgentResponse{}, operationContext.Err()
	}
	fixture.services.mutex.Unlock()
	dropMessages(test, fixture, []int64{view.Sources["decision"]}, true)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		test.Fatal("worker did not start")
	}
	testutil.RequireNoError(test, fixture.plugin.EndTurn(context.Background(), fixture.session, "cancelled"))
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		test.Fatal("cancelled turn left its worker running")
	}
	waitForJob(test, fixture, "cancelled")
	if len(fixture.view(test).Removed) != 0 {
		test.Fatal("cancellation changed the live context")
	}
}

func TestDisabledWorkspaceCancelsBackgroundWorkAfterGroup(test *testing.T) {
	fixture := newPluginFixture(test)
	messages := fixtureMessages(fixture.session, 1000)
	fixture.prepare(test, messages, 200000)
	view := fixture.view(test)
	started := make(chan struct{})
	cancelled := make(chan struct{})
	fixture.services.mutex.Lock()
	fixture.services.respond = func(operationContext context.Context, input harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error) {
		close(started)
		<-operationContext.Done()
		close(cancelled)
		return harness.WorkspaceAgentResponse{}, operationContext.Err()
	}
	fixture.services.mutex.Unlock()
	dropMessages(test, fixture, []int64{view.Sources["decision"]}, true)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		test.Fatal("worker did not start")
	}
	_, operationError := fixture.plugin.UpdateConfiguration(context.Background(), fixture.session.InstanceID, json.RawMessage(`{"enabled":false}`))
	testutil.RequireNoError(test, operationError)
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		test.Fatal("disabled workspace kept running a model")
	}
	if len(fixture.view(test).Removed) != 0 {
		test.Fatal("disabling removed source content")
	}
}
