package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/processes"
)

type Bash struct {
	manager *processes.Manager
}

func NewBash(manager *processes.Manager) *Bash {
	return &Bash{manager: manager}
}

func (bashTool *Bash) Name() string {
	return "bash"
}

func (bashTool *Bash) Description() string {
	return "Start a shell command. Read the output or monitor the process."
}

func (bashTool *Bash) Categories() []string {
	return []string{"process", "command"}
}

func (bashTool *Bash) InputSchema() atom.Schema {
	return schema(`{"type":"object","properties":{"command":{"type":"string"},"wait":{"type":"boolean"},"notify":{"type":"string","enum":["exit","error","interval"]},"interval":{"type":"integer"},"timeout":{"type":"integer"}},"required":["command"]}`)
}

func (bashTool *Bash) Check(operationContext context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

type bashInput struct {
	Command  string `json:"command"`
	Wait     *bool  `json:"wait"`
	Notify   string `json:"notify"`
	Interval int    `json:"interval"`
	Timeout  int    `json:"timeout"`
}

func (bashTool *Bash) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	var input bashInput
	if operationError := json.Unmarshal(call.Input, &input); operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "bash: the input is not correct"}, operationError
	}
	processSpec := atom.ProcessSpec{
		Command: "sh",
		Args:    []string{"-c", input.Command},
		Notify:  atom.NotifyPolicy{Mode: atom.NotifyMode(input.Notify)},
	}
	workspace, found := harness.WorkspaceFrom(operationContext)
	if !found {
		return atom.ToolResult{}, fmt.Errorf("bash: no instance workspace")
	}
	processSpec.Cwd = workspace
	if input.Interval > 0 {
		processSpec.Notify.Interval = time.Duration(input.Interval) * time.Millisecond
	}
	if input.Timeout > 0 {
		processSpec.Timeout = time.Duration(input.Timeout) * time.Millisecond
	}
	runningProcess, operationError := bashTool.manager.Start(operationContext, processSpec)
	if operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: operationError.Error()}, operationError
	}
	wait := true
	if input.Wait != nil {
		wait = *input.Wait
	}
	if !wait {
		return atom.ToolResult{
			CallID:  call.ID,
			Status:  atom.StatusOK,
			Content: []atom.Content{{Type: atom.Text, Text: fmt.Sprintf("process %s is started", runningProcess.ID())}},
		}, nil
	}
	type waitResult struct {
		status         atom.ExitStatus
		operationError error
	}
	waited := make(chan waitResult, 1)
	go func() {
		status, operationError := runningProcess.Wait()
		waited <- waitResult{status: status, operationError: operationError}
	}()
	var status atom.ExitStatus
	select {
	case result := <-waited:
		status, operationError = result.status, result.operationError
	case <-operationContext.Done():
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "the wait is cancelled"}, nil
	}
	stdout, stderr, _ := bashTool.manager.Output(runningProcess.ID())
	var builder strings.Builder
	if len(stdout) > 0 {
		builder.Write(stdout)
	}
	if len(stderr) > 0 {
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.Write(stderr)
	}
	result := atom.ToolResult{
		CallID:  call.ID,
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: builder.String()}},
	}
	if operationError != nil || status.Code != 0 {
		result.Status = atom.StatusError
		result.Error = status.Error
		if result.Error == "" {
			result.Error = fmt.Sprintf("the exit code is %d", status.Code)
		}
	}
	return result, nil
}
