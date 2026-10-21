package watcher

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/permission"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	memorystore "github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/toolsearch"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/plugins"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/registry"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
	"github.com/matheustavarestrindade/mtt-harness/internal/tools"
)

type integrationStack struct {
	plugin   *Plugin
	database *memorystore.Store
	queue    *loop.Queue
	runtime  *harness.Harness
	provider *provider.Test
	session  atom.Session
	loop     *loop.Loop
}

func newIntegration(test *testing.T, script ...[]atom.ResponsePart) *integrationStack {
	test.Helper()
	databaseURL := os.Getenv("MTT_TEST_DATABASE_URL")
	if databaseURL == "" {
		test.Skip("MTT_TEST_DATABASE_URL is not set")
	}
	operationContext, cancel := context.WithCancel(context.Background())
	database := memorystore.New()
	runtime := harness.New()
	events := eventbus.New(runtime)
	events.SetRecorder(database.Events().Record)
	catalog := registry.New(runtime, toolsearch.NewLexical(0))
	models := gateway.New(runtime)
	testProvider := provider.NewTest("test", script...)
	testutil.RequireNoError(test, models.Add(testProvider))
	manager := instances.New(func(string) instances.SessionManager { return nil }, database.Settings())
	workspace := atom.InstanceSpec{ID: newID(), Workspace: test.TempDir(), DefaultModel: "test/test-model", AgentDepthLimit: 3}
	_, operationError := manager.Start(operationContext, workspace)
	testutil.RequireNoError(test, operationError)
	testutil.RequireNoError(test, database.Instances().Save(operationContext, workspace))
	session := atom.Session{ID: atom.SessionID(newID()), InstanceID: workspace.ID, Model: workspace.DefaultModel, CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	host := plugins.New(runtime)
	agentLoop := loop.New(runtime, loop.Config{Store: database, Gateway: models, Registry: catalog, Bus: events, Broker: permission.NewBroker(), Engine: permission.NewEngine(), Instances: manager, Plugins: host})
	queue := loop.NewQueue(agentLoop)
	for _, tool := range []harness.Tool{tools.NewSearch(catalog), tools.NewFileActions(false), tools.ReviewDecision{}, tools.ReviewDecision{Intervene: true}, tools.Finish{}} {
		testutil.RequireNoError(test, catalog.Add(tool))
	}
	services := &plugins.Services{Store: database, Gateway: models, Instances: manager}
	plugin, operationError := New(operationContext, Options{DatabaseURL: databaseURL, Services: harness.PluginServices{Settings: database.Settings(), Conversations: services, Workspaces: services, Models: services, Usage: database.Usage(), Tasks: services, Sessions: agentLoop.SessionWorkers(queue), Events: events}})
	testutil.RequireNoError(test, operationError)
	if plugin.unavailable != nil {
		test.Fatal(plugin.unavailable)
	}
	testutil.RequireNoError(test, host.Attach(plugin))
	test.Cleanup(func() {
		cancel()
		testutil.RequireNoError(test, queue.Close(context.Background()))
		testutil.RequireNoError(test, host.Close(context.Background()))
	})
	return &integrationStack{plugin: plugin, database: database, queue: queue, runtime: runtime, provider: testProvider, session: session, loop: agentLoop}
}
func enableDefinition(test *testing.T, stack *integrationStack, triggers ...string) {
	test.Helper()
	value := definition{ID: "review", Name: "Review", Enabled: true, Model: "test/test-model", Prompt: "Review {todo} {doing} {event} {permission_request} {tool_call} <conversation_to_analize>{last_512_context}</conversation_to_analize>", Triggers: triggers, Tools: []atom.ToolGrant{{Name: "file_actions", Operations: []string{"read", "list", "glob"}}}, ContextTokens: 512, IntervalTokens: 256, MaxContinuations: 2}
	applyDefaults(&value)
	data, _ := json.Marshal(configuration{Enabled: true, Watchers: []definition{value}})
	_, operationError := stack.plugin.UpdateConfiguration(context.Background(), stack.session.InstanceID, data)
	testutil.RequireNoError(test, operationError)
}
func waitRuns(test *testing.T, stack *integrationStack, count int) []runRecord {
	test.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		page, operationError := stack.plugin.database.Runs(context.Background(), stack.session.InstanceID, string(stack.session.ID), "", 100)
		testutil.RequireNoError(test, operationError)
		terminal := len(page.Runs) >= count
		for _, run := range page.Runs {
			terminal = terminal && run.Status != "running" && run.Status != "reviewed"
		}
		if terminal {
			return page.Runs
		}
		time.Sleep(5 * time.Millisecond)
	}
	page, _ := stack.plugin.database.Runs(context.Background(), stack.session.InstanceID, string(stack.session.ID), "", 100)
	state, _ := stack.plugin.ReadState(context.Background(), stack.session.InstanceID)
	test.Fatalf("watcher runs did not settle: %+v state=%+v", page.Runs, state)
	return nil
}

