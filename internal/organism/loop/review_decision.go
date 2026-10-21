package loop

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (agentLoop *Loop) finishReviewSession(session atom.Session, call atom.ToolCall) atom.ToolResult {
	var decision struct {
		Kind         string `json:"kind"`
		Message      string `json:"message,omitempty"`
		Reason       string `json:"reason,omitempty"`
		ContinueChat *bool  `json:"continue_chat,omitempty"`
	}
	if operationError := json.Unmarshal(call.Input, &decision); operationError != nil {
		return failedToolResult(call, operationError)
	}
	decision.Kind = call.Name
	if decision.Kind == "interfere" && strings.TrimSpace(decision.Message) == "" {
		return failedToolResult(call, fmt.Errorf("interfere message is empty"))
	}
	data, operationError := json.Marshal(decision)
	if operationError != nil {
		return failedToolResult(call, operationError)
	}
	agentLoop.mutex.Lock()
	defer agentLoop.mutex.Unlock()
	if _, found := agentLoop.finished[session.ID]; found {
		return failedToolResult(call, fmt.Errorf("review already has a terminal decision"))
	}
	agentLoop.finished[session.ID] = string(data)
	return atom.ToolResult{CallID: call.ID, Status: atom.StatusOK, Content: []atom.Content{{Type: atom.Text, Text: "Review decision recorded."}}}
}
