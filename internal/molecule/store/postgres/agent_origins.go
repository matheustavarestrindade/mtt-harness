package postgres

import (
	"context"
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (eventStore *events) AgentOrigins(operationContext context.Context, instanceID string, sessionIDs []atom.SessionID) ([]atom.AgentOrigin, error) {
	if len(sessionIDs) > 1000 {
		return nil, fmt.Errorf("agent origin limit is 1000")
	}
	if len(sessionIDs) == 0 {
		return []atom.AgentOrigin{}, nil
	}
	identifiers := make([]string, len(sessionIDs))
	for index, identifier := range sessionIDs {
		identifiers[index] = string(identifier)
	}
	rows, operationError := eventStore.store.pool.Query(operationContext, `SELECT DISTINCT ON (start.payload->>'session') start.payload->>'session',start.session_id,COALESCE(NULLIF(start.payload->>'message_id',''),previous.message_id,''),COALESCE(start.payload->>'call_id','') FROM events start LEFT JOIN LATERAL (SELECT payload->>'message_id' AS message_id FROM events WHERE session_id=start.session_id AND name='model.call' AND seq<start.seq ORDER BY seq DESC LIMIT 1) previous ON COALESCE(start.payload->>'message_id','')='' WHERE start.instance_id=$1 AND start.name='agent.start' AND start.payload->>'session'=ANY($2::text[]) ORDER BY start.payload->>'session',start.seq DESC`, instanceID, identifiers)
	if operationError != nil {
		return nil, operationError
	}
	defer rows.Close()
	origins := []atom.AgentOrigin{}
	for rows.Next() {
		var origin atom.AgentOrigin
		if operationError = rows.Scan(&origin.SessionID, &origin.Parent, &origin.MessageID, &origin.ToolCallID); operationError != nil {
			return nil, operationError
		}
		origins = append(origins, origin)
	}
	return origins, rows.Err()
}
