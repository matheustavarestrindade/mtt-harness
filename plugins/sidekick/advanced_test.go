package sidekick

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/workspaceread"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

type fixtureFiles struct{ directory string }

func TestAlreadyVisibleMemoryDoesNotStartAFilteringCall(test *testing.T) {
	fixture := newFixture(test, false)
	_, operationError := fixture.plugin.UpdateConfiguration(context.Background(), fixture.session.InstanceID, json.RawMessage(`{"debounce_ms":100}`))
	testutil.RequireNoError(test, operationError)
	operationContext, release := fixture.begin(test)
	defer release()
	request := fixture.request()
	request.Request.Messages = append([]atom.Message{{Role: atom.RoleSystem, Ephemeral: true, Content: []atom.Content{{Type: atom.Text, Text: "Startup instructions"}}}, {Role: atom.RoleSystem, Ephemeral: true, Content: []atom.Content{{Type: atom.Text, Text: "<memory>Use PostgreSQL for transactional storage.</memory>"}}}}, request.Request.Messages...)
	_, operationError = fixture.plugin.PrepareReminder(operationContext, request)
	testutil.RequireNoError(test, operationError)
	fixture.waitState(test, func(state sessionState) bool { return state.LastStatus == "skipped" })
	fixture.services.mutex.Lock()
	calls := len(fixture.services.calls)
	fixture.services.mutex.Unlock()
	if calls != 0 {
		test.Fatal("visible memory caused another model call")
	}
	counts, operationError := fixture.plugin.database.Counters(context.Background(), fixture.session.InstanceID)
	testutil.RequireNoError(test, operationError)
	if counts["sources/already_present"] != 1 {
		test.Fatalf("missing visible-source accounting: %+v", counts)
	}
}

func (files fixtureFiles) SearchWorkspaceFiles(operationContext context.Context, query harness.WorkspaceFileQuery) (harness.WorkspaceFileResult, error) {
	return workspaceread.Search(operationContext, files.directory, query)
}
func (files fixtureFiles) VerifyWorkspaceFiles(operationContext context.Context, _ string, _ atom.SessionID, references []harness.WorkspaceFileReference) (bool, error) {
	return workspaceread.Verify(operationContext, files.directory, references)
}

func TestFileOnlyRetrievalAndChangedFileFence(test *testing.T) {
	fixture := newFixture(test, false)
	directory := test.TempDir()
	path := filepath.Join(directory, "AGENTS.md")
	testutil.RequireNoError(test, os.WriteFile(path, []byte("Use PostgreSQL for transactional persistence.\n"), 0600))
	fixture.plugin.services.Files = fixtureFiles{directory: directory}
	_, operationError := fixture.plugin.UpdateConfiguration(context.Background(), fixture.session.InstanceID, json.RawMessage(`{"memory_enabled":false,"files_enabled":true}`))
	testutil.RequireNoError(test, operationError)
	fixture.services.run = func(_ context.Context, input harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error) {
		text := input.Messages[1].Content[0].Text
		var data struct {
			Sources []sourceData `json:"sources"`
		}
		testutil.RequireNoError(test, json.Unmarshal([]byte(text[strings.Index(text, "\n")+1:]), &data))
		if len(data.Sources) != 1 || data.Sources[0].Origin != "AGENTS.md:1-1" {
			test.Fatalf("unexpected file source: %+v", data.Sources)
		}
		output, _ := json.Marshal(workerOutput{Notes: []suggestedNote{{Text: "Transactional persistence uses PostgreSQL.", Sources: []string{data.Sources[0].Key}}}})
		return harness.WorkspaceAgentResponse{Text: string(output)}, nil
	}
	operationContext, release := fixture.begin(test)
	defer release()
	state := fixture.waitState(test, func(state sessionState) bool { return state.Pending != nil })
	if len(state.Pending.Files) != 1 || len(state.Pending.Memories) != 0 {
		test.Fatal("source attribution was lost")
	}
	testutil.RequireNoError(test, os.WriteFile(path, []byte("The storage decision has changed."), 0600))
	contribution, operationError := fixture.plugin.PrepareReminder(operationContext, fixture.request())
	testutil.RequireNoError(test, operationError)
	if len(contribution.Messages) != 0 {
		test.Fatal("changed file guidance was injected")
	}
}

