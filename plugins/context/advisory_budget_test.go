package contextplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func failDecisionPreparation(test *testing.T, fixture *pluginFixture) memoryJob {
	test.Helper()
	view := fixture.view(test)
	proposal := memoryProposal{Title: "Unsupported decision", Categories: []string{"architecture"}, Kind: "approved_decision", Text: summaries{Low: "Use PostgreSQL.", Medium: "Use PostgreSQL.", High: "Use PostgreSQL."}, Evidence: []memoryEvidence{{SourceID: view.Sources["decision"], Quote: "Use PostgreSQL for durable data."}}}
	encoded, operationError := json.Marshal(map[string]any{"memories": []memoryProposal{proposal}})
	testutil.RequireNoError(test, operationError)
	fixture.services.mutex.Lock()
	fixture.services.respond = func(context.Context, harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error) {
		return harness.WorkspaceAgentResponse{Text: string(encoded)}, nil
	}
	fixture.services.mutex.Unlock()
	dropMessages(test, fixture, []int64{view.Sources["decision"]}, true)
	job := waitForJob(test, fixture, "failed")
	if job.Error != "decision 1 has no approving user source" {
		test.Fatalf("unexpected worker failure: %s", job.Error)
	}
	return job
}

func TestAdvisoryThresholdsDoNotBlockOrForceReductionAfterWorkerFailure(test *testing.T) {
	for _, percentage := range []int{50, 65, 79} {
		for _, wrapRequested := range []bool{false, true} {
			test.Run(fmt.Sprintf("percent_%d_wrap_%t", percentage, wrapRequested), func(test *testing.T) {
				fixture := newPluginFixture(test)
				messages := fixtureMessages(fixture.session, 10000)
				initial := fixture.prepare(test, messages, 200000)
				measurement := measuredInput(fixture.session, initial.Request.Messages, 200000)
				budget, operationError := measurement.Measure(context.Background(), initial.Request)
				testutil.RequireNoError(test, operationError)
				failedJob := failDecisionPreparation(test, fixture)
				if wrapRequested {
					_, operationError = fixture.plugin.requestWrapup(context.Background(), fixture.session)
					testutil.RequireNoError(test, operationError)
				}
				selection := fixture.prepare(test, messages, budget.InputTokens*100/percentage)
				view := fixture.view(test)
				if len(view.Removed) != 0 || view.Revision != 0 || snapshotText(selection.Request) != snapshotText(initial.Request) {
					test.Fatal("advisory context notice removed source history or rewrote the snapshot")
				}
				for _, original := range messages {
					found := false
					for _, retained := range selection.Request.Messages {
						found = found || retained.ID == original.ID
					}
					if !found {
						test.Fatalf("advisory request lost %s", original.ID)
					}
				}
				operationError = fixture.plugin.database.Transact(context.Background(), fixture.session.InstanceID, func(database transaction) error {
					jobs, operationError := readTransactionValues[memoryJob](database, "job")
					if operationError != nil {
						return operationError
					}
					if len(jobs) != 1 || jobs[0].ID != failedJob.ID || jobs[0].Status != "failed" {
						test.Fatalf("advisory boundary started forced work: %+v", jobs)
					}
					return nil
				})
				testutil.RequireNoError(test, operationError)
				// A larger selected catalog limit must immediately govern admission,
				// even though the saved view previously measured the smaller limit.
				larger := fixture.prepare(test, messages, 1000000)
				if fixture.view(test).InputLimit != 1000000 || len(fixture.view(test).Removed) != 0 || snapshotText(larger.Request) != snapshotText(initial.Request) {
					test.Fatal("larger model limit did not release the request with its history intact")
				}
			})
		}
	}
}

