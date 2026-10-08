package sidekick

import (
	"context"
	"errors"
	"slices"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (plugin *Plugin) cancelSessionWorker(sessionID atom.SessionID) <-chan struct{} {
	plugin.mutex.Lock()
	plugin.paused[sessionID] = true
	delete(plugin.views, sessionID)
	var done <-chan struct{}
	if job := plugin.jobs[sessionID]; job != nil {
		job.cancel()
		done = job.done
	}
	plugin.signalChangedLocked()
	plugin.mutex.Unlock()
	return done
}

func (plugin *Plugin) EndTurn(operationContext context.Context, session atom.Session, status string) error {
	plugin.cancelSessionWorker(session.ID)
	if plugin.database == nil || status == "completed" {
		return nil
	}
	_, operationError := plugin.database.UpdateSession(operationContext, session.InstanceID, string(session.ID), func(state *sessionState) (mutation, error) {
		if state.Deleted {
			return mutation{}, nil
		}
		changes := mutation{}
		if state.Pending != nil {
			changes.RunID = state.Pending.ID
			changes.RunStatus = "cancelled"
		}
		state.Epoch++
		state.Pending = nil
		return changes, nil
	})
	plugin.recordError(session.InstanceID, operationError)
	return nil
}

// The epoch fence is durable before core history mutation. Completion rolls
// back only the temporary fence, never the higher epoch of abandoned work.
func (plugin *Plugin) BeginHistoryChange(operationContext context.Context, change harness.HistoryChange) (func(context.Context, bool) error, error) {
	if plugin.database == nil {
		return func(context.Context, bool) error { return nil }, nil
	}
	type fencedSession struct {
		session  atom.Session
		epoch    int64
		retained map[string]bool
		deleted  bool
	}
	fenced := []fencedSession{}
	complete := func(completionContext context.Context, committed bool) error {
		var result error
		for _, item := range fenced {
			_, operationError := plugin.database.UpdateSession(completionContext, item.session.InstanceID, string(item.session.ID), func(state *sessionState) (mutation, error) {
				if state.Epoch != item.epoch {
					return mutation{}, errRetired
				}
				state.Fenced = false
				state.Pending = nil
				if !committed {
					state.Deleted = item.deleted
					return mutation{}, nil
				}
				if change.Kind == harness.HistoryDelete {
					*state = sessionState{Epoch: item.epoch, Deleted: true}
					return mutation{PurgeRuns: true}, nil
				}
				state.Events = slices.DeleteFunc(state.Events, func(event hintEvent) bool { return !item.retained[event.Anchor] })
				state.LastKey = ""
				state.TaskKey = ""
				state.SourceUser = ""
				return mutation{PurgeRuns: true}, nil
			})
			result = errors.Join(result, operationError)
			plugin.mutex.Lock()
			delete(plugin.paused, item.session.ID)
			plugin.mutex.Unlock()
		}
		return result
	}
	var workers []<-chan struct{}
	for _, snapshot := range change.Snapshots {
		if done := plugin.cancelSessionWorker(snapshot.Session.ID); done != nil {
			workers = append(workers, done)
		}
		wasDeleted := false
		state, operationError := plugin.database.UpdateSession(operationContext, snapshot.Session.InstanceID, string(snapshot.Session.ID), func(state *sessionState) (mutation, error) {
			wasDeleted = state.Deleted
			state.Epoch++
			state.Fenced = true
			state.Pending = nil
			return mutation{}, nil
		})
		if operationError != nil {
			return nil, errors.Join(operationError, complete(context.WithoutCancel(operationContext), false))
		}
		retained := map[string]bool{}
		for _, message := range snapshot.Messages {
			retained[message.ID] = true
			if message.ID == change.KeepMessageID {
				break
			}
		}
		fenced = append(fenced, fencedSession{snapshot.Session, state.Epoch, retained, wasDeleted})
	}
	for _, done := range workers {
		select {
		case <-done:
		case <-operationContext.Done():
			return nil, errors.Join(operationContext.Err(), complete(context.WithoutCancel(operationContext), false))
		}
	}
	return complete, nil
}