func TestEndTurnInterferenceStartsOneRuntimeTurnWithoutRecursiveWatchers(test *testing.T) {
	stack := newIntegration(test, provider.Text("Initial answer"), provider.Call("interfere", `{"message":"Complete the verification."}`), provider.Text("Verification complete"), provider.Call("continue", `{"reason":"Done"}`))
	enableDefinition(test, stack, "end_turn")
	_, _, operationError := stack.queue.Submit(context.Background(), stack.session, "finish the task")
	testutil.RequireNoError(test, operationError)
	runs := waitRuns(test, stack, 2)
	if len(runs) != 2 || runs[0].Status != "completed" || runs[1].Status != "intervened" {
		test.Fatalf("decisions: %+v", runs)
	}
	messages, operationError := stack.database.Sessions().Messages(context.Background(), stack.session.ID)
	testutil.RequireNoError(test, operationError)
	users, runtimeInputs, answers := 0, 0, 0
	for _, message := range messages {
		if message.Role == atom.RoleUser {
			users++
		}
		if message.Role == atom.RoleRuntime {
			runtimeInputs++
			if !strings.Contains(message.Content[0].Text, "Complete the verification") {
				test.Fatal("intervention missing")
			}
		}
		if message.Role == atom.RoleAssistant {
			answers++
		}
	}
	if users != 1 || runtimeInputs != 1 || answers != 2 || stack.provider.Calls() != 4 {
		test.Fatalf("execution duplicated: users=%d runtime=%d answers=%d calls=%d", users, runtimeInputs, answers, stack.provider.Calls())
	}
	statistics, operationError := stack.database.Usage().Session(context.Background(), stack.session.ID)
	testutil.RequireNoError(test, operationError)
	if statistics.Calls != 2 {
		test.Fatalf("parent included watcher usage: %+v", statistics)
	}
	activity, operationError := stack.plugin.SessionActivity(context.Background(), stack.session, harness.PluginActivityQuery{Limit: 50})
	testutil.RequireNoError(test, operationError)
	if len(activity.Entries) != 2 || len(activity.Entries[1].Notes) != 1 {
		test.Fatal("read-only activity did not preserve decisions")
	}
}

func TestPermissionWatcherApprovesPendingCallAndOwnReadsNeverAsk(test *testing.T) {
	path := filepath.Join(test.TempDir(), "outside.txt")
	testutil.RequireNoError(test, os.WriteFile(path, []byte("outside evidence"), 0600))
	input, _ := json.Marshal(map[string]any{"path": path, "actions": []map[string]string{{"op": "read"}}})
	stack := newIntegration(test, provider.Call("file_actions", string(input)), provider.Call("file_actions", string(input)), provider.Call("continue", `{"reason":"Task needs this read"}`), provider.Text("Read complete"))
	enableDefinition(test, stack, "permission_request")
	var asks atomic.Int64
	harness.Decide(stack.runtime, atom.StageToolInput, func(context.Context, atom.ToolCall) (atom.Verdict, error) {
		asks.Add(1)
		return atom.Verdict{Kind: atom.VerdictAsk, Target: path, Why: "outside workspace"}, nil
	})
	_, _, operationError := stack.queue.Submit(context.Background(), stack.session, "read external reference")
	testutil.RequireNoError(test, operationError)
	runs := waitRuns(test, stack, 1)
	if runs[0].Status != "completed" || asks.Load() != 1 {
		test.Fatalf("permission flow: %+v asks=%d", runs, asks.Load())
	}
	deadline := time.Now().Add(time.Second)
	for stack.provider.Calls() < 4 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if stack.provider.Calls() != 4 {
		test.Fatal("main tool did not resume")
	}
	requests := stack.provider.Requests
	if !strings.Contains(requests[2].Messages[len(requests[2].Messages)-1].Content[0].Text, "outside evidence") {
		test.Fatal("watcher could not read outside without permission")
	}
}

