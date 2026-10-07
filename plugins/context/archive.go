package contextplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func loadView(database transaction, sessionID atom.SessionID) (sessionView, error) {
	view, operationError := readTransactionValue[sessionView](database, "view", string(sessionID))
	if operationError != nil {
		return view, operationError
	}
	if view.SessionID == "" {
		view = newSessionView(sessionID)
	}
	if view.Removed == nil {
		view.Removed = map[string]bool{}
	}
	if view.Sources == nil {
		view.Sources = map[string]int64{}
	}
	if view.Protected == nil {
		view.Protected = map[int64]bool{}
	}
	if view.Promote == nil {
		view.Promote = map[string]bool{}
	}
	return view, nil
}

func archiveMessages(database transaction, view *sessionView, messages []atom.Message) ([]source, error) {
	sources := make([]source, 0, len(messages))
	for position, message := range messages {
		if message.Ephemeral || message.ID == "" {
			continue
		}
		identifier := view.Sources[message.ID]
		entry := source{ID: identifier, SessionID: view.SessionID, MessageID: message.ID, Sequence: int64(position + 1), Role: message.Role, CreatedAt: message.CreatedAt}
		if identifier == 0 {
			var operationError error
			identifier, operationError = database.NextSourceID()
			if operationError != nil {
				return nil, operationError
			}
			message.ProviderState = nil
			entry.ID, entry.Message, entry.Text = identifier, message, messageSourceText(message)
			if entry.CreatedAt.IsZero() {
				entry.CreatedAt = time.Now().UTC()
			}
			if operationError := database.Put("source", strconv.FormatInt(identifier, 10), entry); operationError != nil {
				return nil, operationError
			}
			view.Sources[message.ID] = identifier
		}
		sources = append(sources, entry)
	}
	return sources, nil
}

func messageSourceText(message atom.Message) string {
	var text strings.Builder
	for _, content := range message.Content {
		if content.Type == atom.Text {
			text.WriteString(content.Text)
			text.WriteByte('\n')
			continue
		}
		text.WriteString(fmt.Sprintf("[%s content]\n", content.Type))
	}
	for _, call := range message.ToolCalls {
		encoded, _ := json.Marshal(call)
		text.Write(encoded)
		text.WriteByte('\n')
	}
	return text.String()
}

func readSources(database transaction, identifiers []int64) ([]source, error) {
	result := make([]source, 0, len(identifiers))
	seen := map[int64]bool{}
	for _, identifier := range identifiers {
		if seen[identifier] {
			continue
		}
		seen[identifier] = true
		entry, operationError := readTransactionValue[source](database, "source", strconv.FormatInt(identifier, 10))
		if operationError != nil {
			return nil, operationError
		}
		if entry.ID == 0 {
			return nil, fmt.Errorf("source message %d is not in this workspace", identifier)
		}
		result = append(result, entry)
	}
	return result, nil
}

func (plugin *Plugin) archiveRequest(operationContext context.Context, workspaceID string, sessionID atom.SessionID, messages []atom.Message) (sessionView, []source, error) {
	var view sessionView
	var sources []source
	operationError := plugin.database.Transact(operationContext, workspaceID, func(database transaction) error {
		var operationError error
		view, operationError = loadView(database, sessionID)
		if operationError != nil {
			return operationError
		}
		if view.Deleted || view.Mutation != nil {
			return fmt.Errorf("session context is fenced for a history change")
		}
		sources, operationError = archiveMessages(database, &view, messages)
		if operationError != nil {
			return operationError
		}
		if operationError := database.Put("workspace", "state", map[string]any{"id": workspaceID}); operationError != nil {
			return operationError
		}
		return database.Put("view", string(sessionID), view)
	})
	return view, sources, operationError
}
