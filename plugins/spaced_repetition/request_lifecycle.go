package spacedrepetition

import (
	"context"
	"fmt"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (plugin *Plugin) BeginRequest(operationContext context.Context, session atom.Session) (context.Context, func(), error) {
	if operationError := plugin.validateSession(operationContext, session); operationError != nil {
		return operationContext, nil, operationError
	}
	if parent, found := operationContext.Value(requestContextKey{plugin}).(requestState); found && session.Parent != "" && parent.sessionID == session.Parent && parent.workspaceID == session.InstanceID {
		select {
		case <-parent.done:
		default:
			plugin.mutex.Lock()
			if plugin.closed {
				plugin.mutex.Unlock()
				return operationContext, nil, fmt.Errorf("spaced repetition is closed")
			}
			scope := plugin.scopeLocked(session.InstanceID)
			scope.requests++
			plugin.mutex.Unlock()
			return plugin.makeRequestLease(operationContext, session, parent.configuration, scope)
		}
	}
	for {
		if operationError := operationContext.Err(); operationError != nil {
			return operationContext, nil, operationError
		}
		plugin.configurationMutex.Lock()
		value, _, operationError := plugin.loadConfiguration(operationContext, session.InstanceID)
		if operationError != nil {
			plugin.configurationMutex.Unlock()
			return operationContext, nil, operationError
		}
		if value.Enabled && plugin.unavailable != nil {
			plugin.configurationMutex.Unlock()
			return operationContext, nil, plugin.unavailable
		}
		plugin.mutex.Lock()
		if plugin.closed {
			plugin.mutex.Unlock()
			plugin.configurationMutex.Unlock()
			return operationContext, nil, fmt.Errorf("spaced repetition is closed")
		}
		scope := plugin.scopeLocked(session.InstanceID)
		if !scope.initialized || !configurationEqual(scope.configuration, value) {
			if scope.requests > 0 {
				desired := value
				scope.pending = &desired
				changed := scope.changed
				plugin.mutex.Unlock()
				plugin.configurationMutex.Unlock()
				select {
				case <-changed:
					continue
				case <-operationContext.Done():
					return operationContext, nil, operationContext.Err()
				}
			}
			plugin.applyConfigurationLocked(scope, value)
		}
		scope.requests++
		plugin.mutex.Unlock()
		plugin.configurationMutex.Unlock()
		return plugin.makeRequestLease(operationContext, session, value, scope)
	}
}

func (plugin *Plugin) makeRequestLease(operationContext context.Context, session atom.Session, value configuration, scope *workspaceRuntime) (context.Context, func(), error) {
	done := make(chan struct{})
	var once sync.Once
	release := func() {
		once.Do(func() {
			plugin.mutex.Lock()
			defer plugin.mutex.Unlock()
			close(done)
			scope.requests--
			if scope.requests == 0 && scope.pending != nil {
				plugin.applyConfigurationLocked(scope, *scope.pending)
			}
		})
	}
	state := sessionState{}
	if plugin.database != nil {
		var operationError error
		state, operationError = plugin.database.ReadSession(operationContext, session.InstanceID, string(session.ID))
		if operationError != nil {
			release()
			return operationContext, nil, operationError
		}
		if state.Deleted || state.Fenced {
			release()
			return operationContext, nil, errSessionRetired
		}
	}
	lease := requestState{workspaceID: session.InstanceID, sessionID: session.ID, configuration: value, state: state, done: done}
	return context.WithValue(operationContext, requestContextKey{plugin}, lease), release, nil
}

func (plugin *Plugin) requestConfiguration(operationContext context.Context, workspaceID string) (configuration, error) {
	if request, found := operationContext.Value(requestContextKey{plugin}).(requestState); found && request.workspaceID == workspaceID {
		return request.configuration, nil
	}
	value, _, operationError := plugin.loadConfiguration(operationContext, workspaceID)
	return value, operationError
}
