package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/processes"
)

type ProcessOutput struct {
	manager *processes.Manager
}

func NewProcessOutput(manager *processes.Manager) *ProcessOutput {
	return &ProcessOutput{manager: manager}
}

func (p *ProcessOutput) Name() string {
	return "process_output"
}

func (p *ProcessOutput) Description() string {
	return "Read the output of a running process."
}

func (p *ProcessOutput) Categories() []string {
	return []string{"process"}
}

func (p *ProcessOutput) InputSchema() atom.Schema {
	return schema(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`)
}

func (p *ProcessOutput) Check(ctx context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (p *ProcessOutput) Run(ctx context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	var input struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(call.Input, &input); err != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "process_output: the input is not correct"}, err
	}
	stdout, stderr, ok := p.manager.Output(input.ID)
	if !ok {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "process_output: the process is not in the manager"}, nil
	}
	text := string(stdout)
	if len(stderr) > 0 {
		text += "\n" + string(stderr)
	}
	return atom.ToolResult{
		CallID:  call.ID,
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: text}},
	}, nil
}

type ProcessKill struct {
	manager *processes.Manager
}

func NewProcessKill(manager *processes.Manager) *ProcessKill {
	return &ProcessKill{manager: manager}
}

func (p *ProcessKill) Name() string {
	return "process_kill"
}

func (p *ProcessKill) Description() string {
	return "Stop a running process."
}

func (p *ProcessKill) Categories() []string {
	return []string{"process"}
}

func (p *ProcessKill) InputSchema() atom.Schema {
	return schema(`{"type":"object","properties":{"id":{"type":"string"},"signal":{"type":"string"}},"required":["id"]}`)
}

func (p *ProcessKill) Check(ctx context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (p *ProcessKill) Run(ctx context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	var input struct {
		ID     string `json:"id"`
		Signal string `json:"signal"`
	}
	if err := json.Unmarshal(call.Input, &input); err != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "process_kill: the input is not correct"}, err
	}
	proc, ok := p.manager.Get(input.ID)
	if !ok {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "process_kill: the process is not in the manager"}, nil
	}
	if err := proc.Kill(atom.Signal(input.Signal)); err != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: err.Error()}, err
	}
	return atom.ToolResult{
		CallID:  call.ID,
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: fmt.Sprintf("the signal is sent at %s", time.Now().Format(time.RFC3339))}},
	}, nil
}
