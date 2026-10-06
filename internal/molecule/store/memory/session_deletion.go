package memory

import (
	"context"
	"sort"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

// The store lock makes cleanup and the stale-writer fence one operation.
func (sessionStore *sessions) DeleteConversation(operationContext context.Context, sessionID atom.SessionID) ([]atom.SessionID, error) {
	database := sessionStore.store
	database.mutex.Lock()
	defer database.mutex.Unlock()
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	root, found := database.sessions[sessionID]
	if !found {
		return nil, store.ErrSessionNotFound
	}
	if database.deletedSessions[sessionID] {
		return nil, store.ErrSessionDeleted
	}
	selected := map[atom.SessionID]bool{sessionID: true}
	for changed := true; changed; {
		changed = false
		for identifier, session := range database.sessions {
			if session.InstanceID == root.InstanceID && selected[session.Parent] && !selected[identifier] {
				selected[identifier], changed = true, true
			}
		}
	}
	for _, entry := range database.queued {
		if selected[entry.Message.SessionID] {
			return nil, store.ErrConversationBusy
		}
	}
	for _, process := range database.processes {
		if selected[process.SessionID] && process.Status == "running" {
			return nil, store.ErrConversationBusy
		}
	}
	var deleted []atom.SessionID
	for identifier := range selected {
		if !database.deletedSessions[identifier] {
			deleted = append(deleted, identifier)
		}
		database.deletedSessions[identifier] = true
		session := database.sessions[identifier]
		session.Model, session.ReasoningEffort, session.Completed = "", "", true
		database.sessions[identifier] = session
		delete(database.messages, identifier)
		delete(database.taskStates, identifier)
	}
	remainingEvents := make([]atom.Event, 0, len(database.events))
	for _, event := range database.events {
		if !selected[event.SessionID] {
			remainingEvents = append(remainingEvents, event)
		}
	}
	database.events = remainingEvents
	for identifier, process := range database.processes {
		if selected[process.SessionID] {
			delete(database.processes, identifier)
		}
	}
	for identifier, decision := range database.permissions {
		if !selected[decision.SessionID] {
			continue
		}
		if decision.Scope == atom.ScopeAlways {
			decision.SessionID = ""
			database.permissions[identifier] = decision
			continue
		}
		delete(database.permissions, identifier)
	}
	sort.Slice(deleted, func(first, second int) bool { return deleted[first] < deleted[second] })
	return deleted, nil
}
