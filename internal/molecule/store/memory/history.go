package memory

import (
	"context"
	"slices"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

func (sessionStore *sessions) History(operationContext context.Context, sessionID atom.SessionID, query store.HistoryQuery) (atom.HistoryPage, error) {
	page := atom.HistoryPage{SessionID: sessionID, Messages: []atom.Message{}, Context: []atom.Message{}, Models: map[string]string{}, Sizes: map[string]int{}}
	query, operationError := query.Validate()
	if operationError != nil {
		return page, operationError
	}
	if operationError = operationContext.Err(); operationError != nil {
		return page, operationError
	}
	database := sessionStore.store
	database.mutex.RLock()
	defer database.mutex.RUnlock()
	session, found := database.sessions[sessionID]
	if !found || database.deletedSessions[sessionID] {
		return page, store.ErrSessionNotFound
	}
	page.Revision = session.HistoryRevision
	if query.Revision != nil && *query.Revision != page.Revision {
		return page, store.ErrHistoryChanged
	}
	messages := database.messages[sessionID]
	if session.Parent != "" {
		for _, message := range messages {
			if message.Role == atom.RoleUser {
				page.TaskMessageID = message.ID
				page.TaskSeq = database.messageSequences[message.ID]
				for _, content := range message.Content {
					if content.Type == atom.Text {
						page.Task = content.Text
						if len([]rune(page.Task)) > 4096 {
							page.Task = string([]rune(page.Task)[:4096]) + "…"
						}
						break
					}
				}
				break
			}
		}
	}
	if len(messages) > 0 {
		page.HeadSeq = database.messageSequences[messages[len(messages)-1].ID]
	}
	models := map[string]string{}
	for _, usage := range database.usage {
		if usage.SessionID == sessionID && usage.RequestID != "" {
			models[usage.RequestID] = usage.ModelID
		}
	}
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message.Role == atom.RoleAssistant {
			page.LatestResponse = &atom.ResponseUsageSnapshot{MessageID: message.ID, Seq: database.messageSequences[message.ID], Model: models[message.ID], Usage: message.Usage, CreatedAt: message.CreatedAt}
			break
		}
	}
	if query.MetadataOnly {
		return page, nil
	}
	start, step := len(messages)-1, -1
	if query.After > 0 {
		start, step = 0, 1
	}
	bytes := 0
	for index := start; index >= 0 && index < len(messages); index += step {
		message := messages[index]
		message.Seq = database.messageSequences[message.ID]
		message.ProviderState = nil
		if query.Before > 0 && message.Seq >= query.Before || query.After > 0 && message.Seq <= query.After {
			continue
		}
		size := store.PublicMessageSize(message)
		if len(page.Messages) > 0 && (len(page.Messages) >= query.Limit || bytes+size > store.HistoryPageBytes) {
			break
		}
		page.Messages = append(page.Messages, message)
		page.Sizes[message.ID] = size
		bytes += size
		if model := models[message.ID]; model != "" {
			page.Models[message.ID] = model
		}
	}
	if step < 0 {
		slices.Reverse(page.Messages)
	}
	if len(page.Messages) == 0 {
		return page, nil
	}
	page.FirstSeq = page.Messages[0].Seq
	page.LastSeq = page.Messages[len(page.Messages)-1].Seq
	page.HasOlder = database.messageSequences[messages[0].ID] < page.FirstSeq
	page.HasNewer = page.HeadSeq > page.LastSeq
	if page.Messages[0].Role == atom.RoleTool {
		for index := len(messages) - 1; index >= 0; index-- {
			owner := messages[index]
			owner.Seq = database.messageSequences[owner.ID]
			if owner.Seq >= page.FirstSeq || owner.Role != atom.RoleAssistant {
				continue
			}
			owner = store.BoundaryToolOwner(owner, page.Messages)
			if len(owner.ToolCalls) > 0 {
				page.Context = append(page.Context, owner)
				page.Sizes[owner.ID] = store.PublicMessageSize(owner)
			}
			break
		}
	}
	return page, nil
}
