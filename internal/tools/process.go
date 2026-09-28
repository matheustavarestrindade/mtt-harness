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

func (processOutputTool *ProcessOutput) Name() string {
	return "process_output"
}

func (processOutputTool *ProcessOutput) Description() string {
	return "Read the output of a running process."
}

func (processOutputTool *ProcessOutput) Categories() []string {
	return []string{"process"}
}

func (processOutputTool *ProcessOutput) InputSchema() atom.Schema {
	return schema(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`)
}

func (processOutputTool *ProcessOutput) Check(operationContext context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (processOutputTool *ProcessOutput) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	var input struct {
		ID string `json:"id"`
	}
	if operationError := json.Unmarshal(call.Input, &input); operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "process_output: the input is not correct"}, operationError
	}
	if operationError := processOutputTool.manager.CheckOwner(operationContext, input.ID); operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusDenied, Error: operationError.Error()}, operationError
	}
	stdout, stderr, found := processOutputTool.manager.Output(input.ID)
	if !found {
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

func (processKillTool *ProcessKill) Name() string {
	return "process_kill"
}

func (processKillTool *ProcessKill) Description() string {
	return "Stop a running process."
}

func (processKillTool *ProcessKill) Categories() []string {
	return []string{"process"}
}

func (processKillTool *ProcessKill) InputSchema() atom.Schema {
	return schema(`{"type":"object","properties":{"id":{"type":"string"},"signal":{"type":"string"}},"required":["id"]}`)
}

func (processKillTool *ProcessKill) Check(operationContext context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (processKillTool *ProcessKill) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	var input struct {
		ID     string `json:"id"`
		Signal string `json:"signal"`
	}
	if operationError := json.Unmarshal(call.Input, &input); operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "process_kill: the input is not correct"}, operationError
	}
	if operationError := processKillTool.manager.CheckOwner(operationContext, input.ID); operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusDenied, Error: operationError.Error()}, operationError
	}
	runningProcess, found := processKillTool.manager.Get(input.ID)
	if !found {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "process_kill: the process is not in the manager"}, nil
	}
	if operationError := runningProcess.Kill(atom.Signal(input.Signal)); operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: operationError.Error()}, operationError
	}
	return atom.ToolResult{
		CallID:  call.ID,
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: fmt.Sprintf("the signal is sent at %s", time.Now().Format(time.RFC3339))}},
	}, nil
}
