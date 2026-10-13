package sidekick

import (
	"context"
	"slices"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func hintMessage(session atom.Session, event hintEvent) atom.Message {
	return atom.Message{ID: pluginName + ":" + event.ID, SessionID: session.ID, Role: atom.RoleRuntime, Ephemeral: true, Content: []atom.Content{{Type: atom.Text, Text: event.Text}}}
}

// ProjectReminders only replays frozen insertions, preserving existing message
// bytes and anchors while the fitter explores candidate context views.
func (plugin *Plugin) ProjectReminders(operationContext context.Context, session atom.Session, request atom.Request) (atom.Request, error) {
	lease, found := operationContext.Value(requestContextKey{plugin}).(requestState)
	if !found || lease.session.ID != session.ID || lease.session.InstanceID != session.InstanceID || lease.state.Deleted || lease.state.Fenced {
		return request, nil
	}
	present := map[string]bool{}
	for _, message := range request.Messages {
		present[message.ID] = true
	}
	anchors := map[string][]hintEvent{}
	for _, event := range lease.state.Events {
		if !present[pluginName+":"+event.ID] {
			anchors[event.Anchor] = append(anchors[event.Anchor], event)
		}
	}
	result := make([]atom.Message, 0, len(request.Messages)+len(lease.state.Events))
	for _, message := range request.Messages {
		result = append(result, message)
		if message.Ephemeral {
			continue
		}
		for _, event := range anchors[message.ID] {
			result = append(result, hintMessage(session, event))
		}
	}
	request.Messages = result
	return request, operationContext.Err()
}

func (plugin *Plugin) PrepareReminder(operationContext context.Context, input harness.ContextRequest) (harness.ReminderContribution, error) {
	configuration, operationError := plugin.requestConfiguration(operationContext, input.Session.InstanceID)
	if operationError != nil {
		plugin.recordError(input.Session.InstanceID, operationError)
		return harness.ReminderContribution{}, nil
	}
	if !configuration.Enabled || plugin.database == nil {
		return harness.ReminderContribution{}, nil
	}
	task, operationError := plugin.services.Tasks.ReadTaskState(operationContext, input.Session.InstanceID, input.Session.ID)
	if operationError != nil {
		plugin.recordError(input.Session.InstanceID, operationError)
		return harness.ReminderContribution{}, nil
	}
	visible := map[string]bool{}
	var anchor atom.Message
	sourceUser := ""
	for _, message := range input.Request.Messages {
		if message.Ephemeral || message.ID == "" {
			continue
		}
		visible[message.ID] = true
		anchor = message
		if message.Role == atom.RoleUser {
			sourceUser = message.ID
		}
	}
	if anchor.ID == "" {
		return harness.ReminderContribution{}, nil
	}
	plugin.captureModelView(input, sourceUser, configuration.SourceBytes)
	taskKey := doingFingerprint(task.Doing)
	configurationHash := configurationFingerprint(configuration, plugin.promptVersion)
	state, operationError := plugin.database.UpdateSession(operationContext, input.Session.InstanceID, string(input.Session.ID), func(state *sessionState) (mutation, error) {
		if state.Deleted || state.Fenced {
			return mutation{}, errRetired
		}
		changes := mutation{Counters: map[string]int64{}}
		if state.SourceUser != sourceUser || state.TaskKey != taskKey || state.ConfigurationHash != configurationHash {
			state.Epoch++
			if state.Pending != nil {
				changes.RunID = state.Pending.ID
				changes.RunStatus = "stale"
				changes.Counters["hints/stale"]++
			}
			state.Pending = nil
		}
		state.SourceUser = sourceUser
		state.TaskKey = taskKey
		state.ConfigurationHash = configurationHash
		before := len(state.Events)
		state.Events = slices.DeleteFunc(state.Events, func(event hintEvent) bool { return !visible[event.Anchor] })
		if len(state.Events) < before {
			state.LastKey = ""
			changes.Counters["hints/retired"] += int64(before - len(state.Events))
		}
		return changes, nil
	})
	if operationError != nil {
		plugin.recordError(input.Session.InstanceID, operationError)
		return harness.ReminderContribution{}, nil
	}
	if state.Pending == nil || taskKey == "" {
		return harness.ReminderContribution{}, nil
	}
	pending := *state.Pending
	valid, operationError := plugin.verifyHintSources(operationContext, input.Session, pending)
	if operationError != nil {
		plugin.recordError(input.Session.InstanceID, operationError)
	}
	if operationError != nil || !valid {
		_, writeError := plugin.database.UpdateSession(operationContext, input.Session.InstanceID, string(input.Session.ID), func(current *sessionState) (mutation, error) {
			if current.Pending == nil || current.Pending.ID != pending.ID {
				return mutation{}, nil
			}
			current.Pending = nil
			return mutation{RunID: pending.ID, RunStatus: "stale", Counters: map[string]int64{"hints/stale": 1}}, nil
		})
		plugin.recordError(input.Session.InstanceID, writeError)
		return harness.ReminderContribution{}, nil
	}
	event := hintEvent{ID: pending.ID, Anchor: anchor.ID, AnchorSequence: anchor.Seq, Text: pending.Text, Sources: pending.Sources, CreatedAt: time.Now().UTC()}
	contribution := harness.ReminderContribution{Messages: []atom.Message{hintMessage(input.Session, event)}}
	contribution.Deferred = func(deferContext context.Context) error {
		_, operationError := plugin.database.UpdateSession(deferContext, input.Session.InstanceID, string(input.Session.ID), func(current *sessionState) (mutation, error) {
			if current.Deleted || current.Fenced {
				return mutation{}, nil
			}
			return mutation{Counters: map[string]int64{"hints/deferred": 1}}, nil
		})
		return operationError
	}
	contribution.Commit = func(commitContext context.Context, delivery harness.ReminderDelivery) error {
		if operationError := plugin.validateCurrentSource(commitContext, input.Session, taskKey, sourceUser, configurationHash); operationError != nil {
			return operationError
		}
		_, operationError := plugin.database.UpdateSession(commitContext, input.Session.InstanceID, string(input.Session.ID), func(current *sessionState) (mutation, error) {
			if current.Deleted || current.Fenced || current.Epoch != state.Epoch || current.SourceUser != sourceUser || current.TaskKey != taskKey {
				return mutation{}, errRetired
			}
			for _, previous := range current.Events {
				if previous.ID == event.ID {
					return mutation{}, nil
				}
			}
			if current.Pending == nil || current.Pending.ID != pending.ID {
				return mutation{}, errRetired
			}
			current.Events = append(current.Events, event)
			current.Pending = nil
			current.LastStatus = "delivered"
			counts := map[string]int64{"hints/delivered": 1, "hints/input_tokens": int64(delivery.Tokens)}
			if delivery.Estimated {
				counts["hints/estimated_tokens"] = int64(delivery.Tokens)
			}
			return mutation{RunID: pending.ID, RunStatus: "delivered", Counters: counts}, nil
		})
		return operationError
	}
	return contribution, nil
}
