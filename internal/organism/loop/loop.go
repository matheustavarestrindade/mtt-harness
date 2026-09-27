package loop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/permission"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/pipeline"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/schema"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/processes"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/registry"
)

const defaultMaxRounds = 64

type Config struct {
	Gateway   *gateway.Gateway
	Registry  *registry.Registry
	Store     store.Store
	Bus       *eventbus.Bus
	Broker    *permission.Broker
	Engine    *permission.Engine
	Processes *processes.Manager
	Instances *instances.Manager
	MaxRounds int
}

type Loop struct {
	h        *harness.Harness
	p        *pipeline.Pipeline
	cfg      Config
	mu       sync.Mutex
	groups   map[atom.SessionID][]atom.ToolSpec
	finished map[atom.SessionID]string
}

func New(h *harness.Harness, cfg Config) *Loop {
	if cfg.MaxRounds <= 0 {
		cfg.MaxRounds = defaultMaxRounds
	}
	return &Loop{
		h:        h,
		p:        pipeline.New(h),
		cfg:      cfg,
		groups:   map[atom.SessionID][]atom.ToolSpec{},
		finished: map[atom.SessionID]string{},
	}
}

func (l *Loop) Run(ctx context.Context, session atom.Session) error {
	ctx = harness.WithSession(ctx, session)
	l.emit(ctx, session, atom.EventTurnStart, nil)
	for round := 0; round < l.cfg.MaxRounds; round++ {
		messages, err := l.cfg.Store.Sessions().Messages(ctx, session.ID)
		if err != nil {
			return err
		}
		messages, err = l.p.Context(ctx, messages)
		if err != nil {
			return err
		}
		modelID, info, provider, err := l.resolve(session)
		if err != nil {
			return err
		}
		if err := checkMediaTypes(info, messages); err != nil {
			return err
		}
		request := atom.Request{
			Model:    modelID,
			Messages: messages,
			Tools:    l.toolsFor(ctx, session),
		}
		request, err = l.p.Request(ctx, request)
		if err != nil {
			return err
		}
		verdict, err := l.p.DecideRequest(ctx, request)
		if err != nil {
			return err
		}
		if verdict.Kind == atom.VerdictDeny {
			return nil
		}
		if verdict.Kind == atom.VerdictAsk && !l.allow(ctx, session, "model.request", verdict) {
			return nil
		}
		l.emit(ctx, session, atom.EventModelCall, map[string]any{"model": modelID})
		stream, err := provider.Stream(ctx, request)
		if err != nil {
			return err
		}
		var text strings.Builder
		var calls []atom.ToolCall
		var usage *atom.Usage
		for {
			part, err := stream.Recv(ctx)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return err
			}
			if part.Text != "" {
				text.WriteString(part.Text)
				l.emit(ctx, session, atom.EventModelChunk, map[string]any{"text": part.Text})
			}
			if part.ToolCall != nil {
				calls = append(calls, *part.ToolCall)
				l.emit(ctx, session, atom.EventActionReceived, part.ToolCall)
			}
			if part.Usage != nil {
				usage = part.Usage
			}
		}
		message := atom.Message{
			ID:        newID(),
			SessionID: session.ID,
			Role:      atom.RoleAssistant,
			Content:   []atom.Content{{Type: atom.Text, Text: text.String()}},
			ToolCalls: calls,
			Usage:     usage,
			CreatedAt: time.Now(),
		}
		if usage != nil {
			usage.Cost = cost(info, usage)
		}
		message, err = l.p.Response(ctx, message)
		if err != nil {
			return err
		}
		message, err = l.p.Save(ctx, message)
		if err != nil {
			return err
		}
		if err := l.cfg.Store.Sessions().Append(ctx, message); err != nil {
			return err
		}
		if usage != nil {
			record := atom.UsageRecord{
				InstanceID: session.InstanceID,
				SessionID:  session.ID,
				ModelID:    modelID,
				Usage:      *usage,
				CreatedAt:  time.Now(),
			}
			if err := l.cfg.Store.Usage().Save(ctx, record); err != nil {
				return err
			}
		}
		if len(calls) == 0 {
			l.emit(ctx, session, atom.EventTurnEnd, nil)
			return nil
		}
		results := l.runAll(ctx, session, calls)
		for index, call := range calls {
			result := results[index]
			result, err = l.p.Result(ctx, result)
			if err != nil {
				return err
			}
			message := atom.Message{
				ID:         newID(),
				SessionID:  session.ID,
				Role:       atom.RoleTool,
				ToolCallID: call.ID,
				Content:    []atom.Content{{Type: atom.Text, Text: result.Text()}},
				CreatedAt:  time.Now(),
			}
			if err := l.cfg.Store.Sessions().Append(ctx, message); err != nil {
				return err
			}
		}
		if l.isFinished(session.ID) {
			l.emit(ctx, session, atom.EventTurnEnd, nil)
			return nil
		}
	}
	l.emit(ctx, session, atom.EventTurnEnd, nil)
	return nil
}

