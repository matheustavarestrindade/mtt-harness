package memory

import (
	"context"
	"errors"
	"sort"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type sessions struct{ store *Store }

func (sessionStore *sessions) Save(operationContext context.Context, session atom.Session) error {
	sessionStore.store.mutex.Lock()
	defer sessionStore.store.mutex.Unlock()
	sessionStore.store.sessions[session.ID] = session
	return nil
}

func (sessionStore *sessions) Get(operationContext context.Context, sessionID atom.SessionID) (atom.Session, error) {
	sessionStore.store.mutex.RLock()
	defer sessionStore.store.mutex.RUnlock()
	session, found := sessionStore.store.sessions[sessionID]
	if !found {
		return atom.Session{}, errors.New("memory: the session is not in the store")
	}
	return session, nil
}

func (sessionStore *sessions) Agents(operationContext context.Context, parent atom.SessionID) ([]atom.SessionID, error) {
	sessionStore.store.mutex.RLock()
	defer sessionStore.store.mutex.RUnlock()
	var list []atom.SessionID
	for sessionID, session := range sessionStore.store.sessions {
		if session.Parent == parent {
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
	sessionStore.store.messages[message.SessionID] = append(sessionStore.store.messages[message.SessionID], message)
	return nil
}

func (sessionStore *sessions) List(operationContext context.Context, instanceID string) ([]atom.Session, error) {
	sessionStore.store.mutex.RLock()
	defer sessionStore.store.mutex.RUnlock()
	var sessions []atom.Session
	for _, session := range sessionStore.store.sessions {
		if session.InstanceID == instanceID {
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
