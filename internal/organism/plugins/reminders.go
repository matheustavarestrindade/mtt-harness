package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (pluginHost *Host) ProjectReminders(operationContext context.Context, session atom.Session, request atom.Request) (atom.Request, error) {
	for _, registered := range pluginHost.All() {
		plugin, available := registered.(harness.RequestReminderPlugin)
		if !available {
			continue
		}
		fields, operationError := requestInvariants(request)
		if operationError != nil {
			return request, operationError
		}
		messages, operationError := reminderMessageSnapshots(request.Messages)
		if operationError != nil {
			return request, operationError
		}
		next, operationError := plugin.ProjectReminders(operationContext, session, request)
		if operationError != nil {
			return request, operationError
		}
		after, operationError := requestInvariants(next)
		if operationError != nil {
			return request, operationError
		}
		if !bytes.Equal(fields, after) {
			return request, fmt.Errorf("plugin %q changed model request fields outside reminder projection", registered.Name())
		}
		if operationError := validateReminderProjection(registered.Name(), messages, next.Messages); operationError != nil {
			return request, operationError
		}
		request = next
	}
	return request, nil
}

func (pluginHost *Host) PrepareReminders(operationContext context.Context, input harness.ContextRequest) (harness.ReminderPlan, error) {
	projected, operationError := pluginHost.ProjectReminders(operationContext, input.Session, input.Request)
	if operationError != nil {
		return harness.ReminderPlan{}, operationError
	}
	plan := harness.ReminderPlan{Request: projected}
	var commits []func(context.Context) error
	for _, registered := range pluginHost.All() {
		plugin, available := registered.(harness.RequestReminderPlugin)
		if !available {
			continue
		}
		originalFields, operationError := requestInvariants(input.Request)
		if operationError != nil {
			return plan, operationError
		}
		originalMessages, operationError := reminderMessageSnapshots(input.Request.Messages)
		if operationError != nil {
			return plan, operationError
		}
		contribution, operationError := plugin.PrepareReminder(operationContext, input)
		if operationError != nil {
			return plan, operationError
		}
		afterFields, operationError := requestInvariants(input.Request)
		if operationError != nil {
			return plan, operationError
		}
		afterMessages, operationError := reminderMessageSnapshots(input.Request.Messages)
		if operationError != nil {
			return plan, operationError
		}
		if !bytes.Equal(originalFields, afterFields) || !equalMessageSnapshots(originalMessages, afterMessages) {
			return plan, fmt.Errorf("plugin %q changed the request while preparing a reminder", registered.Name())
		}
		if len(contribution.Messages) == 0 {
			continue
		}
		before, operationError := input.Measure(operationContext, plan.Request)
		if operationError != nil {
			return plan, operationError
		}
		candidate := plan.Request
		candidate.Messages = append(append([]atom.Message(nil), plan.Request.Messages...), contribution.Messages...)
		priorMessages, operationError := reminderMessageSnapshots(plan.Request.Messages)
		if operationError != nil {
			return plan, operationError
		}
		if operationError := validateReminderProjection(registered.Name(), priorMessages, candidate.Messages); operationError != nil {
			return plan, operationError
		}
		after, operationError := input.Measure(operationContext, candidate)
		if operationError != nil {
			return plan, operationError
		}
		ceiling := after.InputLimit
		if input.InputCeiling > 0 && (ceiling == 0 || input.InputCeiling < ceiling) {
			ceiling = input.InputCeiling
		}
		if ceiling > 0 && after.InputTokens > ceiling {
			if contribution.Deferred != nil {
				if operationError := contribution.Deferred(operationContext); operationError != nil {
					return plan, operationError
				}
			}
			continue
		}
		plan.Request = candidate
		if contribution.Commit != nil {
			delivery := harness.ReminderDelivery{Tokens: max(0, after.InputTokens-before.InputTokens), Estimated: after.Estimated || before.Estimated}
			commits = append(commits, func(commitContext context.Context) error { return contribution.Commit(commitContext, delivery) })
		}
	}
	plan.Commit = func(commitContext context.Context) error {
		for _, commit := range commits {
			if operationError := commit(commitContext); operationError != nil {
				return operationError
			}
		}
		return nil
	}
	return plan, nil
}

func reminderMessageSnapshots(messages []atom.Message) ([][]byte, error) {
	result := make([][]byte, 0, len(messages))
	for _, message := range messages {
		encoded, operationError := snapshotReminderMessage(message)
		if operationError != nil {
			return nil, operationError
		}
		result = append(result, encoded)
	}
	return result, nil
}

func snapshotReminderMessage(message atom.Message) ([]byte, error) {
	return json.Marshal(struct {
		Message   atom.Message
		Private   *atom.ProviderState
		Ephemeral bool
		InContext bool
	}{message, message.ProviderState, message.Ephemeral, message.InContext})
}

func validateReminderProjection(name string, original [][]byte, messages []atom.Message) error {
	position := 0
	identifiers := map[string]bool{}
	for _, message := range messages {
		encoded, operationError := snapshotReminderMessage(message)
		if operationError != nil {
			return operationError
		}
		if message.ID != "" {
			if identifiers[message.ID] {
				return fmt.Errorf("plugin %q repeated message %q", name, message.ID)
			}
			identifiers[message.ID] = true
		}
		if position < len(original) && bytes.Equal(original[position], encoded) {
			position++
			continue
		}
		if operationError := validateReminderMessage(name, message); operationError != nil {
			return operationError
		}
	}
	if position != len(original) {
		return fmt.Errorf("plugin %q removed or changed existing request messages", name)
	}
	return nil
}

func validateReminderMessage(name string, message atom.Message) error {
	if !message.Ephemeral || !strings.HasPrefix(message.ID, name+":") || len(message.ToolCalls) > 0 || message.ToolCallID != "" || message.ProviderState != nil {
		return fmt.Errorf("plugin %q returned invalid reminder metadata", name)
	}
	if message.Role != atom.RoleRuntime && (message.Role != atom.RoleSystem || !message.InContext) {
		return fmt.Errorf("plugin %q returned a reminder outside in-context instructions or runtime data", name)
	}
	for _, content := range message.Content {
		if content.Type != atom.Text {
			return fmt.Errorf("plugin %q returned non-text reminder content", name)
		}
	}
	return nil
}

func equalMessageSnapshots(first, second [][]byte) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if !bytes.Equal(first[index], second[index]) {
			return false
		}
	}
	return true
}