func (l *Loop) runAll(ctx context.Context, session atom.Session, calls []atom.ToolCall) []atom.ToolResult {
	results := make([]atom.ToolResult, len(calls))
	var wait sync.WaitGroup
	for index := range calls {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			results[index] = l.execute(ctx, session, calls[index])
		}(index)
	}
	wait.Wait()
	return results
}

func (l *Loop) execute(ctx context.Context, session atom.Session, call atom.ToolCall) atom.ToolResult {
	started := time.Now()
	l.emit(ctx, session, atom.EventToolStart, call)
	result := l.run(ctx, session, call)
	result.CallID = call.ID
	result.Duration = time.Since(started)
	l.emit(ctx, session, atom.EventToolEnd, map[string]any{
		"call":   call,
		"status": result.Status,
	})
	return result
}

func (l *Loop) run(ctx context.Context, session atom.Session, call atom.ToolCall) atom.ToolResult {
	call, err := l.p.Plan(ctx, call)
	if err != nil {
		return failure(call, err)
	}
	call, err = l.p.Input(ctx, call)
	if err != nil {
		return failure(call, err)
	}
	verdict, err := l.p.DecideInput(ctx, call)
	if err != nil {
		return failure(call, err)
	}
	if !l.allow(ctx, session, call.Name, verdict) {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusDenied, Error: "the tool call is denied"}
	}
	tool, ok := l.cfg.Registry.Get(call.Name)
	if !ok {
		return failure(call, errors.New("loop: the tool is not in the registry"))
	}
	if err := schema.Validate(tool.InputSchema().JSON, call.Input); err != nil {
		return failure(call, fmt.Errorf("loop: the input of %s is not correct: %w", call.Name, err))
	}
	if check := tool.Check(ctx, call); check.Kind != atom.VerdictAllow {
		if !l.allow(ctx, session, call.Name, check) {
			return atom.ToolResult{CallID: call.ID, Status: atom.StatusDenied, Error: "the tool call is denied"}
		}
	}
	if call.Name == "finish" {
		return l.finish(session, call)
	}
	result, err := tool.Run(ctx, call)
	if err != nil {
		if result.Status == "" {
			result.Status = atom.StatusError
		}
		if result.Error == "" {
			result.Error = err.Error()
		}
	}
	if result.Status == "" {
		result.Status = atom.StatusOK
	}
	if call.Name == "search_tool" && result.Status == atom.StatusOK {
		l.absorb(ctx, session, result)
	}
	return result
}

func (l *Loop) finish(session atom.Session, call atom.ToolCall) atom.ToolResult {
	var input struct {
		Result string `json:"result"`
	}
	_ = json.Unmarshal(call.Input, &input)
	l.mu.Lock()
	l.finished[session.ID] = input.Result
	l.mu.Unlock()
	return atom.ToolResult{
		CallID:  call.ID,
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: "the agent stops"}},
	}
}

