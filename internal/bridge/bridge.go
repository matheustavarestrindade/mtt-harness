package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/registry"
)

type Config struct {
	Harness  *harness.Harness
	Registry *registry.Registry
	Timeout  time.Duration
}

type Bridge struct {
	cfg     Config
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	mu      sync.Mutex
	next    int64
	pending map[int64]chan response
	tools   map[string]bool
	closed  bool
}

type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int64 `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type response struct {
	ID     *int64          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type toolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Categories  []string        `json:"categories"`
	InputSchema json.RawMessage `json:"input_schema"`
}

func Start(ctx context.Context, cfg Config, command string, args ...string) (*Bridge, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	cmd := exec.CommandContext(ctx, command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	b := &Bridge{
		cfg:     cfg,
		cmd:     cmd,
		stdin:   stdin,
		pending: map[int64]chan response{},
		tools:   map[string]bool{},
	}
	go b.read(stdout)
	result, err := b.call(ctx, "initialize", map[string]any{"protocol": 1})
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	var init struct {
		Name  string     `json:"name"`
		Tools []toolSpec `json:"tools"`
	}
	if err := json.Unmarshal(result, &init); err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	for _, spec := range init.Tools {
		tool := &remoteTool{bridge: b, spec: spec}
		if cfg.Registry != nil {
			if err := cfg.Registry.Add(tool); err != nil {
				return nil, err
			}
		}
		if cfg.Harness != nil {
			cfg.Harness.Tool(tool)
		}
		b.tools[spec.Name] = true
	}
	b.attach(ctx)
	return b, nil
}

func (b *Bridge) owns(name string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tools[name]
}

func (b *Bridge) attach(ctx context.Context) {
	if b.cfg.Harness == nil {
		return
	}
	harness.Decide(b.cfg.Harness, atom.StageToolInput, func(ctx context.Context, call atom.ToolCall) (atom.Verdict, error) {
		if !b.owns(call.Name) {
			return atom.Verdict{Kind: atom.VerdictAllow}, nil
		}
		result, err := b.call(ctx, "decide", map[string]any{"name": call.Name, "input": json.RawMessage(call.Input)})
		if err != nil {
			return atom.Verdict{Kind: atom.VerdictAsk, Why: err.Error()}, nil
		}
		var verdict atom.Verdict
		if err := json.Unmarshal(result, &verdict); err != nil {
			return atom.Verdict{Kind: atom.VerdictAllow}, nil
		}
		return verdict, nil
	})
	pipe(ctx, b, "context.build", atom.StageContextBuild)
	pipe(ctx, b, "model.request", atom.StageModelRequest)
	pipe(ctx, b, "model.response", atom.StageModelResponse)
	pipe(ctx, b, "action.plan", atom.StageActionPlan)
	pipe(ctx, b, "tool.input", atom.StageToolInput)
	pipe(ctx, b, "tool.result", atom.StageToolResult)
	pipe(ctx, b, "message.save", atom.StageMessageSave)
	pipe(ctx, b, "process.output", atom.StageProcessOutput)
}

func pipe[T any](ctx context.Context, b *Bridge, name string, stage atom.Stage[T]) {
	harness.Pipe(b.cfg.Harness, stage, func(ctx context.Context, value T) (T, error) {
		result, err := b.call(ctx, "pipe", map[string]any{"stage": name, "value": value})
		if err != nil {
			return value, nil
		}
		var output T
		if err := json.Unmarshal(result, &output); err != nil {
			return value, nil
		}
		return output, nil
	})
}

func (b *Bridge) Notify(event atom.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	data, err := json.Marshal(request{JSONRPC: "2.0", Method: "event", Params: event})
	if err != nil {
		return
	}
	_, _ = b.stdin.Write(append(data, '\n'))
}

func (b *Bridge) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, errors.New("bridge: the connection is closed")
	}
	b.next++
	id := b.next
	channel := make(chan response, 1)
	b.pending[id] = channel
	data, err := json.Marshal(request{JSONRPC: "2.0", ID: &id, Method: method, Params: params})
	if err != nil {
		delete(b.pending, id)
		b.mu.Unlock()
		return nil, err
	}
	if _, err := b.stdin.Write(append(data, '\n')); err != nil {
		delete(b.pending, id)
		b.mu.Unlock()
		return nil, err
	}
	b.mu.Unlock()
	select {
	case answer, ok := <-channel:
		if !ok {
			return nil, errors.New("bridge: the connection is closed")
		}
		if answer.Error != nil {
			return nil, fmt.Errorf("bridge: %s", answer.Error.Message)
		}
		return answer.Result, nil
	case <-time.After(b.cfg.Timeout):
		b.mu.Lock()
		delete(b.pending, id)
		b.mu.Unlock()
		return nil, errors.New("bridge: the request timeout elapsed")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (b *Bridge) read(stdout io.ReadCloser) {
	defer stdout.Close()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		var message response
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			continue
		}
		if message.ID == nil {
			continue
		}
		b.mu.Lock()
		channel, ok := b.pending[*message.ID]
		if ok {
			delete(b.pending, *message.ID)
		}
		b.mu.Unlock()
		if ok {
			channel <- message
		}
	}
	b.mu.Lock()
	b.closed = true
	for id, channel := range b.pending {
		close(channel)
		delete(b.pending, id)
	}
	b.mu.Unlock()
}

func (b *Bridge) Close() error {
	if b.cmd.Process != nil {
		return b.cmd.Process.Kill()
	}
	return nil
}

type remoteTool struct {
	bridge *Bridge
	spec   toolSpec
}

func (t *remoteTool) Name() string             { return t.spec.Name }
func (t *remoteTool) Description() string      { return t.spec.Description }
func (t *remoteTool) Categories() []string     { return t.spec.Categories }
func (t *remoteTool) InputSchema() atom.Schema { return atom.Schema{JSON: t.spec.InputSchema} }

func (t *remoteTool) Check(ctx context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (t *remoteTool) Run(ctx context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	result, err := t.bridge.call(ctx, "tool/run", map[string]any{"name": t.spec.Name, "input": json.RawMessage(call.Input)})
	if err != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: err.Error()}, nil
	}
	var output struct {
		Status  string `json:"status"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(result, &output); err != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: err.Error()}, nil
	}
	toolResult := atom.ToolResult{CallID: call.ID, Status: atom.Status(output.Status), Error: output.Error}
	for _, item := range output.Content {
		media := atom.MediaType(item.Type)
		if media == "" {
			media = atom.Text
		}
		toolResult.Content = append(toolResult.Content, atom.Content{Type: media, Text: item.Text})
	}
	if toolResult.Status == "" {
		toolResult.Status = atom.StatusOK
	}
	return toolResult, nil
}
