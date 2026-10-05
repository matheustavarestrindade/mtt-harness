package memory

import (
	"context"
	"errors"
	"sort"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

type sessions struct{ store *Store }

func (sessionStore *sessions) GetModelSelection(operationContext context.Context, sessionID atom.SessionID) (atom.SessionModelSelection, bool, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return atom.SessionModelSelection{}, false, operationError
	}
	sessionStore.store.mutex.RLock()
	defer sessionStore.store.mutex.RUnlock()
	if sessionStore.store.deletedSessions[sessionID] {
		return atom.SessionModelSelection{}, false, store.ErrSessionDeleted
	}
	session, found := sessionStore.store.sessions[sessionID]
	return session.ModelSelection(), found, nil
}

func (sessionStore *sessions) Save(operationContext context.Context, session atom.Session) error {
	sessionStore.store.mutex.Lock()
	defer sessionStore.store.mutex.Unlock()
	if operationError := operationContext.Err(); operationError != nil {
		return operationError
	}
	if sessionStore.store.deletedSessions[session.ID] || sessionStore.store.deletedSessions[session.Parent] {
		return store.ErrSessionDeleted
	}
	if stored, found := sessionStore.store.sessions[session.ID]; found {
		session.Model = stored.Model
		session.ReasoningEffort = stored.ReasoningEffort
	}
	sessionStore.store.sessions[session.ID] = session
	return nil
}

func (sessionStore *sessions) SetModelSelection(operationContext context.Context, sessionID atom.SessionID, previous, next atom.SessionModelSelection) error {
	sessionStore.store.mutex.Lock()
	defer sessionStore.store.mutex.Unlock()
	if operationError := operationContext.Err(); operationError != nil {
		return operationError
	}
	session, found := sessionStore.store.sessions[sessionID]
	if !found || sessionStore.store.deletedSessions[sessionID] || session.Completed || session.ModelSelection() != previous {
		return store.ErrSessionSelectionChanged
	}
	session.Model, session.ReasoningEffort = next.Model, next.ReasoningEffort
	sessionStore.store.sessions[sessionID] = session
	return nil
}

func (sessionStore *sessions) Get(operationContext context.Context, sessionID atom.SessionID) (atom.Session, error) {
	sessionStore.store.mutex.RLock()
	defer sessionStore.store.mutex.RUnlock()
	session, found := sessionStore.store.sessions[sessionID]
	if sessionStore.store.deletedSessions[sessionID] {
		return atom.Session{}, store.ErrSessionDeleted
	}
	if !found {
		return atom.Session{}, store.ErrSessionNotFound
	}
	return session, nil
}

func (sessionStore *sessions) Agents(operationContext context.Context, parent atom.SessionID) ([]atom.SessionID, error) {
	sessionStore.store.mutex.RLock()
	defer sessionStore.store.mutex.RUnlock()
	var list []atom.SessionID
	for sessionID, session := range sessionStore.store.sessions {
		if session.Parent == parent && !sessionStore.store.deletedSessions[sessionID] {
			list = append(list, sessionID)
		}
	}
	sort.Slice(list, func(firstIndex, secondIndex int) bool {
		return list[firstIndex] < list[secondIndex]
	})
	return list, nil
}

func (sessionStore *sessions) Append(operationContext context.Context, message atom.Message) error {
	sessionStore.store.mutex.Lock()
	defer sessionStore.store.mutex.Unlock()
	if operationError := operationContext.Err(); operationError != nil {
		return operationError
	}
	if sessionStore.store.deletedSessions[message.SessionID] {
		return store.ErrSessionDeleted
	}
	sessionStore.store.messages[message.SessionID] = append(sessionStore.store.messages[message.SessionID], message)
	return nil
}

func (sessionStore *sessions) List(operationContext context.Context, instanceID string) ([]atom.Session, error) {
	sessionStore.store.mutex.RLock()
	defer sessionStore.store.mutex.RUnlock()
	var sessions []atom.Session
	for _, session := range sessionStore.store.sessions {
		if session.InstanceID == instanceID && !sessionStore.store.deletedSessions[session.ID] {
			sessions = append(sessions, session)
		}
	}
	sort.Slice(sessions, func(first, second int) bool {
		return sessions[first].ID < sessions[second].ID
	})
	return sessions, operationContext.Err()
}

func (sessionStore *sessions) DeleteAfter(operationContext context.Context, sessionID atom.SessionID, messageID string) (int, error) {
	sessionStore.store.mutex.Lock()
	defer sessionStore.store.mutex.Unlock()
	list := sessionStore.store.messages[sessionID]
	index := -1
	for position := range list {
		if list[position].ID == messageID {
			index = position
			break
		}
	}
	if index < 0 {
		return 0, errors.New("memory: the message is not in the store")
	}
	removed := len(list) - index - 1
	sessionStore.store.messages[sessionID] = list[:index+1]
	return removed, nil
}

func (sessionStore *sessions) Messages(operationContext context.Context, sessionID atom.SessionID) ([]atom.Message, error) {
	sessionStore.store.mutex.RLock()
	defer sessionStore.store.mutex.RUnlock()
	return append([]atom.Message(nil), sessionStore.store.messages[sessionID]...), nil
}
