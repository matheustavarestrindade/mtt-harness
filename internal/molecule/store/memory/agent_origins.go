package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (eventStore *events) AgentOrigins(operationContext context.Context, instanceID string, sessionIDs []atom.SessionID) ([]atom.AgentOrigin, error) {
	if len(sessionIDs) > 1000 {
		return nil, fmt.Errorf("agent origin limit is 1000")
	}
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	eventStore.store.mutex.RLock()
	defer eventStore.store.mutex.RUnlock()
	wanted := map[atom.SessionID]bool{}
	for _, identifier := range sessionIDs {
		wanted[identifier] = true
	}
	current := map[atom.SessionID]string{}
	origins := map[atom.SessionID]atom.AgentOrigin{}
	for _, event := range eventStore.store.events {
		if event.InstanceID != instanceID || (event.Name != atom.EventAgentStart && event.Name != atom.EventModelCall) {
			continue
		}
		var payload struct {
			Session   atom.SessionID `json:"session"`
			MessageID string         `json:"message_id"`
			CallID    string         `json:"call_id"`
		}
		if operationError := json.Unmarshal(event.Payload, &payload); operationError != nil {
			return nil, operationError
		}
		if event.Name == atom.EventModelCall {
			current[event.SessionID] = payload.MessageID
			continue
		}
		if wanted[payload.Session] {
			messageID := payload.MessageID
			if messageID == "" {
				messageID = current[event.SessionID]
			}
			origins[payload.Session] = atom.AgentOrigin{SessionID: payload.Session, Parent: event.SessionID, MessageID: messageID, ToolCallID: payload.CallID}
		}
	}
	result := []atom.AgentOrigin{}
	for _, identifier := range sessionIDs {
		if origin, found := origins[identifier]; found {
			result = append(result, origin)
		}
	}
	return result, nil
}