func TestNewUserAndDoingFenceAnUncooperativeWorker(test *testing.T) {
	fixture := newFixture(test, false)
	entered := make(chan struct{})
	finish := make(chan struct{})
	fixture.services.run = func(context.Context, harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error) {
		close(entered)
		<-finish
		return harness.WorkspaceAgentResponse{Text: `{"notes":[{"text":"Stale database hint.","sources":["memory:decision:1"]}]}`}, nil
	}
	_, release := fixture.begin(test)
	defer release()
	select {
	case <-entered:
	case <-time.After(time.Second):
		test.Fatal("worker did not start")
	}
	fixture.services.mutex.Lock()
	fixture.services.messages = append(fixture.services.messages, atom.Message{ID: "new-user", SessionID: fixture.session.ID, Role: atom.RoleUser, CreatedAt: time.Now(), Content: []atom.Content{{Type: atom.Text, Text: "Stop that task."}}})
	fixture.services.mutex.Unlock()
	fixture.event(fixture.services.updateDoing(""))
	close(finish)
	fixture.waitState(test, func(state sessionState) bool { return state.LastStatus == "cancelled" || state.LastStatus == "stale" })
	state, operationError := fixture.plugin.database.ReadSession(context.Background(), fixture.session.InstanceID, string(fixture.session.ID))
	testutil.RequireNoError(test, operationError)
	if state.Pending != nil {
		test.Fatal("old source published after a new user message")
	}
}

func TestHistoryMutationJoinsItsBackgroundWorker(test *testing.T) {
	fixture := newFixture(test, false)
	entered := make(chan struct{})
	finish := make(chan struct{})
	fixture.services.run = func(context.Context, harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error) {
		close(entered)
		<-finish
		return harness.WorkspaceAgentResponse{Text: `{"notes":[]}`}, nil
	}
	_, release := fixture.begin(test)
	defer release()
	select {
	case <-entered:
	case <-time.After(time.Second):
		test.Fatal("worker did not start")
	}
	messages, _ := fixture.services.Messages(context.Background(), fixture.session.ID)
	completed := make(chan error, 1)
	go func() {
		complete, operationError := fixture.plugin.BeginHistoryChange(context.Background(), harness.HistoryChange{Kind: harness.HistoryRevert, WorkspaceID: fixture.session.InstanceID, KeepMessageID: "user", Snapshots: []harness.HistorySnapshot{{Session: fixture.session, Messages: messages}}})
		if operationError == nil {
			operationError = complete(context.Background(), true)
		}
		completed <- operationError
	}()
	fixture.waitState(test, func(state sessionState) bool { return state.Fenced })
	select {
	case <-completed:
		test.Fatal("history mutation did not join the worker")
	default:
	}
	close(finish)
	select {
	case operationError := <-completed:
		testutil.RequireNoError(test, operationError)
	case <-time.After(time.Second):
		test.Fatal("history mutation did not complete")
	}
}

func TestDeferredHintDoesNotCommitAndQuietOutputRequiresRealCitations(test *testing.T) {
	fixture := newFixture(test, false)
	operationContext, release := fixture.begin(test)
	defer release()
	fixture.waitState(test, func(state sessionState) bool { return state.Pending != nil })
	contribution, operationError := fixture.plugin.PrepareReminder(operationContext, fixture.request())
	testutil.RequireNoError(test, operationError)
	testutil.RequireNoError(test, contribution.Deferred(operationContext))
	state, operationError := fixture.plugin.database.ReadSession(context.Background(), fixture.session.InstanceID, string(fixture.session.ID))
	testutil.RequireNoError(test, operationError)
	if state.Pending == nil || len(state.Events) != 0 {
		test.Fatal("deferral consumed or published the note")
	}
	fixture.services.mutex.Lock()
	fixture.services.run = func(context.Context, harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error) {
		return harness.WorkspaceAgentResponse{Text: `{"notes":[{"text":"Unfounded rule","sources":["invented"]}]}`}, nil
	}
	fixture.services.mutex.Unlock()
	fixture.event(fixture.services.updateDoing("Review a different storage issue"))
	fixture.waitState(test, func(state sessionState) bool { return state.LastStatus == "error" })
	state, operationError = fixture.plugin.database.ReadSession(context.Background(), fixture.session.InstanceID, string(fixture.session.ID))
	testutil.RequireNoError(test, operationError)
	if state.Pending != nil {
		test.Fatal("uncited output was published")
	}
}
