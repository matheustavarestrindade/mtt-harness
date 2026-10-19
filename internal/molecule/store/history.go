package store

import (
	"errors"
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

const HistoryPageBytes = 2 * 1024 * 1024

var ErrHistoryChanged = errors.New("session history changed")

// Before/After are exclusive saved-message sequence positions. Omitted limits
// select 50 messages, up to 100; a byte budget can shorten a page. One oversized
// message is returned alone so media and literal tool data remain recoverable.
type HistoryQuery struct {
	Before       int64
	After        int64
	Limit        int
	Revision     *uint64
	MetadataOnly bool
}

func (query HistoryQuery) Validate() (HistoryQuery, error) {
	if query.Before < 0 || query.After < 0 || query.Before > 0 && query.After > 0 {
		return query, fmt.Errorf("history accepts one non-negative before or after position")
	}
	if query.Limit == 0 {
		query.Limit = 50
	}
	if query.Limit < 1 || query.Limit > 100 {
		return query, fmt.Errorf("history limit must be between 1 and 100")
	}
	return query, nil
}

// PublicMessageSize bounds returned public data without counting private state
// or serializing base64 attachments a second time solely to measure them.
func PublicMessageSize(message atom.Message) int {
	size := 256 + len(message.ID) + len(message.Reasoning)
	for _, content := range message.Content {
		size += 128 + len(content.Text) + len(content.URL) + len(content.Filename) + (len(content.Data)+2)/3*4
	}
	for _, call := range message.ToolCalls {
		size += 64 + len(call.ID) + len(call.Name) + len(call.Input)
	}
	return size
}

func BoundaryToolOwner(owner atom.Message, messages []atom.Message) atom.Message {
	needed := map[string]bool{}
	for _, message := range messages {
		if message.Role == atom.RoleTool {
			needed[message.ToolCallID] = true
		}
	}
	var calls []atom.ToolCall
	for _, call := range owner.ToolCalls {
		if needed[call.ID] {
			calls = append(calls, call)
		}
	}
	return atom.Message{ID: owner.ID, SessionID: owner.SessionID, Seq: owner.Seq, Role: atom.RoleAssistant, ToolCalls: calls, CreatedAt: owner.CreatedAt}
}