func TestContextGrowthSchedulesAtCommittedModelRequest(test *testing.T) {
	stack := newIntegration(test, provider.Text("Response"), provider.Call("continue", `{}`))
	enableDefinition(test, stack, "context_growth")
	_, _, operationError := stack.queue.Submit(context.Background(), stack.session, strings.Repeat("context source ", 100))
	testutil.RequireNoError(test, operationError)
	runs := waitRuns(test, stack, 1)
	if runs[0].Trigger != "context_growth" || runs[0].Status != "completed" {
		test.Fatalf("context event: %+v", runs)
	}
	if !strings.HasPrefix(runs[0].InputPreview, "<watcher_event>") || !strings.Contains(runs[0].InputPreview, `"source_phase":"active_request"`) {
		test.Fatal("custom watcher prompt omitted required event metadata")
	}
	if !strings.Contains(runs[0].InputPreview, `"context_basis":"visible_input"`) || !strings.Contains(runs[0].InputPreview, `"context_estimated":true`) {
		test.Fatal("watcher input did not identify its visible-context estimate")
	}
}

func TestDisablingWatchersCancelsTheReviewWithoutRestartingMain(test *testing.T) {
	stack := newIntegration(test, provider.Text("Done"), provider.Call("interfere", `{"message":"obsolete"}`))
	enableDefinition(test, stack, "end_turn")
	started := make(chan struct{}, 1)
	stack.runtime.On(atom.EventModelCall, func(operationContext context.Context, _ atom.Event) {
		session, _ := harness.SessionFrom(operationContext)
		if session.IsWatcher() {
			select {
			case started <- struct{}{}:
			default:
			}
			<-operationContext.Done()
		}
	})
	_, _, operationError := stack.queue.Submit(context.Background(), stack.session, "task")
	testutil.RequireNoError(test, operationError)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		test.Fatal("watcher did not start")
	}
	_, operationError = stack.plugin.UpdateConfiguration(context.Background(), stack.session.InstanceID, json.RawMessage(`{"enabled":false}`))
	testutil.RequireNoError(test, operationError)
	runs := waitRuns(test, stack, 1)
	if runs[0].Status != "cancelled" {
		test.Fatalf("disabled run: %+v", runs[0])
	}
	messages, operationError := stack.database.Sessions().Messages(context.Background(), stack.session.ID)
	testutil.RequireNoError(test, operationError)
	for _, message := range messages {
		if message.Role == atom.RoleRuntime {
			test.Fatal("disabled review injected a continuation")
		}
	}
}

func TestContinuationLimitAppliesAcrossNewMainTurns(test *testing.T) {
	stack := newIntegration(test, provider.Text("First"), provider.Call("interfere", `{"message":"Check once more"}`), provider.Text("Second"))
	enableDefinition(test, stack, "end_turn")
	configuration, _, operationError := stack.plugin.loadConfiguration(context.Background(), stack.session.InstanceID)
	testutil.RequireNoError(test, operationError)
	configuration.Watchers[0].MaxContinuations = 1
	data, _ := json.Marshal(configuration)
	_, operationError = stack.plugin.UpdateConfiguration(context.Background(), stack.session.InstanceID, data)
	testutil.RequireNoError(test, operationError)
	_, _, operationError = stack.queue.Submit(context.Background(), stack.session, "task")
	testutil.RequireNoError(test, operationError)
	runs := waitRuns(test, stack, 2)
	if runs[0].Status != "limited" || runs[1].Status != "intervened" || stack.provider.Calls() != 3 {
		test.Fatalf("continuation budget: %+v calls=%d", runs, stack.provider.Calls())
	}
}

