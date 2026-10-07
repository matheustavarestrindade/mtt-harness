package contextplugin

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestRetryUsesNewRequestIdentityAndPreservesSourceUntilCheckpoint(test *testing.T) {
	fixture := newPluginFixture(test)
	messages := fixtureMessages(fixture.session, 1000)
	fixture.prepare(test, messages, 200000)
	_, operationError := fixture.plugin.UpdateConfiguration(context.Background(), fixture.session.InstanceID, json.RawMessage(`{"retry_limit":1}`))
	testutil.RequireNoError(test, operationError)
	var attempts atomic.Int32
	fixture.services.mutex.Lock()
	fixture.services.respond = func(context.Context, harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error) {
		if attempts.Add(1) == 1 {
			return harness.WorkspaceAgentResponse{}, errors.New("synthetic provider failure")
		}
		return harness.WorkspaceAgentResponse{Text: `{"memories":[]}`}, nil
	}
	fixture.services.mutex.Unlock()
	view := fixture.view(test)
	dropMessages(test, fixture, []int64{view.Sources["decision"]}, true)
	job := waitForJob(test, fixture, "ready")
	if job.Attempts != 2 || job.Failures != 1 || len(fixture.view(test).Removed) != 0 {
		test.Fatal("retry lost attempt data or changed the live context")
	}
	fixture.services.mutex.Lock()
	defer fixture.services.mutex.Unlock()
	if len(fixture.services.calls) != 2 || fixture.services.calls[0].RequestID == fixture.services.calls[1].RequestID {
		test.Fatal("separate provider attempts reused one accounting identity")
	}
}

type unavailableTransactionRepository struct{ repository }

func (unavailableTransactionRepository) Transact(context.Context, string, func(transaction) error) error {
	return errors.New("synthetic database write failure")
}

func TestPreparationDatabaseFailurePreservesDeliveredView(test *testing.T) {
	fixture := newPluginFixture(test)
	messages := fixtureMessages(fixture.session, 1000)
	fixture.prepare(test, messages, 200000)
	before := fixture.view(test)
	fixture.plugin.cancel()
	<-fixture.plugin.dispatchDone
	fixture.plugin.workers.Wait()
	fixture.plugin.database = unavailableTransactionRepository{fixture.plugin.database}
	_, operationError := fixture.plugin.PrepareContext(context.Background(), measuredInput(fixture.session, messages, 200000))
	if operationError == nil {
		test.Fatal("failed archive write was accepted")
	}
	after := fixture.view(test)
	if after.Revision != before.Revision || len(after.Removed) != 0 {
		test.Fatal("failed preparation committed a context change")
	}
}
