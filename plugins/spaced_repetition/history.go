package spacedrepetition

import (
	"context"
	"errors"
	"slices"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

// BeginHistoryChange fences workers before the core mutation. A failed core
// change releases the fence but keeps the higher epoch, rejecting stale work.
func (plugin *Plugin) BeginHistoryChange(operationContext context.Context, change harness.HistoryChange) (func(context.Context, bool) error, error) {
	if plugin.database == nil {
		return func(context.Context, bool) error { return nil }, nil
	}
	type fencedSession struct {
		session  atom.Session
		previous sessionState
		epoch    int64
		retained map[string]bool
	}
	var fenced []fencedSession
	complete := func(completionContext context.Context, committed bool) error {
		var result error
		for _, item := range fenced {
			_, operationError := plugin.database.UpdateSession(completionContext, item.session.InstanceID, string(item.session.ID), func(state *sessionState) (map[string]int64, error) {
				if state.Epoch != item.epoch {
					return nil, errSessionRetired
				}
				state.Fenced = false
				state.Pending = nil
				if !committed {
					state.Deleted = item.previous.Deleted
					return nil, nil
				}
				if change.Kind == harness.HistoryDelete {
					*state = sessionState{Epoch: item.epoch, Deleted: true}
					return nil, nil
				}
				state.Events = slices.DeleteFunc(state.Events, func(event reminderEvent) bool { return !item.retained[event.Anchor] })
				// A reverted branch starts measuring at its retained view. Earlier
				// reminders remain anchored, but an abandoned branch cannot leak phase.
				cursor := int64(0)
				if len(state.Events) > 0 {
					cursor = state.Events[len(state.Events)-1].CursorAfter
				}
				state.Schedule = scheduleState{Initialized: true, Cursor: cursor}
				state.SourceUser = ""
				return nil, nil
			})
			result = errors.Join(result, operationError)
			if committed {
				result = errors.Join(result, plugin.database.DeleteRecoveries(completionContext, item.session.InstanceID, string(item.session.ID)))
			}
		}
		return result
	}
	for _, snapshot := range change.Snapshots {
		var previous sessionState
		state, operationError := plugin.database.UpdateSession(operationContext, snapshot.Session.InstanceID, string(snapshot.Session.ID), func(state *sessionState) (map[string]int64, error) {
			previous = *state
			state.Epoch++
			state.Fenced = true
			state.Pending = nil
			return nil, nil
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
		fenced = append(fenced, fencedSession{session: snapshot.Session, previous: previous, epoch: state.Epoch, retained: retained})
		plugin.mutex.Lock()
		if scope := plugin.scopes[snapshot.Session.InstanceID]; scope != nil {
			if cancel := scope.jobs[string(snapshot.Session.ID)]; cancel != nil {
				cancel()
			}
		}
		plugin.mutex.Unlock()
	}
	return complete, nil
}

func (plugin *Plugin) EndTurn(operationContext context.Context, session atom.Session, status string) error {
	if plugin.database == nil || status == "completed" {
		return nil
	}
	plugin.mutex.Lock()
	if scope := plugin.scopes[session.InstanceID]; scope != nil {
		if cancel := scope.jobs[string(session.ID)]; cancel != nil {
			cancel()
		}
	}
	plugin.mutex.Unlock()
	_, operationError := plugin.database.UpdateSession(operationContext, session.InstanceID, string(session.ID), func(state *sessionState) (map[string]int64, error) {
		if state.Deleted {
			return nil, nil
		}
		state.Epoch++
		state.Pending = nil
		return nil, nil
	})
	return operationError
}
