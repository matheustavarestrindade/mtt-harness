package watcher

import (
	"context"
	"errors"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (plugin *Plugin) BeginHistoryChange(operationContext context.Context, change harness.HistoryChange) (func(context.Context, bool) error, error) {
	if plugin.database == nil {
		return func(context.Context, bool) error { return nil }, nil
	}
	retained := map[atom.SessionID][]string{}
	workspaces := map[atom.SessionID]string{}
	var waits []<-chan struct{}
	plugin.mutex.Lock()
	for _, snapshot := range change.Snapshots {
		if snapshot.Session.Execution != nil {
			continue
		}
		identifier := snapshot.Session.ID
		plugin.paused[identifier] = true
		workspaces[identifier] = snapshot.Session.InstanceID
		for _, message := range snapshot.Messages {
			retained[identifier] = append(retained[identifier], message.ID)
			if message.ID == change.KeepMessageID {
				break
			}
		}
		for key, value := range plugin.inbox {
			if value.Session.ID == identifier {
				delete(plugin.inbox, key)
			}
		}
		for _, group := range plugin.active {
			if group.sessionID == identifier {
				group.cancel()
				waits = append(waits, group.done)
			}
		}
	}
	plugin.compactInboxLocked()
	plugin.mutex.Unlock()
	complete := func(completionContext context.Context, committed bool) error {
		var result error
		for identifier, workspace := range workspaces {
			if committed {
				result = errors.Join(result, plugin.database.RetireHistory(completionContext, workspace, string(identifier), change.Kind == harness.HistoryDelete, retained[identifier]))
			}
			plugin.mutex.Lock()
			delete(plugin.paused, identifier)
			plugin.mutex.Unlock()
		}
		return result
	}
	for _, done := range waits {
		select {
		case <-done:
		case <-operationContext.Done():
			return nil, errors.Join(operationContext.Err(), complete(context.WithoutCancel(operationContext), false))
		}
	}
	return complete, nil
}
