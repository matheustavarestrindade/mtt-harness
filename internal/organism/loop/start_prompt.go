package loop

import (
	"context"
	"fmt"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/startprompt"
)

// prependStartPrompt builds a request-local system message before context/request
// middleware and budgeting. It never appends to durable history, so subsequent
// tool rounds, resumed sessions and child agents each receive one fresh copy.
func (agentLoop *Loop) prependStartPrompt(operationContext context.Context, session atom.Session, messages []atom.Message) ([]atom.Message, error) {
	if agentLoop.configuration.StartPrompt == nil {
		return messages, nil
	}
	values := startprompt.Values{Session: session, Model: session.Model}
	if agentLoop.configuration.Instances != nil {
		if instance, found := agentLoop.configuration.Instances.Get(session.InstanceID); found {
			values.Workspace = instance.Spec().Workspace
			if values.Model == "" {
				values.Model = instance.Spec().DefaultModel
			}
		}
	}
	if toolRegistry := agentLoop.configuration.Registry; toolRegistry != nil {
		values.ToolList = func() []atom.ToolReference {
			var references []atom.ToolReference
			for _, tool := range toolRegistry.All() {
				references = append(references, atom.ToolReference{Name: tool.Name(), Categories: tool.Categories()})
			}
			return references
		}
		values.ToolInfo = func(name string) (atom.ToolSpec, error) {
			tool, found := toolRegistry.Get(name)
			if !found {
				return atom.ToolSpec{}, fmt.Errorf("tool %q is not registered", name)
			}
			specification := describeTool(tool)
			if name == "agent" {
				return agentLoop.addAgentModelChoices(session, specification)
			}
			return specification, nil
		}
	}
	prompt, operationError := agentLoop.configuration.StartPrompt.Render(operationContext, values)
	if operationError != nil {
		return nil, operationError
	}
	if strings.TrimSpace(prompt) == "" {
		return messages, nil
	}
	contextMessages := make([]atom.Message, 1, len(messages)+1)
	contextMessages[0] = atom.Message{SessionID: session.ID, Role: atom.RoleSystem, Content: []atom.Content{{Type: atom.Text, Text: prompt}}}
	return append(contextMessages, messages...), nil
}
