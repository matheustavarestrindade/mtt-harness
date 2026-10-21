package loop_test

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
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
	"github.com/matheustavarestrindade/mtt-harness/internal/tools"
)

func registerReviewTools(test *testing.T, testStack *stack) {
	test.Helper()
	for _, tool := range []harness.Tool{tools.NewSearch(testStack.registry), tools.NewFileActions(false), tools.ReviewDecision{}, tools.ReviewDecision{Intervene: true}, tools.Finish{}} {
		testutil.RequireNoError(test, testStack.registry.Add(tool))
	}
}
func workerRequest(session atom.Session, grants []atom.ToolGrant) harness.WorkerSessionRequest {
	return harness.WorkerSessionRequest{WorkspaceID: session.InstanceID, SourceSessionID: session.ID, Agent: "watcher.test", RunID: "run-test", Model: "test/test-model", Prompt: "Review the supplied task, then decide.", Tools: grants, MaxRounds: 12, MaxOutputTokens: 1024}
}

func TestWatcherUsesRealLoopWithRestrictedSchemasAndNoPermissionPipeline(test *testing.T) {
	outside := filepath.Join(test.TempDir(), "outside.txt")
	testutil.RequireNoError(test, os.WriteFile(outside, []byte("outside evidence"), 0600))
	readInput, _ := json.Marshal(map[string]any{"path": outside, "actions": []map[string]string{{"op": "read"}}})
	writeInput, _ := json.Marshal(map[string]any{"path": outside, "actions": []map[string]string{{"op": "write", "content": "wrong"}, {"op": "read"}}, "return": map[string]string{"type": "summary"}})
	testStack := newStack(test, provider.Call("search_tool", `{"query":"file_actions"}`), provider.Call("file_actions", string(readInput)), provider.Call("file_actions", string(writeInput)), provider.Call("blocked", `{}`), provider.Call("search_memory", `{}`), provider.Call("continue", `{"reason":"Evidence checked"}`))
	session := testStack.instance(test, 3)
	testStack.user(test, session, "Review source task")
	registerReviewTools(test, testStack)
	var decisionChecks, toolChecks, blockedRuns int64
	harness.Decide(testStack.harnessRuntime, atom.StageToolInput, func(context.Context, atom.ToolCall) (atom.Verdict, error) {
		atomic.AddInt64(&decisionChecks, 1)
		return atom.Verdict{Kind: atom.VerdictDeny}, nil
	})
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "blocked", counter: &blockedRuns}))
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "search_memory", check: func(context.Context, atom.ToolCall) atom.Verdict {
		atomic.AddInt64(&toolChecks, 1)
		return atom.Verdict{Kind: atom.VerdictAsk}
	}, run: func(context.Context, atom.ToolCall) (atom.ToolResult, error) {
		return atom.ToolResult{Content: []atom.Content{{Type: atom.Text, Text: "workspace memory evidence"}}}, nil
	}}))
	queue := loop.NewQueue(testStack.loop)
	defer queue.Close(context.Background())
	result, operationError := testStack.loop.SessionWorkers(queue).RunSession(context.Background(), workerRequest(session, []atom.ToolGrant{{Name: "file_actions", Operations: []string{"read", "list", "glob"}}, {Name: "search_memory"}}))
	testutil.RequireNoError(test, operationError)
	if result.Decision != "continue" || result.Reason != "Evidence checked" || decisionChecks != 0 || toolChecks != 0 || blockedRuns != 0 {
		test.Fatalf("restricted execution: %+v checks=%d/%d blocked=%d", result, decisionChecks, toolChecks, blockedRuns)
	}
	data, operationError := os.ReadFile(outside)
	testutil.RequireNoError(test, operationError)
	if string(data) != "outside evidence" {
		test.Fatal("restricted mutation changed the file")
	}
	requests := testStack.provider.Requests
	if len(requests) != 6 {
		test.Fatalf("model rounds=%d", len(requests))
	}
	if len(requests[0].Tools) != 3 {
		test.Fatalf("bootstrap tools: %+v", requests[0].Tools)
	}
	for _, request := range requests {
		for _, tool := range request.Tools {
			if tool.Name == "blocked" || tool.Name == "finish" {
				test.Fatal("unavailable tool leaked")
			}
			if tool.Name == "file_actions" {
				var definition map[string]any
				testutil.RequireNoError(test, json.Unmarshal(tool.InputSchema.JSON, &definition))
				properties := definition["properties"].(map[string]any)
				operations := properties["actions"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["op"].(map[string]any)["enum"].([]any)
				if len(operations) != 3 {
					test.Fatalf("provider received unrestricted schema: %v", operations)
				}
				if !strings.Contains(properties["path"].(map[string]any)["description"].(string), "relative to the instance workspace") {
					test.Fatal("schema description was lost")
				}
			}
		}
	}
	if !strings.Contains(requests[2].Messages[len(requests[2].Messages)-1].Content[0].Text, "outside evidence") || !strings.Contains(requests[5].Messages[len(requests[5].Messages)-1].Content[0].Text, "workspace memory evidence") {
		test.Fatal("read tools did not execute in the real loop")
	}
	stored, operationError := testStack.database.Sessions().Get(context.Background(), result.SessionID)
	testutil.RequireNoError(test, operationError)
	encoded, _ := json.Marshal(stored)
	if !stored.IsWatcher() || strings.Contains(string(encoded), "system_prompt") || strings.Contains(string(encoded), "tools") {
		test.Fatal("private execution profile leaked")
	}
	statistics, operationError := testStack.database.Usage().Session(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	if statistics.Calls != 0 {
		test.Fatal("watcher usage entered parent chat totals")
	}
	agents, operationError := testStack.database.Usage().Agents(context.Background(), session.InstanceID)
	testutil.RequireNoError(test, operationError)
	if len(agents) != 1 || agents[0].Statistics.Calls != 6 {
		test.Fatalf("worker usage missing: %+v", agents)
	}
}

func TestWatcherPermissionResolutionAndCancellationFence(test *testing.T) {
	testStack := newStack(test, provider.Text("unused"))
	session := testStack.instance(test, 3)
	testStack.user(test, session, "source")
	queue := loop.NewQueue(testStack.loop)
	defer queue.Close(context.Background())
	services := testStack.loop.SessionWorkers(queue)
	source, operationError := services.Source(context.Background(), session.ID, 4096, false)
	testutil.RequireNoError(test, operationError)
	requestContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	decisions := make(chan atom.PermissionDecision, 1)
	go func() {
		decision, _ := testStack.broker.Request(requestContext, atom.PermissionRequest{ID: "permission-1", SessionID: session.ID, InstanceID: session.InstanceID, Target: "file_actions:/outside"}, func() error { close(ready); return nil })
		decisions <- decision
	}()
	<-ready
	applied, operationError := services.ResolvePermission(context.Background(), source, "permission-1", true, "watcher.test", "")
	testutil.RequireNoError(test, operationError)
	if !applied {
		test.Fatal("permission was not resolved")
	}
	decision := <-decisions
	if decision.Actor != "watcher.test" || decision.Scope != atom.ScopeOnce || decision.Kind != atom.VerdictAllow {
		test.Fatalf("incorrect approval: %+v", decision)
	}
	_, operationError = queue.Cancel(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	if current, operationError := services.Current(context.Background(), source); operationError != nil || current {
		test.Fatalf("idle cancellation did not fence review: %v %v", current, operationError)
	}
	if _, operationError = services.SubmitContinuation(context.Background(), source, "stale-hint", "old correction"); !errors.Is(operationError, harness.ErrReviewStale) {
		test.Fatalf("stale continuation: %v", operationError)
	}
	entries, operationError := testStack.database.Queue().All(context.Background())
	testutil.RequireNoError(test, operationError)
	if len(entries) != 0 {
		test.Fatal("stale review left queue data")
	}
}

func TestWatcherPermissionCorrectionDoesNotSurviveManualResolution(test *testing.T) {
	testStack := newStack(test, provider.Text("unused"))
	session := testStack.instance(test, 3)
	testStack.user(test, session, "source")
	queue := loop.NewQueue(testStack.loop)
	defer queue.Close(context.Background())
	services := testStack.loop.SessionWorkers(queue)
	source, operationError := services.Source(context.Background(), session.ID, 4096, false)
	testutil.RequireNoError(test, operationError)
	requestContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ready := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = testStack.broker.Request(requestContext, atom.PermissionRequest{ID: "permission-2", SessionID: session.ID, InstanceID: session.InstanceID}, func() error { close(ready); return nil })
	}()
	<-ready
	if !testStack.broker.Resolve("permission-2", atom.PermissionDecision{Kind: atom.VerdictDeny, Scope: atom.ScopeOnce}) {
		test.Fatal("manual decision failed")
	}
	<-done
	applied, operationError := services.ResolvePermission(context.Background(), source, "permission-2", false, "watcher.test", "outdated correction")
	testutil.RequireNoError(test, operationError)
	if applied {
		test.Fatal("obsolete decision was applied")
	}
	status, operationError := queue.Status(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	entries, operationError := testStack.database.Queue().All(context.Background())
	testutil.RequireNoError(test, operationError)
	if status.Running || len(entries) != 0 || testStack.provider.Calls() != 0 {
		test.Fatal("obsolete review started a turn")
	}
}

func TestWatcherDescendantsInheritCapabilitiesAndCompletionTool(test *testing.T) {
	testStack := newStack(test, provider.Call("agent", `{"task":"Inspect evidence","model":"test/test-model"}`), provider.Call("blocked", `{}`), provider.Call("finish", `{"result":"Evidence inspected"}`), provider.Call("continue", `{}`))
	session := testStack.instance(test, 4)
	registerReviewTools(test, testStack)
	testutil.RequireNoError(test, testStack.registry.Add(&tools.Agent{RunTask: testStack.loop.RunAgentTask}))
	var blocked int64
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "blocked", counter: &blocked}))
	queue := loop.NewQueue(testStack.loop)
	defer queue.Close(context.Background())
	result, operationError := testStack.loop.SessionWorkers(queue).RunSession(context.Background(), workerRequest(session, []atom.ToolGrant{{Name: "agent"}}))
	testutil.RequireNoError(test, operationError)
	if result.Decision != "continue" || blocked != 0 {
		test.Fatalf("descendant escaped: %+v", result)
	}
	sessions, operationError := testStack.database.Sessions().List(context.Background(), session.InstanceID)
	testutil.RequireNoError(test, operationError)
	watchers := 0
	for _, stored := range sessions {
		if stored.IsWatcher() {
			watchers++
			if stored.Execution.RunID != "run-test" || stored.Execution.SourceSessionID != session.ID {
				test.Fatal("watcher lineage was lost")
			}
		}
	}
	if watchers != 2 {
		test.Fatalf("tagged sessions=%d", watchers)
	}
	childRequest := testStack.provider.Requests[1]
	for _, tool := range childRequest.Tools {
		if tool.Name == "continue" || tool.Name == "interfere" || tool.Name == "blocked" {
			test.Fatalf("invalid child tool %s", tool.Name)
		}
	}
}

