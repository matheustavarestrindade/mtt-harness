package api

import (
	"context"
	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (server *Server) resolveAgentOrigins(operationContext context.Context, instanceID string, sessions []atom.Session) error {
	indices := map[atom.SessionID]int{}
	identifiers := []atom.SessionID{}
	for index, session := range sessions {
		if session.Parent != "" && session.ParentMessageID == "" {
			indices[session.ID] = index
			identifiers = append(identifiers, session.ID)
		}
	}
	for len(identifiers) > 0 {
		limit := min(1000, len(identifiers))
		origins, operationError := server.store.Events().AgentOrigins(operationContext, instanceID, identifiers[:limit])
		if operationError != nil {
			return operationError
		}
		for _, origin := range origins {
			index, found := indices[origin.SessionID]
			if found && sessions[index].Parent == origin.Parent {
				sessions[index].ParentMessageID = origin.MessageID
				sessions[index].ParentToolCallID = origin.ToolCallID
			}
		}
		identifiers = identifiers[limit:]
	}
	return nil
}
