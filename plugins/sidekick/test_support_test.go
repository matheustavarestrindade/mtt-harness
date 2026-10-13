package sidekick

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	memorystore "github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

type memoryRepository struct {
	mutex  sync.Mutex
	states map[string]sessionState
	runs   map[string]workerRun
	counts map[string]int64
}

func cloneState(state sessionState) sessionState {
	data, _ := json.Marshal(state)
	var result sessionState
	_ = json.Unmarshal(data, &result)
	return result
}
func (database *memoryRepository) ReadSession(_ context.Context, workspaceID, sessionID string) (sessionState, error) {
	database.mutex.Lock()
	defer database.mutex.Unlock()
	return cloneState(database.states[workspaceID+sessionID]), nil
}
func (database *memoryRepository) UpdateSession(operationContext context.Context, workspaceID, sessionID string, update func(*sessionState) (mutation, error)) (sessionState, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return sessionState{}, operationError
	}
	database.mutex.Lock()
	defer database.mutex.Unlock()
	key := workspaceID + sessionID
	state := cloneState(database.states[key])
	changes, operationError := update(&state)
	if operationError != nil {
		return state, operationError
	}
	state.Revision++
	database.states[key] = cloneState(state)
	if changes.PurgeRuns {
		for identifier, run := range database.runs {
			if run.WorkspaceID == workspaceID && string(run.SessionID) == sessionID {
				delete(database.runs, identifier)
			}
		}
	}
	if changes.Run != nil {
		database.runs[changes.Run.ID] = *changes.Run
	}
	if changes.RunID != "" {
		run := database.runs[changes.RunID]
		run.Status = changes.RunStatus
		database.runs[changes.RunID] = run
	}
	for name, value := range changes.Counters {
		database.counts[name] += value
	}
	return state, nil
}
func (database *memoryRepository) Counters(context.Context, string) (map[string]int64, error) {
	database.mutex.Lock()
	defer database.mutex.Unlock()
	result := map[string]int64{}
	for name, value := range database.counts {
		result[name] = value
	}
	return result, nil
}
func (database *memoryRepository) Close() {}

type fixtureServices struct {
	mutex         sync.Mutex
	session       atom.Session
	task          atom.TaskState
	messages      []atom.Message
	calls         []harness.WorkspaceAgentRequest
	queries       []harness.MemoryQuery
	memoryVersion int
	run           func(context.Context, harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error)
}

func (services *fixtureServices) Get(_ context.Context, identifier atom.SessionID) (atom.Session, error) {
	services.mutex.Lock()
	defer services.mutex.Unlock()
	if identifier != services.session.ID {
		return atom.Session{}, harness.ErrConversationUnavailable
	}
	return services.session, nil
}
func (services *fixtureServices) Messages(context.Context, atom.SessionID) ([]atom.Message, error) {
	services.mutex.Lock()
	defer services.mutex.Unlock()
	return append([]atom.Message(nil), services.messages...), nil
}
func (services *fixtureServices) Workspace(_ context.Context, identifier string) (atom.InstanceSpec, error) {
	return atom.InstanceSpec{ID: identifier}, nil
}
func (services *fixtureServices) ReadTaskState(_ context.Context, workspaceID string, identifier atom.SessionID) (atom.TaskState, error) {
	services.mutex.Lock()
	defer services.mutex.Unlock()
	if identifier != services.session.ID || workspaceID != services.session.InstanceID {
		return atom.TaskState{}, harness.ErrConversationUnavailable
	}
	return services.task, nil
}
func (services *fixtureServices) Model(context.Context, string, string, string) (atom.ModelInfo, error) {
	return atom.ModelInfo{ID: "worker", ContextMax: 65536}, nil
}
func (services *fixtureServices) RenderPrompt(context.Context, atom.Session, string, string) (string, error) {
	return "Select useful cited context and return JSON notes.", nil
}
func (services *fixtureServices) Agents(context.Context, string) ([]atom.AgentStatistics, error) {
	return nil, nil
}
func (services *fixtureServices) SearchWorkspaceMemory(_ context.Context, query harness.MemoryQuery) ([]harness.MemoryReference, error) {
	services.mutex.Lock()
	defer services.mutex.Unlock()
	services.queries = append(services.queries, query)
	return []harness.MemoryReference{{ID: "decision", Version: services.memoryVersion, Text: "Use PostgreSQL for transactional storage."}}, nil
}
func (services *fixtureServices) VerifyWorkspaceMemory(_ context.Context, _ string, _ atom.SessionID, references []harness.MemoryReference) (bool, error) {
	services.mutex.Lock()
	defer services.mutex.Unlock()
	for _, reference := range references {
		if reference.Version != services.memoryVersion {
			return false, nil
		}
	}
	return true, nil
}
func (services *fixtureServices) Run(operationContext context.Context, input harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error) {
	services.mutex.Lock()
	services.calls = append(services.calls, input)
	callback := services.run
	version := services.memoryVersion
	services.mutex.Unlock()
	if callback != nil {
		return callback(operationContext, input)
	}
	return harness.WorkspaceAgentResponse{Text: fmt.Sprintf(`{"notes":[{"text":"Use PostgreSQL for transactional storage.","sources":["memory:decision:%d"]}]}`, version)}, nil
}
func (services *fixtureServices) updateDoing(title string) atom.TaskState {
	services.mutex.Lock()
	defer services.mutex.Unlock()
	services.task.Revision++
	services.task.UpdatedAt = time.Now()
	if title == "" {
		services.task.Doing = nil
	} else {
		services.task.Doing = &atom.DoingState{Title: title, Description: "Check database ownership and persistence."}
	}
	return services.task
}

