package loop_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/permission"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/toolsearch"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/registry"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
	"github.com/matheustavarestrindade/mtt-harness/internal/tools"
)

type fakeTool struct {
	name        string
	run         func(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error)
	check       func(operationContext context.Context, call atom.ToolCall) atom.Verdict
	counter     *int64
	delay       time.Duration
	inputSchema string
}

func (fakeTool *fakeTool) Name() string {
	return fakeTool.name
}
func (fakeTool *fakeTool) Description() string {
	return "A test tool"
}
func (fakeTool *fakeTool) Categories() []string {
	return []string{"test"}
}
func (fakeTool *fakeTool) InputSchema() atom.Schema {
	if fakeTool.inputSchema != "" {
		return atom.Schema{JSON: []byte(fakeTool.inputSchema)}
	}
	return atom.Schema{JSON: []byte(`{"type":"object"}`)}
}
func (fakeTool *fakeTool) Check(operationContext context.Context, call atom.ToolCall) atom.Verdict {
	if fakeTool.check != nil {
		return fakeTool.check(operationContext, call)
	}
	return atom.Verdict{Kind: atom.VerdictAllow}
}
func (fakeTool *fakeTool) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	if fakeTool.counter != nil {
		atomic.AddInt64(fakeTool.counter, 1)
	}
	if fakeTool.delay > 0 {
		time.Sleep(fakeTool.delay)
	}
	if fakeTool.run != nil {
		return fakeTool.run(operationContext, call)
	}
	return atom.ToolResult{Status: atom.StatusOK, Content: []atom.Content{{Type: atom.Text, Text: "the tool ran"}}}, nil
}

type stack struct {
	harnessRuntime *harness.Harness
	database       *memory.Store
	registry       *registry.Registry
	loop           *loop.Loop
	provider       *provider.Test
	bus            *eventbus.Bus
	broker         *permission.Broker
	instances      *instances.Manager
}

func newStack(test *testing.T, script ...[]atom.ResponsePart) *stack {
	test.Helper()
	database := memory.New()
	harnessRuntime := harness.New()
	bus := eventbus.New(harnessRuntime)
	searchIndex := toolsearch.New(testutil.EmbedderFunc(func(operationContext context.Context, texts []string) ([][]float64, error) {
		vectors := make([][]float64, len(texts))
		for position, text := range texts {
			vectors[position] = []float64{0, 1}
			if text == "shell command exec" || strings.HasPrefix(text, "Tool: bash\n") {
				vectors[position] = []float64{1, 0}
			}
		}
		return vectors, nil
	}), toolsearch.Options{MinimumSimilarity: 0.5})
	toolRegistry := registry.New(harnessRuntime, searchIndex)
	engine := permission.NewEngine()
	broker := permission.NewBroker()
	models := gateway.New(harnessRuntime)
	testProvider := provider.NewTest("test", script...)
	testutil.RequireNoError(test, models.Add(testProvider))

	instanceManager := instances.New(func(instanceID string) instances.SessionManager {
		return nil
	}, nil)
	runner := loop.New(harnessRuntime, loop.Config{
		Gateway:   models,
		Registry:  toolRegistry,
		Store:     database,
		Bus:       bus,
		Broker:    broker,
		Engine:    engine,
		Instances: instanceManager,
	})
	return &stack{
		harnessRuntime: harnessRuntime,
		database:       database,
		registry:       toolRegistry,
		loop:           runner,
		provider:       testProvider,
		bus:            bus,
		broker:         broker,
		instances:      instanceManager,
	}
}

func (testStack *stack) instance(test *testing.T, depthLimit int) atom.Session {
	test.Helper()
	instanceSpec := atom.InstanceSpec{ID: "instance-1", Workspace: test.TempDir(), DefaultModel: "test-model", AgentDepthLimit: depthLimit}
	if _, operationError := testStack.instances.Start(context.Background(), instanceSpec); operationError != nil {
		test.Fatal(operationError)
	}
	session := atom.Session{ID: "session-1", InstanceID: instanceSpec.ID, Model: "test-model", CreatedAt: time.Now()}
	testutil.RequireNoError(test, testStack.database.Sessions().Save(context.Background(), session))

	return session
}

