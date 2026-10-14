package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/process"
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
	return "Execute a shell command or script with sh -c in the instance workspace. By default, wait for exit or 10000 milliseconds without output. Exit returns retained stdout and stderr (last 256 KiB per stream) without a duplicate notification. Inactivity returns a running process ID and recent output (last 20 lines / 4096 bytes per stream), leaving the command alive and enabling its background notify policy. Silence does not prove completion or failure. Use process_output to inspect it or process_kill to stop it. Set idle_timeout=0 to wait until exit without inactivity handling. Set wait=false to return the process ID immediately. Background processes can send one inactivity notice, plus the selected notification policy; notify=none suppresses background updates. timeout, idle_timeout and interval are milliseconds."
}

func (bashTool *Bash) Categories() []string {
	return []string{"process", "command", "shell"}
}

func (bashTool *Bash) InputSchema() atom.Schema {
	return schema(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "Shell command to execute with sh -c. The working directory is the instance workspace; use shell quoting and redirection as needed."
			},
			"wait": {
				"type": "boolean",
				"default": true,
				"description": "Wait for exit or the idle_timeout quiet period. An inactivity return leaves the command running and activates notify; it is not a completed command. Exit returns output only through this tool result. False starts in the background immediately. Cancelling a foreground wait does not stop the command and does not enable background notifications; use process_kill to stop it."
			},
			"notify": {
				"type": "string",
				"enum": ["none", "exit", "error", "interval"],
				"default": "exit",
				"description": "Notification policy after wait=false or an inactivity return: none disables updates; exit reports exit; error reports an error exit; interval reports periodic output and exit. With a positive idle_timeout, non-none policies also allow one no-output status notice per process. A foreground inactivity return is that process's notice, so it is not queued again. Runtime notifications are process data, not user messages. Default: exit."
			},
			"interval": {
				"type": "integer",
				"minimum": 0,
				"description": "Background output notification interval in milliseconds (ms). Required and greater than zero when notify=interval, including after a foreground inactivity return. Ignored for other modes. For example, 1000 means one second."
			},
			"timeout": {
				"type": "integer",
				"default": 0,
				"minimum": 0,
				"description": "Maximum process runtime in milliseconds (ms). A positive value stops the process when elapsed; omitted or zero means no automatic timeout. Applies to both wait=true and wait=false. For example, 60000 means 60 seconds."
			},
			"idle_timeout": {
				"type": "integer",
				"default": 10000,
				"minimum": 0,
				"description": "No-output period in milliseconds (ms). Default 10000 (10 seconds); zero disables inactivity handling. For wait=true, a quiet running command returns its process ID and recent output before timeout, then continues in the background. For wait=false and notify other than none, one quiet-period runtime notice is allowed. New stdout/stderr bytes postpone the notice; silence is not proof of exit or failure. Inactivity never kills the process or changes its runtime timeout."
			}
		},
		"required": ["command"]
	}`)
}

func (bashTool *Bash) Check(operationContext context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

type bashInput struct {
	Command     string `json:"command"`
	Wait        *bool  `json:"wait"`
	Notify      string `json:"notify"`
	Interval    int    `json:"interval"`
	Timeout     int    `json:"timeout"`
	IdleTimeout *int64 `json:"idle_timeout"`
}

func (bashTool *Bash) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	var input bashInput
	if operationError := json.Unmarshal(call.Input, &input); operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "bash: the input is not correct"}, operationError
	}
	waitForExit := input.Wait == nil || *input.Wait
	idleMilliseconds := int64(10000)
	if input.IdleTimeout != nil {
		idleMilliseconds = *input.IdleTimeout
	}
	const maximumMilliseconds = int64((1<<63 - 1) / int64(time.Millisecond))
	if idleMilliseconds < 0 || idleMilliseconds > maximumMilliseconds || input.Timeout < 0 || int64(input.Timeout) > maximumMilliseconds || input.Interval < 0 || int64(input.Interval) > maximumMilliseconds {
		return atom.ToolResult{}, fmt.Errorf("bash: process durations must be between 0 and %d milliseconds", maximumMilliseconds)
	}
	policy := atom.NotifyPolicy{Mode: atom.NotifyMode(input.Notify), Interval: time.Duration(input.Interval) * time.Millisecond}
	if policy.Mode == "" {
		policy.Mode = atom.NotifyExit
	}
	if policy.Mode != atom.NotifyNone && policy.Mode != atom.NotifyExit && policy.Mode != atom.NotifyError && policy.Mode != atom.NotifyInterval {
		return atom.ToolResult{}, fmt.Errorf("invalid process notification mode")
	}
	if policy.Mode == atom.NotifyInterval && policy.Interval <= 0 {
		return atom.ToolResult{}, fmt.Errorf("interval notifications require a positive interval")
	}
	processSpec := atom.ProcessSpec{
		Command:     "sh",
		Args:        []string{"-c", input.Command},
		Notify:      policy,
		IdleTimeout: time.Duration(idleMilliseconds) * time.Millisecond,
	}
	// The tool owns foreground output. Set delivery before Start so even an
	// immediately exiting command cannot queue a duplicate notification.
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
	if _, found := harness.ToolOriginFrom(operationContext); !found {
		operationContext = harness.WithToolOrigin(operationContext, harness.ToolOrigin{CallID: call.ID})
	}
	var runningProcess harness.Process
	var operationError error
	var release func(atom.NotifyPolicy)
	if waitForExit {
		runningProcess, release, operationError = bashTool.manager.StartForeground(operationContext, processSpec)
		defer release(atom.NotifyPolicy{Mode: atom.NotifyNone})
	} else {
		runningProcess, operationError = bashTool.manager.Start(operationContext, processSpec)
	}
	if operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: operationError.Error()}, operationError
	}
	if !waitForExit {
		return atom.ToolResult{
			CallID:  call.ID,
			Status:  atom.StatusOK,
			Content: []atom.Content{{Type: atom.Text, Text: fmt.Sprintf("process %s is started", runningProcess.ID())}},
		}, nil
	}
	processHandle := runningProcess.(*process.Handle)
	status, inactivity, operationError := processHandle.WaitForExitOrIdle(operationContext, processSpec.IdleTimeout)
	if operationContext.Err() != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "the wait is cancelled"}, nil
	}
	stdout, stderr := processHandle.Output()
	if inactivity != nil {
		release(policy)
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusOK, Content: []atom.Content{{Type: atom.Text, Text: processes.InactivityReport(runningProcess.ID(), *inactivity, inactivity.Stdout, inactivity.Stderr)}}}, nil
	}
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
