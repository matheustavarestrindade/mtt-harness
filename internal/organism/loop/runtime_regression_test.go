package loop_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestExplicitDenyWinsOverStoredApproval(test *testing.T) {
	testStack := newStack(test, provider.Call("guarded", `{}`), provider.Text("done"))
	session := testStack.instance(test, 2)
	var calls int64
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "guarded", counter: &calls}))
	testutil.RequireNoError(test, testStack.database.Permissions().Save(context.Background(), atom.PermissionDecision{RequestID: "old", InstanceID: session.InstanceID, SessionID: session.ID, Target: "guarded", Kind: atom.VerdictAllow, Scope: atom.ScopeAlways, CreatedAt: time.Now()}))
	harness.Decide(testStack.harnessRuntime, atom.StageToolInput, func(context.Context, atom.ToolCall) (atom.Verdict, error) {
		return atom.Verdict{Kind: atom.VerdictDeny}, nil
	})
	testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))
	if atomic.LoadInt64(&calls) != 0 {
		test.Fatal("a cached approval overrode deny")
	}
}

func TestOneTimeApprovalCannotCrossInstances(test *testing.T) {
	testStack := newStack(test, provider.Call("guarded", `{}`), provider.Text("done"), provider.Call("guarded", `{}`), provider.Text("done"))
	first := testStack.instance(test, 2)
	if _, operationError := testStack.instances.Start(context.Background(), atom.InstanceSpec{ID: "other", Workspace: test.TempDir(), DefaultModel: "test-model"}); operationError != nil {
		test.Fatal(operationError)
	}
	second := atom.Session{ID: "second", InstanceID: "other", Model: "test-model"}
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "guarded"}))
	var prompts atomic.Int32
	testStack.broker.SetRequestHandler(func(request atom.PermissionRequest) {
		prompts.Add(1)
		testStack.broker.Resolve(request.ID, atom.PermissionDecision{Kind: atom.VerdictAllow, Scope: atom.ScopeOnce})
	})
	harness.Decide(testStack.harnessRuntime, atom.StageToolInput, func(context.Context, atom.ToolCall) (atom.Verdict, error) {
		return atom.Verdict{Kind: atom.VerdictAsk}, nil
	})
	testutil.RequireNoError(test, testStack.loop.Run(context.Background(), first))
	testutil.RequireNoError(test, testStack.loop.Run(context.Background(), second))
	if prompts.Load() != 2 {
		test.Fatalf("one-time approval reused: prompts=%d", prompts.Load())
	}
}

func TestPluginRegistrationsReachExecutionAndEvents(test *testing.T) {
	testStack := newStack(test)
	session := testStack.instance(test, 2)
	dynamic := provider.NewTest("dynamic", provider.Call("plugin_tool", `{}`), provider.Text("done"))
	testStack.harnessRuntime.Provider(dynamic)
	session.Model = "dynamic-model"
	var calls int64
	var events atomic.Int32
	testStack.harnessRuntime.Tool(&fakeTool{name: "plugin_tool", counter: &calls})
	testStack.harnessRuntime.On(atom.EventModelCall, func(context.Context, atom.Event) {
		events.Add(1)
	})
	testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))
	if calls != 1 || events.Load() != 2 {
		test.Fatalf("registrations disconnected: tools=%d events=%d", calls, events.Load())
	}
}

type gatedProvider struct {
	started chan struct{}
	calls   atomic.Int32
}

func (provider *gatedProvider) Name() string {
	return "gated"
}
func (provider *gatedProvider) Models() []atom.ModelInfo {
	return []atom.ModelInfo{{ID: "gated-model", Input: []atom.MediaType{atom.Text}, Tools: true}}
}
func (provider *gatedProvider) Stream(context.Context, atom.Request) (harness.Stream, error) {
	return &gatedStream{started: provider.started, first: provider.calls.Add(1) == 1}, nil
}

type gatedStream struct {
	started chan struct{}
	first   bool
	step    int
}

func (stream *gatedStream) Recv(operationContext context.Context) (atom.ResponsePart, error) {
	stream.step++
	if !stream.first {
		if stream.step == 1 {
			return atom.ResponsePart{Text: "done"}, nil
		}
		return atom.ResponsePart{}, io.EOF
	}
	if stream.step == 1 {
		return atom.ResponsePart{ToolCall: &atom.ToolCall{ID: "early", Name: "early", Input: []byte(`{}`)}}, nil
	}
	select {
	case <-stream.started:
		return atom.ResponsePart{}, io.EOF
	case <-operationContext.Done():
		return atom.ResponsePart{}, operationContext.Err()
	case <-time.After(time.Second):
		return atom.ResponsePart{}, fmt.Errorf("tool did not start before stream EOF")
	}
}

func TestCompleteToolStartsBeforeModelEOF(test *testing.T) {
	testStack := newStack(test)
	session := testStack.instance(test, 2)
	gated := &gatedProvider{started: make(chan struct{})}
	testStack.harnessRuntime.Provider(gated)
	session.Model = "gated-model"
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "early", run: func(context.Context, atom.ToolCall) (atom.ToolResult, error) {
		close(gated.started)
		return atom.ToolResult{Status: atom.StatusOK}, nil
	}}))
	testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))
}

