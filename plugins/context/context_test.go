package contextplugin

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestDeferredReductionKeepsPrefixUntilCommittedRefresh(test *testing.T) {
	fixture := newPluginFixture(test)
	messages := fixtureMessages(fixture.session, 5000)
	before := fixture.prepare(test, messages, 200000)
	view := fixture.view(test)
	selected := []int64{view.Sources["decision"], view.Sources["tool-plan"], view.Sources["tool-result"]}
	dropMessages(test, fixture, selected, true)
	waitForJob(test, fixture, "ready")
	queued := fixture.prepare(test, messages, 200000)
	if !reflect.DeepEqual(before.Request.Messages, queued.Request.Messages) {
		test.Fatal("background preparation changed the cached request")
	}
	if len(fixture.view(test).Removed) != 0 {
		test.Fatal("ctx_drop removed messages before wrap-up")
	}
	_, operationError := fixture.plugin.requestWrapup(context.Background(), fixture.session)
	testutil.RequireNoError(test, operationError)
	wrapped := fixture.prepare(test, messages, 200000)
	requireContains(test, snapshotText(wrapped.Request), "L Use PostgreSQL for durable workspace")
	for _, message := range wrapped.Request.Messages {
		if message.ID == "decision" || message.ID == "tool-plan" || message.ID == "tool-result" {
			test.Fatal("prepared old group remained in request")
		}
	}
	view = fixture.view(test)
	if view.Revision != 1 || len(view.Removed) != 3 {
		test.Fatalf("unexpected projection: %+v", view)
	}
	stable := fixture.prepare(test, messages, 200000)
	if !reflect.DeepEqual(wrapped.Request.Messages, stable.Request.Messages) {
		test.Fatal("memory changed between refreshes")
	}
	_, operationError = fixture.plugin.requestWrapup(context.Background(), fixture.session)
	testutil.RequireNoError(test, operationError)
	noop := fixture.prepare(test, messages, 200000)
	if snapshotText(noop.Request) != snapshotText(stable.Request) {
		test.Fatal("empty wrap-up aged memory")
	}
	messages = append(messages, testMessage("new-answer", atom.RoleAssistant, "The current step is complete."), testMessage("next-question", atom.RoleUser, "Next step."))
	fixture.prepare(test, messages, 200000)
	view = fixture.view(test)
	dropMessages(test, fixture, []int64{view.Sources["ack"]}, false)
	waitForJob(test, fixture, "ready")
	_, operationError = fixture.plugin.requestWrapup(context.Background(), fixture.session)
	testutil.RequireNoError(test, operationError)
	second := fixture.prepare(test, messages, 200000)
	requireContains(test, snapshotText(second.Request), "M PostgreSQL stores durable data.")
	for _, entry := range fixture.view(test).Snapshot {
		if strings.Contains(snapshotText(second.Request), entry.MemoryID) {
			test.Fatal("memory IDs entered injected text")
		}
	}
	messages = append(messages, testMessage("later-answer", atom.RoleAssistant, "Completed."), testMessage("later-question", atom.RoleUser, "Continue again."))
	fixture.prepare(test, messages, 200000)
	view = fixture.view(test)
	dropMessages(test, fixture, []int64{view.Sources["current"]}, false)
	waitForJob(test, fixture, "ready")
	_, operationError = fixture.plugin.requestWrapup(context.Background(), fixture.session)
	testutil.RequireNoError(test, operationError)
	third := fixture.prepare(test, messages, 200000)
	requireContains(test, snapshotText(third.Request), "H Use PostgreSQL.")
	reply, operationError := fixture.plugin.searchMemory(context.Background(), fixture.session, memorySearchInput{Query: "PostgreSQL", Categories: []string{"architecture"}, Compression: "low"})
	testutil.RequireNoError(test, operationError)
	if len(reply.Matches) == 0 {
		test.Fatal("prepared memory is not retrievable by query")
	}
	metrics, operationError := fixture.plugin.Metrics(context.Background(), fixture.session.InstanceID)
	testutil.RequireNoError(test, operationError)
	if metrics.Counters["context.compactor/checkpoints"] != 3 || metrics.Counters["context.compactor/removed_messages"] != 5 {
		test.Fatalf("wrong compaction counters: %+v", metrics.Counters)
	}
	if metrics.Counters["context.presentation/level_L"] != 1 || metrics.Counters["context.presentation/level_M"] != 1 || metrics.Counters["context.presentation/level_H"] != 1 {
		test.Fatal("unchanged snapshots were counted as fresh injections")
	}
}

