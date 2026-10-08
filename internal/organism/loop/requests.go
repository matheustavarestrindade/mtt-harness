package loop

import (
	"context"
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/contextbuilder"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/taskstate"
)

type preparedModelCall struct {
	modelID           string
	model             atom.ModelInfo
	provider          harness.Provider
	request           atom.Request
	session           atom.Session
	taskStateRevision int64
	taskStateActive   bool
	operationContext  context.Context
	release           func()
	commitReminders   func(context.Context) error
}

// A nil call means policy stopped the turn. Resolve and validate after request
// middleware, so a plugin's model or content changes cannot bypass constraints.
func (agentLoop *Loop) prepareModelCall(operationContext context.Context, session atom.Session) (prepared *preparedModelCall, operationError error) {
	// Workers read one atomic selection at each request boundary, before prompt
	// rendering and middleware. An active stream retains its original selection.
	selection, saved, operationError := agentLoop.configuration.Store.Sessions().GetModelSelection(operationContext, session.ID)
	if operationError != nil {
		return nil, operationError
	}
	if saved {
		session.Model, session.ReasoningEffort = selection.Model, selection.ReasoningEffort
	}
	operationContext = harness.WithSession(operationContext, session)
	release := func() {}
	if agentLoop.configuration.Plugins != nil {
		operationContext, release, operationError = agentLoop.configuration.Plugins.BeginRequest(operationContext, session)
		if operationError != nil {
			return nil, operationError
		}
	}
	defer func() {
		if prepared == nil {
			release()
		}
	}()
	messages, operationError := agentLoop.contextBuilder.Build(operationContext, session.ID)
	if operationError != nil {
		return nil, operationError
	}
	messages, operationError = agentLoop.prependStartPrompt(operationContext, session, messages)
	if operationError != nil {
		return nil, operationError
	}
	taskState, operationError := agentLoop.configuration.Store.TaskStates().Get(operationContext, session.ID)
	if operationError != nil {
		return nil, operationError
	}
	messages, operationError = appendTaskStateContext(messages, session.ID, taskState)
	if operationError != nil {
		return nil, operationError
	}
	messages, operationError = agentLoop.pipeline.Context(operationContext, messages)
	if operationError != nil {
		return nil, operationError
	}
	modelID := session.Model
	if modelID == "" && agentLoop.configuration.Instances != nil {
		if instance, found := agentLoop.configuration.Instances.Get(session.InstanceID); found {
			modelID = instance.Spec().DefaultModel
		}
	}
	tools, operationError := agentLoop.sessionToolDefinitions(operationContext, session)
	if operationError != nil {
		return nil, operationError
	}
	request := atom.Request{Model: modelID, Messages: messages, Tools: tools, ReasoningEffort: session.ReasoningEffort}
	request, operationError = agentLoop.pipeline.Request(operationContext, request)
	if operationError != nil {
		return nil, operationError
	}
	if request.Model == "" {
		request.Model = modelID
	}
	modelInfo, modelProvider, operationError := agentLoop.resolveModel(session.InstanceID, request.Model)
	if operationError != nil {
		return nil, operationError
	}
	if operationError := validateModelMedia(modelInfo, request.Messages); operationError != nil {
		return nil, operationError
	}
	if operationError := modelInfo.ValidateReasoningEffort(request.ReasoningEffort); operationError != nil {
		return nil, operationError
	}
	verdict, operationError := agentLoop.pipeline.DecideRequest(operationContext, request)
	if operationError != nil {
		return nil, operationError
	}
	allowed, operationError := agentLoop.resolvePermissionVerdict(operationContext, session, "model.request", verdict)
	if operationError != nil || !allowed {
		return nil, operationError
	}
	modelID = modelProvider.Name() + "/" + modelInfo.ID
	request.Model = modelInfo.ID
	if !modelInfo.Tools && !modelInfo.ToolSupportUnknown {
		request.Tools = nil
	}
	managed := false
	inputCeiling := 0
	measure := func(measurementContext context.Context, candidate atom.Request) (harness.RequestBudget, error) {
		return contextbuilder.MeasureRequest(measurementContext, candidate, modelInfo, modelProvider)
	}
	measureProjected := measure
	reminderRuntime, hasReminders := agentLoop.configuration.Plugins.(harness.RequestReminderRuntime)
	if hasReminders {
		measureProjected = func(measurementContext context.Context, candidate atom.Request) (harness.RequestBudget, error) {
			projected, projectionError := reminderRuntime.ProjectReminders(measurementContext, session, candidate)
			if projectionError != nil {
				return harness.RequestBudget{}, projectionError
			}
			return measure(measurementContext, projected)
		}
	}
	if agentLoop.configuration.Plugins != nil {
		selection, selectionError := agentLoop.configuration.Plugins.PrepareContext(operationContext, harness.ContextRequest{Session: session, Request: request, Model: modelInfo, ModelID: modelID, Measure: measureProjected})
		if selectionError != nil {
			return nil, selectionError
		}
		request, managed = selection.Request, selection.Managed
		inputCeiling = selection.InputCeiling
		if managed {
			budget, budgetError := measureProjected(operationContext, request)
			if budgetError != nil {
				return nil, budgetError
			}
			if budget.InputLimit > 0 && budget.InputTokens > budget.InputLimit {
				return nil, fmt.Errorf("plugin context uses %d tokens; input budget is %d", budget.InputTokens, budget.InputLimit)
			}
		}
	}
	if !managed {
		request, operationError = agentLoop.contextBuilder.FitMeasured(operationContext, request, measureProjected)
		if operationError != nil {
			return nil, operationError
		}
	}
	var commitReminders func(context.Context) error
	if hasReminders {
		budget, budgetError := measure(operationContext, request)
		if budgetError != nil {
			return nil, budgetError
		}
		plan, planError := reminderRuntime.PrepareReminders(operationContext, harness.ContextRequest{Session: session, Request: request, Model: modelInfo, ModelID: modelID, Budget: budget, Measure: measure, InputCeiling: inputCeiling})
		if planError != nil {
			return nil, planError
		}
		request, commitReminders = plan.Request, plan.Commit
		finalBudget, budgetError := measure(operationContext, request)
		if budgetError != nil {
			return nil, budgetError
		}
		ceiling := finalBudget.InputLimit
		if inputCeiling > 0 && (ceiling == 0 || inputCeiling < ceiling) {
			ceiling = inputCeiling
		}
		if ceiling > 0 && finalBudget.InputTokens > ceiling {
			return nil, fmt.Errorf("request reminders use %d tokens; input ceiling is %d", finalBudget.InputTokens, ceiling)
		}
	}
	if operationError := validateModelMedia(modelInfo, request.Messages); operationError != nil {
		return nil, operationError
	}
	return &preparedModelCall{modelID: modelID, model: modelInfo, provider: modelProvider, request: request, session: session, taskStateRevision: taskState.Revision, taskStateActive: taskstate.Active(taskState), operationContext: operationContext, release: release, commitReminders: commitReminders}, nil
}
