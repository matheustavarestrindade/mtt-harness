package memory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sort"
	"sync"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type Memory struct {
	mu         sync.RWMutex
	instanceID string
	sessions   map[atom.SessionID]atom.Session
}

func New(instanceID string) *Memory {
	return &Memory{
		instanceID: instanceID,
		sessions:   map[atom.SessionID]atom.Session{},
	}
}

func (m *Memory) Start(ctx context.Context, parent atom.SessionID) (atom.SessionID, error) {
	id := atom.SessionID(newID())
	depth := 0
	if parent != "" {
		if session, ok := m.Get(ctx, parent); ok {
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
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[id] = session
	return id, nil
}

func (m *Memory) Get(ctx context.Context, id atom.SessionID) (atom.Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[id]
	return session, ok
}

func (m *Memory) Agents(ctx context.Context, parent atom.SessionID) ([]atom.SessionID, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []atom.SessionID
	for id, session := range m.sessions {
		if session.Parent == parent {
			list = append(list, id)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i] < list[j] })
	return list, nil
}

func newID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(data[:])
}
