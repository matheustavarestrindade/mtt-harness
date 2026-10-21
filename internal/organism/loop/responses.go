package loop

import (
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type toolTask struct {
	call   atom.ToolCall
	index  int
	result chan atom.ToolResult
}

func waitForTools(tasks []*toolTask) []atom.ToolResult {
	results := make([]atom.ToolResult, len(tasks))
	for index, task := range tasks {
		results[index] = <-task.result
	}
	return results
}

func (agentLoop *Loop) receiveModelResponse(operationContext context.Context, session atom.Session, modelCall *preparedModelCall) (response atom.Message, toolTasks []*toolTask, responseError error) {
	session = modelCall.session
	operationContext = harness.WithSession(operationContext, session)
	messageID := newID()
	agentLoop.beginLiveResponse(session, messageID, modelCall.modelID)
	if operationError := agentLoop.emitSessionEvent(operationContext, session, atom.EventModelCall, map[string]any{"model": modelCall.modelID, "message_id": messageID, "reasoning_effort": modelCall.request.ReasoningEffort}); operationError != nil {
		return atom.Message{}, nil, operationError
	}
	var usage *atom.Usage
	started := time.Now()
	if session.Execution != nil {
		defer func() {
			responseError = errors.Join(responseError, agentLoop.recordWorkerAttempt(operationContext, session, modelCall.model, modelCall.modelID, messageID, started, usage, responseError))
		}()
	}
	responseStream, operationError := modelCall.provider.Stream(operationContext, modelCall.request)
	if operationError != nil {
		return atom.Message{}, nil, operationError
	}
	var text strings.Builder
	var reasoning strings.Builder
	var tasks []*toolTask
	var content []atom.Content
	var providerState *atom.ProviderState
	var streamError error
	for {
		part, operationError := responseStream.Recv(operationContext)
		if part.Usage != nil {
			usage = part.Usage
		}
		if errors.Is(operationError, io.EOF) {
			break
		}
		if operationError != nil {
			streamError = operationError
			break
		}
		if part.Text != "" {
			text.WriteString(part.Text)
			chunkIndex := agentLoop.appendLiveText(session.ID, part.Text, false)
			if operationError := agentLoop.emitSessionEvent(operationContext, session, atom.EventModelChunk, map[string]any{"text": part.Text, "message_id": messageID, "chunk_index": chunkIndex}); operationError != nil {
				return atom.Message{}, tasks, operationError
			}
		}
		if part.Reasoning != "" {
			reasoning.WriteString(part.Reasoning)
			chunkIndex := agentLoop.appendLiveText(session.ID, part.Reasoning, true)
			if operationError := agentLoop.emitSessionEvent(operationContext, session, atom.EventModelChunk, map[string]any{"reasoning": part.Reasoning, "message_id": messageID, "chunk_index": chunkIndex}); operationError != nil {
				return atom.Message{}, tasks, operationError
			}
		}
		if part.ToolCall != nil {
			agentLoop.appendLiveTool(session.ID, *part.ToolCall)
			if operationError := agentLoop.emitSessionEvent(operationContext, session, atom.EventActionReceived, part.ToolCall); operationError != nil {
				return atom.Message{}, tasks, operationError
			}
			task := &toolTask{call: *part.ToolCall, index: len(tasks), result: make(chan atom.ToolResult, 1)}
			if part.ToolIndex != nil {
				task.index = *part.ToolIndex
			}
			tasks = append(tasks, task)
			go func() {
				toolContext := harness.WithToolOrigin(operationContext, harness.ToolOrigin{MessageID: messageID, CallID: task.call.ID})
				task.result <- agentLoop.executeToolCall(toolContext, session, task.call)
			}()
		}
		content = append(content, part.Content...)
		if part.ProviderState != nil {
			providerState = part.ProviderState
		}
	}
	sort.SliceStable(tasks, func(first, second int) bool {
		return tasks[first].index < tasks[second].index
	})
	var calls []atom.ToolCall
	for _, task := range tasks {
		calls = append(calls, task.call)
	}
	if text.Len() > 0 || len(content) == 0 {
		content = append([]atom.Content{{Type: atom.Text, Text: text.String()}}, content...)
	}
	message := atom.Message{
		ID: messageID, SessionID: session.ID, Role: atom.RoleAssistant,
		Content:   content,
		Reasoning: reasoning.String(),
		ToolCalls: calls, Usage: usage, CreatedAt: time.Now(),
		ProviderState: providerState,
	}
	if usage != nil {
		usage.Cost = calculateUsageCost(modelCall.model, usage)
	}
	return message, tasks, streamError
}