func TestForcedCheckpointWaitsForPreparedGroups(test *testing.T) {
	fixture := newPluginFixture(test)
	messages := fixtureMessages(fixture.session, 15000)
	selection := fixture.prepare(test, messages, 10000)
	input := measuredInput(fixture.session, selection.Request.Messages, 10000)
	budget, operationError := input.Measure(context.Background(), selection.Request)
	testutil.RequireNoError(test, operationError)
	if budget.InputTokens >= 8000 {
		test.Fatalf("forced request exceeded ceiling: %d", budget.InputTokens)
	}
	view := fixture.view(test)
	if view.Revision != 1 {
		test.Fatalf("forced checkpoint committed %d refreshes instead of one", view.Revision)
	}
	if !view.Removed["tool-plan"] || !view.Removed["tool-result"] || view.Removed["current"] {
		test.Fatal("forced checkpoint broke group or latest-input protection")
	}
}

func TestPartialToolGroupsAndFailedPreparationKeepHistory(test *testing.T) {
	fixture := newPluginFixture(test)
	messages := fixtureMessages(fixture.session, 1000)
	fixture.prepare(test, messages, 200000)
	view := fixture.view(test)
	remember := true
	_, operationError := fixture.plugin.queueDrop(context.Background(), fixture.session, dropInput{MessageIDs: []int64{view.Sources["tool-result"]}, Remember: &remember})
	if operationError == nil {
		test.Fatal("orphaned tool result was accepted")
	}
	fixture.services.mutex.Lock()
	fixture.services.respond = func(context.Context, harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error) {
		return harness.WorkspaceAgentResponse{Text: "invalid JSON"}, nil
	}
	fixture.services.mutex.Unlock()
	dropMessages(test, fixture, []int64{view.Sources["decision"]}, true)
	waitForJob(test, fixture, "failed")
	_, operationError = fixture.plugin.requestWrapup(context.Background(), fixture.session)
	testutil.RequireNoError(test, operationError)
	fixture.prepare(test, messages, 200000)
	if len(fixture.view(test).Removed) != 0 {
		test.Fatal("failed extraction removed source context")
	}
}

func TestRoleEvidenceAndSchemaContracts(test *testing.T) {
	sources := []source{{ID: 1, Role: atom.RoleAssistant, Sequence: 1, Text: "Use Postgres.", MessageID: "proposal"}, {ID: 2, Role: atom.RoleUser, Sequence: 2, Text: "Yes, use that database."}}
	proposal := memoryProposal{Title: "Database", Categories: []string{"architecture"}, Text: summaries{Low: "Use Postgres.", Medium: "Use Postgres.", High: "Use Postgres."}, Kind: "user_fact", Evidence: []memoryEvidence{{SourceID: 1, Quote: "Use Postgres."}}}
	encode := func() string {
		data, _ := json.Marshal(map[string]any{"memories": []memoryProposal{proposal}})
		return string(data)
	}
	if _, operationError := parseProposals(encode(), sources, nil); operationError == nil {
		test.Fatal("assistant proposal became a user fact")
	}
	proposal.Kind = "approved_decision"
	proposal.Evidence[0].ApprovalID = 2
	proposal.Evidence[0].ApprovalQuote = "Yes, use that database."
	_, operationError := parseProposals(encode(), sources, nil)
	testutil.RequireNoError(test, operationError)
	for _, name := range []string{"ctx_drop", "ctx_wrapup", "remember", "search_memory", "list_memory_categories"} {
		tool := &memoryTool{name: name}
		if !json.Valid(tool.InputSchema().JSON) || tool.Description() == "" {
			test.Fatalf("invalid callable contract for %s", name)
		}
	}
}
