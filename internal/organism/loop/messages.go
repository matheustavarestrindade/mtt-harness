package loop

import (
	"context"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (agentLoop *Loop) saveModelResponse(operationContext context.Context, session atom.Session, modelID string, message atom.Message) error {
	usage := message.Usage
	message, operationError := agentLoop.pipeline.Response(operationContext, message)
	if operationError != nil {
		return operationError
	}
	message, operationError = agentLoop.pipeline.Save(operationContext, message)
	if operationError != nil {
		return operationError
	}
	if operationError := agentLoop.configuration.Store.Sessions().Append(operationContext, message); operationError != nil {
		return operationError
	}
	if usage == nil {
		usage = &atom.Usage{}
	}
	return agentLoop.configuration.Store.Usage().Save(operationContext, atom.UsageRecord{
		InstanceID: session.InstanceID, SessionID: session.ID, ModelID: modelID,
		Usage: *usage, CreatedAt: time.Now(),
	})
}

// Results are stored in model call order, regardless of tool completion order.
func (agentLoop *Loop) saveToolResults(operationContext context.Context, session atom.Session, calls []atom.ToolCall, results []atom.ToolResult) error {
	for index, call := range calls {
		result, operationError := agentLoop.pipeline.Result(operationContext, results[index])
		if operationError != nil {
			return operationError
		}
		content := result.Content
		if result.Error != "" {
			content = append(append([]atom.Content(nil), content...), atom.Content{Type: atom.Text, Text: result.Error})
		}
		message := atom.Message{
			ID: newID(), SessionID: session.ID, Role: atom.RoleTool,
			ToolCallID: call.ID, Content: content,
			CreatedAt: time.Now(),
		}
		if operationError := agentLoop.configuration.Store.Sessions().Append(operationContext, message); operationError != nil {
			return operationError
		}
	}
	return nil
}

// A cancelled turn may have already persisted its assistant tool plan. Close
// every missing tool response before another turn can build provider context.
func (agentLoop *Loop) repairToolHistory(operationContext context.Context, session atom.Session) error {
	messages, operationError := agentLoop.configuration.Store.Sessions().Messages(operationContext, session.ID)
	if operationError != nil {
		return operationError
	}
	var pending []atom.ToolCall
	completed := map[string]bool{}
	for _, message := range messages {
		if message.Role == atom.RoleAssistant {
			pending = message.ToolCalls
			completed = map[string]bool{}
		}
		if message.Role == atom.RoleTool {
			completed[message.ToolCallID] = true
		}
	}
	for _, call := range pending {
		if completed[call.ID] {
			continue
		}
		message := atom.Message{ID: newID(), SessionID: session.ID, Role: atom.RoleTool, ToolCallID: call.ID, Content: []atom.Content{{Type: atom.Text, Text: "The turn was interrupted; this tool result is unavailable."}}, CreatedAt: time.Now()}
		if operationError := agentLoop.configuration.Store.Sessions().Append(operationContext, message); operationError != nil {
			return operationError
		}
	}
	return nil
}
