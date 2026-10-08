package spacedrepetition

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	memorystore "github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

type testRepository struct {
	mutex   sync.Mutex
	states  map[string]sessionState
	runs    map[string]recoveryRun
	metrics map[string]map[string]int64
}

func newTestRepository() *testRepository {
	return &testRepository{states: map[string]sessionState{}, runs: map[string]recoveryRun{}, metrics: map[string]map[string]int64{}}
}
func cloneState(state sessionState) sessionState {
	data, _ := json.Marshal(state)
	var copy sessionState
	_ = json.Unmarshal(data, &copy)
	return copy
}
func (database *testRepository) ReadSession(operationContext context.Context, workspaceID, sessionID string) (sessionState, error) {
	database.mutex.Lock()
	defer database.mutex.Unlock()
	return cloneState(database.states[workspaceID+":"+sessionID]), operationContext.Err()
}
func (database *testRepository) UpdateSession(operationContext context.Context, workspaceID, sessionID string, update func(*sessionState) (map[string]int64, error)) (sessionState, error) {
	database.mutex.Lock()
	defer database.mutex.Unlock()
	if operationError := operationContext.Err(); operationError != nil {
		return sessionState{}, operationError
	}
	key := workspaceID + ":" + sessionID
	state := cloneState(database.states[key])
	counters, operationError := update(&state)
	if operationError != nil {
		return state, operationError
	}
	state.Revision++
	database.states[key] = cloneState(state)
	database.addCounters(workspaceID, counters)
	return state, nil
}
func (database *testRepository) addCounters(workspaceID string, counters map[string]int64) {
	if database.metrics[workspaceID] == nil {
		database.metrics[workspaceID] = map[string]int64{}
	}
	for name, count := range counters {
		database.metrics[workspaceID][name] += count
	}
}
func (database *testRepository) ReadRecovery(operationContext context.Context, workspaceID, identifier string) (recoveryRun, error) {
	database.mutex.Lock()
	defer database.mutex.Unlock()
	return database.runs[workspaceID+":"+identifier], operationContext.Err()
}
func (database *testRepository) SaveRecovery(operationContext context.Context, run recoveryRun) error {
	database.mutex.Lock()
	defer database.mutex.Unlock()
	if operationError := operationContext.Err(); operationError != nil {
		return operationError
	}
	state := database.states[run.WorkspaceID+":"+run.SessionID]
	if state.Deleted || state.Fenced || state.Epoch != run.Epoch {
		return errSessionRetired
	}
	database.runs[run.WorkspaceID+":"+run.ID] = run
	return nil
}
func (database *testRepository) CompleteRecovery(operationContext context.Context, run recoveryRun, report recoveryReport, counters map[string]int64) error {
	database.mutex.Lock()
	defer database.mutex.Unlock()
	if operationError := operationContext.Err(); operationError != nil {
		return operationError
	}
	key := run.WorkspaceID + ":" + run.SessionID
	state := cloneState(database.states[key])
	if state.Deleted || state.Fenced || state.Epoch != run.Epoch || state.SourceUser != run.SourceUser {
		return errSessionRetired
	}
	state.Pending = &report
	database.states[key] = state
	database.runs[run.WorkspaceID+":"+run.ID] = run
	database.addCounters(run.WorkspaceID, counters)
	return nil
}
func (database *testRepository) DeleteRecoveries(operationContext context.Context, workspaceID, sessionID string) error {
	database.mutex.Lock()
	defer database.mutex.Unlock()
	for key, run := range database.runs {
		if run.WorkspaceID == workspaceID && run.SessionID == sessionID {
			delete(database.runs, key)
		}
	}
	return operationContext.Err()
}
func (database *testRepository) Counters(operationContext context.Context, workspaceID string) (map[string]int64, error) {
	database.mutex.Lock()
	defer database.mutex.Unlock()
	result := map[string]int64{}
	for key, value := range database.metrics[workspaceID] {
		result[key] = value
	}
	for _, run := range database.runs {
		if run.WorkspaceID == workspaceID {
			result["recovery/"+run.Status]++
		}
	}
	return result, operationContext.Err()
}
func (database *testRepository) Close() {}

type testServices struct {
	mutex       sync.Mutex
	session     atom.Session
	messages    []atom.Message
	calls       []harness.WorkspaceAgentRequest
	queries     []harness.MemoryQuery
	run         func(context.Context, harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error)
	unavailable bool
}