func (l *Loop) allow(ctx context.Context, session atom.Session, name string, verdict atom.Verdict) bool {
	if verdict.Kind == atom.VerdictAllow {
		return true
	}
	target := verdict.Target
	if target == "" {
		target = name
	}
	if decision, ok := l.cfg.Engine.Cached(target); ok {
		return decision.Kind != atom.VerdictDeny
	}
	if verdict.Kind == atom.VerdictDeny {
		return false
	}
	request := atom.PermissionRequest{
		ID:        newID(),
		SessionID: session.ID,
		Target:    target,
		Why:       verdict.Why,
	}
	l.emit(ctx, session, atom.EventPermissionRequest, request)
	decision, err := l.cfg.Broker.Request(ctx, request)
	if err != nil {
		return false
	}
	_ = l.cfg.Store.Permissions().Save(ctx, decision)
	l.cfg.Engine.Remember(target, decision)
	l.emit(ctx, session, atom.EventPermissionDecision, decision)
	return decision.Kind != atom.VerdictDeny
}

func (l *Loop) RunAgentTask(ctx context.Context, task atom.AgentTask) (atom.ToolResult, error) {
	parent, ok := harness.SessionFrom(ctx)
	if !ok {
		return atom.ToolResult{Status: atom.StatusError, Error: "agent: the parent session is not in the context"}, nil
	}
	depth := parent.Depth + 1
	if l.cfg.Instances != nil && depth > l.cfg.Instances.AgentDepthLimit(ctx, parent.InstanceID) {
		return atom.ToolResult{
			Status:  atom.StatusError,
			Error:   "agent: the agent depth limit is reached",
			Content: []atom.Content{{Type: atom.Text, Text: "the agent depth limit is reached"}},
		}, nil
	}
	model := task.Model
	if model == "" {
		if instance, ok := l.cfg.Instances.Get(parent.InstanceID); ok {
			model = instance.Spec().DefaultModel
		}
	}
	child := atom.Session{
		ID:         atom.SessionID(newID()),
		InstanceID: parent.InstanceID,
		Parent:     parent.ID,
		Depth:      depth,
		Model:      model,
		CreatedAt:  time.Now(),
	}
	if err := l.cfg.Store.Sessions().Save(ctx, child); err != nil {
		return atom.ToolResult{Status: atom.StatusError, Error: err.Error()}, nil
	}
	taskMessage := atom.Message{
		ID:        newID(),
		SessionID: child.ID,
		Role:      atom.RoleUser,
		Content:   []atom.Content{{Type: atom.Text, Text: task.Task}},
		CreatedAt: time.Now(),
	}
	if err := l.cfg.Store.Sessions().Append(ctx, taskMessage); err != nil {
		return atom.ToolResult{Status: atom.StatusError, Error: err.Error()}, nil
	}
	l.emit(ctx, parent, atom.EventAgentStart, map[string]any{"session": child.ID, "task": task.Task})
	if err := l.Run(ctx, child); err != nil {
		return atom.ToolResult{Status: atom.StatusError, Error: err.Error()}, nil
	}
	result := l.popFinished(child.ID)
	stats, _ := l.cfg.Store.Usage().Session(ctx, child.ID)
	text := fmt.Sprintf("result: %s\nusage: input %d, output %d, cache read %d, cache hit rate %.2f",
		result, stats.Input, stats.Output, stats.CacheRead, stats.CacheHitRate())
	l.emit(ctx, parent, atom.EventAgentEnd, map[string]any{"session": child.ID})
	return atom.ToolResult{
		CallID:  string(child.ID),
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: text}},
	}, nil
}

func (l *Loop) resolve(session atom.Session) (string, atom.ModelInfo, harness.Provider, error) {
	modelID := session.Model
	if modelID == "" {
		if instance, ok := l.cfg.Instances.Get(session.InstanceID); ok {
			modelID = instance.Spec().DefaultModel
		}
	}
	info, provider, ok := l.cfg.Gateway.Model(modelID)
	if !ok {
		return "", atom.ModelInfo{}, nil, fmt.Errorf("loop: the model %q is not in the gateway", modelID)
	}
	return modelID, info, provider, nil
}