type fixture struct {
	plugin   *Plugin
	services *fixtureServices
	runtime  *harness.Harness
	session  atom.Session
}

func newFixture(test *testing.T, postgres bool) *fixture {
	test.Helper()
	session := atom.Session{ID: atom.SessionID(newIdentifier()), InstanceID: newIdentifier(), Model: "fixture/main"}
	services := &fixtureServices{session: session, memoryVersion: 1, messages: []atom.Message{{ID: "user", SessionID: session.ID, Seq: 1, Role: atom.RoleUser, CreatedAt: time.Now().Add(-time.Minute), Content: []atom.Content{{Type: atom.Text, Text: "Fix the storage implementation."}}}}}
	services.task = atom.TaskState{SessionID: session.ID}
	services.updateDoing("Inspect persistence")
	storage := memorystore.New()
	testutil.RequireNoError(test, storage.Settings().Save(context.Background(), session.InstanceID, settingsKey, `{"enabled":true,"worker_model":"fixture/worker","files_enabled":false,"debounce_ms":10,"cooldown_ms":0}`))
	plugin, operationError := New(context.Background(), Options{Services: harness.PluginServices{Settings: storage.Settings(), Conversations: services, Workspaces: services, Models: services, Usage: services, Prompts: services, Memory: services, Tasks: services}, PromptVersion: "fixture"})
	testutil.RequireNoError(test, operationError)
	if postgres {
		databaseURL := os.Getenv("MTT_TEST_DATABASE_URL")
		if databaseURL == "" {
			test.Skip("MTT_TEST_DATABASE_URL is not configured")
		}
		plugin.database, operationError = openRepository(context.Background(), databaseURL)
		testutil.RequireNoError(test, operationError)
	} else {
		plugin.database = &memoryRepository{states: map[string]sessionState{}, runs: map[string]workerRun{}, counts: map[string]int64{}}
	}
	plugin.unavailable = nil
	runtime := harness.New()
	testutil.RequireNoError(test, plugin.Setup(runtime))
	test.Cleanup(func() { testutil.RequireNoError(test, plugin.Close(context.Background())) })
	return &fixture{plugin: plugin, services: services, runtime: runtime, session: session}
}
func (fixture *fixture) begin(test *testing.T) (context.Context, func()) {
	test.Helper()
	operationContext, release, operationError := fixture.plugin.BeginRequest(context.Background(), fixture.session)
	testutil.RequireNoError(test, operationError)
	return operationContext, release
}
func (fixture *fixture) event(state atom.TaskState) {
	data, _ := json.Marshal(state)
	fixture.runtime.Emit(harness.WithSession(context.Background(), fixture.session), atom.Event{Name: atom.EventTaskStateUpdated, InstanceID: fixture.session.InstanceID, SessionID: fixture.session.ID, Payload: data})
}
func (fixture *fixture) waitState(test *testing.T, condition func(sessionState) bool) sessionState {
	test.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state, operationError := fixture.plugin.database.ReadSession(context.Background(), fixture.session.InstanceID, string(fixture.session.ID))
		testutil.RequireNoError(test, operationError)
		if condition(state) {
			return state
		}
		time.Sleep(5 * time.Millisecond)
	}
	state, _ := fixture.plugin.database.ReadSession(context.Background(), fixture.session.InstanceID, string(fixture.session.ID))
	test.Fatalf("sidekick state did not settle: %+v", state)
	return state
}
func (fixture *fixture) request() harness.ContextRequest {
	messages, _ := fixture.services.Messages(context.Background(), fixture.session.ID)
	return harness.ContextRequest{Session: fixture.session, Request: atom.Request{Model: "main", Messages: messages}, ModelID: "fixture/main", Budget: harness.RequestBudget{InputLimit: 32000, InputTokens: 100}}
}
