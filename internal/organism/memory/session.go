package memory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

func (m *Memory) Start(ctx context.Context, parent atom.SessionID) (atom.SessionID, error) {
	id := atom.SessionID(newID())
	depth := 0
	if parent != "" {
		if session, err := m.sessions.Get(ctx, parent); err == nil {
			depth = session.Depth + 1
		}
	}
	session := atom.Session{
		ID:         id,
		InstanceID: m.instanceID,
		Parent:     parent,
		Depth:      depth,
		CreatedAt:  time.Now(),
	}
	if err := m.sessions.Save(ctx, session); err != nil {
		return "", err
	}
	return id, nil
}

func (m *Memory) Get(ctx context.Context, id atom.SessionID) (atom.Session, bool) {
	session, err := m.sessions.Get(ctx, id)
	return session, err == nil
}

func (m *Memory) Agents(ctx context.Context, parent atom.SessionID) ([]atom.SessionID, error) {
	return m.sessions.Agents(ctx, parent)
}

func newID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(data[:])
}