func (testStack *stack) user(test *testing.T, session atom.Session, text string) {
	test.Helper()
	message := atom.Message{ID: "user-1", SessionID: session.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: text}}, CreatedAt: time.Now()}
	testutil.RequireNoError(test, testStack.database.Sessions().Append(context.Background(), message))
}

func TestLoopRunsToolCall(test *testing.T) {
	usage := atom.Usage{Input: 1_000_000, Output: 10}
	testStack := newStack(test,
		provider.CallWithUsage("fake", `{"value":"x"}`, usage),
		provider.TextWithUsage("the answer", usage),
	)
	ran := int64(0)
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "fake", counter: &ran}))

	session := testStack.instance(test, 2)
	testStack.user(test, session, "do the task")
	testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))

	if atomic.LoadInt64(&ran) != 1 {
		test.Fatalf("tool runs = %d", ran)
	}
	messages, _ := testStack.database.Sessions().Messages(context.Background(), session.ID)
	if len(messages) != 4 {
		test.Fatalf("messages = %d", len(messages))
	}
	if messages[3].Role != atom.RoleAssistant || messages[3].Content[0].Text != "the answer" {
		test.Fatalf("last message = %+v", messages[3])
	}
	statistics, _ := testStack.database.Usage().Session(context.Background(), session.ID)
	if statistics.Calls != 2 || statistics.Input != 2_000_000 {
		test.Fatalf("statistics = %+v", statistics)
	}
	if statistics.Cost == nil || statistics.Cost.Value != 2.00004 {
		test.Fatalf("cost = %+v", statistics.Cost)
	}
}

func TestLoopRunsToolCallsAtTheSameTime(test *testing.T) {
	testStack := newStack(test,
		[]atom.ResponsePart{
			{ToolCall: &atom.ToolCall{ID: "call_1", Name: "slow", Input: []byte(`{}`)}},
			{ToolCall: &atom.ToolCall{ID: "call_2", Name: "slow", Input: []byte(`{}`)}},
		},
		provider.Text("done"),
	)
	var current, maximum int64
	tool := &fakeTool{name: "slow", delay: 150 * time.Millisecond}
	tool.run = func(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
		value := atomic.AddInt64(&current, 1)
		for {
			old := atomic.LoadInt64(&maximum)
			if value <= old || atomic.CompareAndSwapInt64(&maximum, old, value) {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
		atomic.AddInt64(&current, -1)
		return atom.ToolResult{Status: atom.StatusOK, Content: []atom.Content{{Type: atom.Text, Text: call.ID}}}, nil
	}
	testutil.RequireNoError(test, testStack.registry.Add(tool))

	session := testStack.instance(test, 2)
	testStack.user(test, session, "run 2 tools")
	testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))

	if atomic.LoadInt64(&maximum) < 2 {
		test.Fatalf("the tool calls did not run at the same time: maximum = %d", maximum)
	}
}

func TestLoopAsksForPermission(test *testing.T) {
	testStack := newStack(test,
		provider.Call("danger", `{}`),
		provider.Text("done"),
	)
	harness.Decide(testStack.harnessRuntime, atom.StageToolInput, func(operationContext context.Context, call atom.ToolCall) (atom.Verdict, error) {
		if call.Name == "danger" {
			return atom.Verdict{Kind: atom.VerdictAsk, Target: "danger", Why: "the tool is dangerous"}, nil
		}
		return atom.Verdict{Kind: atom.VerdictAllow}, nil
	})
	ran := int64(0)
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "danger", counter: &ran}))

	session := testStack.instance(test, 2)
	testStack.user(test, session, "do the dangerous task")

	done := make(chan error, 1)
	go func() {
		done <- testStack.loop.Run(context.Background(), session)
	}()

	request := make(chan atom.PermissionRequest, 1)
	testStack.bus.On(atom.EventPermissionRequest, func(operationContext context.Context, event atom.Event) {
		var parsed atom.PermissionRequest
		if operationError := jsonUnmarshal(event.Payload, &parsed); operationError == nil {
			request <- parsed
		}
	})
	select {
	case parsed := <-request:
		if !testStack.broker.Resolve(parsed.ID, atom.PermissionDecision{Kind: atom.VerdictDeny, Scope: atom.ScopeOnce}) {
			test.Fatal("the permission request is not open")
		}
	case <-time.After(2 * time.Second):
		test.Fatal("the permission request did not arrive")
	}
	testutil.RequireNoError(test, <-done)

	if atomic.LoadInt64(&ran) != 0 {
		test.Fatal("the denied tool ran")
	}
	messages, _ := testStack.database.Sessions().Messages(context.Background(), session.ID)
	denied := false
	for _, message := range messages {
		if message.Role == atom.RoleTool && strings.Contains(message.Content[0].Text, "denied") {
			denied = true
		}
	}
	if !denied {
		test.Fatalf("the result is not denied: %+v", messages)
	}
}

