package contextplugin

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestWorkspaceDisableWaitsForRequestAndKeepsDeliveredView(test *testing.T) {
	fixture := newPluginFixture(test)
	messages := fixtureMessages(fixture.session, 1000)
	before := fixture.prepare(test, messages, 200000)
	operationContext, release, operationError := fixture.plugin.BeginRequest(harness.WithSession(context.Background(), fixture.session), fixture.session)
	testutil.RequireNoError(test, operationError)
	state, operationError := fixture.plugin.UpdateConfiguration(context.Background(), fixture.session.InstanceID, json.RawMessage(`{"enabled":false}`))
	testutil.RequireNoError(test, operationError)
	if !state.Pending || !state.Enabled || state.RequestedEnabled {
		test.Fatalf("disable did not respect active request: %+v", state)
	}
	tool := &memoryTool{plugin: fixture.plugin, name: "remember"}
	available, operationError := tool.Available(operationContext)
	testutil.RequireNoError(test, operationError)
	if !available {
		test.Fatal("accepted tool group lost its configuration")
	}
	child := fixture.session
	child.ID = atom.SessionID(newIdentifier())
	child.Parent = fixture.session.ID
	childContext, cancelChild := context.WithTimeout(operationContext, time.Second)
	defer cancelChild()
	_, releaseChild, operationError := fixture.plugin.BeginRequest(childContext, child)
	testutil.RequireNoError(test, operationError)
	releaseChild()
	release()
	state, operationError = fixture.plugin.ReadState(context.Background(), fixture.session.InstanceID)
	testutil.RequireNoError(test, operationError)
	if state.Enabled || state.Pending {
		test.Fatalf("disable was not applied: enabled=%t requested=%t pending=%t", state.Enabled, state.RequestedEnabled, state.Pending)
	}
	after := fixture.prepare(test, messages, 200000)
	if !reflect.DeepEqual(before.Request.Messages, after.Request.Messages) {
		test.Fatal("disable rewrote delivered context")
	}
	available, operationError = tool.Available(harness.WithSession(context.Background(), fixture.session))
	testutil.RequireNoError(test, operationError)
	if available {
		test.Fatal("disabled tool is still available to a new request")
	}
}

func TestDeletionRetainsSearchableArchiveAndRejectsLatePublication(test *testing.T) {
	fixture := newPluginFixture(test)
	messages := fixtureMessages(fixture.session, 1000)
	for _, message := range messages {
		testutil.RequireNoError(test, fixture.services.database.Sessions().Append(context.Background(), message))
	}
	fixture.prepare(test, messages, 200000)
	view := fixture.view(test)
	dropMessages(test, fixture, []int64{view.Sources["decision"]}, true)
	job := waitForJob(test, fixture, "ready")
	complete, operationError := fixture.plugin.BeginHistoryChange(context.Background(), harness.HistoryChange{Kind: harness.HistoryDelete, WorkspaceID: fixture.session.InstanceID, Snapshots: []harness.HistorySnapshot{{Session: fixture.session, Messages: messages}}})
	testutil.RequireNoError(test, operationError)
	_, operationError = fixture.services.database.Sessions().DeleteConversation(context.Background(), fixture.session.ID)
	testutil.RequireNoError(test, operationError)
	testutil.RequireNoError(test, complete(context.Background(), true))
	if operationError := fixture.plugin.publishProposals(context.Background(), fixture.session.InstanceID, job); operationError == nil {
		test.Fatal("late worker published into deleted history")
	}
	view = fixture.view(test)
	if !view.Deleted || view.Mutation != nil {
		test.Fatalf("deletion state: %+v", view)
	}
	other := fixture.session
	other.ID = atom.SessionID(newIdentifier())
	testutil.RequireNoError(test, fixture.services.database.Sessions().Save(context.Background(), other))
	_, operationError = fixture.plugin.PrepareContext(context.Background(), measuredInput(other, []atom.Message{testMessage("hello", atom.RoleUser, "Hello")}, 200000))
	testutil.RequireNoError(test, operationError)
	reply, operationError := fixture.plugin.searchMemory(context.Background(), other, memorySearchInput{Query: "PostgreSQL", Compression: "low"})
	testutil.RequireNoError(test, operationError)
	if len(reply.Matches) != 0 {
		test.Fatal("deleted memory appeared in normal retrieval")
	}
	reply, operationError = fixture.plugin.searchMemory(context.Background(), other, memorySearchInput{Query: "PostgreSQL", Source: "archive", Compression: "raw", IncludeDeleted: true})
	testutil.RequireNoError(test, operationError)
	if len(reply.Matches) == 0 || !reply.Matches[0].Deleted {
		test.Fatal("deleted raw evidence was not recoverable")
	}
	requireContains(test, reply.Matches[0].Data, "Use PostgreSQL for durable data.")
}