func TestRevertWaitsForOldWriterAndFencesNewSubmissions(test *testing.T) {
	testStack := newStack(test, provider.Call("blocked", `{}`), provider.Text("new answer"))
	session := testStack.instance(test, 2)
	queue := newTestQueue(test, testStack.loop)
	entered, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	releaseTool := sync.OnceFunc(func() {
		close(release)
	})
	defer func() {
		releaseTool()
		shutdownContext, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		testutil.RequireNoError(test, queue.Close(shutdownContext))
	}()
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "blocked", run: func(operationContext context.Context, _ atom.ToolCall) (atom.ToolResult, error) {
		close(entered)
		<-operationContext.Done()
		close(cancelled)
		<-release
		return atom.ToolResult{Status: atom.StatusOK}, nil
	}}))
	message, _, operationError := queue.Submit(context.Background(), session, "keep")
	testutil.RequireNoError(test, operationError)
	select {
	case <-entered:
	case <-time.After(time.Second):
		test.Fatal("tool did not start")
	}
	reverted := make(chan error, 1)
	go func() {
		_, operationError := queue.Revert(context.Background(), session, message.ID)
		reverted <- operationError
	}()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		test.Fatal("revert did not cancel the old writer")
	}
	if _, _, operationError := queue.Submit(context.Background(), session, "race"); !errors.Is(operationError, loop.ErrSessionBusy) {
		test.Fatalf("submission crossed revert barrier: %v", operationError)
	}
	select {
	case operationError := <-reverted:
		test.Fatalf("revert returned before the writer stopped: %v", operationError)
	default:
	}
	releaseTool()
	testutil.RequireNoError(test, <-reverted)
	messages, operationError := testStack.database.Sessions().Messages(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	if len(messages) != 1 || messages[0].ID != message.ID {
		test.Fatalf("late messages after revert: %+v", messages)
	}
}

func TestPendingMessagesSurviveQueueShutdown(test *testing.T) {
	testStack := newStack(test, provider.Text("old"), provider.Text("restored"))
	testStack.provider.SetDelay(time.Second)
	session := testStack.instance(test, 2)
	queue := newTestQueue(test, testStack.loop)
	_, _, operationError := queue.Submit(context.Background(), session, "first")
	testutil.RequireNoError(test, operationError)
	deadline := time.Now().Add(time.Second)
	for testStack.provider.Calls() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	second, _, operationError := queue.Submit(context.Background(), session, "second")
	testutil.RequireNoError(test, operationError)
	shutdownContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	testutil.RequireNoError(test, queue.Close(shutdownContext))
	entries, operationError := testStack.database.Queue().All(context.Background())
	testutil.RequireNoError(test, operationError)
	if len(entries) != 1 || entries[0].Message.ID != second.ID || entries[0].Running {
		test.Fatalf("pending request lost: %+v", entries)
	}
	testStack.provider.SetDelay(0)
	restored := newTestQueue(test, testStack.loop)
	testutil.RequireNoError(test, restored.Restore(context.Background()))
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		entries, _ = testStack.database.Queue().All(context.Background())
		if len(entries) == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	testutil.RequireNoError(test, restored.Close(context.Background()))
	messages, _ := testStack.database.Sessions().Messages(context.Background(), session.ID)
	count := 0
	for _, message := range messages {
		if message.ID == second.ID {
			count++
		}
	}
	if count != 1 {
		test.Fatalf("pending message ran %d times", count)
	}
}

func TestRecoveryDoesNotReplayInterruptedTools(test *testing.T) {
	testStack := newStack(test, provider.Text("remaining message"))
	session := testStack.instance(test, 2)
	operationContext := context.Background()
	interrupted := atom.Message{ID: "interrupted", SessionID: session.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "old"}}, CreatedAt: time.Now()}
	pending := atom.Message{ID: "pending", SessionID: session.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "new"}}, CreatedAt: time.Now()}
	testutil.RequireNoError(test, testStack.database.Queue().Enqueue(operationContext, interrupted, 128))
	testutil.RequireNoError(test, testStack.database.Queue().Start(operationContext, interrupted.ID))
	testutil.RequireNoError(test, testStack.database.Queue().Enqueue(operationContext, pending, 128))
	queue := newTestQueue(test, testStack.loop)
	testStack.harnessRuntime.On(atom.EventName("run.interrupted"), func(context.Context, atom.Event) {
		queue.Status(operationContext, session.ID)
	})
	testutil.RequireNoError(test, queue.Restore(operationContext))
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		entries, _ := testStack.database.Queue().All(operationContext)
		if len(entries) == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	testutil.RequireNoError(test, queue.Close(operationContext))
	if testStack.provider.Calls() != 1 {
		test.Fatalf("interrupted request replayed: calls=%d", testStack.provider.Calls())
	}
	messages, _ := testStack.database.Sessions().Messages(operationContext, session.ID)
	if len(messages) != 3 || messages[0].ID != interrupted.ID || messages[1].ID != pending.ID {
		test.Fatalf("recovery history: %+v", messages)
	}
}