func TestCancelledWatcherRetainsReportedWorkspaceUsage(test *testing.T) {
	testStack := newStack(test, []atom.ResponsePart{{Text: "partial", Usage: &atom.Usage{Input: 17, CacheRead: 9, Output: 5}}, {Text: "more"}})
	testStack.provider.SetDelay(100 * time.Millisecond)
	session := testStack.instance(test, 3)
	registerReviewTools(test, testStack)
	queue := loop.NewQueue(testStack.loop)
	defer queue.Close(context.Background())
	started := make(chan struct{}, 1)
	operationContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	testStack.bus.On(atom.EventModelChunk, func(context.Context, atom.Event) {
		cancel()
		select {
		case started <- struct{}{}:
		default:
		}
	})
	finished := make(chan error, 1)
	go func() {
		_, operationError := testStack.loop.SessionWorkers(queue).RunSession(operationContext, workerRequest(session, nil))
		finished <- operationError
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		test.Fatal("worker did not stream")
	}
	if operationError := <-finished; !errors.Is(operationError, context.Canceled) {
		test.Fatalf("cancellation: %v", operationError)
	}
	agents, operationError := testStack.database.Usage().Agents(context.Background(), session.InstanceID)
	testutil.RequireNoError(test, operationError)
	if len(agents) != 1 || agents[0].Statistics.Input != 17 || agents[0].Statistics.CacheRead != 9 || agents[0].Statistics.Output != 5 || agents[0].Statistics.Calls != 1 {
		test.Fatalf("cancelled usage lost: %+v", agents)
	}
}
