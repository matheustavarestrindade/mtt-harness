package contextplugin

import (
	"context"
	"reflect"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestPublicMemoryRetrievalUsesMediumWithoutChangingSessionView(test *testing.T) {
	fixture := newPluginFixture(test)
	record := seedMemory(test, fixture, "Database", "PostgreSQL stores transaction data.", false)
	record.Text = summaries{Low: "LOW detail about database conventions.", Medium: "MEDIUM PostgreSQL transaction rule.", High: "HIGH database summary."}
	testutil.RequireNoError(test, fixture.plugin.database.Transact(context.Background(), fixture.session.InstanceID, func(database transaction) error { return database.Put("memory", record.ID, record) }))
	fixture.prepare(test, fixtureMessages(fixture.session, 0), 20000)
	before := fixture.view(test)
	query := harness.MemoryQuery{WorkspaceID: fixture.session.InstanceID, SourceSessionID: fixture.session.ID, Agent: "spaced_repetition.instruction_recovery", RunID: "recovery-run", Query: "PostgreSQL", Compression: "medium", Limit: 1, MaxBytes: 2000}
	matches, operationError := fixture.plugin.SearchWorkspaceMemory(context.Background(), query)
	testutil.RequireNoError(test, operationError)
	if len(matches) != 1 || matches[0].Text != record.Text.Medium {
		test.Fatalf("wrong retrieval compression: %+v", matches)
	}
	if after := fixture.view(test); !reflect.DeepEqual(before, after) {
		test.Fatal("worker retrieval changed the cached session view")
	}
	query.WorkspaceID = "another-workspace"
	if _, operationError := fixture.plugin.SearchWorkspaceMemory(context.Background(), query); operationError == nil {
		test.Fatal("worker used a source session from another workspace")
	}
}