func TestLoopAddsFoundTools(test *testing.T) {
	testStack := newStack(test,
		provider.Call("search_tool", `{"query":"fake"}`),
		provider.Call("fake", `{}`),
		provider.Text("done"),
	)
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "fake"}))
	testutil.RequireNoError(test, testStack.registry.Add(tools.NewSearch(testStack.registry)))

	session := testStack.instance(test, 2)
	testStack.user(test, session, "find a tool")
	testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))

	requests := testStack.provider.Requests
	if len(requests) < 2 {
		test.Fatalf("requests = %d", len(requests))
	}
	found := false
	for _, toolSpec := range requests[1].Tools {
		if toolSpec.Name == "fake" {
			found = true
		}
	}
	if !found {
		test.Fatalf("the found tool is not in the next request: %+v", requests[1].Tools)
	}
}

type agentTool struct {
	runner *loop.Loop
}

func (agentTool *agentTool) Name() string {
	return "agent"
}
func (agentTool *agentTool) Description() string {
	return "Start an agent"
}
func (agentTool *agentTool) Categories() []string {
	return []string{"agent"}
}
func (agentTool *agentTool) InputSchema() atom.Schema {
	return atom.Schema{JSON: []byte(`{"type":"object"}`)}
}
func (agentTool *agentTool) Check(operationContext context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}
func (agentTool *agentTool) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	var input struct {
		Task string `json:"task"`
	}
	_ = jsonUnmarshal(call.Input, &input)
	return agentTool.runner.RunAgentTask(operationContext, atom.AgentTask{Task: input.Task})
}

func TestLoopRefusesUnsupportedMedia(test *testing.T) {
	testStack := newStack(test, provider.Text("never"))
	session := testStack.instance(test, 2)
	message := atom.Message{
		ID:        "user-image",
		SessionID: session.ID,
		Role:      atom.RoleUser,
		Content:   []atom.Content{{Type: atom.Image, Data: []byte("x"), MIME: "image/png"}},
		CreatedAt: time.Now(),
	}
	testutil.RequireNoError(test, testStack.database.Sessions().Append(context.Background(), message))

	operationError := testStack.loop.Run(context.Background(), session)
	if operationError == nil || !strings.Contains(operationError.Error(), "media type") {
		test.Fatalf("the media type check is absent: %v", operationError)
	}
}

func TestLoopValidatesTheToolInput(test *testing.T) {
	testStack := newStack(test,
		provider.Call("strict", `{}`),
		provider.Text("done"),
	)
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{
		name:        "strict",
		inputSchema: `{"type":"object","properties":{"value":{"type":"string"}},"required":["value"]}`,
	}))

	session := testStack.instance(test, 2)
	testStack.user(test, session, "run the strict tool")
	testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))

	messages, _ := testStack.database.Sessions().Messages(context.Background(), session.ID)
	found := false
	for _, message := range messages {
		if message.Role == atom.RoleTool && strings.Contains(message.Content[0].Text, "input of strict is not correct") && strings.Contains(message.Content[0].Text, "value") {
			found = true
		}
	}
	if !found {
		test.Fatalf("the input validation is absent: %+v", messages)
	}
}
