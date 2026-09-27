package loop_test

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/permission"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/registry"
	"github.com/matheustavarestrindade/mtt-harness/internal/tools"
)

type fakeTool struct {
	name    string
	run     func(ctx context.Context, call atom.ToolCall) (atom.ToolResult, error)
	check   func(ctx context.Context, call atom.ToolCall) atom.Verdict
	counter *int64
	delay   time.Duration
}

func (f *fakeTool) Name() string             { return f.name }
func (f *fakeTool) Description() string      { return "A test tool" }
func (f *fakeTool) Categories() []string     { return []string{"test"} }
func (f *fakeTool) InputSchema() atom.Schema { return atom.Schema{JSON: []byte(`{"type":"object"}`)} }
func (f *fakeTool) Check(ctx context.Context, call atom.ToolCall) atom.Verdict {
	if f.check != nil {
		return f.check(ctx, call)
	}
	return atom.Verdict{Kind: atom.VerdictAllow}
}
func (f *fakeTool) Run(ctx context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	if f.counter != nil {
		atomic.AddInt64(f.counter, 1)
	}
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.run != nil {
		return f.run(ctx, call)
	}
	return atom.ToolResult{Status: atom.StatusOK, Content: []atom.Content{{Type: atom.Text, Text: "the tool ran"}}}, nil
}

type stack struct {
	h         *harness.Harness
	database  *memory.Store
	registry  *registry.Registry
	loop      *loop.Loop
	provider  *provider.Test
	bus       *eventbus.Bus
	broker    *permission.Broker
	instances *instances.Manager
}

func newStack(t *testing.T, script ...[]atom.ResponsePart) *stack {
	t.Helper()
	database := memory.New()
	bus := eventbus.New()
	h := harness.New()
	reg := registry.New()
	engine := permission.NewEngine()
	broker := permission.NewBroker(2 * time.Second)
	models := gateway.New()
	testProvider := provider.NewTest("test", script...)
	if err := models.Add(testProvider); err != nil {
		t.Fatal(err)
	}
	instanceManager := instances.New(func(instanceID string) instances.SessionManager {
		return nil
	})
	runner := loop.New(h, loop.Config{
		Gateway:   models,
		Registry:  reg,
		Store:     database,
		Bus:       bus,
		Broker:    broker,
		Engine:    engine,
		Instances: instanceManager,
	})
	return &stack{
		h:         h,
		database:  database,
		registry:  reg,
		loop:      runner,
		provider:  testProvider,
		bus:       bus,
		broker:    broker,
		instances: instanceManager,
	}
}

func (s *stack) instance(t *testing.T, depthLimit int) atom.Session {
	t.Helper()
	spec := atom.InstanceSpec{ID: "instance-1", Workspace: t.TempDir(), DefaultModel: "test-model", AgentDepthLimit: depthLimit}
	if _, err := s.instances.Start(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	session := atom.Session{ID: "session-1", InstanceID: spec.ID, Model: "test-model", CreatedAt: time.Now()}
	if err := s.database.Sessions().Save(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	return session
}

func (s *stack) user(t *testing.T, session atom.Session, text string) {
	t.Helper()
	message := atom.Message{ID: "user-1", SessionID: session.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: text}}, CreatedAt: time.Now()}
	if err := s.database.Sessions().Append(context.Background(), message); err != nil {
		t.Fatal(err)
	}
}

