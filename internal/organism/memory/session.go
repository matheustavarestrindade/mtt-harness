package memory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

type Memory struct {
	instanceID string
	sessions   store.SessionStore
}

func New(instanceID string, sessions store.SessionStore) *Memory {
	return &Memory{instanceID: instanceID, sessions: sessions}
}

func (sessionMemory *Memory) Start(operationContext context.Context, parent atom.SessionID) (atom.SessionID, error) {
	sessionID := atom.SessionID(newID())
	depth := 0
	if parent != "" {
		session, operationError := sessionMemory.sessions.Get(operationContext, parent)
		if operationError != nil {
			return "", operationError
		}
		if session.InstanceID != sessionMemory.instanceID {
			return "", fmt.Errorf("parent session belongs to another instance")
		}
		depth = session.Depth + 1
	}
	session := atom.Session{
		ID:         sessionID,
		InstanceID: sessionMemory.instanceID,
		Parent:     parent,
		Depth:      depth,
		CreatedAt:  time.Now(),
	}
	if operationError := sessionMemory.sessions.Save(operationContext, session); operationError != nil {
		return "", operationError
	}
	return sessionID, nil
}

func (sessionMemory *Memory) Get(operationContext context.Context, sessionID atom.SessionID) (atom.Session, bool) {
	session, operationError := sessionMemory.sessions.Get(operationContext, sessionID)
	return session, operationError == nil && session.InstanceID == sessionMemory.instanceID
}

func (sessionMemory *Memory) Agents(operationContext context.Context, parent atom.SessionID) ([]atom.SessionID, error) {
	sessions, operationError := sessionMemory.sessions.List(operationContext, sessionMemory.instanceID)
	if operationError != nil {
		return nil, operationError
	}
	var identifiers []atom.SessionID
	for _, session := range sessions {
		if session.Parent == parent {
			identifiers = append(identifiers, session.ID)
		}
	}
	return identifiers, nil
}

func newID() string {
	var data [16]byte
	if _, operationError := rand.Read(data[:]); operationError != nil {
		return "unknown"
	}
	return hex.EncodeToString(data[:])
}
