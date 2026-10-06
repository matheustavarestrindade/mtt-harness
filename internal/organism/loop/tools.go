package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/schema"
)

func (agentLoop *Loop) executeToolCall(operationContext context.Context, session atom.Session, call atom.ToolCall) atom.ToolResult {
	started := time.Now()
	if operationError := agentLoop.emitSessionEvent(operationContext, session, atom.EventToolStart, call); operationError != nil {
		return failedToolResult(call, operationError)
	}
	result := agentLoop.runValidatedToolCall(operationContext, session, call)
	result.CallID = call.ID
	result.Duration = time.Since(started)
	completionContext, cancel := context.WithTimeout(context.WithoutCancel(operationContext), 5*time.Second)
	defer cancel()
	if operationError := agentLoop.emitSessionEvent(completionContext, session, atom.EventToolEnd, map[string]any{
		"call":   call,
		"status": result.Status,
		"result": result,
	}); operationError != nil {
		return failedToolResult(call, operationError)
	}
	return result
}

func describeTool(tool harness.Tool) atom.ToolSpec {
	return atom.ToolSpec{
		Name:        tool.Name(),
		Description: tool.Description(),
		Categories:  tool.Categories(),
		InputSchema: tool.InputSchema(),
	}
}

func removeTool(specifications []atom.ToolSpec, name string) []atom.ToolSpec {
	output := specifications[:0]
	for _, toolSpec := range specifications {
		if toolSpec.Name != name {
			output = append(output, toolSpec)
		}
	}
	return output
}
func (agentLoop *Loop) runValidatedToolCall(operationContext context.Context, session atom.Session, call atom.ToolCall) atom.ToolResult {
	call, operationError := agentLoop.pipeline.Plan(operationContext, call)
	if operationError != nil {
		return failedToolResult(call, operationError)
	}
	call, operationError = agentLoop.pipeline.Input(operationContext, call)
	if operationError != nil {
		return failedToolResult(call, operationError)
	}
	tool, found := agentLoop.configuration.Registry.Get(call.Name)
	if !found {
		return failedToolResult(call, fmt.Errorf("tool %q is not registered", call.Name))
	}
	if operationError := schema.Validate(tool.InputSchema().JSON, call.Input); operationError != nil {
		return failedToolResult(call, fmt.Errorf("loop: the input of %s is not correct: %w", call.Name, operationError))
	}
	check := tool.Check(operationContext, call)
	verdict, operationError := agentLoop.pipeline.DecideInput(operationContext, call)
	if operationError != nil {
		return failedToolResult(call, operationError)
	}
	if check.Kind == atom.VerdictDeny || verdict.Kind == atom.VerdictDeny {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusDenied, Error: "the tool call is denied"}
	}
	for _, decision := range []atom.Verdict{check, verdict} {
		allowed, operationError := agentLoop.resolvePermissionVerdict(operationContext, session, call.Name, decision)
		if operationError != nil {
			return failedToolResult(call, operationError)
		}
		if !allowed {
			return atom.ToolResult{CallID: call.ID, Status: atom.StatusDenied, Error: "the tool call is denied"}
		}
	}
	if operationError := operationContext.Err(); operationError != nil {
		return failedToolResult(call, operationError)
	}
	if call.Name == "finish" {
		if session.Parent == "" {
			return atom.ToolResult{CallID: call.ID, Status: atom.StatusDenied, Error: "finish is only available to child agents"}
		}
		return agentLoop.finishChildAgent(session, call)
	}
	result, operationError := tool.Run(operationContext, call)
	if operationError != nil {
		if result.Status == "" {
			result.Status = atom.StatusError
		}
		if result.Error == "" {
			result.Error = operationError.Error()
		}
	}
	if result.Status == "" {
		result.Status = atom.StatusOK
	}
	if call.Name == "search_tool" && result.Status == atom.StatusOK {
		if operationError := agentLoop.activateDiscoveredTools(operationContext, session, result); operationError != nil {
			return failedToolResult(call, operationError)
		}
	}
	return result
}

func (agentLoop *Loop) sessionToolDefinitions(operationContext context.Context, session atom.Session) ([]atom.ToolSpec, error) {
	agentLoop.mutex.Lock()
	group := append([]atom.ToolSpec(nil), agentLoop.groups[session.ID]...)
	agentLoop.mutex.Unlock()
	var specifications []atom.ToolSpec
	for _, selected := range group {
		if tool, found := agentLoop.configuration.Registry.Get(selected.Name); found {
			specifications = append(specifications, describeTool(tool))
		}
	}
	names := map[string]bool{}
	for _, toolSpec := range specifications {
		names[toolSpec.Name] = true
	}
	// Discovery is the bootstrap capability. Other definitions enter a session
	// through discovery and always resolve against the current registry.
	for _, name := range []string{"search_tool"} {
		if names[name] {
			continue
		}
		if tool, found := agentLoop.configuration.Registry.Get(name); found {
			specifications = append(specifications, describeTool(tool))
			names[name] = true
		}
	}
	if session.Depth > 0 && !names["finish"] {
		if tool, found := agentLoop.configuration.Registry.Get("finish"); found {
			specifications = append(specifications, describeTool(tool))
		}
	}
	if agentLoop.configuration.Instances != nil {
		limit, operationError := agentLoop.configuration.Instances.AgentDepthLimit(operationContext, session.InstanceID)
		if operationError != nil {
			return nil, operationError
		}
		if session.Depth >= limit {
			specifications = removeTool(specifications, "agent")
		}
	}
	for index := range specifications {
		if specifications[index].Name != "agent" {
			continue
		}
		var operationError error
		specifications[index], operationError = agentLoop.addAgentModelChoices(session, specifications[index])
		if operationError != nil {
			return nil, operationError
		}
	}
	return specifications, nil
}

func (agentLoop *Loop) activateDiscoveredTools(operationContext context.Context, session atom.Session, result atom.ToolResult) error {
	var references []atom.ToolReference
	if operationError := json.Unmarshal([]byte(result.Text()), &references); operationError != nil {
		return operationError
	}
	var found []atom.ToolSpec
	for _, reference := range references {
		if tool, exists := agentLoop.configuration.Registry.Get(reference.Name); exists {
			found = append(found, describeTool(tool))
		}
	}
	agentLimit := 0
	if agentLoop.configuration.Instances != nil {
		var operationError error
		agentLimit, operationError = agentLoop.configuration.Instances.AgentDepthLimit(operationContext, session.InstanceID)
		if operationError != nil {
			return operationError
		}
	}
	agentLoop.mutex.Lock()
	defer agentLoop.mutex.Unlock()
	known := map[string]bool{}
	for _, toolSpec := range agentLoop.groups[session.ID] {
		known[toolSpec.Name] = true
	}
	for _, toolSpec := range found {
		if toolSpec.Name == "agent" && agentLimit > 0 && session.Depth >= agentLimit {
			continue
		}
		if known[toolSpec.Name] {
			continue
		}
		agentLoop.groups[session.ID] = append(agentLoop.groups[session.ID], toolSpec)
		known[toolSpec.Name] = true
	}
	return nil
}

func failedToolResult(call atom.ToolCall, operationError error) atom.ToolResult {
	return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: operationError.Error()}
}