func TestLoopRunsToolCall(t *testing.T) {
	usage := atom.Usage{Input: 1_000_000, Output: 10}
	s := newStack(t,
		provider.CallWithUsage("fake", `{"value":"x"}`, usage),
		provider.TextWithUsage("the answer", usage),
	)
	ran := int64(0)
	if err := s.registry.Add(&fakeTool{name: "fake", counter: &ran}); err != nil {
		t.Fatal(err)
	}
	session := s.instance(t, 2)
	s.user(t, session, "do the task")
	if err := s.loop.Run(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt64(&ran) != 1 {
		t.Fatalf("tool runs = %d", ran)
	}
	messages, _ := s.database.Sessions().Messages(context.Background(), session.ID)
	if len(messages) != 4 {
		t.Fatalf("messages = %d", len(messages))
	}
	if messages[3].Role != atom.RoleAssistant || messages[3].Content[0].Text != "the answer" {
		t.Fatalf("last message = %+v", messages[3])
	}
	stats, _ := s.database.Usage().Session(context.Background(), session.ID)
	if stats.Calls != 2 || stats.Input != 2_000_000 {
		t.Fatalf("statistics = %+v", stats)
	}
	if stats.Cost == nil || stats.Cost.Value != 2.00004 {
		t.Fatalf("cost = %+v", stats.Cost)
	}
}

func TestLoopRunsToolCallsAtTheSameTime(t *testing.T) {
	s := newStack(t,
		[]atom.ResponsePart{
			{ToolCall: &atom.ToolCall{ID: "call_1", Name: "slow", Input: []byte(`{}`)}},
			{ToolCall: &atom.ToolCall{ID: "call_2", Name: "slow", Input: []byte(`{}`)}},
		},
		provider.Text("done"),
	)
	var current, maximum int64
	tool := &fakeTool{name: "slow", delay: 150 * time.Millisecond}
	tool.run = func(ctx context.Context, call atom.ToolCall) (atom.ToolResult, error) {
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
	if err := s.registry.Add(tool); err != nil {
		t.Fatal(err)
	}
	session := s.instance(t, 2)
	s.user(t, session, "run 2 tools")
	if err := s.loop.Run(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt64(&maximum) < 2 {
		t.Fatalf("the tool calls did not run at the same time: maximum = %d", maximum)
	}
}

func TestLoopAsksForPermission(t *testing.T) {
	s := newStack(t,
		provider.Call("danger", `{}`),
		provider.Text("done"),
	)
	harness.Decide(s.h, atom.StageToolInput, func(ctx context.Context, call atom.ToolCall) (atom.Verdict, error) {
		if call.Name == "danger" {
			return atom.Verdict{Kind: atom.VerdictAsk, Target: "danger", Why: "the tool is dangerous"}, nil
		}
		return atom.Verdict{Kind: atom.VerdictAllow}, nil
	})
	ran := int64(0)
	if err := s.registry.Add(&fakeTool{name: "danger", counter: &ran}); err != nil {
		t.Fatal(err)
	}
	session := s.instance(t, 2)
	s.user(t, session, "do the dangerous task")

	done := make(chan error, 1)
	go func() {
		done <- s.loop.Run(context.Background(), session)
	}()

	request := make(chan atom.PermissionRequest, 1)
	s.bus.On(atom.EventPermissionRequest, func(ctx context.Context, event atom.Event) {
		var parsed atom.PermissionRequest
		if err := jsonUnmarshal(event.Payload, &parsed); err == nil {
			request <- parsed
		}
	})
	select {
	case parsed := <-request:
		if !s.broker.Resolve(parsed.ID, atom.PermissionDecision{Kind: atom.VerdictDeny, Scope: atom.ScopeOnce}) {
			t.Fatal("the permission request is not open")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the permission request did not arrive")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt64(&ran) != 0 {
		t.Fatal("the denied tool ran")
	}
	messages, _ := s.database.Sessions().Messages(context.Background(), session.ID)
	denied := false
	for _, message := range messages {
		if message.Role == atom.RoleTool && strings.Contains(message.Content[0].Text, "denied") {
			denied = true
		}
	}
	if !denied {
		t.Fatalf("the result is not denied: %+v", messages)
	}
}

func TestLoopAddsFoundTools(t *testing.T) {
	s := newStack(t,
		provider.Call("search_tool", `{"query":"fake"}`),
		provider.Call("fake", `{}`),
		provider.Text("done"),
	)
	if err := s.registry.Add(&fakeTool{name: "fake"}); err != nil {
		t.Fatal(err)
	}
	if err := s.registry.Add(tools.NewSearch(s.registry)); err != nil {
		t.Fatal(err)
	}
	session := s.instance(t, 2)
	s.user(t, session, "find a tool")
	if err := s.loop.Run(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	requests := s.provider.Requests
	if len(requests) < 2 {
		t.Fatalf("requests = %d", len(requests))
	}
	found := false
	for _, spec := range requests[1].Tools {
		if spec.Name == "fake" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the found tool is not in the next request: %+v", requests[1].Tools)
	}
}

func TestAgentGivesResultToParent(t *testing.T) {
	s := newStack(t,
		provider.Call("agent", `{"task":"find the answer"}`),
		provider.Call("finish", `{"result":"the child answer"}`),
		provider.Text("the parent is done"),
	)
	if err := s.registry.Add(&agentTool{runner: s.loop}); err != nil {
		t.Fatal(err)
	}
	if err := s.registry.Add(tools.Finish{}); err != nil {
		t.Fatal(err)
	}
	session := s.instance(t, 2)
	s.user(t, session, "start an agent")
	if err := s.loop.Run(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	messages, _ := s.database.Sessions().Messages(context.Background(), session.ID)
	found := false
	for _, message := range messages {
		if message.Role == atom.RoleTool && strings.Contains(message.Content[0].Text, "the child answer") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the parent does not have the child result: %+v", messages)
	}
}

func TestAgentDepthLimit(t *testing.T) {
	s := newStack(t, provider.Text("done"))
	session := s.instance(t, 2)
	session.Depth = 2
	ctx := harness.WithSession(context.Background(), session)
	result, err := s.loop.RunAgentTask(ctx, atom.AgentTask{Task: "work"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Error, "depth limit") {
		t.Fatalf("the depth limit is not applied: %+v", result)
	}
}

type agentTool struct {
	runner *loop.Loop
}

func (a *agentTool) Name() string             { return "agent" }
func (a *agentTool) Description() string      { return "Start an agent" }
func (a *agentTool) Categories() []string     { return []string{"agent"} }
func (a *agentTool) InputSchema() atom.Schema { return atom.Schema{JSON: []byte(`{"type":"object"}`)} }
func (a *agentTool) Check(ctx context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}
func (a *agentTool) Run(ctx context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	var input struct {
		Task string `json:"task"`
	}
	_ = jsonUnmarshal(call.Input, &input)
	return a.runner.RunAgentTask(ctx, atom.AgentTask{Task: input.Task})
}

var _ = sync.Mutex{}