func (l *Loop) toolsFor(ctx context.Context, session atom.Session) []atom.ToolSpec {
	l.mu.Lock()
	group := append([]atom.ToolSpec(nil), l.groups[session.ID]...)
	l.mu.Unlock()
	specs := group
	names := map[string]bool{}
	for _, spec := range specs {
		names[spec.Name] = true
	}
	if !names["search_tool"] {
		if tool, ok := l.cfg.Registry.Get("search_tool"); ok {
			specs = append(specs, specOf(tool))
			names["search_tool"] = true
		}
	}
	if session.Depth > 0 && !names["finish"] {
		if tool, ok := l.cfg.Registry.Get("finish"); ok {
			specs = append(specs, specOf(tool))
		}
	}
	if l.cfg.Instances != nil && session.Depth >= l.cfg.Instances.AgentDepthLimit(ctx, session.InstanceID) {
		specs = removeTool(specs, "agent")
	}
	return specs
}

func (l *Loop) absorb(ctx context.Context, session atom.Session, result atom.ToolResult) {
	var found []atom.ToolSpec
	if err := json.Unmarshal([]byte(result.Text()), &found); err != nil {
		return
	}
	agentLimit := 0
	if l.cfg.Instances != nil {
		agentLimit = l.cfg.Instances.AgentDepthLimit(ctx, session.InstanceID)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	known := map[string]bool{}
	for _, spec := range l.groups[session.ID] {
		known[spec.Name] = true
	}
	for _, spec := range found {
		if spec.Name == "agent" && agentLimit > 0 && session.Depth >= agentLimit {
			continue
		}
		if known[spec.Name] {
			continue
		}
		l.groups[session.ID] = append(l.groups[session.ID], spec)
		known[spec.Name] = true
	}
}

func (l *Loop) isFinished(session atom.SessionID) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.finished[session]
	return ok
}

func (l *Loop) popFinished(session atom.SessionID) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := l.finished[session]
	delete(l.finished, session)
	return result
}

func (l *Loop) emit(ctx context.Context, session atom.Session, name atom.EventName, payload any) {
	var raw json.RawMessage
	if payload != nil {
		raw, _ = json.Marshal(payload)
	}
	event := atom.Event{
		InstanceID: session.InstanceID,
		SessionID:  session.ID,
		Name:       name,
		Payload:    raw,
		Time:       time.Now(),
	}
	if l.cfg.Bus != nil {
		l.cfg.Bus.Send(ctx, event)
	}
}

func specOf(tool harness.Tool) atom.ToolSpec {
	return atom.ToolSpec{
		Name:        tool.Name(),
		Description: tool.Description(),
		Categories:  tool.Categories(),
		InputSchema: tool.InputSchema(),
	}
}

func removeTool(specs []atom.ToolSpec, name string) []atom.ToolSpec {
	out := specs[:0]
	for _, spec := range specs {
		if spec.Name != name {
			out = append(out, spec)
		}
	}
	return out
}

func checkMediaTypes(info atom.ModelInfo, messages []atom.Message) error {
	if len(info.Input) == 0 {
		return nil
	}
	allowed := map[atom.MediaType]bool{}
	for _, media := range info.Input {
		allowed[media] = true
	}
	for _, message := range messages {
		for _, item := range message.Content {
			if item.Type == "" {
				continue
			}
			if !allowed[item.Type] {
				return fmt.Errorf("loop: the model %s does not accept the media type %s", info.ID, item.Type)
			}
		}
	}
	return nil
}

func failure(call atom.ToolCall, err error) atom.ToolResult {
	return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: err.Error()}
}

func cost(info atom.ModelInfo, usage *atom.Usage) *atom.Cost {
	if info.Prices == nil || usage == nil {
		return nil
	}
	prices := info.Prices
	value := (float64(usage.Input)*prices.Input +
		float64(usage.Output)*prices.Output +
		float64(usage.CacheRead)*prices.CacheRead +
		float64(usage.CacheWrite)*prices.CacheWrite) / 1_000_000
	return &atom.Cost{Currency: prices.Currency, Value: value}
}

func newID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(data[:])
}
