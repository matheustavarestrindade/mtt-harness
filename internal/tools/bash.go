package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/processes"
)

type Bash struct {
	manager *processes.Manager
}

func NewBash(manager *processes.Manager) *Bash {
	return &Bash{manager: manager}
}

func (b *Bash) Name() string {
	return "bash"
}

func (b *Bash) Description() string {
	return "Start a shell command. Read the output or monitor the process."
}

func (b *Bash) Categories() []string {
	return []string{"process", "command"}
}

func (b *Bash) InputSchema() atom.Schema {
	return schema(`{"type":"object","properties":{"command":{"type":"string"},"wait":{"type":"boolean"},"notify":{"type":"string","enum":["exit","error","interval"]},"interval":{"type":"integer"},"timeout":{"type":"integer"}},"required":["command"]}`)
}

func (b *Bash) Check(ctx context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

type bashInput struct {
	Command  string `json:"command"`
	Wait     *bool  `json:"wait"`
	Notify   string `json:"notify"`
	Interval int    `json:"interval"`
	Timeout  int    `json:"timeout"`
}

func (b *Bash) Run(ctx context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	var input bashInput
	if err := json.Unmarshal(call.Input, &input); err != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "bash: the input is not correct"}, err
	}
	spec := atom.ProcessSpec{
		Command: "sh",
		Args:    []string{"-c", input.Command},
		Notify:  atom.NotifyPolicy{Mode: atom.NotifyMode(input.Notify)},
	}
	if input.Interval > 0 {
		spec.Notify.Interval = time.Duration(input.Interval) * time.Millisecond
	}
	if input.Timeout > 0 {
		spec.Timeout = time.Duration(input.Timeout) * time.Millisecond
	}
	proc, err := b.manager.Start(ctx, spec)
	if err != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: err.Error()}, err
	}
	wait := true
	if input.Wait != nil {
		wait = *input.Wait
	}
	if !wait {
		return atom.ToolResult{
			CallID:  call.ID,
			Status:  atom.StatusOK,
			Content: []atom.Content{{Type: atom.Text, Text: fmt.Sprintf("process %s is started", proc.ID())}},
		}, nil
	}
	status, err := proc.Wait()
	stdout, stderr, _ := b.manager.Output(proc.ID())
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
	if err != nil || status.Code != 0 {
		result.Status = atom.StatusError
		result.Error = status.Error
		if result.Error == "" {
			result.Error = fmt.Sprintf("the exit code is %d", status.Code)
		}
	}
	return result, nil
}