func (services *testServices) Get(operationContext context.Context, identifier atom.SessionID) (atom.Session, error) {
	services.mutex.Lock()
	defer services.mutex.Unlock()
	if identifier != services.session.ID || services.session.ID == "" {
		return atom.Session{}, harness.ErrConversationUnavailable
	}
	return services.session, operationContext.Err()
}
func (services *testServices) Messages(operationContext context.Context, identifier atom.SessionID) ([]atom.Message, error) {
	services.mutex.Lock()
	defer services.mutex.Unlock()
	return append([]atom.Message(nil), services.messages...), operationContext.Err()
}
func (services *testServices) Workspace(operationContext context.Context, identifier string) (atom.InstanceSpec, error) {
	return atom.InstanceSpec{ID: identifier, Workspace: "/workspace", Models: []string{"fixture/worker"}}, operationContext.Err()
}
func (services *testServices) Model(operationContext context.Context, workspaceID, model, effort string) (atom.ModelInfo, error) {
	if model != "fixture/worker" {
		return atom.ModelInfo{}, fmt.Errorf("model is not allowed")
	}
	return atom.ModelInfo{ID: model, ContextMax: 262144}, operationContext.Err()
}
func (services *testServices) Run(operationContext context.Context, input harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error) {
	services.mutex.Lock()
	services.calls = append(services.calls, input)
	callback := services.run
	services.mutex.Unlock()
	if callback != nil {
		return callback(operationContext, input)
	}
	if strings.Contains(input.Messages[0].Content[0].Text, "\"queries\"") {
		return harness.WorkspaceAgentResponse{Text: `{"queries":["database policy","writing style"]}`}, nil
	}
	return harness.WorkspaceAgentResponse{Text: `{"guidance":[{"instruction":"Use the prescribed database.","scope":"This workspace","sources":["reported_problem"]}],"next_action":"Apply the supplied database and communication rules to the current task."}`}, nil
}
func (services *testServices) RenderPrompt(operationContext context.Context, session atom.Session, name, model string) (string, error) {
	return name + ": follow the current task and workspace instructions.", operationContext.Err()
}
func (services *testServices) SearchWorkspaceMemory(operationContext context.Context, input harness.MemoryQuery) ([]harness.MemoryReference, error) {
	services.mutex.Lock()
	defer services.mutex.Unlock()
	services.queries = append(services.queries, input)
	if services.unavailable {
		return nil, harness.ErrWorkspaceMemoryUnavailable
	}
	return []harness.MemoryReference{{ID: "policy", Version: 2, Text: "MEDIUM: Use PostgreSQL for transactional storage and concise English replies."}}, operationContext.Err()
}
func (services *testServices) Agents(context.Context, string) ([]atom.AgentStatistics, error) {
	return nil, nil
}

type pluginFixture struct {
	plugin   *Plugin
	services *testServices
	database *testRepository
	session  atom.Session
	settings harness.PluginSettings
}

func newFixture(test *testing.T) *pluginFixture {
	test.Helper()
	database := newTestRepository()
	session := atom.Session{ID: "session", InstanceID: "workspace", Model: "fixture/primary"}
	services := &testServices{session: session, messages: []atom.Message{{ID: "user", SessionID: session.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Follow the database and writing instructions."}}}}}
	settings := memorystore.New().Settings()
	testutil.RequireNoError(test, settings.Save(context.Background(), session.InstanceID, settingsKey, `{"enabled":true,"worker_model":"fixture/worker","interval":{"mode":"tokens","tokens":32768}}`))
	operationContext, cancel := context.WithCancel(context.Background())
	plugin := &Plugin{services: harness.PluginServices{Settings: settings, Conversations: services, Workspaces: services, Models: services, Prompts: services, Memory: services, Usage: services}, database: database, promptVersion: "templates-v1", operationContext: operationContext, cancel: cancel, scopes: map[string]*workspaceRuntime{}}
	test.Cleanup(func() { testutil.RequireNoError(test, plugin.Close(context.Background())) })
	return &pluginFixture{plugin: plugin, services: services, database: database, session: session, settings: settings}
}
func (fixture *pluginFixture) prepare(test *testing.T, tokens int, messages []atom.Message) (context.Context, func(), harness.ReminderContribution) {
	test.Helper()
	operationContext, release, operationError := fixture.plugin.BeginRequest(context.Background(), fixture.session)
	testutil.RequireNoError(test, operationError)
	request := atom.Request{Model: fixture.session.Model, Messages: messages}
	contribution, operationError := fixture.plugin.PrepareReminder(operationContext, harness.ContextRequest{Session: fixture.session, Request: request, ModelID: fixture.session.Model, Budget: harness.RequestBudget{ContextLimit: 262144, InputLimit: 261120, InputTokens: tokens, Estimated: true}})
	testutil.RequireNoError(test, operationError)
	return operationContext, release, contribution
}
