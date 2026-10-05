package loop

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type preparedModelCall struct {
	modelID  string
	model    atom.ModelInfo
	provider harness.Provider
	request  atom.Request
	session  atom.Session
}

// A nil call means policy stopped the turn. Resolve and validate after request
// middleware, so a plugin's model or content changes cannot bypass constraints.
func (agentLoop *Loop) prepareModelCall(operationContext context.Context, session atom.Session) (*preparedModelCall, error) {
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
	messages, operationError := agentLoop.contextBuilder.Build(operationContext, session.ID)
	if operationError != nil {
		return nil, operationError
	}
	messages, operationError = agentLoop.prependStartPrompt(operationContext, session, messages)
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
	request, operationError = agentLoop.contextBuilder.Fit(operationContext, request, modelInfo, modelProvider)
	if operationError != nil {
		return nil, operationError
	}
	return &preparedModelCall{modelID: modelID, model: modelInfo, provider: modelProvider, request: request, session: session}, nil
}