func TestUrgentWrapupDoesNotWaitForPendingMemoryPreparation(test *testing.T) {
	fixture := newPluginFixture(test)
	messages := fixtureMessages(fixture.session, 10000)
	initial := fixture.prepare(test, messages, 200000)
	measurement := measuredInput(fixture.session, initial.Request.Messages, 200000)
	budget, operationError := measurement.Measure(context.Background(), initial.Request)
	testutil.RequireNoError(test, operationError)
	workerStarted := make(chan struct{})
	finishWorker := make(chan struct{})
	defer close(finishWorker)
	fixture.services.mutex.Lock()
	fixture.services.respond = func(operationContext context.Context, request harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error) {
		close(workerStarted)
		select {
		case <-finishWorker:
			return harness.WorkspaceAgentResponse{Text: `{"memories":[]}`}, nil
		case <-operationContext.Done():
			return harness.WorkspaceAgentResponse{}, operationContext.Err()
		}
	}
	fixture.services.mutex.Unlock()
	view := fixture.view(test)
	dropMessages(test, fixture, []int64{view.Sources["decision"]}, true)
	select {
	case <-workerStarted:
	case <-time.After(5 * time.Second):
		test.Fatal("memory worker did not start")
	}
	_, operationError = fixture.plugin.requestWrapup(context.Background(), fixture.session)
	testutil.RequireNoError(test, operationError)
	operationContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	selection, operationError := fixture.plugin.PrepareContext(operationContext, measuredInput(fixture.session, messages, budget.InputTokens*100/70))
	testutil.RequireNoError(test, operationError)
	if snapshotText(selection.Request) != snapshotText(initial.Request) || len(fixture.view(test).Removed) != 0 {
		test.Fatal("pending preparation changed the delivered context")
	}
	last := selection.Request.Messages[len(selection.Request.Messages)-1]
	if last.Role != atom.RoleRuntime || !strings.Contains(last.Content[0].Text, `"notice":"urgent"`) || !strings.Contains(last.Content[0].Text, `"target_percent":45`) {
		test.Fatal("urgent reduction goal was not delivered as advisory tail metadata")
	}
}

func weightedContextInput(session atom.Session, messages []atom.Message, protectedTokens int) harness.ContextRequest {
	input := measuredInput(session, messages, 10000)
	input.Measure = func(operationContext context.Context, request atom.Request) (harness.RequestBudget, error) {
		tokens := 2000
		for _, message := range request.Messages {
			switch message.ID {
			case "decision":
				tokens += 2000
			case "tool-result":
				tokens += 3000
			case "current":
				tokens += protectedTokens
			}
		}
		return harness.RequestBudget{ContextLimit: 11024, OutputReserve: 1024, InputLimit: 10000, InputTokens: tokens, Estimated: true}, operationContext.Err()
	}
	input.Budget, _ = input.Measure(context.Background(), input.Request)
	return input
}

func TestForcedCompactionAcceptsPreparedContextBelowHardCeilingAboveTarget(test *testing.T) {
	fixture := newPluginFixture(test)
	messages := fixtureMessages(fixture.session, 1000)
	fixture.prepare(test, messages, 200000)
	failDecisionPreparation(test, fixture)
	view := fixture.view(test)
	dropMessages(test, fixture, []int64{view.Sources["tool-plan"], view.Sources["tool-result"]}, false)
	ready := waitForJob(test, fixture, "ready")
	input := weightedContextInput(fixture.session, messages, 3000)
	selection, operationError := fixture.plugin.PrepareContext(context.Background(), input)
	testutil.RequireNoError(test, operationError)
	budget, operationError := input.Measure(context.Background(), selection.Request)
	testutil.RequireNoError(test, operationError)
	if budget.InputTokens != 7000 || selection.InputCeiling != 7999 {
		test.Fatalf("unexpected reduced input or hard ceiling: %+v %+v", budget, selection)
	}
	view = fixture.view(test)
	if view.Revision != 1 || len(view.Removed) != 2 || !view.Removed["tool-plan"] || !view.Removed["tool-result"] || view.Removed["decision"] || view.Removed["current"] {
		test.Fatalf("partial compaction removed unsupported/protected sources: %+v", view)
	}
	stored, operationError := readValue[memoryJob](context.Background(), fixture.plugin.database, fixture.session.InstanceID, "job", ready.ID)
	testutil.RequireNoError(test, operationError)
	if stored.Status != "applied" {
		test.Fatalf("ready work was not committed: %s", stored.Status)
	}
}

func TestForcedCompactionStillRejectsContextAboveHardCeiling(test *testing.T) {
	fixture := newPluginFixture(test)
	messages := fixtureMessages(fixture.session, 1000)
	fixture.prepare(test, messages, 200000)
	failDecisionPreparation(test, fixture)
	view := fixture.view(test)
	dropMessages(test, fixture, []int64{view.Sources["tool-plan"], view.Sources["tool-result"]}, false)
	waitForJob(test, fixture, "ready")
	_, operationError := fixture.plugin.PrepareContext(context.Background(), weightedContextInput(fixture.session, messages, 9000))
	if operationError == nil || !strings.Contains(operationError.Error(), "mandatory ceiling is below 80%") {
		test.Fatalf("unsafe forced context was admitted or misreported: %v", operationError)
	}
	if !reflect.DeepEqual(view.Removed, fixture.view(test).Removed) || fixture.view(test).Revision != view.Revision {
		test.Fatal("failed forced compaction committed a partial context change")
	}
}
