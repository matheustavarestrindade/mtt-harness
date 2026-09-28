package instances

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	Start(operationContext context.Context, parent atom.SessionID) (atom.SessionID, error)
	Get(operationContext context.Context, sessionID atom.SessionID) (atom.Session, bool)
	Agents(operationContext context.Context, parent atom.SessionID) ([]atom.SessionID, error)
}

type Instance struct {
	instanceSpec atom.InstanceSpec
	sessions     SessionManager
}

func (instance *Instance) ID() string {
	return instance.instanceSpec.ID
}
func (instance *Instance) Workspace() string {
	return instance.instanceSpec.Workspace
}
func (instance *Instance) Sessions() SessionManager {
	return instance.sessions
}
func (instance *Instance) Spec() atom.InstanceSpec {
	return instance.instanceSpec
}

type Manager struct {
	mutex       sync.RWMutex
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

func (instanceManager *Manager) AgentDepthLimit(operationContext context.Context, instanceID string) (int, error) {
	fallback := defaultAgentDepthLimit
	if instance, found := instanceManager.Get(instanceID); found && instance.Spec().AgentDepthLimit > 0 {
		fallback = instance.Spec().AgentDepthLimit
	}
	return instanceManager.limit(operationContext, instanceID, "agent_depth_limit", fallback)
}

func (instanceManager *Manager) ProcessLimit(operationContext context.Context, instanceID string) (int, error) {
	fallback := defaultProcessLimit
	if instance, found := instanceManager.Get(instanceID); found && instance.Spec().ProcessLimit > 0 {
		fallback = instance.Spec().ProcessLimit
	}
	return instanceManager.limit(operationContext, instanceID, "process_limit", fallback)
}

func (instanceManager *Manager) limit(operationContext context.Context, instanceID string, key string, fallback int) (int, error) {
	if instanceManager.settings == nil {
		return fallback, nil
	}
	value, operationError := instanceManager.settings.Resolve(operationContext, instanceID, key)
	if operationError != nil {
		return 0, operationError
	}
	if value == "" {
		return fallback, nil
	}
	number, operationError := strconv.Atoi(value)
	if operationError != nil || number < 0 {
		return 0, fmt.Errorf("setting %s must be a non-negative integer", key)
	}
	return number, nil
}

func (instanceManager *Manager) Start(operationContext context.Context, instanceSpec atom.InstanceSpec) (*Instance, error) {
	if instanceSpec.Stopped {
		return nil, fmt.Errorf("instance is stopped")
	}
	if instanceSpec.Workspace == "" {
		return nil, errors.New("instances: the workspace is necessary")
	}
	if instanceSpec.ID == "" {
		instanceSpec.ID = newID()
	}
	workspace, operationError := filepath.Abs(instanceSpec.Workspace)
	if operationError != nil {
		return nil, operationError
	}
	workspaceInfo, operationError := os.Stat(workspace)
	if operationError != nil {
		return nil, operationError
	}
	if !workspaceInfo.IsDir() {
		return nil, fmt.Errorf("workspace must be a directory")
	}
	instanceSpec.Workspace = workspace
	if instanceSpec.AgentDepthLimit < 0 || instanceSpec.ProcessLimit < 0 {
		return nil, fmt.Errorf("limits cannot be negative")
	}
	if instanceManager.settings != nil {
		for key, limit := range map[string]int{"agent_depth_limit": instanceSpec.AgentDepthLimit, "process_limit": instanceSpec.ProcessLimit} {
			if limit == 0 {
				continue
			}
			value, operationError := instanceManager.settings.Get(operationContext, instanceSpec.ID, key)
			if operationError != nil {
				return nil, operationError
			}
			if value != "" {
				continue
			}
			if operationError := instanceManager.settings.Save(operationContext, instanceSpec.ID, key, strconv.Itoa(limit)); operationError != nil {
				return nil, operationError
			}
		}
		instanceSpec.AgentDepthLimit, instanceSpec.ProcessLimit = 0, 0
	}
	instance := &Instance{instanceSpec: instanceSpec, sessions: instanceManager.newSessions(instanceSpec.ID)}
	instanceManager.mutex.Lock()
	defer instanceManager.mutex.Unlock()
	instanceManager.instances[instanceSpec.ID] = instance
	return instance, nil
}

func (instanceManager *Manager) Get(identifier string) (*Instance, bool) {
	instanceManager.mutex.RLock()
	defer instanceManager.mutex.RUnlock()
	instance, found := instanceManager.instances[identifier]
	return instance, found
}

func (instanceManager *Manager) IsRunning(identifier string) bool {
	_, found := instanceManager.Get(identifier)
	return found
}

func (instanceManager *Manager) All() []*Instance {
	instanceManager.mutex.RLock()
	defer instanceManager.mutex.RUnlock()
	list := make([]*Instance, 0, len(instanceManager.instances))
	for _, instance := range instanceManager.instances {
		list = append(list, instance)
	}
	return list
}

func (instanceManager *Manager) Stop(operationContext context.Context, identifier string) error {
	instanceManager.mutex.Lock()
	defer instanceManager.mutex.Unlock()
	if _, found := instanceManager.instances[identifier]; !found {
		return errors.New("instances: the instance is not in the manager")
	}
	delete(instanceManager.instances, identifier)
	return nil
}

func newID() string {
	var data [16]byte
	if _, operationError := rand.Read(data[:]); operationError != nil {
		return "unknown"
	}
	return hex.EncodeToString(data[:])
}