func TestMultiplePermissionReviewersRequireAgreement(test *testing.T) {
	path := filepath.Join(test.TempDir(), "reference")
	testutil.RequireNoError(test, os.WriteFile(path, []byte("reference"), 0600))
	input, _ := json.Marshal(map[string]any{"path": path, "actions": []map[string]string{{"op": "read"}}})
	stack := newIntegration(test, provider.Call("file_actions", string(input)), provider.Call("continue", `{}`), provider.Call("interfere", `{"message":"Use the workspace reference instead."}`), provider.Text("Used the workspace"))
	enableDefinition(test, stack, "permission_request")
	configuration, _, operationError := stack.plugin.loadConfiguration(context.Background(), stack.session.InstanceID)
	testutil.RequireNoError(test, operationError)
	second := configuration.Watchers[0]
	second.ID = "strict"
	second.Name = "Strict review"
	configuration.Watchers = append(configuration.Watchers, second)
	data, _ := json.Marshal(configuration)
	_, operationError = stack.plugin.UpdateConfiguration(context.Background(), stack.session.InstanceID, data)
	testutil.RequireNoError(test, operationError)
	harness.Decide(stack.runtime, atom.StageToolInput, func(context.Context, atom.ToolCall) (atom.Verdict, error) {
		return atom.Verdict{Kind: atom.VerdictAsk, Target: path}, nil
	})
	_, _, operationError = stack.queue.Submit(context.Background(), stack.session, "read a reference")
	testutil.RequireNoError(test, operationError)
	runs := waitRuns(test, stack, 2)
	for _, run := range runs {
		if run.Status != "intervened" {
			test.Fatalf("group outcome: %+v", runs)
		}
	}
	deadline := time.Now().Add(time.Second)
	for stack.provider.Calls() < 4 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	messages, operationError := stack.database.Sessions().Messages(context.Background(), stack.session.ID)
	testutil.RequireNoError(test, operationError)
	runtimeInputs := 0
	denied := false
	for _, message := range messages {
		if message.Role == atom.RoleRuntime {
			runtimeInputs++
		}
		if message.Role == atom.RoleTool && len(message.Content) > 0 && strings.Contains(message.Content[0].Text, "denied") {
			denied = true
		}
	}
	if runtimeInputs != 1 || !denied {
		test.Fatalf("permission group did not deny and coalesce: %+v", messages)
	}
}

func TestWatcherActivityPersistencePaginationAndHistoryRetirement(test *testing.T) {
	databaseURL := os.Getenv("MTT_TEST_DATABASE_URL")
	if databaseURL == "" {
		test.Skip("MTT_TEST_DATABASE_URL is not set")
	}
	database, operationError := openRepository(context.Background(), databaseURL)
	testutil.RequireNoError(test, operationError)
	workspace, session := newID(), newID()
	now := time.Now().UTC()
	for index, status := range []string{"completed", "running"} {
		testutil.RequireNoError(test, database.SaveRun(context.Background(), runRecord{ID: []string{"one", "two"}[index], WorkspaceID: workspace, SessionID: atom.SessionID(session), Name: "Review", Status: status, CreatedAt: now.Add(time.Duration(index) * time.Second), AnchorID: []string{"kept", "removed"}[index], Message: "decision evidence"}))
	}
	database.Close()
	database, operationError = openRepository(context.Background(), databaseURL)
	testutil.RequireNoError(test, operationError)
	defer database.Close()
	page, state, operationError := database.Activity(context.Background(), workspace, session, "", 1)
	testutil.RequireNoError(test, operationError)
	if len(page.Runs) != 1 || page.Runs[0].Status != "interrupted" || page.NextCursor == "" || state.Revision < 2 {
		test.Fatalf("recovery page: %+v %+v", page, state)
	}
	if _, operationError = database.Runs(context.Background(), "other", session, page.NextCursor, 1); operationError == nil {
		test.Fatal("cursor crossed workspace")
	}
	second, operationError := database.Runs(context.Background(), workspace, session, page.NextCursor, 1)
	testutil.RequireNoError(test, operationError)
	if len(second.Runs) != 1 || second.Runs[0].Message != "decision evidence" {
		test.Fatal("saved decision was lost")
	}
	previousCursor := page.NextCursor
	testutil.RequireNoError(test, database.RetireHistory(context.Background(), workspace, session, false, []string{"kept"}))
	if _, operationError = database.Runs(context.Background(), workspace, session, previousCursor, 1); !errors.Is(operationError, harness.ErrPluginActivityChanged) {
		test.Fatalf("old history cursor remained valid: %v", operationError)
	}
	page, operationError = database.Runs(context.Background(), workspace, session, "", 50)
	testutil.RequireNoError(test, operationError)
	if len(page.Runs) != 1 || page.Runs[0].AnchorID != "kept" {
		test.Fatal("revert retained an invalid anchor")
	}
	testutil.RequireNoError(test, database.RetireHistory(context.Background(), workspace, session, true, nil))
	if operationError = database.SaveRun(context.Background(), runRecord{ID: "late", WorkspaceID: workspace, SessionID: atom.SessionID(session), Status: "completed", CreatedAt: now}); operationError == nil {
		test.Fatal("deleted activity was resurrected")
	}
}
