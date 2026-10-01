package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (agentLoop *Loop) RunAgentTask(operationContext context.Context, task atom.AgentTask) (atom.ToolResult, error) {
	parent, found := harness.SessionFrom(operationContext)
	if !found {
		return atom.ToolResult{Status: atom.StatusError, Error: "agent: the parent session is not in the context"}, nil
	}
	depth := parent.Depth + 1
	limit := 0
	if agentLoop.configuration.Instances != nil {
		var operationError error
		limit, operationError = agentLoop.configuration.Instances.AgentDepthLimit(operationContext, parent.InstanceID)
		if operationError != nil {
			return atom.ToolResult{}, operationError
		}
	}
	if depth > limit {
		return atom.ToolResult{
			Status:  atom.StatusError,
			Error:   "agent: the agent depth limit is reached",
			Content: []atom.Content{{Type: atom.Text, Text: "the agent depth limit is reached"}},
		}, nil
	}
	model := task.Model
	if model == "" {
		if instance, found := agentLoop.configuration.Instances.Get(parent.InstanceID); found {
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
	if _, _, operationError := agentLoop.resolveModel(child.InstanceID, child.Model); operationError != nil {
		return atom.ToolResult{}, operationError
	}
	if operationError := agentLoop.configuration.Store.Sessions().Save(operationContext, child); operationError != nil {
		return atom.ToolResult{Status: atom.StatusError, Error: operationError.Error()}, nil
	}
	defer func() {
		agentLoop.mutex.Lock()
		delete(agentLoop.finished, child.ID)
		delete(agentLoop.groups, child.ID)
		agentLoop.mutex.Unlock()
	}()
	taskMessage := atom.Message{
		ID:        newID(),
		SessionID: child.ID,
		Role:      atom.RoleUser,
		Content:   []atom.Content{{Type: atom.Text, Text: task.Task}},
		CreatedAt: time.Now(),
	}
	if operationError := agentLoop.configuration.Store.Sessions().Append(operationContext, taskMessage); operationError != nil {
		return atom.ToolResult{Status: atom.StatusError, Error: operationError.Error()}, nil
	}
	agentLoop.emitSessionEvent(operationContext, parent, atom.EventAgentStart, map[string]any{"session": child.ID, "task": task.Task})
	if operationError := agentLoop.Run(operationContext, child); operationError != nil {
		return atom.ToolResult{Status: atom.StatusError, Error: operationError.Error()}, nil
	}
	if !agentLoop.isFinished(child.ID) {
		return atom.ToolResult{Status: atom.StatusError, Error: "child agent stopped without calling finish"}, nil
	}
	result := agentLoop.takeCompletedAgentResult(child.ID)
	statistics, _ := agentLoop.configuration.Store.Usage().Session(operationContext, child.ID)
	text := fmt.Sprintf("result: %s\nusage: input %d, output %d, cache read %d, cache hit rate %.2f",
		result, statistics.Input, statistics.Output, statistics.CacheRead, statistics.CacheHitRate())
	agentLoop.emitSessionEvent(operationContext, parent, atom.EventAgentEnd, map[string]any{"session": child.ID})
	return atom.ToolResult{
		CallID:  string(child.ID),
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: text}},
	}, nil
}

func (agentLoop *Loop) finishChildAgent(session atom.Session, call atom.ToolCall) atom.ToolResult {
	var input struct {
		Result string `json:"result"`
	}
	_ = json.Unmarshal(call.Input, &input)
	agentLoop.mutex.Lock()
	agentLoop.finished[session.ID] = input.Result
	agentLoop.mutex.Unlock()
	return atom.ToolResult{
		CallID:  call.ID,
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: "the agent stops"}},
	}
}

func (agentLoop *Loop) isFinished(session atom.SessionID) bool {
	agentLoop.mutex.Lock()
	defer agentLoop.mutex.Unlock()
	_, found := agentLoop.finished[session]
	return found
}

func (agentLoop *Loop) takeCompletedAgentResult(session atom.SessionID) string {
	agentLoop.mutex.Lock()
	defer agentLoop.mutex.Unlock()
	result := agentLoop.finished[session]
	delete(agentLoop.finished, session)
	return result
}
