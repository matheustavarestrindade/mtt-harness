package sidekick

import (
	"context"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (plugin *Plugin) BeginRequest(operationContext context.Context, session atom.Session) (context.Context, func(), error) {
	value, _, operationError := plugin.loadConfiguration(operationContext, session.InstanceID)
	if operationError != nil {
		plugin.recordError(session.InstanceID, operationError)
		value = defaultConfiguration()
	}
	plugin.mutex.Lock()
	if plugin.closed {
		plugin.mutex.Unlock()
		return operationContext, func() {}, nil
	}
	scope := plugin.scopeLocked(session.InstanceID)
	if !scope.initialized {
		scope.configuration = value
		scope.initialized = true
	}
	if !configurationEqual(scope.configuration, value) {
		if scope.requests == 0 {
			scope.configuration = value
			scope.pending = nil
		} else {
			copy := value
			scope.pending = &copy
		}
	}
	value = scope.configuration
	scope.requests++
	plugin.mutex.Unlock()
	state := sessionState{}
	if plugin.database != nil {
		state, operationError = plugin.database.ReadSession(operationContext, session.InstanceID, string(session.ID))
		if operationError != nil {
			plugin.recordError(session.InstanceID, operationError)
			value.Enabled = false
		}
	}
	if plugin.unavailable != nil || state.Deleted || state.Fenced {
		value.Enabled = false
	}
	done := make(chan struct{})
	var once sync.Once
	release := func() {
		once.Do(func() {
			plugin.mutex.Lock()
			defer plugin.mutex.Unlock()
			close(done)
			scope.requests--
			if scope.requests == 0 && scope.pending != nil {
				scope.configuration = *scope.pending
				scope.pending = nil
			}
			plugin.signalChangedLocked()
		})
	}
	lease := requestState{session: session, configuration: value, state: state, done: done}
	if value.Enabled {
		plugin.mutex.Lock()
		delete(plugin.paused, session.ID)
		plugin.mutex.Unlock()
		task, readError := plugin.services.Tasks.ReadTaskState(operationContext, session.InstanceID, session.ID)
		if readError != nil {
			plugin.recordError(session.InstanceID, readError)
		} else {
			plugin.observeTaskState(session, task)
		}
	}
	return context.WithValue(operationContext, requestContextKey{plugin}, lease), release, nil
}

func (plugin *Plugin) requestConfiguration(operationContext context.Context, workspaceID string) (configuration, error) {
	if lease, found := operationContext.Value(requestContextKey{plugin}).(requestState); found && lease.session.InstanceID == workspaceID {
		return lease.configuration, nil
	}
	value, _, operationError := plugin.loadConfiguration(operationContext, workspaceID)
	return value, operationError
}