func TestInterruptedHistoryChangeRecoversFromAuthoritativeStore(test *testing.T) {
	fixture := newPluginFixture(test)
	messages := fixtureMessages(fixture.session, 1000)
	for _, message := range messages {
		testutil.RequireNoError(test, fixture.services.database.Sessions().Append(context.Background(), message))
	}
	fixture.prepare(test, messages, 200000)
	_, operationError := fixture.plugin.BeginHistoryChange(context.Background(), harness.HistoryChange{Kind: harness.HistoryRevert, WorkspaceID: fixture.session.InstanceID, KeepMessageID: "decision", Snapshots: []harness.HistorySnapshot{{Session: fixture.session, Messages: messages}}})
	testutil.RequireNoError(test, operationError)
	_, operationError = fixture.services.database.Sessions().DeleteAfter(context.Background(), fixture.session.ID, "decision")
	testutil.RequireNoError(test, operationError)
	testutil.RequireNoError(test, fixture.plugin.Close(context.Background()))
	restarted, operationError := New(context.Background(), Options{DatabaseURL: fixture.databaseURL, EmbeddingModel: "test-word-vectors-v1", Services: harness.PluginServices{Settings: fixture.services.database.Settings(), Conversations: fixture.services, Workspaces: fixture.services, Models: fixture.services, Embeddings: testEmbeddings{}, Usage: fixture.services.database.Usage()}})
	testutil.RequireNoError(test, operationError)
	fixture.plugin = restarted
	testutil.RequireNoError(test, restarted.recoverHistoryChanges(context.Background(), fixture.session.InstanceID))
	view := fixture.view(test)
	if view.Mutation != nil || view.Deleted {
		test.Fatalf("revert recovery: %+v", view)
	}
	archived, operationError := readValue[source](context.Background(), fixture.plugin.database, fixture.session.InstanceID, "source", sourceKey(view.Sources["tool-result"]))
	testutil.RequireNoError(test, operationError)
	if !archived.Deleted {
		test.Fatal("reverted output became live again")
	}
	kept, operationError := readValue[source](context.Background(), fixture.plugin.database, fixture.session.InstanceID, "source", sourceKey(view.Sources["decision"]))
	testutil.RequireNoError(test, operationError)
	if kept.Deleted {
		test.Fatal("kept anchor was deleted")
	}
}

func TestRawSearchContinuationPreservesUnicodeAndWorkspaceBoundary(test *testing.T) {
	fixture := newPluginFixture(test)
	text := strings.Repeat("needle café 🧠\n", 3000)
	messages := fixtureMessages(fixture.session, 1000)
	messages[0].Content = []atom.Content{{Type: atom.Text, Text: text}}
	fixture.prepare(test, messages, 200000)
	view := fixture.view(test)
	dropMessages(test, fixture, []int64{view.Sources["decision"]}, false)
	waitForJob(test, fixture, "ready")
	input := memorySearchInput{Query: "needle", Source: "archive", Compression: "raw", Limit: 1}
	var recovered strings.Builder
	for page := 0; page < 10; page++ {
		reply, operationError := fixture.plugin.searchMemory(context.Background(), fixture.session, input)
		testutil.RequireNoError(test, operationError)
		for _, match := range reply.Matches {
			if len(match.Data) > searchTextBudget {
				test.Fatal("unbounded search page")
			}
			recovered.WriteString(match.Data)
		}
		if reply.NextCursor == "" {
			break
		}
		wrong := fixture.session
		wrong.InstanceID = "other-workspace"
		otherInput := input
		otherInput.Cursor = reply.NextCursor
		if _, operationError := fixture.plugin.searchMemory(context.Background(), wrong, otherInput); operationError == nil {
			test.Fatal("cursor crossed a workspace boundary")
		}
		input.Cursor = reply.NextCursor
	}
	if recovered.String() != "[user]\n"+text+"\n\n" {
		test.Fatalf("raw continuation changed bytes: got %d want %d", recovered.Len(), len(text)+9)
	}
}

func sourceKey(identifier int64) string { return strconv.FormatInt(identifier, 10) }

func TestNewSessionReceivesWorkspaceMemoryWithoutAdvancingAnotherView(test *testing.T) {
	fixture := newPluginFixture(test)
	messages := fixtureMessages(fixture.session, 1000)
	first := fixture.prepare(test, messages, 200000)
	view := fixture.view(test)
	dropMessages(test, fixture, []int64{view.Sources["decision"]}, true)
	waitForJob(test, fixture, "ready")
	other := fixture.session
	other.ID = atom.SessionID(newIdentifier())
	other.CreatedAt = time.Now()
	testutil.RequireNoError(test, fixture.services.database.Sessions().Save(context.Background(), other))
	selection, operationError := fixture.plugin.PrepareContext(context.Background(), measuredInput(other, []atom.Message{testMessage("greeting", atom.RoleUser, "Hello")}, 200000))
	testutil.RequireNoError(test, operationError)
	requireContains(test, snapshotText(selection.Request), "L Use PostgreSQL")
	after := fixture.prepare(test, messages, 200000)
	if snapshotText(first.Request) != snapshotText(after.Request) {
		test.Fatal("new session refreshed an existing session's memory")
	}
}
