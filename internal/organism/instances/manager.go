package instances

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

const (
	defaultAgentDepthLimit = 2
	defaultProcessLimit    = 8
)

type SessionManager interface {
	Start(ctx context.Context, parent atom.SessionID) (atom.SessionID, error)
	Get(ctx context.Context, id atom.SessionID) (atom.Session, bool)
	Agents(ctx context.Context, parent atom.SessionID) ([]atom.SessionID, error)
}

type Instance struct {
	spec     atom.InstanceSpec
	sessions SessionManager
}

func (i *Instance) ID() string               { return i.spec.ID }
func (i *Instance) Workspace() string        { return i.spec.Workspace }
func (i *Instance) Sessions() SessionManager { return i.sessions }
func (i *Instance) Spec() atom.InstanceSpec  { return i.spec }

type Manager struct {
	mu          sync.RWMutex
	instances   map[string]*Instance
	newSessions func(instanceID string) SessionManager
	settings    store.SettingsStore
}

func New(newSessions func(instanceID string) SessionManager, settings store.SettingsStore) *Manager {
	return &Manager{
		instances:   map[string]*Instance{},
		newSessions: newSessions,
		settings:    settings,
	}
}

func (m *Manager) AgentDepthLimit(ctx context.Context, instanceID string) int {
	if instance, ok := m.Get(instanceID); ok && instance.Spec().AgentDepthLimit > 0 {
		return instance.Spec().AgentDepthLimit
	}
	return m.limit(ctx, instanceID, "agent_depth_limit", defaultAgentDepthLimit)
}

func (m *Manager) ProcessLimit(ctx context.Context, instanceID string) int {
	if instance, ok := m.Get(instanceID); ok && instance.Spec().ProcessLimit > 0 {
		return instance.Spec().ProcessLimit
	}
	return m.limit(ctx, instanceID, "process_limit", defaultProcessLimit)
}

func (m *Manager) limit(ctx context.Context, instanceID string, key string, fallback int) int {
	if m.settings == nil {
		return fallback
	}
	value, err := m.settings.Resolve(ctx, instanceID, key)
	if err != nil || value == "" {
		return fallback
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return number
}

func (m *Manager) Start(ctx context.Context, spec atom.InstanceSpec) (*Instance, error) {
	if spec.Workspace == "" {
		return nil, errors.New("instances: the workspace is necessary")
	}
	if spec.ID == "" {
		spec.ID = newID()
	}
	instance := &Instance{spec: spec, sessions: m.newSessions(spec.ID)}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.instances[spec.ID] = instance
	return instance, nil
}

func (m *Manager) Get(id string) (*Instance, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	instance, ok := m.instances[id]
	return instance, ok
}

func (m *Manager) All() []*Instance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]*Instance, 0, len(m.instances))
	for _, instance := range m.instances {
		list = append(list, instance)
	}
	return list
}

func (m *Manager) Stop(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.instances[id]; !ok {
		return errors.New("instances: the instance is not in the manager")
	}
	delete(m.instances, id)
	return nil
}

func newID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(data[:])
}
